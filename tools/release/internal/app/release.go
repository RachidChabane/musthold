package app

import (
	"fmt"
	"path"

	"github.com/RachidChabane/spec-proof/tools/release/internal/domain"
)

const notesMode = 0o644

// DistVerifier checks a dist directory against the release it must hold.
type DistVerifier struct {
	Reader DistReader
}

// Verify reads the directory and returns what is wrong with it. A module that
// ships no binary has nothing to verify.
func (v DistVerifier) Verify(dir string, tag domain.Tag, commit string, commitTime int64, targets []domain.Target) ([]domain.Finding, error) {
	if !tag.Module.ShipsBinary() {
		return nil, nil
	}
	facts, err := v.Reader.ReadDist(dir, tag, targets)
	if err != nil {
		return nil, err
	}
	facts.Tag, facts.Commit, facts.CommitTime, facts.Targets = tag, commit, domain.CommitTime(commitTime), targets
	return domain.VerifyDist(facts), nil
}

// FormulaWriter renders the Homebrew formula of a built release.
type FormulaWriter struct{ Files Files }

// Write reads SHA256SUMS of the dist directory and writes the formula next to it.
func (w FormulaWriter) Write(dir string, tag domain.Tag) (string, error) {
	text, err := w.Files.ReadFile(path.Join(dir, domain.ChecksumsFileName))
	if err != nil {
		return "", fmt.Errorf("read the checksums: %w", err)
	}
	sums, err := domain.ParseChecksums(string(text))
	if err != nil {
		return "", err
	}
	formula, err := domain.RenderFormula(tag, sums)
	if err != nil {
		return "", err
	}
	target := path.Join(dir, domain.FormulaFileName(tag.Module.Binary))
	return target, w.Files.WriteFile(target, []byte(formula), domain.FileMode)
}

// NotesWriter renders the release notes of a tag.
type NotesWriter struct {
	History HistoryReader
	Files   Files
}

// Write lists the commits since the previous tag of the module that touched the
// module or the format library, and writes the notes to a file.
func (w NotesWriter) Write(target string, tag domain.Tag) error {
	previous, err := w.History.PreviousTag(tag.Module.Name, tag)
	if err != nil {
		return fmt.Errorf("find the previous tag: %w", err)
	}
	commits, err := w.History.CommitsSince(previous, notePaths(tag.Module))
	if err != nil {
		return fmt.Errorf("list the changes: %w", err)
	}
	notes := domain.RenderNotes(domain.NotesInput{Tag: tag, Previous: previous, Commits: commits})
	return w.Files.WriteFile(target, []byte(notes), notesMode)
}

// notePaths are the directories whose changes belong in the notes of a module:
// the module itself and the format library it builds against.
func notePaths(module domain.Module) []string {
	if module.Name == domain.FormatModuleName {
		return []string{module.Name}
	}
	return []string{module.Name, domain.FormatModuleName}
}
