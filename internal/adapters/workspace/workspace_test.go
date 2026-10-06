package workspace_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/internal/adapters/workspace"
	"github.com/RachidChabane/spec-proof/internal/domain/ecosystem"
)

const (
	sourceFile   = "pkg/app.py"
	originalText = "VALUE = 1\n"
	changedText  = "VALUE = 2\n"
)

var layout = ecosystem.Layout{
	ExcludedDirs:   []string{"__pycache__", ".pytest_cache"},
	DependencyDirs: []string{".venv"},
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func exists(root, rel string) bool {
	_, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}

func project(t *testing.T) string {
	t.Helper()
	source := t.TempDir()
	write(t, source, sourceFile, originalText)
	write(t, source, "pkg/__pycache__/app.cpython-313.pyc", "bytecode")
	write(t, source, ".pytest_cache/README.md", "cache")
	write(t, source, ".git/HEAD", "ref")
	write(t, source, ".venv/lib/site.py", "dependency")
	return source
}

func newManager(t *testing.T, opts ...workspace.Option) *workspace.Manager {
	t.Helper()
	manager, err := workspace.New(t.TempDir(), opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	return manager
}

func TestPrepareCopiesTheProjectOnceOutsideTheRepository(t *testing.T) {
	source := project(t)
	manager := newManager(t)
	reference, err := manager.Prepare(context.Background(), source, layout)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("the reference lives under the run directory of the work root [verifies SP-68~1]", func(t *testing.T) {
		if !strings.HasPrefix(reference.Dir, manager.RunDir()) || filepath.Base(filepath.Dir(manager.RunDir())) != workspace.RootDirName {
			t.Fatalf("reference %q, run %q", reference.Dir, manager.RunDir())
		}
		if strings.HasPrefix(reference.Dir, source) {
			t.Fatal("the reference lies inside the project")
		}
	})

	t.Run("excluded directories and version control data are not copied [verifies SP-68~1]", func(t *testing.T) {
		for _, rel := range []string{"pkg/__pycache__", ".pytest_cache", ".git"} {
			if exists(reference.Dir, rel) {
				t.Errorf("%s was copied", rel)
			}
		}
		if !exists(reference.Dir, sourceFile) || !exists(reference.Dir, ".venv/lib/site.py") {
			t.Fatal("source or dependencies are missing")
		}
	})

	t.Run("results are kept outside the workspace tree [verifies SP-68~1]", func(t *testing.T) {
		if strings.HasPrefix(reference.ResultsDir, reference.Dir) {
			t.Fatalf("results %q inside %q", reference.ResultsDir, reference.Dir)
		}
		if _, err := os.Stat(reference.ResultsDir); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("the default work root is the temporary directory [verifies SP-68~1]", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("TMPDIR", tmp)
		defaultManager, err := workspace.New("")
		if err != nil {
			t.Fatal(err)
		}
		defer defaultManager.Close()
		if !strings.HasPrefix(defaultManager.RunDir(), tmp) {
			t.Fatalf("run dir %q is not under %q", defaultManager.RunDir(), tmp)
		}
	})
}

func cloneWith(t *testing.T, cloners ...workspace.Cloner) (*workspace.Manager, ecosystem.Workspace, ecosystem.Workspace) {
	t.Helper()
	manager := newManager(t, workspace.WithCloners(cloners...))
	reference, err := manager.Prepare(context.Background(), project(t), layout)
	if err != nil {
		t.Fatal(err)
	}
	write(t, reference.Dir, "pkg/__pycache__/app.cpython-313.pyc", "built")
	mutant, err := manager.Clone(context.Background(), reference, layout)
	if err != nil {
		t.Fatal(err)
	}
	return manager, reference, mutant
}

func TestCloneDerivesMutantWorkspacesFromTheBuiltReference(t *testing.T) {
	strategies := map[string][]workspace.Cloner{
		"platform default": workspace.DefaultCloners(),
		"plain copy":       {workspace.PlainCloner{}},
	}
	for name, cloners := range strategies {
		t.Run(name+" clone keeps the built tree, drops excluded directories and is independent [verifies SP-69~1]", func(t *testing.T) {
			manager, reference, mutant := cloneWith(t, cloners...)
			if !exists(mutant.Dir, sourceFile) || !exists(mutant.Dir, ".venv/lib/site.py") {
				t.Fatal("the clone lost source or dependencies")
			}
			if exists(mutant.Dir, "pkg/__pycache__") {
				t.Fatal("bytecode was cloned")
			}
			if err := manager.WriteFile(mutant, layout, sourceFile, []byte(changedText)); err != nil {
				t.Fatal(err)
			}
			got, err := manager.ReadFile(reference, sourceFile)
			if err != nil || string(got) != originalText {
				t.Fatalf("the reference changed: %q %v", got, err)
			}
		})
	}

	t.Run("the plain copy links dependency directories instead of copying them [verifies SP-69~1]", func(t *testing.T) {
		_, reference, mutant := cloneWith(t, workspace.PlainCloner{})
		info, err := os.Lstat(filepath.Join(mutant.Dir, ".venv"))
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("dependency dir is not a symlink: %v %v", info, err)
		}
		target, _ := os.Readlink(filepath.Join(mutant.Dir, ".venv"))
		if target != filepath.Join(reference.Dir, ".venv") {
			t.Fatalf("link target = %q", target)
		}
	})

	t.Run("the next strategy is tried when one fails [verifies SP-69~1]", func(t *testing.T) {
		_, _, mutant := cloneWith(t, failingCloner{}, workspace.PlainCloner{})
		if !exists(mutant.Dir, sourceFile) {
			t.Fatal("the fallback clone is empty")
		}
	})

	t.Run("a failure of every strategy is reported [verifies SP-69~1]", func(t *testing.T) {
		manager := newManager(t, workspace.WithCloners(failingCloner{}))
		reference, err := manager.Prepare(context.Background(), project(t), layout)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := manager.Clone(context.Background(), reference, layout); err == nil {
			t.Fatal("expected a clone error")
		}
	})

	t.Run("each clone gets its own directory and results [verifies SP-69~1]", func(t *testing.T) {
		manager := newManager(t)
		reference, _ := manager.Prepare(context.Background(), project(t), layout)
		a, errA := manager.Clone(context.Background(), reference, layout)
		b, errB := manager.Clone(context.Background(), reference, layout)
		if errA != nil || errB != nil || a.Dir == b.Dir || a.ResultsDir == b.ResultsDir {
			t.Fatalf("%v %v %v", errA, errB, a == b)
		}
		if err := manager.Release(a); err != nil || exists(a.Dir, ".") {
			t.Fatalf("release: %v", err)
		}
	})
}

type failingCloner struct{}

func (failingCloner) Name() string { return "failing" }
func (failingCloner) Clone(_ context.Context, _, destination string, _ ecosystem.Layout) error {
	_ = os.MkdirAll(destination, 0o755)
	return errors.New("copy-on-write is unavailable")
}

func TestWriteFileKeepsTheSpliceNewerThanBuildOutputs(t *testing.T) {
	manager, reference, mutant := cloneWith(t, workspace.PlainCloner{})

	t.Run("the written file is newer than a build output dated in the future [verifies SP-70~1]", func(t *testing.T) {
		future := time.Now().Add(time.Hour)
		output := filepath.Join(mutant.Dir, "build", "app.o")
		write(t, mutant.Dir, "build/app.o", "object")
		if err := os.Chtimes(output, future, future); err != nil {
			t.Fatal(err)
		}
		if err := manager.WriteFile(mutant, layout, sourceFile, []byte(changedText)); err != nil {
			t.Fatal(err)
		}
		written, _ := os.Stat(filepath.Join(mutant.Dir, sourceFile))
		built, _ := os.Stat(output)
		if !written.ModTime().After(built.ModTime()) {
			t.Fatalf("splice %s is not after output %s", written.ModTime(), built.ModTime())
		}
	})

	t.Run("a same-size rewrite in the same second is still newer [verifies SP-70~1]", func(t *testing.T) {
		before, _ := os.Stat(filepath.Join(mutant.Dir, sourceFile))
		if err := manager.WriteFile(mutant, layout, sourceFile, []byte(strings.Replace(changedText, "2", "3", 1))); err != nil {
			t.Fatal(err)
		}
		after, _ := os.Stat(filepath.Join(mutant.Dir, sourceFile))
		if !after.ModTime().After(before.ModTime()) || after.Size() != before.Size() {
			t.Fatalf("before %s after %s", before.ModTime(), after.ModTime())
		}
	})

	t.Run("dependency directories do not push the splice time [verifies SP-70~1]", func(t *testing.T) {
		far := time.Now().Add(100 * time.Hour)
		if err := os.Chtimes(filepath.Join(reference.Dir, ".venv", "lib", "site.py"), far, far); err != nil {
			t.Fatal(err)
		}
		if err := manager.WriteFile(reference, layout, sourceFile, []byte(changedText)); err != nil {
			t.Fatal(err)
		}
		written, _ := os.Stat(filepath.Join(reference.Dir, sourceFile))
		if written.ModTime().After(time.Now().Add(2 * time.Hour)) {
			t.Fatalf("splice time %s follows a dependency", written.ModTime())
		}
	})

	t.Run("paths leaving the workspace are refused [verifies SP-70~1]", func(t *testing.T) {
		for _, file := range []string{"../escape.py", "/abs.py", "a/../b.py", ""} {
			if err := manager.WriteFile(mutant, layout, file, nil); !errors.Is(err, workspace.ErrOutsideWorkspace) {
				t.Errorf("WriteFile(%q) = %v", file, err)
			}
			if _, err := manager.ReadFile(mutant, file); !errors.Is(err, workspace.ErrOutsideWorkspace) {
				t.Errorf("ReadFile(%q) = %v", file, err)
			}
		}
	})
}
