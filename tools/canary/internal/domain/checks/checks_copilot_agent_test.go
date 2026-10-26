package checks_test

import (
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/domain/verdict"
)

func TestCopilotModelSelected(t *testing.T) {
	const check = "session_model"

	t.Run("a session that ran on the model it was launched with passes and points at its model.call_start [verifies SP-1514~2]", func(t *testing.T) {
		v := want(t, evaluate(t, "copilot", readFiles(t, copilotSession)), check, verdict.Pass)
		if !strings.Contains(v.Summary, "gpt-5.6-luna") || v.Evidence[0].File != "stream.jsonl" || v.Evidence[0].Line != 11 {
			t.Errorf("verdict = %+v", v)
		}
	})

	t.Run("a session that ran on another model than it was launched with is inconclusive and names both [verifies SP-1514~2]", func(t *testing.T) {
		files := readFiles(t, copilotSession)
		files.Session = replace(files.Session, `"model": "gpt-5.6-luna"`, `"model": "claude-haiku-4.5"`)
		v := want(t, evaluate(t, "copilot", files), check, verdict.Inconclusive)
		for _, part := range []string{"claude-haiku-4.5", "gpt-5.6-luna"} {
			if !strings.Contains(v.Summary, part) {
				t.Errorf("the summary lacks %q: %s", part, v.Summary)
			}
		}
	})

	t.Run("a stream with no model.call_start leaves the model unknown [verifies SP-1514~2]", func(t *testing.T) {
		files := readFiles(t, copilotSession)
		files.Stream = dropLines(files.Stream, `"model.call_start"`)
		want(t, evaluate(t, "copilot", files), check, verdict.Inconclusive)
	})
}

func TestCopilotAgentNamedWithAPluginPrefix(t *testing.T) {
	t.Run("the executor named with the plugin prefix in the subagent and the task call passes and the report gives the exact name [verifies SP-1515~2]", func(t *testing.T) {
		v := want(t, evaluate(t, "copilot", readFiles(t, copilotSession)), "executor_agent_starts", verdict.Pass)
		if !strings.Contains(v.Summary, `the task tool was called with agent_type "musthold-dp:musthold-dp-decider"`) {
			t.Errorf("summary = %s", v.Summary)
		}
	})

	t.Run("the plain name starts the executor too [verifies SP-1515~2]", func(t *testing.T) {
		files := readFiles(t, copilotSession)
		files.Stream = replace(files.Stream, `musthold-dp:musthold-dp-decider`, `musthold-dp-decider`)
		want(t, evaluate(t, "copilot", files), "executor_agent_starts", verdict.Pass)
	})
}
