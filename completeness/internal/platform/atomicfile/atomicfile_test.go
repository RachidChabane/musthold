package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const fileMode = 0o600

func TestWrite(t *testing.T) {
	t.Run("a write creates the file with the mode and leaves no temporary file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "a.json")
		if err := Write(path, []byte("one"), fileMode); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != fileMode {
			t.Fatalf("stat = %v, %v", info, err)
		}
		if entries, _ := os.ReadDir(dir); len(entries) != 1 {
			t.Fatalf("directory holds %d entries", len(entries))
		}
	})

	t.Run("a write replaces the previous content", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "a.json")
		for _, content := range []string{"one", "two"} {
			if err := Write(path, []byte(content), fileMode); err != nil {
				t.Fatal(err)
			}
		}
		if got, _ := os.ReadFile(path); string(got) != "two" {
			t.Fatalf("content = %q", got)
		}
	})

	t.Run("a write into a missing directory fails", func(t *testing.T) {
		if err := Write(filepath.Join(t.TempDir(), "missing", "a"), nil, fileMode); err == nil {
			t.Fatal("expected an error")
		}
	})
}

func TestWriteNew(t *testing.T) {
	t.Run("the second creation is refused and keeps the first content", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "a.json")
		if err := WriteNew(path, []byte("one"), fileMode); err != nil {
			t.Fatal(err)
		}
		if err := WriteNew(path, []byte("two"), fileMode); !errors.Is(err, ErrExists) {
			t.Fatalf("error = %v", err)
		}
		if got, _ := os.ReadFile(path); string(got) != "one" {
			t.Fatalf("content = %q", got)
		}
		if entries, _ := os.ReadDir(dir); len(entries) != 1 {
			t.Fatalf("directory holds %d entries", len(entries))
		}
	})
}

func TestIsTemp(t *testing.T) {
	t.Run("temporary names are recognised", func(t *testing.T) {
		if !IsTemp(TempPrefix+"123") || IsTemp("a.json") || IsTemp("") {
			t.Fatal("IsTemp is wrong")
		}
	})
}
