package prove

import (
	"cmp"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/prompt"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// requestDraft is what a request is assembled from: its kind, the built prompt
// and the inputs it depends on.
type requestDraft struct {
	kind   hostproto.Kind
	built  prompt.Built
	inputs []hostproto.Input
	round  int
	seed   string
}

// assemble builds the request of a draft: the module, the role of the agent that
// serves the kind, the shape, the output limit, the sorted inputs and the cache
// key that names the work.
func (p *project) assemble(draft requestDraft) (hostproto.Request, error) {
	policyDigest, err := p.settings.Policy.Digest()
	if err != nil {
		return hostproto.Request{}, err
	}
	inputs := append([]hostproto.Input{
		{Role: format.RolePromptTemplate, Digest: draft.built.Prompt.TemplateDigest},
		{Role: format.RoleDecisionPolicy, Digest: policyDigest},
	}, draft.inputs...)
	slices.SortFunc(inputs, func(a, b hostproto.Input) int {
		return cmp.Or(strings.Compare(string(a.Role), string(b.Role)), strings.Compare(a.Digest, b.Digest))
	})
	inputs = slices.Compact(inputs)
	cacheKey, err := format.Canonicalize(struct {
		Kind   hostproto.Kind    `json:"kind"`
		Inputs []hostproto.Input `json:"inputs"`
	}{draft.kind, inputs})
	if err != nil {
		return hostproto.Request{}, err
	}
	request := hostproto.Request{
		Type:           hostproto.RequestType,
		Module:         p.settings.Module,
		Kind:           draft.kind,
		Role:           hostproto.RoleFor(p.settings.Plugin, draft.kind.Agent()),
		Shape:          hostproto.ShapeOpen,
		Prompt:         draft.built.Prompt,
		ResponseSchema: draft.built.Schema,
		Closed:         draft.built.Closed,
		Limits:         hostproto.Limits{MaxOutputBytes: hostproto.MaxOutputBytes},
		Inputs:         inputs,
		CacheKey:       format.SHA256Hex(cacheKey),
	}
	if draft.kind == hostproto.KindDecideClosed {
		request.Shape = hostproto.ShapeClosed
	}
	if draft.kind.IsGeneration() {
		round := draft.round
		request.Round, request.Seed = &round, draft.seed
	}
	return request, nil
}

// refsOf lists the references of every criterion of the run.
func (p *project) refsOf() []spec.Ref {
	refs := make([]spec.Ref, len(p.criteria))
	for index, criterion := range p.criteria {
		refs[index] = criterion.Ref
	}
	return refs
}

// blindStatement is the statement of a criterion as an executor may see it: with
// no reference to any criterion by its identifier.
func (p *project) blindStatement(criterion spec.Criterion) string {
	return prompt.Scrub(criterion.Statement, p.refsOf())
}

// digestText digests a text.
func digestText(text string) string { return format.SHA256Hex([]byte(text)) }
