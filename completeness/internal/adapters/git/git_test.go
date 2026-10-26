package git_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/completeness/internal/adapters/git"
	"github.com/RachidChabane/musthold/completeness/internal/adapters/process"
	"github.com/RachidChabane/musthold/completeness/internal/app/ports"
	"github.com/RachidChabane/musthold/completeness/internal/testkit"
)

func changes(t *testing.T, root, base string) (ports.ChangeSet, error) {
	t.Helper()
	return git.Source{Process: process.New(), Root: root}.Changes(context.Background(), base)
}

func paths(set ports.ChangeSet) []string {
	found := make([]string, len(set.Changes))
	for index, change := range set.Changes {
		found[index] = change.Path
	}
	slices.Sort(found)
	return found
}

func byPath(set ports.ChangeSet, path string) ports.Change {
	for _, change := range set.Changes {
		if change.Path == path {
			return change
		}
	}
	return ports.Change{}
}

func output(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = root
	command.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	out, err := command.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

func TestChanges(t *testing.T) {
	t.Run("the work tree is compared with the base: modified files with both sides, new files untracked with only the after side [verifies SP-507~1]", func(t *testing.T) {
		root := testkit.Materialize(t, testkit.PythonLedger)
		set, err := changes(t, root, "")
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"src/invoicing/receipts.py", "src/invoicing/totals.py", "tests/test_totals.py"}
		if got := paths(set); !slices.Equal(got, want) {
			t.Fatalf("changed files %v, want %v", got, want)
		}
		if set.Base != output(t, root, "rev-parse", "HEAD") {
			t.Fatalf("base %q is not the commit", set.Base)
		}
		modified := byPath(set, "src/invoicing/totals.py")
		if !strings.Contains(string(modified.Before), "0.20") || !strings.Contains(string(modified.After), "0.25") {
			t.Fatalf("modified file: before %q after %q", modified.Before, modified.After)
		}
		added := byPath(set, "src/invoicing/receipts.py")
		if added.Before != nil || !strings.Contains(string(added.After), "receipt_line") {
			t.Fatalf("added file: %+v", added)
		}
	})

	t.Run("the repository is left as it was, the index included [verifies SP-507~1]", func(t *testing.T) {
		root := testkit.Materialize(t, testkit.PythonLedger)
		before := output(t, root, "status", "--porcelain")
		if _, err := changes(t, root, ""); err != nil {
			t.Fatal(err)
		}
		if after := output(t, root, "status", "--porcelain"); after != before || !strings.Contains(after, "?? src/invoicing/receipts.py") {
			t.Fatalf("status changed from %q to %q", before, after)
		}
		if staged := output(t, root, "diff", "--cached", "--name-only"); staged != "" {
			t.Fatalf("staged files %q", staged)
		}
	})

	t.Run("by default the base is the merge base with the default branch, so committed work counts [verifies SP-507~1]", func(t *testing.T) {
		root := testkit.Materialize(t, testkit.GoLedger)
		trunk := output(t, root, "rev-parse", "HEAD")
		testkit.Git(t, root, "checkout", "-q", "-b", "feature")
		testkit.Git(t, root, "add", "-A")
		testkit.Git(t, root, "commit", "-q", "-m", "change")
		set, err := changes(t, root, "")
		if err != nil {
			t.Fatal(err)
		}
		if set.Base != trunk {
			t.Fatalf("base %q, want the merge base %q", set.Base, trunk)
		}
		if got := paths(set); !slices.Contains(got, "totals.go") {
			t.Fatalf("changed files %v", got)
		}
	})

	t.Run("the default branch is the one init.defaultBranch names [verifies SP-507~1]", func(t *testing.T) {
		root := testkit.Materialize(t, testkit.GoLedger)
		trunk := output(t, root, "rev-parse", "HEAD")
		testkit.Git(t, root, "branch", "-m", "trunk")
		testkit.Git(t, root, "config", "init.defaultBranch", "trunk")
		testkit.Git(t, root, "checkout", "-q", "-b", "feature")
		testkit.Git(t, root, "add", "-A")
		testkit.Git(t, root, "commit", "-q", "-m", "change")
		testkit.Git(t, root, "checkout", "-q", "trunk")
		testkit.Git(t, root, "checkout", "-q", "feature")
		set, err := changes(t, root, "")
		if err != nil || set.Base != trunk {
			t.Fatalf("base %q, want %q: %v", set.Base, trunk, err)
		}
	})

	t.Run("an explicit base reference is used as it is [verifies SP-507~1]", func(t *testing.T) {
		root := testkit.Materialize(t, testkit.GoLedger)
		testkit.Git(t, root, "add", "-A")
		testkit.Git(t, root, "commit", "-q", "-m", "change")
		first := output(t, root, "rev-parse", "HEAD~1")
		set, err := changes(t, root, "HEAD~1")
		if err != nil || set.Base != first || !slices.Contains(paths(set), "totals.go") {
			t.Fatalf("base %q changes %v: %v", set.Base, paths(set), err)
		}
		if again, err := changes(t, root, ""); err != nil || len(again.Changes) != 0 {
			t.Fatalf("on the default branch with nothing uncommitted: %v %v", again.Changes, err)
		}
	})

	t.Run("a deleted file has no after side and a renamed file keeps the path it had [verifies SP-507~1]", func(t *testing.T) {
		root := testkit.Materialize(t, testkit.PythonLedger)
		if err := os.Remove(filepath.Join(root, "src/invoicing/receipts.py")); err != nil {
			t.Fatal(err)
		}
		testkit.Git(t, root, "rm", "-q", "-f", "tests/test_totals.py")
		testkit.Git(t, root, "checkout", "-q", "HEAD", "--", "src/invoicing/totals.py")
		testkit.Git(t, root, "mv", "src/invoicing/totals.py", "src/invoicing/amounts.py")
		appended, err := os.OpenFile(filepath.Join(root, "src/invoicing/amounts.py"), os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := appended.WriteString("\n\ndef net(total):\n    return total\n"); err != nil {
			t.Fatal(err)
		}
		if err := appended.Close(); err != nil {
			t.Fatal(err)
		}
		set, err := changes(t, root, "")
		if err != nil {
			t.Fatal(err)
		}
		deleted := byPath(set, "tests/test_totals.py")
		if deleted.After != nil || deleted.Before == nil {
			t.Fatalf("deleted file %+v", deleted)
		}
		renamed := byPath(set, "src/invoicing/amounts.py")
		if renamed.OldPath != "src/invoicing/totals.py" || !strings.Contains(string(renamed.Before), "0.20") || !strings.Contains(string(renamed.After), "def net") {
			t.Fatalf("renamed file %+v", renamed)
		}
	})

	t.Run("a directory that is not a repository, a repository without a commit and an unknown base have no base to compare with [verifies SP-507~1]", func(t *testing.T) {
		if _, err := changes(t, t.TempDir(), ""); !errors.Is(err, ports.ErrNoRepository) {
			t.Fatalf("no repository: %v", err)
		}
		empty := t.TempDir()
		testkit.Git(t, empty, "init", "-q", "-b", "main")
		if _, err := changes(t, empty, ""); !errors.Is(err, ports.ErrNoRepository) {
			t.Fatalf("no commit: %v", err)
		}
		root := testkit.Materialize(t, testkit.GoLedger)
		if _, err := changes(t, root, "no-such-branch"); !errors.Is(err, ports.ErrNoRepository) {
			t.Fatalf("unknown base: %v", err)
		}
	})

	t.Run("a repository with no default branch compares the work tree with HEAD [verifies SP-507~1]", func(t *testing.T) {
		root := testkit.Materialize(t, testkit.GoLedger)
		head := output(t, root, "rev-parse", "HEAD")
		testkit.Git(t, root, "branch", "-m", "elsewhere")
		set, err := changes(t, root, "")
		if err != nil || set.Base != head || !slices.Contains(paths(set), "totals.go") {
			t.Fatalf("base %q changes %v: %v", set.Base, paths(set), err)
		}
	})
}
