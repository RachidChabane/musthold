package hostgit_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/hostgit"
	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/process"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostbundle"
)

const testRef = "refs/spec-proof/host/alignment/abc"

// runGit runs the git binary in dir for the test's own set-up and checks.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-c", "user.name=test", "-c", "user.email=test@localhost", "-c", "commit.gpgsign=false"}, args...)...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func repository(t *testing.T) (string, hostgit.Repository) {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	if err := os.WriteFile(filepath.Join(root, "code.py"), []byte("x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "code.py")
	runGit(t, root, "commit", "-q", "-m", "first")
	return root, hostgit.Repository{Process: process.New(), Root: root}
}

var files = []hostbundle.File{
	{Path: "manifest.json", Content: []byte("{\"a\": 1}\n")},
	{Path: "responses/one.0.json", Content: []byte("no trailing newline")},
	{Path: "responses/two.0.json", Content: []byte("two\n")},
}

func TestPublish(t *testing.T) {
	t.Run("the files become a tree under the ref, byte for byte, and the working tree and branch are untouched [verifies SP-748~1]", func(t *testing.T) {
		root, repo := repository(t)
		branchBefore, headBefore := runGit(t, root, "symbolic-ref", "HEAD"), runGit(t, root, "rev-parse", "HEAD")
		commit, err := repo.Publish(context.Background(), testRef, files)
		if err != nil {
			t.Fatal(err)
		}
		if got := runGit(t, root, "rev-parse", testRef); got != commit {
			t.Fatalf("ref = %s, want %s", got, commit)
		}
		if got := runGit(t, root, "show", testRef+":responses/one.0.json"); got != "no trailing newline" {
			t.Errorf("blob = %q", got)
		}
		listing := runGit(t, root, "ls-tree", "-r", "--name-only", testRef)
		if listing != "manifest.json\nresponses/one.0.json\nresponses/two.0.json" {
			t.Errorf("tree = %q", listing)
		}
		if runGit(t, root, "symbolic-ref", "HEAD") != branchBefore || runGit(t, root, "rev-parse", "HEAD") != headBefore {
			t.Error("the current branch moved")
		}
		if status := runGit(t, root, "status", "--porcelain"); status != "" {
			t.Errorf("the working tree or the index changed: %q", status)
		}
	})

	t.Run("publishing again replaces the ref [verifies SP-748~1]", func(t *testing.T) {
		root, repo := repository(t)
		first, err := repo.Publish(context.Background(), testRef, files)
		if err != nil {
			t.Fatal(err)
		}
		second, err := repo.Publish(context.Background(), testRef, files[:1])
		if err != nil || second == first {
			t.Fatalf("second = %s, %v", second, err)
		}
		if got := runGit(t, root, "ls-tree", "-r", "--name-only", testRef); got != "manifest.json" {
			t.Errorf("tree = %q", got)
		}
	})

	t.Run("a tracked file that differs from the head commit is reported, an untracked one is not [verifies SP-748~1]", func(t *testing.T) {
		root, repo := repository(t)
		if dirty, err := repo.HasUncommittedChanges(context.Background()); err != nil || dirty {
			t.Fatalf("clean tree: %v, %v", dirty, err)
		}
		if err := os.WriteFile(filepath.Join(root, "untracked.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if dirty, _ := repo.HasUncommittedChanges(context.Background()); dirty {
			t.Fatal("an untracked file made the tree dirty")
		}
		if err := os.WriteFile(filepath.Join(root, "code.py"), []byte("x = 2\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if dirty, err := repo.HasUncommittedChanges(context.Background()); err != nil || !dirty {
			t.Fatalf("edited tree: %v, %v", dirty, err)
		}
	})

	t.Run("the head commit is read from the repository [verifies SP-748~1]", func(t *testing.T) {
		root, repo := repository(t)
		head, err := repo.HeadSHA(context.Background())
		if err != nil || head != runGit(t, root, "rev-parse", "HEAD") {
			t.Fatalf("head = %q, %v", head, err)
		}
	})

	t.Run("the ref can be pushed to a remote, replacing its copy [verifies SP-748~1]", func(t *testing.T) {
		root, repo := repository(t)
		remote := t.TempDir()
		runGit(t, remote, "init", "-q", "--bare")
		runGit(t, root, "remote", "add", "origin", remote)
		for range 2 {
			if _, err := repo.Publish(context.Background(), testRef, files); err != nil {
				t.Fatal(err)
			}
			if err := repo.PushRef(context.Background(), "origin", testRef); err != nil {
				t.Fatal(err)
			}
		}
		if got, want := runGit(t, remote, "rev-parse", testRef), runGit(t, root, "rev-parse", testRef); got != want {
			t.Fatalf("remote ref = %s, want %s", got, want)
		}
	})

	t.Run("a directory that is not a repository, and an adapter without a runner, fail [verifies SP-748~1]", func(t *testing.T) {
		outside := hostgit.Repository{Process: process.New(), Root: t.TempDir()}
		if _, err := outside.HeadSHA(context.Background()); !errors.Is(err, hostgit.ErrGit) {
			t.Errorf("head outside a repository: %v", err)
		}
		if _, err := outside.Publish(context.Background(), testRef, files); !errors.Is(err, hostgit.ErrGit) {
			t.Errorf("publish outside a repository: %v", err)
		}
		if _, err := (hostgit.Repository{}).HeadSHA(context.Background()); !errors.Is(err, hostgit.ErrNoRunner) {
			t.Errorf("no runner: %v", err)
		}
		_, repo := repository(t)
		if err := repo.PushRef(context.Background(), "nowhere", testRef); !errors.Is(err, hostgit.ErrGit) {
			t.Errorf("push to an unknown remote: %v", err)
		}
	})
}
