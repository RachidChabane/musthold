// Package executorrun holds what the module knows of one executor run: the bare
// receipt a well-behaved executor replies with, the details the hooks journal
// about the run, and the profile a response is credited with once the journal is
// read. It knows no I/O.
package executorrun

import (
	"encoding/json"
	"slices"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// receiptPrefix starts the receipt `submit_response` returns.
const receiptPrefix = "receipt "

// Receipt is the whole reply of an executor that did its job.
func Receipt(ticket hostproto.TicketID) string { return receiptPrefix + ticket.String() }

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

func encode(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// markKinds are the events that cost a response its attestation and its judged
// pass credit.
var markKinds = []hostproto.EventKind{hostproto.EventLeaked, hostproto.EventExposed, hostproto.EventResubmissionConflict}

// Marked reports whether the events of a ticket mark its response as leaked,
// exposed or conflicting.
func Marked(events []hostproto.Event) bool {
	return slices.ContainsFunc(events, func(event hostproto.Event) bool { return slices.Contains(markKinds, event.Kind) })
}

// Effective returns the profile a response is credited with: the profile stored at
// acceptance, raised to `restricted-attested` when the stop hook attested the very
// agent that answered and nothing marks the run, and carrying the model the
// harness resolved. The events are those journaled for the response's ticket.
func Effective(stored format.ExecutorProfile, events []hostproto.Event) format.ExecutorProfile {
	profile := stored
	if report, found := lastReport(events); found && report.Model != "" {
		profile.Model = format.ExecutorModel{Name: report.Model, Source: format.ModelSourceResolved}
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
