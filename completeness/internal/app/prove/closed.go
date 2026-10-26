package prove

import (
	"cmp"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/completeness/internal/domain/closedq"
	"github.com/RachidChabane/musthold/completeness/internal/domain/prompt"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostbridge"
)

// askState is where an ask of the host stands.
type askState int

const (
	askAnswered askState = iota
	askPending
	askUnresponsive
)

// closedSignal is the answer to one closed request.
type closedSignal struct {
	state   askState
	reading closedq.Reading
	// answer is the host's answer the reading was taken from.
	answer hostbridge.Answer
}

// closedRequest is a closed request together with what the Statement it serves
// must record about it.
type closedRequest struct {
	built prompt.Built
	// statementDigests are the digests of the criterion statements the request shows.
	statementDigests []string
	// files are the digests of the files the request shows code of.
	fileDigests []string
	// criteria are the criteria of the spec the request serves, for the queue.
	criteria []string
}

// assemble builds the `decide.closed` request of a built prompt: the module, the
// role of the agent that serves the kind, the closed shape, the output limit, the
// sorted inputs and the cache key that names the work.
func (p *passState) assemble(request closedRequest) (hostproto.Request, error) {
	inputs := []hostproto.Input{
		{Role: format.RolePromptTemplate, Digest: request.built.Prompt.TemplateDigest},
		{Role: format.RoleDecisionPolicy, Digest: p.policyDigest},
	}
	for _, digest := range request.statementDigests {
		inputs = append(inputs, hostproto.Input{Role: format.RoleSpec, Digest: digest})
	}
	for _, digest := range request.fileDigests {
		inputs = append(inputs, hostproto.Input{Role: format.RoleCodeTree, Digest: digest})
	}
	slices.SortFunc(inputs, func(a, b hostproto.Input) int {
		return cmp.Or(strings.Compare(string(a.Role), string(b.Role)), strings.Compare(a.Digest, b.Digest))
	})
	inputs = slices.Compact(inputs)
	cacheKey, err := format.Canonicalize(struct {
		Kind   hostproto.Kind    `json:"kind"`
		Inputs []hostproto.Input `json:"inputs"`
		Prompt string            `json:"prompt"`
	}{hostproto.KindDecideClosed, inputs, format.SHA256Hex([]byte(request.built.Prompt.User))})
	if err != nil {
		return hostproto.Request{}, err
	}
	return hostproto.Request{
		Type:           hostproto.RequestType,
		Module:         p.settings.Module,
		Kind:           hostproto.KindDecideClosed,
		Role:           hostproto.RoleFor(p.settings.Plugin, hostproto.KindDecideClosed.Agent()),
		Shape:          hostproto.ShapeClosed,
		Prompt:         request.built.Prompt,
		ResponseSchema: request.built.Schema,
		Closed:         request.built.Closed,
		Limits:         hostproto.Limits{MaxOutputBytes: hostproto.MaxOutputBytes},
		Inputs:         inputs,
		CacheKey:       format.SHA256Hex(cacheKey),
	}, nil
}

// standsForTheVotes reports whether the first vote replaces the host votes: the
// answer of a System One relay or the classification of the host's classifier.
func standsForTheVotes(vote hostbridge.VoteState) bool {
	return vote.Executor.Channel == format.ChannelSystemOneRelay || vote.Executor.Channel == format.ChannelHostClassify
}

// ask puts a closed request to the host and reads its answer. The first ticket is
// the one a System One relay or a classification answers; only when a host decider
// answered it do the other `decide_k` tickets follow, because a relay answer or a
// classification replaces the host votes.
func (p *passState) ask(request closedRequest) (*closedSignal, error) {
	assembled, err := p.assemble(request)
	if err != nil {
		return nil, err
	}
	id, err := assembled.ID()
	if err != nil {
		return nil, err
	}
	if signal, known := p.closed[string(id)]; known {
		return signal, nil
	}
	signal := &closedSignal{}
	p.closed[string(id)] = signal
	want := hostbridge.Ask{Request: assembled, Votes: 1, Criteria: request.criteria, Purpose: string(hostproto.KindDecideClosed)}
	answer, state, err := p.want(want)
	if err != nil || state != askAnswered {
		signal.state = state
		return signal, err
	}
	if !standsForTheVotes(answer.Votes[0]) && p.settings.Policy.DecideK > 1 {
		want.Votes = p.settings.Policy.DecideK
		if answer, state, err = p.want(want); err != nil || state != askAnswered {
			signal.state = state
			return signal, err
		}
	}
	signal.answer, signal.reading = answer, readVotes(answer)
	return signal, p.bridge.Validate(answer)
}

func (p *passState) want(ask hostbridge.Ask) (hostbridge.Answer, askState, error) {
	answer, err := p.bridge.Want(ask)
	if err != nil {
		return answer, askAnswered, err
	}
	switch answer.Standing {
	case hostbridge.Pending:
		return answer, askPending, nil
	case hostbridge.Unresponsive:
		return answer, askUnresponsive, nil
	}
	return answer, askAnswered, nil
}

// readVotes turns the votes of an answer into one reading. Completeness is
// advisory and claims nothing a response's isolation could take away, so every
// vote counts.
func readVotes(answer hostbridge.Answer) closedq.Reading {
	votes := make([]closedq.Vote, len(answer.Votes))
	for index, vote := range answer.Votes {
		votes[index] = closedq.Vote{
			Output: vote.Output, Relayed: vote.Executor.Channel == format.ChannelSystemOneRelay, Source: vote.Executor.Model.Name,
			Classify: vote.Classify,
		}
	}
	return closedq.Read(votes)
}
