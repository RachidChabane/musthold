package gitsource_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/gitsource"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/process"
)

const (
	verifiesGit = " [verifies SP-2120~1]"
	fileMode    = 0o644
)

func TestParseChangedLines(t *testing.T) {
	diff := strings.Join([]string{
		"diff --git a/src/parse.py b/src/parse.py",
		"--- a/src/parse.py",
		"+++ b/src/parse.py",
		"@@ -3 +3 @@ def split(line):",
		"-    start = 0",
		"+    start = 1",
		"@@ -10,0 +11,2 @@",
		"+    a = 1",
		"+    b = 2",
		"@@ -20,3 +21,0 @@",
		"diff --git a/old.py b/old.py",
		"--- a/old.py",
		"+++ /dev/null",
		"@@ -1,2 +0,0 @@",
	}, "\n")
	t.Run("the new-side lines of each hunk are changed, a deletion marks the line it follows [verifies SP-2120~1]", func(t *testing.T) {
		got := gitsource.ParseChangedLines(diff)
		want := map[string][]int{"src/parse.py": {3, 11, 12, 21}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("changed = %v", got)
		}
	})
}

func TestNameOfRemote(t *testing.T) {
	cases := map[string]string{
		"https://github.com/acme/invoicing.git": "acme/invoicing",
		"https://github.com/acme/invoicing":     "acme/invoicing",
		"git@github.com:acme/invoicing.git":     "acme/invoicing",
		"ssh://git@host:2222/acme/invoicing":    "acme/invoicing",
	}
	for url, want := range cases {
		t.Run(url+" names "+want+verifiesGit, func(t *testing.T) {
			if got, ok := gitsource.NameOfRemote(url); !ok || got != want {
				t.Fatalf("name = %q, %v", got, ok)
			}
		})
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-c", "user.name=scan", "-c", "user.email=scan@example.com", "-c", "commit.gpgsign=false"}, args...)...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func write(t *testing.T, dir, file, content string) {
	t.Helper()
	path := filepath.Join(dir, file)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), fileMode); err != nil {
		t.Fatal(err)
	}
}

func TestRepository(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	write(t, dir, "src/parse.py", "def split(line):\n    start = 0\n    return start\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "first")
	base := git(t, dir, "rev-parse", "HEAD")
	write(t, dir, "src/parse.py", "def split(line):\n    start = 1\n    return start\n")
	git(t, dir, "commit", "-qam", "second")
	head := git(t, dir, "rev-parse", "HEAD")
	repository := gitsource.Repository{Process: process.New(), Root: dir}
	ctx := context.Background()

	t.Run("the head and a base resolve to full commit ids [verifies SP-2120~1]", func(t *testing.T) {
		gotHead, err := repository.Head(ctx)
		if err != nil || gotHead != head {
			t.Fatalf("head = %q, %v", gotHead, err)
		}
		gotBase, err := repository.Resolve(ctx, "HEAD~1")
		if err != nil || gotBase != base {
			t.Fatalf("base = %q, %v", gotBase, err)
		}
	})
	t.Run("the changed lines are those of the head that differ from the base [verifies SP-2120~1]", func(t *testing.T) {
		changed, err := repository.ChangedLines(ctx, base)
		if err != nil || !reflect.DeepEqual(changed, map[string][]int{"src/parse.py": {2}}) {
			t.Fatalf("changed = %v, %v", changed, err)
		}
	})
	t.Run("the tracked files are listed [verifies SP-2120~1]", func(t *testing.T) {
		files, err := repository.TrackedFiles(ctx)
		if err != nil || !slices.Equal(files, []string{"src/parse.py"}) {
			t.Fatalf("files = %v, %v", files, err)
		}
	})
	t.Run("a tracked change makes the tree dirty, a scan report and an untracked file do not [verifies SP-2120~1]", func(t *testing.T) {
		write(t, dir, ".spec-proof/scan/abc/report.md", "report\n")
		write(t, dir, "notes.txt", "draft\n")
		if dirty, err := repository.Dirty(ctx); err != nil || dirty {
			t.Fatalf("dirty = %v, %v", dirty, err)
		}
		write(t, dir, "src/parse.py", "def split(line):\n    return 2\n")
		if dirty, err := repository.Dirty(ctx); err != nil || !dirty {
			t.Fatalf("dirty = %v, %v", dirty, err)
		}
	})
	t.Run("a repository without an origin is named after its directory [verifies SP-2120~1]", func(t *testing.T) {
		if name := repository.Name(ctx); name != filepath.Base(dir) {
			t.Fatalf("name = %q", name)
		}
	})
}
