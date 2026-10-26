// Package execution runs mutants against the suite and turns what happened into
// the executed facts of the discriminating-power pipeline: the kill vector (F2),
// the examples (E1), the differential search (F3) and the confirmation of
// witnesses (F4). Every fact is memoized under a key that names the mutant and
// everything the run depended on, so a wave that recomputes a pool re-executes
// nothing.
package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"

	"github.com/RachidChabane/musthold/discpower/internal/app/mutantrun"
	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/coverage"
	"github.com/RachidChabane/musthold/discpower/internal/domain/differential"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/footprint"
	"github.com/RachidChabane/musthold/discpower/internal/domain/power"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
	"github.com/RachidChabane/musthold/format"
)

// Namespaces of the memoized facts.
const (
	namespaceKillVector   = "kill-vector-2"
	namespaceDifferential = "differential"
	namespaceWitness      = "witness"
	keySeparator          = "\x00"
	siteSeparator         = "::"
)

// Reasons a differential search is skipped.
const (
	SkipNoProber       = "no_prober"
	SkipNoOperation    = "no_bound_operation"
	SkipDoesNotBuild   = "mutant_does_not_build"
	skipNoSignature    = "no_signature"
	unselectedOutcome  = format.OutcomeSurvived
	timedOutOutcome    = format.OutcomeError
	collectionOutcome  = format.OutcomeError
	failedTestOutcome  = format.OutcomeKilled
	passedTestOutcome  = format.OutcomeSurvived
	unknownTestOutcome = format.OutcomeError
)

// Operations resolves the operation names of the spec to the targets the
// ecosystem binds them to.
type Operations map[string]string

// Names lists the bound operation names, sorted.
func (o Operations) Names() []string {
	names := make([]string, 0, len(o))
	for name := range o {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Targets lists the distinct bound targets, sorted.
func (o Operations) Targets() []string {
	var targets []string
	for _, target := range o {
		if !slices.Contains(targets, target) {
			targets = append(targets, target)
		}
	}
	sort.Strings(targets)
	return targets
}

// Bound reports whether the operation is bound.
func (o Operations) Bound(operation string) bool {
	_, bound := o[operation]
	return bound
}

func (o Operations) resolve(calls []spec.Call) []ports.ProbeCall {
	resolved := make([]ports.ProbeCall, len(calls))
	for index, call := range calls {
		resolved[index] = ports.ProbeCall{Target: o[call.Operation], Args: call.Args}
	}
	return resolved
}

// OwnedExample is an example with the criterion that states it.
type OwnedExample struct {
	Owner   spec.Ref
	Example spec.Example
}

// MutantSpec is a mutant to execute.
type MutantSpec struct {
	ID    string
	Edits mutation.EditSet
	// FullSuite runs every test: sentinels and mutants of module-level units.
	FullSuite bool
	// Sentinel mutants skip the examples.
	Sentinel bool
}

// Execution is the memoized result of running one mutant: whether it built, what
// every test did against it and which criteria's examples it fails.
type Execution struct {
	Built       bool                      `json:"built"`
	Diagnostics string                    `json:"diagnostics,omitempty"`
	Outcomes    map[string]format.Outcome `json:"outcomes,omitempty"`
	Failed      []spec.Ref                `json:"failedExamples,omitempty"`
	// FailedAt names each failed example by its criterion and its index.
	FailedAt []power.ExampleFailure `json:"exampleFailures,omitempty"`
}

// Facts converts the execution into the facts the decision rules read.
func (e Execution) Facts(id string, origin power.Origin, slot power.Slot, edits mutation.EditSet) power.Facts {
	outcomes := make(map[ecosystem.TestID]format.Outcome, len(e.Outcomes))
	for test, outcome := range e.Outcomes {
		outcomes[ecosystem.TestID(test)] = outcome
	}
	var sites []string
	for _, edit := range edits.Canonical().Edits {
		sites = append(sites, edit.File+siteSeparator+edit.Qualname)
	}
	return power.Facts{ID: id, Origin: origin, Slot: slot, Sites: sites, Outcomes: outcomes, FailedExamples: e.Failed, ExampleFailures: e.FailedAt}
}

// Runner executes mutants of one project against its suite.
type Runner struct {
	Executor  mutantrun.Executor
	Reference mutantrun.Reference
	// Tests are the tests bound to every criterion: the suite the kill vector
	// is read from.
	Tests    []ecosystem.BoundTest
	Coverage coverage.Map
	// Prober is nil for an ecosystem that cannot probe calls.
	Prober     ports.Prober
	Operations Operations
	Examples   []OwnedExample
	Memo       ports.ExecutionMemo
	// SuiteKey identifies the code, the tests, the examples and the bindings the
	// facts depend on.
	SuiteKey string
	// Witnesses lays out and runs witness sets; its host is nil for an ecosystem
	// that cannot run them.
	Witnesses Witnesses
	// Compiler is nil for an ecosystem with no comparable compiled form.
	Compiler ports.FormCompiler
	// Recorder records the coverage of the witnesses; nil for an ecosystem that
	// records none.
	Recorder ports.CoverageRecorder
	// SpecTexts are the statements of the criteria, whose literals seed the
	// differential search at full resolution.
	SpecTexts []string
}

func (r Runner) memoKey(parts ...string) string {
	return format.SHA256Hex([]byte(joinParts(append([]string{r.SuiteKey}, parts...))))
}

func joinParts(parts []string) string {
	joined := ""
	for index, part := range parts {
		if index > 0 {
			joined += keySeparator
		}
		joined += part
	}
	return joined
}

func (r Runner) recall(key string, into any) bool {
	if r.Memo == nil {
		return false
	}
	raw, found, err := r.Memo.Get(key)
	if err != nil || !found {
		return false
	}
	return json.Unmarshal(raw, into) == nil
}

func (r Runner) remember(key string, value any) error {
	if r.Memo == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode a memoized fact: %w", err)
	}
	return r.Memo.Put(key, raw)
}

// Observe runs the mutant: the tests that can see the edit, each kill rerun once,
// then every example of every criterion. The result is memoized.
func (r Runner) Observe(ctx context.Context, mutant MutantSpec) (Execution, error) {
	key := r.memoKey(namespaceKillVector, mutant.ID)
	var cached Execution
	if r.recall(key, &cached) {
		return cached, nil
	}
	selected, full := r.selection(mutant)
	var execution Execution
	status, err := r.Executor.Mutate(ctx, r.Reference, mutant.Edits, func(ctx context.Context, workspace ecosystem.Workspace) error {
		outcomes, err := r.killVector(ctx, workspace, selected, full)
		if err != nil {
			return err
		}
		execution.Outcomes = outcomes
		if mutant.Sentinel {
			return nil
		}
		execution.Failed, execution.FailedAt, err = r.failedExamples(ctx, workspace)
		return err
	})
	if err != nil {
		return Execution{}, err
	}
	execution.Built, execution.Diagnostics = status.Built, status.Diagnostics
	return execution, r.remember(key, execution)
}

// selection chooses the tests a mutant is run against: every test for a
// sentinel, a module-level edit or an unknown coverage, otherwise the tests that
// executed one of the edited definitions.
func (r Runner) selection(mutant MutantSpec) (selected []ecosystem.BoundTest, full bool) {
	if mutant.FullSuite || len(r.Coverage) == 0 {
		return r.Tests, true
	}
	var units []footprint.Ref
	for _, edit := range mutant.Edits.Edits {
		if edit.Kind() != mutation.TargetDefinition {
			return r.Tests, true
		}
		units = append(units, footprint.Ref{File: edit.File, Qualname: edit.Qualname})
	}
	ids := make([]ecosystem.TestID, len(r.Tests))
	for index, test := range r.Tests {
		ids[index] = test.ID
	}
	covering := r.Coverage.Covering(ids, units)
	if len(covering) == 0 {
		return r.Tests, true
	}
	for _, test := range r.Tests {
		if slices.Contains(covering, test.ID) {
			selected = append(selected, test)
		}
	}
	return selected, false
}

// killVector runs the selection and rereads every kill once: a test that failed
// the first time and passed the second one counts as survived.
func (r Runner) killVector(ctx context.Context, workspace ecosystem.Workspace, selected []ecosystem.BoundTest, full bool) (map[string]format.Outcome, error) {
	first, err := r.Executor.Run(ctx, workspace, r.Reference.Toolchain, selected, full)
	if err != nil {
		return nil, err
	}
	outcomes := r.classify(first, selected, full)
	if !hasKill(outcomes) {
		return outcomes, nil
	}
	again, err := r.Executor.Run(ctx, workspace, r.Reference.Toolchain, selected, full)
	if err != nil {
		return nil, err
	}
	for test, outcome := range r.classify(again, selected, full) {
		if outcomes[test] == format.OutcomeKilled && outcome != format.OutcomeKilled {
			outcomes[test] = format.OutcomeSurvived
		}
	}
	return outcomes, nil
}

func hasKill(outcomes map[string]format.Outcome) bool {
	for _, outcome := range outcomes {
		if outcome == format.OutcomeKilled {
			return true
		}
	}
	return false
}

func (r Runner) classify(run ecosystem.RunResult, selected []ecosystem.BoundTest, full bool) map[string]format.Outcome {
	inSelection := map[ecosystem.TestID]bool{}
	for _, test := range selected {
		inSelection[test.ID] = true
	}
	outcomes := make(map[string]format.Outcome, len(r.Tests))
	for _, test := range r.Tests {
		outcomes[string(test.ID)] = outcomeOf(run, test.ID, full || inSelection[test.ID])
	}
	return outcomes
}

func outcomeOf(run ecosystem.RunResult, test ecosystem.TestID, ran bool) format.Outcome {
	switch {
	case !ran:
		return unselectedOutcome
	case run.TimedOut:
		return timedOutOutcome
	case run.CollectionFailed:
		return collectionOutcome
	}
	switch run.Outcomes[test] {
	case ecosystem.TestFailed:
		return failedTestOutcome
	case ecosystem.TestPassed:
		return passedTestOutcome
	}
	return unknownTestOutcome
}

// failedExamples runs every bound example in the workspace and lists the
// criteria one of whose examples did not get what it expects, and each such
// example, sorted by criterion and index.
func (r Runner) failedExamples(ctx context.Context, workspace ecosystem.Workspace) ([]spec.Ref, []power.ExampleFailure, error) {
	observed, runnable, err := r.observeExamples(ctx, workspace)
	if err != nil {
		return nil, nil, err
	}
	var failed []spec.Ref
	var failures []power.ExampleFailure
	for index, example := range runnable {
		if example.Example.Met(observed[index]) {
			continue
		}
		failures = append(failures, power.ExampleFailure{Owner: example.Owner, Index: example.Example.Index})
		if !slices.Contains(failed, example.Owner) {
			failed = append(failed, example.Owner)
		}
	}
	slices.SortFunc(failed, spec.Ref.Compare)
	slices.SortFunc(failures, power.ExampleFailure.Compare)
	return failed, failures, nil
}

// observeExamples runs the examples whose operation is bound.
func (r Runner) observeExamples(ctx context.Context, workspace ecosystem.Workspace) ([]spec.Observation, []OwnedExample, error) {
	if r.Prober == nil {
		return nil, nil, nil
	}
	var runnable []OwnedExample
	var calls []spec.Call
	for _, example := range r.Examples {
		if r.Operations.Bound(example.Example.Call.Operation) {
			runnable = append(runnable, example)
			calls = append(calls, example.Example.Call)
		}
	}
	if len(calls) == 0 {
		return nil, nil, nil
	}
	observed, err := r.Prober.Invoke(ctx, workspace, r.Reference.Toolchain, r.Operations.resolve(calls))
	if err != nil {
		return nil, nil, fmt.Errorf("run the examples: %w", err)
	}
	if len(observed) != len(calls) {
		return nil, nil, fmt.Errorf("run the examples: %d observations for %d calls", len(observed), len(calls))
	}
	return observed, runnable, nil
}

// ExampleResult is the outcome of an example on the reference code.
type ExampleResult struct {
	Owner  spec.Ref
	Index  int
	Passed bool
}

// ReferenceExamples runs the bound examples on the reference and reports each
// result; examples whose operation is unbound are not run.
func (r Runner) ReferenceExamples(ctx context.Context) ([]ExampleResult, error) {
	observed, runnable, err := r.observeExamples(ctx, r.Reference.Workspace)
	if err != nil {
		return nil, err
	}
	results := make([]ExampleResult, len(runnable))
	for index, example := range runnable {
		results[index] = ExampleResult{Owner: example.Owner, Index: example.Example.Index, Passed: example.Example.Met(observed[index])}
	}
	return results, nil
}

// Draws are the inputs of one differential search with what the reference did
// on them.
type Draws struct {
	Calls      []spec.Call
	Reference  []spec.Observation
	Digest     string
	Signatures []differential.Signature
}

// PrepareDraws draws the inputs of the differential search over the bound
// operations, biased by the constants of the given sources and the examples, and
// runs them on the reference. It returns nil and the reason when no search can
// run.
func (r Runner) PrepareDraws(ctx context.Context, sources []string, seed uint64) (*Draws, string, error) {
	if r.Prober == nil {
		return nil, SkipNoProber, nil
	}
	if len(r.Operations) == 0 {
		return nil, SkipNoOperation, nil
	}
	signatures, err := r.signatures(ctx)
	if err != nil {
		return nil, "", err
	}
	if len(signatures) == 0 {
		return nil, skipNoSignature, nil
	}
	exampleCalls := make([]spec.Call, len(r.Examples))
	for index, example := range r.Examples {
		exampleCalls[index] = example.Example.Call
	}
	pool := differential.NewPool(sources, exampleCalls).WithSpec(r.SpecTexts)
	var calls []spec.Call
	for _, signature := range signatures {
		calls = append(calls, pool.Draw(signature, seed, differential.DefaultDraws)...)
	}
	if len(calls) == 0 {
		return nil, skipNoSignature, nil
	}
	observed, err := r.Prober.Invoke(ctx, r.Reference.Workspace, r.Reference.Toolchain, r.Operations.resolve(calls))
	if err != nil {
		return nil, "", fmt.Errorf("run the differential inputs on the reference: %w", err)
	}
	canonical, err := format.Canonicalize(calls)
	if err != nil {
		return nil, "", err
	}
	return &Draws{Calls: calls, Reference: observed, Digest: format.SHA256Hex(canonical), Signatures: signatures}, "", nil
}

func (r Runner) signatures(ctx context.Context) ([]differential.Signature, error) {
	found, err := r.Prober.Signatures(ctx, r.Reference.Workspace, r.Reference.Toolchain, r.Operations.Targets())
	if err != nil {
		return nil, fmt.Errorf("read the signatures of the bound operations: %w", err)
	}
	byTarget := map[string][]differential.Param{}
	for _, signature := range found {
		byTarget[signature.Target] = signature.Params
	}
	var signatures []differential.Signature
	for _, name := range r.Operations.Names() {
		if params, known := byTarget[r.Operations[name]]; known {
			signatures = append(signatures, differential.Signature{Operation: name, Params: params})
		}
	}
	return signatures, nil
}

// Differential runs the drawn inputs on the mutant and compares them with what
// the reference did.
func (r Runner) Differential(ctx context.Context, mutant MutantSpec, draws *Draws, skipReason string) (power.Differential, error) {
	if draws == nil {
		return differential.SkippedBecause(skipReason), nil
	}
	key := r.memoKey(namespaceDifferential, mutant.ID, draws.Digest)
	var cached power.Differential
	if r.recall(key, &cached) {
		return cached, nil
	}
	var observed []spec.Observation
	status, err := r.Executor.Mutate(ctx, r.Reference, mutant.Edits, func(ctx context.Context, workspace ecosystem.Workspace) error {
		var invokeErr error
		observed, invokeErr = r.Prober.Invoke(ctx, workspace, r.Reference.Toolchain, r.Operations.resolve(draws.Calls))
		return invokeErr
	})
	if err != nil {
		return power.Differential{}, fmt.Errorf("run the differential inputs on a mutant: %w", err)
	}
	result := differential.SkippedBecause(SkipDoesNotBuild)
	if status.Built {
		result = differential.Compare(draws.Calls, draws.Reference, observed)
	}
	return result, r.remember(key, result)
}

// witnessRecord is the evidence a confirmed witness is digested from.
type witnessRecord struct {
	Mutant    string           `json:"mutant"`
	Call      spec.Call        `json:"call"`
	Expected  spec.Observation `json:"expected"`
	Reference spec.Observation `json:"reference"`
	Observed  spec.Observation `json:"mutant_observed"`
}

// Confirmation is the result of confirming a witness (F4).
type Confirmation struct {
	Check  power.WitnessCheck `json:"check"`
	Digest string             `json:"digest,omitempty"`
}

// Confirm executes a judge's witness on the reference and on the mutant: it is
// confirmed when the reference returns what the judge expects and the mutant
// does not, refused when execution says otherwise, and absent when it cannot be
// run.
func (r Runner) Confirm(ctx context.Context, mutant MutantSpec, call spec.Call, expected spec.Expectation) (Confirmation, error) {
	if r.Prober == nil || !r.Operations.Bound(call.Operation) {
		return Confirmation{Check: power.WitnessAbsent}, nil
	}
	encodedCall, err := format.Canonicalize(struct {
		Call     spec.Call        `json:"call"`
		Expected spec.Observation `json:"expected"`
	}{call, expected})
	if err != nil {
		return Confirmation{}, err
	}
	key := r.memoKey(namespaceWitness, mutant.ID, format.SHA256Hex(encodedCall))
	var cached Confirmation
	if r.recall(key, &cached) {
		return cached, nil
	}
	confirmation, err := r.confirm(ctx, mutant, call, expected)
	if err != nil {
		return Confirmation{}, err
	}
	return confirmation, r.remember(key, confirmation)
}

func (r Runner) confirm(ctx context.Context, mutant MutantSpec, call spec.Call, expected spec.Expectation) (Confirmation, error) {
	probe := r.Operations.resolve([]spec.Call{call})
	onReference, err := r.Prober.Invoke(ctx, r.Reference.Workspace, r.Reference.Toolchain, probe)
	if err != nil || len(onReference) != 1 {
		return Confirmation{Check: power.WitnessAbsent}, nil
	}
	var onMutant []spec.Observation
	status, err := r.Executor.Mutate(ctx, r.Reference, mutant.Edits, func(ctx context.Context, workspace ecosystem.Workspace) error {
		var invokeErr error
		onMutant, invokeErr = r.Prober.Invoke(ctx, workspace, r.Reference.Toolchain, probe)
		return invokeErr
	})
	if err != nil || !status.Built || len(onMutant) != 1 {
		return Confirmation{Check: power.WitnessAbsent}, nil
	}
	if !onReference[0].Same(expected) || onMutant[0].Same(expected) {
		return Confirmation{Check: power.WitnessRefused}, nil
	}
	canonical, err := format.Canonicalize(witnessRecord{Mutant: mutant.ID, Call: call, Expected: expected, Reference: onReference[0], Observed: onMutant[0]})
	if err != nil {
		return Confirmation{}, err
	}
	return Confirmation{Check: power.WitnessConfirmed, Digest: format.SHA256Hex(canonical)}, nil
}
