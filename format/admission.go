package format

import (
	"bytes"
	"errors"
	"fmt"
)

// ErrNotCanonical is returned for statement bytes that are not their own RFC 8785
// canonical form: the bytes that are signed and stored must be exactly the bytes
// the format produces.
var ErrNotCanonical = errors.New("statement bytes are not canonical")

// Admitted is a statement that decodes, satisfies every invariant of the format
// and is stored as its exact canonical bytes.
type Admitted struct {
	Proof Proof
	Bytes []byte
}

// Admit decodes statement bytes, validates them and requires them to be
// canonical. A statement of a predicate type outside the format yields
// ErrUnknownPredicateType, and a violation a *ValidationError.
func Admit(data []byte) (Admitted, error) {
	proof, err := Decode(data)
	if err != nil {
		return Admitted{}, err
	}
	if err := proof.Validate(); err != nil {
		return Admitted{}, err
	}
	canonical, err := CanonicalizeJSON(data)
	if err != nil {
		return Admitted{}, err
	}
	if !bytes.Equal(canonical, data) {
		return Admitted{}, ErrNotCanonical
	}
	return Admitted{Proof: proof, Bytes: data}, nil
}

// Subject returns the single subject of an admitted statement.
func (a Admitted) Subject() Subject { return a.Proof.Subjects()[0] }

// Location returns where the statement lives under the given module: the subject
// key derived from its subject and its cache key.
func (a Admitted) Location(module string) ProofLocation {
	return ProofLocation{
		Module:     module,
		SubjectKey: SubjectKey(a.Subject().Name),
		CacheKey:   a.Proof.Common().CacheKey,
	}
}

// CheckLocation requires the statement to live exactly where its content says it
// does.
func (a Admitted) CheckLocation(found ProofLocation) error {
	expected := a.Location(found.Module)
	if found != expected {
		return fmt.Errorf("statement belongs at %s, found at %s", expected.StatementPath(), found.StatementPath())
	}
	return nil
}
