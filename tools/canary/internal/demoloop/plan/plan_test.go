package plan_test

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/launch"
)

var layout = plan.Layout{Repo: "/repo", Run: "/runs/r1"}

func argvs(steps []plan.Step) []string {
	var lines []string
	for _, step := range steps {
		lines = append(lines, strings.Join(step.Argv, " "))
	}
	return lines
}

func TestEquipSteps(t *testing.T) {
	t.Run("the traceability arm installs Shallnot alone [verifies SP-2401~1]", func(t *testing.T) {
		if got := argvs(plan.EquipSteps(layout, config.ArmTraceability)); !slices.Equal(got, []string{"/repo/.tools/bin/shallnot init"}) {
			t.Errorf("steps = %v", got)
		}
	})

	t.Run("the four-proofs arm runs init then enable of each module, in order, after Shallnot [verifies SP-2401~1]", func(t *testing.T) {
		want := []string{
			"/repo/.tools/bin/shallnot init",
			"/repo/.tools/bin/spec-proof-dp init", "/repo/.tools/bin/spec-proof-dp enable",
			"/repo/.tools/bin/spec-proof-alignment init", "/repo/.tools/bin/spec-proof-alignment enable",
			"/repo/.tools/bin/spec-proof-completeness init", "/repo/.tools/bin/spec-proof-completeness enable",
		}
		if got := argvs(plan.EquipSteps(layout, config.ArmFourProofs)); !slices.Equal(got, want) {
			t.Errorf("steps = %v", got)
		}
	})

	t.Run("the commits of the preparation carry a fixed identity and date and read no user configuration [verifies SP-2401~1]", func(t *testing.T) {
		env := strings.Join(plan.GitEnvironment(), " ")
		for _, wanted := range []string{"GIT_AUTHOR_DATE=2026-01-01T00:00:00+00:00", "GIT_COMMITTER_NAME=demo", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null"} {
			if !strings.Contains(env, wanted) {
				t.Errorf("the environment lacks %s: %s", wanted, env)
			}
		}
	})
}

func TestBindOperations(t *testing.T) {
	operations := map[string]map[string]string{"zeta": {"python": "pkg.mod:zeta"}, "alpha": {"python": "pkg.mod:alpha", "go": "example.com/pkg:Alpha"}}

	t.Run("the operations are appended in alphabetical order with their targets quoted [verifies SP-2401~1]", func(t *testing.T) {
		got, err := plan.BindOperations([]byte("proofs:\n  alignment: true\n# operations:\n#   x: y"), operations)
		if err != nil {
			t.Fatal(err)
		}
		want := "proofs:\n  alignment: true\n# operations:\n#   x: y\noperations:\n  alpha:\n    go: \"example.com/pkg:Alpha\"\n    python: \"pkg.mod:alpha\"\n  zeta:\n    python: \"pkg.mod:zeta\"\n"
		if string(got) != want {
			t.Errorf("got\n%s\nwant\n%s", got, want)
		}
	})

	t.Run("a configuration that already binds operations is refused [verifies SP-2401~1]", func(t *testing.T) {
		if _, err := plan.BindOperations([]byte("operations:\n  x:\n    python: a:b\n"), operations); !errors.Is(err, plan.ErrOperationsBound) {
			t.Errorf("error = %v", err)
		}
	})
}

func TestSessionCommand(t *testing.T) {
	cfg := config.Config{Model: "claude-sonnet-5-5", Caps: config.Caps{AssistantMessages: 10, Seconds: 60, BudgetUSD: "3.00"}}
	run := config.Run{Task: config.Task{Name: "t", Prompt: "Do the task."}, Arm: config.ArmFourProofs, Repetition: 1}
	command := plan.SessionCommand(plan.SessionInput{Layout: layout, Run: run, Config: cfg, Claude: "/repo/.tools/claude-code/9.9.9/bin/claude"})
	line := strings.Join(command.Argv, " ")

	t.Run("one headless turn on the prompt and model, streamed with the hook events, the project's and the session's settings only [verifies SP-2402~1]", func(t *testing.T) {
		if command.Argv[0] != "/repo/.tools/claude-code/9.9.9/bin/claude" || command.Argv[1] != "-p" || command.Argv[2] != "Do the task." {
			t.Errorf("argv = %v", command.Argv)
		}
		for _, wanted := range []string{
			"--model claude-sonnet-5-5", "--output-format stream-json --verbose", "--include-hook-events",
			"--setting-sources project", "--settings /runs/r1/session-settings.json", "--permission-mode acceptEdits", "--max-budget-usd 3.00",
		} {
			if !strings.Contains(line, wanted) {
				t.Errorf("the command lacks %q: %s", wanted, line)
			}
		}
		if command.Dir != "/runs/r1/project" || !slices.Equal(command.Env, []string{"CLAUDE_CONFIG_DIR=/runs/r1/claude-config"}) {
			t.Errorf("dir %s, env %v", command.Dir, command.Env)
		}
	})

	t.Run("the four-proofs arm loads each module's plugin and the umbrella plugin, the traceability arm none [verifies SP-2402~1]", func(t *testing.T) {
		for _, dir := range []string{"/repo/discpower/plugin/claude", "/repo/alignment/plugin/claude", "/repo/completeness/plugin/claude", "/repo/.tools/plugins/spec-proof"} {
			if !strings.Contains(line, "--plugin-dir "+dir) {
				t.Errorf("the command lacks the plugin %s", dir)
			}
		}
		run.Arm = config.ArmTraceability
		bare := plan.SessionCommand(plan.SessionInput{Layout: layout, Run: run, Config: cfg, Claude: "claude"})
		if slices.Contains(bare.Argv, "--plugin-dir") {
			t.Errorf("the traceability arm loads plugins: %v", bare.Argv)
		}
		if !slices.Equal(plan.Shimmed(config.ArmTraceability), []string{"shallnot"}) || len(plan.Shimmed(config.ArmFourProofs)) != 4 {
			t.Errorf("shimmed %v / %v", plan.Shimmed(config.ArmTraceability), plan.Shimmed(config.ArmFourProofs))
		}
	})

	t.Run("the session settings hold the permission rules and nothing else [verifies SP-2402~1]", func(t *testing.T) {
		encoded, err := plan.SessionSettingsJSON([]string{"Bash(pytest *)"})
		if err != nil {
			t.Fatal(err)
		}
		var settings map[string]map[string][]string
		if err := json.Unmarshal(encoded, &settings); err != nil {
			t.Fatal(err)
		}
		if len(settings) != 1 || len(settings["permissions"]) != 1 || !slices.Equal(settings["permissions"]["allow"], []string{"Bash(pytest *)"}) {
			t.Errorf("settings = %s", encoded)
		}
	})
}

func TestEnvironment(t *testing.T) {
	leading := []string{"/runs/r1/shim", "/repo/.tools/bin", "/py/bin", "/repo/.tools/claude-code/9.9.9/bin"}
	base := []string{"PATH=/usr/bin:/bin", "HOME=/home/me", "TMPDIR=/tmp", launch.OAuthTokenVariable + "=secret"}
	env := plan.Environment(base, plan.Variables(layout, leading))

	t.Run("the shims lead the PATH, then the binaries, the toolchain and the pinned CLI, then the inherited PATH [verifies SP-2402~1]", func(t *testing.T) {
		if !slices.Contains(env, "PATH=/runs/r1/shim:/repo/.tools/bin:/py/bin:/repo/.tools/claude-code/9.9.9/bin:/usr/bin:/bin") {
			t.Errorf("env = %v", env)
		}
	})

	t.Run("the run's state, caches and temporary files replace the inherited ones, and the token is dropped [verifies SP-2402~1]", func(t *testing.T) {
		for _, wanted := range []string{"XDG_STATE_HOME=/runs/r1/state", "XDG_CACHE_HOME=/runs/r1/cache", "TMPDIR=/runs/r1/tmp", "HOME=/home/me", "PYTHONDONTWRITEBYTECODE=1", "DISABLE_AUTOUPDATER=1"} {
			if !slices.Contains(env, wanted) {
				t.Errorf("env lacks %s: %v", wanted, env)
			}
		}
		for _, pair := range env {
			if strings.HasPrefix(pair, "TMPDIR=/tmp") || strings.HasPrefix(pair, launch.OAuthTokenVariable+"=") {
				t.Errorf("env keeps %s", pair)
			}
		}
	})

	t.Run("a shim exports the project directory and the run's variables, not the PATH, and hands over to the runner's shim [verifies SP-2402~1]", func(t *testing.T) {
		script := plan.ShimScript("/repo/.tools/bin/spec-proof-demo-loop", "/repo/.tools/bin/shallnot", "/runs/r1/records/hooks.jsonl", plan.ShimExports(layout, plan.Variables(layout, leading)))
		if strings.Contains(script, "export PATH") || !strings.Contains(script, "export XDG_STATE_HOME=/runs/r1/state\n") || !strings.HasPrefix(script, "#!/bin/sh\nexport CLAUDE_PROJECT_DIR=/runs/r1/project\n") {
			t.Errorf("script:\n%s", script)
		}
		if !strings.HasSuffix(script, "exec /repo/.tools/bin/spec-proof-demo-loop shim --real /repo/.tools/bin/shallnot --record /runs/r1/records/hooks.jsonl -- \"$@\"\n") {
			t.Errorf("script:\n%s", script)
		}
	})
}
