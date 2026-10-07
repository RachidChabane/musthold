package shallnot

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
)

// Report format.
const (
	statusActive        = "active"
	versionSeparator    = "."
	versionParts        = 2
	minimumMajor        = 1
	minimumMinor        = 2
	minimumVersionLabel = "1.2"
)

// Errors returned for a report the module cannot use.
var (
	// ErrReportTooOld is returned for a report below schema 1.2 or whose schema
	// version cannot be read.
	ErrReportTooOld = errors.New("shallnot report is older than " + minimumVersionLabel)
	// ErrNoStatementDigest is returned for an active requirement without a
	// statement digest.
	ErrNoStatementDigest = errors.New("shallnot requirement has no statement digest")
	// ErrMalformedReport is returned for a report that is not the documented JSON.
	ErrMalformedReport = errors.New("malformed shallnot report")
)

type wireReport struct {
	SchemaVersion string            `json:"schema_version"`
	Requirements  []wireRequirement `json:"requirements"`
}

type wireRequirement struct {
	ID              string        `json:"id"`
	Revision        int           `json:"revision"`
	Statement       string        `json:"statement"`
	Status          string        `json:"status"`
	StatementDigest string        `json:"statement_digest"`
	Examples        []wireExample `json:"examples"`
	Tests           []wireTest    `json:"tests"`
}

type wireExample struct {
	Index     int                    `json:"index"`
	Operation string                 `json:"operation"`
	Args      map[string]wireLiteral `json:"args"`
	Expect    wireExpect             `json:"expect"`
}

type wireLiteral struct {
	Type  string     `json:"type"`
	Value scalarText `json:"value"`
}

type wireExpect struct {
	Kind  string     `json:"kind"`
	Type  string     `json:"type"`
	Value scalarText `json:"value"`
}

type wireTest struct {
	Name      string `json:"name"`
	Classname string `json:"classname"`
	Source    struct {
		File string `json:"file"`
		Line int    `json:"line"`
	} `json:"source"`
}

// scalarText reads a JSON string, number or boolean as its text.
type scalarText string

func (s *scalarText) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return err
		}
		*s = scalarText(text)
		return nil
	}
	var scalar any
	if err := json.Unmarshal(trimmed, &scalar); err != nil {
		return err
	}
	switch scalar.(type) {
	case nil:
		*s = ""
	case float64, bool:
		*s = scalarText(trimmed)
	default:
		return fmt.Errorf("a literal value is a string, a number or a boolean, not %s", trimmed)
	}
	return nil
}

// ParseReport reads a Shallnot report: the active requirements with their
// statement, digest, examples and bound tests, criteria sorted by reference.
func ParseReport(content []byte) (ports.Report, error) {
	var wire wireReport
	if err := json.Unmarshal(content, &wire); err != nil {
		return ports.Report{}, fmt.Errorf("%w: %v", ErrMalformedReport, err)
	}
	if !atLeastMinimumVersion(wire.SchemaVersion) {
		return ports.Report{}, fmt.Errorf("%w: schema_version %q", ErrReportTooOld, wire.SchemaVersion)
	}
	criteria := make([]spec.Criterion, 0, len(wire.Requirements))
	for _, requirement := range wire.Requirements {
		if requirement.Status != statusActive {
			continue
		}
		criterion, err := requirement.criterion()
		if err != nil {
			return ports.Report{}, err
		}
		criteria = append(criteria, criterion)
	}
	slices.SortFunc(criteria, func(a, b spec.Criterion) int { return a.Ref.Compare(b.Ref) })
	return ports.Report{SchemaVersion: wire.SchemaVersion, Criteria: criteria}, nil
}

func atLeastMinimumVersion(version string) bool {
	parts := strings.Split(version, versionSeparator)
	if len(parts) < versionParts {
		return false
	}
	majorNumber, majorErr := strconv.Atoi(parts[0])
	minorNumber, minorErr := strconv.Atoi(parts[1])
	if majorErr != nil || minorErr != nil {
		return false
	}
	return majorNumber > minimumMajor || (majorNumber == minimumMajor && minorNumber >= minimumMinor)
}

func (r wireRequirement) criterion() (spec.Criterion, error) {
	ref := spec.Ref{ID: r.ID, Revision: r.Revision}
	if r.StatementDigest == "" {
		return spec.Criterion{}, fmt.Errorf("%w: %s", ErrNoStatementDigest, ref)
	}
	examples, err := r.examples()
	if err != nil {
		return spec.Criterion{}, fmt.Errorf("%s: %w", ref, err)
	}
	tests := make([]ecosystem.BoundTest, len(r.Tests))
	for index, test := range r.Tests {
		tests[index] = ecosystem.NewBoundTest(test.Classname, test.Name, test.Source.File, test.Source.Line)
	}
	return spec.Criterion{Ref: ref, Statement: r.Statement, StatementDigest: r.StatementDigest, Examples: examples, Tests: tests}, nil
}

func (r wireRequirement) examples() ([]spec.Example, error) {
	examples := make([]spec.Example, len(r.Examples))
	for index, wire := range r.Examples {
		example, err := wire.example()
		if err != nil {
			return nil, fmt.Errorf("example %d: %w", wire.Index, err)
		}
		examples[index] = example
	}
	slices.SortStableFunc(examples, func(a, b spec.Example) int { return a.Index - b.Index })
	return examples, nil
}

func (e wireExample) example() (spec.Example, error) {
	args := make(map[string]spec.Literal, len(e.Args))
	for name, literal := range e.Args {
		parsed, err := parseLiteral(literal.Type, string(literal.Value))
		if err != nil {
			return spec.Example{}, fmt.Errorf("argument %s: %w", name, err)
		}
		args[name] = parsed
	}
	expect, err := e.Expect.observation()
	if err != nil {
		return spec.Example{}, err
	}
	return spec.Example{Index: e.Index, Call: spec.Call{Operation: e.Operation, Args: args}, Expect: expect}, nil
}

func parseLiteral(typeName, value string) (spec.Literal, error) {
	literalType, err := spec.ParseLiteralType(typeName)
	if err != nil {
		return spec.Literal{}, err
	}
	return spec.Literal{Type: literalType, Value: value}, nil
}

// observation reads what an example expects: a returned literal, or the name of a
// raised exception, which Shallnot reports as the value or, when empty, the type.
func (x wireExpect) observation() (spec.Observation, error) {
	switch spec.ObservationKind(x.Kind) {
	case spec.Returns:
		literal, err := parseLiteral(x.Type, string(x.Value))
		if err != nil {
			return spec.Observation{}, fmt.Errorf("expected value: %w", err)
		}
		return spec.Observation{Kind: spec.Returns, Type: literal.Type, Value: literal.Value}, nil
	case spec.Raises:
		name := string(x.Value)
		if name == "" {
			name = x.Type
		}
		return spec.Observation{Kind: spec.Raises, Value: name}, nil
	}
	return spec.Observation{}, fmt.Errorf("%w: expectation kind %q", ErrMalformedReport, x.Kind)
}
