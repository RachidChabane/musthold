package proofcache_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/proofcache"
	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/schemacheck"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/criterion"
	"github.com/RachidChabane/spec-proof/format"
)

const (
	goldenValid = "../../../../format/testdata/golden/alignment-valid.json"
	goldenKey   = "5023b3341bb79ff8b797c79aee976493b15a15ef21f97210bc1aae0c05fb37fd"
	otherKey    = "0000000000000000000000000000000000000000000000000000000000000001"
)

var ref = criterion.Ref{ID: "PAR-1", Revision: 1}

func newCache(t *testing.T) (*proofcache.Cache, string) {
	t.Helper()
	checker, err := schemacheck.New()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	return proofcache.New(root, checker), root
}

func validStatement(t *testing.T) []byte {
	t.Helper()
	content, err := os.ReadFile(goldenValid)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func TestWrite(t *testing.T) {
	t.Run("a valid statement is written at the documented path [verifies SP-345~1]", func(t *testing.T) {
		cache, root := newCache(t)
		statement := validStatement(t)
		if err := cache.Write(ref, goldenKey, statement); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, ".spec-proof", "cache", "alignment", format.SubjectName("PAR-1", 1), goldenKey+".json")
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, statement) {
			t.Fatalf("read %s: %v", path, err)
		}
	})

	t.Run("an invalid statement is refused and nothing is written [verifies SP-345~1]", func(t *testing.T) {
		cache, root := newCache(t)
		err := cache.Write(ref, goldenKey, []byte(`{"predicateType":"urn:unknown","predicate":{}}`))
		if !errors.Is(err, proofcache.ErrInvalidStatement) {
			t.Fatalf("error = %v", err)
		}
		if _, statErr := os.Stat(filepath.Join(root, ".spec-proof")); !os.IsNotExist(statErr) {
			t.Fatalf("the cache was created: %v", statErr)
		}
	})

	t.Run("a statement that is not JSON is refused [verifies SP-345~1]", func(t *testing.T) {
		cache, _ := newCache(t)
		if err := cache.Write(ref, goldenKey, []byte("not json")); !errors.Is(err, proofcache.ErrInvalidStatement) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("writing again replaces the statement and leaves no temporary file [verifies SP-345~1]", func(t *testing.T) {
		cache, root := newCache(t)
		for range 2 {
			if err := cache.Write(ref, goldenKey, validStatement(t)); err != nil {
				t.Fatal(err)
			}
		}
		entries, _ := os.ReadDir(filepath.Join(root, ".spec-proof", "cache", "alignment", format.SubjectName("PAR-1", 1)))
		if len(entries) != 1 {
			t.Fatalf("directory holds %d entries", len(entries))
		}
	})

	t.Run("a criterion or key that could escape the cache is refused [verifies SP-345~1]", func(t *testing.T) {
		cache, _ := newCache(t)
		for _, bad := range []struct {
			ref criterion.Ref
			key string
		}{
			{criterion.Ref{ID: "../PAR", Revision: 1}, goldenKey},
			{criterion.Ref{ID: "", Revision: 1}, goldenKey},
			{ref, "../" + goldenKey},
			{ref, "short"},
		} {
			if err := cache.Write(bad.ref, bad.key, validStatement(t)); !errors.Is(err, proofcache.ErrInvalidKey) {
				t.Fatalf("Write(%v, %q) error = %v", bad.ref, bad.key, err)
			}
			if _, err := cache.Has(bad.ref, bad.key); !errors.Is(err, proofcache.ErrInvalidKey) {
				t.Fatalf("Has(%v, %q) error = %v", bad.ref, bad.key, err)
			}
		}
	})
}

func TestHas(t *testing.T) {
	t.Run("a statement is found once written and only for its key [verifies SP-345~1]", func(t *testing.T) {
		cache, _ := newCache(t)
		if found, err := cache.Has(ref, goldenKey); err != nil || found {
			t.Fatalf("Has before = %v, %v", found, err)
		}
		if err := cache.Write(ref, goldenKey, validStatement(t)); err != nil {
			t.Fatal(err)
		}
		if found, err := cache.Has(ref, goldenKey); err != nil || !found {
			t.Fatalf("Has after = %v, %v", found, err)
		}
		if found, _ := cache.Has(ref, otherKey); found {
			t.Fatal("another key is found")
		}
		if found, _ := cache.Has(criterion.Ref{ID: "PAR-1", Revision: 2}, goldenKey); found {
			t.Fatal("another revision is found")
		}
	})
}
