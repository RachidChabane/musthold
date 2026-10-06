// Package mutantrun is the use case that executes one mutant: clone the built
// reference workspace, splice the edits, run the build check, run the selected
// tests and classify the evidence.
package mutantrun

import (
	"context"
	"fmt"
	"slices"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/matrix"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/format"
)

// Executor runs mutants of one project with one ecosystem strategy.
type Executor struct {
	Ecosystem  ports.Ecosystem
	Workspaces ports.Workspaces
	// Policy refuses edits outside the source roots and in test files.
	Policy mutation.Policy
}

// Reference is the reference workspace and the toolchain resolved in it.
type Reference struct {
	Workspace ecosystem.Workspace
	Toolchain ecosystem.Toolchain
}

// Request is one mutant execution.
type Request struct {
	Edits mutation.EditSet
	// Bound lists the tests bound to the criterion the mutant is run against.
	Bound []ecosystem.BoundTest
	// FullSuite runs every test instead of the bound ones, as module-level sites
	// and sentinels require; the outcome still reads the bound tests.
	FullSuite bool
}

// Result is the classified outcome of one mutant and what led to it.
type Result struct {
	Outcome     format.Outcome
	Run         ecosystem.RunResult
	Diagnostics string
}

// PrepareReference copies the project once and resolves the toolchain.
func (e Executor) PrepareReference(ctx context.Context, source string) (Reference, error) {
	workspace, err := e.Workspaces.Prepare(ctx, source, e.Ecosystem.Layout())
	if err != nil {
		return Reference{}, fmt.Errorf("prepare the reference workspace: %w", err)
	}
	toolchain, err := e.Ecosystem.Toolchain().Resolve(ctx, workspace)
	if err != nil {
		return Reference{}, fmt.Errorf("resolve the toolchain: %w", err)
	}
	return Reference{Workspace: workspace, Toolchain: toolchain}, nil
}

// BuildReference runs the build check over the whole reference workspace, so
// that build outputs exist before mutants are cloned from it.
func (e Executor) BuildReference(ctx context.Context, reference Reference, sources []string) (ecosystem.BuildResult, error) {
	return e.Ecosystem.BuildCheck().Check(ctx, reference.Workspace, reference.Toolchain, sources)
}

// RunReference runs the selection of the bound tests, or the whole suite, on
// the unmutated reference; it is the evidence for `correctCodePasses`.
func (e Executor) RunReference(ctx context.Context, reference Reference, bound []ecosystem.BoundTest, fullSuite bool) (ecosystem.RunResult, error) {
	selection, err := e.selection(reference.Workspace, bound, fullSuite)
	if err != nil {
		return ecosystem.RunResult{}, err
	}
	return e.Ecosystem.Runner().Run(ctx, reference.Workspace, reference.Toolchain, selection)
}

// Execute runs one mutant. A path the policy refuses is an error of the caller;
// a mutant that cannot be spliced, built or run to a verdict is the outcome
// `error`, not a Go error.
func (e Executor) Execute(ctx context.Context, reference Reference, request Request) (Result, error) {
	for _, file := range request.Edits.Files() {
		if err := e.Policy.Check(file); err != nil {
			return Result{}, err
		}
	}
	layout := e.Ecosystem.Layout()
	mutant, err := e.Workspaces.Clone(ctx, reference.Workspace, layout)
	if err != nil {
		return Result{}, fmt.Errorf("clone the reference workspace: %w", err)
	}
	defer func() { _ = e.Workspaces.Release(mutant) }()

	changed, diagnostics, err := e.apply(mutant, layout, request.Edits)
	if err != nil {
		return Result{}, err
	}
	if diagnostics != "" {
		return Result{Outcome: format.OutcomeError, Diagnostics: diagnostics}, nil
	}
	build, err := e.Ecosystem.BuildCheck().Check(ctx, mutant, reference.Toolchain, changed)
	if err != nil {
		return Result{}, err
	}
	if !build.OK {
		return Result{Outcome: format.OutcomeError, Diagnostics: build.Diagnostics}, nil
	}
	selection, err := e.selection(mutant, request.Bound, request.FullSuite)
	if err != nil {
		return Result{Outcome: format.OutcomeError, Diagnostics: err.Error()}, nil
	}
	run, err := e.Ecosystem.Runner().Run(ctx, mutant, reference.Toolchain, selection)
	if err != nil {
		return Result{}, err
	}
	return Result{Outcome: matrix.Classify(matrix.Evidence{BuildOK: true, Bound: boundIDs(request.Bound), Run: run}), Run: run}, nil
}

// apply splices every edit into the mutant workspace. A splice that fails is
// reported as diagnostics; a workspace I/O failure is an error.
func (e Executor) apply(mutant ecosystem.Workspace, layout ecosystem.Layout, edits mutation.EditSet) (changed []string, diagnostics string, err error) {
	splicer := mutation.Splicer{Locator: e.Ecosystem.Locator(), Assignments: e.Ecosystem.Assignments()}
	for _, file := range edits.Files() {
		source, err := e.Workspaces.ReadFile(mutant, file)
		if err != nil {
			return nil, "", fmt.Errorf("read %q: %w", file, err)
		}
		spliced, err := splicer.SpliceAll(file, source, edits.EditsOf(file))
		if err != nil {
			return nil, err.Error(), nil
		}
		if err := e.Workspaces.WriteFile(mutant, layout, file, spliced); err != nil {
			return nil, "", fmt.Errorf("write %q: %w", file, err)
		}
		changed = append(changed, file)
	}
	return changed, "", nil
}

func (e Executor) selection(workspace ecosystem.Workspace, bound []ecosystem.BoundTest, fullSuite bool) (ecosystem.Selection, error) {
	if fullSuite {
		return ecosystem.Selection{Tests: boundIDs(bound)}, nil
	}
	definitions := map[string][]ecosystem.Definition{}
	for _, test := range bound {
		if _, done := definitions[test.File]; done {
			continue
		}
		source, err := e.Workspaces.ReadFile(workspace, test.File)
		if err != nil {
			return ecosystem.Selection{}, fmt.Errorf("read bound test file %q: %w", test.File, err)
		}
		located, err := e.Ecosystem.Locator().Locate(test.File, source)
		if err != nil {
			return ecosystem.Selection{}, fmt.Errorf("locate bound test file %q: %w", test.File, err)
		}
		definitions[test.File] = located
	}
	return e.Ecosystem.Selector().Select(bound, definitions)
}

func boundIDs(bound []ecosystem.BoundTest) []ecosystem.TestID {
	ids := make([]ecosystem.TestID, 0, len(bound))
	for _, test := range bound {
		if !slices.Contains(ids, test.ID) {
			ids = append(ids, test.ID)
		}
	}
	return ids
}
