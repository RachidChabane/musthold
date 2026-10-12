package vacuity_test

import (
	"reflect"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
	"github.com/RachidChabane/spec-proof/format"
)

const verifiesConfirmation = " [verifies SP-2144~1]"

// The lines of the fixture's five survivals: three changes on the first site,
// two on the second.
var (
	firstSite  = line(parseFile, 41)
	secondSite = line(parseFile, 44)
)

func confirm(t *testing.T, input vacuity.Input, result vacuity.Result, confirmation vacuity.Confirmation) vacuity.Result {
	t.Helper()
	confirmed, err := vacuity.Confirm(input, result, confirmation)
	if err != nil {
		t.Fatal(err)
	}
	return confirmed
}

// passesAlone is the test passing every mutant, alone, three runs out of three.
func passesAlone(mutants []string) map[string][]vacuity.Run {
	byMutant := map[string][]vacuity.Run{}
	for _, mutant := range mutants {
		byMutant[mutant] = runs(vacuity.RunPassed)
	}
	return byMutant
}

// ownsAlone is a recording of a test alone that ran and owns the lines.
func ownsAlone(lines ...vacuity.Line) vacuity.Coverage {
	return vacuity.Coverage{Lines: lines, Owned: lines}
}

func mutantsOf(verdict vacuity.Verdict) []string {
	var ids []string
	for _, change := range verdict.Changes {
		ids = append(ids, change.Mutant)
	}
	return ids
}

func TestConfirm(t *testing.T) {
	t.Run("a claim the test upholds alone keeps every change and its count"+verifiesConfirmation, func(t *testing.T) {
		f := newFixture().sampled(vacuousTest)
		ids := f.fiveSurvivedOnTwoSites(vacuousTest)
		result := classify(t, f.input)
		confirmed := confirm(t, f.input, result, vacuity.Confirmation{Tests: map[ecosystem.TestID]vacuity.Alone{
			vacuousTest: {Coverage: ownsAlone(firstSite, secondSite), Runs: passesAlone(ids)},
		}})
		if !reflect.DeepEqual(confirmed, result) {
			t.Fatalf("confirmed %+v, classified %+v", confirmed, result)
		}
	})

	t.Run("a change on a line the test owns only beside other tests leaves its claim"+verifiesConfirmation, func(t *testing.T) {
		f := newFixture().sampled(vacuousTest)
		ids := f.fiveSurvivedOnTwoSites(vacuousTest)
		confirmed := confirm(t, f.input, classify(t, f.input), vacuity.Confirmation{Tests: map[ecosystem.TestID]vacuity.Alone{
			vacuousTest: {Coverage: ownsAlone(firstSite), Runs: passesAlone(ids)},
		}})
		verdict := verdictOf(t, confirmed, vacuousTest)
		assertClass(t, verdict, format.ScanUnproven, format.ScanTooFewChanges)
		if len(verdict.Changes) != 3 || verdict.Sites() != 1 {
			t.Fatalf("changes %+v", verdict.Changes)
		}
	})

	t.Run("a line the test ran alone without owning it confirms no change"+verifiesConfirmation, func(t *testing.T) {
		f := newFixture().sampled(vacuousTest)
		ids := f.fiveSurvivedOnTwoSites(vacuousTest)
		ranOnly := vacuity.Coverage{Lines: []vacuity.Line{firstSite, secondSite}, Owned: []vacuity.Line{firstSite}}
		confirmed := confirm(t, f.input, classify(t, f.input), vacuity.Confirmation{Tests: map[ecosystem.TestID]vacuity.Alone{
			vacuousTest: {Coverage: ranOnly, Runs: passesAlone(ids)},
		}})
		assertClass(t, verdictOf(t, confirmed, vacuousTest), format.ScanUnproven, format.ScanTooFewChanges)
	})

	t.Run("a change the test does not pass alone three runs out of three is inconclusive"+verifiesConfirmation, func(t *testing.T) {
		for _, alone := range [][]vacuity.Run{
			runs(vacuity.RunFailed),
			{vacuity.RunPassed, vacuity.RunFailed, vacuity.RunPassed},
			runs(vacuity.RunTimedOut),
			runs(vacuity.RunNotStarted),
			nil,
		} {
			f := newFixture().sampled(vacuousTest)
			ids := f.fiveSurvivedOnTwoSites(vacuousTest)
			ran := passesAlone(ids)
			ran[ids[4]] = alone
			confirmed := confirm(t, f.input, classify(t, f.input), vacuity.Confirmation{Tests: map[ecosystem.TestID]vacuity.Alone{
				vacuousTest: {Coverage: ownsAlone(firstSite, secondSite), Runs: ran},
			}})
			verdict := verdictOf(t, confirmed, vacuousTest)
			assertClass(t, verdict, format.ScanUnproven, format.ScanInconclusiveOutcome)
			if !slices.Contains(verdict.Changes, vacuity.Change{Mutant: ids[4], Site: secondSite, Outcome: format.ScanInconclusive}) {
				t.Fatalf("runs alone %v: changes %+v", alone, verdict.Changes)
			}
		}
	})

	t.Run("a test the confirmation holds nothing for is unproven, each of its changes inconclusive"+verifiesConfirmation, func(t *testing.T) {
		f := newFixture().sampled(vacuousTest)
		ids := f.fiveSurvivedOnTwoSites(vacuousTest)
		verdict := verdictOf(t, confirm(t, f.input, classify(t, f.input), vacuity.Confirmation{}), vacuousTest)
		assertClass(t, verdict, format.ScanUnproven, format.ScanInconclusiveOutcome)
		if !slices.Equal(slices.Sorted(slices.Values(mutantsOf(verdict))), slices.Sorted(slices.Values(ids))) {
			t.Fatalf("changes %+v", verdict.Changes)
		}
	})

	t.Run("a change only an unconfirmed killer proves leaves the test's changes, and the killer and the route stand"+verifiesConfirmation, func(t *testing.T) {
		f := newFixture().sampled(vacuousTest).sampled(killerTest, firstSite)
		ids := f.fiveSurvivedOnTwoSites(vacuousTest)
		killed := f.mutant(vacuity.Mutant{ID: mutantID("killed"), Lines: []vacuity.Line{firstSite}})
		f.outcome(killed, killerTest, runs(vacuity.RunFailed)).outcome(killed, vacuousTest, runs(vacuity.RunPassed))
		result := classify(t, f.input)
		if len(verdictOf(t, result, vacuousTest).Changes) != len(ids)+1 {
			t.Fatalf("changes %+v", verdictOf(t, result, vacuousTest).Changes)
		}
		runsAlone := passesAlone(append(slices.Clone(ids), killed))
		alone := vacuity.Confirmation{Tests: map[ecosystem.TestID]vacuity.Alone{
			vacuousTest: {Coverage: ownsAlone(firstSite, secondSite), Runs: runsAlone},
		}}
		unconfirmed := confirm(t, f.input, result, alone)
		if slices.Contains(mutantsOf(verdictOf(t, unconfirmed, vacuousTest)), killed) || len(verdictOf(t, unconfirmed, vacuousTest).Changes) != len(ids) {
			t.Fatalf("changes %+v", verdictOf(t, unconfirmed, vacuousTest).Changes)
		}
		assertClass(t, verdictOf(t, unconfirmed, vacuousTest), format.ScanVerifiesNothing, "")
		if !reflect.DeepEqual(verdictOf(t, unconfirmed, killerTest), verdictOf(t, result, killerTest)) || !reflect.DeepEqual(unconfirmed.Routes, result.Routes) {
			t.Fatalf("killer %+v, routes %v", verdictOf(t, unconfirmed, killerTest), unconfirmed.Routes)
		}
		alone.Killers = map[string][]ecosystem.TestID{killed: {killerTest}}
		if upheld := confirm(t, f.input, result, alone); !reflect.DeepEqual(upheld, result) {
			t.Fatalf("a confirmed killer: %+v", verdictOf(t, upheld, vacuousTest).Changes)
		}
	})

	t.Run("a killer confirmed for another mutant or not among the mutant's killers confirms nothing"+verifiesConfirmation, func(t *testing.T) {
		f := newFixture().sampled(vacuousTest).sampled(killerTest, firstSite)
		ids := f.fiveSurvivedOnTwoSites(vacuousTest)
		killed := f.mutant(vacuity.Mutant{ID: mutantID("killed"), Lines: []vacuity.Line{firstSite}})
		f.outcome(killed, killerTest, runs(vacuity.RunFailed)).outcome(killed, vacuousTest, runs(vacuity.RunPassed))
		result := classify(t, f.input)
		for _, killers := range []map[string][]ecosystem.TestID{{ids[0]: {killerTest}}, {killed: {otherTest}}} {
			confirmed := confirm(t, f.input, result, vacuity.Confirmation{
				Tests:   map[ecosystem.TestID]vacuity.Alone{vacuousTest: {Coverage: ownsAlone(firstSite, secondSite), Runs: passesAlone(append(slices.Clone(ids), killed))}},
				Killers: killers,
			})
			if slices.Contains(mutantsOf(verdictOf(t, confirmed, vacuousTest)), killed) {
				t.Fatalf("killers %v confirmed %s", killers, killed)
			}
		}
	})

	t.Run("a change another route proves keeps its place whatever its killers do alone"+verifiesConfirmation, func(t *testing.T) {
		f := newFixture().sampled(vacuousTest).sampled(killerTest, firstSite)
		ids := f.fiveSurvivedOnTwoSites(vacuousTest)
		f.outcome(ids[0], killerTest, runs(vacuity.RunFailed))
		result := classify(t, f.input)
		claims, err := vacuity.Claims(f.input, result)
		if err != nil || len(claims) != 1 {
			t.Fatalf("claims %+v, %v", claims, err)
		}
		for _, change := range claims[0].Changes {
			if change.KillsOnly() {
				t.Fatalf("%s has a difference and kills, yet reads as proven by kills only", change.Mutant)
			}
		}
		confirmed := confirm(t, f.input, result, vacuity.Confirmation{Tests: map[ecosystem.TestID]vacuity.Alone{
			vacuousTest: {Coverage: ownsAlone(firstSite, secondSite), Runs: passesAlone(ids)},
		}})
		if !reflect.DeepEqual(confirmed, result) {
			t.Fatalf("changes %+v", verdictOf(t, confirmed, vacuousTest).Changes)
		}
	})

	t.Run("a claim lists the killers of a change only kills prove, and the changes it cites"+verifiesConfirmation, func(t *testing.T) {
		f := newFixture().sampled(vacuousTest).sampled(killerTest, firstSite).sampled(otherTest, firstSite)
		ids := f.fiveSurvivedOnTwoSites(vacuousTest)
		killed := f.mutant(vacuity.Mutant{ID: mutantID("killed"), Lines: []vacuity.Line{firstSite}})
		f.outcome(killed, otherTest, runs(vacuity.RunFailed)).outcome(killed, killerTest, runs(vacuity.RunFailed)).outcome(killed, vacuousTest, runs(vacuity.RunPassed))
		claims, err := vacuity.Claims(f.input, classify(t, f.input))
		if err != nil || len(claims) != 1 || claims[0].Test != vacuousTest || len(claims[0].Changes) != len(ids)+1 {
			t.Fatalf("claims %+v, %v", claims, err)
		}
		for _, change := range claims[0].Changes {
			want := []ecosystem.TestID(nil)
			if change.Mutant == killed {
				want = []ecosystem.TestID{killerTest, otherTest}
			}
			if !slices.Equal(change.Killers, want) || len(change.Lines) == 0 {
				t.Fatalf("change %+v", change)
			}
		}
		if owned := claims[0].OwnedAlone(ownsAlone(secondSite)); len(owned.Changes) != 2 {
			t.Fatalf("owned alone %+v", owned.Changes)
		}
		if kept := claims[0].KilledAlone(nil); len(kept.Changes) != len(ids) {
			t.Fatalf("killed alone %+v", kept.Changes)
		}
	})

	t.Run("a claim with fewer changes left than it needs is unproven and prints no count"+verifiesConfirmation, func(t *testing.T) {
		f := newFixture().sampled(vacuousTest)
		ids := f.fiveSurvivedOnTwoSites(vacuousTest)
		confirmed := confirm(t, f.input, classify(t, f.input), vacuity.Confirmation{Tests: map[ecosystem.TestID]vacuity.Alone{
			vacuousTest: {Coverage: ownsAlone(), Runs: passesAlone(ids)},
		}})
		assertClass(t, verdictOf(t, confirmed, vacuousTest), format.ScanUnproven, format.ScanTooFewChanges)
		if confirmed.Counts.VerifiesNothing != 0 || confirmed.Counts.Unproven != 1 || confirmed.Counts.Tests != 1 {
			t.Fatalf("counts %+v", confirmed.Counts)
		}
	})

	t.Run("two mutants with one id are refused"+verifiesConfirmation, func(t *testing.T) {
		f := newFixture().sampled(vacuousTest)
		f.fiveSurvivedOnTwoSites(vacuousTest)
		result := classify(t, f.input)
		f.input.Mutants = append(f.input.Mutants, f.input.Mutants[0])
		if _, err := vacuity.Confirm(f.input, result, vacuity.Confirmation{}); err == nil {
			t.Fatal("no error")
		}
	})
}

// confirmationGenerator draws, for the claims of a result, what running their
// tests alone established: some tests without an entry, recordings alone that
// own some of the lines, runs alone of any kind, and some killers confirmed.
func confirmationGenerator(input vacuity.Input, result vacuity.Result) *rapid.Generator[vacuity.Confirmation] {
	return rapid.Custom(func(t *rapid.T) vacuity.Confirmation {
		claims, err := vacuity.Claims(input, result)
		if err != nil {
			t.Fatal(err)
		}
		confirmation := vacuity.Confirmation{Tests: map[ecosystem.TestID]vacuity.Alone{}, Killers: map[string][]ecosystem.TestID{}}
		if !chance(t, "killersRan", likelyShare) {
			confirmation.Killers = nil
		}
		for _, claim := range claims {
			if !chance(t, "held", likelyShare) {
				continue
			}
			lines := eachOf(t, "aloneLines", linePool(), likelyShare)
			alone := vacuity.Alone{Coverage: vacuity.Coverage{Lines: lines, Owned: eachOf(t, "aloneOwned", lines, likelyShare)}, Runs: map[string][]vacuity.Run{}}
			for _, change := range claim.Changes {
				alone.Runs[change.Mutant] = mutantRuns(t, false)
				if confirmation.Killers != nil {
					confirmation.Killers[change.Mutant] = eachOf(t, "confirmedKillers", append(slices.Clone(change.Killers), testPool()...), ownedShare)
				}
			}
			confirmation.Tests[claim.Test] = alone
		}
		return confirmation
	})
}

func mustConfirm(t *rapid.T, input vacuity.Input, result vacuity.Result, confirmation vacuity.Confirmation) vacuity.Result {
	confirmed, err := vacuity.Confirm(input, result, confirmation)
	if err != nil {
		t.Fatal(err)
	}
	return confirmed
}

func TestConfirmationProperties(t *testing.T) {
	t.Run("confirmation only demotes a test that verifies nothing: no other verdict, no route, no new change"+verifiesConfirmation, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			input := inputGenerator().Draw(t, "input")
			result := mustClassify(t, input)
			confirmed := mustConfirm(t, input, result, confirmationGenerator(input, result).Draw(t, "confirmation"))
			if !reflect.DeepEqual(confirmed.Routes, result.Routes) {
				t.Fatal("confirmation changed the routes")
			}
			before := verdictsByTest(result)
			for test, verdict := range verdictsByTest(confirmed) {
				was := before[test]
				if was.Class != format.ScanVerifiesNothing {
					if !reflect.DeepEqual(verdict, was) {
						t.Fatalf("confirmation moved %s from %+v to %+v", test, was, verdict)
					}
					continue
				}
				if verdict.Class != format.ScanVerifiesNothing && verdict.Class != format.ScanUnproven {
					t.Fatalf("confirmation made %s %s", test, verdict.Class)
				}
				for _, change := range verdict.Changes {
					if !slices.ContainsFunc(was.Changes, func(c vacuity.Change) bool { return c.Mutant == change.Mutant && c.Site == change.Site }) {
						t.Fatalf("confirmation gave %s the change %s", test, change.Mutant)
					}
					if change.Outcome == format.ScanKilled {
						t.Fatalf("confirmation gave %s a kill", test)
					}
				}
				for _, mutant := range verdict.NotStarted {
					if !slices.Contains(was.NotStarted, mutant) {
						t.Fatalf("confirmation listed %s apart for %s", mutant, test)
					}
				}
			}
			if len(confirmed.Verdicts) != len(result.Verdicts) || confirmed.Counts.Tests != result.Counts.Tests ||
				confirmed.Counts.VerifiesNothing > result.Counts.VerifiesNothing ||
				confirmed.Counts.Caught != result.Counts.Caught || confirmed.Counts.NotSampled != result.Counts.NotSampled || confirmed.Counts.NotGreen != result.Counts.NotGreen {
				t.Fatalf("counts %+v, then %+v", result.Counts, confirmed.Counts)
			}
		})
	})

	t.Run("a confirmed result counts its verdicts, ranks them and writes a statement the format accepts"+verifiesConfirmation, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			input := inputGenerator().Draw(t, "input")
			result := mustClassify(t, input)
			confirmed := mustConfirm(t, input, result, confirmationGenerator(input, result).Draw(t, "confirmation"))
			var counts format.ScanCounts
			for _, verdict := range confirmed.Verdicts {
				counts = counts.Add(verdict.Class)
			}
			if counts != confirmed.Counts {
				t.Fatalf("counts %+v, verdicts give %+v", confirmed.Counts, counts)
			}
			reranked := slices.Clone(confirmed.Verdicts)
			slices.SortStableFunc(reranked, func(a, b vacuity.Verdict) int { return compareRank(a, b) })
			if !reflect.DeepEqual(reranked, confirmed.Verdicts) {
				t.Fatal("the confirmed verdicts are not ranked")
			}
			for _, test := range confirmed.ScanTests() {
				if test.Class == format.ScanVerifiesNothing && slices.ContainsFunc(test.Changes, func(c format.ScanChange) bool { return c.Outcome != format.ScanSurvived }) {
					t.Fatalf("%s verifies nothing with %+v", test.ID, test.Changes)
				}
			}
		})
	})

	t.Run("a confirmation that upholds every change changes nothing, and confirming twice is confirming once"+verifiesConfirmation, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			input := inputGenerator().Draw(t, "input")
			result := mustClassify(t, input)
			claims, err := vacuity.Claims(input, result)
			if err != nil {
				t.Fatal(err)
			}
			upheld := vacuity.Confirmation{Tests: map[ecosystem.TestID]vacuity.Alone{}, Killers: map[string][]ecosystem.TestID{}}
			for _, claim := range claims {
				alone := vacuity.Alone{Coverage: input.Coverage[claim.Test], Runs: map[string][]vacuity.Run{}}
				for _, change := range claim.Changes {
					alone.Runs[change.Mutant] = runs(vacuity.RunPassed)
					upheld.Killers[change.Mutant] = change.Killers
				}
				upheld.Tests[claim.Test] = alone
			}
			if !sameJudgement(mustConfirm(t, input, result, upheld), result) {
				t.Fatal("upholding every change changed the judgement")
			}
			drawn := confirmationGenerator(input, result).Draw(t, "confirmation")
			once := mustConfirm(t, input, result, drawn)
			if !sameJudgement(mustConfirm(t, input, once, drawn), once) {
				t.Fatal("confirming twice differs from confirming once")
			}
		})
	})

	t.Run("the order of the mutants and of the lines changes no confirmed judgement"+verifiesConfirmation, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			input := inputGenerator().Draw(t, "input")
			result := mustClassify(t, input)
			confirmation := confirmationGenerator(input, result).Draw(t, "confirmation")
			reordered := cloneInput(input)
			slices.Reverse(reordered.Mutants)
			for test, alone := range confirmation.Tests {
				lines, owned := slices.Clone(alone.Coverage.Lines), slices.Clone(alone.Coverage.Owned)
				slices.Reverse(lines)
				slices.Reverse(owned)
				alone.Coverage = vacuity.Coverage{Lines: lines, Owned: owned}
				confirmation.Tests[test] = alone
			}
			if !sameJudgement(mustConfirm(t, input, result, confirmation), mustConfirm(t, reordered, mustClassify(t, reordered), confirmation)) {
				t.Fatal("reordering changed the confirmed judgement")
			}
		})
	})
}

// compareRank ranks verdicts as a result does: by class in the order of the
// format, then the most changes first, then the test id byte-wise.
func compareRank(a, b vacuity.Verdict) int {
	if byClass := slices.Index(format.ScanClasses(), a.Class) - slices.Index(format.ScanClasses(), b.Class); byClass != 0 {
		return byClass
	}
	if byChanges := len(b.Changes) - len(a.Changes); byChanges != 0 {
		return byChanges
	}
	switch {
	case a.Test < b.Test:
		return -1
	case a.Test > b.Test:
		return 1
	}
	return 0
}
