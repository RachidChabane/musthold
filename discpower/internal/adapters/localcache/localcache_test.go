package localcache

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
)

const (
	mutantID = "00ff12ab"
	factKey  = "abcdef0123"
)

func sampleEdits() mutation.EditSet {
	return mutation.EditSet{Edits: []mutation.Edit{
		{File: "src/parcel.py", Qualname: "line_total", Target: mutation.TargetDefinition, NewSource: "def line_total(a, b):\n    return a + b\n"},
		{File: "src/parcel.py", Qualname: "RATE", Target: mutation.TargetModuleAssignment, NewSource: "RATE = 2\n"},
	}}
}

func TestVault(t *testing.T) {
	t.Run("a kept mutant is returned exactly [verifies SP-154~1]", func(t *testing.T) {
		vault := NewVault(t.TempDir())
		want := sampleEdits()
		if err := vault.Put(mutantID, want); err != nil {
			t.Fatal(err)
		}
		got, err := vault.Get(mutantID)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("Get = %+v, %v", got, err)
		}
	})

	t.Run("the body lives under the cache directory, in the mutants folder [verifies SP-154~1]", func(t *testing.T) {
		cacheDir := t.TempDir()
		if err := NewVault(cacheDir).Put(mutantID, sampleEdits()); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(cacheDir, "mutants", mutantID+".json")); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("an unknown mutant is a not-found error [verifies SP-154~1]", func(t *testing.T) {
		if _, err := NewVault(t.TempDir()).Get(mutantID); !errors.Is(err, ErrMutantNotFound) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("an identifier that is not lowercase hex is refused [verifies SP-154~1]", func(t *testing.T) {
		vault := NewVault(t.TempDir())
		for _, id := range []string{"", "../escape", "ABC", "a/b", "xyz"} {
			if err := vault.Put(id, sampleEdits()); !errors.Is(err, ErrInvalidKey) {
				t.Fatalf("Put(%q) error = %v", id, err)
			}
			if _, err := vault.Get(id); !errors.Is(err, ErrInvalidKey) {
				t.Fatalf("Get(%q) error = %v", id, err)
			}
		}
	})

	t.Run("a corrupted body is an error, not a mutant [verifies SP-154~1]", func(t *testing.T) {
		cacheDir := t.TempDir()
		vault := NewVault(cacheDir)
		if err := vault.Put(mutantID, sampleEdits()); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cacheDir, "mutants", mutantID+".json"), []byte("{"), fileMode); err != nil {
			t.Fatal(err)
		}
		if _, err := vault.Get(mutantID); err == nil || errors.Is(err, ErrMutantNotFound) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("any edit set comes back unchanged [verifies SP-154~1]", func(t *testing.T) {
		vault := NewVault(t.TempDir())
		targets := []mutation.TargetKind{mutation.TargetDefinition, mutation.TargetModuleAssignment, mutation.TargetModuleAddition}
		rapid.Check(t, func(t *rapid.T) {
			edit := rapid.Custom(func(t *rapid.T) mutation.Edit {
				return mutation.Edit{
					File:      rapid.StringMatching(`[a-z/]{1,12}\.py`).Draw(t, "file"),
					Qualname:  rapid.StringMatching(`[A-Za-z_][A-Za-z0-9_.]{0,10}`).Draw(t, "qualname"),
					Target:    rapid.SampledFrom(targets).Draw(t, "target"),
					NewSource: rapid.String().Draw(t, "source"),
				}
			})
			want := mutation.EditSet{Edits: rapid.SliceOfN(edit, 1, 4).Draw(t, "edits")}
			id := rapid.StringMatching(`[0-9a-f]{8,64}`).Draw(t, "id")
			if err := vault.Put(id, want); err != nil {
				t.Fatal(err)
			}
			got, err := vault.Get(id)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("Get = %+v, %v; want %+v", got, err, want)
			}
		})
	})
}

func TestMemo(t *testing.T) {
	t.Run("a memoized fact is returned for its key [verifies SP-155~1]", func(t *testing.T) {
		memo := NewMemo(t.TempDir())
		if err := memo.Put(factKey, []byte(`{"passed":true}`)); err != nil {
			t.Fatal(err)
		}
		got, found, err := memo.Get(factKey)
		if err != nil || !found || string(got) != `{"passed":true}` {
			t.Fatalf("Get = %q, %v, %v", got, found, err)
		}
	})

	t.Run("an absent entry is a miss [verifies SP-155~1]", func(t *testing.T) {
		got, found, err := NewMemo(t.TempDir()).Get(factKey)
		if err != nil || found || got != nil {
			t.Fatalf("Get = %q, %v, %v", got, found, err)
		}
	})

	t.Run("an unreadable entry is a miss [verifies SP-155~1]", func(t *testing.T) {
		cacheDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(cacheDir, "facts", factKey+".json"), dirMode); err != nil {
			t.Fatal(err)
		}
		_, found, err := NewMemo(cacheDir).Get(factKey)
		if err != nil || found {
			t.Fatalf("found = %v, err = %v", found, err)
		}
	})

	t.Run("a key that is not lowercase hex is refused, so no path can be traversed [verifies SP-155~1]", func(t *testing.T) {
		memo := NewMemo(t.TempDir())
		for _, key := range []string{"", "../x", "A1", "a.b", "a/b"} {
			if err := memo.Put(key, []byte("{}")); !errors.Is(err, ErrInvalidKey) {
				t.Fatalf("Put(%q) error = %v", key, err)
			}
			if _, _, err := memo.Get(key); !errors.Is(err, ErrInvalidKey) {
				t.Fatalf("Get(%q) error = %v", key, err)
			}
		}
	})

	t.Run("a write is atomic and leaves no temporary file [verifies SP-155~1]", func(t *testing.T) {
		cacheDir := t.TempDir()
		memo := NewMemo(cacheDir)
		for _, value := range []string{"one", "two"} {
			if err := memo.Put(factKey, []byte(value)); err != nil {
				t.Fatal(err)
			}
		}
		entries, _ := os.ReadDir(filepath.Join(cacheDir, "facts"))
		if len(entries) != 1 || strings.HasPrefix(entries[0].Name(), ".") {
			t.Fatalf("entries = %v", entries)
		}
		if got, _, _ := memo.Get(factKey); string(got) != "two" {
			t.Fatalf("value = %q", got)
		}
	})

	t.Run("any value comes back unchanged [verifies SP-155~1]", func(t *testing.T) {
		memo := NewMemo(t.TempDir())
		rapid.Check(t, func(t *rapid.T) {
			key := rapid.StringMatching(`[0-9a-f]{1,64}`).Draw(t, "key")
			value := rapid.SliceOf(rapid.Byte()).Draw(t, "value")
			if err := memo.Put(key, value); err != nil {
				t.Fatal(err)
			}
			got, found, err := memo.Get(key)
			if err != nil || !found || string(got) != string(value) {
				t.Fatalf("Get = %q, %v, %v", got, found, err)
			}
		})
	})
}
