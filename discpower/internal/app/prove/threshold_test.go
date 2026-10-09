package prove

import (
	"encoding/json"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/execution"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostbridge"
	"github.com/RachidChabane/spec-proof/format/hostproto/requests"
)

func TestRecordedThreshold(t *testing.T) {
	const (
		defaultTau   = 0.70
		hostVotesTau = 0.90
		exactTau     = 0.95
		judgeK       = 2
	)
	fixture := newFixture()
	fixture.run.rules.JudgeK = judgeK
	fixture.run.rules.TauJudge = format.Threshold{Default: defaultTau, BySource: map[format.ConfidenceSource]float64{
		format.SourceHostVotes: hostVotesTau,
	}}
	mutant := &poolMutant{spec: execution.MutantSpec{ID: format.SHA256Hex([]byte("mutant"))}, origin: power.OriginTargeted}

	recorded := func(t *testing.T, decision power.Decision) float64 {
		t.Helper()
		entry, err := fixture.run.mutantEntry(mutant, decision)
		if err != nil {
			t.Fatal(err)
		}
		return entry.Extensions.Decision.Threshold
	}

	t.Run("a decision of the host judges records the threshold of host votes [verifies SP-902~1]", func(t *testing.T) {
		decision := power.Undetermined(power.ReasonJudgesSplit)
		decision.Source = format.HostVotesSource(judgeK)
		if got := recorded(t, decision); got != hostVotesTau {
			t.Fatalf("threshold = %v, want %v", got, hostVotesTau)
		}
	})

	t.Run("a decision the module fixed itself records the default [verifies SP-902~1]", func(t *testing.T) {
		if got := recorded(t, power.Undetermined(power.ReasonJudgesSplit)); got != defaultTau {
			t.Fatalf("threshold = %v, want %v", got, defaultTau)
		}
	})

	t.Run("a relayed answer is recorded under exactly system-one:<model>@<digest> and held to the threshold keyed on it [verifies SP-902~1]", func(t *testing.T) {
		const imported = "system-one:nimble:9b@0123abcd"
		const relayTau = 0.55
		source, err := format.ParseSystemOneSource(imported)
		if err != nil {
			t.Fatal(err)
		}
		profile := requests.RelayProfile(source)
		answer := hostbridge.Answer{Votes: []hostbridge.VoteState{{
			Output:   json.RawMessage(`{"answers":[{"id":"q1","choice":"VIOLATES","probabilities":{"VIOLATES":0.8,"CONSISTENT":0.2}}]}`),
			Executor: profile,
		}}}
		_, relayed := closedAnswers(answer, "q1", format.UniformThreshold(0.2))
		fixture.run.rules.TauJudge.BySource[source] = relayTau
		decision := power.Undetermined(power.ReasonJudgesSplit)
		decision.Source = relayed
		entry, err := fixture.run.mutantEntry(mutant, decision)
		if err != nil {
			t.Fatal(err)
		}
		if got := entry.Extensions.Decision; got.ConfidenceSource != imported || got.Threshold != relayTau {
			t.Fatalf("decision source %q threshold %v, want %q and %v", got.ConfidenceSource, got.Threshold, imported, relayTau)
		}
	})

	t.Run("the threshold of an exact source beats the one of its kind [verifies SP-902~1]", func(t *testing.T) {
		fixture.run.rules.TauJudge.BySource[format.HostVotesSource(judgeK)] = exactTau
		decision := power.Undetermined(power.ReasonJudgesSplit)
		decision.Source = format.HostVotesSource(judgeK)
		if got := recorded(t, decision); got != exactTau {
			t.Fatalf("threshold = %v, want %v", got, exactTau)
		}
	})
}
