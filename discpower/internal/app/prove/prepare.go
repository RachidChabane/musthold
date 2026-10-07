package prove

import (
	"context"
	"fmt"
	"io/fs"
	"slices"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/execution"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/mutantrun"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/coverage"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/format"
)

// Module findings of a run, reported next to the outcomes.
const (
	FindingUnboundExampleOperation = "unbound_example_operation"
	FindingNoExampleExecutor       = "example_executor_unavailable"
	FindingCoverageUnavailable     = "coverage_unavailable"
	FindingDifferentialSkipped     = "differential_skipped"
)

// project is the state every criterion of a run shares: the reference workspace
// built and run once, the coverage and the examples observed on it.
type project struct {
	deps     Deps
	settings Settings

	criteria []spec.Criterion
	sources  []mutation.SourceFile
	tests    []ecosystem.BoundTest

	executor  mutantrun.Executor
	reference mutantrun.Reference
	runner    execution.Runner
	// referenceOutcomes is what every bound test did on the unmutated code.
	referenceOutcomes map[ecosystem.TestID]ecosystem.TestOutcome
	exampleResults    []execution.ExampleResult
	testFileDigests   map[string]string
	findings          []string
	// suiteKey identifies the code, tests, examples and bindings of the run.
	suiteKey string

	statics map[spec.Ref]*staticAnalysis
	// scopes lists, per operator mutant, the criteria whose footprint it lies in.
	scopes map[string][]spec.Ref
}

// prepare builds the reference, runs the suite on it and observes what the
// criteria need of it: coverage and the examples.
func prepare(ctx context.Context, deps Deps, settings Settings) (*project, error) {
	report, err := deps.Criteria.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("load the criteria: %w", err)
	}
	p := &project{deps: deps, settings: settings, criteria: withTests(report.Criteria), statics: map[spec.Ref]*staticAnalysis{}}
	layout := deps.Strategy.Layout()
	if p.sources, err = discoverSources(deps.Project, layout, deps.Engines); err != nil {
		return nil, err
	}
	p.tests = distinctTests(p.criteria)
	if p.testFileDigests, err = digestTestFiles(deps.Project, p.tests); err != nil {
		return nil, err
	}
	p.executor = mutantrun.Executor{Ecosystem: deps.Strategy, Workspaces: deps.Workspaces, Policy: mutation.Policy{SourceRoots: []string{rootDirectory}, Layout: layout}}
	if err := p.buildReference(ctx); err != nil {
		return nil, err
	}
	p.suiteKey, err = p.digestSuite()
	if err != nil {
		return nil, err
	}
	p.runner = p.newRunner(ctx)
	return p, p.observeReference(ctx)
}

func withTests(criteria []spec.Criterion) []spec.Criterion {
	kept := slices.DeleteFunc(slices.Clone(criteria), func(c spec.Criterion) bool { return len(c.Tests) == 0 })
	slices.SortFunc(kept, func(a, b spec.Criterion) int { return a.Ref.Compare(b.Ref) })
	return kept
}

func distinctTests(criteria []spec.Criterion) []ecosystem.BoundTest {
	var tests []ecosystem.BoundTest
	seen := map[ecosystem.TestID]bool{}
	for _, criterion := range criteria {
		for _, test := range criterion.Tests {
			if !seen[test.ID] {
				seen[test.ID] = true
				tests = append(tests, test)
			}
		}
	}
	return tests
}

func digestTestFiles(project fs.FS, tests []ecosystem.BoundTest) (map[string]string, error) {
	digests := map[string]string{}
	for _, test := range tests {
		if _, done := digests[test.File]; done {
			continue
		}
		content, err := fs.ReadFile(project, test.File)
		if err != nil {
			return nil, fmt.Errorf("read the test file %q: %w", test.File, err)
		}
		digests[test.File] = format.SHA256Hex(content)
	}
	return digests, nil
}

func (p *project) buildReference(ctx context.Context) error {
	reference, err := p.executor.PrepareReference(ctx, p.settings.Root)
	if err != nil {
		return err
	}
	p.reference = reference
	paths := make([]string, len(p.sources))
	for index, source := range p.sources {
		paths[index] = source.Path
	}
	build, err := p.executor.BuildReference(ctx, reference, paths)
	if err != nil {
		return fmt.Errorf("build the reference: %w", err)
	}
	if !build.OK {
		return fmt.Errorf("the reference does not build: %s", build.Diagnostics)
	}
	run, err := p.executor.RunReference(ctx, reference, p.tests, true)
	if err != nil {
		return fmt.Errorf("run the suite on the reference: %w", err)
	}
	p.referenceOutcomes = run.Outcomes
	if run.CollectionFailed || run.TimedOut {
		return fmt.Errorf("the suite does not run on the reference (collection failed: %t, timed out: %t)", run.CollectionFailed, run.TimedOut)
	}
	return nil
}

// digestSuite digests everything a kill vector or an example result depends on:
// the source files, the test files, the examples and the operation bindings.
func (p *project) digestSuite() (string, error) {
	var sources []format.File
	for _, source := range p.sources {
		sources = append(sources, format.File{Path: source.Path, Content: source.Content})
	}
	sourceDigest, err := format.FileSetDigest(sources)
	if err != nil {
		return "", err
	}
	testDigests := make([]string, 0, len(p.testFileDigests))
	for file, digest := range p.testFileDigests {
		testDigests = append(testDigests, file+"="+digest)
	}
	slices.Sort(testDigests)
	canonical, err := format.Canonicalize(struct {
		Ecosystem  ecosystem.Name    `json:"ecosystem"`
		Sources    string            `json:"sources"`
		Tests      []string          `json:"tests"`
		Examples   []spec.Example    `json:"examples"`
		Operations map[string]string `json:"operations"`
	}{p.settings.Ecosystem, sourceDigest, testDigests, p.allExamples(), p.settings.Operations})
	if err != nil {
		return "", err
	}
	return format.SHA256Hex(canonical), nil
}

func (p *project) allExamples() []spec.Example {
	var examples []spec.Example
	for _, criterion := range p.criteria {
		examples = append(examples, criterion.Examples...)
	}
	return examples
}

func (p *project) newRunner(ctx context.Context) execution.Runner {
	runner := execution.Runner{
		Executor: p.executor, Reference: p.reference, Tests: p.tests,
		Operations: p.settings.Operations, Memo: p.deps.Memo, SuiteKey: p.suiteKey,
	}
	if probing, ok := p.deps.Strategy.(ports.Probing); ok {
		runner.Prober = probing.Prober()
	}
	if covering, ok := p.deps.Strategy.(ports.Covering); ok {
		recorded, err := covering.Coverage().Record(ctx, p.reference.Workspace, p.reference.Toolchain, p.tests)
		if err != nil {
			p.findings = append(p.findings, FindingCoverageUnavailable)
		} else {
			runner.Coverage = coverageOf(recorded)
		}
	} else {
		p.findings = append(p.findings, FindingCoverageUnavailable)
	}
	for _, criterion := range p.criteria {
		for _, example := range criterion.Examples {
			runner.Examples = append(runner.Examples, execution.OwnedExample{Owner: criterion.Ref, Example: example})
			if !p.settings.Operations.Bound(example.Call.Operation) {
				p.findings = append(p.findings, fmt.Sprintf("%s: %s %s example %d", FindingUnboundExampleOperation, criterion.Ref, example.Call.Operation, example.Index))
			}
		}
	}
	if runner.Prober == nil && len(runner.Examples) > 0 {
		p.findings = append(p.findings, FindingNoExampleExecutor)
	}
	return runner
}

func coverageOf(recorded coverage.Map) coverage.Map {
	if len(recorded) == 0 {
		return nil
	}
	return recorded
}

func (p *project) observeReference(ctx context.Context) error {
	results, err := p.runner.ReferenceExamples(ctx)
	if err != nil {
		return err
	}
	p.exampleResults = results
	return nil
}

// byRef finds a criterion by its reference.
func (p *project) byRef(ref spec.Ref) (spec.Criterion, bool) {
	for _, criterion := range p.criteria {
		if criterion.Ref == ref {
			return criterion, true
		}
	}
	return spec.Criterion{}, false
}

// referencePasses reports whether every test bound to the criterion passed on the
// reference code.
func (p *project) referencePasses(criterion spec.Criterion) bool {
	for _, test := range criterion.TestIDs() {
		if p.referenceOutcomes[test] != ecosystem.TestPassed {
			return false
		}
	}
	return true
}

// exampleFailsOnReference reports whether an example of the criterion fails on the
// reference code.
func (p *project) exampleFailsOnReference(ref spec.Ref) bool {
	return slices.ContainsFunc(p.exampleResults, func(r execution.ExampleResult) bool { return r.Owner == ref && !r.Passed })
}

// release removes the reference workspace.
func (p *project) release() { _ = p.deps.Workspaces.Release(p.reference.Workspace) }
