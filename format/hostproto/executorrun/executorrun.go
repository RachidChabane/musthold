// Package executorrun holds what a proof module knows of one executor run: the bare
// receipt a well-behaved executor replies with, the details the hooks journal
// about the run, and the profile a response is credited with once the journal is
// read. It knows no I/O.
package executorrun

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
)

// receiptPrefix starts the receipt `submit_response` returns.
const receiptPrefix = "receipt "

// Receipt is the whole reply of an executor that did its job.
func Receipt(ticket hostproto.TicketID) string { return receiptPrefix + ticket.String() }

// ReceiptTicket reads the ticket of a reply that is a bare receipt, whatever
// whitespace surrounds it.
func ReceiptTicket(reply string) (hostproto.TicketID, bool) {
	text, found := strings.CutPrefix(strings.TrimSpace(reply), receiptPrefix)
	if !found {
		return hostproto.TicketID{}, false
	}
	ticket, err := hostproto.ParseTicketID(text)
	return ticket, err == nil
}

// attestation is the detail of an `attested` event.
type attestation struct {
	TranscriptDigest string `json:"transcriptDigest"`
}

// AttestationDetail encodes the detail of an `attested` event.
func AttestationDetail(transcriptDigest string) string {
	return encode(attestation{TranscriptDigest: transcriptDigest})
}

// RunReport is the detail of a `run_reported` event.
type RunReport struct {
	Model string               `json:"model,omitempty"`
	Usage hostproto.TokenUsage `json:"usage"`
}

// RunReportDetail encodes the detail of a `run_reported` event.
func RunReportDetail(report RunReport) string { return encode(report) }

// Witness is the detail of a `classify_witnessed` event: the model the raw API
// log shows answered the calls, and the models of every call.
type Witness struct {
	Model  string   `json:"model"`
	Models []string `json:"models"`
}

// WitnessDetail encodes the detail of a `classify_witnessed` event.
func WitnessDetail(witness Witness) string { return encode(witness) }

func encode(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// blindnessKinds are the events that say an executor run was not shown to be
// blind: they put its answer at the isolation `inline`.
var blindnessKinds = []hostproto.EventKind{hostproto.EventHistoryInherited, hostproto.EventBlindnessUnproven}

// markKinds are the events that cost a response its attestation and its judged
// pass credit.
var markKinds = append([]hostproto.EventKind{hostproto.EventLeaked, hostproto.EventExposed, hostproto.EventResubmissionConflict}, blindnessKinds...)

// Marked reports whether the events of a ticket mark its response as leaked,
// exposed, conflicting, or not shown to be blind.
func Marked(events []hostproto.Event) bool { return containsKind(events, markKinds) }

// BlindnessLost reports whether the events of a response say that the run that
// gave it was not shown to be blind.
func BlindnessLost(events []hostproto.Event) bool { return containsKind(events, blindnessKinds) }

func containsKind(events []hostproto.Event, kinds []hostproto.EventKind) bool {
	return slices.ContainsFunc(events, func(event hostproto.Event) bool { return slices.Contains(kinds, event.Kind) })
}

// Effective returns the profile a response is credited with: the profile stored at
// acceptance, lowered to `inline` when the run that gave it was not shown to be
// blind, raised to `restricted-attested` when the stop hook attested the very
// agent that answered and nothing marks the run, and carrying the model the
// harness resolved. An unmarked Copilot subagent answer stored as
// `restricted-unverified` reaches `restricted` when both observations identify
// its answering agent; it never reaches `restricted-attested`. The events are
// those JournalIndex.Response gives for the response.
func Effective(stored format.ExecutorProfile, events []hostproto.Event) format.ExecutorProfile {
	profile := stored
	if report, found := lastReport(events); found && report.Model != "" {
		profile.Model = format.ExecutorModel{Name: report.Model, Source: format.ModelSourceResolved}
	}
	if BlindnessLost(events) {
		profile.Isolation = format.IsolationInline
		return profile
	}
	if raised, witnessed := witnessedProfile(profile, events); witnessed {
		return raised
	}
	if profile.Harness.Name == hostproto.HarnessCopilot {
		if profile.Channel == format.ChannelHostSubagent && profile.Isolation == format.IsolationRestrictedUnverified &&
			!Marked(events) && observedRun(events, profile.Attestation.AgentID) {
			profile.Isolation = format.IsolationRestricted
		}
		// Observations grant no attestation, even if this profile is read again.
		return profile
	}
	if profile.Isolation != format.IsolationRestricted || Marked(events) {
		return profile
	}
	if digest, attested := attestedDigest(events, stored.Attestation.AgentID); attested {
		profile.Isolation = format.IsolationRestrictedAttested
		profile.Attestation.TranscriptDigest = digest
	}
	return profile
}

// observedRun requires both observations for the same ticket and nonempty agent.
// The stop hook binds that agent to the stored answer by checking its submitted
// output. Copilot issues no capability, so acceptance normally records no agent;
// when a stored identity exists, the observations must name that agent.
func observedRun(events []hostproto.Event, answeringAgent string) bool {
	type run struct{ ticket, agent string }
	allowed := map[run]bool{}
	for _, event := range events {
		if event.Kind == hostproto.EventAllowlistObserved && event.Ticket != "" && event.AgentID != "" &&
			(answeringAgent == "" || event.AgentID == answeringAgent) {
			allowed[run{event.Ticket, event.AgentID}] = true
		}
	}
	return slices.ContainsFunc(events, func(event hostproto.Event) bool {
		return event.Kind == hostproto.EventLaunchBare && allowed[run{event.Ticket, event.AgentID}]
	})
}

// witnessedProfile raises a classification from `restricted-unverified` to
// `restricted`, with the model the raw API log resolved, when the check of that log
// journaled a witness for it and nothing marks the run. It never reaches
// `restricted-attested`: no hook issues a capability for a mod's model call.
func witnessedProfile(profile format.ExecutorProfile, events []hostproto.Event) (format.ExecutorProfile, bool) {
	if profile.Channel != format.ChannelHostClassify || profile.Isolation != format.IsolationRestrictedUnverified || Marked(events) {
		return profile, false
	}
	for _, event := range slices.Backward(events) {
		var witness Witness
		if event.Kind == hostproto.EventClassifyWitnessed && json.Unmarshal([]byte(event.Detail), &witness) == nil && witness.Model != "" {
			profile.Isolation = format.IsolationRestricted
			profile.Model = format.ExecutorModel{Name: witness.Model, Source: format.ModelSourceResolved}
			return profile, true
		}
	}
	return profile, false
}

func attestedDigest(events []hostproto.Event, agentID string) (string, bool) {
	if agentID == "" {
		return "", false
	}
	for _, event := range slices.Backward(events) {
		if event.Kind != hostproto.EventAttested || event.AgentID != agentID {
			continue
		}
		var detail attestation
		if json.Unmarshal([]byte(event.Detail), &detail) == nil && format.IsSHA256Hex(detail.TranscriptDigest) {
			return detail.TranscriptDigest, true
		}
	}
	return "", false
}

// lastReport is the latest run report of the ticket. The report comes from the
// hook on the launch, which knows the ticket and not the agent.
func lastReport(events []hostproto.Event) (RunReport, bool) {
	for _, event := range slices.Backward(events) {
		if event.Kind != hostproto.EventRunReported {
			continue
		}
		var report RunReport
		if json.Unmarshal([]byte(event.Detail), &report) == nil {
			return report, true
		}
	}
	return RunReport{}, false
}
