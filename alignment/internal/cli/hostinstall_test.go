package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/alignment/internal/cli"
)

const (
	oldSchemaScript = "#!/bin/sh\nprintf '%s\\n' '{\"properties\":{\"schema_version\":{\"pattern\":\"^1\\\\.[0-9]+$\"}}}'\n"
	shallnotYAML    = "version: 1\nspecs:\n  - specs\ntests:\n  - tests\nresults:\n  - build/test-results/*.xml\n"
	sharedConfig    = ".spec-proof/config.yaml"
	absentFile      = "<absent>"
	agentsFile      = "AGENTS.md"
	agentsBegin     = "<!-- spec-proof-alignment:begin -->"
)

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return absentFile
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// oldShallnot returns a Shallnot whose report schema is older than 1.2.
func oldShallnot(t *testing.T) string {
	t.Helper()
	script := filepath.Join(t.TempDir(), "shallnot")
	writeFile(t, script, oldSchemaScript)
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

func TestInitCommand(t *testing.T) {
	t.Run("init installs the harness files, keeps a configuration that exists and enables nothing [verifies SP-745~1]", func(t *testing.T) {
		f := newHostFixture(t)
		writeFile(t, filepath.Join(f.root, "shallnot.yaml"), shallnotYAML)
		existing := "proofs:\n  alignment: true\n"
		writeFile(t, filepath.Join(f.root, sharedConfig), existing)
		code, stdout, stderr := f.run(nil, "init", "--hooks", "claude,codex")
		if code != cli.ExitClean {
			t.Fatalf("exit %d: %s%s", code, stdout, stderr)
		}
		for _, want := range []string{"unchanged " + sharedConfig, "create    .codex/agents/spec-proof-alignment-judge.toml", "create    .claude/settings.json", "next: "} {
			if !strings.Contains(stdout, want) {
				t.Errorf("output lacks %q:\n%s", want, stdout)
			}
		}
		if got := readFile(t, filepath.Join(f.root, "shallnot.yaml")); got != shallnotYAML {
			t.Errorf("shallnot.yaml changed: %q", got)
		}
		if got := readFile(t, filepath.Join(f.root, sharedConfig)); got != existing {
			t.Errorf("an existing configuration was changed: %q", got)
		}
		if got := readFile(t, filepath.Join(f.root, agentsFile)); got != absentFile {
			t.Errorf("init wrote AGENTS.md: %q", got)
		}
	})

	t.Run("init seeds the shared configuration with the proof disabled and ignores the module's local state [verifies SP-745~1]", func(t *testing.T) {
		f := newHostFixture(t)
		if code, stdout, stderr := f.run(nil, "init", "--hooks", "none"); code != cli.ExitClean {
			t.Fatalf("exit %d: %s%s", code, stdout, stderr)
		}
		if got := readFile(t, filepath.Join(f.root, sharedConfig)); got != "proofs:\n  alignment: false\n" {
			t.Errorf("configuration = %q", got)
		}
		ignored := readFile(t, filepath.Join(f.root, ".gitignore"))
		for _, line := range []string{".spec-proof/cache/", ".spec-proof/verified/"} {
			if !strings.Contains(ignored, line+"\n") {
				t.Errorf(".gitignore lacks %q: %q", line, ignored)
			}
		}
		if code, stdout, _ := f.run(nil, "init", "--hooks", "none", "--check"); code != cli.ExitClean || strings.Contains(stdout, "create") {
			t.Errorf("a second init would change something: exit %d\n%s", code, stdout)
		}
	})

	t.Run("init --check reports a pending change by its exit code and writes nothing [verifies SP-745~1]", func(t *testing.T) {
		f := newHostFixture(t)
		if code, _, _ := f.run(nil, "init", "--hooks", "codex", "--check"); code != cli.ExitBlocked {
			t.Fatalf("exit = %d", code)
		}
		if got := readFile(t, filepath.Join(f.root, ".codex", "hooks.json")); got != absentFile {
			t.Fatalf("--check wrote %q", got)
		}
		if code, _, stderr := f.run(nil, "init", "--hooks", "codex"); code != cli.ExitClean {
			t.Fatalf("init: %d %s", code, stderr)
		}
		if code, _, _ := f.run(nil, "init", "--hooks", "codex", "--check"); code != cli.ExitClean {
			t.Fatalf("a second --check exit = %d", code)
		}
	})

	t.Run("doctor names the harnesses whose files init installed [verifies SP-745~1]", func(t *testing.T) {
		f := newHostFixture(t)
		if code, _, stderr := f.run(nil, "init", "--hooks", "claude,codex"); code != cli.ExitClean {
			t.Fatalf("init: %d %s", code, stderr)
		}
		_, stdout, _ := f.run(nil, "doctor")
		if !strings.Contains(stdout, "ok host-files claude, codex") {
			t.Fatalf("report:\n%s", stdout)
		}
	})

	t.Run("init refuses a Shallnot older than report schema 1.2, a harness it has no files for and a stray argument [verifies SP-745~1]", func(t *testing.T) {
		f := newHostFixture(t)
		code, _, stderr := f.runWithEnv(map[string]string{shallnotVariable: oldShallnot(t)}, "init")
		if code != cli.ExitBlocked || !strings.Contains(stderr, "1.2") {
			t.Errorf("old Shallnot: exit %d, stderr %q", code, stderr)
		}
		if got := readFile(t, filepath.Join(f.root, sharedConfig)); got != absentFile {
			t.Errorf("a refused init wrote the configuration: %q", got)
		}
		if code, _, _ := f.run(nil, "init", "--hooks", "vim"); code != cli.ExitBlocked {
			t.Errorf("unknown harness exit = %d", code)
		}
		if code, _, _ := f.run(nil, "init", "extra"); code != cli.ExitToolFailure {
			t.Errorf("stray argument exit = %d", code)
		}
	})
}

func TestEnableAndDisableCommands(t *testing.T) {
	t.Run("enable adds the proof to shallnot.yaml, the shared configuration and AGENTS.md and disable takes it away [verifies SP-746~1]", func(t *testing.T) {
		f := newHostFixture(t)
		if code, _, _ := f.run(nil, "enable"); code != cli.ExitBlocked {
			t.Fatalf("enable without shallnot.yaml exit = %d", code)
		}
		writeFile(t, filepath.Join(f.root, "shallnot.yaml"), shallnotYAML)
		if code, stdout, stderr := f.run(nil, "enable", "--dir", f.root); code != cli.ExitClean {
			t.Fatalf("enable exit %d: %s%s", code, stdout, stderr)
		}
		if got := readFile(t, filepath.Join(f.root, "shallnot.yaml")); got != shallnotYAML+"proofs:\n  require:\n    - alignment\n" {
			t.Errorf("shallnot.yaml = %q", got)
		}
		if got := readFile(t, filepath.Join(f.root, sharedConfig)); !strings.Contains(got, "alignment: true") {
			t.Errorf("configuration = %q", got)
		}
		agents := readFile(t, filepath.Join(f.root, agentsFile))
		if !strings.Contains(agents, agentsBegin) || !strings.Contains(agents, "spec-proof-alignment prove --changed") {
			t.Errorf("AGENTS.md = %q", agents)
		}
		if code, _, stderr := f.run(nil, "disable"); code != cli.ExitClean {
			t.Fatalf("disable: %d %s", code, stderr)
		}
		if got := readFile(t, filepath.Join(f.root, "shallnot.yaml")); got != shallnotYAML {
			t.Errorf("shallnot.yaml after disable = %q", got)
		}
		if got := readFile(t, filepath.Join(f.root, sharedConfig)); !strings.Contains(got, "alignment: false") {
			t.Errorf("configuration after disable = %q", got)
		}
		if got := readFile(t, filepath.Join(f.root, agentsFile)); got != absentFile {
			t.Errorf("AGENTS.md after disable = %q", got)
		}
	})

	t.Run("enable leaves what AGENTS.md held and disable restores it [verifies SP-746~1]", func(t *testing.T) {
		f := newHostFixture(t)
		writeFile(t, filepath.Join(f.root, "shallnot.yaml"), shallnotYAML)
		const own = "# Project rules\n\nRun the formatter.\n"
		writeFile(t, filepath.Join(f.root, agentsFile), own)
		if code, _, stderr := f.run(nil, "enable"); code != cli.ExitClean {
			t.Fatalf("enable: %d %s", code, stderr)
		}
		if got := readFile(t, filepath.Join(f.root, agentsFile)); !strings.HasPrefix(got, own) || !strings.Contains(got, agentsBegin) {
			t.Errorf("AGENTS.md = %q", got)
		}
		if code, _, stderr := f.run(nil, "disable"); code != cli.ExitClean {
			t.Fatalf("disable: %d %s", code, stderr)
		}
		if got := readFile(t, filepath.Join(f.root, agentsFile)); got != own {
			t.Errorf("AGENTS.md after disable = %q", got)
		}
	})

	t.Run("enable --check reports a pending change and writes nothing [verifies SP-746~1]", func(t *testing.T) {
		f := newHostFixture(t)
		writeFile(t, filepath.Join(f.root, "shallnot.yaml"), shallnotYAML)
		if code, _, _ := f.run(nil, "enable", "--check"); code != cli.ExitBlocked {
			t.Fatalf("exit = %d", code)
		}
		if got := readFile(t, filepath.Join(f.root, "shallnot.yaml")); got != shallnotYAML {
			t.Errorf("--check wrote shallnot.yaml: %q", got)
		}
	})

	t.Run("enable and disable refuse a Shallnot older than report schema 1.2 and write nothing [verifies SP-746~1]", func(t *testing.T) {
		f := newHostFixture(t)
		writeFile(t, filepath.Join(f.root, "shallnot.yaml"), shallnotYAML)
		overrides := map[string]string{shallnotVariable: oldShallnot(t)}
		for _, command := range []string{"enable", "disable"} {
			code, _, stderr := f.runWithEnv(overrides, command)
			if code != cli.ExitBlocked || !strings.Contains(stderr, "1.2") {
				t.Errorf("%s: exit %d, stderr %q", command, code, stderr)
			}
		}
		if got := readFile(t, filepath.Join(f.root, agentsFile)); got != absentFile {
			t.Errorf("a refused command wrote AGENTS.md: %q", got)
		}
	})
}

func TestDoctorSchemaCheck(t *testing.T) {
	t.Run("doctor reports the report schema of Shallnot and fails on an older one [verifies SP-747~1]", func(t *testing.T) {
		f := newHostFixture(t)
		if _, stdout, _ := f.run(nil, "doctor"); !strings.Contains(stdout, "ok shallnot-schema") {
			t.Fatalf("report:\n%s", stdout)
		}
		code, stdout, _ := f.runWithEnv(map[string]string{shallnotVariable: oldShallnot(t)}, "doctor")
		if code != cli.ExitToolFailure || !strings.Contains(stdout, "fail shallnot-schema") {
			t.Fatalf("exit %d, report:\n%s", code, stdout)
		}
	})
}
