package checks_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/verdict"
)

const (
	forkTurnsNone  = `,\"fork_turns\":\"none\"`
	forkContext    = "codex-fork-context"
	childOfSession = "rollout-2026-10-07T21-05-15-01a11334-f9a0-7c11-8e02-1b3f0c2d4e55.jsonl"
	taskDelivery   = `"type":"agent_message"`
	spawnDecider   = "executor_spawn_blind"
	// deniedSpawn is a spawn of the decider that sets no blind form, its output,
	// and the module's pre-tool hook that denied it.
	deniedSpawn       = `{"timestamp":"2026-10-07T21:05:14.402Z","type":"response_item","payload":{"type":"function_call","name":"spawn_agent","arguments":"{\"task_name\":\"decide_first\",\"message\":\"gAAAAABqRk3sT0pBmX9u\",\"agent_type\":\"spec-proof-dp-decider\"}","call_id":"call_Zz00Yy11"}}`
	deniedSpawnOutput = `{"timestamp":"2026-10-07T21:05:14.410Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_Zz00Yy11","output":"an executor must start with none of your history"}}`
	deniedSpawnHook   = `{"module":"spec-proof-dp","args":["hook","pre-tool","--harness","codex"],"stdin":{"session_id":"01a11334-f5df-7a33-ba1e-3dd65b19d059","hook_event_name":"PreToolUse","tool_name":"spawn_agent","tool_use_id":"call_Zz00Yy11","tool_input":{"agent_type":"spec-proof-dp-decider","message":"ticket 864f5e696da12d6944d9380733d5c5a9fba0f0140f3b243c3efae9a6cc74cb38.0.1"}},"stdout":"{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"an executor must start with none of your history\"}}","exitCode":0}`
	// parentPrompt is the prompt of the main agent, as an inherited thread shows it.
	parentPrompt = `{"timestamp":"2026-10-07T21:05:15.955Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"scripted prompt"}]}}`
)

// withSpawnDeniedFirst puts before the session's spawn a spawn of the decider that
// sets no blind form, which the module's pre-tool hook denied.
func withSpawnDeniedFirst(t *testing.T, files evidence.Files) evidence.Files {
	t.Helper()
	lines := strings.SplitAfterN(string(files.RolloutMain), "\n", 3)
	files.RolloutMain = []byte(lines[0] + lines[1] + deniedSpawn + "\n" + deniedSpawnOutput + "\n" + lines[2])
	files.Hooks = append(files.Hooks, []byte(deniedSpawnHook+"\n")...)
	return files
}

func withChild(files evidence.Files, edit func([]byte) []byte) evidence.Files {
	files.RolloutChildren = map[string][]byte{childOfSession: edit(files.RolloutChildren[childOfSession])}
	return files
}

func TestExecutorSpawnIsBlind(t *testing.T) {
	t.Run("a thread of the executor whose rollout holds the harness context before its task passes and points at the rollout, and the report says what the spawns and the start hook show [verifies SP-1511~2]", func(t *testing.T) {
		v := want(t, evaluate(t, "codex", readFiles(t, codexSession)), spawnDecider, verdict.Pass)
		if len(v.Evidence) == 0 || v.Evidence[0].File != evidence.RolloutChildrenDir+"/"+childOfSession || v.Evidence[0].Line != 1 {
			t.Errorf("evidence = %+v", v.Evidence)
		}
		for _, part := range []string{
			"1 thread(s) of spec-proof-dp-decider started with none of the main agent's history",
			"of 1 spawn(s) of spec-proof-dp-decider, the module's pre-tool hook received 0, 1 set a blind form, 0 that set none were denied by the pre-tool hook and 0 that set none were not denied",
			"agent_id, agent_type, hook_event_name, session_id", "none of them says how much history the child inherited",
		} {
			if !strings.Contains(v.Summary, part) {
				t.Errorf("the summary lacks %q: %s", part, v.Summary)
			}
		}
	})

	t.Run("a recorded spawn with fork_context false whose thread started blind passes [verifies SP-1511~2]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		recorded := readFiles(t, forkContext)
		files.RolloutMain, files.RolloutChildren = recorded.RolloutMain, recorded.RolloutChildren
		v := want(t, evaluate(t, "codex", files), spawnDecider, verdict.Pass)
		if !strings.Contains(v.Summary, "1 set a blind form") || !strings.HasPrefix(v.Evidence[0].File, filepath.ToSlash(evidence.RolloutChildrenDir)+"/") {
			t.Errorf("verdict = %+v", v)
		}
	})

	t.Run("a spawn that sets no blind form, denied by the hook and followed by a blind spawn, passes and the report shows the denial [verifies SP-1511~2]", func(t *testing.T) {
		v := want(t, evaluate(t, "codex", withSpawnDeniedFirst(t, readFiles(t, codexSession))), spawnDecider, verdict.Pass)
		if !strings.Contains(v.Summary, "of 2 spawn(s) of spec-proof-dp-decider, the module's pre-tool hook received 1, 1 set a blind form, 1 that set none were denied by the pre-tool hook and 0 that set none were not denied") {
			t.Errorf("summary = %s", v.Summary)
		}
	})

	t.Run("a spawn that sets no blind form and was not denied is reported, and the thread still decides [verifies SP-1511~2]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.RolloutMain = replace(files.RolloutMain, forkTurnsNone, "")
		v := want(t, evaluate(t, "codex", files), spawnDecider, verdict.Pass)
		if !strings.Contains(v.Summary, "0 set a blind form, 0 that set none were denied by the pre-tool hook and 1 that set none were not denied") {
			t.Errorf("summary = %s", v.Summary)
		}
	})

	t.Run("a thread whose rollout shows the main agent's prompt before its task fails, whatever its spawn wrote [verifies SP-1511~2]", func(t *testing.T) {
		files := withChild(readFiles(t, codexSession), func(child []byte) []byte {
			lines := strings.SplitAfterN(string(child), "\n", 5)
			return []byte(lines[0] + lines[1] + lines[2] + lines[3] + parentPrompt + "\n" + lines[4])
		})
		v := want(t, evaluate(t, "codex", files), spawnDecider, verdict.Fail)
		if !strings.Contains(v.Summary, "started with history of the main agent") || !strings.Contains(v.Summary, "precedes the task message") || v.Evidence[0].File != evidence.RolloutChildrenDir+"/"+childOfSession {
			t.Errorf("verdict = %+v", v)
		}
		if len(v.Evidence) != 2 || v.Evidence[1].File != evidence.RolloutMainFile || v.Evidence[1].Line != 3 {
			t.Errorf("the verdict does not point at the spawn that started the thread: %+v", v.Evidence)
		}
	})

	t.Run("a thread whose rollout holds no task message is inconclusive [verifies SP-1511~2]", func(t *testing.T) {
		files := withChild(readFiles(t, codexSession), func(child []byte) []byte { return dropLines(dropLines(child, taskDelivery), `"function_call"`) })
		v := want(t, evaluate(t, "codex", files), spawnDecider, verdict.Inconclusive)
		if !strings.Contains(v.Summary, "cannot show the history it started with") {
			t.Errorf("summary = %s", v.Summary)
		}
	})

	t.Run("a spawn of another agent type is not the executor's [verifies SP-1511~2]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.RolloutMain = replace(files.RolloutMain, `spec-proof-dp-decider`, `explorer`)
		files.RolloutChildren = nil
		want(t, evaluate(t, "codex", files), spawnDecider, verdict.Inconclusive)
	})

	t.Run("no spawn of the executor is inconclusive, and the summary says so [verifies SP-1511~2]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.RolloutMain, files.RolloutChildren = nil, nil
		files.Stream = dropLines(files.Stream, "collab_tool_call")
		v := want(t, evaluate(t, "codex", files), spawnDecider, verdict.Inconclusive)
		if !strings.Contains(v.Summary, "spawned no executor agent") {
			t.Errorf("summary = %s", v.Summary)
		}
	})

	t.Run("a spawn whose thread left no rollout is inconclusive, not a pass [verifies SP-1511~2]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.RolloutChildren = nil
		v := want(t, evaluate(t, "codex", files), spawnDecider, verdict.Inconclusive)
		if !strings.Contains(v.Summary, "no rollout of its thread was recorded") || v.Evidence[0].File != evidence.RolloutMainFile {
			t.Errorf("verdict = %+v", v)
		}
	})

	t.Run("a session with no SubagentStart payload says so and still concludes from the thread [verifies SP-1511~2]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.Hooks = dropLines(files.Hooks, `"subagent-start"`)
		v := want(t, evaluate(t, "codex", files), spawnDecider, verdict.Pass)
		if !strings.Contains(v.Summary, "no SubagentStart payload was recorded") {
			t.Errorf("summary = %s", v.Summary)
		}
	})

	t.Run("a start payload that names a fork member is reported as possibly telling the depth [verifies SP-1511~2]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.Hooks = replace(files.Hooks, `"hook_event_name":"SubagentStart"`, `"hook_event_name":"SubagentStart","fork_turns":"none"`)
		v := want(t, evaluate(t, "codex", files), spawnDecider, verdict.Pass)
		if !strings.Contains(v.Summary, "member fork_turns may say how much history") {
			t.Errorf("summary = %s", v.Summary)
		}
	})
}

func TestExecutorStartsAreReadFromTheRollouts(t *testing.T) {
	t.Run("the spawn of the executor is found in the rollouts when the stream shows none [verifies SP-1510~1]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.Stream = dropLines(files.Stream, "collab_tool_call")
		files.Hooks = dropLines(files.Hooks, `"subagent-start"`)
		v := want(t, evaluate(t, "codex", files), "agent_files_load", verdict.Inconclusive)
		if !strings.Contains(v.Summary, "no SubagentStart hook recorded it") || v.Evidence[0].File != evidence.RolloutMainFile {
			t.Errorf("verdict = %+v", v)
		}
	})

	t.Run("a thread whose role is the executor's counts as its start even when the main rollout is missing [verifies SP-1510~1]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.Stream = dropLines(files.Stream, "collab_tool_call")
		files.RolloutMain = nil
		files.SandboxProbe = replace(files.SandboxProbe, `"exists": false`, `"exists": true`)
		v := want(t, evaluate(t, "codex", files), "executor_sandbox", verdict.Fail)
		if !strings.Contains(v.Summary, "the executor wrote") {
			t.Errorf("summary = %s", v.Summary)
		}
	})

	t.Run("a failed start the stream reports fails the agent files check even when the rollouts show the spawn [verifies SP-1510~1]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.Hooks = dropLines(files.Hooks, `"subagent-start"`)
		files.Stream = replace(files.Stream, `"status":"completed","sender_thread_id"`, `"status":"failed","sender_thread_id"`)
		want(t, evaluate(t, "codex", files), "agent_files_load", verdict.Fail)
	})
}

func TestConfigurationWrittenBySession(t *testing.T) {
	const check = "user_config_untouched"
	withWrites := func(t *testing.T, mode string, writes string, attributed string) evidence.Files {
		t.Helper()
		files := readFiles(t, codexSession)
		files.Touched = []byte(`{"roots":["/home/u/.codex"],"attributed":[` + attributed + `],"unattributed":[],"configWrites":[` + writes + `]}`)
		files.Session = replace(files.Session, `"configMode": "dedicated"`, `"configMode": "`+mode+`"`)
		return files
	}
	const restored = `{"path":"/home/u/.codex/config.toml","table":"[projects.\"/run/project\"]\ntrust_level = \"trusted\"\n","removed":true,"verified":true}`
	const configPath = `"/home/u/.codex/config.toml"`

	t.Run("a table the runner removed and a re-read confirmed passes, saying the session wrote it and the runner removed it [verifies SP-1512~1]", func(t *testing.T) {
		for _, mode := range []string{"dedicated", "shared"} {
			v := want(t, evaluate(t, "codex", withWrites(t, mode, restored, configPath)), check, verdict.Pass)
			for _, part := range []string{"the session wrote 1 table(s) naming its run", "/home/u/.codex/config.toml", "removed exactly those", "re-read confirmed"} {
				if !strings.Contains(v.Summary, part) {
					t.Errorf("%s: the summary lacks %q: %s", mode, part, v.Summary)
				}
			}
		}
	})

	t.Run("a table that was not removed, or whose removal no re-read confirmed, fails [verifies SP-1512~1]", func(t *testing.T) {
		for name, write := range map[string]string{
			"not removed":    `{"path":"/home/u/.codex/config.toml","table":"","removed":false,"verified":false}`,
			"not re-checked": `{"path":"/home/u/.codex/config.toml","table":"t","removed":true,"verified":false}`,
		} {
			v := want(t, evaluate(t, "codex", withWrites(t, "dedicated", write, configPath)), check, verdict.Fail)
			if !strings.Contains(v.Summary, "/home/u/.codex/config.toml") || !strings.Contains(v.Summary, "not removed and verified") {
				t.Errorf("%s: summary = %s", name, v.Summary)
			}
		}
	})

	t.Run("an entry that names the run beside the restored file still fails an isolated session and leaves a shared one open [verifies SP-1512~1]", func(t *testing.T) {
		attributed := configPath + `,"/home/u/.codex/sessions/run-1/x"`
		v := want(t, evaluate(t, "codex", withWrites(t, "dedicated", restored, attributed)), check, verdict.Fail)
		if !strings.Contains(v.Summary, "sessions/run-1/x") || strings.Contains(v.Summary, "config.toml,") {
			t.Errorf("summary = %s", v.Summary)
		}
		want(t, evaluate(t, "codex", withWrites(t, "shared", restored, attributed)), check, verdict.Inconclusive)
	})

	t.Run("a session that wrote nothing keeps the verdicts it had [verifies SP-1512~1]", func(t *testing.T) {
		want(t, evaluate(t, "codex", withWrites(t, "dedicated", "", "")), check, verdict.Pass)
		want(t, evaluate(t, "codex", withWrites(t, "shared", "", "")), check, verdict.Inconclusive)
	})
}
