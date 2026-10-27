package construct

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
)

// Acceptor runs the acceptance checks on a task lying in the repository.
type Acceptor interface {
	Accept(ctx context.Context, task string, round int) (accept.Report, error)
}

// Errors that stop construction.
var (
	// ErrFixtureAmendment is returned when a check of the fixed task can only be
	// satisfied by changing its fixture or its reference.
	ErrFixtureAmendment = errors.New("the task's fixture or reference would have to change")
	// ErrFixedTaskDropped is returned when the fixed task fails acceptance after
	// its repairs.
	ErrFixedTaskDropped = errors.New("the fixed task failed acceptance after its repairs")
)

// The check a call that produced no usable files is reported as.
const (
	checkCallID   = "call"
	checkCallName = "model call"
)

// builder constructs one task: the calls of its roles, their repairs, and the
// acceptance rounds in between.
type builder struct {
	c      *Constructor
	author authorRole
	// attempts counts the calls per role; repairs those that repair.
	attempts map[records.Role]int
	repairs  map[records.Role]int
	// authorFiles and testFiles are the last files each role wrote.
	authorFiles []File
	testFiles   []File
	// testsSpec is the digest of the spec the current tests were written from.
	testsSpec string
	// replay holds the calls and rounds an earlier invocation already recorded
	// for this task; nil when there are none.
	replay *replay
}

// outcome is how a task's construction ended.
type outcome struct {
	accepted bool
	drop     *records.Drop
}

func newBuilder(c *Constructor, author authorRole, history records.Log) *builder {
	return &builder{
		c: c, author: author, attempts: map[records.Role]int{}, repairs: map[records.Role]int{},
		replay: newReplay(history, author.name()),
	}
}

// run writes the task, then evaluates and repairs it until it passes, cannot be
// repaired or needs a change that is not allowed.
func (b *builder) run(ctx context.Context) (outcome, error) {
	failures, err := b.writeAuthor(ctx, records.PurposeGenerate, nil)
	if err != nil {
		return outcome{}, err
	}
	for round := 1; ; round++ {
		if len(failures) == 0 {
			if failures, err = b.ensureTests(ctx); err != nil {
				return outcome{}, err
			}
		}
		report, err := b.evaluate(ctx, round, failures)
		if err != nil {
			return outcome{}, err
		}
		if report.Passed() {
			return outcome{accepted: true}, nil
		}
		problems, blocked := b.route(report)
		var called bool
		if failures, called, err = b.repair(ctx, problems); err != nil {
			return outcome{}, err
		}
		if !called {
			return b.stop(report, blocked)
		}
	}
}

// evaluate runs the acceptance checks, or reports the failure of a call that
// left nothing to check.
func (b *builder) evaluate(ctx context.Context, round int, failures []records.Finding) (accept.Report, error) {
	if report, recorded := b.replay.report(b.author.name(), round); recorded {
		return report, nil
	}
	if len(failures) == 0 {
		return b.c.Acceptor.Accept(ctx, b.author.name(), round)
	}
	result := accept.Result{ID: checkCallID, Name: checkCallName, Findings: failures, Agreed: true}
	entry := records.Check{Domain: b.author.name(), Round: round, ID: result.ID, Name: result.Name, Agreed: true, Findings: failures}
	if err := b.c.Records.Append(records.Entry{Check: &entry}); err != nil {
		return accept.Report{}, err
	}
	return accept.Report{Task: b.author.name(), Results: []accept.Result{result}}, nil
}

// route sorts the findings of a failed report by the role they concern. Findings
// that need a change to a fixed fixture or reference are returned apart.
func (b *builder) route(report accept.Report) (map[records.Role][]string, []records.Finding) {
	problems := map[records.Role][]string{}
	var blocked []records.Finding
	for _, result := range report.Failed() {
		for _, finding := range result.Findings {
			if unfixable(b.author, finding) {
				blocked = append(blocked, finding)
				continue
			}
			problems[finding.Role] = append(problems[finding.Role], fmt.Sprintf("%s %s: %s", result.ID, result.Name, finding.Text))
		}
	}
	return problems, blocked
}

// repair makes one repair call for each role that has problems and calls left,
// and reports whether it made any. When the author's repair changed the spec the
// tests are written again from the new spec, which costs no repair call.
func (b *builder) repair(ctx context.Context, problems map[records.Role][]string) ([]records.Finding, bool, error) {
	authorRepaired := b.canRepair(records.RoleAuthor, problems)
	if authorRepaired {
		b.c.progress("%s: repairing the author (%d of %d)", b.author.name(), b.repairs[records.RoleAuthor]+1, MaxRepairs)
		failures, err := b.writeAuthor(ctx, records.PurposeRepair, problems[records.RoleAuthor])
		if err != nil || len(failures) > 0 {
			return failures, true, err
		}
		stale, err := b.testsStale()
		if err != nil {
			return b.badSpec(err), true, nil
		}
		if stale && b.testsSpec != "" {
			failures, err := b.writeTests(ctx, records.PurposeGenerate, nil)
			return failures, true, err
		}
	}
	if b.canRepair(records.RoleOracle, problems) {
		b.c.progress("%s: repairing the tests (%d of %d)", b.author.name(), b.repairs[records.RoleOracle]+1, MaxRepairs)
		failures, err := b.writeTests(ctx, records.PurposeRepair, problems[records.RoleOracle])
		return failures, true, err
	}
	return nil, authorRepaired, nil
}

func (b *builder) canRepair(role records.Role, problems map[records.Role][]string) bool {
	return len(problems[role]) > 0 && b.repairs[role] < MaxRepairs
}

// stop ends a task that no repair can improve.
func (b *builder) stop(report accept.Report, blocked []records.Finding) (outcome, error) {
	first := report.Failed()[0]
	if len(blocked) > 0 {
		return outcome{}, fmt.Errorf("%w: check %s %s: %s", ErrFixtureAmendment, first.ID, first.Name, blocked[0].Text)
	}
	reason := "no finding"
	if len(first.Findings) > 0 {
		reason = first.Findings[0].Text
	}
	drop := &records.Drop{Domain: b.author.name(), Check: first.ID + " " + first.Name, Reason: reason, Time: b.c.Now().UTC()}
	if err := b.c.Records.Append(records.Entry{Drop: drop}); err != nil {
		return outcome{}, err
	}
	return outcome{drop: drop}, nil
}

// writeAuthor makes a call to the author and writes its files. A call that fails
// or answers with files that cannot be used yields a failure for the next round.
func (b *builder) writeAuthor(ctx context.Context, purpose records.Purpose, problems []string) ([]records.Finding, error) {
	prompt, err := b.author.prompt(b.authorFiles, problems)
	if err != nil {
		return nil, err
	}
	files, failure, err := b.call(ctx, records.RoleAuthor, purpose, prompt)
	if err != nil || failure != nil {
		return failure, err
	}
	b.authorFiles = files
	if err := b.author.apply(b.c.Store, files); err != nil {
		return []records.Finding{{Role: records.RoleAuthor, Target: records.TargetOutput, Text: err.Error()}}, nil
	}
	return nil, nil
}

// ensureTests writes the tests when none exist for the current spec.
func (b *builder) ensureTests(ctx context.Context) ([]records.Finding, error) {
	stale, err := b.testsStale()
	if err != nil {
		return b.badSpec(err), nil
	}
	if !stale {
		return nil, nil
	}
	return b.writeTests(ctx, records.PurposeGenerate, nil)
}

func (b *builder) badSpec(err error) []records.Finding {
	return []records.Finding{{Role: records.RoleAuthor, Target: records.TargetOutput, Text: err.Error()}}
}

func (b *builder) testsStale() (bool, error) {
	spec, _, err := b.author.tests(b.c.Store)
	if err != nil {
		return false, err
	}
	return digest(spec) != b.testsSpec, nil
}

// writeTests makes a call to the test writer, which is given the spec and the
// interface and nothing else, and writes its files.
func (b *builder) writeTests(ctx context.Context, purpose records.Purpose, problems []string) ([]records.Finding, error) {
	spec, iface, err := b.author.tests(b.c.Store)
	if err != nil {
		return b.badSpec(err), nil
	}
	input := TestsInput{Spec: spec, Interface: iface, Problems: problems}
	if purpose == records.PurposeRepair {
		input.Previous = b.testFiles
	}
	prompt, err := b.c.Prompts.Tests(input)
	if err != nil {
		return []records.Finding{{Role: records.RoleAuthor, Target: records.TargetOutput, Text: err.Error()}}, nil
	}
	files, failure, err := b.call(ctx, records.RoleOracle, purpose, prompt)
	if err != nil || failure != nil {
		return failure, err
	}
	b.testFiles = files
	b.testsSpec = digest(spec)
	if err := CheckTestFiles(files); err != nil {
		return []records.Finding{{Role: records.RoleOracle, Target: records.TargetOutput, Text: err.Error()}}, nil
	}
	if err := b.c.Store.WriteTests(b.author.name(), files); err != nil {
		return nil, err
	}
	return nil, nil
}

// call makes one model call and records it. A call the model could not answer is
// returned as a failure on the role and counts as an attempt; a call that failed
// on the harness side is recorded, counts as nothing, and is returned as an
// error so that construction stops where it is. Only a failure to record is
// otherwise an error.
func (b *builder) call(ctx context.Context, role records.Role, purpose records.Purpose, prompt string) ([]File, []records.Finding, error) {
	schema, err := FilesSchema()
	if err != nil {
		return nil, nil, err
	}
	request := Request{Role: role, Purpose: purpose, Domain: b.author.name(), Attempt: b.attempts[role] + 1, Prompt: prompt, Schema: schema}
	recorded, err := b.replay.nextCall(request)
	if err != nil {
		return nil, nil, err
	}
	reply, callErr := b.answer(ctx, request, recorded)
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	var harness *HarnessFailure
	if errors.As(callErr, &harness) {
		return nil, nil, b.stopOnHarness(request, reply, callErr, harness)
	}
	b.attempts[role]++
	if purpose == records.PurposeRepair {
		b.repairs[role]++
	}
	if recorded == nil {
		if err := b.c.Records.Append(records.Entry{Call: callRecord(request, reply, callErr)}); err != nil {
			return nil, nil, err
		}
	}
	if callErr != nil {
		return nil, []records.Finding{{Role: role, Target: records.TargetOutput, Text: "the call failed: " + callErr.Error()}}, nil
	}
	return reply.Files, nil, nil
}

// answer returns the recorded answer of a call that an earlier invocation made,
// or asks the model.
func (b *builder) answer(ctx context.Context, request Request, recorded *answer) (Reply, error) {
	if recorded != nil {
		return recorded.reply, recorded.callErr
	}
	return b.c.Model.Complete(ctx, request)
}

// stopOnHarness records a call that failed on the harness side and returns the
// error that stops construction, naming where it stopped.
func (b *builder) stopOnHarness(request Request, reply Reply, callErr error, harness *HarnessFailure) error {
	entry := callRecord(request, reply, callErr)
	entry.HarnessFailure = string(harness.Kind)
	if err := b.c.Records.Append(records.Entry{Call: entry}); err != nil {
		return err
	}
	return fmt.Errorf("%s %s attempt %d (%s): %w", request.Domain, request.Role, request.Attempt, request.Purpose, callErr)
}

func callRecord(request Request, reply Reply, callErr error) *records.Call {
	call := &records.Call{
		Role: request.Role, Purpose: request.Purpose, Domain: request.Domain, Attempt: request.Attempt,
		PromptSHA256: digest(request.Prompt), Schema: string(request.Schema.JSON),
		Output: string(reply.Output), Stderr: string(reply.Stderr), CodexVersion: reply.CodexVersion,
		Start: reply.Start.UTC(), End: reply.End.UTC(), Exit: reply.Exit,
	}
	if callErr != nil {
		call.Error = callErr.Error()
	}
	return call
}

func digest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// Constructor builds the tasks of the study.
type Constructor struct {
	Repo     string
	Config   string
	Store    Store
	Model    Model
	Acceptor Acceptor
	Records  records.Writer
	Prompts  Prompts
	Domains  []Domain
	// History reads the records back so that construction resumes where an earlier
	// invocation stopped; nil means there is nothing to resume.
	History records.Reader
	// Target is how many domains must pass; zero means PassTarget.
	Target   int
	Now      func() time.Time
	Progress io.Writer
}

func (c *Constructor) progress(format string, args ...any) {
	if c.Progress != nil {
		fmt.Fprintf(c.Progress, format+"\n", args...)
	}
}
