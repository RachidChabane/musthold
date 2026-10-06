// Package ecosystemcontract is the EcosystemContract suite of ADR 0002: six
// cases every ecosystem Strategy must pass against its fixture project, run
// through the real process, workspace and JUnit adapters and the real toolchain.
package ecosystemcontract

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/junitxml"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/process"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/workspace"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/mutantrun"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/testkit/bindings"
	"github.com/RachidChabane/spec-proof/format"
)

// Adapters are the real adapters a Strategy is built over.
type Adapters struct {
	Process ports.ProcessRunner
	Reader  ports.ResultsReader
}

// Subject describes one strategy and its fixture project.
type Subject struct {
	Name string
	// Build constructs the strategy over the adapters.
	Build func(Adapters) ports.Ecosystem
	// FixtureDir is the fixture project's directory.
	FixtureDir string
	// Sources are the project's source files, relative POSIX.
	Sources []string
	// SourceRoots are the directories edits may touch.
	SourceRoots []string
	// BuildOutputs, when not empty, is a glob of build outputs the reference
	// workspace must hold once it is built (the stale-bytecode precondition).
	BuildOutputs string

	// Bindings are the tests bound to each criterion of the fixture.
	Bindings bindings.Bindings
	// KillingRef is the criterion whose tests the Killing mutant fails.
	KillingRef string
	// EquivalentRef is the criterion the Equivalent mutant is run against.
	EquivalentRef string
	// UnaffectedRef is a criterion whose tests no mutant below fails.
	UnaffectedRef string
	// DisplayRef is a criterion bound through display-named or parametrized tests.
	DisplayRef string
	// UnboundTests are tests of the fixture bound to no criterion.
	UnboundTests []ecosystem.TestID

	// Killing is a mutant a test of KillingRef fails.
	Killing mutation.EditSet
	// Equivalent changes the code without changing its behaviour.
	Equivalent mutation.EditSet
	// Broken does not compile or parse.
	Broken mutation.EditSet
	// DisplayKilling is a mutant a test of DisplayRef fails.
	DisplayKilling mutation.EditSet
}

// session is the shared state of one subject's cases: the built reference.
type session struct {
	subject   Subject
	executor  mutantrun.Executor
	reference mutantrun.Reference
	full      ecosystem.RunResult
}

// Run executes the six contract cases for the subject. The cases run in order,
// never in parallel, because they spawn the real toolchain.
func Run(t *testing.T, subject Subject) {
	t.Helper()
	s := open(t, subject)

	t.Run("a known-killing mutant is killed [verifies SP-76~1]", s.knownKillingIsKilled)
	t.Run("a known-equivalent mutant survives [verifies SP-77~1]", s.knownEquivalentSurvives)
	t.Run("a syntax-breaking mutant gives error [verifies SP-78~1]", s.syntaxBreakingGivesError)
	t.Run("selection runs only the bound tests [verifies SP-79~1]", s.selectionRunsOnlyBoundTests)
	t.Run("a mutant cloned from an already-built reference is still killed [verifies SP-80~1]", s.staleBuildOutputsDoNotHideAKill)
	t.Run("a display-named or parametrized bound test is selected and attributed [verifies SP-81~1]", s.displayNamedTestsAreAttributed)
}

func open(t *testing.T, subject Subject) *session {
	t.Helper()
	manager, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })

	strategy := subject.Build(Adapters{Process: process.New(), Reader: junitxml.NewReader()})
	executor := mutantrun.Executor{
		Ecosystem:  strategy,
		Workspaces: manager,
		Policy:     mutation.Policy{SourceRoots: subject.SourceRoots, Layout: strategy.Layout()},
	}
	ctx := context.Background()
	reference, err := executor.PrepareReference(ctx, subject.FixtureDir)
	if err != nil {
		t.Fatalf("prepare the reference: %v", err)
	}
	build, err := executor.BuildReference(ctx, reference, subject.Sources)
	if err != nil || !build.OK {
		t.Fatalf("build the reference: %+v %v", build, err)
	}
	full, err := executor.RunReference(ctx, reference, subject.Bindings.All(), true)
	if err != nil {
		t.Fatalf("run the reference suite: %v", err)
	}
	s := &session{subject: subject, executor: executor, reference: reference, full: full}
	s.requireReferencePasses(t)
	return s
}

// requireReferencePasses is the correctCodePasses evidence: every bound test
// and every unbound test passes on the unmutated reference.
func (s *session) requireReferencePasses(t *testing.T) {
	t.Helper()
	for _, test := range append(boundIDs(s.subject.Bindings.All()), s.subject.UnboundTests...) {
		if s.full.Outcomes[test] != ecosystem.TestPassed {
			t.Fatalf("the unmutated reference does not pass %s: %v", test, s.full.Outcomes[test])
		}
	}
}

func (s *session) execute(t *testing.T, edits mutation.EditSet, ref string) mutantrun.Result {
	t.Helper()
	result, err := s.executor.Execute(context.Background(), s.reference, mutantrun.Request{Edits: edits, Bound: s.subject.Bindings[ref]})
	if err != nil {
		t.Fatalf("execute the mutant against %s: %v", ref, err)
	}
	return result
}

func (s *session) knownKillingIsKilled(t *testing.T) {
	if got := s.execute(t, s.subject.Killing, s.subject.KillingRef); got.Outcome != format.OutcomeKilled {
		t.Fatalf("outcome = %s (%s)", got.Outcome, got.Diagnostics)
	}
}

func (s *session) knownEquivalentSurvives(t *testing.T) {
	if got := s.execute(t, s.subject.Equivalent, s.subject.EquivalentRef); got.Outcome != format.OutcomeSurvived {
		t.Fatalf("outcome = %s (%s)", got.Outcome, got.Diagnostics)
	}
}

func (s *session) syntaxBreakingGivesError(t *testing.T) {
	got := s.execute(t, s.subject.Broken, s.subject.KillingRef)
	if got.Outcome != format.OutcomeError || got.Diagnostics == "" {
		t.Fatalf("outcome = %s, diagnostics = %q", got.Outcome, got.Diagnostics)
	}
}

func (s *session) selectionRunsOnlyBoundTests(t *testing.T) {
	got := s.execute(t, s.subject.Killing, s.subject.UnaffectedRef)
	if got.Outcome != format.OutcomeSurvived {
		t.Fatalf("outcome = %s (%s)", got.Outcome, got.Diagnostics)
	}
	bound := boundIDs(s.subject.Bindings[s.subject.UnaffectedRef])
	for id := range got.Run.Outcomes {
		if !isBoundOrParent(id, bound) {
			t.Errorf("an unbound test ran: %s", id)
		}
	}
	for _, id := range s.subject.UnboundTests {
		if _, ran := got.Run.Outcomes[id]; ran {
			t.Errorf("the unbound test %s ran", id)
		}
		if _, known := s.full.Outcomes[id]; !known {
			t.Errorf("the unbound test %s is not in the full suite", id)
		}
	}
	if len(got.Run.Outcomes) >= len(s.full.Outcomes) {
		t.Fatalf("the selection ran %d tests, the full suite has %d", len(got.Run.Outcomes), len(s.full.Outcomes))
	}
}

func (s *session) staleBuildOutputsDoNotHideAKill(t *testing.T) {
	if s.subject.BuildOutputs != "" {
		matches, err := filepath.Glob(filepath.Join(s.reference.Workspace.Dir, filepath.FromSlash(s.subject.BuildOutputs)))
		if err != nil || len(matches) == 0 {
			t.Fatalf("the reference holds no build output matching %s", s.subject.BuildOutputs)
		}
	}
	for attempt := 0; attempt < 2; attempt++ {
		if got := s.execute(t, s.subject.Killing, s.subject.KillingRef); got.Outcome != format.OutcomeKilled {
			t.Fatalf("attempt %d: outcome = %s (%s)", attempt, got.Outcome, got.Diagnostics)
		}
	}
}

func (s *session) displayNamedTestsAreAttributed(t *testing.T) {
	killed := s.execute(t, s.subject.DisplayKilling, s.subject.DisplayRef)
	if killed.Outcome != format.OutcomeKilled {
		t.Fatalf("against %s: outcome = %s (%s)", s.subject.DisplayRef, killed.Outcome, killed.Diagnostics)
	}
	for _, id := range boundIDs(s.subject.Bindings[s.subject.DisplayRef]) {
		if outcome, ran := killed.Run.Outcomes[id]; !ran || (outcome != ecosystem.TestPassed && outcome != ecosystem.TestFailed) {
			t.Errorf("bound test %s has outcome %q in the run", id, outcome)
		}
	}
	if other := s.execute(t, s.subject.DisplayKilling, s.subject.UnaffectedRef); other.Outcome != format.OutcomeSurvived {
		t.Fatalf("against %s: outcome = %s (%s)", s.subject.UnaffectedRef, other.Outcome, other.Diagnostics)
	}
}

func boundIDs(tests []ecosystem.BoundTest) []ecosystem.TestID {
	ids := make([]ecosystem.TestID, 0, len(tests))
	for _, test := range tests {
		ids = append(ids, test.ID)
	}
	return ids
}

// isBoundOrParent reports whether id is a bound test or the parent of one: a
// Go parent test runs whenever one of its subtests does.
func isBoundOrParent(id ecosystem.TestID, bound []ecosystem.TestID) bool {
	for _, candidate := range bound {
		if candidate == id || strings.HasPrefix(string(candidate), string(id)+"/") {
			return true
		}
	}
	return false
}
