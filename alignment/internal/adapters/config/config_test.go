package config_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/alignment/internal/adapters/config"
	"github.com/RachidChabane/musthold/alignment/internal/domain/policy"
	"github.com/RachidChabane/musthold/format"
)

func TestDecode(t *testing.T) {
	t.Run("an empty document gives the default policy [verifies SP-355~1]", func(t *testing.T) {
		got, err := config.Decode(nil)
		if err != nil || !reflect.DeepEqual(got, policy.Default()) {
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
			TauRoute: format.UniformThreshold(0.95), AlignK: 3, ModalMin: format.UniformThreshold(0.6), ThetaOwn: 0.4, ThetaOther: 0.3, ThetaNone: 0.1,
			ShadowRate: 0.25, MaxReasoningCalls: 24, DecideK: 5, TwoSignal: false, JudgeAlone: false, MinIsolation: format.IsolationInline,
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("policy %+v", got)
		}
	})

	t.Run("a threshold is a bare number or a mapping of sources, and a mapping keeps the default it does not set [verifies SP-903~1]", func(t *testing.T) {
		got, err := config.Decode([]byte("policy:\n  tau_route:\n    system-one: 0.97\n    host-votes: 0.8\n  modal_min:\n    default: 0.6\n    host-votes(5): 0.9\n"))
		if err != nil {
			t.Fatal(err)
		}
		endpoint := format.SystemOneSource("nimble:9b@" + strings.Repeat("a", format.Sha256HexLength))
		switch {
		case got.TauRoute.For(endpoint) != 0.97, got.TauRoute.For(format.HostVotesSource(3)) != 0.8, got.TauRoute.For(format.SourceExecution) != policy.DefaultTauRoute:
			t.Fatalf("tau_route %+v", got.TauRoute)
		case got.ModalMin.For(format.HostVotesSource(5)) != 0.9, got.ModalMin.For(format.HostVotesSource(4)) != 0.6:
			t.Fatalf("modal_min %+v", got.ModalMin)
		}
		const exact = "system-one:exact-model@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		keyed, err := config.Decode([]byte("policy:\n  tau_route:\n    system-one: 0.97\n    " + exact + ": 0.85\n"))
		relayed, parseErr := format.ParseSystemOneSource(exact)
		if err != nil || parseErr != nil || keyed.TauRoute.For(relayed) != 0.85 || keyed.TauRoute.For(endpoint) != 0.97 {
			t.Fatalf("tau_route %+v (%v, %v)", keyed.TauRoute, err, parseErr)
		}
	})

	t.Run("a threshold keyed by an unknown source or above one is refused [verifies SP-903~1]", func(t *testing.T) {
		for name, document := range map[string]string{
			"unknown source": "policy:\n  tau_route:\n    gut-feeling: 0.5\n",
			"above one":      "policy:\n  modal_min:\n    host-votes: 1.5\n",
		} {
			if _, err := config.Decode([]byte(document)); err == nil {
				t.Errorf("%s: accepted", name)
			}
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
