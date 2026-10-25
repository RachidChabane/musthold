package hostproto

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/format"
)

// Identity and limits of the request format.
const (
	// RequestType is the URI of the request format; its major version is part of it.
	RequestType = "https://rachid-chabane.com/host-protocol/request/v1"
	// MaxOutputBytes is the largest output an executor may return.
	MaxOutputBytes = 262144
	// RoleSeparator separates the plugin from the agent in an executor role.
	RoleSeparator = ":"
	// responseSchemaRootType is the JSON type every response schema has at its root.
	responseSchemaRootType = "object"
	moduleVersionMember    = "version"
	moduleMember           = "module"
)

// Kind names the work a request asks of an executor.
type Kind string

// Request kinds.
const (
	KindGenerateTargeted        Kind = "generate.targeted"
	KindGenerateWitnessStrategy Kind = "generate.witnessStrategy"
	// KindGenerateWitness asks a blind writer for the witnesses of a criterion:
	// tests derived from the spec alone, before and without any mutant.
	KindGenerateWitness Kind = "generate.witness"
	// KindGeneratePromises asks the generator for the promises a change makes,
	// each a verbatim quote of one source among the README, the description and
	// the closing issues.
	KindGeneratePromises Kind = "generate.promises"
	KindJudgeValidity    Kind = "judge.validity"
	KindJudgeEquivalent  Kind = "judge.equivalent"
	KindJudgeAlignment   Kind = "judge.alignment"
	// KindJudgeScope asks a blind scope judge, per witness, whether the
	// criterion's text alone fixes the output the witness checks.
	KindJudgeScope Kind = "judge.scope"
	// KindJudgeLabels asks the judge which functions of a generation view each
	// kept promise concerns.
	KindJudgeLabels  Kind = "judge.labels"
	KindDecideClosed Kind = "decide.closed"
)

// Agent is the executor agent that serves a kind of request.
type Agent string

// Executor agents.
const (
	AgentGenerator Agent = "generator"
	AgentJudge     Agent = "judge"
	AgentDecider   Agent = "decider"
)

// Shape tells whether a request asks an open or a closed question.
type Shape string

// Request shapes.
const (
	ShapeOpen   Shape = "open"
	ShapeClosed Shape = "closed"
)

var agentOfKind = map[Kind]Agent{
	KindGenerateTargeted:        AgentGenerator,
	KindGenerateWitnessStrategy: AgentGenerator,
	KindGenerateWitness:         AgentGenerator,
	KindGeneratePromises:        AgentGenerator,
	KindJudgeValidity:           AgentJudge,
	KindJudgeEquivalent:         AgentJudge,
	KindJudgeAlignment:          AgentJudge,
	KindJudgeScope:              AgentJudge,
	KindJudgeLabels:             AgentJudge,
	KindDecideClosed:            AgentDecider,
}

var kinds = []Kind{
	KindGenerateTargeted, KindGenerateWitnessStrategy, KindGenerateWitness, KindGeneratePromises, KindJudgeValidity,
	KindJudgeEquivalent, KindJudgeAlignment, KindJudgeScope, KindJudgeLabels, KindDecideClosed,
}

// Kinds lists the closed set of request kinds.
func Kinds() []Kind { return slices.Clone(kinds) }

// Agents lists the executor agents.
func Agents() []Agent { return []Agent{AgentGenerator, AgentJudge, AgentDecider} }

// Agent returns the executor agent that serves the kind, or "" for an unknown kind.
func (k Kind) Agent() Agent { return agentOfKind[k] }

// IsGeneration reports whether the kind asks for generated artefacts, which are
// the only requests that carry a round and a seed.
func (k Kind) IsGeneration() bool { return k.Agent() == AgentGenerator }

// Prompt is the blind prompt an executor answers.
type Prompt struct {
	System         string `json:"system"`
	User           string `json:"user"`
	TemplateDigest string `json:"templateDigest"`
}

// Limits bound what an executor may return.
type Limits struct {
	MaxOutputBytes int `json:"maxOutputBytes"`
}

// Input is one input the request depends on.
type Input struct {
	Role   format.InputRole `json:"role"`
	Digest string           `json:"digest"`
}

// Request is the structured request a module emits when a proof needs model work.
// It carries no queue metadata: the criterion and the wave live in queue state and
// are never shown to an executor.
type Request struct {
	Type           string          `json:"type"`
	Module         format.Module   `json:"module"`
	Kind           Kind            `json:"kind"`
	Role           string          `json:"role"`
	Shape          Shape           `json:"shape"`
	Prompt         Prompt          `json:"prompt"`
	ResponseSchema json.RawMessage `json:"responseSchema"`
	Closed         json.RawMessage `json:"closed,omitempty"`
	Round          *int            `json:"round,omitempty"`
	Seed           string          `json:"seed,omitempty"`
	Limits         Limits          `json:"limits"`
	Inputs         []Input         `json:"inputs"`
	CacheKey       string          `json:"cacheKey"`
}

// Problem is one broken rule of a request, a response or a ticket.
type Problem struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// InvalidError lists every problem found in one document.
type InvalidError struct {
	Problems []Problem
}

// Error lists the problems, one per line.
func (e *InvalidError) Error() string {
	lines := make([]string, len(e.Problems))
	for index, problem := range e.Problems {
		lines[index] = problem.Path + ": " + problem.Message
	}
	return strings.Join(lines, "\n")
}

type problemSink struct{ problems []Problem }

func (s *problemSink) add(path, message string, args ...any) {
	s.problems = append(s.problems, Problem{Path: path, Message: fmt.Sprintf(message, args...)})
}

func (s *problemSink) err() error {
	if len(s.problems) == 0 {
		return nil
	}
	return &InvalidError{Problems: s.problems}
}

// RoleFor builds the executor role of a plugin and an agent, such as
// `spec-proof-dp:judge`.
func RoleFor(plugin string, agent Agent) string { return plugin + RoleSeparator + string(agent) }

// ParseRole splits an executor role into its plugin and agent.
func ParseRole(role string) (plugin string, agent Agent, err error) {
	plugin, name, found := strings.Cut(role, RoleSeparator)
	if !found || plugin == "" || strings.Contains(name, RoleSeparator) || !slices.Contains(Agents(), Agent(name)) {
		return "", "", fmt.Errorf("%q is not a role of the form plugin%sagent", role, RoleSeparator)
	}
	return plugin, Agent(name), nil
}

// Validate checks every rule of the request and returns an *InvalidError.
func (r Request) Validate() error {
	var sink problemSink
	if r.Type != RequestType {
		sink.add("type", "%q is not %q", r.Type, RequestType)
	}
	r.checkIdentity(&sink)
	r.checkShape(&sink)
	r.checkGeneration(&sink)
	if r.Prompt.User == "" {
		sink.add("prompt.user", "must not be empty")
	}
	checkDigestField(&sink, "prompt.templateDigest", r.Prompt.TemplateDigest)
	checkResponseSchema(&sink, r.ResponseSchema)
	if r.Limits.MaxOutputBytes < 1 || r.Limits.MaxOutputBytes > MaxOutputBytes {
		sink.add("limits.maxOutputBytes", "%d is outside [1,%d]", r.Limits.MaxOutputBytes, MaxOutputBytes)
	}
	r.checkInputs(&sink)
	checkDigestField(&sink, "cacheKey", r.CacheKey)
	return sink.err()
}

func (r Request) checkIdentity(sink *problemSink) {
	if r.Module.Name == "" {
		sink.add("module.name", "must not be empty")
	}
	if r.Module.Version == "" {
		sink.add("module.version", "must not be empty")
	}
	if r.Kind.Agent() == "" {
		sink.add("kind", "%q is not a request kind", r.Kind)
	}
	_, agent, err := ParseRole(r.Role)
	switch {
	case err != nil:
		sink.add("role", "%v", err)
	case r.Kind.Agent() != "" && agent != r.Kind.Agent():
		sink.add("role", "agent %q does not serve kind %q", agent, r.Kind)
	}
}

func (r Request) checkShape(sink *problemSink) {
	closed := r.Kind == KindDecideClosed
	switch {
	case r.Shape != ShapeOpen && r.Shape != ShapeClosed:
		sink.add("shape", "%q is neither %q nor %q", r.Shape, ShapeOpen, ShapeClosed)
	case closed != (r.Shape == ShapeClosed):
		sink.add("shape", "kind %q has shape %q", r.Kind, r.Shape)
	}
	if hasClosed := len(r.Closed) > 0; hasClosed != (r.Shape == ShapeClosed) {
		sink.add("closed", "the question set is present exactly when the shape is %q", ShapeClosed)
	}
	if len(r.Closed) > 0 && r.Shape == ShapeClosed {
		sink.problems = append(sink.problems, closedProblems(r.Closed)...)
	}
}

func (r Request) checkGeneration(sink *problemSink) {
	if !r.Kind.IsGeneration() {
		if r.Round != nil || r.Seed != "" {
			sink.add("round", "only a generation request carries a round and a seed")
		}
		return
	}
	if r.Round == nil || *r.Round < 1 {
		sink.add("round", "a generation request carries a round of at least 1")
	}
	checkDigestField(sink, "seed", r.Seed)
}

func (r Request) checkInputs(sink *problemSink) {
	for index, input := range r.Inputs {
		path := fmt.Sprintf("inputs[%d]", index)
		if !slices.Contains(format.InputRoles(), input.Role) {
			sink.add(path+".role", "%q is not an input role", input.Role)
		}
		checkDigestField(sink, path+".digest", input.Digest)
		if index > 0 && compareInputs(r.Inputs[index-1], input) >= 0 {
			sink.add("inputs", "element %d is not strictly after element %d", index, index-1)
		}
	}
}

func compareInputs(a, b Input) int {
	if byRole := strings.Compare(string(a.Role), string(b.Role)); byRole != 0 {
		return byRole
	}
	return strings.Compare(a.Digest, b.Digest)
}

func checkDigestField(sink *problemSink, path, value string) {
	if !format.IsSHA256Hex(value) {
		sink.add(path, "%q is not a SHA-256 hex digest", value)
	}
}

func checkResponseSchema(sink *problemSink, schema json.RawMessage) {
	var root struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(schema, &root); err != nil {
		sink.add("responseSchema", "is not a JSON object: %v", err)
		return
	}
	if root.Type != responseSchemaRootType {
		sink.add("responseSchema.type", "%q is not %q", root.Type, responseSchemaRootType)
	}
}

// RequestID is the content address of a request: the SHA-256 of its JCS bytes
// with the module version removed.
type RequestID string

// ErrNotARequestDocument is returned when a document cannot be read as a JSON
// object with a module member.
var ErrNotARequestDocument = errors.New("not a request document")

// ID computes the request's content address.
func (r Request) ID() (RequestID, error) {
	document, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}
	return RequestIDOfJSON(document)
}

// RequestIDOfJSON computes the content address of a request document: it removes
// `module.version`, canonicalizes the rest with RFC 8785 and hashes the bytes.
// Because it works on the document, member order never reaches the id.
func RequestIDOfJSON(document []byte) (RequestID, error) {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(document, &members); err != nil {
		return "", fmt.Errorf("%w: %v", ErrNotARequestDocument, err)
	}
	var module map[string]json.RawMessage
	if err := json.Unmarshal(members[moduleMember], &module); err != nil {
		return "", fmt.Errorf("%w: module: %v", ErrNotARequestDocument, err)
	}
	delete(module, moduleVersionMember)
	stripped, err := json.Marshal(module)
	if err != nil {
		return "", fmt.Errorf("marshal module: %w", err)
	}
	members[moduleMember] = stripped
	rewritten, err := json.Marshal(members)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}
	canonical, err := format.CanonicalizeJSON(rewritten)
	if err != nil {
		return "", err
	}
	return RequestID(format.SHA256Hex(canonical)), nil
}

// SchemaDigest is the digest of the canonical response schema, which an entry
// records at emission so that a later change of the stored request is detected.
func (r Request) SchemaDigest() (string, error) {
	canonical, err := format.CanonicalizeJSON(r.ResponseSchema)
	if err != nil {
		return "", err
	}
	return format.SHA256Hex(canonical), nil
}

// ExecutorView is everything an executor sees of a request: the prompt, the
// response schema, the ticket and its nonce. It never carries queue metadata, the
// inputs or the cache key.
type ExecutorView struct {
	Ticket         string          `json:"ticket"`
	Nonce          string          `json:"nonce"`
	System         string          `json:"system"`
	User           string          `json:"user"`
	ResponseSchema json.RawMessage `json:"responseSchema"`
	Closed         json.RawMessage `json:"closed,omitempty"`
}

// ViewFor builds what the executor of a ticket sees.
func (r Request) ViewFor(ticket Ticket) ExecutorView {
	return ExecutorView{
		Ticket:         ticket.TicketID.String(),
		Nonce:          ticket.Nonce,
		System:         r.Prompt.System,
		User:           r.Prompt.User,
		ResponseSchema: r.ResponseSchema,
		Closed:         r.Closed,
	}
}

// Meta is the queue-only metadata of a request.
type Meta struct {
	Criterion string `json:"criterion"`
	Wave      int    `json:"wave"`
}
