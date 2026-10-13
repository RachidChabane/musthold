package scansources_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/scansources"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/scan"
	"github.com/RachidChabane/spec-proof/format"
)

const (
	verifiesSave = " [verifies SP-2209~1]"
	verifiesName = " [verifies SP-2210~1]"
	verifiesSafe = " [verifies SP-2211~1]"
)

var commit = strings.Repeat("ab12", 10)

func TestSaveAndLoad(t *testing.T) {
	root := t.TempDir()
	store := scansources.Store{Root: root}
	sources := []scan.Source{
		{Name: "README", Content: []byte("# Title\r\nline  \n")},
		{Name: "description", Content: []byte("naïve – 日本語\n")},
		{Name: "issue/7", Content: []byte{0x00, 0xff, 0xfe, '\n'}},
	}

	t.Run("sources round trip byte for byte under the scan's folder with the format digest"+verifiesSave, func(t *testing.T) {
		saved, err := store.Save(commit, sources)
		if err != nil || len(saved) != len(sources) {
			t.Fatalf("saved = %v, %v", saved, err)
		}
		for index, source := range sources {
			if saved[index].Name != source.Name || saved[index].Digest != format.SHA256Hex(source.Content) {
				t.Errorf("saved[%d] = %+v", index, saved[index])
			}
			back, err := store.Load(commit, source.Name)
			if err != nil || !bytes.Equal(back, source.Content) {
				t.Errorf("load %q = %q, %v", source.Name, back, err)
			}
		}
		if _, err := os.Stat(filepath.Join(root, ".spec-proof", "scan", commit, "sources", "README")); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("saving again replaces a source and leaves no temporary file"+verifiesSave, func(t *testing.T) {
		if _, err := store.Save(commit, []scan.Source{{Name: "README", Content: []byte("new")}}); err != nil {
			t.Fatal(err)
		}
		if back, _ := store.Load(commit, "README"); string(back) != "new" {
			t.Fatalf("README = %q", back)
		}
		entries, _ := os.ReadDir(filepath.Join(root, ".spec-proof", "scan", commit, "sources"))
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".source-") {
				t.Fatalf("leftover %s", entry.Name())
			}
		}
	})
}

func TestFileName(t *testing.T) {
	t.Run("a name maps to the same file every time and distinct names to distinct files"+verifiesName, func(t *testing.T) {
		names := []string{"README", "README.md", "issue/7", "issue%2F7", "issue 7", "issue-7", "a\\b", ".hidden", "%2Ehidden", "日本語", "UPPER", "upper"}
		seen := map[string]string{}
		for _, name := range names {
			first, err := scansources.FileName(name)
			second, _ := scansources.FileName(name)
			if err != nil || first != second {
				t.Fatalf("%q: %q, %q, %v", name, first, second, err)
			}
			if strings.ContainsAny(first, `/\`) || strings.HasPrefix(first, ".") {
				t.Errorf("%q maps to %q", name, first)
			}
			if other, dup := seen[first]; dup {
				t.Errorf("%q and %q share %q", other, name, first)
			}
			seen[first] = name
		}
	})

	t.Run("a name too long for a file is refused"+verifiesName, func(t *testing.T) {
		if _, err := scansources.FileName(strings.Repeat("x", 300)); !errors.Is(err, scansources.ErrNameTooLong) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("two sources of one save that share a file are refused before any write"+verifiesName, func(t *testing.T) {
		root := t.TempDir()
		_, err := scansources.Store{Root: root}.Save(commit, []scan.Source{{Name: "a"}, {Name: "a"}})
		if !errors.Is(err, scansources.ErrDuplicateName) {
			t.Fatalf("error = %v", err)
		}
		if _, statErr := os.Stat(filepath.Join(root, ".spec-proof")); !os.IsNotExist(statErr) {
			t.Fatal("something was written")
		}
	})
}

func TestRefusals(t *testing.T) {
	t.Run("traversal, empty and NUL names are refused and nothing is written"+verifiesSafe, func(t *testing.T) {
		for _, name := range []string{"", "..", ".", "../escape", "a/../../escape", `..\escape`, "a/./b", "nul\x00byte"} {
			root := t.TempDir()
			store := scansources.Store{Root: root}
			if _, err := store.Save(commit, []scan.Source{{Name: name, Content: []byte("x")}}); !errors.Is(err, scansources.ErrUnsafeName) {
				t.Errorf("save %q: %v", name, err)
			}
			if _, err := store.Load(commit, name); !errors.Is(err, scansources.ErrUnsafeName) {
				t.Errorf("load %q: %v", name, err)
			}
			if entries, _ := os.ReadDir(root); len(entries) != 0 {
				t.Errorf("%q wrote %v", name, entries)
			}
		}
	})

	t.Run("a commit that is not a full hexadecimal id and a store without a root are refused"+verifiesSafe, func(t *testing.T) {
		store := scansources.Store{Root: t.TempDir()}
		for _, bad := range []string{"", "../..", "abc", strings.Repeat("g", 40), strings.Repeat("AB", 20)} {
			if _, err := store.Save(bad, nil); !errors.Is(err, scansources.ErrBadCommit) {
				t.Errorf("commit %q: %v", bad, err)
			}
		}
		if _, err := (scansources.Store{}).Save(commit, nil); !errors.Is(err, scansources.ErrNoRoot) {
			t.Errorf("no root: %v", err)
		}
	})
}
