package policy_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/format"
)

const verifiesSP163 = " [verifies SP-163~1]"

const (
	outsideAbove = 1.01
	outsideBelow = -0.01
)

func TestDefaults(t *testing.T) {
	t.Run("the default policy carries the documented parameters"+verifiesSP163, func(t *testing.T) {
		got := policy.Default()
		want := policy.Policy{
			TauJudge: format.UniformThreshold(0.70), JudgeK: 2, TauScreen: format.UniformThreshold(0.20), DecideK: 3, MinValid: 3, MinSites: 2,
			MaxRounds: 3, KOp: 400, MaxHostTickets: 40,
			SignedPassBasis: policy.Corroborated, MinIsolation: format.IsolationRestricted,
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Default() = %+v, want %+v", got, want)
		}
	})
	t.Run("the default policy is valid"+verifiesSP163, func(t *testing.T) {
		if err := policy.Default().Validate(); err != nil {
			t.Errorf("Validate() = %v", err)
		}
	})
	t.Run("the pass bases are listed from the most conservative"+verifiesSP163, func(t *testing.T) {
		want := []policy.SignedPassBasis{policy.ExecutedOnly, policy.Corroborated, policy.Judged}
		if got := policy.Bases(); !slices.Equal(got, want) {
			t.Errorf("Bases() = %v, want %v", got, want)
		}
	})
	t.Run("the list of bases is a copy"+verifiesSP163, func(t *testing.T) {
		policy.Bases()[0] = policy.Judged
		if policy.Bases()[0] != policy.ExecutedOnly {
			t.Error("mutating the returned list changed the bases")
		}
	})
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name   string
		change func(*policy.Policy)
		valid  bool
	}{
		{"tau_judge keyed by a source kind", func(p *policy.Policy) {
			p.TauJudge = format.Threshold{Default: 0.7, BySource: map[format.ConfidenceSource]float64{format.SourceHostVotes: 0.9}}
		}, true},
		{"tau_screen keyed by an exact source", func(p *policy.Policy) {
			p.TauScreen = format.Threshold{Default: 0.2, BySource: map[format.ConfidenceSource]float64{format.HostVotesSource(3): 0.4}}
		}, true},
		{"tau_judge keyed by an unknown source", func(p *policy.Policy) {
			p.TauJudge = format.Threshold{Default: 0.7, BySource: map[format.ConfidenceSource]float64{"gut-feeling": 0.9}}
		}, false},
		{"tau_screen with an entry above one", func(p *policy.Policy) {
			p.TauScreen = format.Threshold{Default: 0.2, BySource: map[format.ConfidenceSource]float64{format.SourceSystemOne: outsideAbove}}
		}, false},
		{"tau_judge below zero", func(p *policy.Policy) { p.TauJudge = format.UniformThreshold(outsideBelow) }, false},
		{"tau_judge above one", func(p *policy.Policy) { p.TauJudge = format.UniformThreshold(outsideAbove) }, false},
		{"tau_screen below zero", func(p *policy.Policy) { p.TauScreen = format.UniformThreshold(outsideBelow) }, false},
		{"tau_screen above one", func(p *policy.Policy) { p.TauScreen = format.UniformThreshold(outsideAbove) }, false},
		{"thresholds at zero", func(p *policy.Policy) {
			p.TauJudge, p.TauScreen = format.UniformThreshold(0), format.UniformThreshold(0)
		}, true},
		{"thresholds at one", func(p *policy.Policy) {
			p.TauJudge, p.TauScreen = format.UniformThreshold(1), format.UniformThreshold(1)
		}, true},
		{"judge count zero", func(p *policy.Policy) { p.JudgeK = 0 }, false},
		{"decide count zero", func(p *policy.Policy) { p.DecideK = 0 }, false},
		{"min_valid zero", func(p *policy.Policy) { p.MinValid = 0 }, false},
		{"min_sites zero", func(p *policy.Policy) { p.MinSites = 0 }, false},
		{"rounds zero", func(p *policy.Policy) { p.MaxRounds = 0 }, false},
		{"K_op zero", func(p *policy.Policy) { p.KOp = 0 }, false},
		{"host tickets zero", func(p *policy.Policy) { p.MaxHostTickets = 0 }, false},
		{"negative count", func(p *policy.Policy) { p.MinValid = -3 }, false},
		{"counts of one", func(p *policy.Policy) {
			p.JudgeK, p.DecideK, p.MinValid, p.MinSites, p.MaxRounds, p.KOp, p.MaxHostTickets = 1, 1, 1, 1, 1, 1, 1
		}, true},
		{"unknown basis", func(p *policy.Policy) { p.SignedPassBasis = "trusting" }, false},
		{"empty basis", func(p *policy.Policy) { p.SignedPassBasis = "" }, false},
		{"executed-only basis", func(p *policy.Policy) { p.SignedPassBasis = policy.ExecutedOnly }, true},
		{"judged basis", func(p *policy.Policy) { p.SignedPassBasis = policy.Judged }, true},
		{"unknown isolation", func(p *policy.Policy) { p.MinIsolation = "sandboxed" }, false},
		{"empty isolation", func(p *policy.Policy) { p.MinIsolation = "" }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name+verifiesSP163, func(t *testing.T) {
			rules := policy.Default()
			tc.change(&rules)
			err := rules.Validate()
			if tc.valid && err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
			if !tc.valid && !errors.Is(err, policy.ErrInvalid) {
				t.Errorf("Validate() = %v, want ErrInvalid", err)
			}
		})
	}
	for _, level := range format.Isolations() {
		t.Run("the isolation floor "+string(level)+" is accepted"+verifiesSP163, func(t *testing.T) {
			rules := policy.Default()
			rules.MinIsolation = level
			if err := rules.Validate(); err != nil {
				t.Errorf("Validate() = %v", err)
			}
		})
	}
}

func TestDigest(t *testing.T) {
	t.Run("a policy without per-source thresholds keeps thresholds that are bare numbers"+verifiesSP163, func(t *testing.T) {
		encoded, err := json.Marshal(policy.Default())
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(encoded), `"tauJudge":0.7,`) || !strings.Contains(string(encoded), `"tauScreen":0.2,`) {
			t.Fatalf("policy = %s", encoded)
		}
	})
	t.Run("the digest is stable"+verifiesSP163, func(t *testing.T) {
		first, err := policy.Default().Digest()
		if err != nil {
			t.Fatal(err)
		}
		second, _ := policy.Default().Digest()
		if first != second || len(first) == 0 {
			t.Errorf("digests %q and %q", first, second)
		}
	})
	changes := []struct {
		name   string
		change func(*policy.Policy)
	}{
		{"tau_judge", func(p *policy.Policy) { p.TauJudge = format.UniformThreshold(0.71) }},
		{"tau_judge of a source", func(p *policy.Policy) {
			p.TauJudge.BySource = map[format.ConfidenceSource]float64{format.SourceHostVotes: 0.9}
		}},
		{"tau_screen of a source", func(p *policy.Policy) {
			p.TauScreen.BySource = map[format.ConfidenceSource]float64{format.SourceSystemOne: 0.4}
		}},
		{"judge_k", func(p *policy.Policy) { p.JudgeK++ }},
		{"tau_screen", func(p *policy.Policy) { p.TauScreen = format.UniformThreshold(0.21) }},
		{"decide_k", func(p *policy.Policy) { p.DecideK++ }},
		{"min_valid", func(p *policy.Policy) { p.MinValid++ }},
		{"min_sites", func(p *policy.Policy) { p.MinSites++ }},
		{"max_rounds", func(p *policy.Policy) { p.MaxRounds++ }},
		{"K_op", func(p *policy.Policy) { p.KOp++ }},
		{"host tickets", func(p *policy.Policy) { p.MaxHostTickets++ }},
		{"pass basis", func(p *policy.Policy) { p.SignedPassBasis = policy.Judged }},
		{"isolation floor", func(p *policy.Policy) { p.MinIsolation = format.IsolationUnrestricted }},
	}
	base, _ := policy.Default().Digest()
	seen := map[string]string{base: "default"}
	for _, tc := range changes {
		t.Run("changing "+tc.name+" gives another digest"+verifiesSP163, func(t *testing.T) {
			rules := policy.Default()
			tc.change(&rules)
			digest, err := rules.Digest()
			if err != nil {
				t.Fatal(err)
			}
			if digest == base {
				t.Errorf("the digest did not change")
			}
			if other, clash := seen[digest]; clash {
				t.Errorf("the digest equals that of %s", other)
			}
			seen[digest] = tc.name
		})
	}
}

func TestBypassesScreen(t *testing.T) {
	for _, basis := range policy.Bases() {
		t.Run("the "+string(basis)+" basis"+verifiesSP163, func(t *testing.T) {
			rules := policy.Default()
			rules.SignedPassBasis = basis
			if got, want := rules.BypassesScreen(), basis == policy.ExecutedOnly; got != want {
				t.Errorf("BypassesScreen() = %v, want %v", got, want)
			}
		})
	}
}
