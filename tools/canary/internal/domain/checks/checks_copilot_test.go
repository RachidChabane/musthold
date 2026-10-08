package checks_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/checks"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/verdict"
)

// copilotSession is a real session of Copilot CLI 1.0.92: the executor ran end to
// end, and the shell command it was asked to try first was never attempted.
const copilotSession = "copilot-session"

const (
	executorAgentID = "00726893-8f86-477f-a613-48e93e4efa1a"
	shellStart      = `{"type":"tool.execution_start","agentId":"` + executorAgentID + `","data":{"toolCallId":"c-shell","toolName":"bash","arguments":{"command":"touch sandbox-probe.txt"}}}` + "\n"
	shellFailed     = `{"type":"tool.execution_complete","agentId":"` + executorAgentID + `","data":{"toolCallId":"c-shell","success":false}}` + "\n"
	shellSucceeded  = `{"type":"tool.execution_complete","agentId":"` + executorAgentID + `","data":{"toolCallId":"c-shell","success":true}}` + "\n"
	mainAgentShell  = `{"type":"tool.execution_start","data":{"toolCallId":"c-main","toolName":"bash","arguments":{"command":"touch sandbox-probe.txt"}}}` + "\n"
)

// replaceInLines replaces text in the lines of a JSON-lines file that hold a marker.
func replaceInLines(data []byte, marker, from, to string) []byte {
	lines := bytes.Split(data, []byte("\n"))
	changed := false
	for index, line := range lines {
		if bytes.Contains(line, []byte(marker)) && bytes.Contains(line, []byte(from)) {
			lines[index] = bytes.ReplaceAll(line, []byte(from), []byte(to))
			changed = true
		}
	}
	if !changed {
		panic("fixture lacks " + from + " in a line holding " + marker)
	}
	return bytes.Join(lines, []byte("\n"))
}

func TestRealCopilotSessionAsTheADRRelies(t *testing.T) {
	t.Run("every check passes on the real session but the allowlist, which the executor never exercised, and each cites its evidence [verifies SP-1707~1]", func(t *testing.T) {
		got := evaluate(t, "copilot", readFiles(t, copilotSession))
		set, err := checks.ForHarness("copilot")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(set) {
			t.Fatalf("%d verdicts for %d checks", len(got), len(set))
		}
		for _, check := range set {
			status := verdict.Pass
			if check.Name() == "agent_allowlist_honoured" {
				status = verdict.Inconclusive
			}
			v := want(t, got, check.Name(), status)
			if len(v.Evidence) == 0 {
				t.Errorf("%s: no evidence pointer", check.Name())
			}
		}
	})

	t.Run("the executor tool is recognized under the name the payload spells, with the server and the tool joined by a hyphen [verifies SP-1517~1]", func(t *testing.T) {
		v := want(t, evaluate(t, "copilot", readFiles(t, copilotSession)), "mcp_tool_naming", verdict.Pass)
		if !strings.Contains(v.Summary, "spec-proof-dp-exec-fetch_request") {
			t.Errorf("summary = %s", v.Summary)
		}
	})

	t.Run("a payload without agent identity passes the attribution check of a harness whose strategy declares none and says so [verifies SP-1517~1]", func(t *testing.T) {
		v := want(t, evaluate(t, "copilot", readFiles(t, copilotSession)), "agent_type_in_pretool", verdict.Pass)
		if !strings.Contains(v.Summary, "no agent identity") || !strings.Contains(v.Summary, "IdentifiesCaller is false") {
			t.Errorf("summary = %s", v.Summary)
		}
	})

	t.Run("a stream with events and members the canary does not know still passes [verifies SP-1501~1]", func(t *testing.T) {
		files := readFiles(t, copilotSession)
		files.Stream = append(files.Stream, []byte(`{"id":"x","type":"some.future.event","data":{"unknown":[1,2]},"extra":true}`+"\n")...)
		want(t, evaluate(t, "copilot", files), "session_within_caps", verdict.Pass)
	})

	t.Run("a journal that closed the executor as restricted with attestation waived misreports nothing [verifies SP-1707~1]", func(t *testing.T) {
		files := readFiles(t, copilotSession)
		if !bytes.Contains(files.Journal, []byte(`"kind":"restricted"`)) || !bytes.Contains(files.Journal, []byte("attestation waived")) {
			t.Fatal("the fixture journal does not hold the waived attestation")
		}
		for name, v := range evaluate(t, "copilot", files) {
			if v.Status == verdict.Fail {
				t.Errorf("%s failed: %s", name, v.Summary)
			}
		}
	})

	t.Run("the end of the session is the result record and the summary reports its premium requests [verifies SP-1518~1]", func(t *testing.T) {
		v := want(t, evaluate(t, "copilot", readFiles(t, copilotSession)), "session_within_caps", verdict.Pass)
		if !strings.Contains(v.Summary, "(result)") || !strings.Contains(v.Summary, "1 premium requests") {
			t.Errorf("summary = %s", v.Summary)
		}
	})
}

func TestRealCopilotSessionThatBreaksARule(t *testing.T) {
	cases := []struct {
		name   string
		tag    string
		check  string
		status verdict.Status
		edit   func(*evidence.Files)
	}{
		{"a plugin missing from the listing fails the listing check", "SP-1707~1", "plugins_and_skills_listed", verdict.Fail, func(f *evidence.Files) {
			f.NoCall = replace(f.NoCall, `\"name\": \"spec-proof-alignment\"`, `\"name\": \"other\"`)
		}},
		{"a plugin that is listed but disabled fails the listing check", "SP-1707~1", "plugins_and_skills_listed", verdict.Fail, func(f *evidence.Files) {
			f.NoCall = replace(f.NoCall, `\"enabled\": true`, `\"enabled\": false`)
		}},
		{"a plugin listed from another source fails the listing check", "SP-1707~1", "plugins_and_skills_listed", verdict.Fail, func(f *evidence.Files) {
			f.NoCall = replace(f.NoCall, `\"source\": \"external\"`, `\"source\": \"marketplace\"`)
		}},
		{"a listing that was not recorded leaves the listing check inconclusive", "SP-1707~1", "plugins_and_skills_listed", verdict.Inconclusive, func(f *evidence.Files) {
			f.NoCall = replace(f.NoCall, `"copilotListing"`, `"unrelatedMember"`)
		}},
		{"a listing that is not JSON leaves the listing check inconclusive", "SP-1707~1", "plugins_and_skills_listed", verdict.Inconclusive, func(f *evidence.Files) {
			f.NoCall = replace(f.NoCall, `"pluginsOutput": "[`, `"pluginsOutput": "Plugins: [`)
		}},
		{"a tree with a problem fails the tree check", "SP-1707~1", "plugin_trees_valid", verdict.Fail, func(f *evidence.Files) {
			f.NoCall = replace(f.NoCall, `"problems": null`, `"problems": ["plugin.json is missing"]`)
		}},
		{"trees that were not inspected leave the tree check inconclusive", "SP-1707~1", "plugin_trees_valid", verdict.Inconclusive, func(f *evidence.Files) {
			f.NoCall = replace(f.NoCall, `"pluginTrees"`, `"unrelatedMember"`)
		}},
		{"a failed subagent fails the agent check", "SP-1707~1", "executor_agent_starts", verdict.Fail, func(f *evidence.Files) {
			f.Stream = replace(f.Stream, `"type":"subagent.started"`, `"type":"subagent.failed"`)
		}},
		{"a session that started no subagent leaves the agent check inconclusive", "SP-1707~1", "executor_agent_starts", verdict.Inconclusive, func(f *evidence.Files) {
			f.Stream = dropLines(f.Stream, `"subagent.`)
		}},
		{"a pre-tool hook that exits non-zero fails the hooks check", "SP-1707~1", "hooks_fire", verdict.Fail, func(f *evidence.Files) {
			f.Hooks = replaceInLines(f.Hooks, `"module":"spec-proof-dp","args":["hook","pre-tool"`, `"exitCode":0}`, `"exitCode":2}`)
		}},
		{"hooks the recorder never saw leave the hooks check inconclusive", "SP-1707~1", "hooks_fire", verdict.Inconclusive, func(f *evidence.Files) {
			f.Hooks = dropLines(f.Hooks, `"hook"`)
		}},
		{"a finished executor with no subagent-stop hook fails the stop check", "SP-1707~1", "subagent_stop_fires", verdict.Fail, func(f *evidence.Files) {
			f.Hooks = dropLines(f.Hooks, `"subagent-stop"`)
		}},
		{"a missing subagent-stop hook proves nothing when the executor did not finish", "SP-1707~1", "subagent_stop_fires", verdict.Inconclusive, func(f *evidence.Files) {
			f.Hooks = dropLines(f.Hooks, `"subagent-stop"`)
			f.Stream = dropLines(f.Stream, `"subagent.completed"`)
		}},
		{"a missing subagent-stop hook proves nothing when the hooks never ran", "SP-1707~1", "subagent_stop_fires", verdict.Inconclusive, func(f *evidence.Files) {
			f.Hooks = dropLines(f.Hooks, `"hook"`)
		}},
		{"an executor tool spelled another way fails the naming check", "SP-1707~1", "mcp_tool_naming", verdict.Fail, func(f *evidence.Files) {
			f.Hooks = replace(f.Hooks, `"toolName":"spec-proof-dp-exec-fetch_request"`, `"toolName":"spec-proof-dp-exec.fetch_request"`)
		}},
		{"the slashed spelling of the allowlist passes the naming check too", "SP-1517~1", "mcp_tool_naming", verdict.Pass, func(f *evidence.Files) {
			f.Hooks = replace(f.Hooks, `"toolName":"spec-proof-dp-exec-fetch_request"`, `"toolName":"spec-proof-dp-exec/fetch_request"`)
		}},
		{"the root server's tool, which shares the names, is not a misspelling", "SP-1707~1", "mcp_tool_naming", verdict.Inconclusive, func(f *evidence.Files) {
			f.Hooks = replace(f.Hooks, `"toolName":"spec-proof-dp-exec-fetch_request"`, `"toolName":"spec-proof-dp-fetch_request"`)
			f.Hooks = replace(f.Hooks, `"toolName":"spec-proof-dp-exec-submit_response"`, `"toolName":"spec-proof-dp-submit_response"`)
			f.Stream = dropLines(f.Stream, `"mcpServerName"`)
		}},
		{"an executor tool the stream puts on another server fails the naming check", "SP-1707~1", "mcp_tool_naming", verdict.Fail, func(f *evidence.Files) {
			f.Hooks = dropLines(f.Hooks, `spec-proof-dp-exec-`)
			f.Stream = replace(f.Stream, `"mcpServerName":"spec-proof-dp-exec"`, `"mcpServerName":"spec-proof-dp-exec-2"`)
		}},
		{"a payload that carries an agent type the strategy does not declare fails the attribution check", "SP-1517~1", "agent_type_in_pretool", verdict.Fail, func(f *evidence.Files) {
			f.Hooks = replace(f.Hooks, `"toolName":"spec-proof-dp-exec-fetch_request"`, `"agentType":"spec-proof-dp:spec-proof-dp-decider","toolName":"spec-proof-dp-exec-fetch_request"`)
		}},
		{"a payload that carries an agent id the strategy does not declare fails the attribution check", "SP-1517~1", "agent_type_in_pretool", verdict.Fail, func(f *evidence.Files) {
			f.Hooks = replace(f.Hooks, `"toolName":"spec-proof-dp-exec-submit_response"`, `"agentId":"`+executorAgentID+`","toolName":"spec-proof-dp-exec-submit_response"`)
		}},
		{"a session whose hooks recorded no executor tool payload leaves the attribution check inconclusive", "SP-1517~1", "agent_type_in_pretool", verdict.Inconclusive, func(f *evidence.Files) {
			f.Hooks = dropLines(f.Hooks, `spec-proof-dp-exec-`)
		}},
		{"a shell command the executor ran fails the allowlist check", "SP-1707~1", "agent_allowlist_honoured", verdict.Fail, func(f *evidence.Files) {
			f.Stream = append(f.Stream, []byte(shellStart+shellSucceeded)...)
		}},
		{"a shell command the executor tried that failed, with no file left, passes the allowlist check", "SP-1707~1", "agent_allowlist_honoured", verdict.Pass, func(f *evidence.Files) {
			f.Stream = append(f.Stream, []byte(shellStart+shellFailed)...)
		}},
		{"a probe file that exists after a failed attempt of the executor fails the allowlist check", "SP-1707~1", "agent_allowlist_honoured", verdict.Fail, func(f *evidence.Files) {
			f.Stream = append(f.Stream, []byte(shellStart+shellFailed)...)
			f.SandboxProbe = replace(f.SandboxProbe, `"exists": false`, `"exists": true`)
		}},
		{"a main agent that ran the command itself leaves the allowlist check inconclusive even though the file exists", "SP-1707~1", "agent_allowlist_honoured", verdict.Inconclusive, func(f *evidence.Files) {
			f.SandboxProbe = replace(f.SandboxProbe, `"exists": false`, `"exists": true`)
			f.Stream = append(f.Stream, []byte(mainAgentShell)...)
		}},
		{"a probe file that exists with no attempt in the stream leaves the allowlist check inconclusive", "SP-1707~1", "agent_allowlist_honoured", verdict.Inconclusive, func(f *evidence.Files) {
			f.SandboxProbe = replace(f.SandboxProbe, `"exists": false`, `"exists": true`)
		}},
		{"an attempt whose ending the stream does not say leaves the allowlist check inconclusive", "SP-1707~1", "agent_allowlist_honoured", verdict.Inconclusive, func(f *evidence.Files) {
			f.Stream = append(f.Stream, []byte(shellStart)...)
		}},
		{"a probe that was not recorded leaves the allowlist check inconclusive", "SP-1707~1", "agent_allowlist_honoured", verdict.Inconclusive, func(f *evidence.Files) {
			f.SandboxProbe = nil
		}},
		{"a refusal of the capability-less fetch fails where attestation is waived", "SP-1707~1", "cap_without_hooks", verdict.Fail, func(f *evidence.Files) {
			f.NoCall = replace(f.NoCall, `"refused": false`, `"refused": true`)
		}},
		{"an unchanged file fails the edit check once the session ended", "SP-1707~1", "edit_applied", verdict.Fail, func(f *evidence.Files) {
			f.Edited = replace(f.Edited, `SHIPPING_FEE = 4.50`, `SHIPPING_FEE = 4.5`)
		}},
		{"an unchanged file proves nothing when the session did not end", "SP-1707~1", "edit_applied", verdict.Inconclusive, func(f *evidence.Files) {
			f.Edited = replace(f.Edited, `SHIPPING_FEE = 4.50`, `SHIPPING_FEE = 4.5`)
			f.Stream = dropLines(f.Stream, `"type":"result"`)
		}},
		{"a session the runner ended fails the caps check", "SP-1707~1", "session_within_caps", verdict.Fail, func(f *evidence.Files) {
			f.Session = replace(f.Session, `"killed": ""`, `"killed": "timeout"`)
		}},
		{"a stream with no result record leaves the caps check inconclusive", "SP-1518~1", "session_within_caps", verdict.Inconclusive, func(f *evidence.Files) {
			f.Stream = dropLines(f.Stream, `"type":"result"`)
		}},
		{"a result record with a failing exit code is no closing event", "SP-1518~1", "session_within_caps", verdict.Inconclusive, func(f *evidence.Files) {
			f.Stream = replace(f.Stream, `"exitCode":0`, `"exitCode":1`)
		}},
		{"a write to the user's configuration fails an isolated session", "SP-1707~1", "user_config_untouched", verdict.Fail, func(f *evidence.Files) {
			f.Touched = replace(f.Touched, `"attributed": []`, `"attributed": ["/home/u/.copilot/logs/run-1"]`)
		}},
		{"a CLI that is not the pinned version leaves the version check inconclusive", "SP-1707~1", "cli_version_pinned", verdict.Inconclusive, func(f *evidence.Files) {
			f.Session = replace(f.Session, `GitHub Copilot CLI 1.0.92.`, `GitHub Copilot CLI 1.1.0.`)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name+" [verifies "+tc.tag+"]", func(t *testing.T) {
			files := readFiles(t, copilotSession)
			tc.edit(&files)
			want(t, evaluate(t, "copilot", files), tc.check, tc.status)
		})
	}
}

func TestRealCopilotSessionThatTheHarnessRefused(t *testing.T) {
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

func TestRealCopilotSessionRejectsACorruptRecord(t *testing.T) {
	t.Run("a stream line that is not JSON names its file and line [verifies SP-1501~1]", func(t *testing.T) {
		files := readFiles(t, copilotSession)
		files.Stream = append([]byte("{\"type\":\"session.mcp_servers_loaded\"}\nnot json\n"), files.Stream...)
		if _, err := evidence.Assemble(files); err == nil || err.Error() != "stream.jsonl:2: not JSON" {
			t.Fatalf("error = %v", err)
		}
	})
}
