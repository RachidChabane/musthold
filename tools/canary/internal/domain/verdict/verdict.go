// Package verdict is what a canary check concludes: PASS, FAIL or INCONCLUSIVE,
// the sentence that says why, and pointers into the recorded session that a
// reader can open to see it for themselves.
package verdict

import "fmt"

// Status is the conclusion of one check.
type Status string

// Statuses. A check fails only on positive contrary evidence: a probe that did
// not happen, or a field the harness did not send, is inconclusive.
const (
	Pass         Status = "PASS"
	Fail         Status = "FAIL"
	Inconclusive Status = "INCONCLUSIVE"
)

// Process exit codes of a report.
const (
	ExitAllPass      = 0
	ExitSomeFailed   = 1
	ExitInconclusive = 3
)

// Pointer names the place in a recorded file that supports a verdict.
type Pointer struct {
	File string `json:"file"`
	Line int    `json:"line,omitempty"`
	Note string `json:"note,omitempty"`
}

// String renders the pointer as `file:line`.
func (p Pointer) String() string {
	if p.Line == 0 {
		return p.File
	}
	return fmt.Sprintf("%s:%d", p.File, p.Line)
}

// Verdict is the outcome of one check.
type Verdict struct {
	Check    string    `json:"check"`
	Proves   string    `json:"proves"`
	Status   Status    `json:"status"`
	Summary  string    `json:"summary"`
	Evidence []Pointer `json:"evidence"`
	// Known is the line that says a declared registry entry covers an INCONCLUSIVE
	// verdict; empty when none does.
	Known string `json:"known,omitempty"`
}

// Passed is the verdict of a check that found what it looks for.
func Passed(summary string, evidence ...Pointer) Verdict {
	return Verdict{Status: Pass, Summary: summary, Evidence: evidence}
}

// Failed is the verdict of a check that found evidence against what it looks for.
func Failed(summary string, evidence ...Pointer) Verdict {
	return Verdict{Status: Fail, Summary: summary, Evidence: evidence}
}

// Undecided is the verdict of a check whose evidence is absent.
func Undecided(summary string, evidence ...Pointer) Verdict {
	return Verdict{Status: Inconclusive, Summary: summary, Evidence: evidence}
}

// Named gives a verdict the identity of the check that reached it.
func (v Verdict) Named(check, proves string) Verdict {
	v.Check, v.Proves = check, proves
	if v.Evidence == nil {
		v.Evidence = []Pointer{}
	}
	return v
}

// Report is the verdicts of one harness's canary.
type Report struct {
	Harness  string    `json:"harness"`
	Verdicts []Verdict `json:"verdicts"`
}

// Count returns how many verdicts have the status.
func (r Report) Count(status Status) int {
	count := 0
	for _, v := range r.Verdicts {
		if v.Status == status {
			count++
		}
	}
	return count
}

// Settled reports whether a verdict needs no further attention: it passed, or it
// is INCONCLUSIVE and a declared registry entry covers it.
func (v Verdict) Settled() bool {
	return v.Status == Pass || (v.Status == Inconclusive && v.Known != "")
}

func (r Report) allSettled() bool {
	for _, v := range r.Verdicts {
		if !v.Settled() {
			return false
		}
	}
	return true
}

// ExitCode is 1 when any check failed, 3 when none failed but some are
// inconclusive, and 0 when every check passed or is a known INCONCLUSIVE.
func (r Report) ExitCode() int {
	switch {
	case r.Count(Fail) > 0:
		return ExitSomeFailed
	case len(r.Verdicts) == 0 || !r.allSettled():
		return ExitInconclusive
	default:
		return ExitAllPass
	}
}
