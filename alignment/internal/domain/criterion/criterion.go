// Package criterion holds the criteria of a project as the module sees them:
// their references, statements, statement digests and the tests bound to them.
// The module never parses a spec; it takes all of this from Shallnot's report.
package criterion

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"

	"github.com/RachidChabane/spec-proof/format"
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

// BoundTest is a test bound to a criterion by Shallnot's report: the names the
// JUnit reporter gives it and the source location it is declared at.
type BoundTest struct {
	Classname string
	Name      string
	File      string
	Line      int
}

const (
	// labelSeparator joins the classname and the name of a test in its label.
	labelSeparator = "."
	// resultSeparator joins them in the per-test results of a discriminating-power
	// Statement, as the JUnit reporter's test identity does.
	resultSeparator = "::"
)

// Label is the identity of the test in an alignment Statement: the name, or
// `ClassName.Name` when the report gives a classname. Shallnot matches a bound
// test by either form.
func (t BoundTest) Label() string {
	if t.Classname == "" {
		return t.Name
	}
	return t.Classname + labelSeparator + t.Name
}

// ResultID is the identity of the test in the per-test results of a
// discriminating-power Statement.
func (t BoundTest) ResultID() string { return t.Classname + resultSeparator + t.Name }

// Criterion is one active requirement of the project.
type Criterion struct {
	Ref             Ref
	Statement       string
	StatementDigest string
	Tests           []BoundTest
}

// Project is the set of active criteria of a project, sorted by reference.
type Project struct {
	Criteria []Criterion
}

// Refs lists the references of every criterion.
func (p Project) Refs() []Ref {
	refs := make([]Ref, len(p.Criteria))
	for index, criterion := range p.Criteria {
		refs[index] = criterion.Ref
	}
	return refs
}

// Find returns the criterion with the reference.
func (p Project) Find(ref Ref) (Criterion, bool) {
	for _, candidate := range p.Criteria {
		if candidate.Ref == ref {
			return candidate, true
		}
	}
	return Criterion{}, false
}

// Subject is one test and the criteria it is bound to: the unit of work of a
// run, since one masked input serves every criterion the test is bound to.
type Subject struct {
	Test     BoundTest
	Declared []Ref
}

// Subjects groups the bound tests of the project by label and file, in the order
// of their first appearance across the sorted criteria.
func (p Project) Subjects() []Subject {
	var subjects []Subject
	index := map[string]int{}
	for _, candidate := range p.Criteria {
		for _, test := range candidate.Tests {
			key := test.File + "\x00" + test.Label()
			position, seen := index[key]
			if !seen {
				position = len(subjects)
				index[key] = position
				subjects = append(subjects, Subject{Test: test})
			}
			subjects[position].Declared = append(subjects[position].Declared, candidate.Ref)
		}
	}
	return subjects
}
