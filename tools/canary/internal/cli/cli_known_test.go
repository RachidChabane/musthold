package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/cli"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/verdict"
)

const (
	witnessCheck   = "classify_witness_recorded"
	witnessJournal = "classify-journal.jsonl"
	// unpinnedWitness is the observation of a witness that raised nothing because the
	// CLI version is not one it was checked on.
	unpinnedWitness = `{\"verdict\":\"none\",\"reason\":\"cli_version_not_pinned\",\"loggedCalls\":0}`
	raisedWitness   = `{\"verdict\":\"raised\",\"model\":\"claude-sonnet-5-5-20260101\",\"querySources\":[\"hook_prompt\"],\"loggedCalls\":5}`
	registryPath    = "../../known-inconclusive.json"
)

// unpinnedClaudeRecords copies the recorded Claude Code session with the witness of
// the classifier left inconclusive for an unpinned CLI version.
func unpinnedClaudeRecords(t *testing.T) string {
	t.Helper()
	records := t.TempDir()
	entries, err := os.ReadDir(recordedSession)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(recordedSession, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if entry.Name() == witnessJournal {
			data = []byte(strings.Replace(string(data), raisedWitness, unpinnedWitness, 1))
		}
		if err := os.WriteFile(filepath.Join(records, entry.Name()), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return records
}

func registryWith(t *testing.T, version, check, reason string) string {
	t.Helper()
	body := `{"entries":[{"harness":"claude","cli_version":"` + version + `","check":"` + check + `","reason":"` + reason +
		`","declared":"2026-10-11","reference":"ADR 0005, fact F: classifier requests not logged"}]}`
	path := filepath.Join(t.TempDir(), "known.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestKnownInconclusive(t *testing.T) {
	t.Run("without the flag an inconclusive check keeps the inconclusive exit code [verifies SP-1527~1]", func(t *testing.T) {
		code, stdout, stderr := run(t, "", "evaluate", "--harness", "claude", "--records", unpinnedClaudeRecords(t))
		if code != verdict.ExitInconclusive || strings.Contains(stdout, `"known"`) || strings.Contains(stderr, "known:") {
			t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
		}
	})

	t.Run("an entry that matches exactly ends the run with the success code and the check is still reported inconclusive [verifies SP-1527~1]", func(t *testing.T) {
		records := unpinnedClaudeRecords(t)
		code, stdout, stderr := run(t, "", "evaluate", "--harness", "claude", "--records", records, "--known-inconclusive", registryPath)
		if code != verdict.ExitAllPass {
			t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
		}
		reported := verdictOf(t, stdout, witnessCheck)
		if reported.Status != verdict.Inconclusive || !strings.HasPrefix(reported.Known, "known: declared 2026-10-11, ADR 0005, fact F") {
			t.Errorf("verdict = %+v", reported)
		}
		if !strings.Contains(stderr, "1 INCONCLUSIVE") || !strings.Contains(stderr, "known: declared 2026-10-11, ADR 0005, fact F: classifier requests not logged") {
			t.Errorf("stderr = %q", stderr)
		}
		saved, err := os.ReadFile(filepath.Join(records, "report.json"))
		if err != nil || !strings.Contains(string(saved), `"known": "known: declared 2026-10-11`) || !strings.Contains(string(saved), `"INCONCLUSIVE"`) {
			t.Errorf("report.json = %s, %v", saved, err)
		}
	})

	t.Run("an entry for another version, reason or check leaves the inconclusive exit code [verifies SP-1527~1]", func(t *testing.T) {
		for name, registry := range map[string]string{
			"another version": registryWith(t, "2.1.293", witnessCheck, "cli_version_not_pinned"),
			"another reason":  registryWith(t, "2.1.295", witnessCheck, "log_unreadable_or_oversize"),
			"another check":   registryWith(t, "2.1.295", "cli_version_pinned", "cli_version_not_pinned"),
		} {
			code, stdout, stderr := run(t, "", "evaluate", "--harness", "claude", "--records", unpinnedClaudeRecords(t), "--known-inconclusive", registry)
			if code != verdict.ExitInconclusive || strings.Contains(stdout, `"known"`) {
				t.Errorf("%s: exit %d, stdout %q, stderr %q", name, code, stdout, stderr)
			}
		}
	})

	t.Run("a registry that is malformed or missing is refused with exit 2 [verifies SP-1527~1]", func(t *testing.T) {
		malformed := filepath.Join(t.TempDir(), "known.json")
		if err := os.WriteFile(malformed, []byte(`{"entries":[{"harness":"claude"}]}`), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{malformed, filepath.Join(t.TempDir(), "absent.json")} {
			code, _, stderr := run(t, "", "evaluate", "--harness", "claude", "--records", unpinnedClaudeRecords(t), "--known-inconclusive", path)
			if code != cli.ExitRefused || !strings.Contains(stderr, path) {
				t.Errorf("%s: exit %d, stderr %q", path, code, stderr)
			}
		}
		code, _, stderr := run(t, "", "run", "--harness", "codex", "--dry-run", "--repo", t.TempDir(), "--known-inconclusive", malformed)
		if code != cli.ExitRefused || !strings.Contains(stderr, "malformed") {
			t.Errorf("run: exit %d, stderr %q", code, stderr)
		}
	})

	t.Run("a dry run accepts the registry [verifies SP-1527~1]", func(t *testing.T) {
		code, _, stderr := run(t, "", "run", "--harness", "codex", "--dry-run", "--repo", t.TempDir(), "--known-inconclusive", registryPath)
		if code != 0 {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
	})
}

func verdictOf(t *testing.T, stdout, check string) verdict.Verdict {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		var v verdict.Verdict
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Fatalf("%q: %v", line, err)
		}
		if v.Check == check {
			return v
		}
	}
	t.Fatalf("no verdict for %s in %q", check, stdout)
	return verdict.Verdict{}
}
