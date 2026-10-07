// Package hostprototest is the test support of the host protocol: fixtures, the
// reference in-memory Store, the Store contract suite, golden JSON-RPC
// transcripts, the fake-host personas with a reusable driver, and a monotonicity
// property helper. It is never linked into a shipped binary.
package hostprototest

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// Names and texts of the fixture request.
const (
	PluginName     = "spec-proof-dp"
	ModuleName     = "spec-proof-dp"
	ModuleVersion  = "0.1.0"
	systemText     = "You judge whether a mutant violates a criterion."
	userText       = "Criterion: a stay of 15 minutes or less is free. Mutant diff follows."
	HarnessName    = "claude-code"
	HarnessVersion = "2.1.292"
	// ResponseSchema is the schema of the fixture request: one validity verdict.
	ResponseSchema = `{"type":"object","additionalProperties":false,"required":["validity"],"properties":{"validity":{"enum":["VALID","EQUIVALENT","NOT_VIOLATING","UNDETERMINED"]}}}`
	// HonestAnswer satisfies ResponseSchema.
	HonestAnswer = `{"validity":"VALID"}`
	// SecretText is the per-install secret of the fixtures.
	SecretText = "fixture-secret-0123456789abcdef"
)

// Epoch is the instant the fixtures start at.
var Epoch = time.Date(2026, time.October, 7, 12, 0, 0, 0, time.UTC)

var epoch = Epoch

// Secret returns the fixture secret.
func Secret() []byte { return []byte(SecretText) }

// Digest returns a stand-in SHA-256 digest of a label.
func Digest(label string) string { return format.SHA256Hex([]byte(label)) }

// Request builds a valid `judge.validity` request and applies the mutations.
func Request(mutations ...func(*hostproto.Request)) hostproto.Request {
	request := hostproto.Request{
		Type:           hostproto.RequestType,
		Module:         format.Module{Name: ModuleName, Version: ModuleVersion},
		Kind:           hostproto.KindJudgeValidity,
		Role:           hostproto.RoleFor(PluginName, hostproto.AgentJudge),
		Shape:          hostproto.ShapeOpen,
		Prompt:         hostproto.Prompt{System: systemText, User: userText, TemplateDigest: Digest("template/judge-validity")},
		ResponseSchema: json.RawMessage(ResponseSchema),
		Limits:         hostproto.Limits{MaxOutputBytes: hostproto.MaxOutputBytes},
		Inputs: []hostproto.Input{
			{Role: format.RoleCodeTree, Digest: Digest("code-tree")},
			{Role: format.RoleSpec, Digest: Digest("spec")},
		},
		CacheKey: Digest("cache-key"),
	}
	for _, mutate := range mutations {
		mutate(&request)
	}
	return request
}

// GenerationRequest builds a valid `generate.targeted` request.
func GenerationRequest(mutations ...func(*hostproto.Request)) hostproto.Request {
	round := 1
	return Request(append([]func(*hostproto.Request){func(r *hostproto.Request) {
		r.Kind = hostproto.KindGenerateTargeted
		r.Role = hostproto.RoleFor(PluginName, hostproto.AgentGenerator)
		r.Round = &round
		r.Seed = Digest("seed")
	}}, mutations...)...)
}

// ClosedRequest builds a valid `decide.closed` request.
func ClosedRequest(mutations ...func(*hostproto.Request)) hostproto.Request {
	return Request(append([]func(*hostproto.Request){func(r *hostproto.Request) {
		r.Kind = hostproto.KindDecideClosed
		r.Role = hostproto.RoleFor(PluginName, hostproto.AgentDecider)
		r.Shape = hostproto.ShapeClosed
		set := ClosedSet()
		r.Closed = mustClosed(set)
		schema, err := set.ResponseSchema()
		if err != nil {
			panic(err)
		}
		r.ResponseSchema = schema
	}}, mutations...)...)
}

// MustID returns the id of a request or panics.
func MustID(request hostproto.Request) hostproto.RequestID {
	id, err := request.ID()
	if err != nil {
		panic(err)
	}
	return id
}

// Ticket returns a ticket of the request with a deterministic nonce.
func Ticket(request hostproto.Request, vote, emission int) hostproto.Ticket {
	random := strings.NewReader(strings.Repeat("\x07", hostproto.NonceBytes))
	ticket, err := hostproto.NewTicket(hostproto.TicketID{RequestID: MustID(request), Vote: vote, Emission: emission}, random)
	if err != nil {
		panic(err)
	}
	return ticket
}

// Profile returns a valid, unattested executor profile.
func Profile() format.ExecutorProfile {
	return format.ExecutorProfile{
		Channel:   format.ChannelHostSubagent,
		Harness:   format.Harness{Name: HarnessName, Version: HarnessVersion},
		Isolation: format.IsolationRestricted,
		Model:     format.ExecutorModel{Name: "claude-sonnet-5-5", Source: format.ModelSourceResolved},
	}
}

// Response returns a valid response to the ticket.
func Response(ticket hostproto.Ticket) hostproto.Response {
	return hostproto.Response{
		RequestID: ticket.RequestID, Vote: ticket.Vote, Nonce: ticket.Nonce,
		Output: json.RawMessage(HonestAnswer), Executor: Profile(),
	}
}

// Names of the closed fixtures.
const (
	// ClosedQuestionID is the id of the question of ClosedSet.
	ClosedQuestionID = "q1"
	// ClosedHonestAnswer satisfies the response schema of ClosedRequest.
	ClosedHonestAnswer = `{"answers":[{"id":"q1","choice":"VIOLATES","confidence":0.9}]}`
	// ClosedSystemOneResponse is what a System One endpoint answers to ClosedSet.
	ClosedSystemOneResponse = `{"model":"tev1:0.8b","answers":{"q1":{"type":"choice","choice":"VIOLATES","probabilities":{"VIOLATES":0.9,"CONSISTENT":0.1},"confidence":0.9}},"usage":{"input_tokens":10,"output_tokens":1}}`
)

// ClosedSet builds a valid question set: one choice question about a state.
func ClosedSet() hostproto.ClosedSet {
	return hostproto.ClosedSet{
		State: "Requirement 1:\nthe fee is waived from 50\n",
		Questions: []hostproto.ClosedQuestion{hostproto.ChoiceQuestion(ClosedQuestionID, "Does the change break requirement 1?", []hostproto.ClosedOption{
			{Name: "VIOLATES", Description: "The change breaks the requirement."},
			{Name: "CONSISTENT", Description: "The requirement is still met."},
		})},
	}
}

func mustClosed(set hostproto.ClosedSet) json.RawMessage {
	encoded, err := set.Marshal()
	if err != nil {
		panic(err)
	}
	return encoded
}
