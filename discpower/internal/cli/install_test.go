package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/cli"
)

const (
	oldSchemaScript = "#!/bin/sh\nprintf '%s\\n' '{\"properties\":{\"schema_version\":{\"pattern\":\"^1\\\\.[0-9]+$\"}}}'\n"
	shallnotYAML    = "version: 1\nspecs:\n  - specs\ntests:\n  - tests\nresults:\n  - build/test-results/*.xml\n"
)

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "<absent>"
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestInstallCommands(t *testing.T) {
	t.Run("init in the parcel project installs the harness files and enables nothing [verifies SP-243~2]", func(t *testing.T) {
		f := newHostFixture(t)
		writeFile(t, filepath.Join(f.root, "shallnot.yaml"), shallnotYAML)
		before := readFile(t, filepath.Join(f.root, ".spec-proof", "config.yaml"))
		code, stdout, stderr := f.run(nil, "init", "--hooks", "claude,codex")
		if code != cli.ExitClean {
			t.Fatalf("exit %d: %s%s", code, stdout, stderr)
		}
		for _, want := range []string{"unchanged .spec-proof/config.yaml", "create    .codex/agents/spec-proof-dp-judge.toml", "create    .claude/settings.json", "next: "} {
			if !strings.Contains(stdout, want) {
				t.Errorf("output lacks %q:\n%s", want, stdout)
			}
		}
		if got := readFile(t, filepath.Join(f.root, "shallnot.yaml")); got != shallnotYAML {
			t.Errorf("shallnot.yaml changed: %q", got)
		}
		config := readFile(t, filepath.Join(f.root, ".spec-proof", "config.yaml"))
		if !strings.Contains(config, "discriminating-power: true") || config != before {
			t.Errorf("an existing configuration was changed: %q", config)
		}
	})

	t.Run("init --check reports a pending change by its exit code and writes nothing [verifies SP-243~2]", func(t *testing.T) {
		f := newHostFixture(t)
		if code, _, _ := f.run(nil, "init", "--hooks", "codex", "--check"); code != cli.ExitBlocked {
			t.Fatalf("exit = %d", code)
		}
		if got := readFile(t, filepath.Join(f.root, ".codex", "hooks.json")); got != "<absent>" {
			t.Fatalf("--check wrote %q", got)
		}
		if code, _, stderr := f.run(nil, "init", "--hooks", "codex"); code != cli.ExitClean {
			t.Fatalf("init: %d %s", code, stderr)
		}
		if code, _, _ := f.run(nil, "init", "--hooks", "codex", "--check"); code != cli.ExitClean {
			t.Fatalf("a second --check exit = %d", code)
		}
	})

	t.Run("init, enable and disable refuse a Shallnot older than report schema 1.2 [verifies SP-243~2]", func(t *testing.T) {
		f := newHostFixture(t)
		script := filepath.Join(t.TempDir(), "shallnot")
		writeFile(t, script, oldSchemaScript)
		if err := os.Chmod(script, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, command := range []string{"init", "enable", "disable"} {
			code, _, stderr := f.runWithEnv(map[string]string{shallnotVariable: script}, command)
			if code != cli.ExitBlocked || !strings.Contains(stderr, "1.2") {
				t.Errorf("%s: exit %d, stderr %q", command, code, stderr)
			}
		}
		if got := readFile(t, filepath.Join(f.root, "AGENTS.md")); got != "<absent>" {
			t.Errorf("a refused command wrote AGENTS.md: %q", got)
		}
	})

	t.Run("init refuses a harness it has no files for and a stray argument [verifies SP-243~2]", func(t *testing.T) {
		f := newHostFixture(t)
		if code, _, _ := f.run(nil, "init", "--hooks", "vim"); code != cli.ExitBlocked {
			t.Errorf("unknown harness exit = %d", code)
		}
		if code, _, _ := f.run(nil, "init", "extra"); code != cli.ExitToolFailure {
			t.Errorf("stray argument exit = %d", code)
		}
	})

	t.Run("enable adds the proof, the local trust and the source roots to shallnot.yaml and the proof to AGENTS.md, and disable takes the proof away [verifies SP-244~2]", func(t *testing.T) {
		f := newHostFixture(t)
		if code, _, _ := f.run(nil, "enable"); code != cli.ExitBlocked {
			t.Fatalf("enable without shallnot.yaml exit = %d", code)
		}
		writeFile(t, filepath.Join(f.root, "shallnot.yaml"), shallnotYAML)
		if code, stdout, stderr := f.run(nil, "enable", "--dir", f.root); code != cli.ExitClean {
			t.Fatalf("enable exit %d: %s%s", code, stdout, stderr)
		}
		if got := readFile(t, filepath.Join(f.root, "shallnot.yaml")); got != shallnotYAML+"proofs:\n  require:\n    - discriminating-power\n  trust: local\n  code:\n    - parcel\n" {
			t.Errorf("shallnot.yaml = %q", got)
		}
		if got := readFile(t, filepath.Join(f.root, "AGENTS.md")); !strings.Contains(got, "spec-proof-dp prove --changed") {
			t.Errorf("AGENTS.md = %q", got)
		}
		if code, _, stderr := f.run(nil, "disable"); code != cli.ExitClean {
			t.Fatalf("disable: %d %s", code, stderr)
		}
		if got := readFile(t, filepath.Join(f.root, "shallnot.yaml")); got != shallnotYAML+"proofs:\n  trust: local\n  code:\n    - parcel\n" {
			t.Errorf("shallnot.yaml after disable = %q", got)
		}
		if got := readFile(t, filepath.Join(f.root, "AGENTS.md")); got != "<absent>" {
			t.Errorf("AGENTS.md after disable = %q", got)
		}
	})
}

func TestDoctorSchemaCheck(t *testing.T) {
	t.Run("doctor reports the report schema of Shallnot and fails on an older one [verifies SP-245~1]", func(t *testing.T) {
		f := newHostFixture(t)
		if _, stdout, _ := f.run(nil, "doctor"); !strings.Contains(stdout, "ok shallnot-schema") {
			t.Fatalf("report:\n%s", stdout)
		}
		script := filepath.Join(t.TempDir(), "shallnot")
		writeFile(t, script, oldSchemaScript)
		if err := os.Chmod(script, 0o755); err != nil {
			t.Fatal(err)
		}
		code, stdout, _ := f.runWithEnv(map[string]string{shallnotVariable: script}, "doctor")
		if code != cli.ExitToolFailure || !strings.Contains(stdout, "fail shallnot-schema") {
			t.Fatalf("exit %d, report:\n%s", code, stdout)
		}
	})
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
