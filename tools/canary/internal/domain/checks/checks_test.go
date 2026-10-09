package checks_test

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/checks"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/verdict"
)

const (
	claudeSession = "claude-session"
	codexSession  = "codex-session"
)

// readFiles reads the recorded files of a session directory.
func readFiles(t *testing.T, name string) evidence.Files {
	t.Helper()
	read := func(file string) []byte {
		data, err := os.ReadFile(filepath.Join("testdata", name, file))
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		return data
	}
	return evidence.Files{
		Session: read(evidence.SessionFile), Stream: read(evidence.StreamFile), Hooks: read(evidence.HooksFile),
		Journal: read(evidence.JournalFile), ClassifyJournal: read(evidence.ClassifyJournalFile), NoCall: read(evidence.NoCallFile), Edited: read(evidence.EditedFile),
		SandboxProbe: read(evidence.SandboxProbeFile), Touched: read(evidence.TouchedFile),
		RolloutMain: read(evidence.RolloutMainFile), RolloutChildren: readChildren(t, name),
	}
}

// readChildren reads the rollouts of the spawned threads of a session directory.
func readChildren(t *testing.T, name string) map[string][]byte {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("testdata", name, evidence.RolloutChildrenDir, "*"))
	if err != nil {
		t.Fatal(err)
	}
	var children map[string][]byte
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if children == nil {
			children = map[string][]byte{}
		}
		children[filepath.Base(path)] = data
	}
	return children
}

// evaluate assembles the files and returns the verdicts by check name.
func evaluate(t *testing.T, flag string, files evidence.Files) map[string]verdict.Verdict {
	t.Helper()
	found, err := evidence.Assemble(files)
	if err != nil {
		t.Fatal(err)
	}
	report, err := checks.Evaluate(flag, found)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]verdict.Verdict{}
	for _, v := range report.Verdicts {
		byName[v.Check] = v
	}
	return byName
}

// dropLines removes the lines of a JSON-lines file that hold the text.
func dropLines(data []byte, text string) []byte {
	var kept [][]byte
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(line) > 0 && !bytes.Contains(line, []byte(text)) {
			kept = append(kept, line)
		}
	}
	return append(bytes.Join(kept, []byte("\n")), '\n')
}

func replace(data []byte, from, to string) []byte {
	if !bytes.Contains(data, []byte(from)) {
		panic("fixture lacks " + from)
	}
	return bytes.ReplaceAll(data, []byte(from), []byte(to))
}

// replaceInAnswer replaces text inside a hook's answer, which the hook log holds as
// a JSON string, so that its quotes are escaped.
func replaceInAnswer(data []byte, from, to string) []byte {
	escape := func(text string) string { return strings.ReplaceAll(text, `"`, `\"`) }
	return replace(data, escape(from), escape(to))
}

func want(t *testing.T, got map[string]verdict.Verdict, check string, status verdict.Status) verdict.Verdict {
	t.Helper()
	v, found := got[check]
	if !found {
		t.Fatalf("no verdict for %s", check)
	}
	if v.Status != status {
		t.Fatalf("%s: %s (%s), want %s", check, v.Status, v.Summary, status)
	}
	return v
}

func TestSyntheticClaudeSessionAsTheADRRelies(t *testing.T) {
	t.Run("every check passes on the expected session and cites its evidence [verifies SP-1502~1]", func(t *testing.T) {
		got := evaluate(t, "claude", readFiles(t, claudeSession))
		set, err := checks.ForHarness("claude")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(set) {
			t.Fatalf("%d verdicts for %d checks", len(got), len(set))
		}
		for _, check := range set {
			v := want(t, got, check.Name(), verdict.Pass)
			if v.Proves == "" || v.Summary == "" {
				t.Errorf("%s: a verdict says neither what it proves nor why", check.Name())
			}
			if len(v.Evidence) == 0 {
				t.Errorf("%s: a verdict points at no evidence", check.Name())
			}
			for _, pointer := range v.Evidence {
				if pointer.File == "" {
					t.Errorf("%s: a pointer names no file", check.Name())
				}
			}
		}
	})

	t.Run("the verdicts keep the order the report lists them in [verifies SP-1501~1]", func(t *testing.T) {
		found, err := evidence.Assemble(readFiles(t, claudeSession))
		if err != nil {
			t.Fatal(err)
		}
		report, err := checks.Evaluate("claude", found)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, v := range report.Verdicts {
			names = append(names, v.Check)
		}
		set, _ := checks.ForHarness("claude")
		var wanted []string
		for _, check := range set {
			wanted = append(wanted, check.Name())
		}
		if !slices.Equal(names, wanted) {
			t.Fatalf("order %v, want %v", names, wanted)
		}
		if report.ExitCode() != verdict.ExitAllPass {
			t.Errorf("exit code %d for an all-pass report", report.ExitCode())
		}
	})
}

func TestSyntheticClaudeSessionThatBreaksARule(t *testing.T) {
	t.Run("a plugin missing from system/init fails the listing [verifies SP-1502~1]", func(t *testing.T) {
		files := readFiles(t, claudeSession)
		files.Stream = replace(files.Stream, `{"name":"spec-proof-completeness","path"`, `{"name":"another-plugin","path"`)
		want(t, evaluate(t, "claude", files), "plugin_and_server_listed", verdict.Fail)
	})

	t.Run("a server that did not connect fails the listing [verifies SP-1502~1]", func(t *testing.T) {
		files := readFiles(t, claudeSession)
		files.Stream = replace(files.Stream, `{"name":"plugin:spec-proof-dp:exec","status":"connected"}`, `{"name":"plugin:spec-proof-dp:exec","status":"failed"}`)
		want(t, evaluate(t, "claude", files), "plugin_and_server_listed", verdict.Fail)
	})

	t.Run("a main-session fetch that returns the ticket fails the denial [verifies SP-1502~1]", func(t *testing.T) {
		files := readFiles(t, claudeSession)
		files.Stream = replace(files.Stream, `"is_error":true,"content":"PreToolUse`, `"is_error":false,"content":"{\"nonce\":\"4efabc\"} PreToolUse`)
		want(t, evaluate(t, "claude", files), "main_session_fetch_denied", verdict.Fail)
	})

	t.Run("a hook that allows the main session's fetch fails the denial [verifies SP-1502~1]", func(t *testing.T) {
		files := readFiles(t, claudeSession)
		files.Hooks = replaceInAnswer(files.Hooks, `"permissionDecision":"deny","permissionDecisionReason":"the executor tools belong to the executor subagents"`, `"permissionDecision":"allow"`)
		files.Journal = dropLines(files.Journal, "exposure_attempt")
		got := evaluate(t, "claude", files)
		want(t, got, "main_session_fetch_denied", verdict.Fail)
	})

	t.Run("a rewrite that keeps the model fails [verifies SP-1502~1]", func(t *testing.T) {
		files := readFiles(t, claudeSession)
		files.Hooks = replaceInAnswer(files.Hooks, `"updatedInput":{"subagent_type":"spec-proof-dp:decider","description":"canary run","prompt":"ticket `, `"updatedInput":{"model":"haiku","subagent_type":"spec-proof-dp:decider","description":"canary run","prompt":"ticket `)
		want(t, evaluate(t, "claude", files), "rewrite_drops_model", verdict.Fail)
	})

	t.Run("an executor that ran on the model the main agent asked for fails the rewrite [verifies SP-1502~1]", func(t *testing.T) {
		files := readFiles(t, claudeSession)
		files.Journal = nil
		journal := readFiles(t, claudeSession).Journal
		files.Journal = replace(journal, `claude-sonnet-5-5`, `claude-haiku-4-5`)
		got := evaluate(t, "claude", files)
		want(t, got, "rewrite_drops_model", verdict.Fail)
	})

	t.Run("a Bash call by the executor fails the check on its tools [verifies SP-1502~1]", func(t *testing.T) {
		files := readFiles(t, claudeSession)
		files.Stream = replace(files.Stream, `"name":"mcp__plugin_spec-proof-dp_exec__submit_response","input":{"ticket":"864f5e696da12d6944d9380733d5c5a9fba0f0140f3b243c3efae9a6cc74cb38.0.1","nonce"`, `"name":"Bash","input":{"command":"cat secrets","nonce"`)
		want(t, evaluate(t, "claude", files), "executor_without_bash", verdict.Fail)
	})

	t.Run("a stop hook that marks the run exposed fails the check on its tools [verifies SP-1502~1]", func(t *testing.T) {
		files := readFiles(t, claudeSession)
		files.Journal = replace(files.Journal, `"kind":"attested"`, `"kind":"exposed"`)
		want(t, evaluate(t, "claude", files), "executor_without_bash", verdict.Fail)
	})

	t.Run("a missing hook fails the hooks check once an executor was launched [verifies SP-1502~1]", func(t *testing.T) {
		files := readFiles(t, claudeSession)
		files.Hooks = dropLines(files.Hooks, `"subagent-stop"`)
		v := want(t, evaluate(t, "claude", files), "hooks_fire_under_print", verdict.Fail)
		if !strings.Contains(v.Summary, "subagent-stop") {
			t.Errorf("the summary does not name the missing hook: %s", v.Summary)
		}
	})

	t.Run("a hook that exits non-zero fails the hooks check [verifies SP-1502~1]", func(t *testing.T) {
		files := readFiles(t, claudeSession)
		files.Hooks = replace(files.Hooks, `"stdout":"{}","exitCode":0}`, `"stdout":"{}","exitCode":2}`)
		want(t, evaluate(t, "claude", files), "hooks_fire_under_print", verdict.Fail)
	})

	t.Run("a hook that gives no capability fails the capability check [verifies SP-1502~1]", func(t *testing.T) {
		files := readFiles(t, claudeSession)
		files.Hooks = replaceInAnswer(files.Hooks, `"updatedInput":{"ticket":"864f5e696da12d6944d9380733d5c5a9fba0f0140f3b243c3efae9a6cc74cb38.0.1","cap":"agent-7f3a.5c1d"}`, `"updatedInput":{"ticket":"864f5e696da12d6944d9380733d5c5a9fba0f0140f3b243c3efae9a6cc74cb38.0.1"}`)
		want(t, evaluate(t, "claude", files), "hook_allow_issues_cap", verdict.Fail)
	})

	t.Run("an executor fetch that ended in an error fails the capability check [verifies SP-1502~1]", func(t *testing.T) {
		files := readFiles(t, claudeSession)
		files.Stream = replace(files.Stream, `{"type":"tool_result","tool_use_id":"toolu_01SubFetch","content"`, `{"type":"tool_result","tool_use_id":"toolu_01SubFetch","is_error":true,"content"`)
		want(t, evaluate(t, "claude", files), "hook_allow_issues_cap", verdict.Fail)
	})

	t.Run("a session that finished with no Stop hook fails the Stop check [verifies SP-1502~1]", func(t *testing.T) {
		files := readFiles(t, claudeSession)
		files.Hooks = dropLines(files.Hooks, "Stop-probe")
		files.Stream = dropLines(files.Stream, `"hook_event":"Stop"`)
		want(t, evaluate(t, "claude", files), "stop_fires", verdict.Fail)
	})

	t.Run("a server that serves a ticket without a capability fails the cap check [verifies SP-1502~1]", func(t *testing.T) {
		files := readFiles(t, claudeSession)
		files.NoCall = replace(files.NoCall, `"refused": true,`, `"refused": false,`)
		files.NoCall = replace(files.NoCall, `"servedTicket": false`, `"servedTicket": true`)
		want(t, evaluate(t, "claude", files), "cap_without_hooks", verdict.Fail)
	})

	t.Run("a plugin the validator rejects fails the manifest check [verifies SP-1502~1]", func(t *testing.T) {
		files := readFiles(t, claudeSession)
		files.NoCall = replace(files.NoCall, "\"exitCode\": 0,\n      \"output\": \"Validation passed\"", "\"exitCode\": 1,\n      \"output\": \"invalid\"")
		want(t, evaluate(t, "claude", files), "plugin_manifests_valid", verdict.Fail)
	})

	t.Run("a line left unchanged fails the edit check [verifies SP-1502~1]", func(t *testing.T) {
		files := readFiles(t, claudeSession)
		files.Edited = []byte("SHIPPING_FEE = 4.5\n")
		want(t, evaluate(t, "claude", files), "edit_applied", verdict.Fail)
	})

	t.Run("a session the runner had to kill fails the cap check [verifies SP-1502~1]", func(t *testing.T) {
		files := readFiles(t, claudeSession)
		files.Session = replace(files.Session, `"killed": ""`, `"killed": "turn-cap"`)
		want(t, evaluate(t, "claude", files), "session_within_caps", verdict.Fail)
	})
}

func TestSyntheticSessionWhereTheSetupIsInDoubt(t *testing.T) {
	t.Run("a Stop probe that stays silent beside silent control hooks proves nothing about Stop [verifies SP-1502~1]", func(t *testing.T) {
		files := readFiles(t, claudeSession)
		for _, label := range []string{"Stop-probe", "SessionStart-probe", "UserPromptSubmit-probe"} {
			files.Hooks = dropLines(files.Hooks, label)
		}
		files.Stream = dropLines(files.Stream, `"hook_event":"Stop"`)
		v := want(t, evaluate(t, "claude", files), "stop_fires", verdict.Inconclusive)
		if !strings.Contains(v.Summary, "not loaded") {
			t.Errorf("the summary does not say the settings were not loaded: %s", v.Summary)
		}
	})

	t.Run("a Codex Stop probe that stays silent beside a silent PreToolUse probe proves nothing about Stop [verifies SP-1503~2]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.Hooks = dropLines(files.Hooks, "Stop-probe")
		files.Hooks = dropLines(files.Hooks, "PreToolUse-probe")
		want(t, evaluate(t, "codex", files), "stop_fires", verdict.Inconclusive)
	})

	t.Run("an executor server that does not start fails the start check and leaves the listing inconclusive [verifies SP-1502~1]", func(t *testing.T) {
		files := readFiles(t, claudeSession)
		files.NoCall = replace(files.NoCall, "\"binary\": \"spec-proof-alignment\",\n      \"started\": true", "\"binary\": \"spec-proof-alignment\",\n      \"started\": false")
		files.Stream = replace(files.Stream, `{"name":"plugin:spec-proof-alignment:exec","status":"connected"}`, `{"name":"plugin:spec-proof-alignment:exec","status":"failed"}`)
		got := evaluate(t, "claude", files)
		v := want(t, got, "executor_servers_start", verdict.Fail)
		if !strings.Contains(v.Summary, "spec-proof-alignment") {
			t.Errorf("the summary does not name the module: %s", v.Summary)
		}
		want(t, got, "plugin_and_server_listed", verdict.Inconclusive)
	})

	t.Run("a server the harness did not list although the module's own server starts is the harness's failure [verifies SP-1502~1]", func(t *testing.T) {
		files := readFiles(t, claudeSession)
		files.Stream = replace(files.Stream, `{"name":"plugin:spec-proof-alignment:exec","status":"connected"}`, `{"name":"plugin:spec-proof-alignment:exec","status":"failed"}`)
		want(t, evaluate(t, "claude", files), "plugin_and_server_listed", verdict.Fail)
	})

	t.Run("an isolated session that wrote an entry naming its run fails the configuration check [verifies SP-1502~1]", func(t *testing.T) {
		files := readFiles(t, claudeSession)
		files.Touched = replace(files.Touched, `"attributed": []`, `"attributed": ["/home/u/.claude/projects/-tmp-spec-proof-canary-claude-1-project"]`)
		v := want(t, evaluate(t, "claude", files), "user_config_untouched", verdict.Fail)
		if !strings.Contains(v.Summary, "spec-proof-canary-claude-1") {
			t.Errorf("the summary does not list the entry: %s", v.Summary)
		}
	})

	t.Run("a session that shares the user's configuration lists what it wrote and concludes nothing [verifies SP-1502~1]", func(t *testing.T) {
		files := readFiles(t, claudeSession)
		files.Session = replace(files.Session, `"configMode": "isolated"`, `"configMode": "shared"`)
		files.Touched = replace(files.Touched, `"attributed": []`, `"attributed": ["/home/u/.claude/projects/-tmp-spec-proof-canary-claude-1-project"]`)
		v := want(t, evaluate(t, "claude", files), "user_config_untouched", verdict.Inconclusive)
		if !strings.Contains(v.Summary, "1 entries") {
			t.Errorf("the summary does not count the entries: %s", v.Summary)
		}
	})

	t.Run("a sandbox that declined the write counts as holding when the file is absent [verifies SP-1503~2]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.Stream = replace(files.Stream, `"exit_code":1,"status":"failed"`, `"status":"declined"`)
		v := want(t, evaluate(t, "codex", files), "executor_sandbox", verdict.Pass)
		if !strings.Contains(v.Summary, "declined") {
			t.Errorf("summary = %s", v.Summary)
		}
	})
}

func TestSyntheticSessionWithAbsentEvidenceIsNeverAFailure(t *testing.T) {
	probes := []string{
		"plugin_and_server_listed", "main_session_fetch_denied", "rewrite_drops_model", "hooks_fire_under_print",
		"hook_allow_issues_cap", "executor_without_bash", "stop_fires", "edit_applied", "session_within_caps",
		"cap_without_hooks", "plugin_manifests_valid", "cli_version_pinned", "executor_servers_start", "user_config_untouched", "classify_witness_recorded",
	}
	t.Run("a session that left nothing leaves every check inconclusive [verifies SP-1501~1]", func(t *testing.T) {
		got := evaluate(t, "claude", evidence.Files{})
		for _, name := range probes {
			want(t, got, name, verdict.Inconclusive)
		}
		report, _ := checks.Evaluate("claude", evidence.Evidence{})
		if report.ExitCode() != verdict.ExitInconclusive {
			t.Errorf("exit code %d, want %d", report.ExitCode(), verdict.ExitInconclusive)
		}
	})

	t.Run("a main agent that never attempted the fetch leaves the denial inconclusive [verifies SP-1501~1]", func(t *testing.T) {
		files := readFiles(t, claudeSession)
		files.Stream = dropLines(files.Stream, "toolu_01Fetch")
		want(t, evaluate(t, "claude", files), "main_session_fetch_denied", verdict.Inconclusive)
	})

	t.Run("a main agent that sent no padding leaves the rewrite inconclusive [verifies SP-1501~1]", func(t *testing.T) {
		files := readFiles(t, claudeSession)
		files.Stream = replace(files.Stream, `"model":"haiku",`, ``)
		want(t, evaluate(t, "claude", files), "rewrite_drops_model", verdict.Inconclusive)
	})

	t.Run("an unreadable transcript leaves the executor check inconclusive [verifies SP-1501~1]", func(t *testing.T) {
		files := readFiles(t, claudeSession)
		files.Journal = replace(files.Journal, `"kind":"attested"`, `"kind":"restricted"`)
		want(t, evaluate(t, "claude", files), "executor_without_bash", verdict.Inconclusive)
	})

	t.Run("a session that did not finish leaves the missing Stop inconclusive [verifies SP-1501~1]", func(t *testing.T) {
		files := readFiles(t, claudeSession)
		files.Hooks = dropLines(files.Hooks, "Stop-probe")
		files.Stream = dropLines(files.Stream, `"hook_event":"Stop"`)
		files.Stream = dropLines(files.Stream, `"type":"result"`)
		want(t, evaluate(t, "claude", files), "stop_fires", verdict.Inconclusive)
	})

	t.Run("a CLI of another version leaves the version check inconclusive [verifies SP-1501~1]", func(t *testing.T) {
		files := readFiles(t, claudeSession)
		files.Session = replace(files.Session, `2.1.295`, `2.2.0`)
		v := want(t, evaluate(t, "claude", files), "cli_version_pinned", verdict.Inconclusive)
		if !strings.Contains(v.Summary, "2.2.0") {
			t.Errorf("the summary does not say which version ran: %s", v.Summary)
		}
	})

	t.Run("a recorded line that is not JSON is an error naming the file and the line [verifies SP-1501~1]", func(t *testing.T) {
		files := readFiles(t, claudeSession)
		files.Hooks = append(files.Hooks, []byte("not json\n")...)
		_, err := evidence.Assemble(files)
		if err == nil || !strings.Contains(err.Error(), "hooks.jsonl:") {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a harness with no checks is refused [verifies SP-1501~1]", func(t *testing.T) {
		if _, err := checks.Evaluate("gemini", evidence.Evidence{}); err == nil {
			t.Fatal("gemini was evaluated")
		}
	})
}

func TestSyntheticCodexSessionAsTheADRRelies(t *testing.T) {
	t.Run("every check passes on the expected session [verifies SP-1503~2]", func(t *testing.T) {
		got := evaluate(t, "codex", readFiles(t, codexSession))
		set, err := checks.ForHarness("codex")
		if err != nil {
			t.Fatal(err)
		}
		for _, check := range set {
			v := want(t, got, check.Name(), verdict.Pass)
			if len(v.Evidence) == 0 {
				t.Errorf("%s: no evidence pointer", check.Name())
			}
		}
	})
}

func TestSyntheticCodexSessionThatBreaksARule(t *testing.T) {
	t.Run("a failed spawn fails the agent files check [verifies SP-1503~2]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.Hooks = dropLines(files.Hooks, `"subagent-start"`)
		files.Stream = replace(files.Stream, `"status":"completed","sender_thread_id"`, `"status":"failed","sender_thread_id"`)
		want(t, evaluate(t, "codex", files), "agent_files_load", verdict.Fail)
	})

	t.Run("a payload with no agent type fails the attribution check [verifies SP-1503~2]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.Hooks = replace(files.Hooks, `"agent_type":"spec-proof-dp-decider","tool_name"`, `"tool_name"`)
		want(t, evaluate(t, "codex", files), "agent_type_in_pretool", verdict.Fail)
	})

	t.Run("a tool spelled another way fails the naming check [verifies SP-1503~2]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.Hooks = replace(files.Hooks, `"tool_name":"mcp__spec_proof_dp_exec__fetch_request"`, `"tool_name":"spec-proof-dp-exec.fetch_request"`)
		want(t, evaluate(t, "codex", files), "mcp_tool_naming", verdict.Fail)
	})

	t.Run("the unsanitized spelling of the server name fails the naming check, which expects the name Codex gives the tool [verifies SP-1503~2]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.Hooks = replace(files.Hooks, `"tool_name":"mcp__spec_proof_dp_exec__fetch_request"`, `"tool_name":"mcp__spec-proof-dp-exec__fetch_request"`)
		v := want(t, evaluate(t, "codex", files), "mcp_tool_naming", verdict.Fail)
		if !strings.Contains(v.Summary, "mcp__spec_proof_dp_exec__fetch_request") {
			t.Errorf("summary = %s", v.Summary)
		}
	})

	t.Run("a write that landed fails the sandbox check [verifies SP-1503~2]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.SandboxProbe = replace(files.SandboxProbe, `"exists": false`, `"exists": true`)
		want(t, evaluate(t, "codex", files), "executor_sandbox", verdict.Fail)
	})

	t.Run("a write that landed without any executor agent is not charged to the executor sandbox [verifies SP-1503~2]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.SandboxProbe = replace(files.SandboxProbe, `"exists": false`, `"exists": true`)
		files.Stream = dropLines(files.Stream, "spawn_agent")
		files.RolloutMain, files.RolloutChildren = nil, nil
		want(t, evaluate(t, "codex", files), "executor_sandbox", verdict.Inconclusive)
	})

	t.Run("an executor that never tried the write leaves the sandbox inconclusive [verifies SP-1503~2]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.Stream = dropLines(files.Stream, "touch sandbox-probe.txt'")
		want(t, evaluate(t, "codex", files), "executor_sandbox", verdict.Inconclusive)
	})

	t.Run("a refusal of the capability-less fetch fails where attestation is waived [verifies SP-1503~2]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.NoCall = replace(files.NoCall, `"refused": false`, `"refused": true`)
		want(t, evaluate(t, "codex", files), "cap_without_hooks", verdict.Fail)
	})

	t.Run("hooks that never ran fail once an executor was spawned [verifies SP-1503~2]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.Hooks = dropLines(files.Hooks, `"subagent-stop"`)
		want(t, evaluate(t, "codex", files), "hooks_fire", verdict.Fail)
	})

	t.Run("a missing pre-tool hook leaves the hooks inconclusive when no executor tool was called for it to match [verifies SP-1516~1]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.Hooks = dropLines(files.Hooks, "fetch_request")
		files.Stream = dropLines(files.Stream, "mcp_tool_call")
		v := want(t, evaluate(t, "codex", files), "hooks_fire", verdict.Inconclusive)
		if !strings.Contains(v.Summary, "no executor tool call was made") {
			t.Errorf("summary = %s", v.Summary)
		}
	})

	t.Run("a missing pre-tool hook fails the hooks when an executor tool call is recorded by the probe or by the stream [verifies SP-1516~1]", func(t *testing.T) {
		probeOnly := readFiles(t, codexSession)
		probeOnly.Hooks = dropLines(probeOnly.Hooks, `"pre-tool"`)
		want(t, evaluate(t, "codex", probeOnly), "hooks_fire", verdict.Fail)

		streamOnly := readFiles(t, codexSession)
		streamOnly.Hooks = dropLines(streamOnly.Hooks, "fetch_request")
		want(t, evaluate(t, "codex", streamOnly), "hooks_fire", verdict.Fail)
	})

	t.Run("a hook other than the pre-tool one that never ran still fails when no executor tool was called [verifies SP-1516~1]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.Hooks = dropLines(files.Hooks, "fetch_request")
		files.Hooks = dropLines(files.Hooks, `"subagent-stop"`)
		files.Stream = dropLines(files.Stream, "mcp_tool_call")
		want(t, evaluate(t, "codex", files), "hooks_fire", verdict.Fail)
	})

	t.Run("the main agent's executor tool call, which carries no agent identity, fails the attribution check [verifies SP-1503~2]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.Hooks = replace(files.Hooks, `"agent_id":"01a11334-f9a0-7c11-8e02-1b3f0c2d4e55","agent_type":"spec-proof-dp-decider","tool_name"`, `"tool_name"`)
		want(t, evaluate(t, "codex", files), "agent_type_in_pretool", verdict.Fail)
	})

	t.Run("a session with no spawn leaves the hooks inconclusive [verifies SP-1503~2]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.Hooks = nil
		files.Stream = dropLines(files.Stream, "collab_tool_call")
		files.RolloutMain, files.RolloutChildren = nil, nil
		got := evaluate(t, "codex", files)
		want(t, got, "hooks_fire", verdict.Inconclusive)
		want(t, got, "agent_files_load", verdict.Inconclusive)
	})

	t.Run("a turn that completed with no Stop hook fails the Stop check [verifies SP-1503~2]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.Hooks = dropLines(files.Hooks, "Stop-probe")
		want(t, evaluate(t, "codex", files), "stop_fires", verdict.Fail)
	})
}

func TestEveryHarnessCheckCarriesAClaim(t *testing.T) {
	t.Run("a check names what a pass proves and what it leaves open [verifies SP-1501~1]", func(t *testing.T) {
		for _, flag := range []string{"claude", "codex", "copilot"} {
			set, err := checks.ForHarness(flag)
			if err != nil {
				t.Fatal(err)
			}
			seen := map[string]bool{}
			for _, check := range set {
				if seen[check.Name()] || check.Name() == "" || len(check.Proves()) < 20 {
					t.Errorf("%s: check %q is unnamed, duplicated or says nothing", flag, check.Name())
				}
				seen[check.Name()] = true
			}
		}
	})

	t.Run("the no-call checks are those whose evidence is gathered before the session [verifies SP-1501~1]", func(t *testing.T) {
		names := map[string]bool{}
		for _, flag := range []string{"claude", "codex", "copilot"} {
			set, _ := checks.ForHarness(flag)
			for _, check := range set {
				names[check.Name()] = true
			}
		}
		for _, name := range checks.NoCallNames() {
			if !names[name] {
				t.Errorf("%s is listed as a no-call check but no harness has such a check", name)
			}
		}
	})
}
