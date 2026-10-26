package prove

import (
	"github.com/RachidChabane/musthold/discpower/internal/app/hostrequest"
	"github.com/RachidChabane/musthold/discpower/internal/domain/prompt"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
	"github.com/RachidChabane/musthold/format/hostproto"
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

// assemble builds the request of a draft under the run's identity: its
// module, its plugin and the digest of its policy.
func (p *project) assemble(draft requestDraft) (hostproto.Request, error) {
	policyDigest, err := p.settings.Policy.Digest()
	if err != nil {
		return hostproto.Request{}, err
	}
	identity := hostrequest.Identity{Module: p.settings.Module, Plugin: p.settings.Plugin, PolicyDigest: policyDigest}
	return hostrequest.Assemble(identity, hostrequest.Draft{Kind: draft.kind, Built: draft.built, Inputs: draft.inputs, Round: draft.round, Seed: draft.seed})
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
func digestText(text string) string { return hostrequest.DigestText(text) }
