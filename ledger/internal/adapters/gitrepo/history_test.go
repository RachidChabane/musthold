package gitrepo_test

import (
	"slices"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/ledger/internal/domain/ledger"
	"github.com/RachidChabane/spec-proof/ledger/internal/testkit"
)

// rootOf writes a root tree that holds one directory of blobs, the way a ledger
// commit does, and returns it.
func rootOf(t *testing.T, store ledger.TreeStore, directory string, blobs map[string]string) ledger.ObjectID {
	t.Helper()
	var entries []ledger.TreeEntry
	for _, name := range slices.Sorted(keys(blobs)) {
		id, err := store.WriteBlob([]byte(blobs[name]))
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, ledger.TreeEntry{Mode: ledger.ModeBlob, Kind: ledger.EntryBlob, ID: id, Name: name})
	}
	subtree, err := store.WriteTree(entries)
	if err != nil {
		t.Fatal(err)
	}
	root, err := store.WriteTree([]ledger.TreeEntry{{Mode: ledger.ModeTree, Kind: ledger.EntryTree, ID: subtree, Name: directory}})
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestHistoryAndRewrites(t *testing.T) {
	t.Run("the history lists every commit with its parents"+verifiesAuditGit, func(t *testing.T) {
		repository, _ := openRepository(t)
		first := appendFiles(t, repository, map[string]string{"proofs/a": "1"})
		second := appendFiles(t, repository, map[string]string{"proofs/b": "2"})
		history, err := repository.History(second)
		if err != nil {
			t.Fatal(err)
		}
		if len(history) != 2 || history[0].ID != second || len(history[0].Parents) != 1 || history[0].Parents[0] != first || len(history[1].Parents) != 0 {
			t.Fatalf("history %+v", history)
		}
	})

	t.Run("an append-only history has no rewrite"+verifiesAuditGit, func(t *testing.T) {
		repository, _ := openRepository(t)
		appendFiles(t, repository, map[string]string{"proofs/a": "1"})
		tip := appendFiles(t, repository, map[string]string{"proofs/b": "2"})
		rewrites, err := repository.Rewrites(tip)
		if err != nil || len(rewrites) != 0 {
			t.Fatalf("rewrites %v: %v", rewrites, err)
		}
	})

	t.Run("a commit that modifies or deletes a path is reported"+verifiesAuditGit, func(t *testing.T) {
		repository, _ := openRepository(t)
		first := appendFiles(t, repository, map[string]string{"proofs/a": "1", "proofs/b": "2"})
		rewritten := rootOf(t, repository, "proofs", map[string]string{"a": "changed"})
		commit, err := repository.Commit(rewritten, &first, "rewrite", commitTime)
		if err != nil {
			t.Fatal(err)
		}
		rewrites, err := repository.Rewrites(commit)
		if err != nil {
			t.Fatal(err)
		}
		slices.Sort(rewrites)
		if !slices.Equal(rewrites, []string{"proofs/a", "proofs/b"}) {
			t.Fatalf("rewrites %v, want a modified and b deleted", rewrites)
		}
	})

	t.Run("a path with unusual characters is reported verbatim"+verifiesAuditGit, func(t *testing.T) {
		repository, _ := openRepository(t)
		first := appendFiles(t, repository, map[string]string{"proofs/ünï cödé": "1"})
		empty, err := repository.WriteTree(nil)
		if err != nil {
			t.Fatal(err)
		}
		deleting, err := repository.Commit(empty, &first, "delete all", commitTime)
		if err != nil {
			t.Fatal(err)
		}
		rewrites, err := repository.Rewrites(deleting)
		if err != nil || !slices.Equal(rewrites, []string{"proofs/ünï cödé"}) {
			t.Fatalf("rewrites %q: %v", rewrites, err)
		}
	})

	t.Run("a merge commit shows with its two parents"+verifiesAuditGit, func(t *testing.T) {
		repository, directory := openRepository(t)
		first := appendFiles(t, repository, map[string]string{"proofs/a": "1"})
		tree, err := repository.TreeOf(first)
		if err != nil {
			t.Fatal(err)
		}
		left, err := repository.Commit(tree, &first, "left", commitTime.Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}
		right, err := repository.Commit(tree, &first, "right", commitTime.Add(2*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		merge := testkit.Git(t, directory, "commit-tree", string(tree), "-p", string(left), "-p", string(right), "-m", "merge")
		history, err := repository.History(ledger.ObjectID(merge))
		if err != nil {
			t.Fatal(err)
		}
		if len(history) != 4 || len(history[0].Parents) != 2 {
			t.Fatalf("history %+v", history)
		}
	})
}
