package execution

import (
	"context"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/coverage"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/differential"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/footprint"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/witness"
)

// Namespaces of the memoized witness and identity facts.
const (
	namespaceWitnessReference = "witness-reference"
	namespaceWitnessEdits     = "witness-edits"
	namespaceIdentity         = "identity"
	namespaceWitnessCoverage  = "witness-coverage"
	referenceFormsKey         = "reference"
)

// Witnesses lays out and runs witness sets in the workspaces of one project.
type Witnesses struct {
	Host    ports.WitnessHost
	Project fs.FS
}

// Available reports whether the ecosystem can run witnesses.
func (w Witnesses) Available() bool { return w.Host != nil }

// witnessOutcome reads what one witness test did in a run: a failure of its body
// is a failure, a pass a pass, and anything else, a timeout or a collection
// failure included, an error.
func witnessOutcome(run ecosystem.RunResult, test ecosystem.BoundTest) witness.Outcome {
	switch {
	case run.TimedOut || run.CollectionFailed:
		return witness.OutcomeErrored
	case run.Outcomes[test.ID] == ecosystem.TestPassed:
		return witness.OutcomePassed
	case run.Outcomes[test.ID] == ecosystem.TestFailed:
		return witness.OutcomeFailed
	}
	return witness.OutcomeErrored
}

// laidOut is a set with its test file, or the reason it has none.
type laidOut struct {
	set  witness.Set
	file ports.WitnessFile
	err  error
}

func (r Runner) layOut(set witness.Set) laidOut {
	file, err := r.Witnesses.Host.Layout(r.Witnesses.Project, set)
	return laidOut{set: set, file: file, err: err}
}

func erroredRuns(set witness.Set, runs int) map[string][]witness.Outcome {
	outcomes := make(map[string][]witness.Outcome, len(set.Witnesses))
	for _, w := range set.Witnesses {
		outcomes[w.ID] = slices.Repeat([]witness.Outcome{witness.OutcomeErrored}, runs)
	}
	return outcomes
}

// ReferenceWitnesses runs a witness set alone on the reference witness.ReferenceRuns
// times and returns what each witness did in each run. A set the ecosystem
// cannot lay out, or that does not build, errors in every run. The result is
// memoized.
func (r Runner) ReferenceWitnesses(ctx context.Context, set witness.Set) (map[string][]witness.Outcome, error) {
	key := r.memoKey(namespaceWitnessReference, set.Digest())
	var cached map[string][]witness.Outcome
	if r.recall(key, &cached) {
		return cached, nil
	}
	laid := r.layOut(set)
	outcomes := erroredRuns(set, witness.ReferenceRuns)
	if laid.err == nil {
		runs, err := r.runSets(ctx, mutation.EditSet{}, []laidOut{laid}, witness.ReferenceRuns)
		if err != nil {
			return nil, err
		}
		if runs != nil {
			outcomes = runs
		}
	}
	return outcomes, r.remember(key, outcomes)
}

// RepeatWitnesses runs a witness set alone on a program, the reference for no
// edits or a mutant, witness.ReferenceRuns times, and returns what each witness
// did in each run. A set the ecosystem cannot lay out, or a program that does
// not build, errors in every run. Nothing is memoized: every call runs.
func (r Runner) RepeatWitnesses(ctx context.Context, edits mutation.EditSet, set witness.Set) (map[string][]witness.Outcome, error) {
	laid := r.layOut(set)
	if laid.err != nil {
		return erroredRuns(set, witness.ReferenceRuns), nil
	}
	runs, err := r.runSets(ctx, edits, []laidOut{laid}, witness.ReferenceRuns)
	if err != nil || runs == nil {
		return erroredRuns(set, witness.ReferenceRuns), err
	}
	return runs, nil
}

// EditWitnesses runs witness sets on an edited program, a mutant or a planted
// refactor, and returns what each witness did: a failure that the rerun
// repeats, a failure it does not (flaky), a pass or an error. Sets already run
// on the program are read from the memo; the others run together in one
// workspace.
func (r Runner) EditWitnesses(ctx context.Context, program string, edits mutation.EditSet, sets []witness.Set) (map[string]witness.Outcome, error) {
	outcomes := map[string]witness.Outcome{}
	var missing []laidOut
	for _, set := range sets {
		var cached map[string]witness.Outcome
		if r.recall(r.memoKey(namespaceWitnessEdits, program, set.Digest()), &cached) {
			for id, outcome := range cached {
				outcomes[id] = outcome
			}
			continue
		}
		missing = append(missing, r.layOut(set))
	}
	if len(missing) == 0 {
		return outcomes, nil
	}
	var runnable []laidOut
	for _, laid := range missing {
		if laid.err == nil {
			runnable = append(runnable, laid)
		}
	}
	runs, err := r.runSets(ctx, edits, runnable, 1)
	if err != nil {
		return nil, err
	}
	separately := len(runnable) > 1 && allErrored(runs)
	if separately {
		if runs, err = r.runEachSet(ctx, edits, runnable); err != nil {
			return nil, err
		}
	}
	if runs, err = r.rerunFailures(ctx, edits, runnable, runs, separately); err != nil {
		return nil, err
	}
	for _, laid := range missing {
		ofSet := map[string]witness.Outcome{}
		for _, w := range laid.set.Witnesses {
			ofSet[w.ID] = witness.OutcomeErrored
			if found, ran := runs[w.ID]; ran && len(found) > 0 {
				ofSet[w.ID] = found[0]
			}
			outcomes[w.ID] = ofSet[w.ID]
		}
		if err := r.remember(r.memoKey(namespaceWitnessEdits, program, laid.set.Digest()), ofSet); err != nil {
			return nil, err
		}
	}
	return outcomes, nil
}

// allErrored reports whether no witness of a batch ran to a pass or a failure:
// in an ecosystem that builds a package with all its test files, one set the
// program breaks takes the others down with it.
func allErrored(runs map[string][]witness.Outcome) bool {
	for _, outcomes := range runs {
		if len(outcomes) > 0 && outcomes[0] != witness.OutcomeErrored {
			return false
		}
	}
	return true
}

// runEachSet runs the sets one by one in their own workspaces.
func (r Runner) runEachSet(ctx context.Context, edits mutation.EditSet, sets []laidOut) (map[string][]witness.Outcome, error) {
	outcomes := map[string][]witness.Outcome{}
	for _, laid := range sets {
		runs, err := r.runSets(ctx, edits, []laidOut{laid}, 1)
		if err != nil {
			return nil, err
		}
		for id, found := range runs {
			outcomes[id] = found
		}
	}
	return outcomes, nil
}

// rerunFailures reruns, in the same program, the sets one of whose witnesses
// failed, each in its own workspace when the batch did not build together: a
// failure the rerun does not repeat is flaky.
func (r Runner) rerunFailures(ctx context.Context, edits mutation.EditSet, sets []laidOut, first map[string][]witness.Outcome, separately bool) (map[string][]witness.Outcome, error) {
	failed := func(id string) bool { return len(first[id]) > 0 && first[id][0] == witness.OutcomeFailed }
	var failing []laidOut
	for _, laid := range sets {
		if slices.ContainsFunc(laid.set.Witnesses, func(w witness.Witness) bool { return failed(w.ID) }) {
			failing = append(failing, laid)
		}
	}
	if len(failing) == 0 {
		return first, nil
	}
	rerun := r.runSets
	if separately {
		rerun = func(ctx context.Context, edits mutation.EditSet, sets []laidOut, _ int) (map[string][]witness.Outcome, error) {
			return r.runEachSet(ctx, edits, sets)
		}
	}
	again, err := rerun(ctx, edits, failing, 1)
	if err != nil {
		return nil, err
	}
	for id := range first {
		if !failed(id) {
			continue
		}
		if repeated, ran := again[id]; !ran || len(repeated) == 0 || repeated[0] != witness.OutcomeFailed {
			first[id] = []witness.Outcome{witness.OutcomeFlaky}
		}
	}
	return first, nil
}

// runSets writes the sets' files into a workspace of the program and runs their
// witnesses the given number of times. It returns nil when the program does not
// build.
func (r Runner) runSets(ctx context.Context, edits mutation.EditSet, sets []laidOut, times int) (map[string][]witness.Outcome, error) {
	if len(sets) == 0 {
		return map[string][]witness.Outcome{}, nil
	}
	outcomes := map[string][]witness.Outcome{}
	var tests []ecosystem.BoundTest
	byWitness := map[string]ecosystem.BoundTest{}
	for _, laid := range sets {
		for _, w := range laid.set.Witnesses {
			if test, known := laid.file.Tests[w.ID]; known {
				tests = append(tests, test)
				byWitness[w.ID] = test
			}
		}
	}
	status, err := r.Executor.Mutate(ctx, r.Reference, edits, func(ctx context.Context, workspace ecosystem.Workspace) error {
		for _, laid := range sets {
			if err := r.Executor.Workspaces.WriteFile(workspace, r.Executor.Ecosystem.Layout(), laid.file.Path, laid.file.Content); err != nil {
				return fmt.Errorf("write the witness file %s: %w", laid.file.Path, err)
			}
		}
		for range times {
			run, err := r.Executor.Run(ctx, workspace, r.Reference.Toolchain, tests, false)
			if err != nil {
				return err
			}
			for _, laid := range sets {
				for _, w := range laid.set.Witnesses {
					outcome := witness.OutcomeErrored
					if test, known := byWitness[w.ID]; known {
						outcome = witnessOutcome(run, test)
					}
					outcomes[w.ID] = append(outcomes[w.ID], outcome)
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("run the witnesses: %w", err)
	}
	if !status.Built {
		return nil, nil
	}
	return outcomes, nil
}

// WitnessEntries records, on the reference, the units the witnesses of the sets
// execute, and returns them as the entry functions of a footprint: the code a
// criterion's blind witnesses reach, whatever the tests. It returns nil when the
// ecosystem records no coverage or no witness ran. The result is memoized.
func (r Runner) WitnessEntries(ctx context.Context, sets []witness.Set) ([]footprint.Ref, error) {
	if r.Recorder == nil || len(sets) == 0 {
		return nil, nil
	}
	key := r.memoKey(namespaceWitnessCoverage, witness.UniverseDigest(sets))
	var cached []footprint.Ref
	if r.recall(key, &cached) {
		return cached, nil
	}
	var laid []laidOut
	var tests []ecosystem.BoundTest
	for _, set := range sets {
		candidate := r.layOut(set)
		if candidate.err != nil {
			continue
		}
		laid = append(laid, candidate)
		for _, w := range set.Witnesses {
			if test, known := candidate.file.Tests[w.ID]; known {
				tests = append(tests, test)
			}
		}
	}
	var recorded coverage.Map
	if len(tests) > 0 {
		_, err := r.Executor.Mutate(ctx, r.Reference, mutation.EditSet{}, func(ctx context.Context, workspace ecosystem.Workspace) error {
			for _, item := range laid {
				if err := r.Executor.Workspaces.WriteFile(workspace, r.Executor.Ecosystem.Layout(), item.file.Path, item.file.Content); err != nil {
					return fmt.Errorf("write the witness file %s: %w", item.file.Path, err)
				}
			}
			var recordErr error
			recorded, recordErr = r.Recorder.Record(ctx, workspace, r.Reference.Toolchain, tests)
			return recordErr
		})
		if err != nil {
			recorded = nil
		}
	}
	ids := make([]ecosystem.TestID, len(tests))
	for index, test := range tests {
		ids[index] = test.ID
	}
	entries := recorded.EntriesOf(ids)
	return entries, r.remember(key, entries)
}

// Identity compares the compiled forms of the units a mutant edits with the
// reference's (E0). It is unknown for an ecosystem with no form compiler. The
// result is memoized.
func (r Runner) Identity(ctx context.Context, mutant MutantSpec) (power.Identity, error) {
	if r.Compiler == nil {
		return power.IdentityUnknown, nil
	}
	sites := sitesOf(mutant.Edits)
	key := r.memoKey(namespaceIdentity, mutant.ID)
	var cached power.Identity
	if r.recall(key, &cached) {
		return cached, nil
	}
	reference, err := r.referenceForms(ctx, sites)
	if err != nil {
		return power.IdentityUnknown, err
	}
	var forms differential.CompiledForms
	status, err := r.Executor.Mutate(ctx, r.Reference, mutant.Edits, func(ctx context.Context, workspace ecosystem.Workspace) error {
		var compileErr error
		forms, compileErr = r.Compiler.Forms(ctx, workspace, r.Reference.Toolchain, sites)
		return compileErr
	})
	if err != nil {
		return power.IdentityUnknown, fmt.Errorf("compile a mutant: %w", err)
	}
	identity := power.IdentityUnknown
	if status.Built {
		identity = differential.CompareForms(sites, reference, forms)
	}
	return identity, r.remember(key, identity)
}

func (r Runner) referenceForms(ctx context.Context, sites []string) (differential.CompiledForms, error) {
	key := r.memoKey(namespaceIdentity, referenceFormsKey, strings.Join(sites, keySeparator))
	var cached differential.CompiledForms
	if r.recall(key, &cached) {
		return cached, nil
	}
	forms, err := r.Compiler.Forms(ctx, r.Reference.Workspace, r.Reference.Toolchain, sites)
	if err != nil {
		return nil, fmt.Errorf("compile the reference: %w", err)
	}
	return forms, r.remember(key, forms)
}

func sitesOf(edits mutation.EditSet) []string {
	var sites []string
	for _, edit := range edits.Canonical().Edits {
		sites = append(sites, edit.File+siteSeparator+edit.Qualname)
	}
	return sites
}
