package construct

import (
	"errors"
	"fmt"
	"slices"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
)

// ErrResumeDiverged is returned when the construction records of a domain do not
// match the calls that construction would make again, so that it cannot resume
// where it stopped.
var ErrResumeDiverged = errors.New("the construction records do not match the calls to resume")

// Replay holds what the records say a domain's construction already did: the
// calls the model answered, in order, and the checks of each failed round. A
// builder that replays them rebuilds, without any model call or check, the state
// it had when the last invocation stopped, so that it resumes at the same
// domain, role and attempt. Calls that failed on the harness side are not part
// of it: they used no attempt.
type Replay struct {
	calls  []records.Call
	rounds map[int][]records.Check
	// parsers read the recorded output of a role whose final message is not a
	// document of files; ParseFiles reads every other role's.
	parsers map[records.Role]func(output []byte) ([]File, error)
}

// NewReplay selects the records of one domain.
func NewReplay(log records.Log, domain string) *Replay {
	r := &Replay{rounds: map[int][]records.Check{}, parsers: map[records.Role]func([]byte) ([]File, error){}}
	for _, call := range log.Calls {
		if call.Domain == domain && call.HarnessFailure == "" {
			r.calls = append(r.calls, call)
		}
	}
	for _, check := range log.Checks {
		if check.Domain == domain {
			r.rounds[check.Round] = append(r.rounds[check.Round], check)
		}
	}
	return r
}

// WithParser reads the recorded output of a role with parse in place of
// ParseFiles, and returns the replay.
func (r *Replay) WithParser(role records.Role, parse func(output []byte) ([]File, error)) *Replay {
	r.parsers[role] = parse
	return r
}

// parse reads the recorded output of a call of a role.
func (r *Replay) parse(role records.Role, output []byte) ([]File, error) {
	if parse, found := r.parsers[role]; found {
		return parse(output)
	}
	return ParseFiles(output)
}

// answer is a recorded call: what the model returned and the error that call
// ended with, which is nil for a call that produced files.
type answer struct {
	reply   Reply
	callErr error
}

// nextCall returns the answer recorded for a request, or nil when the records
// hold no more calls. It refuses a request that is not the recorded call.
func (r *Replay) nextCall(request Request) (*answer, error) {
	if r == nil || len(r.calls) == 0 {
		return nil, nil
	}
	call := r.calls[0]
	r.calls = r.calls[1:]
	if call.Role != request.Role || call.Purpose != request.Purpose || call.Attempt != request.Attempt || call.PromptSHA256 != Digest(request.Prompt) {
		return nil, fmt.Errorf("%w: recorded %s %s attempt %d of %s", ErrResumeDiverged, call.Role, call.Purpose, call.Attempt, call.Domain)
	}
	reply := Reply{
		Output: []byte(call.Output), Stderr: []byte(call.Stderr), CodexVersion: call.CodexVersion, CLIVersion: call.CLIVersion,
		Start: call.Start, End: call.End, Exit: call.Exit,
	}
	if call.Error != "" {
		return &answer{reply: reply, callErr: errors.New(call.Error)}, nil
	}
	files, err := r.parse(call.Role, reply.Output)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrResumeDiverged, err)
	}
	reply.Files = files
	return &answer{reply: reply}, nil
}

// Report returns the recorded outcome of a round that failed, or false when the
// round was not recorded or all of its checks passed: the latter is evaluated
// again so that a task that passed is admitted.
func (r *Replay) Report(domain string, number int) (accept.Report, bool) {
	if r == nil {
		return accept.Report{}, false
	}
	checks := r.rounds[number]
	if len(checks) == 0 || !slices.ContainsFunc(checks, func(check records.Check) bool { return !check.Passed }) {
		return accept.Report{}, false
	}
	report := accept.Report{Task: domain}
	for _, check := range checks {
		report.Results = append(report.Results, accept.Result{
			ID: check.ID, Name: check.Name, Passed: check.Passed, Agreed: check.Agreed, Findings: check.Findings, Output: check.Output,
		})
	}
	return report, true
}
