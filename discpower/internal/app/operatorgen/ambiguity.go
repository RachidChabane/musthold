package operatorgen

import (
	"fmt"
	"slices"

	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
)

// unitInFile is a unit key within one file.
type unitInFile struct {
	file string
	unit mutation.UnitKey
}

// ambiguity holds, for each file of the run, the units whose kind and qualified
// name another unit of the file shares, and the ones a run left out. A source
// that defines a name twice, as a conditional redefinition does, cannot be
// spliced by name, so no candidate is drawn on it. The rule is the one of the
// splicer, and it holds for every engine.
type ambiguity struct {
	byFile  map[string]mutation.AmbiguousUnits
	omitted map[unitInFile]bool
}

// ambiguityOf reads the ambiguous units of the files the scope covers.
func (g *Generator) ambiguityOf(files []mutation.SourceFile, scope scope) (*ambiguity, error) {
	found := &ambiguity{byFile: map[string]mutation.AmbiguousUnits{}, omitted: map[unitInFile]bool{}}
	for _, file := range files {
		if !scope.hasFile(file.Path) {
			continue
		}
		splicer, err := g.registry.SplicerFor(file.Path)
		if err != nil {
			return nil, err
		}
		units, err := splicer.Ambiguous(file.Path, file.Content)
		if err != nil {
			return nil, fmt.Errorf("%w: locate the units of %q: %w", ErrUnsplicable, file.Path, err)
		}
		found.byFile[file.Path] = units
	}
	return found, nil
}

// skipped counts the distinct units left out of the run.
func (a *ambiguity) skipped() int { return len(a.omitted) }

// leaveOut drops the candidates whose edit targets an ambiguous unit of the
// file and records the units it left out.
func leaveOut[C any](a *ambiguity, path string, candidates []C, editOf func(C) mutation.Edit) []C {
	units := a.byFile[path]
	return slices.DeleteFunc(candidates, func(c C) bool {
		edit := editOf(c)
		if !units.Covers(edit) {
			return false
		}
		key, _ := edit.Targets()
		a.omitted[unitInFile{file: path, unit: key}] = true
		return true
	})
}
