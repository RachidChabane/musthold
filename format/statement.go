package format

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrUnknownPredicateType is returned by Decode for a predicate type outside the
// format. Readers ignore such statements.
var ErrUnknownPredicateType = errors.New("unknown predicate type")

// Predicate is implemented by the three predicate types of the format.
type Predicate interface {
	// Kind names the proof kind of the predicate.
	Kind() Kind
	// Common returns the part shared by the three predicates.
	Common() Core
	check(sink *collector, subject []Subject)
}

// Statement is an in-toto Statement v1 carrying one predicate.
type Statement[P Predicate] struct {
	Type          string    `json:"_type"`
	Subject       []Subject `json:"subject"`
	PredicateType string    `json:"predicateType"`
	Predicate     P         `json:"predicate"`
}

// Proof is the behaviour shared by every statement, whatever its predicate.
type Proof interface {
	Kind() Kind
	Subjects() []Subject
	// Common returns the predicate's shared part: module, inputs, cache key,
	// producer and extensions.
	Common() Core
	// Validate checks every invariant of the format and returns a
	// *ValidationError listing all violations.
	Validate() error
	// Encode validates the statement and returns its JCS bytes.
	Encode() ([]byte, error)
}

// NewStatement wraps a predicate in a Statement v1 with the type URIs of its kind.
func NewStatement[P Predicate](subject []Subject, predicate P) Statement[P] {
	return Statement[P]{
		Type:          StatementType,
		Subject:       subject,
		PredicateType: predicate.Kind().PredicateType(),
		Predicate:     predicate,
	}
}

// Kind names the proof kind of the statement's predicate.
func (s Statement[P]) Kind() Kind { return s.Predicate.Kind() }

// Subjects returns the statement's subjects.
func (s Statement[P]) Subjects() []Subject { return s.Subject }

// Common returns the predicate's shared part.
func (s Statement[P]) Common() Core { return s.Predicate.Common() }

// Validate checks the envelope, the subject rule of the predicate and every
// predicate invariant.
func (s Statement[P]) Validate() error {
	var sink collector
	if s.Type != StatementType {
		sink.add(CodeStatementType, "_type", "%q is not %q", s.Type, StatementType)
	}
	if expected := s.Predicate.Kind().PredicateType(); s.PredicateType != expected {
		sink.add(CodePredicateType, "predicateType", "%q is not %q", s.PredicateType, expected)
	}
	s.Predicate.check(&sink, s.Subject)
	return sink.err()
}

// Encode validates the statement and returns its RFC 8785 canonical bytes, the
// exact bytes that are cached, signed and stored.
func (s Statement[P]) Encode() ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return Canonicalize(s)
}

// DecodeAs decodes a statement whose predicate is known to be P. Unknown members
// are ignored, because minor versions of a predicate are additive. The result is
// not validated.
func DecodeAs[P Predicate](data []byte) (Statement[P], error) {
	var statement Statement[P]
	if err := json.Unmarshal(data, &statement); err != nil {
		return Statement[P]{}, fmt.Errorf("decode statement: %w", err)
	}
	return statement, nil
}

// Decode decodes a statement of any known predicate type. It does not validate
// the result: call Validate on it. A predicate type outside the format yields
// ErrUnknownPredicateType.
func Decode(data []byte) (Proof, error) {
	var envelope struct {
		PredicateType string `json:"predicateType"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("decode statement: %w", err)
	}
	kind, known := KindOfPredicateType(envelope.PredicateType)
	if !known {
		return nil, fmt.Errorf("%w: %q", ErrUnknownPredicateType, envelope.PredicateType)
	}
	switch kind {
	case KindAlignment:
		return decodeProof[Alignment](data)
	case KindDiscriminatingPower:
		return decodeProof[DiscriminatingPower](data)
	default:
		return decodeProof[Completeness](data)
	}
}

func decodeProof[P Predicate](data []byte) (Proof, error) {
	statement, err := DecodeAs[P](data)
	if err != nil {
		return nil, err
	}
	return statement, nil
}
