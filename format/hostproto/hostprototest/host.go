package hostprototest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/executorrun"
)

// Errors a Host returns for the situations the personas provoke.
var (
	// ErrSchemaViolation is returned for an output that does not satisfy the response schema.
	ErrSchemaViolation = errors.New("output violates the response schema")
	// ErrUnsupported is returned by a Host that cannot perform an operation, such
	// as a command-line host asked to run a hook.
	ErrUnsupported = errors.New("operation not supported by this host")
)

// Submit is one call of the `submit_response` tool.
type Submit struct {
	Ticket hostproto.TicketID
	Nonce  string
	Cap    string
	Output json.RawMessage
}

// Receipt is what `submit_response` returns.
type Receipt struct {
	// Text is the bare receipt an honest executor replies with.
	Text string
	// CorrectionAllowed tells that a schema failure may be corrected once.
	CorrectionAllowed bool
}

// Host is the system under test as the personas see it: the module's executor
// server together with the hooks of the harness around it. A Host that cannot do
// something returns ErrUnsupported.
type Host interface {
	// Emit issues a ticket for the request.
	Emit(request hostproto.Request) (hostproto.Ticket, error)
	// Launch runs the PreToolUse hook on an Agent call and returns the input the
	// harness would use.
	Launch(input hostproto.AgentInput) (hostproto.AgentInput, error)
	// StartExecutor runs the SubagentStart hook for a role and returns the agent
	// identifier, empty when no hook ran.
	StartExecutor(role string) string
	// Cap returns what the PreToolUse hook injects into an MCP call of the agent.
	Cap(agentID string) string
	// Fetch is the `fetch_request` tool.
	Fetch(ticket hostproto.TicketID, capability string) (hostproto.ExecutorView, error)
	// Submit is the `submit_response` tool.
	Submit(submission Submit) (Receipt, error)
	// AfterRun runs the SubagentStop and PostToolUse hooks: the transcript check,
	// the reply check and the resolved model.
	AfterRun(run Run) error
	// Advance moves the clock.
	Advance(duration time.Duration)
	// Sweep expires the tickets past their time and reissues the rejected and
	// expired ones, as a module does at `--resume`. It returns the current ticket
	// of the request.
	Sweep(request hostproto.RequestID) (hostproto.Ticket, error)
	// Entry returns the stored entry of the ticket's key.
	Entry(key hostproto.Key) (hostproto.Entry, error)
	// Profile returns the executor profile the host records for the key's answer.
	Profile(key hostproto.Key) (format.ExecutorProfile, error)
	// Journal returns the journaled events in order.
	Journal() []hostproto.Event
}

// Run describes how an executor run ended, as the stop hooks see it.
type Run struct {
	Ticket        hostproto.TicketID
	AgentID       string
	Transcript    []hostproto.ToolCall
	Reply         string
	ResolvedModel string
	// InheritedHistory is set for a run that started with history of the agent
	// that spawned it, which the rollout of a Codex thread shows.
	InheritedHistory bool
	// SessionLog is the session log of a harness that records its sessions, which
	// the stop hook reads in place of Transcript.
	SessionLog []byte
}

// ReferenceHost implements Host in memory over the pure protocol, the way a
// module wires it: the lifecycle, the binding check, the journal and
// SelectChannel. The personas' asserted verdicts hold against it, and a module's
// own Host is held to the same verdicts.
type ReferenceHost struct {
	Observed hostproto.Observed
	Policy   hostproto.Policy

	store   *MemoryStore
	now     time.Time
	counter int
	random  io.Reader
}

// NewReferenceHost builds a host that sees what observed says and obeys policy.
func NewReferenceHost(observed hostproto.Observed, policy hostproto.Policy) *ReferenceHost {
	return &ReferenceHost{Observed: observed, Policy: policy, store: NewMemoryStore(), now: Epoch, random: &counterReader{}}
}

// ClaudeCodeObserved is what a Claude Code session with the plugin, its server and
// its hooks shows.
func ClaudeCodeObserved() hostproto.Observed {
	return hostproto.Observed{PluginPresent: true, ServerPresent: true, HooksPresent: true, Allowlist: hostproto.AllowlistEnforced}
}

// RequireAttestation is the default policy.
func RequireAttestation() hostproto.Policy { return hostproto.Policy{RequireAttestation: true} }

// counterReader yields a different nonce for every read, deterministically.
type counterReader struct{ next byte }

func (c *counterReader) Read(p []byte) (int, error) {
	for index := range p {
		c.next++
		p[index] = c.next
	}
	return len(p), nil
}

func (h *ReferenceHost) journal(event hostproto.Event) {
	if _, err := h.store.Append(event); err != nil {
		panic(err)
	}
}

func (h *ReferenceHost) verifier() hostproto.CapVerifier {
	records, _ := h.store.Records()
	entries, _ := h.store.ListEntries()
	return hostproto.CapVerifier{
		Secret: Secret(), Roster: hostproto.JournalRoster(records),
		Bindings: hostproto.EntryCapBindings(entries), RequireAttestation: h.Policy.RequireAttestation,
	}
}

// Emit implements Host.
func (h *ReferenceHost) Emit(request hostproto.Request) (hostproto.Ticket, error) {
	if err := request.Validate(); err != nil {
		return hostproto.Ticket{}, err
	}
	id, err := h.store.PutRequest(request)
	if err != nil {
		return hostproto.Ticket{}, err
	}
	ticket, err := hostproto.NewTicket(hostproto.TicketID{RequestID: id, Emission: hostproto.FirstEmission}, h.random)
	if err != nil {
		return hostproto.Ticket{}, err
	}
	schemaDigest, err := request.SchemaDigest()
	if err != nil {
		return hostproto.Ticket{}, err
	}
	if err := h.store.PutEntry(hostproto.NewEntry(ticket, request.Role, schemaDigest, hostproto.DefaultMaxEmissions)); err != nil {
		return hostproto.Ticket{}, err
	}
	h.journal(hostproto.Event{Kind: hostproto.EventEmitted, Ticket: ticket.TicketID.String()})
	return ticket, nil
}

// Launch implements Host. Without the hooks the call goes through unchanged.
func (h *ReferenceHost) Launch(input hostproto.AgentInput) (hostproto.AgentInput, error) {
	if !h.Observed.HooksPresent {
		return input, nil
	}
	return input.Sanitize(PluginName)
}

// StartExecutor implements Host.
func (h *ReferenceHost) StartExecutor(role string) string {
	if !h.Observed.HooksPresent {
		return ""
	}
	h.counter++
	agentID := fmt.Sprintf("agent-%04d", h.counter)
	h.journal(hostproto.Event{Kind: hostproto.EventAgentStarted, AgentID: agentID, Role: role})
	return agentID
}

// Cap implements Host.
func (h *ReferenceHost) Cap(agentID string) string {
	if !h.Observed.HooksPresent || agentID == "" {
		return ""
	}
	capability, err := hostproto.FormatCap(Secret(), agentID)
	if err != nil {
		return ""
	}
	return capability
}

// Fetch implements Host.
func (h *ReferenceHost) Fetch(ticket hostproto.TicketID, capability string) (hostproto.ExecutorView, error) {
	entry, err := h.store.GetEntry(ticket.Key())
	if err != nil {
		return hostproto.ExecutorView{}, err
	}
	binding, err := hostproto.CheckFetch(entry, ticket, capability, h.verifier())
	if err != nil {
		h.journalRefusal(ticket, err)
		return hostproto.ExecutorView{}, err
	}
	fetched, err := h.store.UpdateEntry(ticket.Key(), func(e hostproto.Entry) (hostproto.Entry, error) {
		return e.Fetch(binding.CapDigest, h.now)
	})
	if err != nil {
		return hostproto.ExecutorView{}, err
	}
	request, err := h.store.GetRequest(ticket.RequestID)
	if err != nil {
		return hostproto.ExecutorView{}, err
	}
	h.journal(hostproto.Event{Kind: hostproto.EventFetched, Ticket: ticket.String()})
	return request.ViewFor(fetched.Ticket), nil
}

func (h *ReferenceHost) journalRefusal(ticket hostproto.TicketID, err error) {
	kind := hostproto.EventExposureAttempt
	if errors.Is(err, hostproto.ErrNonceMismatch) || errors.Is(err, hostproto.ErrTicketMismatch) {
		kind = hostproto.EventRejected
	}
	h.journal(hostproto.Event{Kind: kind, Ticket: ticket.String(), Detail: err.Error()})
}

// Submit implements Host.
func (h *ReferenceHost) Submit(submission Submit) (Receipt, error) {
	entry, err := h.store.GetEntry(submission.Ticket.Key())
	if err != nil {
		return Receipt{}, err
	}
	request, err := h.store.GetRequest(submission.Ticket.RequestID)
	if err != nil {
		return Receipt{}, err
	}
	schemaDigest, err := request.SchemaDigest()
	if err != nil {
		return Receipt{}, err
	}
	binding, err := hostproto.CheckBinding(entry, schemaDigest, hostproto.Submission{Ticket: submission.Ticket, Nonce: submission.Nonce, Cap: submission.Cap}, h.verifier())
	if err != nil {
		h.journalRefusal(submission.Ticket, err)
		return Receipt{}, err
	}
	if !satisfiesSchema(request.ResponseSchema, submission.Output) {
		return h.failSchema(submission.Ticket)
	}
	return h.accept(submission, binding)
}

func (h *ReferenceHost) failSchema(ticket hostproto.TicketID) (Receipt, error) {
	var allowed bool
	_, err := h.store.UpdateEntry(ticket.Key(), func(e hostproto.Entry) (hostproto.Entry, error) {
		next, correctionAllowed, failure := e.FailSchema()
		allowed = correctionAllowed
		return next, failure
	})
	if err != nil {
		return Receipt{}, err
	}
	h.journal(hostproto.Event{Kind: hostproto.EventSchemaFailed, Ticket: ticket.String()})
	if !allowed {
		h.journal(hostproto.Event{Kind: hostproto.EventRejected, Ticket: ticket.String(), Detail: "schema"})
	}
	return Receipt{CorrectionAllowed: allowed}, ErrSchemaViolation
}

func (h *ReferenceHost) accept(submission Submit, binding hostproto.Binding) (Receipt, error) {
	digest, err := hostproto.OutputDigest(submission.Output)
	if err != nil {
		return Receipt{}, err
	}
	_, err = h.store.UpdateEntry(submission.Ticket.Key(), func(e hostproto.Entry) (hostproto.Entry, error) { return e.Accept(digest) })
	receipt := Receipt{Text: receiptText(submission.Ticket)}
	switch {
	case errors.Is(err, hostproto.ErrAlreadyAccepted):
		return receipt, nil
	case errors.Is(err, hostproto.ErrResubmissionConflict):
		h.journal(hostproto.Event{Kind: hostproto.EventResubmissionConflict, Ticket: submission.Ticket.String()})
		return Receipt{}, err
	case err != nil:
		return Receipt{}, err
	}
	response := hostproto.Response{
		RequestID: submission.Ticket.RequestID, Vote: submission.Ticket.Vote, Nonce: submission.Nonce,
		Output: submission.Output, Executor: h.profile(binding),
	}
	if err := h.store.PutResponse(response); err != nil {
		return Receipt{}, err
	}
	h.journal(hostproto.Event{Kind: hostproto.EventAccepted, Ticket: submission.Ticket.String()})
	if !binding.Attested {
		h.journal(hostproto.Event{Kind: hostproto.EventRestricted, Ticket: submission.Ticket.String()})
	}
	return receipt, nil
}

func receiptText(ticket hostproto.TicketID) string { return "receipt " + ticket.String() }

// profile is the executor profile at acceptance. The transcript digest, and with it
// the attested level, comes with the stop hook.
func (h *ReferenceHost) profile(binding hostproto.Binding) format.ExecutorProfile {
	selection := hostproto.SelectChannel(h.Observed, h.Policy)
	profile := Profile()
	profile.Channel = selection.Channel
	if h.Observed.Allowlist == hostproto.AllowlistUnverified {
		copilot := copilotHarness()
		profile.Harness = format.Harness{Name: copilot.Name, Version: copilot.Version}
	}
	profile.Isolation = selection.Isolation
	if selection.Isolation == format.IsolationRestrictedAttested {
		profile.Isolation = format.IsolationRestricted
	}
	profile.Attestation = format.Attestation{AgentID: binding.AgentID, CapDigest: binding.CapDigest}
	return profile
}

// AfterRun implements Host.
func (h *ReferenceHost) AfterRun(run Run) error {
	if run.SessionLog != nil {
		return h.observeSession(run)
	}
	ticket := run.Ticket.String()
	if run.InheritedHistory {
		h.journal(hostproto.Event{Kind: hostproto.EventHistoryInherited, Ticket: ticket, AgentID: run.AgentID})
	}
	if err := hostproto.CheckExecutorTranscript(run.Transcript, run.Ticket); err != nil {
		h.journal(hostproto.Event{Kind: hostproto.EventExposed, Ticket: ticket, AgentID: run.AgentID, Detail: err.Error()})
		return err
	}
	if run.Reply != receiptText(run.Ticket) {
		h.journal(hostproto.Event{Kind: hostproto.EventLeaked, Ticket: ticket, AgentID: run.AgentID})
		return nil
	}
	h.journal(hostproto.Event{Kind: hostproto.EventAttested, Ticket: ticket, AgentID: run.AgentID, Detail: run.ResolvedModel})
	return nil
}

// observeSession runs the stop check of the harness on the session log of the run
// and journals what it finds, the way the stop hook does.
func (h *ReferenceHost) observeSession(run Run) error {
	copilot := copilotHarness()
	session, err := copilot.ReadRun(run.SessionLog, run.AgentID)
	if err != nil {
		h.journal(hostproto.Event{Kind: hostproto.EventRestricted, AgentID: run.AgentID, Detail: err.Error()})
		return nil
	}
	findings := executorrun.CheckStop(executorrun.StopInput{Harness: copilot, Run: session, Record: run.SessionLog, Reply: run.Reply, Stored: h.storedOutput})
	exposed := false
	for _, finding := range findings {
		h.journal(hostproto.Event{Kind: finding.Kind, Ticket: finding.Ticket, AgentID: run.AgentID, Detail: finding.Detail})
		exposed = exposed || finding.Kind == hostproto.EventExposed
	}
	if exposed {
		return hostproto.ErrTranscriptDeviation
	}
	return nil
}

func (h *ReferenceHost) storedOutput(ticket hostproto.TicketID) json.RawMessage {
	response, err := h.store.GetResponse(ticket.Key())
	if err != nil {
		return nil
	}
	return response.Output
}

// Advance implements Host.
func (h *ReferenceHost) Advance(duration time.Duration) { h.now = h.now.Add(duration) }

// Sweep implements Host.
func (h *ReferenceHost) Sweep(request hostproto.RequestID) (hostproto.Ticket, error) {
	key := hostproto.Key{RequestID: request}
	entry, err := h.store.GetEntry(key)
	if err != nil {
		return hostproto.Ticket{}, err
	}
	if entry.State == hostproto.StateFetched {
		if expired, err := entry.Expire(h.now); err == nil {
			entry = h.apply(key, expired, hostproto.EventExpired)
		}
	}
	if entry.State == hostproto.StateRejected || entry.State == hostproto.StateExpired {
		nonce, err := hostproto.NewNonce(h.random)
		if err != nil {
			return hostproto.Ticket{}, err
		}
		reissued, err := entry.Reissue(nonce)
		if err != nil {
			return hostproto.Ticket{}, err
		}
		kind := hostproto.EventEmitted
		if reissued.State == hostproto.StateAbandoned {
			kind = hostproto.EventAbandoned
		}
		entry = h.apply(key, reissued, kind)
	}
	return entry.Ticket, nil
}

func (h *ReferenceHost) apply(key hostproto.Key, next hostproto.Entry, kind hostproto.EventKind) hostproto.Entry {
	stored, err := h.store.UpdateEntry(key, func(hostproto.Entry) (hostproto.Entry, error) { return next, nil })
	if err != nil {
		panic(err)
	}
	h.journal(hostproto.Event{Kind: kind, Ticket: stored.Ticket.TicketID.String()})
	return stored
}

// Entry implements Host.
func (h *ReferenceHost) Entry(key hostproto.Key) (hostproto.Entry, error) {
	return h.store.GetEntry(key)
}

// Profile implements Host: the stored profile, upgraded to attested when the stop
// hook journaled the attestation, lowered to inline when the journal says the run
// was not shown to be blind, and downgraded by no other mark.
func (h *ReferenceHost) Profile(key hostproto.Key) (format.ExecutorProfile, error) {
	response, err := h.store.GetResponse(key)
	if err != nil {
		return format.ExecutorProfile{}, err
	}
	profile := response.Executor
	events := h.Journal()
	attested := slices.ContainsFunc(events, func(e hostproto.Event) bool {
		return e.Kind == hostproto.EventAttested && e.AgentID == profile.Attestation.AgentID
	})
	if attested && profile.Attestation.AgentID != "" && h.Observed.Allowlist == hostproto.AllowlistEnforced {
		profile.Isolation = format.IsolationRestrictedAttested
		profile.Attestation.TranscriptDigest = Digest("transcript/" + profile.Attestation.AgentID)
	}
	for _, event := range events {
		if event.Kind == hostproto.EventAttested && event.AgentID == profile.Attestation.AgentID && event.Detail != "" {
			profile.Model = format.ExecutorModel{Name: event.Detail, Source: format.ModelSourceResolved}
		}
	}
	if executorrun.BlindnessLost(h.responseEvents(key, response.Executor)) {
		profile.Isolation = format.IsolationInline
	}
	if profile.Harness.Name == hostproto.HarnessCopilot {
		profile = executorrun.Effective(profile, h.responseEvents(key, response.Executor))
	}
	return profile, nil
}

// responseEvents are the journaled events that bear on the response of a key.
func (h *ReferenceHost) responseEvents(key hostproto.Key, stored format.ExecutorProfile) []hostproto.Event {
	entry, err := h.store.GetEntry(key)
	if err != nil {
		return nil
	}
	records, _ := h.store.Records()
	return executorrun.IndexJournal(records).Response(entry.Ticket.TicketID.String(), stored)
}

// Journal implements Host.
func (h *ReferenceHost) Journal() []hostproto.Event {
	records, _ := h.store.Records()
	events := make([]hostproto.Event, len(records))
	for index, record := range records {
		events[index] = record.Event
	}
	return events
}

// satisfiesSchema checks an output against the subset of JSON Schema the fixture
// schemas use: an object, required members, closed members and string enums.
func satisfiesSchema(schema, output json.RawMessage) bool {
	var spec struct {
		Required             []string `json:"required"`
		AdditionalProperties *bool    `json:"additionalProperties"`
		Properties           map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
	}
	var members map[string]json.RawMessage
	if json.Unmarshal(schema, &spec) != nil || json.Unmarshal(output, &members) != nil {
		return false
	}
	for _, name := range spec.Required {
		if _, present := members[name]; !present {
			return false
		}
	}
	for name, value := range members {
		property, known := spec.Properties[name]
		if !known {
			if spec.AdditionalProperties != nil && !*spec.AdditionalProperties {
				return false
			}
			continue
		}
		var text string
		if len(property.Enum) > 0 && (json.Unmarshal(value, &text) != nil || !slices.Contains(property.Enum, text)) {
			return false
		}
	}
	return true
}
