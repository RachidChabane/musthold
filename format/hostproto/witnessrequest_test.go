package hostproto_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const verifiesSP1983 = " [verifies SP-1983~1]"

// remoteLoader refuses every load: all resources are registered up front.
type remoteLoader struct{}

func (remoteLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("no remote schema: %s", url)
}

func requestSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(remoteLoader{})
	for _, schema := range append(format.Schemas(), hostproto.Schemas()...) {
		document, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema.Bytes))
		if err != nil {
			t.Fatalf("parse %s: %v", schema.ID, err)
		}
		if err := compiler.AddResource(schema.ID, document); err != nil {
			t.Fatalf("add %s: %v", schema.ID, err)
		}
	}
	compiled, err := compiler.Compile(hostproto.RequestType)
	if err != nil {
		t.Fatal(err)
	}
	return compiled
}

func schemaAcceptsRequest(t fataler, schema *jsonschema.Schema, request hostproto.Request) bool {
	t.Helper()
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	return schema.Validate(instance) == nil
}

// witnessRequest builds a valid `generate.witness` request, or the request a
// mutation makes of it.
func witnessRequest(mutations ...func(*hostproto.Request)) hostproto.Request {
	return hostprototest.GenerationRequest(append([]func(*hostproto.Request){func(r *hostproto.Request) {
		r.Kind = hostproto.KindGenerateWitness
	}}, mutations...)...)
}

// scopeRequest builds a valid `judge.scope` request, or the request a mutation
// makes of it.
func scopeRequest(mutations ...func(*hostproto.Request)) hostproto.Request {
	return hostprototest.Request(append([]func(*hostproto.Request){func(r *hostproto.Request) {
		r.Kind = hostproto.KindJudgeScope
	}}, mutations...)...)
}

func TestWitnessRequestKinds(t *testing.T) {
	round := 1
	cases := []struct {
		name    string
		request hostproto.Request
		want    []string
	}{
		{"a witness request from the generator with a round and a seed", witnessRequest(), nil},
		{"a witness request in a later round", witnessRequest(func(r *hostproto.Request) { *r.Round = 3 }), nil},
		{"a witness request without a round", witnessRequest(func(r *hostproto.Request) { r.Round = nil }), []string{"round"}},
		{"a witness request with round zero", witnessRequest(func(r *hostproto.Request) { *r.Round = 0 }), []string{"round"}},
		{"a witness request without a seed", witnessRequest(func(r *hostproto.Request) { r.Seed = "" }), []string{"seed"}},
		{"a witness request with a seed that is no digest", witnessRequest(func(r *hostproto.Request) { r.Seed = "seed" }), []string{"seed"}},
		{"a witness request from a judge", witnessRequest(func(r *hostproto.Request) { r.Role = hostproto.RoleFor(hostprototest.PluginName, hostproto.AgentJudge) }), []string{"role"}},
		{"a witness request from a decider", witnessRequest(func(r *hostproto.Request) {
			r.Role = hostproto.RoleFor(hostprototest.PluginName, hostproto.AgentDecider)
		}), []string{"role"}},
		{"a scope request from the judge with neither round nor seed", scopeRequest(), nil},
		{"a scope request with a round", scopeRequest(func(r *hostproto.Request) { r.Round = &round }), []string{"round"}},
		{"a scope request with a seed", scopeRequest(func(r *hostproto.Request) { r.Seed = hostprototest.Digest("seed") }), []string{"round"}},
		{"a scope request with a round and a seed", scopeRequest(func(r *hostproto.Request) {
			r.Round = &round
			r.Seed = hostprototest.Digest("seed")
		}), []string{"round"}},
		{"a scope request from the generator", scopeRequest(func(r *hostproto.Request) {
			r.Role = hostproto.RoleFor(hostprototest.PluginName, hostproto.AgentGenerator)
		}), []string{"role"}},
		{"a scope request from a decider", scopeRequest(func(r *hostproto.Request) {
			r.Role = hostproto.RoleFor(hostprototest.PluginName, hostproto.AgentDecider)
		}), []string{"role"}},
	}
	schema := requestSchema(t)
	for _, c := range cases {
		t.Run(c.name+" in the Go validator"+verifiesSP1983, func(t *testing.T) {
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
		t.Run(c.name+" in the JSON schema"+verifiesSP1983, func(t *testing.T) {
			if got, want := schemaAcceptsRequest(t, schema, c.request), c.want == nil; got != want {
				t.Errorf("schema accepts = %v, want %v", got, want)
			}
		})
	}

	t.Run("the generator serves the witness kind and the judge serves the scope kind"+verifiesSP1983, func(t *testing.T) {
		if hostproto.KindGenerateWitness.Agent() != hostproto.AgentGenerator || !hostproto.KindGenerateWitness.IsGeneration() {
			t.Errorf("generate.witness is served by %q", hostproto.KindGenerateWitness.Agent())
		}
		if hostproto.KindJudgeScope.Agent() != hostproto.AgentJudge || hostproto.KindJudgeScope.IsGeneration() {
			t.Errorf("judge.scope is served by %q", hostproto.KindJudgeScope.Agent())
		}
		if !slices.Contains(hostproto.Kinds(), hostproto.KindGenerateWitness) || !slices.Contains(hostproto.Kinds(), hostproto.KindJudgeScope) {
			t.Errorf("kinds = %v", hostproto.Kinds())
		}
	})
	t.Run("a witness or scope request is open and the schema and the validator agree on every combination"+verifiesSP1983, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			kind := rapid.SampledFrom([]hostproto.Kind{hostproto.KindGenerateWitness, hostproto.KindJudgeScope}).Draw(t, "kind")
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
