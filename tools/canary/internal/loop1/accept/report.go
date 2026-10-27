package accept

import (
	"fmt"
	"io"
	"strings"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
)

// The identifiers and names of the acceptance checks (PREREG §3.4).
const (
	CheckShape      = "A1"
	CheckFixture    = "A2"
	CheckReference  = "A3"
	CheckBase       = "A4"
	CheckDiagonal   = "A5"
	CheckRefactors  = "A6"
	CheckRoom       = "A7"
	CheckLeak       = "A8"
	CheckCollection = "A9"
)

var checkNames = map[string]string{
	CheckShape:      "shape",
	CheckFixture:    "fixture",
	CheckReference:  "reference",
	CheckBase:       "base",
	CheckDiagonal:   "diagonal",
	CheckRefactors:  "refactors",
	CheckRoom:       "room",
	CheckLeak:       "leak",
	CheckCollection: "collection",
}

// CheckOrder lists the checks in the order they run.
func CheckOrder() []string {
	return []string{CheckShape, CheckFixture, CheckReference, CheckBase, CheckDiagonal, CheckRefactors, CheckRoom, CheckLeak, CheckCollection}
}

// Result is the outcome of one check.
type Result struct {
	ID   string
	Name string
	// Passed is whether the check's condition holds; Agreed whether its three runs
	// agreed with one another. Skipped marks a check that did not run because the
	// shape check failed.
	Passed  bool
	Agreed  bool
	Skipped bool
	// Findings say what failed, each written for the role it concerns.
	Findings []records.Finding
	// Output is the raw output of every command the check ran.
	Output string
}

// Report is the outcome of the checks on one task.
type Report struct {
	Task    string
	Results []Result
}

// Passed is whether every check passed.
func (r Report) Passed() bool {
	for _, result := range r.Results {
		if !result.Passed {
			return false
		}
	}
	return len(r.Results) > 0
}

// Failed lists the checks that ran and failed, in order.
func (r Report) Failed() []Result {
	var failed []Result
	for _, result := range r.Results {
		if !result.Passed && !result.Skipped {
			failed = append(failed, result)
		}
	}
	return failed
}

// FindingsFor lists the findings of every failed check that concern a role.
func (r Report) FindingsFor(role records.Role) []records.Finding {
	var found []records.Finding
	for _, result := range r.Failed() {
		for _, finding := range result.Findings {
			if finding.Role == role {
				found = append(found, finding)
			}
		}
	}
	return found
}

// Print writes the report: one line per check, then the findings of each failed
// check.
func (r Report) Print(out io.Writer) {
	verdict := "PASS"
	if !r.Passed() {
		verdict = "FAIL"
	}
	fmt.Fprintf(out, "%s: %s\n", r.Task, verdict)
	for _, result := range r.Results {
		state := "ok"
		switch {
		case result.Skipped:
			state = "not run"
		case !result.Passed && !result.Agreed:
			state = "FAILED (the runs disagree)"
		case !result.Passed:
			state = "FAILED"
		}
		fmt.Fprintf(out, "  %s %-10s %s\n", result.ID, result.Name, state)
		for _, finding := range result.Findings {
			fmt.Fprintf(out, "    [%s, %s] %s\n", finding.Role, finding.Target, strings.ReplaceAll(finding.Text, "\n", "\n      "))
		}
	}
}

// result starts a passing result for a check.
func newResult(id string) *Result {
	return &Result{ID: id, Name: checkNames[id], Passed: true, Agreed: true}
}

// fail records a finding and fails the result.
func (res *Result) fail(role records.Role, target records.Target, format string, args ...any) {
	res.Passed = false
	res.Findings = append(res.Findings, records.Finding{Role: role, Target: target, Text: fmt.Sprintf(format, args...)})
}

// disagree fails the result because the runs of a command differ. The author
// is told about its own target; the test writer too when its tests ran.
func (res *Result) disagree(d disagreement, label string, authorTarget records.Target, withOracle bool) {
	res.Passed, res.Agreed = false, false
	const text = "the three runs on %s do not give the same result: a test or the code under test depends on something other than its inputs"
	res.Findings = append(res.Findings, records.Finding{Role: records.RoleAuthor, Target: authorTarget, Text: fmt.Sprintf(text, label)})
	if withOracle {
		res.Findings = append(res.Findings, records.Finding{Role: records.RoleOracle, Target: records.TargetOracle, Text: fmt.Sprintf(text, label)})
	}
	res.Output += d.Error()
}
