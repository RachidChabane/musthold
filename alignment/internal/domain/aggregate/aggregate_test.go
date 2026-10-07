package aggregate_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/alignment/internal/domain/aggregate"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/masking"
)

const (
	declared = "PAR-1~1"
	another  = "PAR-2~1"
	modalMin = 0.8
)

func vote(verifies string, coverage aggregate.Coverage, confidence float64) aggregate.Vote {
	return aggregate.Vote{Verifies: verifies, Coverage: coverage, Confidence: confidence, Credit: true}
}

func TestAggregate(t *testing.T) {
	t.Run("the modal criterion has the highest count over the tickets [verifies SP-307~1]", func(t *testing.T) {
		votes := []aggregate.Vote{
			vote(declared, aggregate.CoverageFull, 0.9), vote(declared, aggregate.CoverageFull, 0.8),
			vote(declared, aggregate.CoveragePartial, 0.7), vote(declared, aggregate.CoverageFull, 0.9),
			vote(another, aggregate.CoverageFull, 0.9),
		}
		result, ok := aggregate.Aggregate(votes, 5)
		if !ok || result.ModalChoice != declared || result.ModalShare != 0.8 || result.ModalCoverage != aggregate.CoverageFull {
			t.Fatalf("result %+v", result)
		}
	})

	t.Run("a tie goes to the higher mean confidence, then to the smaller reference [verifies SP-307~1]", func(t *testing.T) {
		byConfidence, _ := aggregate.Aggregate([]aggregate.Vote{vote(another, aggregate.CoverageFull, 0.9), vote(declared, aggregate.CoverageFull, 0.5)}, 2)
		if byConfidence.ModalChoice != another {
			t.Fatalf("modal %s", byConfidence.ModalChoice)
		}
		byName, _ := aggregate.Aggregate([]aggregate.Vote{vote(another, aggregate.CoverageFull, 0.5), vote(declared, aggregate.CoverageFull, 0.5)}, 2)
		if byName.ModalChoice != declared {
			t.Fatalf("modal %s", byName.ModalChoice)
		}
	})

	t.Run("a missing ticket counts against the share [verifies SP-307~1]", func(t *testing.T) {
		result, ok := aggregate.Aggregate([]aggregate.Vote{vote(declared, aggregate.CoverageFull, 1), vote(declared, aggregate.CoverageFull, 1)}, 5)
		if !ok || result.ModalShare != 0.4 || result.Missing != 3 {
			t.Fatalf("result %+v", result)
		}
		if _, ok := aggregate.Aggregate(nil, 5); ok {
			t.Fatal("no votes aggregated")
		}
	})

	t.Run("the modal coverage is the commonest among the votes on the modal criterion [verifies SP-307~1]", func(t *testing.T) {
		result, _ := aggregate.Aggregate([]aggregate.Vote{
			vote(declared, aggregate.CoverageWeak, 1), vote(declared, aggregate.CoverageWeak, 1), vote(declared, aggregate.CoverageFull, 1),
			vote(another, aggregate.CoverageFull, 1),
		}, 4)
		if result.ModalCoverage != aggregate.CoverageWeak {
			t.Fatalf("coverage %s", result.ModalCoverage)
		}
	})
}

func TestStance(t *testing.T) {
	cases := []struct {
		name  string
		votes []aggregate.Vote
		total int
		want  aggregate.Stance
	}{
		{"the declared criterion with full coverage supports", repeat(5, vote(declared, aggregate.CoverageFull, 1)), 5, aggregate.Supports},
		{"the declared criterion with partial coverage supports", repeat(5, vote(declared, aggregate.CoveragePartial, 1)), 5, aggregate.Supports},
		{"the declared criterion with weak coverage rejects", repeat(5, vote(declared, aggregate.CoverageWeak, 1)), 5, aggregate.Rejects},
		{"another criterion rejects", repeat(5, vote(another, aggregate.CoverageFull, 1)), 5, aggregate.Rejects},
		{"NONE rejects", repeat(5, vote(masking.NoneAnswer, aggregate.CoverageNone, 1)), 5, aggregate.Rejects},
		{"a modal share below the minimum is indeterminate",
			append(repeat(3, vote(declared, aggregate.CoverageFull, 1)), repeat(2, vote(another, aggregate.CoverageFull, 1))...), 5, aggregate.Indeterminate},
		{"a share of exactly the minimum counts",
			append(repeat(4, vote(declared, aggregate.CoverageFull, 1)), vote(another, aggregate.CoverageFull, 1)), 5, aggregate.Supports},
	}
	for _, c := range cases {
		t.Run(c.name+" [verifies SP-307~1]", func(t *testing.T) {
			result, _ := aggregate.Aggregate(c.votes, c.total)
			if got := result.StanceFor(declared, modalMin); got != c.want {
				t.Fatalf("stance %s, want %s (%+v)", got, c.want, result)
			}
		})
	}
}

func repeat(count int, v aggregate.Vote) []aggregate.Vote {
	votes := make([]aggregate.Vote, count)
	for index := range votes {
		votes[index] = v
	}
	return votes
}

func TestCredit(t *testing.T) {
	t.Run("a vote without credit never supports the declared criterion [verifies SP-308~1]", func(t *testing.T) {
		creditless := aggregate.Vote{Verifies: declared, Coverage: aggregate.CoverageFull, Confidence: 1}
		if demoted := creditless.Demote(declared); demoted.Verifies != masking.NoneAnswer || demoted.Coverage != aggregate.CoverageNone {
			t.Fatalf("demoted %+v", demoted)
		}
		if kept := creditless.Demote(another); kept != creditless {
			t.Fatalf("a vote for another criterion changed: %+v", kept)
		}
		against := aggregate.Vote{Verifies: another, Coverage: aggregate.CoverageFull}
		if against.Demote(declared) != against {
			t.Fatal("a vote against the declared criterion lost its power to speak against")
		}
	})

	t.Run("the demotion of unsupported votes can only lower the stance [verifies SP-308~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			count := rapid.IntRange(1, 7).Draw(t, "count")
			votes := make([]aggregate.Vote, count)
			for index := range votes {
				choice := rapid.SampledFrom([]string{declared, another, masking.NoneAnswer}).Draw(t, "choice")
				coverage := rapid.SampledFrom(aggregate.Levels()).Draw(t, "coverage")
				votes[index] = aggregate.Vote{Verifies: choice, Coverage: coverage, Confidence: 0.9, Credit: rapid.Bool().Draw(t, "credit")}
			}
			demoted := make([]aggregate.Vote, count)
			for index, v := range votes {
				demoted[index] = v.Demote(declared)
			}
			full, _ := aggregate.Aggregate(votes, count)
			after, _ := aggregate.Aggregate(demoted, count)
			if after.StanceFor(declared, modalMin) == aggregate.Supports && full.StanceFor(declared, modalMin) != aggregate.Supports {
				t.Fatalf("demotion created support: %+v -> %+v", full, after)
			}
		})
	})
}

func TestSingleStance(t *testing.T) {
	t.Run("the tie-break vote supports only with credit, the declared criterion and a supporting coverage [verifies SP-313~1]", func(t *testing.T) {
		if aggregate.SingleStance(vote(declared, aggregate.CoverageFull, 1), declared) != aggregate.Supports {
			t.Error("a full vote for the declared criterion did not support")
		}
		if aggregate.SingleStance(vote(declared, aggregate.CoverageWeak, 1), declared) != aggregate.Rejects {
			t.Error("a weak vote supported")
		}
		creditless := aggregate.Vote{Verifies: declared, Coverage: aggregate.CoverageFull, Confidence: 1}
		if aggregate.SingleStance(creditless, declared) != aggregate.Rejects {
			t.Error("a vote without credit supported")
		}
	})
}
