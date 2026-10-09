package checks_test

import (
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/verdict"
)

const (
	forkContext    = "codex-fork-context"
	childOfSession = "rollout-2026-10-07T21-05-15-01a11334-f9a0-7c11-8e02-1b3f0c2d4e55.jsonl"
	taskDelivery   = `"type":"agent_message"`
	spawnDecider   = "executor_spawn_blind"
	// firstSpawnCall is the call id of the session's first spawn, which sets no
	// history control.
	firstSpawnCall = "call_Zz00Yy11"
	// refusalOutput is the output the harness gave the first spawn.
	refusalOutput = `"output":"an executor must start with none of your history: call spawn_agent again for spec-proof-dp-decider with the same arguments, fork_turns set to \"none\" and no fork_context"`
	// parentPrompt is the prompt of the main agent, as an inherited thread shows it.
	parentPrompt = `{"timestamp":"2026-10-07T21:05:15.955Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"scripted prompt"}]}}`
)

func withChild(files evidence.Files, edit func([]byte) []byte) evidence.Files {
	files.RolloutChildren = map[string][]byte{childOfSession: edit(files.RolloutChildren[childOfSession])}
	return files
}

func TestExecutorSpawnIsBlind(t *testing.T) {
	t.Run("a first spawn with no history control that the pre-tool hook denied, followed by a spawn whose thread started blind, passes and points at the denial [verifies SP-1511~3]", func(t *testing.T) {
		v := want(t, evaluate(t, "codex", readFiles(t, codexSession)), spawnDecider, verdict.Pass)
		if len(v.Evidence) < 3 || v.Evidence[0].File != evidence.HooksFile || v.Evidence[1].File != evidence.RolloutMainFile || v.Evidence[2].File != evidence.RolloutChildrenDir+"/"+childOfSession {
			t.Errorf("evidence = %+v", v.Evidence)
		}
		for _, part := range []string{
			"the module's pre-tool hook denied the first spawn of spec-proof-dp-decider, which set no blind form, and the spawn that followed started blind",
			"of 2 spawn(s) of spec-proof-dp-decider, the module's pre-tool hook received 1, 1 set a blind form, 1 that set none were denied by the pre-tool hook and 0 that set none were not denied",
			"of 1 thread(s) of spec-proof-dp-decider, 1 started blind, 0 inherited history and 0 cannot show it",
			"agent_id, agent_type, hook_event_name, session_id", "none of them says how much history the child inherited",
		} {
			if !strings.Contains(v.Summary, part) {
				t.Errorf("the summary lacks %q: %s", part, v.Summary)
			}
		}
	})

	t.Run("a recorded session whose first spawn set fork_context false and whose thread started blind is inconclusive, never a failure [verifies SP-1511~3]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		recorded := readFiles(t, forkContext)
		files.RolloutMain, files.RolloutChildren = recorded.RolloutMain, recorded.RolloutChildren
		v := want(t, evaluate(t, "codex", files), spawnDecider, verdict.Inconclusive)
		if !strings.Contains(v.Summary, "already set a blind form") || !strings.Contains(v.Summary, "1 started blind") {
			t.Errorf("summary = %s", v.Summary)
		}
	})

	t.Run("a first spawn with no history control that no denial of the hook answered is inconclusive [verifies SP-1511~3]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.Hooks = dropLines(files.Hooks, firstSpawnCall)
		v := want(t, evaluate(t, "codex", files), spawnDecider, verdict.Inconclusive)
		if !strings.Contains(v.Summary, "no denial of it by the module's pre-tool hook is recorded") || !strings.Contains(v.Summary, "received 0") {
			t.Errorf("summary = %s", v.Summary)
		}
	})

	t.Run("a spawn with no history control that started a thread fails [verifies SP-1511~3]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.RolloutMain = replace(files.RolloutMain, refusalOutput, `"output":"{\"task_name\":\"/root/decide_first\"}"`)
		v := want(t, evaluate(t, "codex", files), spawnDecider, verdict.Fail)
		if !strings.Contains(v.Summary, "sets no blind form and started thread /root/decide_first") || v.Evidence[0].Line != 3 {
			t.Errorf("verdict = %+v", v)
		}
	})

	t.Run("a thread whose rollout shows the main agent's prompt before its task fails, whatever its spawn wrote [verifies SP-1511~3]", func(t *testing.T) {
		files := withChild(readFiles(t, codexSession), func(child []byte) []byte {
			lines := strings.SplitAfterN(string(child), "\n", 5)
			return []byte(lines[0] + lines[1] + lines[2] + lines[3] + parentPrompt + "\n" + lines[4])
		})
		v := want(t, evaluate(t, "codex", files), spawnDecider, verdict.Fail)
		if !strings.Contains(v.Summary, "started with history of the main agent") || !strings.Contains(v.Summary, "precedes the task message") || v.Evidence[0].File != evidence.RolloutChildrenDir+"/"+childOfSession {
			t.Errorf("verdict = %+v", v)
		}
		if len(v.Evidence) != 2 || v.Evidence[1].File != evidence.RolloutMainFile || v.Evidence[1].Line != 5 {
			t.Errorf("the verdict does not point at the spawn that started the thread: %+v", v.Evidence)
		}
	})

	t.Run("a thread whose rollout holds no task message is inconclusive [verifies SP-1511~3]", func(t *testing.T) {
		files := withChild(readFiles(t, codexSession), func(child []byte) []byte { return dropLines(dropLines(child, taskDelivery), `"function_call"`) })
		v := want(t, evaluate(t, "codex", files), spawnDecider, verdict.Inconclusive)
		if !strings.Contains(v.Summary, "cannot show the history it started with") {
			t.Errorf("summary = %s", v.Summary)
		}
	})

	t.Run("a denied first spawn with no thread recorded after it is inconclusive [verifies SP-1511~3]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.RolloutChildren = nil
		v := want(t, evaluate(t, "codex", files), spawnDecider, verdict.Inconclusive)
		if !strings.Contains(v.Summary, "no rollout of a thread started after it was recorded") || v.Evidence[0].File != evidence.RolloutMainFile {
			t.Errorf("verdict = %+v", v)
		}
	})

	t.Run("a thread whose spawns no rollout recorded is inconclusive [verifies SP-1511~3]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.RolloutMain = nil
		v := want(t, evaluate(t, "codex", files), spawnDecider, verdict.Inconclusive)
		if !strings.Contains(v.Summary, "no rollout recorded the spawns that started it") {
			t.Errorf("summary = %s", v.Summary)
		}
	})

	t.Run("a spawn of another agent type is not the executor's [verifies SP-1511~3]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.RolloutMain = replace(files.RolloutMain, `spec-proof-dp-decider`, `explorer`)
		files.RolloutChildren = nil
		want(t, evaluate(t, "codex", files), spawnDecider, verdict.Inconclusive)
	})

	t.Run("no spawn of the executor is inconclusive, and the summary says so [verifies SP-1511~3]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.RolloutMain, files.RolloutChildren = nil, nil
		files.Stream = dropLines(files.Stream, "collab_tool_call")
		v := want(t, evaluate(t, "codex", files), spawnDecider, verdict.Inconclusive)
		if !strings.Contains(v.Summary, "spawned no executor agent") {
			t.Errorf("summary = %s", v.Summary)
		}
	})

	t.Run("a session with no SubagentStart payload says so and still concludes [verifies SP-1511~3]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.Hooks = dropLines(files.Hooks, `"subagent-start"`)
		v := want(t, evaluate(t, "codex", files), spawnDecider, verdict.Pass)
		if !strings.Contains(v.Summary, "no SubagentStart payload was recorded") {
			t.Errorf("summary = %s", v.Summary)
		}
	})

	t.Run("a start payload that names a fork member is reported as possibly telling the depth [verifies SP-1511~3]", func(t *testing.T) {
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

func TestSpawnGuardIsNotTheToolGuard(t *testing.T) {
	t.Run("a pre-tool call on the spawn tool alone does not show that the guard of the executor tools ran [verifies SP-1516~1]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.Hooks = dropLines(files.Hooks, "fetch_request")
		files.Stream = dropLines(files.Stream, "mcp_tool_call")
		v := want(t, evaluate(t, "codex", files), "hooks_fire", verdict.Inconclusive)
		if !strings.Contains(v.Summary, "the pre-tool hook never ran") {
			t.Errorf("summary = %s", v.Summary)
		}
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
