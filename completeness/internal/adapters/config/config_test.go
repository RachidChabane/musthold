package config_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/completeness/internal/adapters/config"
	"github.com/RachidChabane/musthold/completeness/internal/domain/policy"
	"github.com/RachidChabane/musthold/format"
)

func TestDecode(t *testing.T) {
	t.Run("an empty document gives the default policy [verifies SP-521~1]", func(t *testing.T) {
		for _, document := range []string{"", "policy: {}\n", "# nothing set\n"} {
			got, err := config.Decode([]byte(document))
			if err != nil || !reflect.DeepEqual(got, policy.Default()) {
				t.Errorf("%q: %+v, %v", document, got, err)
			}
		}
	})

	t.Run("a parameter that is set replaces its default and the others keep theirs [verifies SP-521~1]", func(t *testing.T) {
		got, err := config.Decode([]byte("policy:\n  tau_adv: 0.8\n  max_hunks: 7\n"))
		if err != nil {
			t.Fatal(err)
		}
		want := policy.Default()
		want.TauAdv, want.MaxHunks = format.UniformThreshold(0.8), 7
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("policy %+v, want %+v", got, want)
		}
	})

	t.Run("every parameter can be set [verifies SP-521~1]", func(t *testing.T) {
		got, err := config.Decode([]byte("policy:\n  tau_adv: 0.1\n  decide_k: 5\n  max_hunk_bytes: 100\n  max_hunks: 3\n"))
		if err != nil || !reflect.DeepEqual(got, policy.Policy{TauAdv: format.UniformThreshold(0.1), DecideK: 5, MaxHunkBytes: 100, MaxHunks: 3}) {
			t.Fatalf("policy %+v, %v", got, err)
		}
	})

	t.Run("tau_adv is a bare number or a mapping of sources, and a mapping keeps the default it does not set [verifies SP-904~1]", func(t *testing.T) {
		got, err := config.Decode([]byte("policy:\n  tau_adv:\n    system-one: 0.9\n    host-votes(3): 0.7\n"))
		if err != nil {
			t.Fatal(err)
		}
		endpoint := format.SystemOneSource("nimble:9b@" + strings.Repeat("a", format.Sha256HexLength))
		if got.TauAdv.For(endpoint) != 0.9 || got.TauAdv.For(format.HostVotesSource(3)) != 0.7 || got.TauAdv.For(format.HostVotesSource(2)) != policy.DefaultTauAdv {
			t.Fatalf("tau_adv %+v", got.TauAdv)
		}
		const exact = "system-one:exact-model@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		keyed, err := config.Decode([]byte("policy:\n  tau_adv:\n    system-one: 0.9\n    " + exact + ": 0.85\n"))
		relayed, parseErr := format.ParseSystemOneSource(exact)
		if err != nil || parseErr != nil || keyed.TauAdv.For(relayed) != 0.85 || keyed.TauAdv.For(endpoint) != 0.9 {
			t.Fatalf("tau_adv %+v (%v, %v)", keyed.TauAdv, err, parseErr)
		}
		for name, document := range map[string]string{
			"unknown source": "policy:\n  tau_adv:\n    gut-feeling: 0.5\n",
			"above one":      "policy:\n  tau_adv:\n    host-votes: 1.5\n",
		} {
			if _, err := config.Decode([]byte(document)); err == nil {
				t.Errorf("%s: accepted", name)
			}
		}
	})

	t.Run("an unknown key, an invalid parameter and a malformed document are refused [verifies SP-521~1]", func(t *testing.T) {
		if _, err := config.Decode([]byte("policy:\n  tau_route: 0.9\n")); err == nil {
			t.Error("an unknown key was accepted")
		}
		if _, err := config.Decode([]byte("policy:\n  tau_adv: 1.5\n")); !errors.Is(err, policy.ErrInvalid) {
			t.Errorf("an invalid parameter: %v", err)
		}
		if _, err := config.Decode([]byte("policy: [")); err == nil {
			t.Error("a malformed document was accepted")
		}
	})
}
