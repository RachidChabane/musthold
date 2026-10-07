package checks_test

import (
	"testing"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/checks"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/verdict"
)

const copilotSession = "copilot-session"

func TestSyntheticCopilotSessionAsTheADRRelies(t *testing.T) {
	t.Run("every check passes on the expected session and cites its evidence [verifies SP-1707~1]", func(t *testing.T) {
		got := evaluate(t, "copilot", readFiles(t, copilotSession))
		set, err := checks.ForHarness("copilot")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(set) {
			t.Fatalf("%d verdicts for %d checks", len(got), len(set))
		}
		for _, check := range set {
			v := want(t, got, check.Name(), verdict.Pass)
			if len(v.Evidence) == 0 {
				t.Errorf("%s: no evidence pointer", check.Name())
			}
		}
	})

	t.Run("a stream with events and members the canary does not know still passes [verifies SP-1501~1]", func(t *testing.T) {
		files := readFiles(t, copilotSession)
		files.Stream = append(files.Stream, []byte(`{"id":"x","type":"some.future.event","data":{"unknown":[1,2]},"extra":true}`+"\n")...)
		files.Stream = replace(files.Stream, `"toolName":"edit"`, `"toolName":"edit","futureMember":{"a":1}`)
		want(t, evaluate(t, "copilot", files), "session_within_caps", verdict.Pass)
	})
}

func TestSyntheticCopilotSessionThatBreaksARule(t *testing.T) {
	cases := []struct {
		name   string
		check  string
		status verdict.Status
		edit   func(*evidence.Files)
	}{
		{"a plugin missing from the listing fails the listing check", "plugins_and_skills_listed", verdict.Fail, func(f *evidence.Files) {
			f.NoCall = replace(f.NoCall, `\"name\": \"spec-proof-alignment\"`, `\"name\": \"other\"`)
		}},
		{"a plugin that is listed but disabled fails the listing check", "plugins_and_skills_listed", verdict.Fail, func(f *evidence.Files) {
			f.NoCall = replace(f.NoCall, `\"enabled\": true`, `\"enabled\": false`)
		}},
		{"a listing that was not recorded leaves the listing check inconclusive", "plugins_and_skills_listed", verdict.Inconclusive, func(f *evidence.Files) {
			f.NoCall = replace(f.NoCall, `"copilotListing"`, `"unrelatedMember"`)
		}},
		{"a listing that is not JSON leaves the listing check inconclusive", "plugins_and_skills_listed", verdict.Inconclusive, func(f *evidence.Files) {
			f.NoCall = replace(f.NoCall, `"pluginsOutput": "[`, `"pluginsOutput": "Plugins: [`)
		}},
		{"a tree with a problem fails the tree check", "plugin_trees_valid", verdict.Fail, func(f *evidence.Files) {
			f.NoCall = replace(f.NoCall, `"problems": null`, `"problems": ["plugin.json is missing"]`)
		}},
		{"a tree whose root server is not the plugin's fails the tree check", "plugin_trees_valid", verdict.Fail, func(f *evidence.Files) {
			f.NoCall = replace(f.NoCall, `"servers": [
        "spec-proof-completeness"`, `"servers": [
        "other"`)
		}},
		{"trees that were not inspected leave the tree check inconclusive", "plugin_trees_valid", verdict.Inconclusive, func(f *evidence.Files) {
			f.NoCall = replace(f.NoCall, `"pluginTrees"`, `"unrelatedMember"`)
		}},
		{"a failed subagent fails the agent check", "executor_agent_starts", verdict.Fail, func(f *evidence.Files) {
			f.Stream = replace(f.Stream, `"type":"subagent.started"`, `"type":"subagent.failed"`)
		}},
		{"a session that started no subagent leaves the agent check inconclusive", "executor_agent_starts", verdict.Inconclusive, func(f *evidence.Files) {
			f.Stream = dropLines(f.Stream, `"subagent.`)
		}},
		{"a pre-tool hook that exits non-zero fails the hooks check", "hooks_fire", verdict.Fail, func(f *evidence.Files) {
			f.Hooks = replace(f.Hooks, `"stdout":"","exitCode":0}`, `"stdout":"","exitCode":2}`)
		}},
		{"hooks the recorder never saw leave the hooks check inconclusive", "hooks_fire", verdict.Inconclusive, func(f *evidence.Files) {
			f.Hooks = dropLines(f.Hooks, `"hook"`)
		}},
		{"a finished executor with no subagent-stop hook fails the stop check", "subagent_stop_fires", verdict.Fail, func(f *evidence.Files) {
			f.Hooks = dropLines(f.Hooks, `"subagent-stop"`)
		}},
		{"a missing subagent-stop hook proves nothing when the executor did not finish", "subagent_stop_fires", verdict.Inconclusive, func(f *evidence.Files) {
			f.Hooks = dropLines(f.Hooks, `"subagent-stop"`)
			f.Stream = dropLines(f.Stream, `"subagent.completed"`)
		}},
		{"a missing subagent-stop hook proves nothing when the hooks never ran", "subagent_stop_fires", verdict.Inconclusive, func(f *evidence.Files) {
			f.Hooks = dropLines(f.Hooks, `"hook"`)
		}},
		{"an executor tool spelled another way fails the naming check", "mcp_tool_naming", verdict.Fail, func(f *evidence.Files) {
			f.Hooks = replace(f.Hooks, `"toolName":"spec-proof-dp-exec/fetch_request"`, `"toolName":"spec-proof-dp-exec.fetch_request"`)
		}},
		{"the root server's tool, which shares the names, is not a misspelling", "mcp_tool_naming", verdict.Inconclusive, func(f *evidence.Files) {
			f.Hooks = replace(f.Hooks, `"toolName":"spec-proof-dp-exec/fetch_request"`, `"toolName":"spec-proof-dp/fetch_request"`)
			f.Hooks = replace(f.Hooks, `"toolName":"spec-proof-dp-exec/submit_response"`, `"toolName":"spec-proof-dp/submit_response"`)
			f.Stream = dropLines(f.Stream, `"mcpServerName"`)
		}},
		{"an executor tool the stream puts on another server fails the naming check", "mcp_tool_naming", verdict.Fail, func(f *evidence.Files) {
			f.Hooks = dropLines(f.Hooks, `exec/`)
			f.Stream = replace(f.Stream, `"mcpServerName":"spec-proof-dp-exec"`, `"mcpServerName":"spec-proof-dp-exec-2"`)
		}},
		{"a payload that names another agent type fails the attribution check", "agent_type_in_pretool", verdict.Fail, func(f *evidence.Files) {
			f.Hooks = replace(f.Hooks, `"agentType":"spec-proof-dp-decider"`, `"agentType":"general-purpose"`)
		}},
		{"a payload with no agent type leaves the attribution check inconclusive", "agent_type_in_pretool", verdict.Inconclusive, func(f *evidence.Files) {
			f.Hooks = replace(f.Hooks, `"agentType":"spec-proof-dp-decider"`, `"role":"subagent"`)
		}},
		{"a shell command that ran fails the allowlist check", "agent_allowlist_honoured", verdict.Fail, func(f *evidence.Files) {
			f.Stream = replace(f.Stream, `"success":false`, `"success":true`)
		}},
		{"a probe file that exists fails the allowlist check", "agent_allowlist_honoured", verdict.Fail, func(f *evidence.Files) {
			f.SandboxProbe = replace(f.SandboxProbe, `"exists": false`, `"exists": true`)
		}},
		{"an executor that never tried a shell leaves the allowlist check inconclusive", "agent_allowlist_honoured", verdict.Inconclusive, func(f *evidence.Files) {
			f.Stream = dropLines(f.Stream, `"toolCallId":"c2"`)
		}},
		{"an attempt whose ending the stream does not say leaves the allowlist check inconclusive", "agent_allowlist_honoured", verdict.Inconclusive, func(f *evidence.Files) {
			f.Stream = dropLines(f.Stream, `"success":false`)
		}},
		{"a probe that was not recorded leaves the allowlist check inconclusive", "agent_allowlist_honoured", verdict.Inconclusive, func(f *evidence.Files) {
			f.SandboxProbe = nil
		}},
		{"a refusal of the capability-less fetch fails where attestation is waived", "cap_without_hooks", verdict.Fail, func(f *evidence.Files) {
			f.NoCall = replace(f.NoCall, `"refused": false`, `"refused": true`)
		}},
		{"an unchanged file fails the edit check once the session ended", "edit_applied", verdict.Fail, func(f *evidence.Files) {
			f.Edited = replace(f.Edited, `SHIPPING_FEE = 4.50`, `SHIPPING_FEE = 4.5`)
		}},
		{"an unchanged file proves nothing when the session did not end", "edit_applied", verdict.Inconclusive, func(f *evidence.Files) {
			f.Edited = replace(f.Edited, `SHIPPING_FEE = 4.50`, `SHIPPING_FEE = 4.5`)
			f.Stream = dropLines(dropLines(f.Stream, "session.idle"), "session.shutdown")
		}},
		{"a session the runner ended fails the caps check", "session_within_caps", verdict.Fail, func(f *evidence.Files) {
			f.Session = replace(f.Session, `"killed": ""`, `"killed": "timeout"`)
		}},
		{"a stream with no closing event leaves the caps check inconclusive", "session_within_caps", verdict.Inconclusive, func(f *evidence.Files) {
			f.Stream = dropLines(dropLines(f.Stream, "session.idle"), "session.shutdown")
		}},
		{"a shutdown that is an error is no closing event", "session_within_caps", verdict.Inconclusive, func(f *evidence.Files) {
			f.Stream = dropLines(f.Stream, "session.idle")
			f.Stream = replace(f.Stream, `"shutdownType":"routine"`, `"shutdownType":"error"`)
		}},
		{"a write to the user's configuration fails an isolated session", "user_config_untouched", verdict.Fail, func(f *evidence.Files) {
			f.Touched = replace(f.Touched, `"attributed": []`, `"attributed": ["/home/u/.copilot/logs/run-1"]`)
		}},
		{"a CLI that is not the pinned version leaves the version check inconclusive", "cli_version_pinned", verdict.Inconclusive, func(f *evidence.Files) {
			f.Session = replace(f.Session, `GitHub Copilot CLI 1.0.92.`, `GitHub Copilot CLI 1.1.0.`)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name+" [verifies SP-1707~1]", func(t *testing.T) {
			files := readFiles(t, copilotSession)
			tc.edit(&files)
			want(t, evaluate(t, "copilot", files), tc.check, tc.status)
		})
	}
}

func TestSyntheticCopilotSessionThatTheHarnessRefused(t *testing.T) {
	refused := []byte(`{"id":"e1","type":"session.error","data":{"errorType":"authentication","message":"you are not logged in"}}` + "\n")
	for _, errorType := range []string{"authentication", "quota", "rate_limit"} {
		t.Run("a "+errorType+" refusal leaves every check that reads the session inconclusive [verifies SP-1707~1]", func(t *testing.T) {
			files := readFiles(t, copilotSession)
			files.Stream = replace(refused, "authentication", errorType)
			files.Edited = replace(files.Edited, `SHIPPING_FEE = 4.50`, `SHIPPING_FEE = 4.5`)
			got := evaluate(t, "copilot", files)
			for _, check := range []string{"executor_agent_starts", "hooks_fire", "subagent_stop_fires", "mcp_tool_naming", "agent_type_in_pretool", "agent_allowlist_honoured", "edit_applied"} {
				want(t, got, check, verdict.Inconclusive)
			}
			want(t, got, "plugins_and_skills_listed", verdict.Pass)
		})
	}

	t.Run("any other session error is not a refusal and the checks read the session [verifies SP-1707~1]", func(t *testing.T) {
		files := readFiles(t, copilotSession)
		files.Stream = append(files.Stream, []byte(`{"id":"e2","type":"session.error","data":{"errorType":"tool","message":"a tool failed"}}`+"\n")...)
		want(t, evaluate(t, "copilot", files), "hooks_fire", verdict.Pass)
	})
}

func TestSyntheticCopilotSessionRejectsACorruptRecord(t *testing.T) {
	t.Run("a stream line that is not JSON names its file and line [verifies SP-1501~1]", func(t *testing.T) {
		files := readFiles(t, copilotSession)
		files.Stream = append([]byte("{\"type\":\"session.start\"}\nnot json\n"), files.Stream...)
		if _, err := evidence.Assemble(files); err == nil || err.Error() != "stream.jsonl:2: not JSON" {
			t.Fatalf("error = %v", err)
		}
	})
}
