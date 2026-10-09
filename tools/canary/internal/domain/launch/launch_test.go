package launch_test

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/launch"
)

const (
	ticket = "864f5e696da12d6944d9380733d5c5a9fba0f0140f3b243c3efae9a6cc74cb38.0.1"
	role   = "spec-proof-dp:decider"
)

func input() launch.Input {
	return launch.Input{RepoRoot: "/repo", RunDir: "/run", Ticket: ticket, Role: role}
}

func flagValue(argv []string, flag string) []string {
	var values []string
	for index, word := range argv {
		if word == flag && index+1 < len(argv) {
			values = append(values, argv[index+1])
		}
	}
	return values
}

func TestClaudeCommand(t *testing.T) {
	command, err := launch.Build("claude", input())
	if err != nil {
		t.Fatal(err)
	}

	t.Run("the session is one non-interactive stream-json run on the pinned model [verifies SP-1504~2]", func(t *testing.T) {
		argv := command.Argv
		if argv[0] != "claude" || !slices.Contains(argv, "-p") || !slices.Contains(argv, "--verbose") {
			t.Fatalf("argv = %v", argv)
		}
		for flag, want := range map[string]string{"--model": "claude-sonnet-5-5", "--output-format": "stream-json", "--setting-sources": "project", "--permission-mode": "acceptEdits"} {
			if got := flagValue(argv, flag); len(got) != 1 || got[0] != want {
				t.Errorf("%s = %v, want %s", flag, got, want)
			}
		}
		if command.Dir != "/run/project" {
			t.Errorf("dir = %s", command.Dir)
		}
	})

	t.Run("the plugin of every module loads from its generated tree and nothing hides the hooks [verifies SP-1504~2]", func(t *testing.T) {
		dirs := flagValue(command.Argv, "--plugin-dir")
		want := []string{"/repo/discpower/plugin/claude", "/repo/alignment/plugin/claude", "/repo/completeness/plugin/claude"}
		if !slices.Equal(dirs, want) {
			t.Errorf("plugin dirs %v, want %v", dirs, want)
		}
		for _, forbidden := range []string{"--bare", "--dangerously-skip-permissions", "--allowedTools", "--allowed-tools", "--strict-mcp-config", "--no-session-persistence"} {
			if slices.Contains(command.Argv, forbidden) {
				t.Errorf("the command passes %s, which would mask what the canary checks", forbidden)
			}
		}
		for _, wanted := range []string{"--include-hook-events", "--forward-subagent-text"} {
			if !slices.Contains(command.Argv, wanted) {
				t.Errorf("the command lacks %s", wanted)
			}
		}
	})

	t.Run("the spend is bounded and the user's configuration is left alone unless a directory is given [verifies SP-1505~1]", func(t *testing.T) {
		if got := flagValue(command.Argv, "--max-budget-usd"); len(got) != 1 || got[0] != launch.ClaudeBudgetUSD {
			t.Errorf("budget = %v", got)
		}
		if !slices.Equal(command.Env, launch.RawLogEnvironment("/run/bodies")) {
			t.Errorf("env = %v", command.Env)
		}
		isolated := input()
		isolated.ClaudeConfigDir = "/run/claude-config"
		withConfig, _ := launch.Build("claude", isolated)
		if !slices.Equal(withConfig.Env, append(launch.RawLogEnvironment("/run/bodies"), "CLAUDE_CONFIG_DIR=/run/claude-config")) {
			t.Errorf("env = %v", withConfig.Env)
		}
	})

	t.Run("the session writes its raw API log into the run directory and nowhere else [verifies SP-1925~1]", func(t *testing.T) {
		want := []string{"CLAUDE_CODE_ENABLE_TELEMETRY=1", "OTEL_LOGS_EXPORTER=none", "OTEL_METRICS_EXPORTER=none", "OTEL_LOG_RAW_API_BODIES=file:/run/bodies"}
		if got := launch.RawLogEnvironment(input().Bodies()); !slices.Equal(got, want) {
			t.Errorf("environment = %v", got)
		}
	})

	t.Run("the project classifies the completeness requests in shadow, no other module's, and names the raw API log [verifies SP-1925~1]", func(t *testing.T) {
		fixture := "proofs:\n  alignment: false\n  discriminating-power: true\n  completeness: false\n"
		got := launch.ClassifyConfig(fixture, "/run/bodies")
		for _, wanted := range []string{"completeness: true", "default: off", "completeness: shadow", "witness_dir: /run/bodies"} {
			if !strings.Contains(got, wanted) {
				t.Errorf("the configuration lacks %q:\n%s", wanted, got)
			}
		}
		if strings.Contains(got, "completeness: false") {
			t.Errorf("the completeness proof stays off:\n%s", got)
		}
	})

	t.Run("the prompt runs and resumes the completeness proof after the edit [verifies SP-1925~1]", func(t *testing.T) {
		prompt := launch.ClaudePrompt("spec-proof-dp:decider", "ticket")
		run, resume := strings.Index(prompt, "spec-proof-completeness prove --all"), strings.Index(prompt, "spec-proof-completeness prove --resume")
		if run < 0 || resume < run || run < strings.Index(prompt, launch.EditFile) {
			t.Errorf("the steps are out of order:\n%s", prompt)
		}
	})
}

func TestCodexCommand(t *testing.T) {
	command, err := launch.Build("codex", input())
	if err != nil {
		t.Fatal(err)
	}

	t.Run("the session is one json exec run on the pinned model in the temporary project [verifies SP-1504~2]", func(t *testing.T) {
		argv := command.Argv
		if !slices.Equal(argv[:2], []string{"codex", "exec"}) || flagValue(argv, "-m")[0] != "gpt-6-luna" || !slices.Contains(argv, "--json") {
			t.Fatalf("argv = %v", argv)
		}
		if flagValue(argv, "-C")[0] != "/run/project" {
			t.Errorf("working root %v", flagValue(argv, "-C"))
		}
	})

	t.Run("the user's configuration is read, since the harness disables its hooks without it, and rules and hook trust are not [verifies SP-1504~2]", func(t *testing.T) {
		for _, flag := range []string{"--ignore-rules", "--dangerously-bypass-hook-trust", "--skip-git-repo-check"} {
			if !slices.Contains(command.Argv, flag) {
				t.Errorf("the command lacks %s", flag)
			}
		}
		if slices.Contains(command.Argv, "--ignore-user-config") {
			t.Error("the command ignores the user configuration, which disables every hook of the session")
		}
		for _, enabled := range []string{"hooks", "multi_agent"} {
			if !slices.Contains(flagValue(command.Argv, "--enable"), enabled) {
				t.Errorf("the command does not enable %s", enabled)
			}
		}
		if slices.Contains(command.Argv, "--dangerously-bypass-approvals-and-sandbox") {
			t.Error("the command bypasses the sandbox")
		}
		trust := strings.Join(flagValue(command.Argv, "-c"), "\n")
		if !strings.Contains(trust, `projects."/run/project".trust_level="trusted"`) {
			t.Errorf("no project trust override in %s", trust)
		}
		if len(command.Env) != 0 {
			t.Errorf("env = %v", command.Env)
		}
		home := input()
		home.CodexHome = "/run/codex-home"
		withHome, _ := launch.Build("codex", home)
		if !slices.Equal(withHome.Env, []string{"CODEX_HOME=/run/codex-home"}) {
			t.Errorf("env = %v", withHome.Env)
		}
	})

	t.Run("an unknown harness has no command [verifies SP-1504~2]", func(t *testing.T) {
		if _, err := launch.Build("gemini", input()); err == nil {
			t.Fatal("gemini was built")
		}
	})
}

func TestPrompts(t *testing.T) {
	t.Run("the padded launch prompt holds exactly one ticket, so the hook accepts and rewrites it [verifies SP-1504~2]", func(t *testing.T) {
		found, err := hostproto.FindTicketID(launch.PaddedLaunchPrompt(ticket))
		if err != nil || found.String() != ticket {
			t.Fatalf("ticket %v, %v", found, err)
		}
		if launch.PaddedLaunchPrompt(ticket) == "ticket "+ticket {
			t.Error("the prompt carries no padding to remove")
		}
	})

	t.Run("the Claude prompt scripts the fetch, the launch with a model, the edit and the end [verifies SP-1504~2]", func(t *testing.T) {
		prompt := launch.ClaudePrompt(role, ticket)
		for _, part := range []string{"mcp__plugin_spec-proof-dp_exec__fetch_request", `model "haiku"`, role, launch.EditFile, launch.EditFrom, launch.EditTo, "foreground"} {
			if !strings.Contains(prompt, part) {
				t.Errorf("the prompt lacks %q", part)
			}
		}
	})

	t.Run("the Codex prompt names the agent type as the harness spells it and asks for a write [verifies SP-1504~2]", func(t *testing.T) {
		prompt := launch.CodexPrompt(role, ticket)
		for _, part := range []string{`"spec-proof-dp-decider"`, "touch " + launch.SandboxProbeFile, launch.EditFrom} {
			if !strings.Contains(prompt, part) {
				t.Errorf("the prompt lacks %q", part)
			}
		}
	})

	t.Run("the Codex prompt names the spawn tool, its arguments and the wait literally and gives the executor the ticket line alone [verifies SP-1504~2]", func(t *testing.T) {
		prompt := launch.CodexPrompt(role, ticket)
		message := launch.ExecutorMessage(ticket)
		for _, part := range []string{
			"spawn_agent with agent_type \"spec-proof-dp-decider\", fork_turns \"none\" and message " + strconv.Quote(message),
			"call wait_agent", launch.EditFile,
		} {
			if !strings.Contains(prompt, part) {
				t.Errorf("the prompt lacks %q:\n%s", part, prompt)
			}
		}
		if found, err := hostproto.FindTicketID(prompt); err != nil || found.String() != ticket {
			t.Errorf("the prompt holds ticket %v, %v", found, err)
		}
	})
}

func TestEventCap(t *testing.T) {
	t.Run("only the model steps of each harness count towards the cap [verifies SP-1505~1]", func(t *testing.T) {
		for _, tc := range []struct {
			flag, line string
			counts     bool
		}{
			{"claude", `{"type":"assistant","message":{}}`, true},
			{"claude", `{"type":"user","message":{}}`, false},
			{"claude", `{"type":"system","subtype":"init"}`, false},
			{"claude", `not json`, false},
			{"codex", `{"type":"item.completed","item":{"type":"agent_message"}}`, true},
			{"codex", `{"type":"item.completed","item":{"type":"command_execution"}}`, true},
			{"codex", `{"type":"item.completed","item":{"type":"reasoning"}}`, false},
			{"codex", `{"type":"turn.started"}`, false},
		} {
			if got := launch.CountsAsEvent(tc.flag, []byte(tc.line)); got != tc.counts {
				t.Errorf("%s %s: counts %v, want %v", tc.flag, tc.line, got, tc.counts)
			}
		}
		if launch.EventCap("claude") <= 0 || launch.EventCap("codex") <= 0 {
			t.Error("a harness has no cap")
		}
	})
}

func TestCommandLine(t *testing.T) {
	t.Run("a command prints as a shell line that quotes what needs it and puts the environment first [verifies SP-1504~2]", func(t *testing.T) {
		command := launch.Command{Argv: []string{"claude", "-p", "it's here", "--model", "m"}, Env: []string{"A=b c"}}
		want := `A='b c' claude -p 'it'\''s here' --model m`
		if got := command.Line(); got != want {
			t.Errorf("line = %s, want %s", got, want)
		}
		if launch.Quote("") != "''" || launch.Quote("a/b-c_d.e") != "a/b-c_d.e" {
			t.Error("Quote mishandles empty or plain words")
		}
	})
}

type decodedClaudeSettings struct {
	Hooks map[string][]struct {
		Hooks []struct{ Command string } `json:"hooks"`
	} `json:"hooks"`
	Permissions struct {
		Allow []string `json:"allow"`
	} `json:"permissions"`
}

// claudeSettings decodes the settings the canary writes and fails on a key it does
// not expect.
func claudeSettings(t *testing.T) decodedClaudeSettings {
	t.Helper()
	settings, err := launch.ClaudeSettings("/bin/canary", "/run/records/hooks.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(settings, &keys); err != nil || len(keys) != 2 {
		t.Fatalf("settings = %s", settings)
	}
	var decoded decodedClaudeSettings
	if err := json.Unmarshal(settings, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestProjectFiles(t *testing.T) {
	t.Run("the Claude settings hold the Stop probe and the control probes of the events before the first tool call, and nothing else [verifies SP-1504~2]", func(t *testing.T) {
		decoded := claudeSettings(t)
		if len(decoded.Hooks) != 3 || len(decoded.Hooks["Stop"]) != 1 || len(decoded.Hooks["SessionStart"]) != 1 || len(decoded.Hooks["UserPromptSubmit"]) != 1 {
			t.Fatalf("hooks = %v", decoded.Hooks)
		}
		command := decoded.Hooks["Stop"][0].Hooks[0].Command
		for _, part := range []string{"/bin/canary", "probe", "--record", "/run/records/hooks.jsonl", "Stop-probe"} {
			if !strings.Contains(command, part) {
				t.Errorf("the probe command %q lacks %q", command, part)
			}
		}
	})

	t.Run("the Claude settings allow the run and the resume of the completeness proof and no other command [verifies SP-1925~1]", func(t *testing.T) {
		want := []string{"Bash(spec-proof-completeness prove --all)", "Bash(spec-proof-completeness prove --resume)"}
		if got := claudeSettings(t).Permissions.Allow; !slices.Equal(got, want) {
			t.Errorf("permissions.allow = %v, want %v", got, want)
		}
	})

	t.Run("the Codex probes are added to the entries the generated file holds and displace none [verifies SP-1504~2]", func(t *testing.T) {
		existing := `{"hooks":{"PreToolUse":[{"matcher":"mcp__x__.*","hooks":[{"type":"command","command":"x hook pre-tool"}]}],"SubagentStop":[{"hooks":[{"type":"command","command":"x hook stop"}]}]}}`
		merged, err := launch.MergeCodexProbes([]byte(existing), "/bin/canary", "/run/hooks.jsonl")
		if err != nil {
			t.Fatal(err)
		}
		var decoded struct {
			Hooks map[string][]struct {
				Matcher string `json:"matcher"`
			} `json:"hooks"`
		}
		if err := json.Unmarshal(merged, &decoded); err != nil {
			t.Fatal(err)
		}
		if got := decoded.Hooks["PreToolUse"]; len(got) != 2 || got[0].Matcher != "mcp__x__.*" || got[1].Matcher != ".*" {
			t.Errorf("PreToolUse = %+v", got)
		}
		if len(decoded.Hooks["Stop"]) != 1 || len(decoded.Hooks["SubagentStop"]) != 1 {
			t.Errorf("hooks = %+v", decoded.Hooks)
		}
		if _, err := launch.MergeCodexProbes([]byte("{"), "/c", "/h"); err == nil {
			t.Error("a corrupt hooks file was merged")
		}
		fresh, err := launch.MergeCodexProbes(nil, "/c", "/h")
		if err != nil || !strings.Contains(string(fresh), "Stop-probe") {
			t.Errorf("fresh = %s, %v", fresh, err)
		}
	})
}
