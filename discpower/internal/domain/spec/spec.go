// Package spec holds the value types of what Shallnot reports about a criterion:
// its statement, its executable examples and the tests bound to it, together with
// the typed literals examples and witnesses are written in.
package spec

import (
	"fmt"
	"math/big"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/format"
)

// Ref designates a criterion as `ID~REV`.
type Ref struct {
	ID       string
	Revision int
}

// String renders the reference as `ID~REV`.
func (r Ref) String() string { return format.SubjectName(r.ID, r.Revision) }

// Compare orders references by identifier, then revision.
func (r Ref) Compare(other Ref) int {
	switch {
	case r.ID < other.ID:
		return -1
	case r.ID > other.ID:
		return 1
	}
	return r.Revision - other.Revision
}

// LiteralType is the type of a literal of the example grammar.
type LiteralType string

// Literal types of ADR 0003's example grammar.
const (
	TypeInteger  LiteralType = "integer"
	TypeDecimal  LiteralType = "decimal"
	TypeString   LiteralType = "string"
	TypeBoolean  LiteralType = "boolean"
	TypeNull     LiteralType = "null"
	TypeDatetime LiteralType = "datetime"
)

// Literal is a typed value written as text, the way Shallnot reports it.
type Literal struct {
	Type  LiteralType `json:"type"`
	Value string      `json:"value"`
}

// isNumeric reports whether the literal is an integer or a decimal.
func (l Literal) isNumeric() bool { return l.Type == TypeInteger || l.Type == TypeDecimal }

// Equal compares two literals by value: numbers compare numerically whatever
// their spelling or their integer-or-decimal type, every other type compares by
// type and text.
func (l Literal) Equal(other Literal) bool {
	if l.isNumeric() && other.isNumeric() {
		left, leftOK := new(big.Rat).SetString(l.Value)
		right, rightOK := new(big.Rat).SetString(other.Value)
		return leftOK && rightOK && left.Cmp(right) == 0
	}
	return l.Type == other.Type && l.Value == other.Value
}

// Call is an operation applied to named arguments.
type Call struct {
	Operation string             `json:"operation"`
	Args      map[string]Literal `json:"args"`
}

// ObservationKind tells a returned value from a raised exception.
type ObservationKind string

// Observation kinds.
const (
	Returns ObservationKind = "returns"
	Raises  ObservationKind = "raises"
)

// Observation is what one call did: it returned a literal or raised an exception.
// For a raise, Value is the name of the exception class and Type is empty.
type Observation struct {
	Kind  ObservationKind `json:"kind"`
	Type  LiteralType     `json:"type,omitempty"`
	Value string          `json:"value"`
}

// Same reports whether two observations are the same behaviour.
func (o Observation) Same(other Observation) bool {
	if o.Kind != other.Kind {
		return false
	}
	if o.Kind == Raises {
		return o.Value == other.Value
	}
	return Literal{Type: o.Type, Value: o.Value}.Equal(Literal{Type: other.Type, Value: other.Value})
}

// Expectation is what an example or a witness expects of a call.
type Expectation = Observation

// Example is one executable example of a criterion.
type Example struct {
	Index  int
	Call   Call
	Expect Expectation
}

// Met reports whether an observation is what the example expects.
func (e Example) Met(observed Observation) bool { return e.Expect.Same(observed) }

// Criterion is one criterion as Shallnot reports it, with the tests bound to it.
type Criterion struct {
	Ref Ref
	// Statement is the criterion's text, examples included.
	Statement string
	// StatementDigest is Shallnot's digest of the statement (ADR 0001).
	StatementDigest string
	Examples        []Example
	Tests           []ecosystem.BoundTest
}

// TestIDs lists the identities of the bound tests without duplicates, in order.
func (c Criterion) TestIDs() []ecosystem.TestID {
	var ids []ecosystem.TestID
	seen := map[ecosystem.TestID]bool{}
	for _, test := range c.Tests {
		if !seen[test.ID] {
			seen[test.ID] = true
			ids = append(ids, test.ID)
		}
	}
	return ids
}

// ErrUnknownLiteralType is returned for a literal type outside the grammar.
var ErrUnknownLiteralType = fmt.Errorf("unknown literal type")

// ParseLiteralType reads a literal type name.
func ParseLiteralType(name string) (LiteralType, error) {
	switch t := LiteralType(name); t {
	case TypeInteger, TypeDecimal, TypeString, TypeBoolean, TypeNull, TypeDatetime:
		return t, nil
	}
	return "", fmt.Errorf("%w: %q", ErrUnknownLiteralType, name)
}
