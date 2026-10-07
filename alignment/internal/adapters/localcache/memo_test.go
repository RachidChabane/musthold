package localcache

import (
	"errors"
	"path/filepath"
	"testing"

	"pgregory.net/rapid"
)

func TestMemo(t *testing.T) {
	t.Run("a value put under a key comes back, and an absent key is a miss [verifies SP-346~1]", func(t *testing.T) {
		memo := NewMemo(t.TempDir())
		if _, found, err := memo.Get("abcdef01"); found || err != nil {
			t.Fatalf("an empty memo answered: %v %v", found, err)
		}
		if err := memo.Put("abcdef01", []byte(`{"a":1}`)); err != nil {
			t.Fatal(err)
		}
		value, found, err := memo.Get("abcdef01")
		if err != nil || !found || string(value) != `{"a":1}` {
			t.Fatalf("value %q found %v err %v", value, found, err)
		}
	})

	t.Run("a key that could address another file is refused [verifies SP-346~1]", func(t *testing.T) {
		memo := NewMemo(t.TempDir())
		for _, key := range []string{"", "../escape", "ABCDEF", "a/b", "xyz"} {
			if _, _, err := memo.Get(key); !errors.Is(err, ErrInvalidKey) {
				t.Errorf("get %q: %v", key, err)
			}
			if err := memo.Put(key, []byte("x")); !errors.Is(err, ErrInvalidKey) {
				t.Errorf("put %q: %v", key, err)
			}
		}
	})

	t.Run("an entry is a file of the cache directory and a later put replaces it [verifies SP-346~1]", func(t *testing.T) {
		dir := t.TempDir()
		memo := NewMemo(dir)
		rapid.Check(t, func(t *rapid.T) {
			key := rapid.StringMatching(`[0-9a-f]{8,64}`).Draw(t, "key")
			first, second := rapid.SliceOfN(rapid.Byte(), 0, 64).Draw(t, "first"), rapid.SliceOfN(rapid.Byte(), 0, 64).Draw(t, "second")
			if err := memo.Put(key, first); err != nil {
				t.Fatal(err)
			}
			if err := memo.Put(key, second); err != nil {
				t.Fatal(err)
			}
			value, found, err := memo.Get(key)
			if err != nil || !found || string(value) != string(second) {
				t.Fatalf("value %q found %v err %v", value, found, err)
			}
		})
		if matches, _ := filepath.Glob(filepath.Join(dir, memoDirectory, "*"+entryExt)); len(matches) == 0 {
			t.Fatal("no entry file")
		}
	})
}
