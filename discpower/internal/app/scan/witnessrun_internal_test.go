package scan

import (
	"context"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/mutantrun"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
)

const verifiesWitnessRunnerWiring = " [verifies SP-2273~1]"

// runnerStub is a suite runner a test tells apart from the ecosystem's.
type runnerStub struct{ name string }

func (runnerStub) Run(context.Context, ecosystem.Workspace, ecosystem.Toolchain, ecosystem.Selection) (ecosystem.RunResult, error) {
	return ecosystem.RunResult{}, nil
}

// suiteEcosystem is an ecosystem whose only behaviour a test reads is its
// suite runner.
type suiteEcosystem struct {
	ports.Ecosystem
	runner ports.SuiteRunner
}

func (e suiteEcosystem) Runner() ports.SuiteRunner { return e.runner }

func TestWitnessesRunWithTheirOwnRunner(t *testing.T) {
	suite, own := runnerStub{name: "suite"}, runnerStub{name: "witnesses"}
	reference := WitnessReference{Executor: mutantrun.Executor{Ecosystem: suiteEcosystem{runner: suite}}}
	t.Run("an ecosystem's runner of witnesses runs every witness run, memoized or not"+verifiesWitnessRunnerWiring, func(t *testing.T) {
		runner := ExecutionWitnesses{Runner: own}.For(reference, nil, "suite").(executionWitnessRunner)
		if runner.memoized.Executor.Ecosystem.Runner() != own || runner.fresh.Executor.Ecosystem.Runner() != own {
			t.Fatal("a witness run goes through the suite's runner")
		}
		if reference.Executor.Ecosystem.Runner() != suite {
			t.Fatal("the reference's own executor changed")
		}
	})
	t.Run("without one, witnesses run with the ecosystem's suite runner"+verifiesWitnessRunnerWiring, func(t *testing.T) {
		runner := ExecutionWitnesses{}.For(reference, nil, "suite").(executionWitnessRunner)
		if runner.memoized.Executor.Ecosystem.Runner() != suite {
			t.Fatal("a witness run does not go through the suite's runner")
		}
	})
}
