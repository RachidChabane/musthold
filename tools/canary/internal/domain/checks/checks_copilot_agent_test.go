package checks_test

import (
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/verdict"
)

func TestCopilotModelSelected(t *testing.T) {
	const check = "session_model"

	t.Run("a session that selected the model it was launched with passes and points at session.start [verifies SP-1514~1]", func(t *testing.T) {
		v := want(t, evaluate(t, "copilot", readFiles(t, copilotSession)), check, verdict.Pass)
		if !strings.Contains(v.Summary, "gpt-5.6-luna") || v.Evidence[0].File != "stream.jsonl" || v.Evidence[0].Line != 1 {
			t.Errorf("verdict = %+v", v)
		}
	})

	t.Run("a session that selected another model than it was launched with is inconclusive and names both [verifies SP-1514~1]", func(t *testing.T) {
		files := readFiles(t, copilotSession)
		files.Session = replace(files.Session, `"model": "gpt-5.6-luna"`, `"model": "claude-haiku-4.5"`)
		v := want(t, evaluate(t, "copilot", files), check, verdict.Inconclusive)
		for _, part := range []string{"claude-haiku-4.5", "gpt-5.6-luna"} {
			if !strings.Contains(v.Summary, part) {
				t.Errorf("the summary lacks %q: %s", part, v.Summary)
			}
		}
	})

	t.Run("a stream with no selected model leaves the model unknown [verifies SP-1514~1]", func(t *testing.T) {
		files := readFiles(t, copilotSession)
		files.Stream = replace(files.Stream, `,"selectedModel":"gpt-5.6-luna"`, ``)
		want(t, evaluate(t, "copilot", files), check, verdict.Inconclusive)
	})
}

func TestCopilotAgentNamedWithAPluginPrefix(t *testing.T) {
	prefixed := func(separator string) string { return "spec-proof-dp" + separator + "spec-proof-dp-decider" }

	t.Run("the executor named with a plugin prefix in the payload passes and the report gives the exact name [verifies SP-1515~1]", func(t *testing.T) {
		for _, separator := range []string{":", "/", "."} {
			files := readFiles(t, copilotSession)
			files.Hooks = replace(files.Hooks, `"agentType":"spec-proof-dp-decider"`, `"agentType":"`+prefixed(separator)+`"`)
			v := want(t, evaluate(t, "copilot", files), "agent_type_in_pretool", verdict.Pass)
			if !strings.Contains(v.Summary, prefixed(separator)) {
				t.Errorf("separator %q: the summary lacks the exact name: %s", separator, v.Summary)
			}
		}
	})

	t.Run("the plain name passes too, and a name that merely ends alike fails [verifies SP-1515~1]", func(t *testing.T) {
		want(t, evaluate(t, "copilot", readFiles(t, copilotSession)), "agent_type_in_pretool", verdict.Pass)
		files := readFiles(t, copilotSession)
		files.Hooks = replace(files.Hooks, `"agentType":"spec-proof-dp-decider"`, `"agentType":"xspec-proof-dp-decider"`)
		want(t, evaluate(t, "copilot", files), "agent_type_in_pretool", verdict.Fail)
	})

	t.Run("a prefixed subagent and task call start the executor and the call's agent_type is reported [verifies SP-1515~1]", func(t *testing.T) {
		files := readFiles(t, copilotSession)
		files.Stream = replace(files.Stream, `"agent_type":"spec-proof-dp-decider"`, `"agent_type":"`+prefixed(":")+`"`)
		files.Stream = replace(files.Stream, `"agentName":"spec-proof-dp-decider"`, `"agentName":"`+prefixed(":")+`"`)
		v := want(t, evaluate(t, "copilot", files), "executor_agent_starts", verdict.Pass)
		if !strings.Contains(v.Summary, `the task tool was called with agent_type "`+prefixed(":")+`"`) {
			t.Errorf("summary = %s", v.Summary)
		}
	})
}
