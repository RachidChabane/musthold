package harness_test

import (
	"testing"

	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
)

func TestCopilotAgentTypeForms(t *testing.T) {
	copilot := harness.Copilot(sample)

	t.Run("the plugin-prefixed type the stop hook reports resolves to the role of its agent [verifies SP-1870~1]", func(t *testing.T) {
		role, ok := copilot.RoleOf("spec-proof-sample:spec-proof-sample-decider")
		if !ok || role != "spec-proof-sample:decider" {
			t.Errorf("role = %q, %v", role, ok)
		}
		if _, ok := copilot.RoleOf("spec-proof-sample-decider"); !ok {
			t.Error("the plain file name does not resolve")
		}
	})

	t.Run("no other prefix and no unshipped agent resolves [verifies SP-1870~1]", func(t *testing.T) {
		for _, reported := range []string{
			"other:spec-proof-sample-decider", "spec-proof-sample/spec-proof-sample-decider", "spec-proof-sample@spec-proof-sample-decider",
			"spec-proof-sample:spec-proof-sample-boss", "spec-proof-sample::spec-proof-sample-decider", "x:spec-proof-sample:spec-proof-sample-decider",
			"spec-proof-sample:spec-proof-sample-decider:x", ":spec-proof-sample-decider",
		} {
			if role, ok := copilot.RoleOf(reported); ok {
				t.Errorf("%q was taken for %q", reported, role)
			}
		}
	})

	t.Run("a role is named by the types of its own agent only [verifies SP-1870~1]", func(t *testing.T) {
		const decider = "spec-proof-sample:decider"
		if !copilot.NamesRole(decider, "spec-proof-sample:spec-proof-sample-decider") || !copilot.NamesRole(decider, "spec-proof-sample-decider") {
			t.Error("a type of the decider does not name its role")
		}
		if copilot.NamesRole(decider, "spec-proof-sample:spec-proof-sample-judge") {
			t.Error("the judge's type names the decider's role")
		}
	})

	t.Run("only Copilot observes the selection of the tools of a run [verifies SP-1870~1]", func(t *testing.T) {
		for _, h := range harness.All(sample) {
			if h.Behaviours.ObservesSelection != (h.Flag == harness.FlagCopilot) {
				t.Errorf("%s: ObservesSelection = %v", h.Flag, h.Behaviours.ObservesSelection)
			}
		}
	})

	t.Run("a Claude transcript is read for its tool calls and a Copilot log for the agent's run [verifies SP-1870~1]", func(t *testing.T) {
		claudeLine := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"x","input":{}}]}}`
		run, err := harness.Claude(sample).ReadRun([]byte(claudeLine), "")
		if err != nil || len(run.Calls) != 1 {
			t.Errorf("claude: %+v, %v", run, err)
		}
		log := `{"type":"session.start","agentId":null,"data":{"copilotVersion":"9.9.9"}}`
		run, err = copilot.ReadRun([]byte(log), "agent")
		if err != nil || run.CLIVersion != "9.9.9" {
			t.Errorf("copilot: %+v, %v", run, err)
		}
	})
}
