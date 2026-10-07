package testkit

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/RachidChabane/spec-proof/completeness/internal/app/ports"
)

// FixtureChanges is what a change source would report for a fixture project: every
// file of its change, with the content of the same file in its base as the before
// side, a file the base lacks having none.
func FixtureChanges(t testing.TB, fixture string) ports.ChangeSet {
	t.Helper()
	changeRoot := filepath.Join(FixturesDir(), fixture, changeDirectory)
	baseRoot := filepath.Join(FixturesDir(), fixture, baseDirectory)
	set := ports.ChangeSet{Base: digestOf(fixture + "base")}
	err := filepath.WalkDir(changeRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		relative, err := filepath.Rel(changeRoot, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		after, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		change := ports.Change{Path: name, OldPath: name, After: after}
		if before, err := os.ReadFile(filepath.Join(baseRoot, relative)); err == nil {
			change.Before = before
		}
		set.Changes = append(set.Changes, change)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return set
}
