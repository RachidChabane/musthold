package shallnot

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/RachidChabane/musthold/completeness/internal/app/ports"
	"github.com/RachidChabane/musthold/completeness/internal/domain/criterion"
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
	// ErrNoSpecFile is returned for an active requirement that names no spec file.
	ErrNoSpecFile = errors.New("shallnot requirement has no spec file")
	// ErrMalformedReport is returned for a report that is not the documented JSON.
	ErrMalformedReport = errors.New("malformed shallnot report")
)

type wireReport struct {
	SchemaVersion string            `json:"schema_version"`
	Run           wireRun           `json:"run"`
	Requirements  []wireRequirement `json:"requirements"`
	OutOfScope    []wireOutOfScope  `json:"out_of_scope"`
}

type wireRun struct {
	SpecFiles []string `json:"spec_files"`
	TestRoots []string `json:"test_roots"`
}

type wireRequirement struct {
	ID              string        `json:"id"`
	Revision        int           `json:"revision"`
	Statement       string        `json:"statement"`
	Status          string        `json:"status"`
	StatementDigest string        `json:"statement_digest"`
	Location        wireLocation  `json:"location"`
	Examples        []wireExample `json:"examples"`
}

type wireLocation struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

type wireExample struct {
	Index     int    `json:"index"`
	Operation string `json:"operation"`
}

type wireOutOfScope struct {
	SpecFile  string `json:"spec_file"`
	Line      int    `json:"line"`
	Dimension string `json:"dimension"`
	Reason    string `json:"reason"`
}

// ParseReport reads a Shallnot report: every spec file with its active
// requirements, their statement, digest and examples, and its out-of-scope
// declarations, plus the test roots. Specs are sorted by path and criteria by
// reference. A report without `out_of_scope` declares nothing out of scope.
func ParseReport(content []byte) (ports.Report, error) {
	var wire wireReport
	if err := json.Unmarshal(content, &wire); err != nil {
		return ports.Report{}, fmt.Errorf("%w: %v", ErrMalformedReport, err)
	}
	if !atLeastMinimumVersion(wire.SchemaVersion) {
		return ports.Report{}, fmt.Errorf("%w: schema_version %q", ErrReportTooOld, wire.SchemaVersion)
	}
	specs := map[string]*criterion.Spec{}
	specOf := func(file string) *criterion.Spec {
		file = path.Clean(strings.ReplaceAll(file, `\`, "/"))
		if specs[file] == nil {
			specs[file] = &criterion.Spec{Path: file}
		}
		return specs[file]
	}
	for _, file := range wire.Run.SpecFiles {
		specOf(file)
	}
	for _, requirement := range wire.Requirements {
		if requirement.Status != statusActive {
			continue
		}
		parsed, err := requirement.criterion()
		if err != nil {
			return ports.Report{}, err
		}
		spec := specOf(requirement.Location.File)
		spec.Criteria = append(spec.Criteria, parsed)
	}
	for _, declared := range wire.OutOfScope {
		spec := specOf(declared.SpecFile)
		spec.OutOfScope = append(spec.OutOfScope, criterion.Declaration{Dimension: declared.Dimension, Reason: declared.Reason, Line: declared.Line})
	}
	return ports.Report{SchemaVersion: wire.SchemaVersion, Project: assemble(specs, wire.Run.TestRoots)}, nil
}

func assemble(specs map[string]*criterion.Spec, testRoots []string) criterion.Project {
	project := criterion.Project{TestRoots: slices.Sorted(slices.Values(cleaned(testRoots)))}
	for _, spec := range specs {
		slices.SortFunc(spec.Criteria, func(a, b criterion.Criterion) int { return a.Ref.Compare(b.Ref) })
		slices.SortFunc(spec.OutOfScope, func(a, b criterion.Declaration) int {
			return cmp.Or(strings.Compare(a.Dimension, b.Dimension), strings.Compare(a.Reason, b.Reason), cmp.Compare(a.Line, b.Line))
		})
		project.Specs = append(project.Specs, *spec)
	}
	slices.SortFunc(project.Specs, func(a, b criterion.Spec) int { return strings.Compare(a.Path, b.Path) })
	return project
}

func cleaned(roots []string) []string {
	result := make([]string, len(roots))
	for index, root := range roots {
		result[index] = path.Clean(strings.ReplaceAll(root, `\`, "/"))
	}
	return result
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
	switch {
	case r.StatementDigest == "":
		return criterion.Criterion{}, fmt.Errorf("%w: %s", ErrNoStatementDigest, ref)
	case r.Location.File == "":
		return criterion.Criterion{}, fmt.Errorf("%w: %s", ErrNoSpecFile, ref)
	}
	examples := make([]criterion.Example, len(r.Examples))
	for index, example := range r.Examples {
		examples[index] = criterion.Example{Index: example.Index, Operation: example.Operation}
	}
	slices.SortStableFunc(examples, func(a, b criterion.Example) int { return cmp.Compare(a.Index, b.Index) })
	return criterion.Criterion{Ref: ref, Statement: r.Statement, StatementDigest: r.StatementDigest, Examples: examples}, nil
}
