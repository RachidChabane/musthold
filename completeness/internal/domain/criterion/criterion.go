// Package criterion holds the specifications of a project as the module sees
// them: their files, active criteria, statement digests, examples and the
// dimensions their owners declared out of scope. The module never parses a spec;
// it takes all of this from Shallnot's report.
package criterion

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/RachidChabane/musthold/format"
)

// Ref identifies a criterion: its requirement ID and revision.
type Ref struct {
	ID       string
	Revision int
}

// String renders the reference as `ID~REV`.
func (r Ref) String() string { return format.SubjectName(r.ID, r.Revision) }

// Compare orders references by ID, then revision.
func (r Ref) Compare(other Ref) int {
	return cmp.Or(strings.Compare(r.ID, other.ID), cmp.Compare(r.Revision, other.Revision))
}

// ParseRef reads an `ID~REV` reference.
func ParseRef(text string) (Ref, error) {
	id, revision, _ := strings.Cut(text, format.RevisionSeparator)
	number, err := strconv.Atoi(revision)
	if err != nil || !format.IsCriterion(text) {
		return Ref{}, fmt.Errorf("%q is not a criterion reference", text)
	}
	return Ref{ID: id, Revision: number}, nil
}

// Example is the part of a spec example the module uses: its position in the
// criterion and the operation it calls.
type Example struct {
	Index     int
	Operation string
}

// Criterion is one active requirement of a spec.
type Criterion struct {
	Ref             Ref
	Statement       string
	StatementDigest string
	Examples        []Example
}

// Declaration is a dimension the owner of a spec declared out of scope.
type Declaration struct {
	Dimension string
	Reason    string
	Line      int
}

// Spec is one spec file: its active criteria and its out-of-scope declarations.
type Spec struct {
	// Path is the spec file relative to the project root, with `/` separators.
	Path       string
	Criteria   []Criterion
	OutOfScope []Declaration
}

// DeclaredDimensions lists the dimension names the spec declares out of scope.
func (s Spec) DeclaredDimensions() []string {
	names := make([]string, len(s.OutOfScope))
	for index, declaration := range s.OutOfScope {
		names[index] = declaration.Dimension
	}
	return names
}

// Project is the specs of a project, sorted by path, and the directories its
// tests live in.
type Project struct {
	Specs     []Spec
	TestRoots []string
}

// Criteria lists every criterion of every spec, sorted by reference.
func (p Project) Criteria() []Criterion {
	var all []Criterion
	for _, spec := range p.Specs {
		all = append(all, spec.Criteria...)
	}
	slices.SortFunc(all, func(a, b Criterion) int { return a.Ref.Compare(b.Ref) })
	return all
}

// Refs lists the reference of every criterion, sorted.
func (p Project) Refs() []Ref {
	criteria := p.Criteria()
	refs := make([]Ref, len(criteria))
	for index, c := range criteria {
		refs[index] = c.Ref
	}
	return refs
}

// Find returns the criterion with the reference.
func (p Project) Find(ref Ref) (Criterion, bool) {
	for _, spec := range p.Specs {
		for _, candidate := range spec.Criteria {
			if candidate.Ref == ref {
				return candidate, true
			}
		}
	}
	return Criterion{}, false
}

// SpecOf returns the path of the spec that holds the criterion.
func (p Project) SpecOf(ref Ref) (string, bool) {
	for _, spec := range p.Specs {
		for _, candidate := range spec.Criteria {
			if candidate.Ref == ref {
				return spec.Path, true
			}
		}
	}
	return "", false
}
