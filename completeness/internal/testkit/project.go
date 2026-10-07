// Package testkit holds what the tests of the module share: the fixture projects
// with a change since their base, builders of the discriminating-power and
// alignment Statements other modules would have written for them, and the answer
// sources a fake host plays them with. It is never linked into the binary.
package testkit

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/RachidChabane/spec-proof/completeness/internal/domain/criterion"
)

// Names of the fixture projects under `fixtures/`.
const (
	PythonLedger = "ledger-python"
	GoLedger     = "ledger-go"
	// ReportFile is the recorded Shallnot report of a fixture project.
	ReportFile = "shallnot-report.json"
	// RecordedShallnot is the script that stands for the shallnot executable.
	RecordedShallnot = "shallnot-recorded.sh"
	// SpecFile is the spec of every fixture project.
	SpecFile = "specs/invoicing.md"

	baseDirectory   = "base"
	changeDirectory = "change"
	directoryMode   = 0o755
	fileMode        = 0o644
)

// Criteria of the fixture projects and the statement digests the recorded reports
// give them.
var (
	Totals  = criterion.Ref{ID: "INV-1", Revision: 1}
	Display = criterion.Ref{ID: "INV-2", Revision: 1}
)

const (
	TotalsDigest  = "1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d5e6f708192a3b4c5d6e7f809"
	DisplayDigest = "9f8e7d6c5b4a39281706f5e4d3c2b1a09f8e7d6c5b4a39281706f5e4d3c2b1a0"
)

// FixturesDir is the absolute path of the fixtures directory of the module.
func FixturesDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "fixtures")
}

// ReportPath is the recorded Shallnot report of a fixture project.
func ReportPath(fixture string) string {
	return filepath.Join(FixturesDir(), fixture, ReportFile)
}

// CopyTree copies a directory tree.
func CopyTree(t testing.TB, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, relative)
		if entry.IsDir() {
			if entry.Name() == "__pycache__" {
				return fs.SkipDir
			}
			return os.MkdirAll(target, directoryMode)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, content, fileMode)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Git runs git in a directory with the user's own configuration kept out of it.
func Git(t testing.TB, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false"}, args...)...)
	command.Dir = dir
	command.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=fixture", "GIT_AUTHOR_EMAIL=fixture@example.com",
		"GIT_COMMITTER_NAME=fixture", "GIT_COMMITTER_EMAIL=fixture@example.com",
		"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z",
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

// Materialize builds the project of a fixture in a temporary directory: its base
// is committed to a repository on `main`, and the files of its change are then
// written over the work tree, new files left untracked. It returns the root.
func Materialize(t testing.TB, fixture string) string {
	t.Helper()
	root := filepath.Join(resolvedTempDir(t), "project")
	CopyTree(t, filepath.Join(FixturesDir(), fixture, baseDirectory), root)
	Git(t, root, "init", "-q", "-b", "main")
	Git(t, root, "add", "-A")
	Git(t, root, "commit", "-q", "-m", "base")
	CopyTree(t, filepath.Join(FixturesDir(), fixture, changeDirectory), root)
	return root
}

// MaterializeWithoutRepository builds the project of a fixture with its change
// applied and no version control at all.
func MaterializeWithoutRepository(t testing.TB, fixture string) string {
	t.Helper()
	root := filepath.Join(resolvedTempDir(t), "project")
	CopyTree(t, filepath.Join(FixturesDir(), fixture, baseDirectory), root)
	CopyTree(t, filepath.Join(FixturesDir(), fixture, changeDirectory), root)
	return root
}

// resolvedTempDir is a temporary directory with its symbolic links resolved, so
// that the root a command line resolves is the root the test built.
func resolvedTempDir(t testing.TB) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
