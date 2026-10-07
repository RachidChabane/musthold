// Package javascript is the mutation engine of JavaScript and TypeScript
// sources, built on the tree-sitter JavaScript, TypeScript and TSX grammars.
// Every mutation is a textual replacement of one node inside a whole outermost
// definition or module-level declarator, so the layout of the rest of the unit
// is kept and the units agree with the shared definition locator.
package javascript

import (
	"fmt"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/operatorgen"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/footprint"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/operator"
)

var (
	_ operatorgen.Engine         = (*Engine)(nil)
	_ mutation.AssignmentLocator = (*Engine)(nil)
)

// Engine is the JavaScript and TypeScript implementation of operatorgen.Engine.
type Engine struct{}

// New returns a JavaScript and TypeScript engine.
func New() *Engine { return &Engine{} }

// Extensions lists the extensions the engine handles.
func (*Engine) Extensions() []string { return append([]string(nil), sourceExtensions...) }

// Families lists the families the engine produces.
func (*Engine) Families() []operator.Family { return operator.Families() }

// Candidates enumerates the operator mutants of the file in source order.
func (*Engine) Candidates(file mutation.SourceFile) ([]operator.Candidate, error) {
	parsedFile, err := parseChecked(file.Path, file.Content)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file.Path, err)
	}
	defer parsedFile.release()
	return parsedFile.candidates(file.Path), nil
}

// SentinelCandidates lists the sentinels the catalogue allows for the file.
func (*Engine) SentinelCandidates(file mutation.SourceFile, seed operator.Seed) ([]operator.SentinelCandidate, error) {
	parsedFile, err := parseChecked(file.Path, file.Content)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file.Path, err)
	}
	defer parsedFile.release()
	return parsedFile.sentinelCandidates(file.Path, seed), nil
}

// CallGraph reads the call graph contribution of the file.
func (*Engine) CallGraph(file mutation.SourceFile) (footprint.FileGraph, error) {
	parsedFile, err := parseChecked(file.Path, file.Content)
	if err != nil {
		return footprint.FileGraph{}, fmt.Errorf("%s: %w", file.Path, err)
	}
	defer parsedFile.release()
	return parsedFile.callGraph(file.Path), nil
}

// LocateAssignments lists the module-level declarators of a source as units of
// kind ecosystem.KindAssignment, so the mutation splicer can check an edit of
// one. It implements mutation.AssignmentLocator.
func (*Engine) LocateAssignments(path string, source []byte) ([]ecosystem.Definition, error) {
	parsedFile, err := parseChecked(path, source)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
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

// parseChecked parses a source and rejects one the parser could not read
// completely.
func parseChecked(filePath string, source []byte) (*parsed, error) {
	parsedSource, err := parse(filePath, source)
	if err != nil {
		return nil, err
	}
	if parsedSource.root().HasError() {
		parsedSource.release()
		return nil, ErrSyntax
	}
	return parsedSource, nil
}
