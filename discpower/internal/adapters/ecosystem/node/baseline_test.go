package node_test

import (
	"context"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/ecosystem/node"
	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

// scriptedTypecheck answers every command with the given tsc output and exit
// status.
type scriptedTypecheck struct {
	stdout string
	exit   int
}

func (s scriptedTypecheck) Run(context.Context, ports.Command) (ports.CommandResult, error) {
	return ports.CommandResult{ExitCode: s.exit, Stdout: []byte(s.stdout)}, nil
}

const (
	deprecatedBaseURL = "tsconfig.json(4,5): error TS5101: Option 'baseUrl' is deprecated and will stop functioning in TypeScript 7.0."
	assignmentError   = "src/rates.ts(3,1): error TS2322: Type 'string' is not assignable to type 'number'."
	movedAssignment   = "src/rates.ts(9,1): error TS2322: Type 'string' is not assignable to type 'number'."
	missingNameError  = "src/rates.ts(5,10): error TS2304: Cannot find name 'rate'."
	failedTypecheck   = 2
)

func TestBuildCheckBaseline(t *testing.T) {
	ctx := context.Background()
	toolchain := ecosystem.Toolchain{Command: []string{"npx", "--no-install"}}
	strategy := func(process ports.ProcessRunner) ports.BuildCheck {
		return node.NewVitest(dependencies(process, nil, found)).BuildCheck()
	}
	baselineOf := func(t *testing.T, workspace ecosystem.Workspace, stdout string) ecosystem.BuildBaseline {
		t.Helper()
		baseline, err := strategy(scriptedTypecheck{stdout: stdout, exit: failedTypecheck}).Baseline(ctx, workspace, toolchain)
		if err != nil {
			t.Fatal(err)
		}
		return baseline
	}
	checkMutant := func(t *testing.T, baseline ecosystem.BuildBaseline, stdout string) ecosystem.BuildResult {
		t.Helper()
		result, err := strategy(scriptedTypecheck{stdout: stdout, exit: failedTypecheck}).Check(ctx, lockfileWorkspace(t, "tsconfig.json"), toolchain, []string{"src/rates.ts"}, baseline)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}

	t.Run("a mutant whose type check reports what the reference reports passes the build check [verifies SP-2440~1]", func(t *testing.T) {
		baseline := baselineOf(t, lockfileWorkspace(t, "tsconfig.json"), deprecatedBaseURL)
		if result := checkMutant(t, baseline, deprecatedBaseURL); !result.OK {
			t.Fatalf("%+v", result)
		}
	})

	t.Run("a diagnostic that moved to another line of the mutated file is the reference's own [verifies SP-2440~1]", func(t *testing.T) {
		baseline := baselineOf(t, lockfileWorkspace(t, "tsconfig.json"), assignmentError)
		if result := checkMutant(t, baseline, movedAssignment); !result.OK {
			t.Fatalf("%+v", result)
		}
	})

	t.Run("a mutant that adds a diagnostic to the reference's fails the build check with the added one [verifies SP-2440~1]", func(t *testing.T) {
		baseline := baselineOf(t, lockfileWorkspace(t, "tsconfig.json"), deprecatedBaseURL)
		result := checkMutant(t, baseline, deprecatedBaseURL+"\n"+missingNameError)
		if result.OK || !strings.Contains(result.Diagnostics, "TS2304") || strings.Contains(result.Diagnostics, "TS5101") {
			t.Fatalf("%+v", result)
		}
	})

	t.Run("a diagnostic the reference reports once and a mutant twice is one the mutant adds [verifies SP-2440~1]", func(t *testing.T) {
		baseline := baselineOf(t, lockfileWorkspace(t, "tsconfig.json"), assignmentError)
		if result := checkMutant(t, baseline, assignmentError+"\n"+movedAssignment); result.OK {
			t.Fatalf("%+v", result)
		}
	})

	t.Run("the directory of a workspace is not part of a diagnostic [verifies SP-2440~1]", func(t *testing.T) {
		reference, mutant := lockfileWorkspace(t, "tsconfig.json"), lockfileWorkspace(t, "tsconfig.json")
		baseline := baselineOf(t, reference, reference.Dir+"/"+assignmentError)
		result, err := strategy(scriptedTypecheck{stdout: mutant.Dir + "/" + assignmentError, exit: failedTypecheck}).Check(ctx, mutant, toolchain, []string{"src/rates.ts"}, baseline)
		if err != nil || !result.OK {
			t.Fatalf("%+v %v", result, err)
		}
	})

	t.Run("a reference that passes the type check leaves every diagnostic of a mutant to fail it [verifies SP-2440~1]", func(t *testing.T) {
		passing, err := strategy(scriptedTypecheck{}).Baseline(ctx, lockfileWorkspace(t, "tsconfig.json"), toolchain)
		if err != nil {
			t.Fatal(err)
		}
		if result := checkMutant(t, passing, assignmentError); result.OK {
			t.Fatalf("%+v", result)
		}
	})

	t.Run("a failing type check without a diagnostic is a failed build [verifies SP-2440~1]", func(t *testing.T) {
		if result := checkMutant(t, ecosystem.BuildBaseline{}, ""); result.OK {
			t.Fatalf("%+v", result)
		}
	})

	t.Run("a project without a tsconfig has no baseline and runs no type check [verifies SP-2440~1]", func(t *testing.T) {
		process := &recorder{}
		baseline, err := strategy(process).Baseline(ctx, lockfileWorkspace(t), toolchain)
		if err != nil || len(process.commands) != 0 || len(baseline.Introduced([]string{assignmentError})) != 1 {
			t.Fatalf("%v %v", err, process.commands)
		}
	})
}
