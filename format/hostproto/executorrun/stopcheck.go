package executorrun

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/harness"
)

// Conditions a stop check names in the detail of the event it journals when a run
// fails to show them. Each is one rule of the observation of a run.
const (
	ConditionNeverFetched      = "the run never fetched a ticket"
	ConditionNotReceipt        = "the reply is not the bare receipt"
	ConditionCLIVersion        = "the CLI is not the pinned version"
	ConditionNoSelection       = "the log shows no tool selection for the agent"
	ConditionManySelections    = "the log shows more than one tool selection for the agent"
	ConditionWideSelection     = "the harness selected a tool besides the two executor tools"
	ConditionShortSelection    = "the harness did not select exactly the two executor tools"
	ConditionNoStoredAnswer    = "the ticket has no stored answer to compare with"
	ConditionOutputMismatch    = "the submitted output is not the stored answer"
	ConditionPromptNotBare     = "the executor's prompt is not the bare ticket"
	ConditionProfileNotShipped = "the executor's system prompt does not begin with its shipped profile"
)

// Finding is one event a stop check asks the hook to journal. The hook adds the
// agent and the role.
type Finding struct {
	Kind   hostproto.EventKind
	Ticket string
	Detail string
}

// StopInput is what a stop check reads of one stopped executor.
type StopInput struct {
	// Harness is the harness the executor ran on.
	Harness harness.Harness
	// Run is the executor's run as the record shows it.
	Run hostproto.SessionRun
	// Record is the record the run was read from, whose digest is journaled.
	Record []byte
	// Reply is the last message of the executor.
	Reply string
	// Stored returns the output accepted for a ticket, nil when none was.
	Stored func(hostproto.TicketID) json.RawMessage
}

// IsReceipt reports whether a reply is the bare receipt of the ticket, whatever
// whitespace the model put around it.
func IsReceipt(reply string, ticket hostproto.TicketID) bool {
	return strings.TrimSpace(reply) == Receipt(ticket)
}

// CheckStop decides what the stop of an executor shows. A run whose calls are not
// exactly the fetch and the submit of one ticket is exposed, one whose reply is
// more than the receipt has leaked. A harness that observes selections then gets
// the observation of the run; any other attests it.
func CheckStop(in StopInput) []Finding {
	calls := canonicalCalls(in.Harness, in.Run.Calls)
	ticket, fetched := fetchedTicket(calls)
	if !fetched {
		return []Finding{{Kind: hostproto.EventExposed, Detail: ConditionNeverFetched}}
	}
	text := ticket.String()
	if err := hostproto.CheckExecutorTranscript(calls, ticket); err != nil {
		return []Finding{{Kind: hostproto.EventExposed, Ticket: text, Detail: err.Error()}}
	}
	if !IsReceipt(in.Reply, ticket) {
		return []Finding{{Kind: hostproto.EventLeaked, Ticket: text, Detail: ConditionNotReceipt}}
	}
	digest := hostproto.TranscriptDigest(in.Record)
	if !in.Harness.Behaviours.ObservesSelection {
		return []Finding{{Kind: hostproto.EventAttested, Ticket: text, Detail: AttestationDetail(digest)}}
	}
	return []Finding{observeAllowlist(in, calls, ticket, digest), observeLaunch(in, ticket)}
}

// canonicalCalls renames the executor tools among the calls to their own names,
// whatever form the harness spells them in. Any other call keeps its name.
func canonicalCalls(h harness.Harness, calls []hostproto.ToolCall) []hostproto.ToolCall {
	renamed := make([]hostproto.ToolCall, len(calls))
	for index, call := range calls {
		renamed[index] = call
		if tool, executor := h.ExecutorTool(call.Name); executor {
			renamed[index].Name = tool
		}
	}
	return renamed
}

// fetchedTicket is the ticket of the first fetch an executor made.
func fetchedTicket(calls []hostproto.ToolCall) (hostproto.TicketID, bool) {
	for _, call := range calls {
		if call.Name != hostproto.ToolFetchRequest {
			continue
		}
		if ticket, err := hostproto.TicketOfInput(call.Input); err == nil {
			return ticket, true
		}
	}
	return hostproto.TicketID{}, false
}

// AllowlistObserved is the detail of an `allowlist_observed` event.
type AllowlistObserved struct {
	CLIVersion    string   `json:"cliVersion"`
	SelectedTools []string `json:"selectedTools"`
	LogDigest     string   `json:"logDigest"`
}

// AllowlistObservedDetail encodes the detail of an `allowlist_observed` event.
func AllowlistObservedDetail(observed AllowlistObserved) string { return encode(observed) }

// ParseAllowlistObserved decodes the detail of an `allowlist_observed` event.
func ParseAllowlistObserved(detail string) (AllowlistObserved, bool) {
	var observed AllowlistObserved
	if json.Unmarshal([]byte(detail), &observed) != nil {
		return AllowlistObserved{}, false
	}
	return observed, true
}

func restrictedFinding(ticket hostproto.TicketID, condition string) Finding {
	return Finding{Kind: hostproto.EventRestricted, Ticket: ticket.String(), Detail: condition}
}

func exposedFinding(ticket hostproto.TicketID, condition string) Finding {
	return Finding{Kind: hostproto.EventExposed, Ticket: ticket.String(), Detail: condition}
}

// observeAllowlist is the finding about the allowlist of the run: the pinned CLI
// selected exactly the two executor tools for the agent, and the output the agent
// submitted last is the answer the module stored.
func observeAllowlist(in StopInput, calls []hostproto.ToolCall, ticket hostproto.TicketID, digest string) Finding {
	if in.Run.CLIVersion != in.Harness.Version {
		return restrictedFinding(ticket, ConditionCLIVersion)
	}
	selected, failure := soleSelection(in.Run.Selected, in.Harness.ToolNames())
	if failure != "" {
		if failure == ConditionWideSelection {
			return exposedFinding(ticket, failure)
		}
		return restrictedFinding(ticket, failure)
	}
	var stored json.RawMessage
	if in.Stored != nil {
		stored = in.Stored(ticket)
	}
	if condition := outputCondition(calls, stored); condition != "" {
		return restrictedFinding(ticket, condition)
	}
	detail := AllowlistObservedDetail(AllowlistObserved{CLIVersion: in.Run.CLIVersion, SelectedTools: selected, LogDigest: digest})
	return Finding{Kind: hostproto.EventAllowlistObserved, Ticket: ticket.String(), Detail: detail}
}

// soleSelection returns the one selection of the log when it is exactly the
// wanted tools, and the condition it fails otherwise.
func soleSelection(selections [][]string, want []string) ([]string, string) {
	switch {
	case len(selections) == 0:
		return nil, ConditionNoSelection
	case len(selections) > 1:
		return nil, ConditionManySelections
	}
	selected := slices.Clone(selections[0])
	slices.Sort(selected)
	wanted := slices.Sorted(slices.Values(want))
	switch {
	case slices.Equal(selected, wanted):
		return selected, ""
	case slices.ContainsFunc(selected, func(tool string) bool { return !slices.Contains(wanted, tool) }):
		return nil, ConditionWideSelection
	default:
		return nil, ConditionShortSelection
	}
}

// outputCondition is the condition the last submission fails against the stored answer.
func outputCondition(calls []hostproto.ToolCall, stored json.RawMessage) string {
	if len(stored) == 0 {
		return ConditionNoStoredAnswer
	}
	for _, call := range slices.Backward(calls) {
		if call.Name != hostproto.ToolSubmitResponse {
			continue
		}
		submitted, found := hostproto.SubmittedOutput(call)
		if found && sameOutput(submitted, stored) {
			return ""
		}
		return ConditionOutputMismatch
	}
	return ConditionOutputMismatch
}

func sameOutput(first, second []byte) bool {
	firstDigest, firstErr := hostproto.OutputDigest(first)
	secondDigest, secondErr := hostproto.OutputDigest(second)
	return firstErr == nil && secondErr == nil && firstDigest == secondDigest
}

// observeLaunch is the finding about the launch of the run: the executor was given
// the bare ticket as its only prompt and its own profile as its only system prompt.
func observeLaunch(in StopInput, ticket hostproto.TicketID) Finding {
	if len(in.Run.UserMessages) != 1 || in.Run.UserMessages[0] != hostproto.ExecutorPrompt(ticket) {
		return restrictedFinding(ticket, ConditionPromptNotBare)
	}
	if len(in.Run.SystemMessages) != 1 {
		return restrictedFinding(ticket, ConditionProfileNotShipped)
	}
	leading, _, _ := strings.Cut(in.Run.SystemMessages[0], hostproto.SystemBlockSeparator)
	if leading != hostproto.ExecutorInstructions {
		return restrictedFinding(ticket, ConditionProfileNotShipped)
	}
	return Finding{Kind: hostproto.EventLaunchBare, Ticket: ticket.String()}
}
