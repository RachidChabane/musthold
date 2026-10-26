package verifiedout_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/RachidChabane/musthold/verify/internal/adapters/verifiedout"
	"github.com/RachidChabane/musthold/verify/internal/domain/proofset"
)

const verifiesOutput = " [verifies SP-217~1]"

func entries(t *testing.T, directory string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative, _ := filepath.Rel(directory, path)
		found = append(found, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func TestWrite(t *testing.T) {
	t.Run("the statements and the manifest are written"+verifiesOutput, func(t *testing.T) {
		out := filepath.Join(t.TempDir(), ".musthold", "verified")
		files := []proofset.File{{Path: "m/S~1/k.json", Content: []byte("one")}, {Path: "m/T~1/k.json", Content: []byte("two")}}
		if err := (verifiedout.Directory{Path: out}).Write(files, []byte("manifest")); err != nil {
			t.Fatal(err)
		}
		got := entries(t, out)
		if len(got) != 3 || got[0] != "m/S~1/k.json" || got[2] != "manifest.json" {
			t.Fatalf("entries %v", got)
		}
		content, err := os.ReadFile(filepath.Join(out, "manifest.json"))
		if err != nil || string(content) != "manifest" {
			t.Fatalf("manifest %q: %v", content, err)
		}
		if info, err := os.Stat(out); err != nil || info.Mode().Perm() != 0o755 {
			t.Fatalf("directory mode %v: %v", info, err)
		}
	})

	t.Run("a second run replaces the directory and leaves no file of the first"+verifiesOutput, func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "verified")
		sink := verifiedout.Directory{Path: out}
		if err := sink.Write([]proofset.File{{Path: "old/S~1/k.json", Content: []byte("old")}}, []byte("first")); err != nil {
			t.Fatal(err)
		}
		if err := sink.Write([]proofset.File{{Path: "new/S~1/k.json", Content: []byte("new")}}, []byte("second")); err != nil {
			t.Fatal(err)
		}
		got := entries(t, out)
		if len(got) != 2 || got[0] != "manifest.json" || got[1] != "new/S~1/k.json" {
			t.Fatalf("entries %v", got)
		}
	})

	t.Run("no staging directory is left behind"+verifiesOutput, func(t *testing.T) {
		parent := t.TempDir()
		if err := (verifiedout.Directory{Path: filepath.Join(parent, "verified")}).Write(nil, []byte("m")); err != nil {
			t.Fatal(err)
		}
		listed, err := os.ReadDir(parent)
		if err != nil || len(listed) != 1 {
			t.Fatalf("entries %v: %v", listed, err)
		}
	})

	t.Run("a failure leaves the previous directory in place"+verifiesOutput, func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "verified")
		sink := verifiedout.Directory{Path: out}
		if err := sink.Write([]proofset.File{{Path: "m/S~1/k.json", Content: []byte("kept")}}, []byte("kept manifest")); err != nil {
			t.Fatal(err)
		}
		// A file path that needs a directory where a file is makes the staging fail.
		failing := []proofset.File{{Path: "a", Content: []byte("x")}, {Path: "a/b", Content: []byte("y")}}
		if err := sink.Write(failing, []byte("new")); err == nil {
			t.Fatal("the write succeeded")
		}
		content, err := os.ReadFile(filepath.Join(out, "manifest.json"))
		if err != nil || string(content) != "kept manifest" {
			t.Fatalf("manifest %q: %v", content, err)
		}
	})
}
