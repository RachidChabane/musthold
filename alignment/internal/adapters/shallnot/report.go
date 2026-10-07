package shallnot

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/RachidChabane/spec-proof/alignment/internal/app/ports"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/criterion"
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
	ID              string     `json:"id"`
	Revision        int        `json:"revision"`
	Statement       string     `json:"statement"`
	Status          string     `json:"status"`
	StatementDigest string     `json:"statement_digest"`
	Tests           []wireTest `json:"tests"`
}

type wireTest struct {
	Name      string `json:"name"`
	Classname string `json:"classname"`
	Source    struct {
		File string `json:"file"`
		Line int    `json:"line"`
	} `json:"source"`
}

// ParseReport reads a Shallnot report: the active requirements with their
// statement, digest and bound tests, criteria sorted by reference. The statement
// is Shallnot's joined text, examples included.
func ParseReport(content []byte) (ports.Report, error) {
	var wire wireReport
	if err := json.Unmarshal(content, &wire); err != nil {
		return ports.Report{}, fmt.Errorf("%w: %v", ErrMalformedReport, err)
	}
	if !atLeastMinimumVersion(wire.SchemaVersion) {
		return ports.Report{}, fmt.Errorf("%w: schema_version %q", ErrReportTooOld, wire.SchemaVersion)
	}
	criteria := make([]criterion.Criterion, 0, len(wire.Requirements))
	for _, requirement := range wire.Requirements {
		if requirement.Status != statusActive {
			continue
		}
		parsed, err := requirement.criterion()
		if err != nil {
			return ports.Report{}, err
		}
		criteria = append(criteria, parsed)
	}
	slices.SortFunc(criteria, func(a, b criterion.Criterion) int { return a.Ref.Compare(b.Ref) })
	return ports.Report{SchemaVersion: wire.SchemaVersion, Project: criterion.Project{Criteria: criteria}}, nil
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

func (r wireRequirement) criterion() (criterion.Criterion, error) {
	ref := criterion.Ref{ID: r.ID, Revision: r.Revision}
	if r.StatementDigest == "" {
		return criterion.Criterion{}, fmt.Errorf("%w: %s", ErrNoStatementDigest, ref)
	}
	tests := make([]criterion.BoundTest, len(r.Tests))
	for index, test := range r.Tests {
		tests[index] = criterion.BoundTest{Classname: test.Classname, Name: test.Name, File: test.Source.File, Line: test.Source.Line}
	}
	return criterion.Criterion{Ref: ref, Statement: r.Statement, StatementDigest: r.StatementDigest, Tests: tests}, nil
}
