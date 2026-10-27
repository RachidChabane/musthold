package accept

import (
	"encoding/xml"
	"fmt"
	"sort"
	"strings"
)

// Status is how one test case ended in a run.
type Status string

// The statuses of a test case.
const (
	StatusPassed  Status = "passed"
	StatusFailed  Status = "failed"
	StatusSkipped Status = "skipped"
)

// CaseResult is one test case of a JUnit file.
type CaseResult struct {
	// ID is the case's class name and name, joined by "::".
	ID      string
	Status  Status
	Message string
	// Criterion is the criterion whose test file holds the case, when the case
	// belongs to an oracle file.
	Criterion string
}

type junitFile struct {
	Suites []junitSuite `xml:"testsuite"`
	Cases  []junitCase  `xml:"testcase"`
}

type junitSuite struct {
	Cases  []junitCase  `xml:"testcase"`
	Suites []junitSuite `xml:"testsuite"`
}

type junitCase struct {
	ClassName string         `xml:"classname,attr"`
	Name      string         `xml:"name,attr"`
	Failure   []junitProblem `xml:"failure"`
	Error     []junitProblem `xml:"error"`
	Skipped   []junitProblem `xml:"skipped"`
}

type junitProblem struct {
	Message string `xml:"message,attr"`
}

// ParseJUnit reads the cases of a JUnit file. A case fails when it has a failure
// or error child, and is skipped when it has a skipped child. The criterion of
// each case is read from its class name or name through the file names given.
func ParseJUnit(data []byte, criterionOfFile map[string]string) ([]CaseResult, error) {
	var parsed junitFile
	if err := xml.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("read the JUnit file: %w", err)
	}
	cases := append([]junitCase(nil), parsed.Cases...)
	var collect func(suites []junitSuite)
	collect = func(suites []junitSuite) {
		for _, suite := range suites {
			cases = append(cases, suite.Cases...)
			collect(suite.Suites)
		}
	}
	collect(parsed.Suites)
	results := make([]CaseResult, 0, len(cases))
	for _, raw := range cases {
		result := CaseResult{ID: caseID(raw), Status: StatusPassed}
		switch {
		case len(raw.Failure) > 0:
			result.Status, result.Message = StatusFailed, raw.Failure[0].Message
		case len(raw.Error) > 0:
			result.Status, result.Message = StatusFailed, raw.Error[0].Message
		case len(raw.Skipped) > 0:
			result.Status = StatusSkipped
		}
		result.Criterion = criterionOf(raw, criterionOfFile)
		results = append(results, result)
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].ID < results[j].ID })
	return results, nil
}

func caseID(raw junitCase) string {
	if raw.ClassName == "" {
		return raw.Name
	}
	return raw.ClassName + "::" + raw.Name
}

// criterionOf finds the test file of a case among the dotted segments of its
// class name, then in its name (a collection error is named after the module).
func criterionOf(raw junitCase, criterionOfFile map[string]string) string {
	for _, segment := range append(strings.Split(raw.ClassName, "."), raw.Name) {
		if criterion, found := criterionOfFile[segment]; found {
			return criterion
		}
	}
	return ""
}

// ModuleOfCriterion is the module name of a criterion's test file.
func ModuleOfCriterion(criterion string) string {
	return strings.TrimSuffix(OracleFileName(criterion), pythonSuffix)
}

// ModulesOfCriteria maps the module name of each criterion's test file to the
// criterion.
func ModulesOfCriteria(criteria []string) map[string]string {
	modules := make(map[string]string, len(criteria))
	for _, criterion := range criteria {
		modules[ModuleOfCriterion(criterion)] = criterion
	}
	return modules
}
