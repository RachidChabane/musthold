package policy_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/alignment/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/format"
)

func TestPolicy(t *testing.T) {
	t.Run("the default policy is the policy of the ADRs and is valid [verifies SP-317~1]", func(t *testing.T) {
		p := policy.Default()
		if err := p.Validate(); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(p.TauRoute, format.UniformThreshold(0.90)) || p.AlignK != 5 || !reflect.DeepEqual(p.ModalMin, format.UniformThreshold(0.80)) || p.ThetaOwn != 0.50 || p.ThetaOther != 0.20 ||
			p.ThetaNone != 0.20 || p.ShadowRate != 0.10 || p.MaxReasoningCalls != 40 || p.DecideK != 3 || !p.TwoSignal || !p.JudgeAlone {
			t.Fatalf("defaults %+v", p)
		}
	})

	t.Run("a threshold outside [0,1], a count below one and an unknown isolation are refused [verifies SP-317~1]", func(t *testing.T) {
		broken := map[string]func(*policy.Policy){
			"tau_route": func(p *policy.Policy) { p.TauRoute = format.UniformThreshold(1.1) },
			"modal_min": func(p *policy.Policy) { p.ModalMin = format.UniformThreshold(-0.1) },
			"tau_route of a source above one": func(p *policy.Policy) {
				p.TauRoute.BySource = map[format.ConfidenceSource]float64{format.SourceSystemOne: 1.1}
			},
			"modal_min of an unknown source": func(p *policy.Policy) {
				p.ModalMin.BySource = map[format.ConfidenceSource]float64{"gut-feeling": 0.5}
			},
			"shadow_rate":   func(p *policy.Policy) { p.ShadowRate = 2 },
			"align_k":       func(p *policy.Policy) { p.AlignK = 0 },
			"decide_k":      func(p *policy.Policy) { p.DecideK = 0 },
			"budget":        func(p *policy.Policy) { p.MaxReasoningCalls = 0 },
			"min_isolation": func(p *policy.Policy) { p.MinIsolation = "paranoid" },
		}
		for name, change := range broken {
			p := policy.Default()
			change(&p)
			if err := p.Validate(); !errors.Is(err, policy.ErrInvalid) {
				t.Errorf("%s: error %v", name, err)
			}
		}
	})

	t.Run("a threshold of one source reaches the digest and a policy without any keeps bare numbers [verifies SP-903~1]", func(t *testing.T) {
		base, err := policy.Default().Digest()
		if err != nil {
			t.Fatal(err)
		}
		for name, change := range map[string]func(*policy.Policy){
			"tau_route": func(p *policy.Policy) {
				p.TauRoute.BySource = map[format.ConfidenceSource]float64{format.SourceSystemOne: 0.97}
			},
			"modal_min": func(p *policy.Policy) {
				p.ModalMin.BySource = map[format.ConfidenceSource]float64{format.SourceHostVotes: 0.9}
			},
		} {
			p := policy.Default()
			change(&p)
			if digest, err := p.Digest(); err != nil || digest == base {
				t.Errorf("%s: digest %q (%v)", name, digest, err)
			}
		}
		encoded, err := json.Marshal(policy.Default())
		if err != nil || !strings.Contains(string(encoded), `"tauRoute":0.9,`) || !strings.Contains(string(encoded), `"modalMin":0.8,`) {
			t.Fatalf("policy %s (%v)", encoded, err)
		}
	})

	t.Run("every parameter reaches the digest [verifies SP-317~1]", func(t *testing.T) {
		base, err := policy.Default().Digest()
		if err != nil {
			t.Fatal(err)
		}
		changes := []func(*policy.Policy){
			func(p *policy.Policy) { p.TauRoute = format.UniformThreshold(0.95) },
			func(p *policy.Policy) { p.AlignK = 3 },
			func(p *policy.Policy) { p.ModalMin = format.UniformThreshold(0.6) },
			func(p *policy.Policy) { p.ThetaOwn = 0.4 },
			func(p *policy.Policy) { p.ThetaOther = 0.3 },
			func(p *policy.Policy) { p.ThetaNone = 0.3 },
			func(p *policy.Policy) { p.ShadowRate = 0.25 },
			func(p *policy.Policy) { p.MaxReasoningCalls = 24 },
			func(p *policy.Policy) { p.DecideK = 5 },
			func(p *policy.Policy) { p.TwoSignal = false },
			func(p *policy.Policy) { p.JudgeAlone = false },
			func(p *policy.Policy) { p.MinIsolation = format.IsolationInline },
		}
		fields := reflect.TypeOf(policy.Policy{}).NumField()
		if len(changes) != fields {
			t.Fatalf("%d changes for %d parameters", len(changes), fields)
		}
		for index, change := range changes {
			p := policy.Default()
			change(&p)
			digest, err := p.Digest()
			if err != nil || digest == base {
				t.Errorf("change %d left the digest at %s (%v)", index, digest, err)
			}
		}
	})
}
