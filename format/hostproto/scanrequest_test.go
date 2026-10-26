package hostproto_test

import (
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
)

const verifiesSP2160 = " [verifies SP-2160~1]"

// promisesRequest builds a valid `generate.promises` request, or the request a
// mutation makes of it.
func promisesRequest(mutations ...func(*hostproto.Request)) hostproto.Request {
	return hostprototest.GenerationRequest(append([]func(*hostproto.Request){func(r *hostproto.Request) {
		r.Kind = hostproto.KindGeneratePromises
	}}, mutations...)...)
}

// labelsRequest builds a valid `judge.labels` request, or the request a
// mutation makes of it.
func labelsRequest(mutations ...func(*hostproto.Request)) hostproto.Request {
	return hostprototest.Request(append([]func(*hostproto.Request){func(r *hostproto.Request) {
		r.Kind = hostproto.KindJudgeLabels
	}}, mutations...)...)
}

func TestScanRequestKinds(t *testing.T) {
	schema := requestSchema(t)
	round := 1
	cases := []struct {
		name    string
		request hostproto.Request
		want    []string
	}{
		{"a promises request from the generator with a round and a seed", promisesRequest(), nil},
		{"a promises request without a round", promisesRequest(func(r *hostproto.Request) { r.Round = nil }), []string{"round"}},
		{"a promises request without a seed", promisesRequest(func(r *hostproto.Request) { r.Seed = "" }), []string{"seed"}},
		{"a promises request from a judge", promisesRequest(func(r *hostproto.Request) {
			r.Role = hostproto.RoleFor(hostprototest.PluginName, hostproto.AgentJudge)
		}), []string{"role"}},
		{"a labels request from the judge with neither round nor seed", labelsRequest(), nil},
		{"a labels request with a round", labelsRequest(func(r *hostproto.Request) { r.Round = &round }), []string{"round"}},
		{"a labels request with a seed", labelsRequest(func(r *hostproto.Request) { r.Seed = hostprototest.Digest("seed") }), []string{"round"}},
		{"a labels request from the generator", labelsRequest(func(r *hostproto.Request) {
			r.Role = hostproto.RoleFor(hostprototest.PluginName, hostproto.AgentGenerator)
		}), []string{"role"}},
		{"a labels request from a decider", labelsRequest(func(r *hostproto.Request) {
			r.Role = hostproto.RoleFor(hostprototest.PluginName, hostproto.AgentDecider)
		}), []string{"role"}},
		{"a labels request with a closed shape", labelsRequest(func(r *hostproto.Request) { r.Shape = hostproto.ShapeClosed }), []string{"closed", "shape"}},
	}
	for _, c := range cases {
		t.Run(c.name+" in the Go validator"+verifiesSP2160, func(t *testing.T) {
			err := c.request.Validate()
			if c.want == nil {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			got := problemPaths(t, err)
			want := slices.Clone(c.want)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Fatalf("problems at %v, want %v (%v)", got, want, err)
			}
		})
		t.Run(c.name+" in the JSON schema"+verifiesSP2160, func(t *testing.T) {
			if got, want := schemaAcceptsRequest(t, schema, c.request), c.want == nil; got != want {
				t.Errorf("schema accepts = %v, want %v", got, want)
			}
		})
	}

	t.Run("the generator serves the promises kind and the judge serves the labels kind"+verifiesSP2160, func(t *testing.T) {
		if hostproto.KindGeneratePromises.Agent() != hostproto.AgentGenerator || !hostproto.KindGeneratePromises.IsGeneration() {
			t.Errorf("generate.promises is served by %q", hostproto.KindGeneratePromises.Agent())
		}
		if hostproto.KindJudgeLabels.Agent() != hostproto.AgentJudge || hostproto.KindJudgeLabels.IsGeneration() {
			t.Errorf("judge.labels is served by %q", hostproto.KindJudgeLabels.Agent())
		}
		for _, kind := range []hostproto.Kind{hostproto.KindGeneratePromises, hostproto.KindJudgeLabels} {
			if !slices.Contains(hostproto.Kinds(), kind) {
				t.Errorf("kinds %v lack %s", hostproto.Kinds(), kind)
			}
		}
		if string(hostproto.KindGeneratePromises) != "generate.promises" || string(hostproto.KindJudgeLabels) != "judge.labels" {
			t.Errorf("names %q and %q", hostproto.KindGeneratePromises, hostproto.KindJudgeLabels)
		}
	})
	t.Run("the kinds are distinct"+verifiesSP2160, func(t *testing.T) {
		kinds := hostproto.Kinds()
		slices.Sort(kinds)
		if len(slices.Compact(kinds)) != len(hostproto.Kinds()) {
			t.Errorf("kinds %v", hostproto.Kinds())
		}
	})
	t.Run("the request id ignores the module version and tells the new kinds apart"+verifiesSP2160, func(t *testing.T) {
		for _, build := range []func(...func(*hostproto.Request)) hostproto.Request{promisesRequest, labelsRequest} {
			request := build()
			bumped := build(func(r *hostproto.Request) { r.Module.Version = "9.9.9" })
			if hostprototest.MustID(bumped) != hostprototest.MustID(request) {
				t.Errorf("the id of a %s request depends on the module version", request.Kind)
			}
		}
		if hostprototest.MustID(promisesRequest()) == hostprototest.MustID(labelsRequest(func(r *hostproto.Request) {
			r.Round = promisesRequest().Round
			r.Seed = promisesRequest().Seed
		})) {
			t.Error("a promises request and a labels request share an id")
		}
	})
	t.Run("the validator and the schema agree on every kind, role, round and seed"+verifiesSP2160, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			kind := rapid.SampledFrom([]hostproto.Kind{hostproto.KindGeneratePromises, hostproto.KindJudgeLabels}).Draw(t, "kind")
			agent := rapid.SampledFrom(hostproto.Agents()).Draw(t, "agent")
			withRound := rapid.Bool().Draw(t, "round")
			withSeed := rapid.Bool().Draw(t, "seed")
			request := hostprototest.Request(func(r *hostproto.Request) {
				r.Kind = kind
				r.Role = hostproto.RoleFor(hostprototest.PluginName, agent)
				if withRound {
					r.Round = &round
				}
				if withSeed {
					r.Seed = hostprototest.Digest("seed")
				}
			})
			want := agent == kind.Agent() && withRound == kind.IsGeneration() && withSeed == kind.IsGeneration()
			if got := request.Validate() == nil; got != want {
				t.Fatalf("%s from %s, round %v, seed %v: validator accepts = %v", kind, agent, withRound, withSeed, got)
			}
			if got := schemaAcceptsRequest(t, schema, request); got != want {
				t.Fatalf("%s from %s, round %v, seed %v: schema accepts = %v", kind, agent, withRound, withSeed, got)
			}
		})
	})
}
