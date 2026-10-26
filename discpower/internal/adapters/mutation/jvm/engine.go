// Package jvm is the mutation engine of Java and Kotlin sources, built on the
// tree-sitter Java and Kotlin grammars. Every mutation is a textual replacement
// of one node inside a whole method, constructor, function or constant
// declaration, so the layout of the rest of the unit is kept. The syntax of each
// language is a dialect chosen by file extension; the rules that do not depend
// on the shape of a grammar are written once.
package jvm

import (
	"errors"
	"fmt"

	"github.com/RachidChabane/musthold/discpower/internal/app/operatorgen"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/footprint"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/operator"
)

// ErrUnsupportedExtension is returned for a path whose extension no dialect
// handles.
var ErrUnsupportedExtension = errors.New("no java or kotlin dialect for the file")

var (
	_ operatorgen.Engine         = (*Engine)(nil)
	_ mutation.AssignmentLocator = (*Engine)(nil)
)

// Engine is the Java and Kotlin implementation of operatorgen.Engine.
type Engine struct{}

// New returns a Java and Kotlin engine.
func New() *Engine { return &Engine{} }

// Extensions lists the extensions the engine handles.
func (*Engine) Extensions() []string { return extensions() }

// Families lists the families the engine produces.
func (*Engine) Families() []operator.Family { return operator.Families() }

// Candidates enumerates the operator mutants of the file in source order.
func (*Engine) Candidates(file mutation.SourceFile) ([]operator.Candidate, error) {
	parsedFile, err := parseChecked(file.Path, file.Content)
	if err != nil {
		return nil, err
	}
	defer parsedFile.release()
	return parsedFile.candidates(file.Path), nil
}

// SentinelCandidates lists the sentinels the catalogue allows for the file.
func (*Engine) SentinelCandidates(file mutation.SourceFile, seed operator.Seed) ([]operator.SentinelCandidate, error) {
	parsedFile, err := parseChecked(file.Path, file.Content)
	if err != nil {
		return nil, err
	}
	defer parsedFile.release()
	return parsedFile.sentinelCandidates(file.Path, seed), nil
}

// CallGraph reads the call graph contribution of the file.
func (*Engine) CallGraph(file mutation.SourceFile) (footprint.FileGraph, error) {
	parsedFile, err := parseChecked(file.Path, file.Content)
	if err != nil {
		return footprint.FileGraph{}, err
	}
	defer parsedFile.release()
	return parsedFile.callGraph(file.Path), nil
}

// LocateAssignments lists the constant declarations of a source as units of
// kind ecosystem.KindAssignment, so the mutation splicer can check an edit of
// one. It implements mutation.AssignmentLocator.
func (*Engine) LocateAssignments(path string, source []byte) ([]ecosystem.Definition, error) {
	parsedFile, err := parseChecked(path, source)
	if err != nil {
		return nil, err
	}
	defer parsedFile.release()
	var found []ecosystem.Definition
	for _, u := range parsedFile.units() {
		if u.target == mutation.TargetModuleAssignment {
			found = append(found, ecosystem.NewAssignment(source, u.qualname, u.span))
		}
	}
	return found, nil
}

// Normalize reduces a unit to its canonical form, ignoring comments and layout.
// It fails with ErrSyntax when the unit does not parse.
func (*Engine) Normalize(path string, _ mutation.TargetKind, source string) (string, error) {
	d, ok := dialectFor(path)
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrUnsupportedExtension, path)
	}
	parsedUnit, err := parseUnit(d, source)
	if err != nil {
		return "", err
	}
	defer parsedUnit.release()
	return parsedUnit.canonical(), nil
}

// parseChecked parses a whole file with the dialect of its extension and fails
// with the path wrapped around ErrSyntax when it does not parse.
func parseChecked(path string, source []byte) (*parsed, error) {
	d, ok := dialectFor(path)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedExtension, path)
	}
	parsedSource, err := parseSource(d, source)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return parsedSource, nil
}

// parseUnit parses the source of one unit, wrapped when the dialect needs it.
func parseUnit(d dialect, unit string) (*parsed, error) {
	return parseSource(d, []byte(d.wrapUnit(unit)))
}

func parseSource(d dialect, source []byte) (*parsed, error) {
	parsedSource, err := parse(d, source)
	if err != nil {
		return nil, err
	}
	if parsedSource.root().HasError() {
		parsedSource.release()
		return nil, ErrSyntax
	}
	return parsedSource, nil
}
