package config_test

import (
	"testing"

	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/config"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/format"
)

func TestDecode(t *testing.T) {
	t.Run("an empty document gives the default policy [verifies SP-355~1]", func(t *testing.T) {
		got, err := config.Decode(nil)
		if err != nil || got != policy.Default() {
			t.Fatalf("policy %+v: %v", got, err)
		}
	})

	t.Run("every parameter can be set [verifies SP-355~1]", func(t *testing.T) {
		got, err := config.Decode([]byte(`
policy:
  tau_route: 0.95
  align_k: 3
  modal_min: 0.6
  theta_own: 0.4
  theta_other: 0.3
  theta_none: 0.1
  shadow_rate: 0.25
  max_reasoning_calls: 24
  decide_k: 5
  two_signal: false
  judge_alone: false
  min_isolation: inline
`))
		if err != nil {
			t.Fatal(err)
		}
		want := policy.Policy{
			TauRoute: 0.95, AlignK: 3, ModalMin: 0.6, ThetaOwn: 0.4, ThetaOther: 0.3, ThetaNone: 0.1,
			ShadowRate: 0.25, MaxReasoningCalls: 24, DecideK: 5, TwoSignal: false, JudgeAlone: false, MinIsolation: format.IsolationInline,
		}
		if got != want {
			t.Fatalf("policy %+v", got)
		}
	})

	t.Run("an unknown key, a wrong type and an invalid parameter are refused [verifies SP-355~1]", func(t *testing.T) {
		for name, document := range map[string]string{
			"unknown key":     "policy:\n  tau_judge: 0.7\n",
			"unknown section": "proofs:\n  alignment: true\n",
			"wrong type":      "policy:\n  align_k: many\n",
			"out of range":    "policy:\n  shadow_rate: 3\n",
			"unknown level":   "policy:\n  min_isolation: paranoid\n",
		} {
			if _, err := config.Decode([]byte(document)); err == nil {
				t.Errorf("%s: accepted", name)
			}
		}
	})
}
