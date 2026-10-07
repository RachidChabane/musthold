package policy_test

import (
	"errors"
	"testing"

	"github.com/RachidChabane/spec-proof/completeness/internal/domain/policy"
)

func TestDefault(t *testing.T) {
	t.Run("the defaults are the parameters of the ADRs and are valid [verifies SP-520~1]", func(t *testing.T) {
		got := policy.Default()
		if got.TauAdv != 0.5 || got.DecideK != 3 || got.MaxHunkBytes != 6000 || got.MaxHunks != 50 {
			t.Fatalf("default %+v", got)
		}
		if err := got.Validate(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestValidate(t *testing.T) {
	t.Run("a threshold outside [0,1] and a count below one are refused [verifies SP-520~1]", func(t *testing.T) {
		mutations := map[string]func(*policy.Policy){
			"tau below":   func(p *policy.Policy) { p.TauAdv = -0.01 },
			"tau above":   func(p *policy.Policy) { p.TauAdv = 1.01 },
			"decide_k":    func(p *policy.Policy) { p.DecideK = 0 },
			"hunk bytes":  func(p *policy.Policy) { p.MaxHunkBytes = 0 },
			"hunk budget": func(p *policy.Policy) { p.MaxHunks = 0 },
		}
		for name, mutate := range mutations {
			candidate := policy.Default()
			mutate(&candidate)
			if err := candidate.Validate(); !errors.Is(err, policy.ErrInvalid) {
				t.Errorf("%s: error %v", name, err)
			}
		}
	})

	t.Run("the bounds of the domain are accepted [verifies SP-520~1]", func(t *testing.T) {
		for _, tau := range []float64{0, 1} {
			candidate := policy.Default()
			candidate.TauAdv = tau
			if err := candidate.Validate(); err != nil {
				t.Errorf("tau %v: %v", tau, err)
			}
		}
	})
}

func TestDigest(t *testing.T) {
	t.Run("every parameter reaches the digest [verifies SP-520~1]", func(t *testing.T) {
		base, err := policy.Default().Digest()
		if err != nil {
			t.Fatal(err)
		}
		changes := map[string]func(*policy.Policy){
			"tau":        func(p *policy.Policy) { p.TauAdv = 0.6 },
			"decide_k":   func(p *policy.Policy) { p.DecideK = 5 },
			"hunk bytes": func(p *policy.Policy) { p.MaxHunkBytes = 100 },
			"hunks":      func(p *policy.Policy) { p.MaxHunks = 3 },
		}
		for name, mutate := range changes {
			candidate := policy.Default()
			mutate(&candidate)
			digest, err := candidate.Digest()
			if err != nil || digest == base {
				t.Errorf("%s: digest %q err %v", name, digest, err)
			}
		}
		again, _ := policy.Default().Digest()
		if again != base {
			t.Fatal("the digest of the same policy differs")
		}
	})
}
