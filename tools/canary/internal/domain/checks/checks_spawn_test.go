package checks_test

import (
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/verdict"
)

const (
	noForkTurns   = `,\"fork_turns\":\"none\"`
	forkTurnsNone = `\"fork_turns\":\"none\"`
)

func TestExecutorSpawnIsBlind(t *testing.T) {
	const check = "executor_spawn_blind"

	t.Run("a spawn that sets fork_turns to none passes and points at the call, and the report says what the start hook receives [verifies SP-1511~1]", func(t *testing.T) {
		v := want(t, evaluate(t, "codex", readFiles(t, codexSession)), check, verdict.Pass)
		if len(v.Evidence) == 0 || v.Evidence[0].File != evidence.RolloutMainFile || v.Evidence[0].Line != 3 {
			t.Errorf("evidence = %+v", v.Evidence)
		}
		for _, part := range []string{`"none"`, "agent_id, agent_type, hook_event_name, session_id", "none of them says how much history the child inherited"} {
			if !strings.Contains(v.Summary, part) {
				t.Errorf("the summary lacks %q: %s", part, v.Summary)
			}
		}
	})

	t.Run("a spawn that omits fork_turns, or forks all of the history or some turns of it, fails [verifies SP-1511~1]", func(t *testing.T) {
		for name, replacement := range map[string]string{"omitted": "", "all": `\"fork_turns\":\"all\"`, "a number": `\"fork_turns\":4`} {
			files := readFiles(t, codexSession)
			if replacement == "" {
				files.RolloutMain = replace(files.RolloutMain, noForkTurns, "")
			} else {
				files.RolloutMain = replace(files.RolloutMain, forkTurnsNone, replacement)
			}
			v := want(t, evaluate(t, "codex", files), check, verdict.Fail)
			if !strings.Contains(v.Summary, "inherits") || v.Evidence[0].Line != 3 {
				t.Errorf("%s: verdict = %+v", name, v)
			}
		}
	})

	t.Run("one spawn of the executor that is not blind fails the check whatever the others did [verifies SP-1511~1]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		extra := strings.Replace(string(files.RolloutMain), "decide_ticket", "decide_again", 1)
		extra = strings.Replace(extra, noForkTurns, "", 1)
		extraLine := strings.SplitN(strings.SplitAfterN(extra, "\n", 4)[2], "\n", 2)[0]
		files.RolloutMain = append(files.RolloutMain, []byte(extraLine+"\n")...)
		want(t, evaluate(t, "codex", files), check, verdict.Fail)
	})

	t.Run("a spawn of another agent type is not the executor's [verifies SP-1511~1]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.RolloutMain = replace(files.RolloutMain, noForkTurns, "")
		files.RolloutMain = replace(files.RolloutMain, `spec-proof-dp-decider`, `explorer`)
		files.RolloutChildren = nil
		want(t, evaluate(t, "codex", files), check, verdict.Inconclusive)
	})

	t.Run("no spawn of the executor is inconclusive, and the summary says so [verifies SP-1511~1]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.RolloutMain, files.RolloutChildren = nil, nil
		files.Stream = dropLines(files.Stream, "collab_tool_call")
		v := want(t, evaluate(t, "codex", files), check, verdict.Inconclusive)
		if !strings.Contains(v.Summary, "spawned no executor agent") {
			t.Errorf("summary = %s", v.Summary)
		}
	})

	t.Run("a spawn the stream shows but no rollout recorded is inconclusive, not a pass [verifies SP-1511~1]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.RolloutMain, files.RolloutChildren = nil, nil
		v := want(t, evaluate(t, "codex", files), check, verdict.Inconclusive)
		if !strings.Contains(v.Summary, "no rollout recorded the arguments of its spawn") {
			t.Errorf("summary = %s", v.Summary)
		}
	})

	t.Run("a session with no SubagentStart payload says so and still concludes from the spawn [verifies SP-1511~1]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.Hooks = dropLines(files.Hooks, `"subagent-start"`)
		v := want(t, evaluate(t, "codex", files), check, verdict.Pass)
		if !strings.Contains(v.Summary, "no SubagentStart payload was recorded") {
			t.Errorf("summary = %s", v.Summary)
		}
	})

	t.Run("a start payload that names a fork member is reported as possibly telling the depth [verifies SP-1511~1]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.Hooks = replace(files.Hooks, `"hook_event_name":"SubagentStart"`, `"hook_event_name":"SubagentStart","fork_turns":"none"`)
		v := want(t, evaluate(t, "codex", files), check, verdict.Pass)
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
