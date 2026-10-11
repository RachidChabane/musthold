package plantedrefactors_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/plantedrefactors"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/prove"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
)

const (
	verifiesSP1972 = " [verifies SP-1972~1]"

	refactorsDirectory = "tests/refactors"
	fileMode           = 0o600
	directoryMode      = 0o700

	editSetJSON = `{"edits":[{"file":"src/a.py","qualname":"f","new_source":"def f():\n    return 1\n"}]}`
	otherEdits  = `{"edits":[{"file":"src/b.py","qualname":"g","new_source":"def g():\n    return 2\n"},{"file":"src/b.py","qualname":"h","new_source":"def h():\n    pass\n"}]}`
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), directoryMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), fileMode); err != nil {
		t.Fatal(err)
	}
}

func names(refactors []prove.Refactor) []string {
	out := make([]string, len(refactors))
	for index, refactor := range refactors {
		out[index] = refactor.Name
	}
	return out
}

func TestLoad(t *testing.T) {
	t.Run("an empty directory name names no refactor"+verifiesSP1972, func(t *testing.T) {
		refactors, err := plantedrefactors.Load(t.TempDir(), "")
		if err != nil || refactors != nil {
			t.Errorf("got %v, %v", refactors, err)
		}
	})
	t.Run("a relative directory is read from the project root"+verifiesSP1972, func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "tests", "refactors", "rename.json"), editSetJSON)
		refactors, err := plantedrefactors.Load(root, refactorsDirectory)
		if err != nil || len(refactors) != 1 {
			t.Fatalf("got %v, %v", refactors, err)
		}
		edit := refactors[0].Edits.Edits[0]
		if edit.File != "src/a.py" || edit.Qualname != "f" || edit.NewSource != "def f():\n    return 1\n" {
			t.Errorf("edit = %+v", edit)
		}
	})
	t.Run("an absolute directory is read as is, whatever the root"+verifiesSP1972, func(t *testing.T) {
		directory := t.TempDir()
		writeFile(t, filepath.Join(directory, "inline.json"), editSetJSON)
		refactors, err := plantedrefactors.Load(t.TempDir(), directory)
		if err != nil || !slices.Equal(names(refactors), []string{"inline"}) {
			t.Errorf("got %v, %v", names(refactors), err)
		}
	})
	t.Run("every edit set is a refactor named after its file, sorted by name"+verifiesSP1972, func(t *testing.T) {
		root := t.TempDir()
		directory := filepath.Join(root, "refactors")
		writeFile(t, filepath.Join(directory, "b-extract.json"), otherEdits)
		writeFile(t, filepath.Join(directory, "a-rename.json"), editSetJSON)
		writeFile(t, filepath.Join(directory, "c-inline.json"), editSetJSON)
		refactors, err := plantedrefactors.Load(root, "refactors")
		if err != nil {
			t.Fatal(err)
		}
		if got := names(refactors); !slices.Equal(got, []string{"a-rename", "b-extract", "c-inline"}) {
			t.Errorf("names = %v", got)
		}
		if len(refactors[1].Edits.Edits) != 2 {
			t.Errorf("the edits of b-extract = %+v", refactors[1].Edits)
		}
	})
	t.Run("other files and subdirectories are ignored"+verifiesSP1972, func(t *testing.T) {
		root := t.TempDir()
		directory := filepath.Join(root, "refactors")
		writeFile(t, filepath.Join(directory, "keep.json"), editSetJSON)
		writeFile(t, filepath.Join(directory, "notes.md"), "not json")
		writeFile(t, filepath.Join(directory, "keep.json.bak"), "{")
		writeFile(t, filepath.Join(directory, "nested", "inner.json"), editSetJSON)
		if err := os.MkdirAll(filepath.Join(directory, "dir.json"), directoryMode); err != nil {
			t.Fatal(err)
		}
		refactors, err := plantedrefactors.Load(root, "refactors")
		if err != nil || !slices.Equal(names(refactors), []string{"keep"}) {
			t.Errorf("got %v, %v", names(refactors), err)
		}
	})
	t.Run("an empty directory gives no refactor"+verifiesSP1972, func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "refactors"), directoryMode); err != nil {
			t.Fatal(err)
		}
		refactors, err := plantedrefactors.Load(root, "refactors")
		if err != nil || len(refactors) != 0 {
			t.Errorf("got %v, %v", refactors, err)
		}
	})
	t.Run("a missing directory is refused"+verifiesSP1972, func(t *testing.T) {
		if _, err := plantedrefactors.Load(t.TempDir(), "absent"); err == nil {
			t.Error("accepted")
		}
	})
	t.Run("unreadable JSON is refused and names the file"+verifiesSP1972, func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "refactors", "broken.json"), `{"edits": [`)
		_, err := plantedrefactors.Load(root, "refactors")
		if err == nil || !strings.Contains(err.Error(), "broken.json") {
			t.Errorf("err = %v", err)
		}
	})
	for name, content := range map[string]string{
		"an empty edit set":              `{"edits":[]}`,
		"an object without edits":        `{}`,
		"an edit without file":           `{"edits":[{"file":"","qualname":"f","new_source":"x"}]}`,
		"an edit without qualname":       `{"edits":[{"file":"src/a.py","qualname":"","new_source":"x"}]}`,
		"an edit with an unknown target": `{"edits":[{"file":"src/a.py","qualname":"f","target":"galaxy","new_source":"x"}]}`,
		"a unit edited twice":            `{"edits":[{"file":"src/a.py","qualname":"f","new_source":"x"},{"file":"src/a.py","qualname":"f","new_source":"y"}]}`,
	} {
		t.Run(name+" is refused"+verifiesSP1972, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "refactors", "invalid.json"), content)
			_, err := plantedrefactors.Load(root, "refactors")
			if !errors.Is(err, mutation.ErrInvalidEditSet) {
				t.Errorf("err = %v", err)
			}
		})
	}
	t.Run("one invalid edit set refuses the whole directory"+verifiesSP1972, func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "refactors", "good.json"), editSetJSON)
		writeFile(t, filepath.Join(root, "refactors", "bad.json"), `{"edits":[]}`)
		refactors, err := plantedrefactors.Load(root, "refactors")
		if err == nil || refactors != nil {
			t.Errorf("got %v, %v", refactors, err)
		}
	})
}
