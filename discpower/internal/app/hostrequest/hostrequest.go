// Package hostrequest assembles the host requests every use case of the module
// emits (ADR 0005): the module that asks, the executor role that serves the
// kind, the shape, the output limit, the sorted inputs and the cache key that
// names the work. `prove` and `scan` share it, so that one kind of work is
// always asked the same way and gets the same request id.
package hostrequest

import (
	"cmp"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/domain/prompt"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
)

// Identity is who asks: the module, the plugin its executor roles belong to,
// and the digest of the decision policy the answers are read under.
type Identity struct {
	Module       format.Module
	Plugin       string
	PolicyDigest string
}

// Draft is what a request is assembled from: its kind, the built prompt, the
// inputs it depends on and, for a generation kind, its round and seed.
type Draft struct {
	Kind   hostproto.Kind
	Built  prompt.Built
	Inputs []hostproto.Input
	Round  int
	Seed   string
}

// Assemble builds the request of a draft. The prompt template and the decision
// policy are inputs of every request; the inputs are sorted and deduplicated,
// and the cache key is the digest of the kind and the inputs.
func Assemble(identity Identity, draft Draft) (hostproto.Request, error) {
	inputs := append([]hostproto.Input{
		{Role: format.RolePromptTemplate, Digest: draft.Built.Prompt.TemplateDigest},
		{Role: format.RoleDecisionPolicy, Digest: identity.PolicyDigest},
	}, draft.Inputs...)
	slices.SortFunc(inputs, func(a, b hostproto.Input) int {
		return cmp.Or(strings.Compare(string(a.Role), string(b.Role)), strings.Compare(a.Digest, b.Digest))
	})
	inputs = slices.Compact(inputs)
	cacheKey, err := format.Canonicalize(struct {
		Kind   hostproto.Kind    `json:"kind"`
		Inputs []hostproto.Input `json:"inputs"`
	}{draft.Kind, inputs})
	if err != nil {
		return hostproto.Request{}, err
	}
	request := hostproto.Request{
		Type:           hostproto.RequestType,
		Module:         identity.Module,
		Kind:           draft.Kind,
		Role:           hostproto.RoleFor(identity.Plugin, draft.Kind.Agent()),
		Shape:          hostproto.ShapeOpen,
		Prompt:         draft.Built.Prompt,
		ResponseSchema: draft.Built.Schema,
		Closed:         draft.Built.Closed,
		Limits:         hostproto.Limits{MaxOutputBytes: hostproto.MaxOutputBytes},
		Inputs:         inputs,
		CacheKey:       format.SHA256Hex(cacheKey),
	}
	if draft.Kind == hostproto.KindDecideClosed {
		request.Shape = hostproto.ShapeClosed
	}
	if draft.Kind.IsGeneration() {
		round := draft.Round
		request.Round, request.Seed = &round, draft.Seed
	}
	return request, nil
}

// DigestText is the digest a request records of a text it depends on.
func DigestText(text string) string { return format.SHA256Hex([]byte(text)) }
