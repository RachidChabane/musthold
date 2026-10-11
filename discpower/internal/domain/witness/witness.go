// Package witness holds the pure rules of the witnesses of G3-W: the tests a
// blind writer derives from the spec alone, before and without any mutant, and
// what their executions and the scope judges' answers make of them. A witness
// that passes the reference three times out of three, every planted refactor
// where the admission names some, and whose output both scope judges hold fixed
// by its criterion's text alone is block-bearing: a mutant that fails it,
// reproduced, violates the criterion on executed evidence.
package witness

import (
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/format"
)

// Kind tells an example witness from a property witness.
type Kind string

// Witness kinds.
const (
	// KindExample checks one input and the output the spec fixes for it.
	KindExample Kind = "example"
	// KindProperty checks a derandomized family of inputs drawn at the full
	// resolution of their types.
	KindProperty Kind = "property"
)

// Bounds of one writer's answer: three to six examples and at least one
// property, at most MaxProperties of them.
const (
	MinExamples   = 3
	MaxExamples   = 6
	MinProperties = 1
	MaxProperties = 3
)

// Valid reports whether the kind is one of the two.
func (k Kind) Valid() bool { return k == KindExample || k == KindProperty }

// Witness is one test a writer wrote for one clause of a criterion.
type Witness struct {
	// ID is the content digest of the witness: its criterion, name, clause, kind
	// and code.
	ID        string   `json:"id"`
	Criterion spec.Ref `json:"criterion"`
	// Name is the test function name the writer gave it.
	Name   string `json:"name"`
	Clause string `json:"clause"`
	Kind   Kind   `json:"kind"`
	// Code is the complete test function.
	Code string `json:"code"`
}

// New builds a witness and its content digest.
func New(criterion spec.Ref, name, clause string, kind Kind, code string) Witness {
	w := Witness{Criterion: criterion, Name: name, Clause: clause, Kind: kind, Code: code}
	w.ID = digestOf(struct {
		Criterion string `json:"criterion"`
		Name      string `json:"name"`
		Clause    string `json:"clause"`
		Kind      Kind   `json:"kind"`
		Code      string `json:"code"`
	}{criterion.String(), name, clause, kind, code})
	return w
}

// ShortID is the prefix of the id that names the witness in a test file.
func (w Witness) ShortID() string { return w.ID[:shortIDLength] }

const shortIDLength = 12

// Writer is who wrote a witness set: the ticket that answered and the
// provenance of its response.
type Writer struct {
	RequestID string `json:"requestId"`
	Vote      int    `json:"vote"`
	// Draw is the witness draw of the criterion the ticket belongs to: 1 for the
	// first writers, 2 for the further writer of a criterion short of witnessed
	// mutants, 3 for the credit writers a CI run asks.
	Draw      int              `json:"draw"`
	Isolation format.Isolation `json:"isolation"`
	// Marked is set for a response journaled as leaked, exposed or conflicting.
	Marked bool `json:"marked,omitempty"`
	// CreditWithheld is set for a writer whose witnesses block and earn no credit
	// in this run: in CI, the writers the developer's host answered, whose
	// witnesses the coder may have seen.
	CreditWithheld bool `json:"creditWithheld,omitempty"`
}

// AtFloor reports whether the writer's response keeps judged pass credit under
// the isolation floor.
func (w Writer) AtFloor(floor format.Isolation) bool {
	return !w.Marked && !w.CreditWithheld && w.Isolation.AtLeast(floor)
}

// Set is the witnesses one writer wrote for one criterion, with the imports
// their file starts with.
type Set struct {
	Criterion spec.Ref  `json:"criterion"`
	Writer    Writer    `json:"writer"`
	Imports   string    `json:"imports"`
	Witnesses []Witness `json:"witnesses"`
}

// Digest is the digest of the set's content: the criterion, the imports and the
// witnesses. The writer's provenance is not part of it, so the digest journaled
// before generation names exactly what will run.
func (s Set) Digest() string {
	ids := make([]string, len(s.Witnesses))
	for index, w := range s.Witnesses {
		ids[index] = w.ID
	}
	return digestOf(struct {
		Criterion string   `json:"criterion"`
		Imports   string   `json:"imports"`
		Witnesses []string `json:"witnesses"`
	}{s.Criterion.String(), s.Imports, ids})
}

// Counts reports how many examples and properties the set holds.
func (s Set) Counts() (examples, properties int) {
	for _, w := range s.Witnesses {
		switch w.Kind {
		case KindExample:
			examples++
		case KindProperty:
			properties++
		}
	}
	return examples, properties
}

// Complete reports whether the set holds the examples and properties a writer
// must give.
func (s Set) Complete() bool {
	examples, properties := s.Counts()
	return examples >= MinExamples && examples <= MaxExamples && properties >= MinProperties && properties <= MaxProperties
}

// UniverseDigest digests the sets a pool is run against, whatever their order.
func UniverseDigest(sets []Set) string {
	digests := make([]string, len(sets))
	for index, set := range sets {
		digests[index] = set.Digest()
	}
	slices.Sort(digests)
	return format.SHA256Hex([]byte(strings.Join(digests, "\n")))
}

func digestOf(value any) string {
	canonical, err := format.Canonicalize(value)
	if err != nil {
		panic("witness: a witness record does not canonicalize: " + err.Error())
	}
	return format.SHA256Hex(canonical)
}
