package execution_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/execution"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/witness"
)

const (
	verifiesSP1965 = " [verifies SP-1965~1]"

	shippingImports    = "from parcel.rates import shipping_fee"
	freeFromFifty      = "def free_from_fifty():\n    assert shipping_fee(50) == 0"
	freeFromFiftyName  = "free_from_fifty"
	brokenWitnessName  = "broken_witness"
	brokenWitnessCode  = "def broken_witness(:\n    pass"
	alwaysChargedSetID = "always-charged"
)

var alwaysCharged = edit(shippingFee, "def shipping_fee(total):\n    return SHIPPING_FEE")

func shippingWitnessSet(name, code string) witness.Set {
	return witness.Set{Criterion: criterionShipping, Imports: shippingImports, Witnesses: []witness.Witness{
		witness.New(criterionShipping, name, "waive the shipping fee", witness.KindExample, code),
	}}
}

func (w *world) withWitnesses() func(*execution.Runner) {
	return func(r *execution.Runner) {
		r.Witnesses = execution.Witnesses{Host: w.strategy.Witnesses(), Project: os.DirFS(filepath.Clean(fixtureDirectory))}
	}
}

// sequenceRunner answers the first runs with the real runner and the later ones by
// rewriting every outcome the given way.
type sequenceRunner struct {
	inner   ports.SuiteRunner
	real    int
	rewrite ecosystem.TestOutcome
	calls   int
}

func (s *sequenceRunner) Run(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, selection ecosystem.Selection) (ecosystem.RunResult, error) {
	s.calls++
	result, err := s.inner.Run(ctx, workspace, toolchain, selection)
	if s.calls <= s.real || err != nil {
		return result, err
	}
	rewritten := map[ecosystem.TestID]ecosystem.TestOutcome{}
	for test := range result.Outcomes {
		rewritten[test] = s.rewrite
	}
	return ecosystem.RunResult{Outcomes: rewritten}, nil
}

func TestWitnessExecution(t *testing.T) {
	requirePython(t)
	w := openWorld(t)
	ctx := context.Background()
	set := shippingWitnessSet(freeFromFiftyName, freeFromFifty)
	id := set.Witnesses[0].ID

	t.Run("a witness failure that the rerun repeats is a failure, run in the program's own workspace"+verifiesSP1965, func(t *testing.T) {
		suite := &countingRunner{inner: w.strategy.Runner()}
		runner := w.runner(w.withSuite(suite), w.withWitnesses())
		outcomes, err := runner.EditWitnesses(ctx, alwaysChargedSetID, alwaysCharged, []witness.Set{set})
		if err != nil {
			t.Fatal(err)
		}
		if outcomes[id] != witness.OutcomeFailed || len(suite.selections) != rerunCount {
			t.Errorf("outcome %q after %d runs", outcomes[id], len(suite.selections))
		}
		if _, err := os.Stat(filepath.Join(w.reference.Workspace.Dir, "tests")); err == nil {
			entries, _ := os.ReadDir(filepath.Join(w.reference.Workspace.Dir, "tests"))
			for _, entry := range entries {
				if filepath.Ext(entry.Name()) == ".py" && entry.Name() != "test_rates.py" && entry.Name() != "__init__.py" {
					t.Errorf("a witness file was written into the reference workspace: %s", entry.Name())
				}
			}
		}
	})

	t.Run("a failure the rerun does not repeat is flaky"+verifiesSP1965, func(t *testing.T) {
		suite := &sequenceRunner{inner: w.strategy.Runner(), real: singleRun, rewrite: ecosystem.TestPassed}
		runner := w.runner(w.withSuite(suite), w.withWitnesses())
		outcomes, err := runner.EditWitnesses(ctx, alwaysChargedSetID, alwaysCharged, []witness.Set{set})
		if err != nil {
			t.Fatal(err)
		}
		if outcomes[id] != witness.OutcomeFlaky {
			t.Errorf("outcome %q", outcomes[id])
		}
	})

	t.Run("outcomes are memoized per program and set"+verifiesSP1965, func(t *testing.T) {
		memo := newCountingMemo()
		suite := &countingRunner{inner: w.strategy.Runner()}
		runner := w.runner(w.withSuite(suite), w.withWitnesses(), withMemo(memo))
		first, err := runner.EditWitnesses(ctx, alwaysChargedSetID, alwaysCharged, []witness.Set{set})
		if err != nil {
			t.Fatal(err)
		}
		runs := len(suite.selections)
		again, err := runner.EditWitnesses(ctx, alwaysChargedSetID, alwaysCharged, []witness.Set{set})
		if err != nil || again[id] != first[id] || len(suite.selections) != runs {
			t.Errorf("outcome %q then %q, %d runs then %d, %v", first[id], again[id], runs, len(suite.selections), err)
		}
	})

	t.Run("a set that does not build errors on its own and leaves the other sets their outcomes"+verifiesSP1965, func(t *testing.T) {
		broken := shippingWitnessSet(brokenWitnessName, brokenWitnessCode)
		runner := w.runner(w.withWitnesses())
		outcomes, err := runner.EditWitnesses(ctx, alwaysChargedSetID, alwaysCharged, []witness.Set{set, broken})
		if err != nil {
			t.Fatal(err)
		}
		if outcomes[id] != witness.OutcomeFailed || outcomes[broken.Witnesses[0].ID] != witness.OutcomeErrored {
			t.Errorf("outcomes %v", outcomes)
		}
	})

	t.Run("a set runs three times on the reference"+verifiesSP1965, func(t *testing.T) {
		suite := &countingRunner{inner: w.strategy.Runner()}
		runner := w.runner(w.withSuite(suite), w.withWitnesses())
		runs, err := runner.ReferenceWitnesses(ctx, set)
		if err != nil {
			t.Fatal(err)
		}
		if len(runs[id]) != witness.ReferenceRuns || witness.ReadingOf(runs[id]) != witness.ReadingPasses || len(suite.selections) != witness.ReferenceRuns {
			t.Errorf("runs %v after %d suite runs", runs[id], len(suite.selections))
		}
	})
}
