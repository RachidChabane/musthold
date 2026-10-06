// Package junitxml reads per-test outcomes from the JUnit XML that pytest,
// jest-junit, Vitest, Surefire, Gradle and gotestsum write.
package junitxml

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
)

// ErrUnknownFormat is returned for a report format without a dialect.
var ErrUnknownFormat = errors.New("unknown report format")

const testcaseElement = "testcase"

// dialect holds what differs between reporters. An error element is raised by
// the body of a test under Surefire, Gradle and Vitest, so it counts as a
// failure there; pytest and jest-junit write it for setup, teardown and suite
// problems, which prove nothing about the code.
type dialect struct {
	errorIsFailure bool
}

var dialects = map[ecosystem.ReportFormat]dialect{
	ecosystem.ReportPytest:    {errorIsFailure: false},
	ecosystem.ReportJestJUnit: {errorIsFailure: false},
	ecosystem.ReportVitest:    {errorIsFailure: true},
	ecosystem.ReportSurefire:  {errorIsFailure: true},
	ecosystem.ReportGradle:    {errorIsFailure: true},
	ecosystem.ReportGotestsum: {errorIsFailure: false},
}

type testcase struct {
	Classname string    `xml:"classname,attr"`
	Name      string    `xml:"name,attr"`
	Failure   *struct{} `xml:"failure"`
	Error     *struct{} `xml:"error"`
	Skipped   *struct{} `xml:"skipped"`
}

func (d dialect) outcome(tc testcase) ecosystem.TestOutcome {
	switch {
	case tc.Failure != nil:
		return ecosystem.TestFailed
	case tc.Error != nil && d.errorIsFailure:
		return ecosystem.TestFailed
	case tc.Error != nil:
		return ecosystem.TestErrored
	case tc.Skipped != nil:
		return ecosystem.TestSkipped
	default:
		return ecosystem.TestPassed
	}
}

// Parse reads one JUnit XML document into per-test outcomes. A test reported
// several times keeps its most severe outcome.
func Parse(reader io.Reader, format ecosystem.ReportFormat) (map[ecosystem.TestID]ecosystem.TestOutcome, error) {
	d, ok := dialects[format]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownFormat, format)
	}
	outcomes := map[ecosystem.TestID]ecosystem.TestOutcome{}
	if err := collect(reader, d, outcomes); err != nil {
		return nil, err
	}
	return outcomes, nil
}

func collect(reader io.Reader, d dialect, into map[ecosystem.TestID]ecosystem.TestOutcome) error {
	decoder := xml.NewDecoder(reader)
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("decode junit xml: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != testcaseElement {
			continue
		}
		var tc testcase
		if err := decoder.DecodeElement(&tc, &start); err != nil {
			return fmt.Errorf("decode testcase: %w", err)
		}
		id := ecosystem.NewTestID(tc.Classname, tc.Name)
		outcome := d.outcome(tc)
		if previous, seen := into[id]; seen {
			outcome = ecosystem.MergeOutcome(previous, outcome)
		}
		into[id] = outcome
	}
}

var _ ports.ResultsReader = (*Reader)(nil)

// Reader implements ports.ResultsReader over files on disk.
type Reader struct{}

// NewReader returns a Reader.
func NewReader() *Reader { return &Reader{} }

// Read expands the glob patterns, parses every matching file and merges the
// outcomes. A pattern that matches nothing contributes nothing.
func (*Reader) Read(patterns []string, format ecosystem.ReportFormat) (map[ecosystem.TestID]ecosystem.TestOutcome, error) {
	if _, ok := dialects[format]; !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownFormat, format)
	}
	var files []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("glob %q: %w", pattern, err)
		}
		files = append(files, matches...)
	}
	slices.Sort(files)
	merged := map[ecosystem.TestID]ecosystem.TestOutcome{}
	for _, file := range files {
		parsed, err := parseFile(file, format)
		if err != nil {
			return nil, err
		}
		for id, outcome := range parsed {
			if previous, seen := merged[id]; seen {
				outcome = ecosystem.MergeOutcome(previous, outcome)
			}
			merged[id] = outcome
		}
	}
	return merged, nil
}

func parseFile(file string, format ecosystem.ReportFormat) (map[ecosystem.TestID]ecosystem.TestOutcome, error) {
	handle, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", file, err)
	}
	defer handle.Close()
	outcomes, err := Parse(handle, format)
	if err != nil {
		return nil, fmt.Errorf("%q: %w", file, err)
	}
	return outcomes, nil
}
