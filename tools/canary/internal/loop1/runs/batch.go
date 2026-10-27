package runs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	canary "github.com/RachidChabane/musthold/tools/canary/internal/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/container"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// Refusals of an invocation of the run verb.
var (
	ErrBeforeReset = errors.New("the run's last attempt was a quota stop whose reset time has not come")
	ErrInputs      = errors.New("the inputs freeze cannot drive the runs")
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
// fixes; a quota stop ends it with no error.
func (b Batch) Run(ctx context.Context, inv Invocation) error {
	if err := CheckOrder(inv.Config, inv.Plan.Inputs); err != nil {
		return err
	}
	ledgers, err := ReadLedgers(b.Runner.App.Files, inv.Layout, inv.Plan.Inputs.Order)
	if err != nil {
		return err
	}
	if inv.DryRun {
		return b.printPlan(inv, ledgers)
	}
	if err := checkInputs(inv); err != nil {
		return err
	}
	lock, err := b.Runner.Start(ctx, &inv.Plan, inv.Layout.Root)
	if err != nil {
		return err
	}
	defer lock.Release()
	if err := b.recordInterrupted(ctx, inv, ledgers); err != nil {
		return err
	}
	return b.runOrder(ctx, inv, ledgers)
}

// recordInterrupted writes the run.json of each attempt whose invocation ended
// before it was recorded, as a harness error, and logs it.
func (b Batch) recordInterrupted(ctx context.Context, inv Invocation, ledgers []Ledger) error {
	runs := runsByName(inv.Config)
	for index := range ledgers {
		ledger := &ledgers[index]
		for at, attempt := range ledger.Attempts {
			if attempt.Run != nil {
				continue
			}
			configured := runs[ledger.Name]
			run := study.Run{Attempt: attempt.Number, Outcome: study.OutcomeHarnessError, Reason: ReasonInterruptedAttempt}
			run.Task, run.Arm, run.Repetition = configured.Task.Name, string(configured.Arm), configured.Repetition
			run.Model, run.Prompt = inv.Config.Model, configured.Task.Prompt
			encoded, err := app.EncodeJSON(run)
			if err != nil {
				return err
			}
			if err := b.Runner.App.Files.Write(filepath.Join(inv.Layout.Records(ledger.Name, attempt.Number), record.MetaFile), encoded, false); err != nil {
				return err
			}
			ledger.Attempts[at].Run = &run
			if err := b.log(ctx, ledger, Result{Run: run}); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkInputs refuses a freeze without an allowlist, quota patterns or the entry
// of a task.
func checkInputs(inv Invocation) error {
	inputs := inv.Plan.Inputs
	if len(inputs.EgressAllowlist) == 0 || len(inputs.QuotaPatterns) == 0 {
		return fmt.Errorf("%w: egress_allowlist and quota_patterns are required", ErrInputs)
	}
	for _, task := range inv.Config.Tasks {
		if _, found := inputs.Task(task.Name); !found {
			return fmt.Errorf("%w: no entry for the task %s", ErrInputs, task.Name)
		}
	}
	return nil
}

// Start readies an invocation before its first attempt: it finds the credential
// source, takes the session lock, reads the image's id, and refuses a CLI that
// is not the pin. The caller releases the lock.
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
	imageID, err := r.App.ImageID(ctx, p.Options)
	if err != nil {
		return err
	}
	p.ImageID = imageID
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

// preflightDir is where the CLI's version is checked before the first attempt.
const preflightDir = "preflight"

// runOrder runs the attempts, one at a time, from the first run that is neither
// counted nor blocked.
func (b Batch) runOrder(ctx context.Context, inv Invocation, ledgers []Ledger) error {
	runs := runsByName(inv.Config)
	for index := range ledgers {
		ledger := &ledgers[index]
		for !ledger.Done() {
			if Capped(FirstStart(ledgers), b.Runner.App.Clock.Now()) {
				return b.endPhase(ctx, ledgers)
			}
			if at := ledger.NotBefore(); at != nil && b.Runner.App.Clock.Now().Before(*at) {
				return fmt.Errorf("%w: %s starts no earlier than %s", ErrBeforeReset, ledger.Name, at.Format(time.RFC3339))
			}
			if err := b.Runner.App.CheckMemory(ctx, inv.Plan.Options); err != nil {
				return err
			}
			result, err := b.attempt(ctx, inv, runs[ledger.Name], ledger)
			if err != nil {
				return err
			}
			if result.Run.Outcome == study.OutcomeQuotaStop {
				fmt.Fprintf(b.Stdout, "%s: quota stop; the next invocation resumes here\n", ledger.Name)
				return nil
			}
		}
	}
	fmt.Fprintln(b.Stdout, "every run of the order is counted or blocked")
	return nil
}

// attempt runs the next attempt of a run, adds it to the run's ledger and logs
// what it calls for.
func (b Batch) attempt(ctx context.Context, inv Invocation, configured config.Run, ledger *Ledger) (Result, error) {
	number := ledger.Next()
	expect, _ := inv.Plan.Inputs.Task(configured.Task.Name)
	spec := Spec{
		Run: configured, Dir: inv.Layout.RunDir(ledger.Name, number), Attempt: number, Expect: &expect,
		Egress: container.HostEndpoints(inv.Plan.Inputs.EgressAllowlist),
	}
	result, err := b.Runner.Attempt(ctx, inv.Plan, spec)
	if err != nil {
		return result, err
	}
	run := result.Run
	ledger.Attempts = append(ledger.Attempts, Attempt{Number: number, Run: &run})
	fmt.Fprintf(b.Stdout, "%s attempt %d: %s %s\n", ledger.Name, number, run.Outcome, run.Reason)
	return result, b.log(ctx, ledger, result)
}

// log appends the deviations an attempt calls for.
func (b Batch) log(ctx context.Context, ledger *Ledger, result Result) error {
	run := result.Run
	scope := ledger.Name
	var entries []study.Deviation
	switch run.Outcome {
	case study.OutcomeQuotaStop:
		happened := fmt.Sprintf("attempt %d ended at a quota stop", run.Attempt)
		if run.QuotaReset != nil {
			happened += "; the error names the reset time " + run.QuotaReset.Format(time.RFC3339)
		}
		entries = append(entries, deviation(study.KindQuotaStop, scope, sectionQuota, saysQuota, happened))
	case study.OutcomeHarnessError:
		entries = append(entries, deviation(study.KindHarnessError, scope, sectionOutcomes, saysHarness, fmt.Sprintf("attempt %d: %s", run.Attempt, run.Reason)))
	}
	for _, difference := range result.Provenance {
		entries = append(entries, deviation(study.KindProvenance, scope, sectionProvenance, saysProvenance, fmt.Sprintf("attempt %d: %s", run.Attempt, difference)))
	}
	if result.TokenMatches > 0 {
		entries = append(entries, deviation(study.KindTokenExposed, scope, sectionToken, saysToken,
			fmt.Sprintf("attempt %d: %d matches of the token were replaced by %s in its records", run.Attempt, result.TokenMatches, record.TokenPlaceholder)))
	}
	for _, entry := range entries {
		if err := b.Runner.Deviations.Log(ctx, b.stamped(entry)); err != nil {
			return err
		}
	}
	if ledger.HarnessBlocked() {
		return b.logBlocked(ctx, scope, study.KindHarnessBlocked, fmt.Sprintf("its attempts ended with %d harness errors", ledger.HarnessErrors()))
	}
	return nil
}

func (b Batch) stamped(entry study.Deviation) study.Deviation {
	entry.Time = b.Runner.App.Clock.Now()
	return entry
}

// logBlocked logs a blocked run once.
func (b Batch) logBlocked(ctx context.Context, scope string, kind study.Kind, happened string) error {
	logged, err := b.Runner.Deviations.Logged(kind, scope)
	if err != nil || logged {
		return err
	}
	return b.Runner.Deviations.Log(ctx, b.stamped(deviation(kind, scope, sectionOutcomes, saysBlocked, happened)))
}

// endPhase gives each run its state at the calendar cap, logs the runs it
// blocks, and starts nothing.
func (b Batch) endPhase(ctx context.Context, ledgers []Ledger) error {
	fmt.Fprintln(b.Stdout, "the calendar cap has ended the run phase")
	for _, ledger := range ledgers {
		state := ledger.State(true)
		fmt.Fprintf(b.Stdout, "%s: %s\n", ledger.Name, state)
		var err error
		switch state {
		case study.OutcomeQuotaBlocked:
			err = b.logBlocked(ctx, ledger.Name, study.KindQuotaBlocked, "the calendar cap ended the run phase after a quota stop")
		case study.OutcomeHarnessBlocked:
			err = b.logBlocked(ctx, ledger.Name, study.KindHarnessBlocked, "the calendar cap ended the run phase after a harness error")
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

// ReadLedgers reads every attempt of each run of the order from the work
// directory: each attempt directory `<run>-<n>` with its run.json, or none.
func ReadLedgers(files canary.Files, layout study.Layout, order []string) ([]Ledger, error) {
	ledgers := make([]Ledger, 0, len(order))
	for _, name := range order {
		ledger := Ledger{Name: name}
		dirs, err := files.Glob(filepath.Join(layout.Root, study.RunsDir, name+"-*"))
		if err != nil {
			return nil, err
		}
		for _, dir := range dirs {
			number, err := strconv.Atoi(strings.TrimPrefix(filepath.Base(dir), name+"-"))
			if err != nil || number < 1 {
				continue
			}
			attempt, err := readAttempt(files, layout, name, number)
			if err != nil {
				return nil, err
			}
			ledger.Attempts = append(ledger.Attempts, attempt)
		}
		slices.SortFunc(ledger.Attempts, func(a, b Attempt) int { return a.Number - b.Number })
		ledgers = append(ledgers, ledger)
	}
	return ledgers, nil
}

func readAttempt(files canary.Files, layout study.Layout, name string, number int) (Attempt, error) {
	content, err := files.Read(filepath.Join(layout.Records(name, number), record.MetaFile))
	if err != nil || content == nil {
		return Attempt{Number: number}, err
	}
	var run study.Run
	if err := json.Unmarshal(content, &run); err != nil {
		return Attempt{}, fmt.Errorf("%s attempt %d: %s: %w", name, number, record.MetaFile, err)
	}
	return Attempt{Number: number, Run: &run}, nil
}

// printPlan prints the order with each run's state and, for each run that still
// needs an attempt, every command of that attempt. It reads no credential value
// and starts nothing.
func (b Batch) printPlan(inv Invocation, ledgers []Ledger) error {
	runs := runsByName(inv.Config)
	capped := Capped(FirstStart(ledgers), b.Runner.App.Clock.Now())
	fmt.Fprintf(b.Stdout, "work: %s\nimage: %s\nCLI: %s (must begin with %s)\negress allowlist: %s\nruns, in order: %d\n",
		inv.Layout.Root, inv.Plan.Options.Image, inv.Plan.Options.Profile.Claude, inv.Plan.Options.ClaudePin,
		strings.Join(inv.Plan.Inputs.EgressAllowlist, ", "), len(ledgers))
	for index, ledger := range ledgers {
		state := ledger.State(capped)
		if state == "" {
			state = "to run"
		}
		fmt.Fprintf(b.Stdout, "%2d. %s: %s, %d attempts\n", index+1, ledger.Name, state, len(ledger.Attempts))
	}
	for _, ledger := range ledgers {
		if ledger.Done() || capped {
			continue
		}
		dir := inv.Layout.RunDir(ledger.Name, ledger.Next())
		fmt.Fprintf(b.Stdout, "\n%s, attempt %d, in %s:\n", ledger.Name, ledger.Next(), dir)
		spec := Spec{Run: runs[ledger.Name], Dir: dir, Egress: container.HostEndpoints(inv.Plan.Inputs.EgressAllowlist)}
		for step, line := range b.Runner.PlanLines(inv.Plan.Options, spec) {
			fmt.Fprintf(b.Stdout, "%2d. %s\n", step+1, line)
		}
	}
	return nil
}
