package ledger_test

import (
	"errors"
	"maps"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/ledger/internal/domain/ledger"
	"github.com/RachidChabane/spec-proof/ledger/internal/domain/refusal"
	"github.com/RachidChabane/spec-proof/ledger/internal/testkit"
)

const verifiesAppend = " [verifies SP-154~1]"

func addition(path, content string) ledger.Addition {
	return ledger.Addition{Path: path, Content: []byte(content)}
}

func apply(t testing.TB, store *testkit.MemoryStore, root *ledger.ObjectID, additions ...ledger.Addition) (ledger.ObjectID, ledger.Outcome) {
	t.Helper()
	id, outcome, err := ledger.ApplyAdditions(store, root, additions)
	if err != nil {
		t.Fatal(err)
	}
	return id, outcome
}

func TestApplyAdditions(t *testing.T) {
	t.Run("additions build the nested trees of their paths"+verifiesAppend, func(t *testing.T) {
		store := testkit.NewMemoryStore()
		root, outcome := apply(t, store, nil, addition("proofs/m/s/k.dsse.json", "one"), addition("seeds/h.json", "two"))
		files, err := store.Flatten(root)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]string{"proofs/m/s/k.dsse.json": "one", "seeds/h.json": "two"}
		if !maps.Equal(files, want) || len(outcome.Added) != 2 || len(outcome.Unchanged) != 0 {
			t.Fatalf("files %v outcome %+v", files, outcome)
		}
	})

	t.Run("the same bytes at an existing path are a no-op that keeps the root"+verifiesAppend, func(t *testing.T) {
		store := testkit.NewMemoryStore()
		first, _ := apply(t, store, nil, addition("proofs/a", "one"))
		second, outcome := apply(t, store, &first, addition("proofs/a", "one"))
		if second != first || len(outcome.Added) != 0 || len(outcome.Unchanged) != 1 {
			t.Fatalf("root %s -> %s, outcome %+v", first, second, outcome)
		}
	})

	t.Run("other bytes at an existing path are refused and never rewritten"+verifiesAppend, func(t *testing.T) {
		store := testkit.NewMemoryStore()
		first, _ := apply(t, store, nil, addition("proofs/a", "one"))
		_, _, err := ledger.ApplyAdditions(store, &first, []ledger.Addition{addition("proofs/a", "other"), addition("proofs/b", "new")})
		if !errors.Is(err, refusal.ErrRefused) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("an existing path keeps its bytes under the keep-existing policy"+verifiesAppend, func(t *testing.T) {
		store := testkit.NewMemoryStore()
		first, _ := apply(t, store, nil, addition("seeds/h.json", "first"))
		keep := ledger.Addition{Path: "seeds/h.json", Content: []byte("second"), Policy: ledger.PolicyKeepExisting}
		second, outcome := apply(t, store, &first, keep)
		files, _ := store.Flatten(second)
		if second != first || files["seeds/h.json"] != "first" || len(outcome.Unchanged) != 1 {
			t.Fatalf("files %v outcome %+v", files, outcome)
		}
	})

	t.Run("a subtree no addition touches keeps its object"+verifiesAppend, func(t *testing.T) {
		store := testkit.NewMemoryStore()
		first, _ := apply(t, store, nil, addition("proofs/m/s/k", "one"), addition("seeds/h.json", "seed"))
		before := untouchedSubtree(t, store, first, "proofs")
		written := len(store.WrittenIDs)
		second, _ := apply(t, store, &first, addition("seeds/g.json", "other seed"))
		if after := untouchedSubtree(t, store, second, "proofs"); after != before {
			t.Fatalf("proofs subtree %s became %s", before, after)
		}
		if rewritten := len(store.WrittenIDs) - written; rewritten != 3 {
			t.Fatalf("%d objects written for one file in one directory, want 3 (blob, seeds tree, root)", rewritten)
		}
	})

	refused := []struct {
		name      string
		existing  []ledger.Addition
		additions []ledger.Addition
	}{
		{"a file where a directory is", []ledger.Addition{addition("proofs/m/a", "x")}, []ledger.Addition{addition("proofs/m", "y")}},
		{"a directory where a file is", []ledger.Addition{addition("proofs/m", "x")}, []ledger.Addition{addition("proofs/m/a", "y")}},
		{"a path added twice", nil, []ledger.Addition{addition("a/b", "x"), addition("a/b", "x")}},
		{"a name that is both a file and a directory", nil, []ledger.Addition{addition("a", "x"), addition("a/b", "y")}},
		{"an empty segment", nil, []ledger.Addition{addition("a//b", "x")}},
		{"a trailing separator", nil, []ledger.Addition{addition("a/", "x")}},
	}
	for _, tt := range refused {
		t.Run(tt.name+" is refused"+verifiesAppend, func(t *testing.T) {
			store := testkit.NewMemoryStore()
			var root *ledger.ObjectID
			if tt.existing != nil {
				id, _ := apply(t, store, nil, tt.existing...)
				root = &id
			}
			if _, _, err := ledger.ApplyAdditions(store, root, tt.additions); !errors.Is(err, refusal.ErrRefused) {
				t.Fatalf("error %v", err)
			}
		})
	}

	t.Run("no addition leaves an empty ledger empty"+verifiesAppend, func(t *testing.T) {
		store := testkit.NewMemoryStore()
		id, outcome, err := ledger.ApplyAdditions(store, nil, nil)
		if err != nil || id != "" || len(outcome.Added) != 0 {
			t.Fatalf("id %q outcome %+v err %v", id, outcome, err)
		}
	})
}

func untouchedSubtree(t testing.TB, store *testkit.MemoryStore, root ledger.ObjectID, name string) ledger.ObjectID {
	t.Helper()
	entries, err := store.ReadTree(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name == name {
			return entry.ID
		}
	}
	t.Fatalf("no entry %s", name)
	return ""
}

func TestApplyAdditionsProperties(t *testing.T) {
	segment := rapid.SampledFrom([]string{"a", "b", "c"})
	pathGen := rapid.Custom(func(t *rapid.T) string {
		depth := rapid.IntRange(1, 3).Draw(t, "depth")
		path := ""
		for index := range depth {
			if index > 0 {
				path += "/"
			}
			path += segment.Draw(t, "segment")
		}
		return path
	})

	t.Run("the tree holds every addition and applying again changes nothing"+verifiesAppend, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			paths := rapid.SliceOfNDistinct(pathGen, 1, 6, func(path string) string { return path }).Draw(t, "paths")
			want := map[string]string{}
			var additions []ledger.Addition
			for _, path := range paths {
				want[path] = "content of " + path
				additions = append(additions, addition(path, want[path]))
			}
			store := testkit.NewMemoryStore()
			root, _, err := ledger.ApplyAdditions(store, nil, additions)
			if err != nil {
				return // a path that is a prefix of another is refused; nothing to check
			}
			files, err := store.Flatten(root)
			if err != nil || !maps.Equal(files, want) {
				t.Fatalf("files %v, want %v (%v)", files, want, err)
			}
			again, outcome, err := ledger.ApplyAdditions(store, &root, additions)
			if err != nil || again != root || len(outcome.Added) != 0 {
				t.Fatalf("second apply: root %s -> %s, outcome %+v, %v", root, again, outcome, err)
			}
		})
	})
}
