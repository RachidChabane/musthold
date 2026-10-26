package proofcache_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/RachidChabane/musthold/completeness/internal/adapters/proofcache"
	"github.com/RachidChabane/musthold/completeness/internal/adapters/schemacheck"
	"github.com/RachidChabane/musthold/format"
)

const (
	goldenValid = "../../../../format/testdata/golden/completeness-valid.json"
	goldenKey   = "e17970e6b3125960f4facb5015a2e98bc33bcbc24b19048d0c570759b7add0a4"
	otherKey    = "0000000000000000000000000000000000000000000000000000000000000001"
	specPath    = "specs/parkfee.md"
)

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

func cachedPath(root, spec, key string) string {
	return filepath.Join(root, ".musthold", "cache", "completeness", format.SubjectKey(spec), key+".json")
}

func TestWrite(t *testing.T) {
	t.Run("a valid statement is written at the documented path, the spec path escaped to one segment [verifies SP-527~1]", func(t *testing.T) {
		cache, root := newCache(t)
		statement := validStatement(t)
		if err := cache.Write(specPath, goldenKey, statement); err != nil {
			t.Fatal(err)
		}
		path := cachedPath(root, specPath, goldenKey)
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, statement) {
			t.Fatalf("read %s: %v", path, err)
		}
		if filepath.Base(filepath.Dir(path)) != "specs%2Fparkfee.md" {
			t.Fatalf("the spec directory is %s", filepath.Base(filepath.Dir(path)))
		}
	})

	t.Run("an invalid statement is refused and nothing is written [verifies SP-527~1]", func(t *testing.T) {
		cache, root := newCache(t)
		err := cache.Write(specPath, goldenKey, []byte(`{"predicateType":"urn:unknown","predicate":{}}`))
		if !errors.Is(err, proofcache.ErrInvalidStatement) {
			t.Fatalf("error = %v", err)
		}
		if _, statErr := os.Stat(filepath.Join(root, ".musthold")); !os.IsNotExist(statErr) {
			t.Fatalf("the cache was created: %v", statErr)
		}
	})

	t.Run("a statement that is not JSON is refused [verifies SP-527~1]", func(t *testing.T) {
		cache, _ := newCache(t)
		if err := cache.Write(specPath, goldenKey, []byte("not json")); !errors.Is(err, proofcache.ErrInvalidStatement) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("writing again replaces the statement and leaves no temporary file [verifies SP-527~1]", func(t *testing.T) {
		cache, root := newCache(t)
		for range 2 {
			if err := cache.Write(specPath, goldenKey, validStatement(t)); err != nil {
				t.Fatal(err)
			}
		}
		entries, _ := os.ReadDir(filepath.Dir(cachedPath(root, specPath, goldenKey)))
		if len(entries) != 1 {
			t.Fatalf("directory holds %d entries", len(entries))
		}
	})

	t.Run("a spec path or key that could escape the cache is refused [verifies SP-527~1]", func(t *testing.T) {
		cache, _ := newCache(t)
		for _, bad := range []struct{ spec, key string }{
			{"../specs/parkfee.md", goldenKey},
			{"/specs/parkfee.md", goldenKey},
			{"", goldenKey},
			{specPath, "../" + goldenKey},
			{specPath, "short"},
		} {
			if err := cache.Write(bad.spec, bad.key, validStatement(t)); !errors.Is(err, proofcache.ErrInvalidKey) {
				t.Fatalf("Write(%q, %q) error = %v", bad.spec, bad.key, err)
			}
			if _, err := cache.Has(bad.spec, bad.key); !errors.Is(err, proofcache.ErrInvalidKey) {
				t.Fatalf("Has(%q, %q) error = %v", bad.spec, bad.key, err)
			}
		}
	})
}

func TestHas(t *testing.T) {
	t.Run("a statement is found once written and only for its spec and key [verifies SP-527~1]", func(t *testing.T) {
		cache, _ := newCache(t)
		if found, err := cache.Has(specPath, goldenKey); err != nil || found {
			t.Fatalf("Has before = %v, %v", found, err)
		}
		if err := cache.Write(specPath, goldenKey, validStatement(t)); err != nil {
			t.Fatal(err)
		}
		if found, err := cache.Has(specPath, goldenKey); err != nil || !found {
			t.Fatalf("Has after = %v, %v", found, err)
		}
		if found, _ := cache.Has(specPath, otherKey); found {
			t.Fatal("another key is found")
		}
		if found, _ := cache.Has("specs/other.md", goldenKey); found {
			t.Fatal("another spec is found")
		}
	})
}
