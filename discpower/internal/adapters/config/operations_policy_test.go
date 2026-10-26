package config

import (
	"errors"
	"maps"
	"reflect"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/policy"
	"github.com/RachidChabane/musthold/discpower/internal/domain/projectconfig"
	"github.com/RachidChabane/musthold/format"
)

const (
	quoteTarget     = "parkfee:quote"
	languageTarget  = "parkfee_py:quote"
	exactTarget     = "parkfee_pytest:quote"
	totalTarget     = "parkfee:total"
	targetPattern   = `[a-z]{1,8}:[a-z_]{1,8}`
	operationNaming = `[a-z]{1,8}`
	headerYAML      = "proofs:\n  alignment: false\n  discriminating-power: false\n  completeness: false\necosystems:\n  - pytest\n"
)

func policyOf(t *testing.T, document string) policy.Policy {
	t.Helper()
	settings, err := Decode([]byte(document))
	if err != nil {
		t.Fatal(err)
	}
	return settings.Policy()
}

func TestDecodeOperations(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		ecosystem ecosystem.Name
		want      map[string]string
	}{
		{
			name:      "an operation bound under the ecosystem name [verifies SP-191~1]",
			yaml:      "operations:\n  quote:\n    pytest: " + quoteTarget + "\n",
			ecosystem: ecosystem.Pytest,
			want:      map[string]string{"quote": quoteTarget},
		},
		{
			name:      "an operation bound under the language of the ecosystem [verifies SP-191~1]",
			yaml:      "operations:\n  quote:\n    python: " + quoteTarget + "\n",
			ecosystem: ecosystem.Pytest,
			want:      map[string]string{"quote": quoteTarget},
		},
		{
			name:      "the ecosystem name wins over its language [verifies SP-191~1]",
			yaml:      "operations:\n  quote:\n    python: " + languageTarget + "\n    pytest: " + exactTarget + "\n",
			ecosystem: ecosystem.Pytest,
			want:      map[string]string{"quote": exactTarget},
		},
		{
			name:      "a binding of another language is not applied [verifies SP-191~1]",
			yaml:      "operations:\n  quote:\n    go: " + quoteTarget + "\n",
			ecosystem: ecosystem.Pytest,
			want:      map[string]string{},
		},
		{
			name:      "several operations are bound independently [verifies SP-191~1]",
			yaml:      "operations:\n  quote:\n    python: " + quoteTarget + "\n  total:\n    pytest: " + totalTarget + "\n",
			ecosystem: ecosystem.Pytest,
			want:      map[string]string{"quote": quoteTarget, "total": totalTarget},
		},
		{
			name:      "one language serves several ecosystems [verifies SP-191~1]",
			yaml:      "operations:\n  quote:\n    typescript: " + quoteTarget + "\n",
			ecosystem: ecosystem.Vitest,
			want:      map[string]string{"quote": quoteTarget},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			settings, err := Decode([]byte(test.yaml))
			if err != nil {
				t.Fatal(err)
			}
			if got := settings.OperationsFor(test.ecosystem); !maps.Equal(got, test.want) {
				t.Fatalf("operations = %v, want %v", got, test.want)
			}
		})
	}
}

func TestDecodePolicy(t *testing.T) {
	t.Run("parameters are applied over the defaults and the others keep them [verifies SP-191~1]", func(t *testing.T) {
		const (
			tauJudge = 0.9
			judgeK   = 5
			maxTotal = 12
		)
		got := policyOf(t, "policy:\n  tau_judge: 0.9\n  judge_k: 5\n  max_host_tickets: 12\n  signed_pass_basis: judged\n  min_isolation: inline\n")
		want := policy.Default()
		want.TauJudge, want.JudgeK, want.MaxHostTickets = format.UniformThreshold(tauJudge), judgeK, maxTotal
		want.SignedPassBasis, want.MinIsolation = policy.Judged, format.IsolationInline
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("policy = %+v, want %+v", got, want)
		}
	})

	t.Run("a threshold is a bare number or a mapping of sources, and a mapping keeps the default it does not set [verifies SP-902~1]", func(t *testing.T) {
		got := policyOf(t, "policy:\n  tau_judge:\n    default: 0.6\n    host-votes: 0.8\n    host-votes(2): 0.95\n  tau_screen:\n    system-one: 0.4\n")
		endpoint := format.SystemOneSource("nimble:9b@" + strings.Repeat("a", format.Sha256HexLength))
		switch {
		case got.TauJudge.For(format.HostVotesSource(2)) != 0.95, got.TauJudge.For(format.HostVotesSource(3)) != 0.8, got.TauJudge.For(endpoint) != 0.6:
			t.Fatalf("tau_judge = %+v", got.TauJudge)
		case got.TauScreen.For(endpoint) != 0.4, got.TauScreen.For(format.HostVotesSource(3)) != policy.DefaultTauScreen:
			t.Fatalf("tau_screen = %+v", got.TauScreen)
		}
	})

	t.Run("a threshold keyed on the exact source of a relayed answer holds that answer [verifies SP-902~1]", func(t *testing.T) {
		const exact = "system-one:exact-model@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		got := policyOf(t, "policy:\n  tau_screen:\n    default: 0.5\n    system-one: 0.4\n    "+exact+": 0.85\n")
		relayed, err := format.ParseSystemOneSource(exact)
		if err != nil || got.TauScreen.For(relayed) != 0.85 || got.TauScreen.For(format.SystemOneSource("tev1:4b@"+"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")) != 0.4 {
			t.Fatalf("tau_screen = %+v (%v)", got.TauScreen, err)
		}
	})

	t.Run("a document without policy gives the default policy [verifies SP-191~1]", func(t *testing.T) {
		if got := policyOf(t, headerYAML); !reflect.DeepEqual(got, policy.Default()) {
			t.Fatalf("policy = %+v", got)
		}
	})

	t.Run("every integer and threshold parameter is applied [verifies SP-191~1]", func(t *testing.T) {
		got := policyOf(t, "policy:\n  tau_screen: 0.3\n  decide_k: 4\n  min_valid: 5\n  min_sites: 6\n  max_rounds: 7\n  k_op: 8\n")
		want := policy.Default()
		want.TauScreen, want.DecideK, want.MinValid, want.MinSites, want.MaxRounds, want.KOp = format.UniformThreshold(0.3), 4, 5, 6, 7, 8
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("policy = %+v, want %+v", got, want)
		}
	})

	refused := []struct {
		name string
		yaml string
	}{
		{"a threshold above one", "policy:\n  tau_judge: 1.5\n"},
		{"a threshold keyed by an unknown source", "policy:\n  tau_judge:\n    gut-feeling: 0.5\n"},
		{"a source threshold above one", "policy:\n  tau_screen:\n    host-votes: 1.5\n"},
		{"a negative threshold", "policy:\n  tau_screen: -0.1\n"},
		{"an unknown signed pass basis", "policy:\n  signed_pass_basis: trusting\n"},
		{"an unknown isolation level", "policy:\n  min_isolation: telepathic\n"},
		{"a count below one", "policy:\n  judge_k: 0\n"},
		{"an unknown key inside the policy", "policy:\n  tau_judges: 0.5\n"},
		{"a parameter of the wrong type", "policy:\n  judge_k: many\n"},
		{"an operation that is no mapping of bindings", "operations:\n  quote: parkfee\n"},
	}
	for _, test := range refused {
		t.Run(test.name+" is refused [verifies SP-191~1]", func(t *testing.T) {
			if _, err := Decode([]byte(test.yaml)); err == nil {
				t.Fatal("the document was accepted")
			}
		})
	}

	t.Run("an out of range parameter is an invalid settings error [verifies SP-191~1]", func(t *testing.T) {
		_, err := Decode([]byte("policy:\n  tau_judge: 1.5\n"))
		if !errors.Is(err, projectconfig.ErrInvalidSettings) {
			t.Fatalf("error %v", err)
		}
	})
}

func TestEncodeOperationsAndPolicy(t *testing.T) {
	t.Run("operations and policy are written after the ecosystems in a fixed order [verifies SP-191~1]", func(t *testing.T) {
		settings := projectconfig.Default([]ecosystem.Name{ecosystem.Pytest})
		settings.Operations = map[string]map[string]string{
			"total": {"python": totalTarget},
			"quote": {"pytest": exactTarget, "python": languageTarget},
		}
		tau, judgeK, basis, isolation := uniform(0.9), 5, policy.Judged, format.IsolationInline
		settings.PolicyOverrides = projectconfig.PolicyOverrides{MinIsolation: &isolation, SignedPassBasis: &basis, JudgeK: &judgeK, TauJudge: tau}

		want := headerYAML +
			"operations:\n  quote:\n    pytest: " + exactTarget + "\n    python: " + languageTarget + "\n  total:\n    python: " + totalTarget + "\n" +
			"policy:\n  tau_judge: 0.9\n  judge_k: 5\n  signed_pass_basis: judged\n  min_isolation: inline\n"
		got, err := Encode(settings)
		if err != nil || string(got) != want {
			t.Fatalf("encoded %q (%v), want %q", got, err, want)
		}
	})

	t.Run("settings without operations and policy write neither [verifies SP-191~1]", func(t *testing.T) {
		got, err := Encode(projectconfig.Default([]ecosystem.Name{ecosystem.Pytest}))
		if err != nil || string(got) != headerYAML {
			t.Fatalf("encoded %q (%v)", got, err)
		}
	})
}

func overridesGenerator() *rapid.Generator[projectconfig.PolicyOverrides] {
	thresholds := rapid.SampledFrom([]float64{0, 0.2, 0.5, 0.7, 1})
	thresholdOverrides := rapid.Custom(func(t *rapid.T) format.ThresholdOverride {
		override := format.ThresholdOverride{}
		if rapid.Bool().Draw(t, "has default") {
			override.Default = ptr(thresholds.Draw(t, "default"))
		}
		if rapid.Bool().Draw(t, "has entries") || override.Default == nil {
			override.BySource = map[format.ConfidenceSource]float64{
				rapid.SampledFrom([]format.ConfidenceSource{format.SourceHostVotes, format.SourceSystemOne, format.HostVotesSource(2)}).Draw(t, "source"): thresholds.Draw(t, "entry"),
			}
		}
		return override
	})
	counts := rapid.IntRange(1, 500)
	return rapid.Custom(func(t *rapid.T) projectconfig.PolicyOverrides {
		var overrides projectconfig.PolicyOverrides
		if rapid.Bool().Draw(t, "tau_judge") {
			overrides.TauJudge = ptr(thresholdOverrides.Draw(t, "tau judge"))
		}
		if rapid.Bool().Draw(t, "tau_screen") {
			overrides.TauScreen = ptr(thresholdOverrides.Draw(t, "tau screen"))
		}
		for name, target := range map[string]**int{
			"judge_k": &overrides.JudgeK, "decide_k": &overrides.DecideK, "min_valid": &overrides.MinValid,
			"min_sites": &overrides.MinSites, "max_rounds": &overrides.MaxRounds, "k_op": &overrides.KOp,
			"max_host_tickets": &overrides.MaxHostTickets,
		} {
			if rapid.Bool().Draw(t, name) {
				*target = ptr(counts.Draw(t, name))
			}
		}
		if rapid.Bool().Draw(t, "basis") {
			overrides.SignedPassBasis = ptr(rapid.SampledFrom(policy.Bases()).Draw(t, "basis value"))
		}
		if rapid.Bool().Draw(t, "isolation") {
			overrides.MinIsolation = ptr(rapid.SampledFrom(format.Isolations()).Draw(t, "isolation value"))
		}
		return overrides
	})
}

func ptr[V any](value V) *V { return &value }

// uniform is a threshold override that sets only the default.
func uniform(value float64) *format.ThresholdOverride {
	return &format.ThresholdOverride{Default: &value}
}

func TestOperationsAndPolicyRoundTrip(t *testing.T) {
	t.Run("encoding then decoding keeps the operations and the policy and is deterministic [verifies SP-191~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			settings := projectconfig.Default(rapid.SliceOf(rapid.SampledFrom(ecosystem.Names())).Draw(t, "ecosystems"))
			settings.Operations = rapid.MapOfN(
				rapid.StringMatching(operationNaming),
				rapid.MapOfN(rapid.SampledFrom([]string{"python", "pytest", "go", "java", "typescript"}), rapid.StringMatching(targetPattern), 1, 3),
				0, 3,
			).Draw(t, "operations")
			settings.PolicyOverrides = overridesGenerator().Draw(t, "policy")

			encoded, err := Encode(settings)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := Decode(encoded)
			if err != nil {
				t.Fatalf("decode %q: %v", encoded, err)
			}
			if !reflect.DeepEqual(decoded.Policy(), settings.Policy()) {
				t.Fatalf("policy %+v became %+v", settings.Policy(), decoded.Policy())
			}
			for _, name := range ecosystem.Names() {
				if !maps.Equal(decoded.OperationsFor(name), settings.OperationsFor(name)) {
					t.Fatalf("operations of %s: %v became %v", name, settings.OperationsFor(name), decoded.OperationsFor(name))
				}
			}
			again, err := Encode(decoded)
			if err != nil || string(again) != string(encoded) {
				t.Fatalf("encoding is not stable: %q vs %q (%v)", encoded, again, err)
			}
		})
	})
}
