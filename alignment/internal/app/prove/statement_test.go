package prove

import (
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostbridge"
)

func profile(isolation format.Isolation, channel format.Channel, agent string) format.ExecutorProfile {
	return format.ExecutorProfile{
		Channel: channel, Harness: format.Harness{Name: "harness", Version: "1"}, Isolation: isolation,
		Attestation: format.Attestation{AgentID: agent}, Model: format.ExecutorModel{Source: format.ModelSourceUnknown},
	}
}

func TestWeakest(t *testing.T) {
	t.Run("the executor of a Statement is the one with the weakest isolation, whatever the order [verifies SP-330~1]", func(t *testing.T) {
		profiles := []format.ExecutorProfile{
			profile(format.IsolationRestricted, format.ChannelHostSubagent, "a"),
			profile(format.IsolationInline, format.ChannelHostInline, "b"),
			profile(format.IsolationRestrictedAttested, format.ChannelHostSubagent, "c"),
		}
		for range len(profiles) {
			got, ok := weakest(profiles)
			if !ok || got.Isolation != format.IsolationInline {
				t.Fatalf("weakest %+v", got)
			}
			profiles = append(profiles[1:], profiles[0])
		}
	})

	t.Run("equal executors are told apart by their attestation, so that no order changes the choice [verifies SP-363~1]", func(t *testing.T) {
		first := profile(format.IsolationRestricted, format.ChannelHostSubagent, "agent-1")
		second := profile(format.IsolationRestricted, format.ChannelHostSubagent, "agent-2")
		forward, _ := weakest([]format.ExecutorProfile{first, second})
		backward, _ := weakest([]format.ExecutorProfile{second, first})
		if forward != backward {
			t.Fatalf("%+v then %+v", forward, backward)
		}
	})

	t.Run("a Statement that rests on no answer records the weakest executor the format admits [verifies SP-330~1]", func(t *testing.T) {
		executor := proofExecutor(nil, format.BasisExecution)
		if executor.Isolation != format.IsolationInline || executor.Channel != format.ChannelHostInline || !executor.Blind {
			t.Fatalf("executor %+v", executor)
		}
		if err := executor.ExecutorProfile.Validate(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestConfidenceSource(t *testing.T) {
	relayed := hostbridge.Answer{Votes: []hostbridge.VoteState{{Executor: format.ExecutorProfile{Channel: format.ChannelSystemOneRelay, Model: format.ExecutorModel{Name: "systemone:nimble@ab12", Source: format.ModelSourceDeclared}}}}}
	voted := hostbridge.Answer{Votes: []hostbridge.VoteState{{Executor: profile(format.IsolationRestricted, format.ChannelHostSubagent, "a")}}}

	t.Run("the confidence of a relayed answer comes from the relay's source, otherwise from the host votes [verifies SP-361~1]", func(t *testing.T) {
		if got := confidenceSource([]hostbridge.Answer{voted, relayed}, 3); got != (sourced{source: format.SystemOneSource("systemone:nimble@ab12"), mapping: format.MappingSystemOneChoice}) {
			t.Errorf("relayed source %+v", got)
		}
		if got := confidenceSource([]hostbridge.Answer{voted}, 3); got != (sourced{source: format.HostVotesSource(3), mapping: format.MappingModalShare}) {
			t.Errorf("host source %+v", got)
		}
	})
}

func TestModalOf(t *testing.T) {
	t.Run("the commonest choice wins and a tie goes to the smaller choice [verifies SP-361~1]", func(t *testing.T) {
		if choice, count := modalOf(map[string]int{"b": 2, "a": 2, "c": 1}); choice != "a" || count != 2 {
			t.Errorf("modal %q %d", choice, count)
		}
		if choice, count := modalOf(nil); choice != "" || count != 0 {
			t.Errorf("empty modal %q %d", choice, count)
		}
	})

	t.Run("the digest of a set does not depend on the order it was built in [verifies SP-363~1]", func(t *testing.T) {
		a, b := map[string]bool{"x": true, "y": true, "z": true}, map[string]bool{"z": true, "y": true, "x": true}
		if digestOfSet(a) != digestOfSet(b) {
			t.Fatal("two orders gave two digests")
		}
	})
}
