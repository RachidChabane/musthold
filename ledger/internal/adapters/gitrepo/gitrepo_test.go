package gitrepo_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/ledger/internal/adapters/gitrepo"
	"github.com/RachidChabane/musthold/ledger/internal/domain/ledger"
	"github.com/RachidChabane/musthold/ledger/internal/domain/refusal"
	"github.com/RachidChabane/musthold/ledger/internal/testkit"
)

const (
	verifiesPlumbing = " [verifies SP-203~1]"
	verifiesRemote   = " [verifies SP-205~1]"
	verifiesAuditGit = " [verifies SP-207~1]"
)

var commitTime = time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)

func openRepository(t *testing.T) (*gitrepo.Repository, string) {
	t.Helper()
	directory := testkit.NewBareRepository(t)
	return gitrepo.New(directory, gitrepo.WithEnvironment(testkit.GitEnvironment(t)...)), directory
}

// appendFiles adds files on top of the ledger tip the way the append use case does,
// and returns the new tip.
func appendFiles(t *testing.T, repository *gitrepo.Repository, files map[string]string) ledger.ObjectID {
	t.Helper()
	tip, exists, err := repository.Tip()
	if err != nil {
		t.Fatal(err)
	}
	var previous, root *ledger.ObjectID
	if exists {
		previous = &tip
		tree, err := repository.TreeOf(tip)
		if err != nil {
			t.Fatal(err)
		}
		root = &tree
	}
	var additions []ledger.Addition
	for _, path := range slices.Sorted(keys(files)) {
		additions = append(additions, ledger.Addition{Path: path, Content: []byte(files[path])})
	}
	tree, _, err := ledger.ApplyAdditions(repository, root, additions)
	if err != nil {
		t.Fatal(err)
	}
	commit, err := repository.Commit(tree, previous, "test commit", commitTime)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Advance(previous, commit); err != nil {
		t.Fatal(err)
	}
	return commit
}

func keys(files map[string]string) func(func(string) bool) {
	return func(yield func(string) bool) {
		for key := range files {
			if !yield(key) {
				return
			}
		}
	}
}

func TestObjectsAndRefs(t *testing.T) {
	t.Run("a repository without the ledger branch has no tip"+verifiesPlumbing, func(t *testing.T) {
		repository, _ := openRepository(t)
		if _, exists, err := repository.Tip(); err != nil || exists {
			t.Fatalf("exists %v err %v", exists, err)
		}
	})

	t.Run("blobs and trees round-trip through real git objects"+verifiesPlumbing, func(t *testing.T) {
		repository, _ := openRepository(t)
		blob, err := repository.WriteBlob([]byte("content\x00with nul"))
		if err != nil {
			t.Fatal(err)
		}
		got, err := repository.ReadBlob(blob)
		if err != nil || string(got) != "content\x00with nul" {
			t.Fatalf("blob %q: %v", got, err)
		}
		tree, err := repository.WriteTree([]ledger.TreeEntry{
			{Mode: ledger.ModeBlob, Kind: ledger.EntryBlob, ID: blob, Name: "name with space"},
			{Mode: ledger.ModeBlob, Kind: ledger.EntryBlob, ID: blob, Name: "a~1"},
		})
		if err != nil {
			t.Fatal(err)
		}
		entries, err := repository.ReadTree(tree)
		if err != nil || len(entries) != 2 || entries[0].Name != "a~1" || entries[1].Name != "name with space" {
			t.Fatalf("entries %+v: %v", entries, err)
		}
	})

	t.Run("a commit becomes the tip and its files are listed"+verifiesPlumbing, func(t *testing.T) {
		repository, directory := openRepository(t)
		tip := appendFiles(t, repository, map[string]string{"proofs/m/s/k": "one", "seeds/h": "two"})
		got, exists, err := repository.Tip()
		if err != nil || !exists || got != tip {
			t.Fatalf("tip %s %v %v", got, exists, err)
		}
		files, err := repository.Files(tip)
		if err != nil {
			t.Fatal(err)
		}
		var paths []string
		for _, file := range files {
			paths = append(paths, file.Path)
		}
		if !slices.Equal(paths, []string{"proofs/m/s/k", "seeds/h"}) {
			t.Fatalf("paths %v", paths)
		}
		if branches := testkit.Git(t, directory, "branch", "--list", "--format=%(refname)"); branches != "refs/heads/"+format.LedgerBranch {
			t.Fatalf("branches %q", branches)
		}
		if message := testkit.Git(t, directory, "log", "-1", "--format=%an <%ae> %at %s", "refs/heads/"+format.LedgerBranch); message != "musthold ledger <ledger@musthold.invalid> 1772366400 test commit" {
			t.Fatalf("commit %q", message)
		}
	})

	t.Run("a branch advances only by fast-forward from the value it still has"+verifiesPlumbing, func(t *testing.T) {
		repository, _ := openRepository(t)
		first := appendFiles(t, repository, map[string]string{"proofs/a": "1"})
		second := appendFiles(t, repository, map[string]string{"proofs/b": "2"})

		tree, err := repository.TreeOf(first)
		if err != nil {
			t.Fatal(err)
		}
		sideways, err := repository.Commit(tree, nil, "unrelated root", commitTime)
		if err != nil {
			t.Fatal(err)
		}
		err = repository.Advance(&second, sideways)
		if !errors.Is(err, refusal.ErrRefused) {
			t.Fatalf("a non-descendant must be refused, got %v", err)
		}
		if err := repository.Advance(&first, second); err == nil {
			t.Fatal("a stale previous value must fail the compare-and-swap")
		}
		if err := repository.Advance(nil, second); err == nil {
			t.Fatal("creating an existing branch must fail")
		}
		if tip, _, _ := repository.Tip(); tip != second {
			t.Fatalf("tip moved to %s", tip)
		}
	})
}

func TestRemote(t *testing.T) {
	newPair := func(t *testing.T) (local, remote *gitrepo.Repository, remoteDirectory string) {
		t.Helper()
		local, localDirectory := openRepository(t)
		remote, remoteDirectory = openRepository(t)
		testkit.Git(t, localDirectory, "remote", "add", "origin", remoteDirectory)
		return local, remote, remoteDirectory
	}

	t.Run("a remote without the ledger branch is not an error"+verifiesRemote, func(t *testing.T) {
		local, _, _ := newPair(t)
		if err := gitrepo.NewRemote(local, "origin").Fetch(); err != nil {
			t.Fatal(err)
		}
		if _, exists, _ := local.Tip(); exists {
			t.Fatal("a branch appeared")
		}
	})

	t.Run("a push publishes the ledger without forcing"+verifiesRemote, func(t *testing.T) {
		local, remote, _ := newPair(t)
		tip := appendFiles(t, local, map[string]string{"proofs/a": "1"})
		if err := gitrepo.NewRemote(local, "origin").Push(); err != nil {
			t.Fatal(err)
		}
		if remoteTip, exists, _ := remote.Tip(); !exists || remoteTip != tip {
			t.Fatalf("remote tip %s %v", remoteTip, exists)
		}
		second := appendFiles(t, local, map[string]string{"proofs/b": "2"})
		if err := gitrepo.NewRemote(local, "origin").Push(); err != nil {
			t.Fatal(err)
		}
		if remoteTip, _, _ := remote.Tip(); remoteTip != second {
			t.Fatalf("remote tip %s", remoteTip)
		}
	})

	t.Run("a fetch creates the local branch from the remote one"+verifiesRemote, func(t *testing.T) {
		local, remote, _ := newPair(t)
		tip := appendFiles(t, remote, map[string]string{"proofs/a": "1"})
		if err := gitrepo.NewRemote(local, "origin").Fetch(); err != nil {
			t.Fatal(err)
		}
		if got, exists, _ := local.Tip(); !exists || got != tip {
			t.Fatalf("local tip %s %v", got, exists)
		}
	})

	t.Run("a fetch fast-forwards a local branch that is behind"+verifiesRemote, func(t *testing.T) {
		local, remote, _ := newPair(t)
		appendFiles(t, remote, map[string]string{"proofs/a": "1"})
		if err := gitrepo.NewRemote(local, "origin").Fetch(); err != nil {
			t.Fatal(err)
		}
		next := appendFiles(t, remote, map[string]string{"proofs/b": "2"})
		if err := gitrepo.NewRemote(local, "origin").Fetch(); err != nil {
			t.Fatal(err)
		}
		if got, _, _ := local.Tip(); got != next {
			t.Fatalf("local tip %s, want %s", got, next)
		}
	})

	t.Run("a fetch keeps a local branch that is ahead and a later push publishes it"+verifiesRemote, func(t *testing.T) {
		local, remote, _ := newPair(t)
		appendFiles(t, local, map[string]string{"proofs/a": "1"})
		if err := gitrepo.NewRemote(local, "origin").Push(); err != nil {
			t.Fatal(err)
		}
		ahead := appendFiles(t, local, map[string]string{"proofs/b": "2"})
		if err := gitrepo.NewRemote(local, "origin").Fetch(); err != nil {
			t.Fatal(err)
		}
		if got, _, _ := local.Tip(); got != ahead {
			t.Fatalf("local tip %s, want %s", got, ahead)
		}
		if err := gitrepo.NewRemote(local, "origin").Push(); err != nil {
			t.Fatal(err)
		}
		if got, _, _ := remote.Tip(); got != ahead {
			t.Fatalf("remote tip %s, want %s", got, ahead)
		}
	})

	t.Run("a fetch refuses a local branch that has diverged from the remote one"+verifiesRemote, func(t *testing.T) {
		local, remote, _ := newPair(t)
		appendFiles(t, local, map[string]string{"proofs/a": "1"})
		appendFiles(t, remote, map[string]string{"proofs/other": "x"})
		err := gitrepo.NewRemote(local, "origin").Fetch()
		if !errors.Is(err, refusal.ErrRefused) || !strings.Contains(err.Error(), "diverged") {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a push of a diverged branch is rejected by the remote"+verifiesRemote, func(t *testing.T) {
		local, remote, _ := newPair(t)
		appendFiles(t, local, map[string]string{"proofs/a": "1"})
		remoteTip := appendFiles(t, remote, map[string]string{"proofs/other": "x"})
		if err := gitrepo.NewRemote(local, "origin").Push(); err == nil {
			t.Fatal("a non-fast-forward push was accepted")
		}
		if got, _, _ := remote.Tip(); got != remoteTip {
			t.Fatalf("remote tip moved to %s", got)
		}
	})
}
