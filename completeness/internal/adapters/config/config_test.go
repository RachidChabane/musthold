package config_test

import (
	"errors"
	"testing"

	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/config"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/policy"
)

func TestDecode(t *testing.T) {
	t.Run("an empty document gives the default policy [verifies SP-521~1]", func(t *testing.T) {
		for _, document := range []string{"", "policy: {}\n", "# nothing set\n"} {
			got, err := config.Decode([]byte(document))
			if err != nil || got != policy.Default() {
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
		want.TauAdv, want.MaxHunks = 0.8, 7
		if got != want {
			t.Fatalf("policy %+v, want %+v", got, want)
		}
	})

	t.Run("every parameter can be set [verifies SP-521~1]", func(t *testing.T) {
		got, err := config.Decode([]byte("policy:\n  tau_adv: 0.1\n  decide_k: 5\n  max_hunk_bytes: 100\n  max_hunks: 3\n"))
		if err != nil || got != (policy.Policy{TauAdv: 0.1, DecideK: 5, MaxHunkBytes: 100, MaxHunks: 3}) {
			t.Fatalf("policy %+v, %v", got, err)
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
