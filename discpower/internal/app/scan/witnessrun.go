package scan

import (
	"context"
	"io/fs"

	"github.com/RachidChabane/musthold/discpower/internal/app/execution"
	"github.com/RachidChabane/musthold/discpower/internal/app/mutantrun"
	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/footprint"
	"github.com/RachidChabane/musthold/discpower/internal/domain/witness"
)

// WitnessRunner runs the witness sets of a scan's promises on the programs of
// one reference: the reference itself and its mutants. Witness files are
// written only into workspaces of their own, never into one a suite runs in.
type WitnessRunner interface {
	// Reference runs a set alone on the reference witness.ReferenceRuns times
	// and gives the outcomes of each witness. It is the admission of the set,
	// remembered when the runner has a memo.
	Reference(ctx context.Context, set witness.Set) (map[string][]witness.Outcome, error)
	// Repeat runs a set alone on a program, the reference for no edits,
	// witness.ReferenceRuns times; it never reads a memo.
	Repeat(ctx context.Context, edits mutation.EditSet, set witness.Set) (map[string][]witness.Outcome, error)
	// Entries records, on the reference, the units the sets run, as the entry
	// functions of a footprint: nil when the ecosystem records no coverage or
	// no witness ran. It is remembered when the runner has a memo.
	Entries(ctx context.Context, sets []witness.Set) ([]footprint.Ref, error)
	// OnMutant runs the sets on a mutant and gives the outcome of each witness:
	// a failure the rerun repeats, a failure it does not (flaky), a pass or an
	// error. It never reads a memo.
	OnMutant(ctx context.Context, program string, edits mutation.EditSet, sets []witness.Set) (map[string]witness.Outcome, error)
}

// WitnessReference is the built reference a witness runner works on.
type WitnessReference struct {
	Executor  mutantrun.Executor
	Reference mutantrun.Reference
}

// WitnessRunners is the Factory of the witness runner of a reference. With a
// memo, the admission runs and the entries are remembered under the suite key;
// a nil memo remembers nothing.
type WitnessRunners interface {
	For(reference WitnessReference, memo ports.ExecutionMemo, suiteKey string) WitnessRunner
}

// ExecutionWitnesses runs witnesses with the module's execution runner, the
// ecosystem's witness host and, when it has them, its runner of witnesses and
// its coverage recorder.
type ExecutionWitnesses struct {
	Host ports.WitnessHost
	// Runner runs the witnesses; nil runs them with the ecosystem's suite
	// runner.
	Runner ports.SuiteRunner
	// Recorder records the coverage of the witnesses; nil for an ecosystem that
	// records none, whose generation view is then every unit of the code.
	Recorder ports.CoverageRecorder
	Project  fs.FS
}

var _ WitnessRunners = ExecutionWitnesses{}

// For is the execution runner of the reference, as a witness runner.
func (w ExecutionWitnesses) For(reference WitnessReference, memo ports.ExecutionMemo, suiteKey string) WitnessRunner {
	executor := reference.Executor
	if w.Runner != nil {
		executor.Ecosystem = witnessEcosystem{Ecosystem: executor.Ecosystem, runner: w.Runner}
	}
	runner := execution.Runner{
		Executor: executor, Reference: reference.Reference, SuiteKey: suiteKey,
		Witnesses: execution.Witnesses{Host: w.Host, Project: w.Project}, Recorder: w.Recorder,
	}
	if memo != nil {
		runner.Memo = memo
	}
	return executionWitnessRunner{memoized: runner, fresh: withoutMemo(runner)}
}

// withoutMemo is the runner with no memo, for the runs no fact may be read
// back for.
func withoutMemo(runner execution.Runner) execution.Runner {
	runner.Memo = nil
	return runner
}

// executionWitnessRunner is the Adapter of the execution runner to the scan's
// witness runner: the admission and the entries go through the memo, every
// other run is fresh.
type executionWitnessRunner struct {
	memoized execution.Runner
	fresh    execution.Runner
}

func (r executionWitnessRunner) Reference(ctx context.Context, set witness.Set) (map[string][]witness.Outcome, error) {
	return r.memoized.ReferenceWitnesses(ctx, set)
}

func (r executionWitnessRunner) Repeat(ctx context.Context, edits mutation.EditSet, set witness.Set) (map[string][]witness.Outcome, error) {
	return r.fresh.RepeatWitnesses(ctx, edits, set)
}

func (r executionWitnessRunner) Entries(ctx context.Context, sets []witness.Set) ([]footprint.Ref, error) {
	return r.memoized.WitnessEntries(ctx, sets)
}

func (r executionWitnessRunner) OnMutant(ctx context.Context, program string, edits mutation.EditSet, sets []witness.Set) (map[string]witness.Outcome, error) {
	return r.fresh.EditWitnesses(ctx, program, edits, sets)
}

// witnessEcosystem is the Decorator of an ecosystem whose witnesses run with a
// runner of their own.
type witnessEcosystem struct {
	ports.Ecosystem
	runner ports.SuiteRunner
}

// Runner is the runner of the witnesses.
func (e witnessEcosystem) Runner() ports.SuiteRunner { return e.runner }
