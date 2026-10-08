package evidence_test

import (
	"testing"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
)

func assembleChild(t *testing.T, rollout string) evidence.RolloutChild {
	t.Helper()
	found, err := evidence.Assemble(evidence.Files{RolloutChildren: map[string][]byte{"child.jsonl": []byte(rollout)}})
	if err != nil || len(found.Rollouts.Children) != 1 {
		t.Fatalf("rollouts = %+v, %v", found.Rollouts, err)
	}
	return found.Rollouts.Children[0]
}

func TestSandboxPolicyDecoder(t *testing.T) {
	const (
		meta     = `{"type":"session_meta","payload":{"id":"t1"}}` + "\n"
		readOnly = `{"type":"turn_context","payload":{"sandbox_policy":{"type":"read-only","network_access":false}}}` + "\n"
		noPolicy = `{"type":"turn_context","payload":{"approval_policy":"never"}}` + "\n"
	)

	t.Run("the policy of a spawned thread is the type its turn context records, with the line it is on [verifies SP-1519~1]", func(t *testing.T) {
		child := assembleRollouts(t).Children[0]
		if child.Sandbox.Type != "workspace-write" || child.Sandbox.Line != 2 || !child.Sandbox.Recorded() || child.Sandbox.ReadOnly() {
			t.Errorf("sandbox = %+v", child.Sandbox)
		}
	})

	t.Run("a read-only policy is recognised as read-only [verifies SP-1519~1]", func(t *testing.T) {
		policy := assembleChild(t, meta+readOnly).Sandbox
		if !policy.ReadOnly() || policy.Line != 2 {
			t.Errorf("sandbox = %+v", policy)
		}
	})

	t.Run("the first turn context that names a policy decides [verifies SP-1519~1]", func(t *testing.T) {
		policy := assembleChild(t, meta+noPolicy+readOnly+`{"type":"turn_context","payload":{"sandbox_policy":{"type":"danger-full-access"}}}`+"\n").Sandbox
		if policy.Type != evidence.SandboxPolicyReadOnly || policy.Line != 3 {
			t.Errorf("sandbox = %+v", policy)
		}
	})

	t.Run("a thread that records no policy has none, and the question stays open [verifies SP-1519~1]", func(t *testing.T) {
		for name, rollout := range map[string]string{
			"no turn context":         meta,
			"no policy in context":    meta + noPolicy,
			"a policy of no type":     meta + `{"type":"turn_context","payload":{"sandbox_policy":{}}}` + "\n",
			"a context not an object": meta + `{"type":"turn_context","payload":"text"}` + "\n",
		} {
			if policy := assembleChild(t, rollout).Sandbox; policy.Recorded() || policy.ReadOnly() {
				t.Errorf("%s: sandbox = %+v", name, policy)
			}
		}
	})
}
