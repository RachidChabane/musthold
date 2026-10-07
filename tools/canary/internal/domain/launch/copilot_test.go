package launch_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/launch"
)

func copilotInput() launch.Input {
	in := input()
	in.Copilot = launch.CopilotDirs{Home: "/run/copilot-home", Cache: "/run/copilot-cache", Logs: "/run/copilot-logs"}
	return in
}

func TestCopilotCommand(t *testing.T) {
	command, err := launch.Build("copilot", copilotInput())
	if err != nil {
		t.Fatal(err)
	}

	t.Run("the session is one non-interactive json run on the pinned model in the temporary project [verifies SP-1708~1]", func(t *testing.T) {
		argv := command.Argv
		if argv[0] != "copilot" || !slices.Contains(argv, "-p") {
			t.Fatalf("argv = %v", argv)
		}
		for flag, want := range map[string]string{"--model": "claude-haiku-4.5", "--output-format": "json", "-C": "/run/project", "--log-dir": "/run/copilot-logs", "--max-ai-credits": launch.CopilotMaxAICredits} {
			if got := flagValue(argv, flag); len(got) != 1 || got[0] != want {
				t.Errorf("%s = %v, want %s", flag, got, want)
			}
		}
		for _, wanted := range []string{"--allow-all-tools", "--no-ask-user", "--no-auto-update", "--no-custom-instructions"} {
			if !slices.Contains(argv, wanted) {
				t.Errorf("the command lacks %s", wanted)
			}
		}
		if command.Dir != "/run/project" {
			t.Errorf("dir = %s", command.Dir)
		}
	})

	t.Run("the portable plugin of every module loads through --plugin-dir [verifies SP-1708~1]", func(t *testing.T) {
		got := flagValue(command.Argv, "--plugin-dir")
		want := []string{"/repo/discpower/plugin/portable", "/repo/alignment/plugin/portable", "/repo/completeness/plugin/portable"}
		if !slices.Equal(got, want) {
			t.Errorf("plugin dirs %v, want %v", got, want)
		}
	})

	t.Run("nothing hides the events and nothing trusts the working directory or approves everything [verifies SP-1708~1]", func(t *testing.T) {
		for _, forbidden := range []string{"--silent", "-s", "--allow-all", "--yolo", "--allow-all-paths", "--allow-all-urls", "--share-gist", "--experimental"} {
			if slices.Contains(command.Argv, forbidden) {
				t.Errorf("the command passes %s", forbidden)
			}
		}
	})

	t.Run("the state of the session is in the run, and the shared home sets no COPILOT_HOME [verifies SP-1708~1]", func(t *testing.T) {
		want := []string{"COPILOT_HOME=/run/copilot-home", "COPILOT_CACHE_HOME=/run/copilot-cache", "COPILOT_AUTO_UPDATE=false"}
		if !slices.Equal(command.Env, want) {
			t.Errorf("env = %v, want %v", command.Env, want)
		}
		shared := copilotInput()
		shared.Copilot.Home = ""
		withShared, _ := launch.Build("copilot", shared)
		if !slices.Equal(withShared.Env, want[1:]) {
			t.Errorf("shared env = %v", withShared.Env)
		}
	})

	t.Run("the variables that trust everything, override the model or route to another provider are dropped, the tokens are not [verifies SP-1708~1]", func(t *testing.T) {
		for _, name := range []string{"COPILOT_ALLOW_ALL", "COPILOT_MODEL", "COPILOT_PROVIDER_BASE_URL", "COPILOT_PROVIDER_API_KEY"} {
			if !launch.DropsCopilotVariable(name) {
				t.Errorf("%s reaches the session", name)
			}
		}
		for _, name := range append([]string{"HOME", "PATH", "COPILOT_HOME"}, launch.CopilotTokenVariables...) {
			if launch.DropsCopilotVariable(name) {
				t.Errorf("%s is dropped", name)
			}
		}
	})

	t.Run("the no-call probes mount the same trees and never run a model [verifies SP-1708~1]", func(t *testing.T) {
		for _, subject := range []string{"plugin", "skill"} {
			argv := launch.CopilotListArgv("/repo", "/run/copilot-logs", subject)
			if !slices.Equal(flagValue(argv, "--plugin-dir"), flagValue(command.Argv, "--plugin-dir")) || slices.Contains(argv, "-p") {
				t.Errorf("argv = %v", argv)
			}
			if !strings.HasSuffix(strings.Join(argv, " "), subject+" list --json") || !slices.Contains(argv, "--no-auto-update") {
				t.Errorf("argv = %v", argv)
			}
		}
		version := launch.CopilotVersionArgv("/run/copilot-logs")
		if !slices.Contains(version, "--no-auto-update") {
			t.Error("the version probe may update the CLI")
		}
		for _, argv := range [][]string{version, launch.CopilotListArgv("/repo", "/run/copilot-logs", "plugin")} {
			if got := flagValue(argv, "--log-dir"); len(got) != 1 || got[0] != "/run/copilot-logs" {
				t.Errorf("a probe logs to %v, not into the run: %v", got, argv)
			}
		}
	})
}

func TestCopilotPrompt(t *testing.T) {
	t.Run("the Copilot prompt names the agent type as the harness spells it, asks for a shell command and one edit [verifies SP-1708~1]", func(t *testing.T) {
		prompt := launch.CopilotPrompt(role, ticket)
		for _, part := range []string{`"spec-proof-dp-decider"`, "touch " + launch.SandboxProbeFile, launch.EditFile, launch.EditFrom, launch.EditTo} {
			if !strings.Contains(prompt, part) {
				t.Errorf("the prompt lacks %q", part)
			}
		}
		if found, err := hostproto.FindTicketID(prompt); err != nil || found.String() != ticket {
			t.Errorf("the prompt holds ticket %v, %v", found, err)
		}
	})
}

func TestCopilotEventCap(t *testing.T) {
	t.Run("the complete assistant messages and the tool starts count towards the cap, the deltas and the rest do not [verifies SP-1505~1]", func(t *testing.T) {
		for line, counts := range map[string]bool{
			`{"type":"assistant.message","data":{}}`:       true,
			`{"type":"tool.execution_start","data":{}}`:    true,
			`{"type":"tool.execution_complete","data":{}}`: false,
			`{"type":"assistant.message_delta","data":{}}`: false,
			`{"type":"session.idle"}`:                      false,
			`not json`:                                     false,
		} {
			if got := launch.CountsAsEvent("copilot", []byte(line)); got != counts {
				t.Errorf("%s: counts %v, want %v", line, got, counts)
			}
		}
		if launch.EventCap("copilot") <= 0 || launch.Model("copilot") != launch.CopilotModel {
			t.Error("Copilot has no cap or no pinned model")
		}
		if launch.Model("claude") != launch.ClaudeModel || launch.Model("codex") != launch.CodexModel {
			t.Error("another harness runs on the wrong model")
		}
	})
}
