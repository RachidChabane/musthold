package checks_test

import (
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/checks"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/verdict"
)

const (
	readOnlyPolicy       = `"sandbox_policy":{"type":"read-only","network_access":false}`
	workspaceWritePolicy = `"sandbox_policy":{"type":"workspace-write","writable_roots":[],"network_access":false}`
)

// withExecutorPolicy swaps the sandbox policy the executor child's rollout records.
func withExecutorPolicy(files evidence.Files, policy string) evidence.Files {
	children := map[string][]byte{}
	for name, rollout := range files.RolloutChildren {
		children[name] = replace(rollout, readOnlyPolicy, policy)
	}
	files.RolloutChildren = children
	return files
}

func judge(t *testing.T, files evidence.Files, declared format.Isolation) verdict.Verdict {
	t.Helper()
	found, err := evidence.Assemble(files)
	if err != nil {
		t.Fatal(err)
	}
	return checks.JudgeExecutorSandbox(found, declared)
}

func TestExecutorSandboxVerdictMatrix(t *testing.T) {
	t.Run("an executor that ran under a policy other than read-only passes where the declared isolation is unrestricted, and says so [verifies SP-1520~1]", func(t *testing.T) {
		files := withExecutorPolicy(readFiles(t, codexSession), workspaceWritePolicy)
		files.SandboxProbe = replace(files.SandboxProbe, `"exists": false`, `"exists": true`)
		v := want(t, evaluate(t, "codex", files), "executor_sandbox", verdict.Pass)
		for _, text := range []string{"sandbox_mode was not applied", "workspace-write", "unrestricted"} {
			if !strings.Contains(v.Summary, text) {
				t.Errorf("summary lacks %q: %s", text, v.Summary)
			}
		}
		if len(v.Evidence) != 1 || v.Evidence[0].Line != 2 || !strings.HasPrefix(v.Evidence[0].File, evidence.RolloutChildrenDir+"/") {
			t.Errorf("evidence = %+v", v.Evidence)
		}
	})

	t.Run("a non-read-only policy fails where the declared isolation is above unrestricted [verifies SP-1520~1]", func(t *testing.T) {
		files := withExecutorPolicy(readFiles(t, codexSession), workspaceWritePolicy)
		for _, declared := range []format.Isolation{format.IsolationRestrictedAttested, format.IsolationRestricted, format.IsolationRestrictedUnverified} {
			if v := judge(t, files, declared); v.Status != verdict.Fail || !strings.Contains(v.Summary, string(declared)) {
				t.Errorf("%s: %s (%s)", declared, v.Status, v.Summary)
			}
		}
	})

	t.Run("a read-only policy passes when the attempted write did not land, whatever the declared isolation [verifies SP-1520~1]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		for _, declared := range []format.Isolation{format.IsolationRestricted, format.IsolationUnrestricted} {
			if v := judge(t, files, declared); v.Status != verdict.Pass || !strings.Contains(v.Summary, "read-only") {
				t.Errorf("%s: %s (%s)", declared, v.Status, v.Summary)
			}
		}
	})

	t.Run("a write that landed under a policy reported read-only fails [verifies SP-1520~1]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.SandboxProbe = replace(files.SandboxProbe, `"exists": false`, `"exists": true`)
		v := judge(t, files, format.IsolationUnrestricted)
		if v.Status != verdict.Fail || len(v.Evidence) != 2 {
			t.Errorf("verdict = %+v", v)
		}
	})

	t.Run("a read-only executor that never tried the write leaves the sandbox inconclusive [verifies SP-1520~1]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.Stream = dropLines(files.Stream, "touch sandbox-probe.txt'")
		want(t, evaluate(t, "codex", files), "executor_sandbox", verdict.Inconclusive)
	})

	t.Run("a session with no executor child rollout leaves the sandbox inconclusive [verifies SP-1520~1]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		files.RolloutChildren = nil
		v := want(t, evaluate(t, "codex", files), "executor_sandbox", verdict.Inconclusive)
		if !strings.Contains(v.Summary, "no rollout of an executor child") {
			t.Errorf("summary = %s", v.Summary)
		}
	})

	t.Run("an executor rollout that records no policy leaves the sandbox inconclusive [verifies SP-1520~1]", func(t *testing.T) {
		files := readFiles(t, codexSession)
		for name, rollout := range files.RolloutChildren {
			files.RolloutChildren[name] = dropLines(rollout, "turn_context")
		}
		v := want(t, evaluate(t, "codex", files), "executor_sandbox", verdict.Inconclusive)
		if !strings.Contains(v.Summary, "records no sandbox policy") {
			t.Errorf("summary = %s", v.Summary)
		}
	})

	t.Run("the check says what its pass establishes in terms of the declared isolation [verifies SP-1520~1]", func(t *testing.T) {
		set, err := checks.ForHarness("codex")
		if err != nil {
			t.Fatal(err)
		}
		for _, check := range set {
			if check.Name() == "executor_sandbox" && !strings.Contains(check.Proves(), "declared isolation is unrestricted") {
				t.Errorf("proves = %s", check.Proves())
			}
		}
	})
}
