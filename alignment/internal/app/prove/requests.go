package prove

import (
	"cmp"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/alignment/internal/domain/prompt"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
)

// assemble builds the request of a built prompt: the module, the role of the
// agent that serves the kind, the shape, the output limit, the sorted inputs and
// the cache key that names the work.
func (s *subjectRun) assemble(kind hostproto.Kind, built prompt.Built, statementDigests []string) (hostproto.Request, error) {
	inputs := []hostproto.Input{
		{Role: format.RolePromptTemplate, Digest: built.Prompt.TemplateDigest},
		{Role: format.RoleDecisionPolicy, Digest: s.pass.policyDigest},
	}
	for _, digest := range statementDigests {
		inputs = append(inputs, hostproto.Input{Role: format.RoleSpec, Digest: digest})
	}
	for _, file := range s.masked.TestFiles {
		if digest, readable := s.pass.digestOfFile(file); readable {
			inputs = append(inputs, hostproto.Input{Role: format.RoleTestSource, Digest: digest})
		}
	}
	for _, file := range s.masked.CodeFiles {
		if digest, readable := s.pass.digestOfFile(file); readable {
			inputs = append(inputs, hostproto.Input{Role: format.RoleCodeTree, Digest: digest})
		}
	}
	slices.SortFunc(inputs, func(a, b hostproto.Input) int {
		return cmp.Or(strings.Compare(string(a.Role), string(b.Role)), strings.Compare(a.Digest, b.Digest))
	})
	inputs = slices.Compact(inputs)
	cacheKey, err := format.Canonicalize(struct {
		Kind   hostproto.Kind    `json:"kind"`
		Inputs []hostproto.Input `json:"inputs"`
	}{kind, inputs})
	if err != nil {
		return hostproto.Request{}, err
	}
	request := hostproto.Request{
		Type:           hostproto.RequestType,
		Module:         s.pass.settings.Module,
		Kind:           kind,
		Role:           hostproto.RoleFor(s.pass.settings.Plugin, kind.Agent()),
		Shape:          hostproto.ShapeOpen,
		Prompt:         built.Prompt,
		ResponseSchema: built.Schema,
		Closed:         built.Closed,
		Limits:         hostproto.Limits{MaxOutputBytes: hostproto.MaxOutputBytes},
		Inputs:         inputs,
		CacheKey:       format.SHA256Hex(cacheKey),
	}
	if kind == hostproto.KindDecideClosed {
		request.Shape = hostproto.ShapeClosed
	}
	return request, nil
}

// allStatementDigests lists the digests of every criterion statement the judge sees.
func (s *subjectRun) allStatementDigests() []string {
	digests := make([]string, len(s.pass.project.Criteria))
	for index, c := range s.pass.project.Criteria {
		digests[index] = c.StatementDigest
	}
	return digests
}

// groupStatementDigests lists the digests of the statements of the criteria of a group.
func (s *subjectRun) groupStatementDigests(group []string) []string {
	var digests []string
	for _, token := range group {
		if c, found := s.pass.project.Find(s.input.Criteria.Refs[token]); found {
			digests = append(digests, c.StatementDigest)
		}
	}
	return digests
}
