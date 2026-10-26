// Package hookhandler serves the hooks a host harness runs around the executor of a
// request: it rebuilds the launch of an executor, journals its start, issues its
// capability, checks its transcript when it stops and records what the harness
// reports about the run. A hook decides nothing about a proof; it only produces
// the evidence the lifecycle and the claim power read.
//
// The plugin's hooks run in every project of the user, so a hook first decides
// whether the call concerns the module's executors at all and opens the host
// store only when it does.
package hookhandler

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/executorrun"
	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
)

// Reasons a hook gives when it refuses a call.
const (
	reasonNotExecutorTicket = "the call names no ticket of this module"
	reasonNotYourTicket     = "the ticket is not for this role"
	reasonMainAgent         = "the executor tools belong to the executor subagents"
	reasonInternal          = "the module could not check the call"
	reasonLaunchRole        = "the ticket belongs to another role than the subagent"
)

// Details the stop hook journals about a transcript it could not use.
const (
	reasonTranscriptUnreadable = "transcript unreadable: "
	reasonForeignRollout       = "the rollout belongs to thread %q, not to the stopped agent"
)

// Store is what the hooks read and write of the host directory.
type Store interface {
	hostproto.Tickets
	hostproto.Responses
	hostproto.Secrets
	hostproto.Journal
}

// TranscriptSource reads the transcript of a subagent run.
type TranscriptSource interface {
	Read(path string) ([]byte, error)
}

// Handler serves the hooks of one harness.
type Handler struct {
	Harness harness.Harness
	// Open opens the host store. A hook that is not about the module's executors
	// never calls it.
	Open        func() (Store, error)
	Transcripts TranscriptSource

	store Store
}

// ErrUnsupportedHook is returned for a hook kind the module does not serve.
var ErrUnsupportedHook = errors.New("unsupported hook")

// Handle runs the hook on the payload the harness sent and returns what the
// harness reads from standard output: a decision, or nothing when the hook has no
// opinion.
func (h Handler) Handle(kind hostproto.HookKind, payload []byte) ([]byte, error) {
	decoder, err := hostproto.DecoderFor(h.Harness.Name)
	if err != nil {
		return nil, err
	}
	event, err := decoder.Decode(kind, payload)
	if err != nil {
		return nil, err
	}
	switch kind {
	case hostproto.HookPreTool:
		return h.preTool(event)
	case hostproto.HookSubagentStart:
		return nil, h.subagentStart(event)
	case hostproto.HookSubagentStop:
		return nil, h.subagentStop(event)
	case hostproto.HookPostTool:
		return nil, h.postTool(event)
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedHook, kind)
	}
}

// bound returns the handler with its store opened.
func (h Handler) bound() (Handler, error) {
	store, err := h.Open()
	if err != nil {
		return h, err
	}
	h.store = store
	return h, nil
}

func (h Handler) journal(kind hostproto.EventKind, ticket, agentID, role, detail string) error {
	_, err := h.store.Append(hostproto.Event{Kind: kind, Ticket: ticket, AgentID: agentID, Role: role, Detail: detail})
	if err != nil {
		return fmt.Errorf("journal %s: %w", kind, err)
	}
	return nil
}

func (h Handler) decide(permission hostproto.Permission, reason string, updated json.RawMessage) ([]byte, error) {
	return hostproto.EncodePreToolDecision(h.Harness.Name, hostproto.PreToolDecision{Permission: permission, Reason: reason, UpdatedInput: updated})
}

func (h Handler) deny(reason string) ([]byte, error) {
	return h.decide(hostproto.PermissionDeny, reason, nil)
}

// roleOfAgent is the role an agent runs as: the role of the type the harness
// reports, or the role the journal recorded when the agent started. It is empty
// for an agent that is not one of the module's executors.
func (h Handler) roleOfAgent(event hostproto.HookEvent) (string, error) {
	if role, ours := h.Harness.RoleOf(event.AgentType); ours {
		return role, nil
	}
	if event.AgentID == "" || event.AgentType != "" {
		return "", nil
	}
	records, err := h.store.Records()
	if err != nil {
		return "", err
	}
	for _, record := range records {
		if record.Event.Kind == hostproto.EventAgentStarted && record.Event.AgentID == event.AgentID {
			return record.Event.Role, nil
		}
	}
	return "", nil
}

func (h Handler) preTool(event hostproto.HookEvent) ([]byte, error) {
	if h.Harness.Behaviours.GuardsSpawn && hostproto.IsCodexSpawnTool(event.ToolName) {
		return h.guardSpawn(event)
	}
	if hostproto.IsAgentTool(event.ToolName) && h.Harness.Behaviours.RewritesLaunch {
		input, err := hostproto.DecodeAgentInput(event.ToolInput)
		if err != nil {
			return nil, nil
		}
		role, ours := h.Harness.RoleOf(input.SubagentType)
		if !ours {
			return nil, nil
		}
		bound, err := h.bound()
		if err != nil {
			return h.deny(reasonInternal + ": " + err.Error())
		}
		return bound.rewriteLaunch(input, role)
	}
	tool, executorTool := h.Harness.ExecutorTool(event.ToolName)
	if !executorTool || tool == hostproto.ToolDrain {
		return nil, nil
	}
	bound, err := h.bound()
	if err != nil {
		return h.deny(reasonInternal + ": " + err.Error())
	}
	decision, err := bound.guardExecutorTool(event)
	if err != nil {
		return h.deny(reasonInternal + ": " + err.Error())
	}
	return decision, nil
}

// guardSpawn refuses a spawn of an executor of the module that does not ask for
// a child with none of the spawning agent's history, and has no opinion on any
// other spawn. It opens no state: the input alone decides.
func (h Handler) guardSpawn(event hostproto.HookEvent) ([]byte, error) {
	spawn, err := hostproto.DecodeCodexSpawn(event.ToolInput)
	if err != nil {
		return nil, nil
	}
	if _, ours := h.Harness.RoleOf(spawn.AgentType); !ours || spawn.RequestsNoHistory() {
		return nil, nil
	}
	return h.deny(hostproto.CodexSpawnDenial(spawn.AgentType))
}

// rewriteLaunch rebuilds the launch of an executor to the bare ticket, in the
// foreground, with no model: the ticket must exist and be for the subagent's role.
func (h Handler) rewriteLaunch(input hostproto.AgentInput, role string) ([]byte, error) {
	sanitized, err := input.Sanitize(h.Harness.Identity.Plugin)
	if err != nil {
		return h.deny(err.Error())
	}
	ticket, err := hostproto.FindTicketID(sanitized.Prompt)
	if err != nil {
		return h.deny(err.Error())
	}
	entry, err := h.store.GetEntry(ticket.Key())
	switch {
	case errors.Is(err, hostproto.ErrNotFound):
		return h.deny(reasonNotExecutorTicket)
	case err != nil:
		return h.deny(reasonInternal + ": " + err.Error())
	case entry.Role != role:
		return h.deny(reasonLaunchRole)
	}
	updated, err := json.Marshal(sanitized)
	if err != nil {
		return h.deny(reasonInternal)
	}
	return h.decide(hostproto.PermissionAllow, "", updated)
}

// guardExecutorTool lets the executor of the ticket's role call the executor
// tools, with its capability added, and refuses anyone else, journaling the
// attempt.
func (h Handler) guardExecutorTool(event hostproto.HookEvent) ([]byte, error) {
	ticket, err := hostproto.TicketOfInput(event.ToolInput)
	if err != nil {
		return h.refuse(event, hostproto.TicketID{}, reasonNotExecutorTicket)
	}
	entry, err := h.store.GetEntry(ticket.Key())
	if errors.Is(err, hostproto.ErrNotFound) {
		return h.refuse(event, ticket, reasonNotExecutorTicket)
	}
	if err != nil {
		return nil, err
	}
	role, err := h.roleOfAgent(event)
	if err != nil {
		return nil, err
	}
	switch {
	case event.AgentID == "" && role == "":
		return h.refuseUnidentified(event, ticket)
	case role != entry.Role:
		return h.refuse(event, ticket, reasonNotYourTicket)
	case !h.Harness.Behaviours.IssuesCap:
		return nil, nil
	}
	return h.issueCap(event)
}

// refuseUnidentified refuses a call that carries no agent identity, which is the
// main agent's, except on a harness that does not identify the caller of a tool,
// where nothing can be concluded from the missing identity.
func (h Handler) refuseUnidentified(event hostproto.HookEvent, ticket hostproto.TicketID) ([]byte, error) {
	if !h.Harness.Behaviours.IdentifiesCaller {
		return nil, nil
	}
	return h.refuse(event, ticket, reasonMainAgent)
}

func (h Handler) issueCap(event hostproto.HookEvent) ([]byte, error) {
	secret, err := h.store.Secret()
	if err != nil {
		return nil, err
	}
	capability, err := hostproto.FormatCap(secret, event.AgentID)
	if err != nil {
		return h.refuse(event, hostproto.TicketID{}, err.Error())
	}
	updated, err := hostproto.InjectCap(event.ToolInput, capability)
	if err != nil {
		return nil, err
	}
	return h.decide(hostproto.PermissionAllow, "", updated)
}

func (h Handler) refuse(event hostproto.HookEvent, ticket hostproto.TicketID, reason string) ([]byte, error) {
	ticketText := ""
	if ticket.RequestID != "" {
		ticketText = ticket.String()
	}
	if err := h.journal(hostproto.EventExposureAttempt, ticketText, event.AgentID, event.AgentType, reason); err != nil {
		return nil, err
	}
	return h.deny(reason)
}

func (h Handler) subagentStart(event hostproto.HookEvent) error {
	role, ours := h.Harness.RoleOf(event.AgentType)
	if !h.Harness.Behaviours.JournalsStart || !ours {
		return nil
	}
	bound, err := h.bound()
	if err != nil {
		return err
	}
	return bound.journal(hostproto.EventAgentStarted, "", event.AgentID, role, "")
}

// runCheck reads the transcript of a stopped executor of a role and journals
// what it shows.
type runCheck func(h Handler, event hostproto.HookEvent, role string) error

// runChecks are the checks of a stopped executor, one per transcript format.
var runChecks = map[hostproto.TranscriptFormat]runCheck{
	hostproto.TranscriptAgentJSONL:     Handler.checkRun,
	hostproto.TranscriptCopilotSession: Handler.checkRun,
	hostproto.TranscriptCodexRollout:   Handler.checkBlindness,
}

// ErrUnsupportedTranscript is returned for a harness whose transcript format no
// stop check reads.
var ErrUnsupportedTranscript = errors.New("unsupported transcript format")

func (h Handler) subagentStop(event hostproto.HookEvent) error {
	role, ours := h.Harness.RoleOf(event.AgentType)
	if !h.Harness.Behaviours.ChecksStop || !ours {
		return nil
	}
	check, known := runChecks[h.Harness.Transcript]
	if !known {
		return fmt.Errorf("%w: %q", ErrUnsupportedTranscript, h.Harness.Transcript)
	}
	bound, err := h.bound()
	if err != nil {
		return err
	}
	return check(bound, event, role)
}

// checkRun reads the record of a stopped executor and journals what it shows: the
// findings of the stop check, each bound to the agent and the role.
func (h Handler) checkRun(event hostproto.HookEvent, role string) error {
	record, err := h.Transcripts.Read(event.TranscriptPath)
	if err != nil {
		return h.journal(hostproto.EventRestricted, "", event.AgentID, role, reasonTranscriptUnreadable+err.Error())
	}
	run, err := h.Harness.ReadRun(record, event.AgentID)
	if err != nil {
		return h.journal(hostproto.EventRestricted, "", event.AgentID, role, "transcript undecodable: "+err.Error())
	}
	findings := executorrun.CheckStop(executorrun.StopInput{
		Harness: h.Harness, Run: run, Record: record, Reply: event.LastMessage, Stored: h.storedOutput,
	})
	for _, finding := range findings {
		if err := h.journal(finding.Kind, finding.Ticket, event.AgentID, role, finding.Detail); err != nil {
			return err
		}
	}
	return nil
}

// blindnessEvents are the events that record a run not shown to be blind, by
// what its rollout shows.
var blindnessEvents = map[hostproto.ChildHistory]hostproto.EventKind{
	hostproto.ChildInherited: hostproto.EventHistoryInherited,
	hostproto.ChildUnproven:  hostproto.EventBlindnessUnproven,
}

// checkBlindness reads the rollout of a stopped executor: a run that started with
// history of the agent that spawned it, or whose rollout cannot show that it did
// not, is journaled against its ticket, so that its answer is credited at the
// isolation `inline`; a reply other than the bare receipt is journaled as a leak.
// The ticket comes from the rollout, or else from the receipt the run replied
// with; without either, the event names the agent alone.
func (h Handler) checkBlindness(event hostproto.HookEvent, role string) error {
	run := h.readChildRun(event)
	ticket, found := run.Ticket, run.TicketFound
	if !found {
		ticket, found = executorrun.ReceiptTicket(event.LastMessage)
	}
	ticketText := ""
	if found {
		ticketText = ticket.String()
	}
	if kind, lost := blindnessEvents[run.History]; lost {
		if err := h.journal(kind, ticketText, event.AgentID, role, run.Reason); err != nil {
			return err
		}
	}
	if found && !executorrun.IsReceipt(event.LastMessage, ticket) {
		return h.journal(hostproto.EventLeaked, ticketText, event.AgentID, role, executorrun.ConditionNotReceipt)
	}
	return nil
}

// readChildRun reads the rollout of the stopped agent. A rollout that cannot be
// read, or that belongs to another thread than the agent, proves nothing.
func (h Handler) readChildRun(event hostproto.HookEvent) hostproto.CodexChildRun {
	transcript, err := h.Transcripts.Read(event.TranscriptPath)
	if err != nil {
		return hostproto.CodexChildRun{History: hostproto.ChildUnproven, Reason: reasonTranscriptUnreadable + err.Error()}
	}
	run := hostproto.ReadCodexChildRollout(transcript)
	if run.History == hostproto.ChildBlind && run.ThreadID != event.AgentID {
		run.History, run.Reason = hostproto.ChildUnproven, fmt.Sprintf(reasonForeignRollout, run.ThreadID)
	}
	return run
}

// storedOutput is the output the module accepted for a ticket, nil when none.
func (h Handler) storedOutput(ticket hostproto.TicketID) json.RawMessage {
	response, err := h.store.GetResponse(ticket.Key())
	if err != nil {
		return nil
	}
	return response.Output
}

// postTool records what the harness reported about a finished executor launch:
// the model it resolved and the usage, and a reply that was more than the receipt.
func (h Handler) postTool(event hostproto.HookEvent) error {
	if !h.Harness.Behaviours.ReportsRun || !hostproto.IsAgentTool(event.ToolName) {
		return nil
	}
	input, err := hostproto.DecodeAgentInput(event.ToolInput)
	if err != nil {
		return nil
	}
	role, ours := h.Harness.RoleOf(input.SubagentType)
	if !ours {
		return nil
	}
	ticket, err := hostproto.FindTicketID(input.Prompt)
	if err != nil {
		return nil
	}
	bound, err := h.bound()
	if err != nil {
		return err
	}
	if event.Reply != "" && !executorrun.IsReceipt(event.Reply, ticket) {
		if err := bound.journal(hostproto.EventLeaked, ticket.String(), "", role, executorrun.ConditionNotReceipt); err != nil {
			return err
		}
	}
	report := executorrun.RunReport{Model: event.ResolvedModel, Usage: event.Usage}
	return bound.journal(hostproto.EventRunReported, ticket.String(), "", role, executorrun.RunReportDetail(report))
}
