package plan_test

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/plan"
)

// enabled is shallnot.yaml as `shallnot init` and the enables of the three
// modules leave it.
const enabled = `version: 1
specs:
  - specs
proofs:
  require:
    - discriminating-power
    - alignment
  trust: local
  code:
    - boxoffice

test_commands:
  - python3 -m pytest
`

func TestSetRequire(t *testing.T) {
	t.Run("the require list becomes the flow list of the arm, and every other line stays [verifies SP-2511~1]", func(t *testing.T) {
		for _, tc := range []struct {
			kinds []string
			want  string
		}{
			{[]string{}, "  require: []\n"},
			{[]string{"discriminating-power", "alignment"}, "  require: [discriminating-power, alignment]\n"},
		} {
			got, err := plan.SetRequire([]byte(enabled), tc.kinds)
			if err != nil {
				t.Fatal(err)
			}
			want := strings.Replace(enabled, "  require:\n    - discriminating-power\n    - alignment\n", tc.want, 1)
			if string(got) != want {
				t.Errorf("kinds %v:\n%s\nwant:\n%s", tc.kinds, got, want)
			}
		}
	})

	t.Run("a flow list, a list at the key's own indentation and a block without require are rewritten too [verifies SP-2511~1]", func(t *testing.T) {
		for input, want := range map[string]string{
			"proofs:\n  require: [alignment]\n  trust: local\n":          "proofs:\n  require: []\n  trust: local\n",
			"proofs:\n  require:\n  - alignment\n  trust: local\n":       "proofs:\n  require: []\n  trust: local\n",
			"proofs:\n    trust: local\nother: 1\n":                      "proofs:\n    require: []\n    trust: local\nother: 1\n",
			"proofs:\n  require:\n    - alignment\n\n# note\nother: 1\n": "proofs:\n  require: []\n\n# note\nother: 1\n",
		} {
			got, err := plan.SetRequire([]byte(input), []string{})
			if err != nil || string(got) != want {
				t.Errorf("%q gave %q (%v), want %q", input, got, err, want)
			}
		}
	})

	t.Run("a configuration whose proofs block cannot be located is refused [verifies SP-2511~1]", func(t *testing.T) {
		for _, input := range []string{"version: 1\n", "proofs: {require: []}\n", "proofs:\n  trust: local\nproofs:\n  code: []\n"} {
			if _, err := plan.SetRequire([]byte(input), nil); !errors.Is(err, plan.ErrRequireShape) {
				t.Errorf("%q: %v", input, err)
			}
		}
	})

	t.Run("the edit only touches the require entry, whatever the kinds and the lines around it [verifies SP-2511~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			kinds := rapid.SliceOfN(rapid.SampledFrom([]string{"discriminating-power", "alignment"}), 0, 2).Draw(t, "kinds")
			before := rapid.SliceOfN(rapid.StringMatching(`[a-z]{1,8}: [a-z0-9]{0,8}`), 0, 3).Draw(t, "before")
			after := rapid.SliceOfN(rapid.StringMatching(`  [a-z]{1,8}: [a-z0-9]{0,8}`), 0, 3).Draw(t, "after")
			input := strings.Join(append(append(before, "proofs:", "  require:", "    - alignment"), after...), "\n") + "\n"
			got, err := plan.SetRequire([]byte(input), kinds)
			if err != nil {
				t.Fatal(err)
			}
			want := strings.Join(append(append(before, "proofs:", "  require: ["+strings.Join(kinds, ", ")+"]"), after...), "\n") + "\n"
			if string(got) != want {
				t.Fatalf("%q, want %q", got, want)
			}
		})
	})
}

func TestStudySessionSettings(t *testing.T) {
	t.Run("the study's session settings allow the configured rules and deny the web tools, and hold nothing else [verifies SP-2512~1]", func(t *testing.T) {
		encoded, err := plan.SessionSettingsJSON([]string{"Bash(pytest *)", "Skill"}, plan.WebTools())
		if err != nil {
			t.Fatal(err)
		}
		var settings map[string]map[string][]string
		if err := json.Unmarshal(encoded, &settings); err != nil {
			t.Fatal(err)
		}
		permissions := settings["permissions"]
		if len(settings) != 1 || len(permissions) != 2 || !slices.Equal(permissions["allow"], []string{"Bash(pytest *)", "Skill"}) || !slices.Equal(permissions["deny"], []string{"WebFetch", "WebSearch"}) {
			t.Errorf("settings = %s", encoded)
		}
	})
}

// shellWorld is a project under git with the fake binaries a shim script runs.
type shellWorld struct {
	layout           plan.Layout
	runner, shallnot string
	env              []string
}

const (
	fakeRunner = "#!/bin/sh\n# shim --real REAL --record LOG -- ARGS\nreal=$3\nshift 6\nexec \"$real\" \"$@\"\n"
	// fakeShallnot blocks every hook call, writes a report on check, and lists, on
	// gate, the files it sees into the report.
	fakeShallnot = "#!/bin/sh\ncase \"$1\" in\n" +
		"hook) cat >/dev/null; echo 'blocked: BOX-2' >&2; exit 2 ;;\n" +
		"check) printf '{\"verdict\":\"blocked\"}' > \"$3\"; echo noise; echo noise >&2; exit 1 ;;\n" +
		"gate) find . -path ./.git -prune -o -type f -print | sort > \"$3\"; cat shallnot.yaml >> \"$3\"; exit 0 ;;\n" +
		"esac\n"
)

func newShellWorld(t *testing.T) shellWorld {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required")
	}
	run := t.TempDir()
	l := plan.Layout{Repo: run, Run: run}
	w := shellWorld{layout: l, runner: filepath.Join(run, "runner"), shallnot: filepath.Join(run, "shallnot"),
		env: append(os.Environ(), plan.GitEnvironment()...)}
	for path, content := range map[string]string{w.runner: fakeRunner, w.shallnot: fakeShallnot} {
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		"shallnot.yaml": "proofs:\n  require: []\n", "AGENTS.md": "agents\n", "specs/tickets.md": "spec\n",
		"tests/test_a.py": "a\n", ".gitignore": "ignored/\n",
	} {
		w.write(t, path, content)
	}
	if err := os.MkdirAll(l.Records(), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, step := range plan.BaseSteps() {
		w.run(t, step.Argv...)
	}
	return w
}

func (w shellWorld) write(t *testing.T, relative, content string) {
	t.Helper()
	path := filepath.Join(w.layout.Project(), relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (w shellWorld) run(t *testing.T, argv ...string) string {
	t.Helper()
	command := exec.Command(argv[0], argv[1:]...)
	command.Dir, command.Env = w.layout.Project(), w.env
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %v\n%s", argv, err, output)
	}
	return string(output)
}

func (w shellWorld) read(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestStopShimScript(t *testing.T) {
	w := newShellWorld(t)
	l := w.layout
	shim := filepath.Join(l.Run, "shim-shallnot")
	script := plan.StopShimScript(w.runner, w.shallnot, l.Record("hooks.jsonl"), []string{"PATH=/ignored", "TMPDIR=" + l.Run}, l)
	if err := os.WriteFile(shim, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	w.write(t, "tests/test_a.py", "a changed\n")
	w.write(t, "tests/test_b.py", "b\n")
	w.write(t, "ignored/cache", "x\n")
	statusBefore := w.run(t, "git", "status", "--porcelain")
	indexBefore := w.read(t, filepath.Join(l.Project(), ".git", "index"))
	call := func(args ...string) (int, string, string) {
		command := exec.Command(shim, args...)
		command.Dir, command.Env, command.Stdin = l.Project(), w.env, strings.NewReader(`{"hook_event_name":"Stop"}`)
		var stdout, stderr strings.Builder
		command.Stdout, command.Stderr = &stdout, &stderr
		err := command.Run()
		var exit *exec.ExitError
		if err != nil && !errors.As(err, &exit) {
			t.Fatal(err)
		}
		return command.ProcessState.ExitCode(), stdout.String(), stderr.String()
	}
	code, stdout, stderr := call("hook", "claude-stop")
	call("hook", "claude-stop")
	call("gate", "--json-out", filepath.Join(l.Run, "elsewhere.json"))

	t.Run("each end-of-turn call writes the gate's report and the project's tree under its number, the harness sees only the hook's answer [verifies SP-2518~1]", func(t *testing.T) {
		if code != 2 || stdout != "" || stderr != "blocked: BOX-2\n" {
			t.Errorf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
		}
		for _, n := range []string{"1", "2"} {
			if report := w.read(t, filepath.Join(l.Stops(), n, plan.StopReportFile)); report != `{"verdict":"blocked"}` {
				t.Errorf("stop %s report %q", n, report)
			}
		}
		if _, err := os.Stat(filepath.Join(l.Stops(), "3")); !errors.Is(err, os.ErrNotExist) {
			t.Error("a call that is not the end-of-turn hook was captured")
		}
	})

	t.Run("the capture trusts the project for git whatever environment the hook inherits [verifies SP-2518~1]", func(t *testing.T) {
		if !strings.Contains(script, "export GIT_CONFIG_VALUE_0="+l.Project()) {
			t.Errorf("the shim does not trust the project:\n%s", script)
		}
	})

	t.Run("the tree is the project's as the session left it, in the run's own object store, and the project's index and status do not change [verifies SP-2518~1]", func(t *testing.T) {
		tree := strings.TrimSpace(w.read(t, filepath.Join(l.Stops(), "1", plan.StopTreeFile)))
		store := []string{"GIT_OBJECT_DIRECTORY=" + l.Path(plan.StopObjectsDir), "GIT_ALTERNATE_OBJECT_DIRECTORIES=" + filepath.Join(l.Project(), ".git", "objects")}
		command := exec.Command("git", "ls-tree", "-r", "--name-only", tree)
		command.Dir, command.Env = l.Project(), append(append([]string{}, w.env...), store...)
		listed, err := command.Output()
		if err != nil {
			t.Fatalf("the tree %q is not in the run's store: %v", tree, err)
		}
		if got := strings.Fields(string(listed)); !slices.Equal(got, []string{".gitignore", "AGENTS.md", "shallnot.yaml", "specs/tickets.md", "tests/test_a.py", "tests/test_b.py"}) {
			t.Errorf("tree lists %v", got)
		}
		probe := exec.Command("git", "cat-file", "-e", tree)
		probe.Dir, probe.Env = l.Project(), w.env
		if probe.Run() == nil {
			t.Error("the tree was written into the project's own object store")
		}
		if w.run(t, "git", "status", "--porcelain") != statusBefore || w.read(t, filepath.Join(l.Project(), ".git", "index")) != indexBefore {
			t.Error("the capture changed the project's index or status")
		}
	})
}

func TestAcceptanceSteps(t *testing.T) {
	w := newShellWorld(t)
	l := w.layout
	equipped := strings.TrimSpace(w.run(t, plan.HeadCommitArgv()...))
	w.write(t, "shallnot.yaml", "proofs:\n  require: []\nedited: true\n")
	w.write(t, "specs/extra.md", "added by the session\n")
	if err := os.Remove(filepath.Join(l.Project(), "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	w.write(t, "ignored/results.xml", "kept\n")
	report := filepath.Join(l.Run, "gate-after.json")
	steps := plan.AcceptanceSteps(l, equipped, report).Steps()
	steps[1].Argv[0] = w.shallnot
	for _, step := range steps {
		w.run(t, step.Argv...)
	}
	seen := w.read(t, report)

	t.Run("the acceptance gate sees the equipped shallnot.yaml, specs and AGENTS.md and every other file as the session left it [verifies SP-2520~1]", func(t *testing.T) {
		for _, wanted := range []string{"./AGENTS.md", "./specs/tickets.md", "./ignored/results.xml", "./tests/test_a.py"} {
			if !strings.Contains(seen, wanted+"\n") {
				t.Errorf("the gate did not see %s:\n%s", wanted, seen)
			}
		}
		if strings.Contains(seen, "extra.md") || strings.Contains(seen, "edited") {
			t.Errorf("the gate saw the session's own gate files:\n%s", seen)
		}
	})

	t.Run("the session's own versions are put back once the gate has run [verifies SP-2520~1]", func(t *testing.T) {
		if got := w.read(t, filepath.Join(l.Project(), "shallnot.yaml")); !strings.Contains(got, "edited: true") {
			t.Errorf("shallnot.yaml = %q", got)
		}
		if _, err := os.Stat(filepath.Join(l.Project(), "AGENTS.md")); !errors.Is(err, os.ErrNotExist) {
			t.Error("AGENTS.md, which the session removed, is back")
		}
		if got := w.read(t, filepath.Join(l.Project(), "specs", "extra.md")); got != "added by the session\n" {
			t.Errorf("specs/extra.md = %q", got)
		}
		if _, err := os.Stat(l.Path(plan.AcceptAsideDir)); !errors.Is(err, os.ErrNotExist) {
			t.Error("the aside directory is left behind")
		}
	})

	t.Run("a put-back after a restore that never ran changes nothing [verifies SP-2520~1]", func(t *testing.T) {
		before := w.run(t, "git", "status", "--porcelain")
		w.run(t, plan.AcceptanceSteps(l, equipped, report).PutBack.Argv...)
		if w.run(t, "git", "status", "--porcelain") != before || !strings.Contains(w.read(t, filepath.Join(l.Project(), "shallnot.yaml")), "edited: true") {
			t.Error("the put-back changed the project")
		}
	})
}
