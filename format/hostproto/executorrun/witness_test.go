package executorrun_test

import (
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/executorrun"
	"pgregory.net/rapid"
)

const witnessedModel = "claude-sonnet-5-5-20260101"

func classified(isolation format.Isolation) format.ExecutorProfile {
	return format.ExecutorProfile{
		Channel: format.ChannelHostClassify, Harness: format.Harness{Name: hostproto.ClassifyHarnessName, Version: "2.1.293"},
		Isolation: isolation, Model: format.ExecutorModel{Name: "claude-sonnet-5-5", Source: format.ModelSourceDeclared},
	}
}

func witnessed() hostproto.Event {
	return hostproto.Event{
		Kind: hostproto.EventClassifyWitnessed, Ticket: ticketText,
		Detail: executorrun.WitnessDetail(executorrun.Witness{Model: witnessedModel, Models: []string{witnessedModel}}),
	}
}

func TestWitnessedClassification(t *testing.T) {
	t.Run("a witnessed classification is raised to restricted with the model the log resolved [verifies SP-1909~1]", func(t *testing.T) {
		got := executorrun.Effective(classified(format.IsolationRestrictedUnverified), []hostproto.Event{witnessed()})
		if got.Isolation != format.IsolationRestricted || got.Model != (format.ExecutorModel{Name: witnessedModel, Source: format.ModelSourceResolved}) {
			t.Fatalf("profile %+v", got)
		}
		if err := got.Validate(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("an unwitnessed classification stays restricted-unverified with its declared model [verifies SP-1909~1]", func(t *testing.T) {
		got := executorrun.Effective(classified(format.IsolationRestrictedUnverified), nil)
		if got.Isolation != format.IsolationRestrictedUnverified || got.Model.Source != format.ModelSourceDeclared {
			t.Fatalf("profile %+v", got)
		}
	})

	t.Run("a marked run is not raised [verifies SP-1909~1]", func(t *testing.T) {
		for _, mark := range []hostproto.EventKind{hostproto.EventLeaked, hostproto.EventExposed, hostproto.EventResubmissionConflict} {
			got := executorrun.Effective(classified(format.IsolationRestrictedUnverified), []hostproto.Event{witnessed(), {Kind: mark, Ticket: ticketText}})
			if got.Isolation != format.IsolationRestrictedUnverified {
				t.Errorf("%s: profile %+v", mark, got)
			}
		}
	})

	t.Run("a witness without a model, or for another channel, raises nothing [verifies SP-1909~1]", func(t *testing.T) {
		empty := hostproto.Event{Kind: hostproto.EventClassifyWitnessed, Ticket: ticketText, Detail: "{}"}
		if got := executorrun.Effective(classified(format.IsolationRestrictedUnverified), []hostproto.Event{empty}); got.Isolation != format.IsolationRestrictedUnverified {
			t.Errorf("a witness without a model: %+v", got)
		}
		other := stored(format.IsolationRestrictedUnverified)
		if got := executorrun.Effective(other, []hostproto.Event{witnessed()}); got.Isolation != format.IsolationRestrictedUnverified {
			t.Errorf("a witness for a subagent answer: %+v", got)
		}
	})

	t.Run("no sequence of events credits a classification with restricted-attested [verifies SP-1909~1]", func(t *testing.T) {
		kinds := []hostproto.EventKind{
			hostproto.EventAttested, hostproto.EventClassifyWitnessed, hostproto.EventRestricted, hostproto.EventAgentStarted,
			hostproto.EventRunReported, hostproto.EventLeaked, hostproto.EventAccepted,
		}
		rapid.Check(t, func(t *rapid.T) {
			chosen := rapid.SliceOf(rapid.SampledFrom(kinds)).Draw(t, "kinds")
			var events []hostproto.Event
			for _, kind := range chosen {
				event := hostproto.Event{Kind: kind, Ticket: ticketText, AgentID: agent}
				switch kind {
				case hostproto.EventAttested:
					event.Detail = executorrun.AttestationDetail(transcriptDigest)
				case hostproto.EventClassifyWitnessed:
					event.Detail = executorrun.WitnessDetail(executorrun.Witness{Model: witnessedModel})
				}
				events = append(events, event)
			}
			for _, isolation := range []format.Isolation{format.IsolationRestrictedUnverified, format.IsolationRestricted} {
				if got := executorrun.Effective(classified(isolation), events); got.Isolation == format.IsolationRestrictedAttested {
					t.Fatalf("events %v raised %s to %s", chosen, isolation, got.Isolation)
				}
			}
		})
	})
}
