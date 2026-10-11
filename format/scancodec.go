package format

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrNotScan is returned by DecodeScan for a statement of another predicate type.
var ErrNotScan = errors.New("not a scan statement")

// Validate checks the envelope and every invariant of the scan predicate, and
// returns a *ValidationError listing all violations.
func (s ScanStatement) Validate() error {
	var sink collector
	if s.Type != StatementType {
		sink.add(CodeStatementType, "_type", "%q is not %q", s.Type, StatementType)
	}
	if s.PredicateType != PredicateTypeScan {
		sink.add(CodePredicateType, "predicateType", "%q is not %q", s.PredicateType, PredicateTypeScan)
	}
	checkScanSubject(&sink, s.Subject)
	s.Predicate.check(&sink)
	return sink.err()
}

// Encode validates the statement and returns its RFC 8785 canonical bytes.
func (s ScanStatement) Encode() ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return Canonicalize(s)
}

// DecodeScan decodes a scan statement without validating it. Unknown members are
// ignored, as in every predicate of the format. A statement of another predicate
// type yields ErrNotScan.
func DecodeScan(data []byte) (ScanStatement, error) {
	var envelope struct {
		PredicateType string `json:"predicateType"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return ScanStatement{}, fmt.Errorf("decode scan statement: %w", err)
	}
	if envelope.PredicateType != PredicateTypeScan {
		return ScanStatement{}, fmt.Errorf("%w: %q", ErrNotScan, envelope.PredicateType)
	}
	var statement ScanStatement
	if err := json.Unmarshal(data, &statement); err != nil {
		return ScanStatement{}, fmt.Errorf("decode scan statement: %w", err)
	}
	return statement, nil
}

// AdmitScan decodes scan statement bytes, validates them and requires them to be
// their own canonical form, the bytes a replay reads back.
func AdmitScan(data []byte) (ScanStatement, error) {
	statement, err := DecodeScan(data)
	if err != nil {
		return ScanStatement{}, err
	}
	if err := statement.Validate(); err != nil {
		return ScanStatement{}, err
	}
	canonical, err := CanonicalizeJSON(data)
	if err != nil {
		return ScanStatement{}, err
	}
	if !bytes.Equal(canonical, data) {
		return ScanStatement{}, ErrNotCanonical
	}
	return statement, nil
}
