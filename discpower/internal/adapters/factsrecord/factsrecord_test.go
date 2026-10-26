package factsrecord_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/factsrecord"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
)

const (
	verifiesSP1977 = " [verifies SP-1977~1]"

	keyLength  = 64
	latestFile = "latest.json"
)

var criterion = spec.Ref{ID: "SP-7", Revision: 2}

func cacheKey(digit string) string { return strings.Repeat(digit, keyLength) }

func TestWrite(t *testing.T) {
	t.Run("the record is kept under its cache key and as the latest"+verifiesSP1977, func(t *testing.T) {
		dir := t.TempDir()
		record := []byte(`{"policy":{}}`)
		if err := factsrecord.New(dir).Write(criterion, cacheKey("a"), record); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{cacheKey("a") + ".json", latestFile} {
			got, err := os.ReadFile(filepath.Join(dir, criterion.String(), name))
			if err != nil || !bytes.Equal(got, record) {
				t.Errorf("%s holds %q, %v", name, got, err)
			}
		}
	})
	t.Run("a second record keeps the first under its key and becomes the latest"+verifiesSP1977, func(t *testing.T) {
		dir := t.TempDir()
		store := factsrecord.New(dir)
		if err := store.Write(criterion, cacheKey("a"), []byte("first")); err != nil {
			t.Fatal(err)
		}
		if err := store.Write(criterion, cacheKey("b"), []byte("second")); err != nil {
			t.Fatal(err)
		}
		first, err := os.ReadFile(filepath.Join(dir, criterion.String(), cacheKey("a")+".json"))
		if err != nil || string(first) != "first" {
			t.Errorf("first = %q, %v", first, err)
		}
		latest, err := store.Latest(criterion)
		if err != nil || string(latest) != "second" {
			t.Errorf("latest = %q, %v", latest, err)
		}
	})
	t.Run("writing the same key again replaces the record"+verifiesSP1977, func(t *testing.T) {
		store := factsrecord.New(t.TempDir())
		for _, record := range []string{"old", "new"} {
			if err := store.Write(criterion, cacheKey("c"), []byte(record)); err != nil {
				t.Fatal(err)
			}
		}
		if latest, err := store.Latest(criterion); err != nil || string(latest) != "new" {
			t.Errorf("latest = %q, %v", latest, err)
		}
	})
	t.Run("the records of two criteria are kept apart"+verifiesSP1977, func(t *testing.T) {
		store := factsrecord.New(t.TempDir())
		other := spec.Ref{ID: "SP-8", Revision: 1}
		if err := store.Write(criterion, cacheKey("a"), []byte("mine")); err != nil {
			t.Fatal(err)
		}
		if err := store.Write(other, cacheKey("a"), []byte("theirs")); err != nil {
			t.Fatal(err)
		}
		if latest, _ := store.Latest(criterion); string(latest) != "mine" {
			t.Errorf("latest = %q", latest)
		}
	})
	for name, key := range map[string]string{
		"an empty key":               "",
		"a short key":                strings.Repeat("a", keyLength-1),
		"a long key":                 strings.Repeat("a", keyLength+1),
		"an upper-case key":          strings.Repeat("A", keyLength),
		"a key with a non-hex digit": strings.Repeat("g", keyLength),
		"a path traversal":           "../" + strings.Repeat("a", keyLength-3),
		"a key ending with a break":  strings.Repeat("a", keyLength) + "\n",
	} {
		t.Run(name+" is refused"+verifiesSP1977, func(t *testing.T) {
			dir := t.TempDir()
			err := factsrecord.New(dir).Write(criterion, key, []byte("x"))
			if !errors.Is(err, factsrecord.ErrInvalidKey) {
				t.Errorf("err = %v", err)
			}
			if entries, _ := os.ReadDir(dir); len(entries) != 0 {
				t.Errorf("a refused write left %d entries", len(entries))
			}
		})
	}
	t.Run("any record is read back byte for byte"+verifiesSP1977, func(t *testing.T) {
		base := t.TempDir()
		rapid.Check(t, func(t *rapid.T) {
			dir, err := os.MkdirTemp(base, "")
			if err != nil {
				t.Fatal(err)
			}
			store := factsrecord.New(dir)
			key := rapid.StringMatching(`[0-9a-f]{64}`).Draw(t, "key")
			record := rapid.SliceOf(rapid.Byte()).Draw(t, "record")
			if err := store.Write(criterion, key, record); err != nil {
				t.Fatal(err)
			}
			got, err := store.Latest(criterion)
			if err != nil || !bytes.Equal(got, record) {
				t.Fatalf("read back %q, %v", got, err)
			}
		})
	})
}

func TestLatest(t *testing.T) {
	t.Run("a criterion with no record has none"+verifiesSP1977, func(t *testing.T) {
		_, err := factsrecord.New(t.TempDir()).Latest(criterion)
		if !errors.Is(err, factsrecord.ErrNoRecord) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("a criterion written under another revision has none"+verifiesSP1977, func(t *testing.T) {
		store := factsrecord.New(t.TempDir())
		if err := store.Write(spec.Ref{ID: criterion.ID, Revision: criterion.Revision + 1}, cacheKey("a"), []byte("x")); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Latest(criterion); !errors.Is(err, factsrecord.ErrNoRecord) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("the latest is the last record written"+verifiesSP1977, func(t *testing.T) {
		store := factsrecord.New(t.TempDir())
		for _, digit := range []string{"1", "2", "3"} {
			if err := store.Write(criterion, cacheKey(digit), []byte("record "+digit)); err != nil {
				t.Fatal(err)
			}
		}
		if latest, err := store.Latest(criterion); err != nil || string(latest) != "record 3" {
			t.Errorf("latest = %q, %v", latest, err)
		}
	})
}
