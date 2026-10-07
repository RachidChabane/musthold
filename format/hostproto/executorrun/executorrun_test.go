package executorrun_test

import (
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/executorrun"
)

const (
	agent      = "agent-0001"
	ticketText = "bbb0107006fad4abd4746fc925c050dff762f20370b46e56c92e74fe9ef5f7a5.0.1"
)

var transcriptDigest = strings.Repeat("a", format.Sha256HexLength)

func stored(isolation format.Isolation) format.ExecutorProfile {
	return format.ExecutorProfile{
		Channel: format.ChannelHostSubagent, Harness: format.Harness{Name: hostproto.HarnessClaudeCode, Version: "2.1.292"},
		Isolation: isolation, Attestation: format.Attestation{AgentID: agent, CapDigest: strings.Repeat("b", format.Sha256HexLength)},
		Model: format.ExecutorModel{Source: format.ModelSourceUnknown},
	}
}

func attested(agentID, digest string) hostproto.Event {
	return hostproto.Event{Kind: hostproto.EventAttested, Ticket: ticketText, AgentID: agentID, Detail: executorrun.AttestationDetail(digest)}
}

func TestEffectiveProfile(t *testing.T) {
	t.Run("an attested run raises a restricted answer to restricted-attested with its transcript digest [verifies SP-701~1]", func(t *testing.T) {
		got := executorrun.Effective(stored(format.IsolationRestricted), []hostproto.Event{attested(agent, transcriptDigest)})
		if got.Isolation != format.IsolationRestrictedAttested || got.Attestation.TranscriptDigest != transcriptDigest {
			t.Fatalf("profile = %+v", got)
		}
		if err := got.Validate(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("an attestation of another agent, or without a usable digest, raises nothing [verifies SP-701~1]", func(t *testing.T) {
		for name, events := range map[string][]hostproto.Event{
			"another agent": {attested("agent-0002", transcriptDigest)},
			"no digest":     {{Kind: hostproto.EventAttested, AgentID: agent}},
			"bad digest":    {attested(agent, "not-a-digest")},
			"no event":      nil,
		} {
			if got := executorrun.Effective(stored(format.IsolationRestricted), events); got.Isolation != format.IsolationRestricted {
				t.Errorf("%s: isolation = %s", name, got.Isolation)
			}
		}
	})

	t.Run("a leak, an exposure or a conflict keeps the attestation from being credited [verifies SP-701~1]", func(t *testing.T) {
		for _, mark := range []hostproto.EventKind{hostproto.EventLeaked, hostproto.EventExposed, hostproto.EventResubmissionConflict} {
			events := []hostproto.Event{attested(agent, transcriptDigest), {Kind: mark, Ticket: ticketText}}
			if got := executorrun.Effective(stored(format.IsolationRestricted), events); got.Isolation != format.IsolationRestricted {
				t.Errorf("%s: isolation = %s", mark, got.Isolation)
			}
			if !executorrun.Marked(events) {
				t.Errorf("%s did not mark the ticket", mark)
			}
		}
		if executorrun.Marked([]hostproto.Event{attested(agent, transcriptDigest)}) {
			t.Error("an attestation marked the ticket")
		}
	})

	t.Run("only a restricted answer is raised, whatever the stop hook saw [verifies SP-701~1]", func(t *testing.T) {
		for _, isolation := range []format.Isolation{format.IsolationUnrestricted, format.IsolationRestrictedUnverified, format.IsolationInline} {
			if got := executorrun.Effective(stored(isolation), []hostproto.Event{attested(agent, transcriptDigest)}); got.Isolation != isolation {
				t.Errorf("%s became %s", isolation, got.Isolation)
			}
		}
	})

	t.Run("the model the harness resolved replaces an unknown one, even on a marked run [verifies SP-701~1]", func(t *testing.T) {
		report := hostproto.Event{Kind: hostproto.EventRunReported, Ticket: ticketText, Detail: executorrun.RunReportDetail(executorrun.RunReport{Model: "claude-sonnet-5-5"})}
		later := hostproto.Event{Kind: hostproto.EventRunReported, Ticket: ticketText, Detail: executorrun.RunReportDetail(executorrun.RunReport{Model: "claude-sonnet-5-6"})}
		got := executorrun.Effective(stored(format.IsolationRestricted), []hostproto.Event{report, later, {Kind: hostproto.EventLeaked}})
		if got.Model.Name != "claude-sonnet-5-6" || got.Model.Source != format.ModelSourceResolved {
			t.Fatalf("model = %+v", got.Model)
		}
		none := hostproto.Event{Kind: hostproto.EventRunReported, Detail: executorrun.RunReportDetail(executorrun.RunReport{})}
		if got := executorrun.Effective(stored(format.IsolationRestricted), []hostproto.Event{none}); got.Model.Source != format.ModelSourceUnknown {
			t.Fatalf("model = %+v", got.Model)
		}
	})

	t.Run("the stored profile is never changed in place [verifies SP-701~1]", func(t *testing.T) {
		profile := stored(format.IsolationRestricted)
		executorrun.Effective(profile, []hostproto.Event{attested(agent, transcriptDigest)})
		if profile.Isolation != format.IsolationRestricted || profile.Attestation.TranscriptDigest != "" {
			t.Fatalf("profile = %+v", profile)
		}
	})

	t.Run("the receipt is the ticket behind a fixed prefix [verifies SP-701~1]", func(t *testing.T) {
		ticket, err := hostproto.ParseTicketID(ticketText)
		if err != nil {
			t.Fatal(err)
		}
		if got := executorrun.Receipt(ticket); got != "receipt "+ticketText {
			t.Fatalf("receipt = %q", got)
		}
	})
}
