package executorrun_test

import (
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/executorrun"
	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
)

func copilotStored(level format.Isolation) format.ExecutorProfile {
	profile := stored(level)
	profile.Harness = format.Harness{Name: hostproto.HarnessCopilot, Version: harness.CopilotVersion}
	// No capability is issued on Copilot; the stop hook identifies the answering
	// agent by matching the submitted output with the stored response.
	profile.Attestation = format.Attestation{}
	return profile
}

func observedPair(id string) []hostproto.Event {
	return []hostproto.Event{
		{Kind: hostproto.EventAllowlistObserved, Ticket: ticketText, AgentID: id},
		{Kind: hostproto.EventLaunchBare, Ticket: ticketText, AgentID: id},
	}
}

func TestEffectiveRaisesObservedRuns(t *testing.T) {
	t.Run("an unverified Copilot answer needs both events of its answering agent and no mark [verifies SP-1872~3]", func(t *testing.T) {
		pair := observedPair(agent)
		for name, events := range map[string][]hostproto.Event{
			"both":                                  pair,
			"reverse":                               {pair[1], pair[0]},
			"uncapped acceptance remains journaled": append([]hostproto.Event{{Kind: hostproto.EventRestricted, Ticket: ticketText}}, pair...),
		} {
			got := executorrun.Effective(copilotStored(format.IsolationRestrictedUnverified), events)
			if got.Isolation != format.IsolationRestricted || got.Attestation != (format.Attestation{}) {
				t.Errorf("%s: profile = %+v", name, got)
			}
		}
		for name, events := range map[string][]hostproto.Event{
			"none":              nil,
			"allowlist alone":   {pair[0]},
			"launch alone":      {pair[1]},
			"two agents":        {pair[0], observedPair("another-agent")[1]},
			"no agent":          observedPair(""),
			"different tickets": {pair[0], {Kind: hostproto.EventLaunchBare, AgentID: agent, Ticket: ticketText + "0"}},
			"no ticket":         {{Kind: hostproto.EventAllowlistObserved, AgentID: agent}, {Kind: hostproto.EventLaunchBare, AgentID: agent}},
		} {
			if got := executorrun.Effective(copilotStored(format.IsolationRestrictedUnverified), events); got.Isolation != format.IsolationRestrictedUnverified {
				t.Errorf("%s: profile = %+v", name, got)
			}
		}
		bound := copilotStored(format.IsolationRestrictedUnverified)
		bound.Attestation.AgentID = agent
		if got := executorrun.Effective(bound, observedPair("another-agent")); got.Isolation != format.IsolationRestrictedUnverified {
			t.Fatalf("another agent raised the stored answer: %+v", got)
		}
		if got := executorrun.Effective(bound, pair); got.Isolation != format.IsolationRestricted {
			t.Fatalf("the answering agent was not credited: %+v", got)
		}
	})

	t.Run("no other harness, channel, or stored level gets the Copilot raise [verifies SP-1872~3]", func(t *testing.T) {
		pair := observedPair(agent)
		for _, name := range []string{hostproto.HarnessClaudeCode, hostproto.HarnessCodex, hostproto.HarnessAgentPlugins, ""} {
			profile := copilotStored(format.IsolationRestrictedUnverified)
			profile.Harness.Name = name
			if got := executorrun.Effective(profile, pair); got.Isolation != profile.Isolation {
				t.Errorf("%s: profile = %+v", name, got)
			}
		}
		for _, channel := range append(format.Channels(), "") {
			if channel == format.ChannelHostSubagent {
				continue
			}
			profile := copilotStored(format.IsolationRestrictedUnverified)
			profile.Channel = channel
			if got := executorrun.Effective(profile, pair); got.Isolation != profile.Isolation {
				t.Errorf("%s: profile = %+v", channel, got)
			}
		}
		for _, level := range format.Isolations() {
			if level == format.IsolationRestrictedUnverified {
				continue
			}
			if got := executorrun.Effective(copilotStored(level), pair); got.Isolation != level {
				t.Errorf("%s became %s", level, got.Isolation)
			}
		}
	})

	t.Run("Copilot never gains attestation even when the raised profile is read again [verifies SP-1872~3]", func(t *testing.T) {
		profile := copilotStored(format.IsolationRestrictedUnverified)
		profile.Attestation.AgentID = agent
		events := append(observedPair(agent), attested(agent, transcriptDigest))
		for range 2 {
			profile = executorrun.Effective(profile, events)
			if profile.Isolation != format.IsolationRestricted || profile.Attestation.TranscriptDigest != "" {
				t.Fatalf("profile = %+v", profile)
			}
		}
	})

	t.Run("main-agent and outside-executor exposure marks cannot be bypassed [verifies SP-1872~3]", func(t *testing.T) {
		for _, channel := range []format.Channel{format.ChannelHostSubagent, format.ChannelHostInline} {
			profile := copilotStored(format.IsolationRestrictedUnverified)
			profile.Channel = channel
			events := append(observedPair(agent), hostproto.Event{Kind: hostproto.EventExposed, Ticket: ticketText})
			if got := executorrun.Effective(profile, events); got.Isolation != profile.Isolation || !executorrun.Marked(events) {
				t.Fatalf("%s: profile = %+v, events = %+v", channel, got, events)
			}
		}
	})

	t.Run("ticketless HEAD blindness marks still lower an observed response to inline [verifies SP-1872~3]", func(t *testing.T) {
		for _, kind := range []hostproto.EventKind{hostproto.EventHistoryInherited, hostproto.EventBlindnessUnproven} {
			profile := copilotStored(format.IsolationRestrictedUnverified)
			profile.Attestation.AgentID = agent
			events := append(observedPair(agent), hostproto.Event{Kind: kind, AgentID: agent})
			records := make([]hostproto.Record, len(events))
			for i, event := range events {
				records[i].Event = event
			}
			indexed := executorrun.IndexJournal(records).Response(ticketText, profile)
			if got := executorrun.Effective(profile, indexed); got.Isolation != format.IsolationInline || !executorrun.Marked(indexed) {
				t.Fatalf("%s: profile = %+v, events = %+v", kind, got, indexed)
			}
		}
	})
}

var observationKinds = []hostproto.EventKind{
	hostproto.EventAllowlistObserved, hostproto.EventLaunchBare, hostproto.EventAttested,
	hostproto.EventRestricted, hostproto.EventAccepted, hostproto.EventRunReported,
}

var observationMarks = []hostproto.EventKind{
	hostproto.EventExposed, hostproto.EventLeaked, hostproto.EventResubmissionConflict,
	hostproto.EventHistoryInherited, hostproto.EventBlindnessUnproven,
}

func observationEvents(t *rapid.T, kinds []hostproto.EventKind) []hostproto.Event {
	return rapid.SliceOfN(rapid.Custom(func(t *rapid.T) hostproto.Event {
		return hostproto.Event{
			Kind: rapid.SampledFrom(kinds).Draw(t, "kind"), Ticket: ticketText,
			AgentID: rapid.SampledFrom([]string{"", agent, "another-agent"}).Draw(t, "agent"),
			Detail:  executorrun.AttestationDetail(transcriptDigest),
		}
	}), 0, 12).Draw(t, "events")
}

func TestEffectiveObservedProperties(t *testing.T) {
	t.Run("adding a mark never raises an answer [verifies SP-1872~3]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			events := observationEvents(t, observationKinds)
			kind := rapid.SampledFrom(observationMarks).Draw(t, "mark")
			events = append(events, hostproto.Event{Kind: kind, Ticket: ticketText})
			events = rapid.Permutation(events).Draw(t, "permutation")
			profile := copilotStored(rapid.SampledFrom(format.Isolations()).Draw(t, "level"))
			got := executorrun.Effective(profile, events)
			want := profile.Isolation
			if kind == hostproto.EventHistoryInherited || kind == hostproto.EventBlindnessUnproven {
				want = format.IsolationInline
			}
			if got.Isolation != want || !executorrun.Marked(events) {
				t.Fatalf("marked profile went from %s to %s with %+v", profile.Isolation, got.Isolation, events)
			}
		})
	})
	t.Run("no permutation raises without both events of one answering agent [verifies SP-1872~3]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			events := observationEvents(t, observationKinds)
			profile := copilotStored(format.IsolationRestrictedUnverified)
			profile.Attestation.AgentID = rapid.SampledFrom([]string{"", agent, "another-agent"}).Draw(t, "answering agent")
			want := format.IsolationRestrictedUnverified
			for _, first := range events {
				for _, second := range events {
					if first.Kind == hostproto.EventAllowlistObserved && second.Kind == hostproto.EventLaunchBare && first.AgentID != "" && first.AgentID == second.AgentID &&
						(profile.Attestation.AgentID == "" || profile.Attestation.AgentID == first.AgentID) {
						want = format.IsolationRestricted
					}
				}
			}
			shuffled := rapid.Permutation(events).Draw(t, "permutation")
			if got := executorrun.Effective(profile, shuffled); got.Isolation != want {
				t.Fatalf("want %s, got %+v for %+v", want, got, shuffled)
			}
		})
	})
	t.Run("an observed answer is never raised above restricted [verifies SP-1872~3]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			events := observationEvents(t, append(slices.Clone(observationKinds), observationMarks...))
			profile := copilotStored(format.IsolationRestrictedUnverified)
			profile.Attestation.AgentID = agent
			got := executorrun.Effective(profile, events)
			again := executorrun.Effective(got, events)
			if got.Isolation == format.IsolationRestrictedAttested || again.Isolation != got.Isolation || got.Attestation.TranscriptDigest != "" {
				t.Fatalf("profiles %+v then %+v for %+v", got, again, events)
			}
		})
	})
}
