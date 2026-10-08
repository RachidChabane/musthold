package executorrun_test

import (
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/executorrun"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	observedAgent = "agent-0001"
	otherAgent    = "agent-0002"
)

func allowlistEvent(agentID string) hostproto.Event {
	return hostproto.Event{Kind: hostproto.EventAllowlistObserved, Ticket: ticketText, AgentID: agentID}
}

func launchEvent(agentID string) hostproto.Event {
	return hostproto.Event{Kind: hostproto.EventLaunchBare, Ticket: ticketText, AgentID: agentID}
}

func markEvent(kind hostproto.EventKind) hostproto.Event {
	return hostproto.Event{Kind: kind, Ticket: ticketText, AgentID: observedAgent}
}

func storedAs(isolation format.Isolation) format.ExecutorProfile {
	profile := hostprototest.Profile()
	profile.Isolation = isolation
	return profile
}

func TestEffectiveRaisesObservedRuns(t *testing.T) {
	t.Run("an unverified answer is raised to restricted only with both events of one agent and no mark [verifies SP-1872~1]", func(t *testing.T) {
		both := []hostproto.Event{allowlistEvent(observedAgent), launchEvent(observedAgent)}
		if got := executorrun.Effective(storedAs(format.IsolationRestrictedUnverified), both).Isolation; got != format.IsolationRestricted {
			t.Errorf("isolation = %s", got)
		}
		for name, events := range map[string][]hostproto.Event{
			"no events":              nil,
			"the allowlist alone":    {allowlistEvent(observedAgent)},
			"the launch alone":       {launchEvent(observedAgent)},
			"two agents":             {allowlistEvent(observedAgent), launchEvent(otherAgent)},
			"no agent":               {allowlistEvent(""), launchEvent("")},
			"two agents in reverse":  {launchEvent(otherAgent), allowlistEvent(observedAgent)},
			"a leak":                 append(slices.Clone(both), markEvent(hostproto.EventLeaked)),
			"an exposure":            append(slices.Clone(both), markEvent(hostproto.EventExposed)),
			"a resubmission":         append(slices.Clone(both), markEvent(hostproto.EventResubmissionConflict)),
			"a mark before the pair": append([]hostproto.Event{markEvent(hostproto.EventExposed)}, both...),
		} {
			if got := executorrun.Effective(storedAs(format.IsolationRestrictedUnverified), events).Isolation; got != format.IsolationRestrictedUnverified {
				t.Errorf("%s: isolation = %s", name, got)
			}
		}
	})

	t.Run("an observed run is never credited as attested [verifies SP-1872~1]", func(t *testing.T) {
		stored := storedAs(format.IsolationRestrictedUnverified)
		stored.Attestation.AgentID = observedAgent
		events := []hostproto.Event{
			allowlistEvent(observedAgent), launchEvent(observedAgent),
			attested(observedAgent, hostprototest.Digest("transcript")),
		}
		if got := executorrun.Effective(stored, events).Isolation; got != format.IsolationRestricted {
			t.Errorf("isolation = %s", got)
		}
	})

	t.Run("an answer of another level is left as stored [verifies SP-1872~1]", func(t *testing.T) {
		events := []hostproto.Event{allowlistEvent(observedAgent), launchEvent(observedAgent)}
		for _, level := range []format.Isolation{format.IsolationUnrestricted, format.IsolationInline, format.IsolationRestricted} {
			if got := executorrun.Effective(storedAs(level), events).Isolation; got != level {
				t.Errorf("%s became %s", level, got)
			}
		}
	})
}

var (
	observedKinds = []hostproto.EventKind{
		hostproto.EventAllowlistObserved, hostproto.EventLaunchBare, hostproto.EventAttested, hostproto.EventRunReported,
		hostproto.EventRestricted, hostproto.EventAccepted, hostproto.EventFetched,
	}
	markKinds = []hostproto.EventKind{hostproto.EventLeaked, hostproto.EventExposed, hostproto.EventResubmissionConflict}
	agentIDs  = []string{"", observedAgent, otherAgent}
)

func drawEvents(t *rapid.T, kinds []hostproto.EventKind) []hostproto.Event {
	return rapid.SliceOfN(rapid.Custom(func(t *rapid.T) hostproto.Event {
		return hostproto.Event{
			Kind: rapid.SampledFrom(kinds).Draw(t, "kind"), Ticket: ticketText, AgentID: rapid.SampledFrom(agentIDs).Draw(t, "agent"),
		}
	}), 0, 8).Draw(t, "events")
}

func TestEffectiveProperties(t *testing.T) {
	t.Run("adding a mark never raises an answer [verifies SP-1872~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			events := drawEvents(t, observedKinds)
			mark := hostproto.Event{Kind: rapid.SampledFrom(markKinds).Draw(t, "mark"), Ticket: ticketText}
			stored := storedAs(rapid.SampledFrom([]format.Isolation{format.IsolationRestrictedUnverified, format.IsolationRestricted}).Draw(t, "level"))
			marked := executorrun.Effective(stored, append(slices.Clone(events), mark))
			if marked.Isolation != stored.Isolation {
				t.Fatalf("a marked run went from %s to %s", stored.Isolation, marked.Isolation)
			}
		})
	})

	t.Run("no permutation of events raises an answer without both events of one agent [verifies SP-1872~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			events := drawEvents(t, observedKinds)
			shuffled := rapid.Permutation(events).Draw(t, "permutation")
			stored := storedAs(format.IsolationRestrictedUnverified)
			if hasObservedPair(events) {
				if got := executorrun.Effective(stored, shuffled).Isolation; got != raisedBy(events) {
					t.Fatalf("a permutation of %+v gives %s", events, got)
				}
				return
			}
			if got := executorrun.Effective(stored, shuffled).Isolation; got != format.IsolationRestrictedUnverified {
				t.Fatalf("%+v raised an answer to %s", shuffled, got)
			}
		})
	})

	t.Run("an answer is never raised above restricted by observed events [verifies SP-1872~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			events := drawEvents(t, append(slices.Clone(observedKinds), markKinds...))
			got := executorrun.Effective(storedAs(format.IsolationRestrictedUnverified), events).Isolation
			if got == format.IsolationRestrictedAttested {
				t.Fatalf("%+v credited the answer as attested", events)
			}
		})
	})
}

// hasObservedPair reports whether the events hold an allowlist and a launch of one agent.
func hasObservedPair(events []hostproto.Event) bool {
	for _, first := range events {
		for _, second := range events {
			if first.Kind == hostproto.EventAllowlistObserved && second.Kind == hostproto.EventLaunchBare && first.AgentID != "" && first.AgentID == second.AgentID {
				return true
			}
		}
	}
	return false
}

// raisedBy is the level the events give an unverified answer: restricted unless a
// mark among them stops the raise.
func raisedBy(events []hostproto.Event) format.Isolation {
	if executorrun.Marked(events) {
		return format.IsolationRestrictedUnverified
	}
	return format.IsolationRestricted
}
