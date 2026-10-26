package checks

import (
	"fmt"
	"strings"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/verdict"
)

const (
	pointerTurnContext = "turn_context"
	pointerCommand     = "command_execution"
)

// sandboxCheck judges the sandbox policy the executor's thread ran under, which
// its rollout records, against the isolation the harness strategy declares.
func sandboxCheck() Check {
	return newCheck(checkCodexSandbox,
		"The sandbox policy the executor's thread ran under, read from its rollout, agrees with the isolation the harness strategy declares: a read-only policy refused the write the executor attempted, and any other policy is accepted only where the declared isolation is unrestricted. An executor that left no rollout, or never tried the write under a read-only policy, leaves the question open.",
		func(found evidence.Evidence) verdict.Verdict {
			return judgeExecutorSandbox(found, codexStrategy().Isolation())
		})
}

// judgeExecutorSandbox concludes the sandbox check for a declared isolation.
func judgeExecutorSandbox(found evidence.Evidence, declared format.Isolation) verdict.Verdict {
	agentType := codexStrategy().AgentType(found.Session.Role)
	children := found.Rollouts.ChildrenOf(agentType)
	if len(children) == 0 {
		return verdict.Undecided("no rollout of an executor child was recorded, so its sandbox policy is unknown")
	}
	child, ok := childWithPolicy(children)
	if !ok {
		return verdict.Undecided("the executor's rollout records no sandbox policy", evidence.At(evidence.RolloutChildrenDir+"/"+children[0].File, 0, pointerTurnContext))
	}
	policy := child.Sandbox
	pointer := evidence.At(evidence.RolloutChildrenDir+"/"+child.File, policy.Line, pointerTurnContext)
	if !policy.ReadOnly() {
		return judgeUnenforcedSandbox(policy.Type, declared, pointer)
	}
	return judgeReadOnlySandbox(found, pointer)
}

func childWithPolicy(children []evidence.RolloutChild) (evidence.RolloutChild, bool) {
	for _, child := range children {
		if child.Sandbox.Recorded() {
			return child, true
		}
	}
	return evidence.RolloutChild{}, false
}

// judgeUnenforcedSandbox concludes for an executor that did not run read-only: the
// role file's sandbox is not applied, which the strategy accepts only where it
// declares the executor keeps its tools.
func judgeUnenforcedSandbox(policy string, declared format.Isolation, pointer verdict.Pointer) verdict.Verdict {
	if declared != format.IsolationUnrestricted {
		return verdict.Failed(fmt.Sprintf("the executor ran with the %s sandbox but the strategy declares isolation %s, which relies on a read-only executor", policy, declared), pointer)
	}
	return verdict.Passed(fmt.Sprintf("the role file's sandbox_mode was not applied and the executor ran with the %s sandbox, which the declared isolation (%s) already assumes", policy, declared), pointer)
}

// judgeReadOnlySandbox concludes for an executor that ran read-only: the write
// it attempted must not have landed.
func judgeReadOnlySandbox(found evidence.Evidence, policyPointer verdict.Pointer) verdict.Verdict {
	probe := found.SandboxProbe
	if probe.File == "" {
		return verdict.Undecided("the sandbox probe was not recorded", policyPointer)
	}
	pointer := evidence.At(evidence.SandboxProbeFile, 0, probe.File)
	if probe.Exists {
		return verdict.Failed("the executor wrote "+probe.File+" under a sandbox policy reported read-only", policyPointer, pointer)
	}
	for _, item := range codexStream(found).ItemsOf(evidence.CodexItemCommand) {
		if item.Event == evidence.CodexEventItemCompleted && strings.Contains(item.Command, probe.File) {
			pointers := []verdict.Pointer{policyPointer, pointer, evidence.StreamAt(item.Line, pointerCommand)}
			switch {
			case item.ExitCode != nil && *item.ExitCode != 0:
				return verdict.Passed(fmt.Sprintf("the sandbox is read-only, the write command exited %d and %s does not exist", *item.ExitCode, probe.File), pointers...)
			case item.Status == evidence.CodexStatusFailed || item.Status == evidence.CodexStatusDeclined:
				return verdict.Passed(fmt.Sprintf("the sandbox is read-only, the write command ended %s and %s does not exist", item.Status, probe.File), pointers...)
			}
		}
	}
	return verdict.Undecided("the sandbox is read-only but no attempted write is recorded, so it was not exercised", policyPointer, pointer)
}
