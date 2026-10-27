package runs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	canary "github.com/RachidChabane/musthold/tools/canary/internal/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/container"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/phase"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// Refusals of an invocation of the run verb.
var (
	ErrBeforeReset = errors.New("the run's last attempt was a quota stop whose reset time has not come")
	ErrInputs      = errors.New("the inputs freeze cannot drive the runs")
	ErrImage       = errors.New("the engine's image is not the one the inputs freeze names")
)

// Invocation is one invocation of the run verb.
type Invocation struct {
	Plan   Plan
	Config config.Config
	Layout study.Layout
	DryRun bool
}

// Batch runs the order of the study until every run is counted or blocked, the
// calendar cap ends the run phase, or an attempt is a quota stop (PREREG §6,
// amendment a1).
type Batch struct {
	Runner Runner
	Stdout io.Writer
}

// Run runs one invocation. It refuses before any attempt when the order, the
// inputs freeze, the credential, the image or the CLI is not what the study
// fixes, and before each attempt when the engine no longer runs that image; a
// quota stop ends it with no error.
func (b Batch) Run(ctx context.Context, inv Invocation) error {
	if err := CheckOrder(inv.Config, inv.Plan.Inputs); err != nil {
		return err
	}
	attempts, err := phase.Load(inv.Layout, inv.Plan.Inputs.Order)
	if err != nil {
		return err
	}
	if inv.DryRun {
		return b.printPlan(inv, attempts)
	}
	if err := checkInputs(inv); err != nil {
		return err
	}
	lock, err := b.Runner.Start(ctx, &inv.Plan, inv.Layout.Root)
	if err != nil {
		return err
	}
	defer lock.Release()
	if err := b.recordInterrupted(ctx, inv, attempts); err != nil {
		return err
	}
	return b.runOrder(ctx, inv, attempts)
}

// checkInputs refuses a freeze without an allowlist or quota patterns, or without
// a task's base commit and equipped tree in each arm.
func checkInputs(inv Invocation) error {
	inputs := inv.Plan.Inputs
	if len(inputs.EgressAllowlist) == 0 || len(inputs.QuotaPatterns) == 0 {
		return fmt.Errorf("%w: egress_allowlist and quota_patterns are required", ErrInputs)
	}
	for _, task := range inv.Config.Tasks {
		entry, found := inputs.Task(task.Name)
		if !found || entry.BaseCommit == "" {
			return fmt.Errorf("%w: no base commit for the task %s", ErrInputs, task.Name)
		}
		for _, arm := range inv.Config.Arms {
			if entry.EquippedTree[string(arm)] == "" {
				return fmt.Errorf("%w: no equipped tree for the task %s in the arm %s", ErrInputs, task.Name, arm)
			}
		}
	}
	return nil
}

// Start readies an invocation before its first attempt: it finds the credential
// source, takes the session lock, refuses an image that is not the frozen one,
// and refuses a CLI that is not the pin. The caller releases the lock.
func (r Runner) Start(ctx context.Context, p *Plan, root string) (canary.Lock, error) {
	source, err := canary.FirstCredential(ctx, r.App.Credentials)
	if err := app.Present(source, err); err != nil {
		return nil, err
	}
	p.Source = source
	lock, err := r.App.Lock(p.Options)
	if err != nil {
		return nil, err
	}
	if err := r.preflight(ctx, p, root); err != nil {
		lock.Release()
		return nil, err
	}
	return lock, nil
}

func (r Runner) preflight(ctx context.Context, p *Plan, root string) error {
	if err := r.CheckImage(ctx, p); err != nil {
		return err
	}
	dir := filepath.Join(root, preflightDir)
	if err := r.App.Files.MkdirAll(dir); err != nil {
		return err
	}
	check, err := r.App.Open(p.Options, dir)
	if err != nil {
		return err
	}
	defer check.Close(ctx)
	_, err = r.App.Versions(ctx, check, p.Options)
	return err
}

// CheckImage reads the id of the image the engine runs, and refuses one that is
// not the image the inputs freeze names, or an engine that does not answer.
func (r Runner) CheckImage(ctx context.Context, p *Plan) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	imageID, err := r.App.ImageID(ctx, p.Options)
	if err != nil {
		return err
	}
	if built := p.Inputs.Images.Built; built != "" && imageID != built {
		return fmt.Errorf("%w: the engine names %s, the inputs freeze %s", ErrImage, imageID, built)
	}
	p.ImageID = imageID
	return nil
}

// preflightDir is where the CLI's version is checked before the first attempt.
const preflightDir = "preflight"

// recordInterrupted writes the run.json of each attempt whose invocation ended
// before it was recorded, as a harness error, and logs it.
func (b Batch) recordInterrupted(ctx context.Context, inv Invocation, attempts map[string][]phase.Attempt) error {
	runs := runsByName(inv.Config)
	for _, name := range inv.Plan.Inputs.Order {
		for index, attempt := range attempts[name] {
			if !Interrupted(attempt) {
				continue
			}
			configured := runs[name]
			run := study.Run{Attempt: attempt.Number, Outcome: study.OutcomeHarnessError, Reason: ReasonInterruptedAttempt}
			run.Task, run.Arm, run.Repetition = configured.Task.Name, string(configured.Arm), configured.Repetition
			run.Model, run.Prompt = inv.Config.Model, configured.Task.Prompt
			encoded, err := app.EncodeJSON(run)
			if err != nil {
				return err
			}
			if err := b.Runner.App.Files.Write(filepath.Join(inv.Layout.Records(name, attempt.Number), record.MetaFile), encoded, false); err != nil {
				return err
			}
			attempts[name][index].Run = run
			if err := b.log(ctx, inv, name, attempts[name], Result{Run: run}); err != nil {
				return err
			}
		}
	}
	return nil
}

// runOrder runs the attempts, one at a time, from the first run that is neither
// counted nor blocked.
func (b Batch) runOrder(ctx context.Context, inv Invocation, attempts map[string][]phase.Attempt) error {
	runs := runsByName(inv.Config)
	order := inv.Plan.Inputs.Order
	for index, name := range order {
		for {
			judged := phase.Judge(order, attempts, b.Runner.App.Clock.Now())
			if judged.CapReached {
				return b.endPhase(ctx, inv, judged)
			}
			if judged.Runs[index].Closed() {
				break
			}
			if at := NotBefore(attempts[name]); at != nil && b.Runner.App.Clock.Now().Before(*at) {
				return fmt.Errorf("%w: %s starts no earlier than %s", ErrBeforeReset, name, at.Format(time.RFC3339))
			}
			if err := b.ready(ctx, &inv); err != nil {
				return err
			}
			result, err := b.attempt(ctx, inv, runs[name], attempts)
			if err != nil {
				return err
			}
			if result.Run.Outcome == study.OutcomeQuotaStop {
				fmt.Fprintf(b.Stdout, "%s: quota stop; the next invocation resumes here\n", name)
				return nil
			}
		}
	}
	fmt.Fprintln(b.Stdout, "every run of the order is counted or blocked")
	return nil
}

// ready refuses to start an attempt on a host the memory guard refuses, after an
// interruption, or on an engine that no longer runs the frozen image.
func (b Batch) ready(ctx context.Context, inv *Invocation) error {
	if err := b.Runner.App.CheckMemory(ctx, inv.Plan.Options); err != nil {
		return err
	}
	return b.Runner.CheckImage(ctx, &inv.Plan)
}

// attempt runs the next attempt of a run, adds it to the run's attempts and logs
// what it calls for.
func (b Batch) attempt(ctx context.Context, inv Invocation, configured config.Run, attempts map[string][]phase.Attempt) (Result, error) {
	name := configured.Name()
	number := NextAttempt(attempts[name])
	expect, _ := inv.Plan.Inputs.Task(configured.Task.Name)
	dir := inv.Layout.RunDir(name, number)
	spec := Spec{Run: configured, Dir: dir, Attempt: number, Expect: &expect, Egress: container.HostEndpoints(inv.Plan.Inputs.EgressAllowlist)}
	result, err := b.Runner.Attempt(ctx, inv.Plan, spec)
	if err != nil {
		return result, err
	}
	attempts[name] = append(attempts[name], phase.Attempt{Number: number, Dir: dir, Run: result.Run})
	fmt.Fprintf(b.Stdout, "%s attempt %d: %s %s\n", name, number, result.Run.Outcome, result.Run.Reason)
	return result, b.log(ctx, inv, name, attempts[name], result)
}

// log appends the deviations an attempt calls for.
func (b Batch) log(ctx context.Context, inv Invocation, name string, attempts []phase.Attempt, result Result) error {
	run := result.Run
	var entries []study.Deviation
	switch run.Outcome {
	case study.OutcomeQuotaStop:
		happened := fmt.Sprintf("attempt %d ended at a quota stop", run.Attempt)
		if run.QuotaReset != nil {
			happened += "; the error names the reset time " + run.QuotaReset.Format(time.RFC3339)
		}
		entries = append(entries, entryOf(study.KindQuotaStop, name, sectionQuota, saysQuota, happened))
	case study.OutcomeHarnessError:
		entries = append(entries, entryOf(study.KindHarnessError, name, sectionOutcomes, saysHarness, fmt.Sprintf("attempt %d: %s", run.Attempt, run.Reason)))
	}
	for _, difference := range result.Provenance {
		entries = append(entries, entryOf(study.KindProvenance, name, sectionProvenance, saysProvenance, fmt.Sprintf("attempt %d: %s", run.Attempt, difference)))
	}
	if result.TokenMatches > 0 {
		entries = append(entries, entryOf(study.KindTokenExposed, name, sectionToken, saysToken,
			fmt.Sprintf("attempt %d: %d matches of the token were replaced by %s in its records", run.Attempt, result.TokenMatches, record.TokenPlaceholder)))
	}
	for _, entry := range entries {
		if err := b.Runner.Deviations.Append(ctx, b.dated(entry)); err != nil {
			return err
		}
	}
	if phase.StateOf(name, attempts, false).State == study.OutcomeHarnessBlocked {
		return b.logBlocked(ctx, inv, name, study.KindHarnessBlocked, "its attempts ended with three harness errors")
	}
	return nil
}

func (b Batch) dated(entry study.Deviation) study.Deviation {
	entry.Time = b.Runner.App.Clock.Now()
	return entry
}

// logBlocked logs a blocked run once.
func (b Batch) logBlocked(ctx context.Context, inv Invocation, name string, kind study.Kind, happened string) error {
	already, err := logged(b.Runner.App.Files, inv.Layout, kind, name)
	if err != nil || already {
		return err
	}
	return b.Runner.Deviations.Append(ctx, b.dated(entryOf(kind, name, sectionOutcomes, saysBlocked, happened)))
}

// endPhase prints each run's state at the calendar cap, logs the runs it
// blocks, and starts nothing.
func (b Batch) endPhase(ctx context.Context, inv Invocation, judged phase.Phase) error {
	fmt.Fprintf(b.Stdout, "the calendar cap ended the run phase at %s\n", judged.Cap.UTC().Format(time.RFC3339))
	for _, state := range judged.Runs {
		fmt.Fprintf(b.Stdout, "%s: %s\n", state.Name, state.State)
		var err error
		switch state.State {
		case study.OutcomeQuotaBlocked:
			err = b.logBlocked(ctx, inv, state.Name, study.KindQuotaBlocked, "the calendar cap ended the run phase after a quota stop")
		case study.OutcomeHarnessBlocked:
			err = b.logBlocked(ctx, inv, state.Name, study.KindHarnessBlocked, "the calendar cap ended the run phase after a harness error")
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func runsByName(c config.Config) map[string]config.Run {
	runs := map[string]config.Run{}
	for _, run := range c.Runs() {
		runs[run.Name()] = run
	}
	return runs
}

// printPlan prints the order with each run's state and, for each run that still
// needs an attempt, every command of that attempt. It reads no credential value
// and starts nothing.
func (b Batch) printPlan(inv Invocation, attempts map[string][]phase.Attempt) error {
	runs := runsByName(inv.Config)
	order := inv.Plan.Inputs.Order
	judged := phase.Judge(order, attempts, b.Runner.App.Clock.Now())
	fmt.Fprintf(b.Stdout, "work: %s\nimage: %s\nCLI: %s (must begin with %s)\negress allowlist: %s\nruns, in order: %d\n",
		inv.Layout.Root, inv.Plan.Options.Image, inv.Plan.Options.Profile.Claude, inv.Plan.Options.ClaudePin,
		joinedOr(inv.Plan.Inputs.EgressAllowlist, "<egress_allowlist of the inputs freeze>"), len(order))
	for index, state := range judged.Runs {
		described := string(state.State)
		if described == "" {
			described = "to run"
		}
		fmt.Fprintf(b.Stdout, "%2d. %s: %s, %d attempts\n", index+1, state.Name, described, len(state.Attempts))
	}
	for _, state := range judged.Runs {
		if state.State != "" {
			continue
		}
		number := NextAttempt(attempts[state.Name])
		dir := inv.Layout.RunDir(state.Name, number)
		fmt.Fprintf(b.Stdout, "\n%s, attempt %d, in %s:\n", state.Name, number, dir)
		spec := Spec{Run: runs[state.Name], Dir: dir, Egress: container.HostEndpoints(inv.Plan.Inputs.EgressAllowlist)}
		for step, line := range b.Runner.PlanLines(inv.Plan.Options, spec) {
			fmt.Fprintf(b.Stdout, "%2d. %s\n", step+1, line)
		}
	}
	return nil
}

func joinedOr(values []string, fallback string) string {
	if len(values) == 0 {
		return fallback
	}
	return strings.Join(values, ", ")
}
