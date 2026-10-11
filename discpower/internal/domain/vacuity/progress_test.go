package vacuity_test

import (
	"fmt"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
)

const verifiesProgress = " [verifies SP-2138~1]"

// progressFixture is a scan of one sentinel and two mutants on the lines of
// the vacuous test and the killer test, each owning line 41.
func progressFixture() (*fixture, string, string, string) {
	f := newFixture()
	at := line(parseFile, 41)
	f.sampled(vacuousTest, at).sampled(killerTest, at)
	sentinel := f.mutant(vacuity.Mutant{ID: mutantID("sentinel"), Lines: []vacuity.Line{at}, Sentinel: true})
	first := f.mutant(vacuity.Mutant{ID: mutantID("first"), Lines: []vacuity.Line{at}})
	second := f.mutant(vacuity.Mutant{ID: mutantID("second"), Lines: []vacuity.Line{at}})
	return f, sentinel, first, second
}

// mutantOf finds a mutant of the input by id.
func mutantOf(input vacuity.Input, id string) vacuity.Mutant {
	for _, mutant := range input.Mutants {
		if mutant.ID == id {
			return mutant
		}
	}
	return vacuity.Mutant{}
}

func outcomes(pairs ...any) map[ecosystem.TestID][]vacuity.Run {
	byTest := map[ecosystem.TestID][]vacuity.Run{}
	for index := 0; index < len(pairs); index += 2 {
		byTest[pairs[index].(ecosystem.TestID)] = pairs[index+1].([]vacuity.Run)
	}
	return byTest
}

func TestProgress(t *testing.T) {
	t.Run("a test that kills a mutant is settled only once every sentinel has finished"+verifiesProgress, func(t *testing.T) {
		f, sentinel, first, _ := progressFixture()
		progress := vacuity.NewProgress(f.input)
		progress.Record(mutantOf(f.input, first), outcomes(killerTest, runs(vacuity.RunFailed), vacuousTest, runs(vacuity.RunPassed)))
		if progress.Settled(killerTest) {
			t.Fatal("settled before its sentinel finished")
		}
		progress.Record(mutantOf(f.input, sentinel), outcomes(killerTest, runs(vacuity.RunPassed), vacuousTest, runs(vacuity.RunPassed)))
		if !progress.Settled(killerTest) || progress.Settled(vacuousTest) {
			t.Fatalf("killer settled %v, vacuous settled %v", progress.Settled(killerTest), progress.Settled(vacuousTest))
		}
	})
	t.Run("a test that fails a run on a sentinel is settled at once"+verifiesProgress, func(t *testing.T) {
		f, sentinel, _, _ := progressFixture()
		progress := vacuity.NewProgress(f.input)
		progress.Record(mutantOf(f.input, sentinel), outcomes(vacuousTest, []vacuity.Run{vacuity.RunPassed, vacuity.RunFailed, vacuity.RunPassed}))
		if !progress.Settled(vacuousTest) || progress.Settled(killerTest) {
			t.Fatalf("vacuous settled %v, killer settled %v", progress.Settled(vacuousTest), progress.Settled(killerTest))
		}
	})
	t.Run("a kill of a mutant identical when compiled, or on lines the test did not run, settles nothing"+verifiesProgress, func(t *testing.T) {
		f, sentinel, first, second := progressFixture()
		progress := vacuity.NewProgress(f.input)
		progress.Record(mutantOf(f.input, sentinel), nil)
		identical := mutantOf(f.input, first)
		identical.CompiledIdentical = true
		progress.Record(identical, outcomes(killerTest, runs(vacuity.RunFailed)))
		elsewhere := mutantOf(f.input, second)
		elsewhere.Lines = []vacuity.Line{line(totalFile, 3)}
		progress.Record(elsewhere, outcomes(killerTest, runs(vacuity.RunFailed)))
		if progress.Settled(killerTest) {
			t.Fatal("settled on a kill that proves nothing")
		}
	})
	t.Run("an inconclusive outcome or a run that did not start settles nothing"+verifiesProgress, func(t *testing.T) {
		f, sentinel, first, second := progressFixture()
		progress := vacuity.NewProgress(f.input)
		progress.Record(mutantOf(f.input, sentinel), outcomes(killerTest, runs(vacuity.RunPassed)))
		progress.Record(mutantOf(f.input, first), outcomes(killerTest, []vacuity.Run{vacuity.RunFailed, vacuity.RunFailed, vacuity.RunTimedOut}))
		progress.Record(mutantOf(f.input, second), outcomes(killerTest, runs(vacuity.RunNotStarted)))
		if progress.Settled(killerTest) {
			t.Fatal("settled without a kill")
		}
	})
	t.Run("survivals, however many, leave a test open"+verifiesProgress, func(t *testing.T) {
		f := newFixture()
		f.fiveSurvivedOnTwoSites(vacuousTest)
		f.input.Sampled = append(f.input.Sampled, vacuousTest)
		f.input.Reference[vacuousTest] = runs(vacuity.RunPassed)
		progress := vacuity.NewProgress(f.input)
		for _, mutant := range f.input.Mutants {
			progress.Record(mutant, f.input.Outcomes[mutant.ID])
		}
		if progress.Settled(vacuousTest) || !progress.Open([]vacuity.Line{line(parseFile, 44)}) {
			t.Fatal("a test that only survived is settled")
		}
	})
	t.Run("a sampled test that is not green has no verdict to wait for"+verifiesProgress, func(t *testing.T) {
		f, _, _, _ := progressFixture()
		f.input.Reference[vacuousTest] = []vacuity.Run{vacuity.RunPassed, vacuity.RunFailed, vacuity.RunPassed}
		progress := vacuity.NewProgress(f.input)
		if !progress.Settled(vacuousTest) || progress.Settled(killerTest) {
			t.Fatalf("red settled %v, green settled %v", progress.Settled(vacuousTest), progress.Settled(killerTest))
		}
	})
	t.Run("lines no open sampled test ran are not open"+verifiesProgress, func(t *testing.T) {
		f, sentinel, first, _ := progressFixture()
		f.input.Reference[otherTest] = runs(vacuity.RunPassed)
		f.owns(otherTest, line(totalFile, 3))
		progress := vacuity.NewProgress(f.input)
		progress.Record(mutantOf(f.input, sentinel), nil)
		progress.Record(mutantOf(f.input, first), outcomes(killerTest, runs(vacuity.RunFailed), vacuousTest, runs(vacuity.RunFailed)))
		if progress.Open([]vacuity.Line{line(parseFile, 41)}) || progress.Open([]vacuity.Line{line(totalFile, 3)}) {
			t.Fatal("open lines with every sampled test that ran them settled")
		}
	})
}

// Shares, in percent, of the scans the progress properties draw: sentinels,
// kills and sentinel failures are frequent, so that settling and every way to
// undo a wrong settling are drawn often.
const (
	progressSentinelShare     = 30
	progressIdenticalShare    = 20
	progressSentinelFailShare = 25
	progressKilledShare       = 35
	progressSurvivedShare     = 45
	progressRecordedShare     = 60
	maxProgressDifferences    = 1
)

// progressRuns are the runs of a test on a mutant: a sentinel mostly passes
// and often fails once, another mutant is often killed or survived.
func progressRuns(t *rapid.T, sentinel bool) []vacuity.Run {
	draw := rapid.IntRange(0, percent-1).Draw(t, "progressOutcome")
	switch {
	case sentinel && draw < progressSentinelFailShare:
		return []vacuity.Run{vacuity.RunPassed, vacuity.RunFailed, vacuity.RunPassed}
	case sentinel:
		return runs(vacuity.RunPassed)
	case draw < progressKilledShare:
		return runs(vacuity.RunFailed)
	case draw < progressKilledShare+progressSurvivedShare:
		return runs(vacuity.RunPassed)
	}
	return rapid.SliceOfN(rapid.SampledFrom(allRuns), 0, unsettledMaxRuns).Draw(t, "progressUnsettled")
}

// progressInputGenerator draws scans whose tests settle often.
func progressInputGenerator() *rapid.Generator[vacuity.Input] {
	return rapid.Custom(func(t *rapid.T) vacuity.Input {
		tests := testPool()
		input := vacuity.Input{
			Reference: map[ecosystem.TestID][]vacuity.Run{},
			Coverage:  map[ecosystem.TestID]vacuity.Coverage{},
			Outcomes:  map[string]map[ecosystem.TestID][]vacuity.Run{},
			Sampled:   eachOf(t, "sampled", tests, sampledShare),
		}
		for _, test := range tests {
			input.Reference[test] = referenceRuns(t)
			lines := eachOf(t, "coverage", linePool(), coveredShare)
			input.Coverage[test] = vacuity.Coverage{Lines: lines, Owned: eachOf(t, "owned", lines, ownedShare)}
		}
		for index := range rapid.IntRange(minMutants, maxMutants).Draw(t, "mutants") {
			mutant := vacuity.Mutant{
				ID:                generatedID(fmt.Sprintf("p%d", index)),
				Lines:             subsetOf(t, "lines", linePool(), minMutantLines, maxMutantLines),
				Sentinel:          chance(t, "sentinel", progressSentinelShare),
				CompiledIdentical: chance(t, "identical", progressIdenticalShare),
			}
			for range rapid.IntRange(0, maxProgressDifferences).Draw(t, "differences") {
				mutant.Differences = append(mutant.Differences, differs())
			}
			input.Mutants = append(input.Mutants, mutant)
			input.Outcomes[mutant.ID] = map[ecosystem.TestID][]vacuity.Run{}
			for _, test := range eachOf(t, "ranAgainst", tests, ranAgainstShare) {
				input.Outcomes[mutant.ID][test] = progressRuns(t, mutant.Sentinel)
			}
		}
		return input
	})
}

// recordedSubset draws the mutants recorded so far, in the order they
// finished.
func recordedSubset(t *rapid.T, input vacuity.Input) []vacuity.Mutant {
	return rapid.Permutation(eachOf(t, "recorded", input.Mutants, progressRecordedShare)).Draw(t, "finished")
}

// restricted keeps the given mutants of the input and their outcomes.
func restricted(input vacuity.Input, kept []vacuity.Mutant) vacuity.Input {
	clone := cloneInput(input)
	clone.Mutants = kept
	clone.Outcomes = map[string]map[ecosystem.TestID][]vacuity.Run{}
	for _, mutant := range kept {
		clone.Outcomes[mutant.ID] = input.Outcomes[mutant.ID]
	}
	return clone
}

func TestProgressSoundness(t *testing.T) {
	t.Run("a settled test gets the same class and reason whatever the outcomes of the mutants not finished yet"+verifiesProgress, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			input := progressInputGenerator().Draw(t, "input")
			complete := verdictsByTest(mustClassify(t, input))
			progress := vacuity.NewProgress(input)
			var finished []vacuity.Mutant
			for _, mutant := range rapid.Permutation(input.Mutants).Draw(t, "finished") {
				progress.Record(mutant, input.Outcomes[mutant.ID])
				finished = append(finished, mutant)
				sofar := verdictsByTest(mustClassify(t, restricted(input, finished)))
				for _, test := range input.Sampled {
					if progress.Settled(test) && (sofar[test].Class != complete[test].Class || sofar[test].Reason != complete[test].Reason) {
						t.Fatalf("%s settled as %s (%s) after %d mutants, ends %s (%s)", test, sofar[test].Class, sofar[test].Reason, len(finished), complete[test].Class, complete[test].Reason)
					}
				}
			}
		})
	})
	t.Run("the settled tests do not depend on the order the mutants finished in"+verifiesProgress, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			input := progressInputGenerator().Draw(t, "input")
			finished := recordedSubset(t, input)
			inOrder, shuffled := vacuity.NewProgress(input), vacuity.NewProgress(input)
			for _, mutant := range finished {
				inOrder.Record(mutant, input.Outcomes[mutant.ID])
			}
			for _, mutant := range rapid.Permutation(finished).Draw(t, "reordered") {
				shuffled.Record(mutant, input.Outcomes[mutant.ID])
			}
			for _, test := range testPool() {
				if inOrder.Settled(test) != shuffled.Settled(test) {
					t.Fatalf("%s settled %v in one order, %v in another", test, inOrder.Settled(test), shuffled.Settled(test))
				}
			}
		})
	})
	t.Run("a test settled once stays settled as more mutants finish"+verifiesProgress, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			input := progressInputGenerator().Draw(t, "input")
			progress := vacuity.NewProgress(input)
			settled := map[ecosystem.TestID]bool{}
			for _, mutant := range rapid.Permutation(input.Mutants).Draw(t, "finished") {
				progress.Record(mutant, input.Outcomes[mutant.ID])
				for _, test := range testPool() {
					if settled[test] && !progress.Settled(test) {
						t.Fatalf("%s reopened after %s", test, mutant.ID)
					}
					settled[test] = progress.Settled(test)
				}
			}
		})
	})
}
