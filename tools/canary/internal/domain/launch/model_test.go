package launch_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/launch"
)

func TestModelChoice(t *testing.T) {
	t.Run("each harness runs on its default model unless the operator names one, which replaces it in the command [verifies SP-1514~1]", func(t *testing.T) {
		for _, tc := range []struct{ flag, option, defaultModel string }{
			{"claude", "--model", launch.ClaudeModel},
			{"codex", "-m", launch.CodexModel},
			{"copilot", "--model", launch.CopilotModel},
		} {
			in := copilotInput()
			defaulted, err := launch.Build(tc.flag, in)
			if err != nil {
				t.Fatal(err)
			}
			if got := flagValue(defaulted.Argv, tc.option); !slices.Equal(got, []string{tc.defaultModel}) {
				t.Errorf("%s: model = %v, want %s", tc.flag, got, tc.defaultModel)
			}
			in.Model = "custom-model"
			chosen, _ := launch.Build(tc.flag, in)
			if got := flagValue(chosen.Argv, tc.option); !slices.Equal(got, []string{"custom-model"}) {
				t.Errorf("%s: model = %v, want custom-model", tc.flag, got)
			}
			if launch.Model(tc.flag, "") != tc.defaultModel || launch.Model(tc.flag, "custom-model") != "custom-model" || launch.DefaultModel(tc.flag) != tc.defaultModel {
				t.Errorf("%s: Model and DefaultModel disagree with the command", tc.flag)
			}
		}
	})

	t.Run("the default model of Copilot CLI is the CLI's own default [verifies SP-1514~1]", func(t *testing.T) {
		if launch.CopilotModel != "gpt-5.6-luna" {
			t.Errorf("CopilotModel = %s", launch.CopilotModel)
		}
	})
}

func TestCopilotPromptStartsTheExecutorThroughTheTaskTool(t *testing.T) {
	t.Run("the prompt names the task tool and its agent_type and prompt literally, and gives the executor the ticket line alone [verifies SP-1504~2]", func(t *testing.T) {
		prompt := launch.CopilotPrompt(role, ticket)
		for _, part := range []string{"Call the task tool once with agent_type \"spec-proof-dp-decider\"", "plugin's prefix", "prompt \"" + launch.ExecutorMessage(ticket) + "\""} {
			if !strings.Contains(prompt, part) {
				t.Errorf("the prompt lacks %q:\n%s", part, prompt)
			}
		}
	})
}
