package promise_test

import (
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/discpower/internal/domain/promise"
	"github.com/RachidChabane/musthold/discpower/internal/domain/witness"
	"github.com/RachidChabane/musthold/format"
)

const (
	verifiesBlockBearing = " [verifies SP-2191~1]"
	verifiesBroken       = " [verifies SP-2192~1]"

	scopeThreshold = 0.8
	sure           = 0.95
)

func fixed(confidence float64) witness.ScopeVote {
	return witness.ScopeVote{Answer: witness.ScopeFixed, Confidence: confidence}
}

func blockBearingWitness(id, promiseID string) promise.WitnessStanding {
	return promise.WitnessStanding{ID: id, Promise: promiseID, Admitted: true, Stable: true, Votes: []witness.ScopeVote{fixed(sure), fixed(sure)}}
}

func evidence(failing []string, suite ...witness.Outcome) promise.MutantEvidence {
	outcomes := map[string]witness.Outcome{}
	for _, id := range failing {
		outcomes[id] = fail
	}
	return promise.MutantEvidence{ID: "m1", Witness: outcomes, Suite: suite}
}

func TestBlockBearing(t *testing.T) {
	cases := map[string]struct {
		change func(*promise.WitnessStanding)
		want   bool
	}{
		"admitted, stable and fixed twice": {func(*promise.WitnessStanding) {}, true},
		"not admitted":                     {func(w *promise.WitnessStanding) { w.Admitted = false }, false},
		"unstable on the reference":        {func(w *promise.WitnessStanding) { w.Stable = false }, false},
		"one vote only":                    {func(w *promise.WitnessStanding) { w.Votes = w.Votes[:1] }, false},
		"one vote not fixed": {func(w *promise.WitnessStanding) {
			w.Votes[1] = witness.ScopeVote{Answer: witness.ScopeNotFixed, Confidence: sure}
		}, false},
		"one vote below the threshold": {func(w *promise.WitnessStanding) { w.Votes[0] = fixed(scopeThreshold / 2) }, false},
	}
	for name, c := range cases {
		t.Run(name+verifiesBlockBearing, func(t *testing.T) {
			standing := blockBearingWitness("w1", "p1")
			c.change(&standing)
			if got := standing.BlockBearing(scopeThreshold); got != c.want {
				t.Fatalf("block-bearing %v, want %v", got, c.want)
			}
		})
	}
}

func TestBrokenPromises(t *testing.T) {
	witnesses := []promise.WitnessStanding{blockBearingWitness("w1", "p1")}
	allPass := runs(pass, pass, pass)
	t.Run("a block-bearing witness fails the mutant and the suite passes it three times"+verifiesBroken, func(t *testing.T) {
		got := promise.BrokenPromises(evidence([]string{"w1"}, allPass...), witnesses, scopeThreshold)
		want := []format.ScanBrokenPromise{{Promise: "p1", Mutant: "m1", Witness: "w1"}}
		if !slices.Equal(got, want) {
			t.Fatalf("broken %v", got)
		}
	})
	denied := map[string]promise.MutantEvidence{
		"a sentinel": func() promise.MutantEvidence {
			e := evidence([]string{"w1"}, allPass...)
			e.Sentinel = true
			return e
		}(),
		"the suite fails once":            evidence([]string{"w1"}, pass, fail, pass),
		"the suite ran twice":             evidence([]string{"w1"}, pass, pass),
		"the suite errored":               evidence([]string{"w1"}, pass, oops, pass),
		"the witness did not reproduce":   {ID: "m1", Witness: map[string]witness.Outcome{"w1": flaky}, Suite: allPass},
		"the witness passes the mutant":   {ID: "m1", Witness: map[string]witness.Outcome{"w1": pass}, Suite: allPass},
		"no witness ran on the mutant":    {ID: "m1", Suite: allPass},
		"another witness failed, unfixed": evidence([]string{"w2"}, allPass...),
	}
	for name, mutant := range denied {
		t.Run("no finding when "+name+verifiesBroken, func(t *testing.T) {
			standings := slices.Concat(witnesses, []promise.WitnessStanding{{ID: "w2", Promise: "p2", Admitted: true, Stable: true}})
			if got := promise.BrokenPromises(mutant, standings, scopeThreshold); len(got) != 0 {
				t.Fatalf("broken %v", got)
			}
		})
	}
	t.Run("a witness that is not block-bearing never makes a finding"+verifiesBroken, func(t *testing.T) {
		weak := blockBearingWitness("w1", "p1")
		weak.Votes[0] = witness.ScopeVote{Answer: witness.ScopeUndetermined, Confidence: sure}
		if got := promise.BrokenPromises(evidence([]string{"w1"}, allPass...), []promise.WitnessStanding{weak}, scopeThreshold); len(got) != 0 {
			t.Fatalf("broken %v", got)
		}
	})
	t.Run("a promise appears once with its first failing witness by id, in promise order"+verifiesBroken, func(t *testing.T) {
		standings := []promise.WitnessStanding{
			blockBearingWitness("w4", "p2"), blockBearingWitness("w3", "p1"), blockBearingWitness("w2", "p1"), blockBearingWitness("w5", "p2"),
		}
		got := promise.BrokenPromises(evidence([]string{"w2", "w3", "w5"}, allPass...), standings, scopeThreshold)
		want := []format.ScanBrokenPromise{{Promise: "p1", Mutant: "m1", Witness: "w2"}, {Promise: "p2", Mutant: "m1", Witness: "w5"}}
		if !slices.Equal(got, want) {
			t.Fatalf("broken %v", got)
		}
	})
}

func drawStanding(t *rapid.T, id string) promise.WitnessStanding {
	answers := []witness.ScopeAnswer{witness.ScopeFixed, witness.ScopeFixed, witness.ScopeNotFixed, witness.ScopeUndetermined}
	vote := rapid.Custom(func(t *rapid.T) witness.ScopeVote {
		return witness.ScopeVote{Answer: rapid.SampledFrom(answers).Draw(t, "answer"), Confidence: rapid.Float64Range(0, 1).Draw(t, "confidence")}
	})
	return promise.WitnessStanding{
		ID:       id,
		Promise:  rapid.SampledFrom([]string{"p1", "p2", "p3"}).Draw(t, "promise"),
		Admitted: rapid.Bool().Draw(t, "admitted"),
		Stable:   rapid.Bool().Draw(t, "stable"),
		Votes:    rapid.SliceOfN(vote, 0, 3).Draw(t, "votes"),
	}
}

func drawEvidence(t *rapid.T, standings []promise.WitnessStanding) promise.MutantEvidence {
	outcome := rapid.SampledFrom([]witness.Outcome{pass, fail, fail, flaky, oops})
	evidence := promise.MutantEvidence{
		ID:       "m1",
		Sentinel: rapid.Bool().Draw(t, "sentinel"),
		Witness:  map[string]witness.Outcome{},
		Suite:    rapid.SliceOfN(rapid.SampledFrom([]witness.Outcome{pass, pass, pass, pass, fail}), 0, 4).Draw(t, "suite"),
	}
	for _, standing := range standings {
		evidence.Witness[standing.ID] = outcome.Draw(t, "witness outcome")
	}
	return evidence
}

func TestBrokenPromisesProperties(t *testing.T) {
	ids := []string{"w1", "w2", "w3", "w4", "w5", "w6"}
	draw := func(t *rapid.T) ([]promise.WitnessStanding, promise.MutantEvidence) {
		count := rapid.IntRange(0, len(ids)).Draw(t, "witnesses")
		standings := make([]promise.WitnessStanding, count)
		for index := range standings {
			standings[index] = drawStanding(t, ids[index])
		}
		return standings, drawEvidence(t, standings)
	}
	t.Run("a broken promise always has a block-bearing witness that failed and a suite that passed three times out of three"+verifiesBroken, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			standings, mutant := draw(t)
			for _, broken := range promise.BrokenPromises(mutant, standings, scopeThreshold) {
				index := slices.IndexFunc(standings, func(s promise.WitnessStanding) bool { return s.ID == broken.Witness })
				if index < 0 || standings[index].Promise != broken.Promise || !standings[index].BlockBearing(scopeThreshold) {
					t.Fatalf("%+v rests on a witness that is not block-bearing for its promise", broken)
				}
				if mutant.Witness[broken.Witness] != fail || mutant.Sentinel || broken.Mutant != mutant.ID {
					t.Fatalf("%+v rests on %+v", broken, mutant)
				}
				if suitePasses := !slices.ContainsFunc(mutant.Suite, func(o witness.Outcome) bool { return o != pass }); !suitePasses || len(mutant.Suite) < format.RunsPerScanOutcome {
					t.Fatalf("%+v rests on a suite of %v", broken, mutant.Suite)
				}
			}
		})
	})
	t.Run("every promise with such a witness is reported, once"+verifiesBroken, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			standings, mutant := draw(t)
			got := promise.BrokenPromises(mutant, standings, scopeThreshold)
			reported := map[string]int{}
			for _, broken := range got {
				reported[broken.Promise]++
			}
			suiteHolds := len(mutant.Suite) >= format.RunsPerScanOutcome && !slices.Contains(mutant.Suite, fail)
			for _, standing := range standings {
				mustReport := suiteHolds && !mutant.Sentinel && mutant.Witness[standing.ID] == fail && standing.BlockBearing(scopeThreshold)
				if mustReport && reported[standing.Promise] != 1 || reported[standing.Promise] > 1 {
					t.Fatalf("promise %s reported %d times in %v", standing.Promise, reported[standing.Promise], got)
				}
			}
		})
	})
	t.Run("the findings do not depend on the order of the witnesses"+verifiesBroken, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			standings, mutant := draw(t)
			shuffled := rapid.Permutation(standings).Draw(t, "order")
			if a, b := promise.BrokenPromises(mutant, standings, scopeThreshold), promise.BrokenPromises(mutant, shuffled, scopeThreshold); !slices.Equal(a, b) {
				t.Fatalf("%v differs from %v", a, b)
			}
		})
	})
}
