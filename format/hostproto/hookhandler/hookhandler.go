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
	reasonNotBareReceipt    = "the reply is not the bare receipt"
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

func (h Handler) subagentStop(event hostproto.HookEvent) error {
	role, ours := h.Harness.RoleOf(event.AgentType)
	if !h.Harness.Behaviours.ChecksStop || !ours {
		return nil
	}
	bound, err := h.bound()
	if err != nil {
		return err
	}
	return bound.checkRun(event, role)
}

// checkRun reads the record of a stopped executor and journals what it shows: the
// findings of the stop check, each bound to the agent and the role.
func (h Handler) checkRun(event hostproto.HookEvent, role string) error {
	record, err := h.Transcripts.Read(event.TranscriptPath)
	if err != nil {
		return h.journal(hostproto.EventRestricted, "", event.AgentID, role, "transcript unreadable: "+err.Error())
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
		if err := bound.journal(hostproto.EventLeaked, ticket.String(), "", role, reasonNotBareReceipt); err != nil {
			return err
		}
	}
	report := executorrun.RunReport{Model: event.ResolvedModel, Usage: event.Usage}
	return bound.journal(hostproto.EventRunReported, ticket.String(), "", role, executorrun.RunReportDetail(report))
}
