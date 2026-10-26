// Package golang is the mutation engine of Go sources, built on go/parser and
// go/ast. Every mutation is a textual replacement of one node inside a whole
// declaration or package-level spec, so the layout of the rest of the unit is
// kept.
//
// Qualnames follow the grammar `pkg.Func`, `(*T).Method` and `T.Method`.
// The families that need a construct Go does not have are absent: there is no
// exception class swap on exceptions, and its place is taken by swapping
// references between the error sentinels (`ErrX`, `errX`) a file declares.
package golang

import (
	"fmt"

	"github.com/RachidChabane/musthold/discpower/internal/app/operatorgen"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/footprint"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/operator"
)

// sourceExtension is the extension of Go sources.
const sourceExtension = ".go"

var (
	_ operatorgen.Engine         = (*Engine)(nil)
	_ mutation.AssignmentLocator = (*Engine)(nil)
)

// Engine is the Go implementation of operatorgen.Engine.
type Engine struct{}

// New returns a Go engine.
func New() *Engine { return &Engine{} }

// Extensions lists the extensions the engine handles.
func (*Engine) Extensions() []string { return []string{sourceExtension} }

// Families lists the families the engine produces.
func (*Engine) Families() []operator.Family {
	return []operator.Family{
		operator.FamilyRelational, operator.FamilyArithmetic, operator.FamilyNumericConstant,
		operator.FamilyBooleanLogical, operator.FamilyReturnValue, operator.FamilyModuleConstant,
		operator.FamilyConstructorArgument, operator.FamilyDecimalString, operator.FamilyMinMax,
		operator.FamilyAndOr, operator.FamilyBooleanLiteral, operator.FamilyExceptionClass,
	}
}

// Candidates enumerates the operator mutants of the file in source order.
func (*Engine) Candidates(file mutation.SourceFile) ([]operator.Candidate, error) {
	parsedFile, err := parseSource(file.Path, file.Content)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file.Path, err)
	}
	return parsedFile.candidates(file.Path), nil
}

// SentinelCandidates lists the sentinels the catalogue allows for the file.
func (*Engine) SentinelCandidates(file mutation.SourceFile, seed operator.Seed) ([]operator.SentinelCandidate, error) {
	parsedFile, err := parseSource(file.Path, file.Content)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file.Path, err)
	}
	return parsedFile.sentinelCandidates(file.Path, seed), nil
}

// CallGraph reads the call graph contribution of the file.
func (*Engine) CallGraph(file mutation.SourceFile) (footprint.FileGraph, error) {
	parsedFile, err := parseSource(file.Path, file.Content)
	if err != nil {
		return footprint.FileGraph{}, fmt.Errorf("%s: %w", file.Path, err)
	}
	return parsedFile.callGraph(file.Path), nil
}

// LocateAssignments lists the package-level const and var specs of a source as
// units of kind ecosystem.KindAssignment, so the mutation splicer can check an
// edit of one. It implements mutation.AssignmentLocator.
func (*Engine) LocateAssignments(path string, source []byte) ([]ecosystem.Definition, error) {
	parsedFile, err := parseSource(path, source)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
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
func (*Engine) Normalize(_ string, target mutation.TargetKind, source string) (string, error) {
	return normalizeUnit(target, source)
}
