// Package hooks serves the hooks a host harness runs around the executor of a
// request: it rebuilds the launch of an executor, journals its start, issues its
// capability, checks its transcript when it stops and records what the harness
// reports about the run. A hook decides nothing about a proof; it only produces
// the evidence the lifecycle and the claim power read.
package hooks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/executorrun"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/hostharness"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// Reasons a hook gives when it refuses a call.
const (
	reasonNotExecutorTicket = "the call names no ticket of this module"
	reasonNotYourTicket     = "the ticket is not for this role"
	reasonMainAgent         = "the executor tools belong to the executor subagents"
	reasonInternal          = "the module could not check the call"
	reasonLaunchRole        = "the ticket belongs to another role than the subagent"
)

// Store is what the hooks read and write of the host directory.
type Store interface {
	hostproto.Tickets
	hostproto.Secrets
	hostproto.Journal
}

// TranscriptSource reads the transcript of a subagent run.
type TranscriptSource interface {
	Read(path string) ([]byte, error)
}

// Handler serves the hooks of one harness.
type Handler struct {
	Harness     hostharness.Harness
	Store       Store
	Transcripts TranscriptSource
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
		return h.subagentStop(event)
	case hostproto.HookPostTool:
		return nil, h.postTool(event)
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedHook, kind)
	}
}

func (h Handler) journal(kind hostproto.EventKind, ticket, agentID, role, detail string) error {
	_, err := h.Store.Append(hostproto.Event{Kind: kind, Ticket: ticket, AgentID: agentID, Role: role, Detail: detail})
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

// ticketOfInput reads the ticket an executor tool call names.
func ticketOfInput(toolInput json.RawMessage) (hostproto.TicketID, error) {
	var input struct {
		Ticket string `json:"ticket"`
	}
	if err := json.Unmarshal(toolInput, &input); err != nil {
		return hostproto.TicketID{}, err
	}
	return hostproto.ParseTicketID(input.Ticket)
}

// roleOfAgent is the role an agent runs as: the type the harness reports, or the
// role the journal recorded when the agent started.
func (h Handler) roleOfAgent(event hostproto.HookEvent) (string, error) {
	if event.AgentType != "" {
		return event.AgentType, nil
	}
	if event.AgentID == "" {
		return "", nil
	}
	records, err := h.Store.Records()
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
		return h.rewriteLaunch(event)
	}
	tool, executorTool := h.Harness.ExecutorTool(event.ToolName)
	if !executorTool || tool == hostproto.ToolDrain {
		return nil, nil
	}
	decision, err := h.guardExecutorTool(event)
	if err != nil {
		return h.deny(reasonInternal + ": " + err.Error())
	}
	return decision, nil
}

// rewriteLaunch rebuilds the launch of an executor to the bare ticket, in the
// foreground, with no model: the ticket must exist and be for the subagent's role.
func (h Handler) rewriteLaunch(event hostproto.HookEvent) ([]byte, error) {
	input, err := hostproto.DecodeAgentInput(event.ToolInput)
	if err != nil || !hostharness.IsExecutorRole(input.SubagentType) {
		return nil, nil
	}
	sanitized, err := input.Sanitize(hostharness.PluginName)
	if err != nil {
		return h.deny(err.Error())
	}
	ticket, err := hostproto.FindTicketID(sanitized.Prompt)
	if err != nil {
		return h.deny(err.Error())
	}
	entry, err := h.Store.GetEntry(ticket.Key())
	switch {
	case errors.Is(err, hostproto.ErrNotFound):
		return h.deny(reasonNotExecutorTicket)
	case err != nil:
		return h.deny(reasonInternal + ": " + err.Error())
	case entry.Role != input.SubagentType:
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
	ticket, err := ticketOfInput(event.ToolInput)
	if err != nil {
		return h.refuse(event, hostproto.TicketID{}, reasonNotExecutorTicket)
	}
	entry, err := h.Store.GetEntry(ticket.Key())
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
	secret, err := h.Store.Secret()
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
	if !h.Harness.Behaviours.JournalsStart || !hostharness.IsExecutorRole(event.AgentType) {
		return nil
	}
	return h.journal(hostproto.EventAgentStarted, "", event.AgentID, event.AgentType, "")
}

func (h Handler) subagentStop(event hostproto.HookEvent) ([]byte, error) {
	if !h.Harness.Behaviours.ChecksStop || !hostharness.IsExecutorRole(event.AgentType) {
		return nil, nil
	}
	return nil, h.checkRun(event)
}

// checkRun reads the transcript of a stopped executor: its calls must be exactly
// the fetch and the submit of one ticket and its reply the bare receipt.
func (h Handler) checkRun(event hostproto.HookEvent) error {
	transcript, err := h.Transcripts.Read(event.TranscriptPath)
	if err != nil {
		return h.journal(hostproto.EventRestricted, "", event.AgentID, event.AgentType, "transcript unreadable: "+err.Error())
	}
	calls, err := hostproto.DecodeTranscript(bytes.NewReader(transcript))
	if err != nil {
		return h.journal(hostproto.EventRestricted, "", event.AgentID, event.AgentType, "transcript undecodable: "+err.Error())
	}
	ticket, found := fetchedTicket(calls)
	if !found {
		return h.journal(hostproto.EventExposed, "", event.AgentID, event.AgentType, "the run never fetched a ticket")
	}
	if err := hostproto.CheckExecutorTranscript(calls, ticket); err != nil {
		return h.journal(hostproto.EventExposed, ticket.String(), event.AgentID, event.AgentType, err.Error())
	}
	if event.LastMessage != executorrun.Receipt(ticket) {
		return h.journal(hostproto.EventLeaked, ticket.String(), event.AgentID, event.AgentType, "the reply is not the bare receipt")
	}
	detail := executorrun.AttestationDetail(hostproto.TranscriptDigest(transcript))
	return h.journal(hostproto.EventAttested, ticket.String(), event.AgentID, event.AgentType, detail)
}

// fetchedTicket is the ticket of the first fetch an executor made.
func fetchedTicket(calls []hostproto.ToolCall) (hostproto.TicketID, bool) {
	for _, call := range calls {
		if toolBase(call.Name) != hostproto.ToolFetchRequest {
			continue
		}
		if ticket, err := ticketOfInput(call.Input); err == nil {
			return ticket, true
		}
	}
	return hostproto.TicketID{}, false
}

// toolBase is a tool's own name, whatever server prefix the harness gives it.
func toolBase(name string) string {
	if tool, ok := hostproto.ParseMCPToolName(name); ok {
		return tool.Tool
	}
	if cut := strings.LastIndex(name, "/"); cut >= 0 {
		return name[cut+1:]
	}
	return name
}

// postTool records what the harness reported about a finished executor launch:
// the model it resolved and the usage, and a reply that was more than the receipt.
func (h Handler) postTool(event hostproto.HookEvent) error {
	if !h.Harness.Behaviours.ReportsRun || !hostproto.IsAgentTool(event.ToolName) {
		return nil
	}
	input, err := hostproto.DecodeAgentInput(event.ToolInput)
	if err != nil || !hostharness.IsExecutorRole(input.SubagentType) {
		return nil
	}
	ticket, err := hostproto.FindTicketID(input.Prompt)
	if err != nil {
		return nil
	}
	if event.Reply != "" && event.Reply != executorrun.Receipt(ticket) {
		if err := h.journal(hostproto.EventLeaked, ticket.String(), "", input.SubagentType, "the reply is not the bare receipt"); err != nil {
			return err
		}
	}
	report := executorrun.RunReport{Model: event.ResolvedModel, Usage: event.Usage}
	return h.journal(hostproto.EventRunReported, ticket.String(), "", input.SubagentType, executorrun.RunReportDetail(report))
}
