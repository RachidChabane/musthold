package vacuity_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
	"github.com/RachidChabane/spec-proof/format"
)

func TestSettle(t *testing.T) {
	cases := []struct {
		name string
		runs []vacuity.Run
		want vacuity.Settled
	}{
		{"three failures", runs(vacuity.RunFailed), vacuity.SettledKilled},
		{"three passes", runs(vacuity.RunPassed), vacuity.SettledSurvived},
		{"three runs that did not start", runs(vacuity.RunNotStarted), vacuity.SettledNotStarted},
		{"a timeout among passes", []vacuity.Run{vacuity.RunPassed, vacuity.RunTimedOut, vacuity.RunPassed}, vacuity.SettledInconclusive},
		{"a timeout among failures", []vacuity.Run{vacuity.RunFailed, vacuity.RunFailed, vacuity.RunTimedOut}, vacuity.SettledInconclusive},
		{"an error", []vacuity.Run{vacuity.RunErrored, vacuity.RunErrored, vacuity.RunErrored}, vacuity.SettledInconclusive},
		{"runs that disagree", []vacuity.Run{vacuity.RunPassed, vacuity.RunFailed, vacuity.RunPassed}, vacuity.SettledInconclusive},
		{"a run that did not start among passes", []vacuity.Run{vacuity.RunNotStarted, vacuity.RunPassed, vacuity.RunPassed}, vacuity.SettledInconclusive},
		{"a run that did not start among failures", []vacuity.Run{vacuity.RunFailed, vacuity.RunNotStarted, vacuity.RunFailed}, vacuity.SettledInconclusive},
		{"runs that did not start and a timeout", []vacuity.Run{vacuity.RunNotStarted, vacuity.RunNotStarted, vacuity.RunTimedOut}, vacuity.SettledInconclusive},
		{"two runs that did not start only", runs(vacuity.RunNotStarted)[:2], vacuity.SettledInconclusive},
		{"two failures only", []vacuity.Run{vacuity.RunFailed, vacuity.RunFailed}, vacuity.SettledInconclusive},
		{"four passes", append(runs(vacuity.RunPassed), vacuity.RunPassed), vacuity.SettledInconclusive},
		{"no run", nil, vacuity.SettledInconclusive},
	}
	for _, c := range cases {
		t.Run(c.name+" settle as "+string(c.want)+" [verifies SP-2020~2]", func(t *testing.T) {
			if got := vacuity.Settle(c.runs); got != c.want {
				t.Fatalf("Settle = %s, want %s", got, c.want)
			}
		})
	}

	t.Run("a settled outcome is recorded as itself, and not_started is never recorded as a change [verifies SP-2020~2]", func(t *testing.T) {
		for settled, want := range map[vacuity.Settled]format.ScanOutcome{
			vacuity.SettledKilled: format.ScanKilled, vacuity.SettledSurvived: format.ScanSurvived, vacuity.SettledInconclusive: format.ScanInconclusive,
		} {
			if got, recorded := settled.Recorded(); !recorded || got != want {
				t.Fatalf("%s is recorded as %q, %v", settled, got, recorded)
			}
		}
		if got, recorded := vacuity.SettledNotStarted.Recorded(); recorded || got != "" {
			t.Fatalf("not_started is recorded as %q", got)
		}
	})

	t.Run("a test is green only on three passes out of three [verifies SP-2020~2]", func(t *testing.T) {
		if !vacuity.Green(runs(vacuity.RunPassed)) {
			t.Fatal("three passes are not green")
		}
		for _, notGreen := range [][]vacuity.Run{
			nil, runs(vacuity.RunPassed)[:2], {vacuity.RunPassed, vacuity.RunPassed, vacuity.RunTimedOut},
			{vacuity.RunPassed, vacuity.RunNotStarted, vacuity.RunPassed}, runs(vacuity.RunNotStarted),
		} {
			if vacuity.Green(notGreen) {
				t.Fatalf("%v is green", notGreen)
			}
		}
	})
}

func TestKilledByTestRoute(t *testing.T) {
	build := func(killer []vacuity.Run, sentinelRuns []vacuity.Run, kill []vacuity.Run) vacuity.Result {
		f := newFixture()
		f.input.Reference[killerTest] = killer
		target := f.mutant(vacuity.Mutant{ID: mutantID("target"), Lines: []vacuity.Line{line(parseFile, 41)}})
		sentinel := f.mutant(vacuity.Mutant{ID: mutantID("sentinel"), Lines: []vacuity.Line{line(parseFile, 43)}, Sentinel: true})
		f.outcome(target, killerTest, kill)
		if sentinelRuns != nil {
			f.outcome(sentinel, killerTest, sentinelRuns)
		}
		return classify(t, f.input)
	}
	target := mutantID("target")

	t.Run("a green test that kills the mutant three times and no sentinel proves it [verifies SP-2021~1]", func(t *testing.T) {
		result := build(runs(vacuity.RunPassed), runs(vacuity.RunPassed), runs(vacuity.RunFailed))
		want := []format.ScanRoute{{Kind: format.ScanRouteKilledByTest, Test: string(killerTest)}}
		if !slices.Equal(result.Routes[target], want) {
			t.Fatalf("routes = %+v, want %+v", result.Routes[target], want)
		}
	})

	t.Run("a killer that did not start on a sentinel still proves the mutant [verifies SP-2021~1]", func(t *testing.T) {
		result := build(runs(vacuity.RunPassed), runs(vacuity.RunNotStarted), runs(vacuity.RunFailed))
		if !result.Proven(target) {
			t.Fatalf("routes = %+v", result.Routes[target])
		}
	})

	refused := []struct {
		name                       string
		killer, sentinelRuns, kill []vacuity.Run
	}{
		{"a killer that is not green", []vacuity.Run{vacuity.RunPassed, vacuity.RunFailed, vacuity.RunPassed}, nil, runs(vacuity.RunFailed)},
		{"a killer that failed a sentinel once", runs(vacuity.RunPassed), []vacuity.Run{vacuity.RunPassed, vacuity.RunFailed, vacuity.RunPassed}, runs(vacuity.RunFailed)},
		{"a kill in two runs out of three", runs(vacuity.RunPassed), nil, []vacuity.Run{vacuity.RunFailed, vacuity.RunPassed, vacuity.RunFailed}},
		{"a kill that timed out once", runs(vacuity.RunPassed), nil, []vacuity.Run{vacuity.RunFailed, vacuity.RunTimedOut, vacuity.RunFailed}},
		{"runs that did not start", runs(vacuity.RunPassed), nil, runs(vacuity.RunNotStarted)},
		{"a kill with a run that did not start", runs(vacuity.RunPassed), nil, []vacuity.Run{vacuity.RunFailed, vacuity.RunNotStarted, vacuity.RunFailed}},
	}
	for _, c := range refused {
		t.Run(c.name+" proves nothing [verifies SP-2021~1]", func(t *testing.T) {
			if result := build(c.killer, c.sentinelRuns, c.kill); result.Proven(target) {
				t.Fatalf("routes = %+v", result.Routes[target])
			}
		})
	}
}

func TestDifferentialRoute(t *testing.T) {
	cases := []struct {
		name   string
		edit   func(*vacuity.Difference)
		proves bool
	}{
		{"a reproduced difference on an agreeing reference", func(*vacuity.Difference) {}, true},
		{"a raised exception", func(d *vacuity.Difference) { d.Channel = format.ScanChannelException }, true},
		{"a mutated argument", func(d *vacuity.Difference) { d.Channel = format.ScanChannelArgumentMutation }, true},
		{"a difference that did not reproduce", func(d *vacuity.Difference) { d.Reproduced = false }, false},
		{"a reference that disagrees with itself", func(d *vacuity.Difference) { d.ReferenceAgrees = false }, false},
		{"equal results", func(d *vacuity.Difference) { d.Mutant = d.Reference }, false},
		{"a difference on the standard output", func(d *vacuity.Difference) { d.Channel = "stdout" }, false},
		{"a difference without input", func(d *vacuity.Difference) { d.Input = "" }, false},
		{"a difference without its operation", func(d *vacuity.Difference) { d.Operation = "" }, false},
	}
	for _, c := range cases {
		t.Run(c.name+" is a proof only when executed and reproduced [verifies SP-2022~1]", func(t *testing.T) {
			difference := differs()
			c.edit(&difference)
			f := newFixture()
			id := f.mutant(vacuity.Mutant{ID: mutantID("m"), Lines: []vacuity.Line{line(parseFile, 41)}, Differences: []vacuity.Difference{difference}})
			result := classify(t, f.input)
			if result.Proven(id) != c.proves {
				t.Fatalf("proven = %v, want %v (routes %+v)", result.Proven(id), c.proves, result.Routes[id])
			}
			if c.proves && (result.Routes[id][0].Kind != format.ScanRouteDifferential || result.Routes[id][0].Operation != difference.Operation || result.Routes[id][0].Input != difference.Input) {
				t.Fatalf("routes = %+v", result.Routes[id])
			}
		})
	}
}

func TestWitnessRouteAndUnprovableMutants(t *testing.T) {
	failure := vacuity.WitnessFailure{Witness: mutantID("witness"), Admitted: true, Blind: true, Reproduced: true}
	cases := []struct {
		name   string
		mutant func() vacuity.Mutant
		proves bool
	}{
		{"an admitted blind witness failing reproduced", func() vacuity.Mutant {
			return vacuity.Mutant{ID: mutantID("m"), Lines: []vacuity.Line{line(parseFile, 41)}, WitnessFailures: []vacuity.WitnessFailure{failure}}
		}, true},
		{"a witness that is not admitted", func() vacuity.Mutant {
			w := failure
			w.Admitted = false
			return vacuity.Mutant{ID: mutantID("m"), Lines: []vacuity.Line{line(parseFile, 41)}, WitnessFailures: []vacuity.WitnessFailure{w}}
		}, false},
		{"a witness whose writer was not blind", func() vacuity.Mutant {
			w := failure
			w.Blind = false
			return vacuity.Mutant{ID: mutantID("m"), Lines: []vacuity.Line{line(parseFile, 41)}, WitnessFailures: []vacuity.WitnessFailure{w}}
		}, false},
		{"a witness failure that did not reproduce", func() vacuity.Mutant {
			w := failure
			w.Reproduced = false
			return vacuity.Mutant{ID: mutantID("m"), Lines: []vacuity.Line{line(parseFile, 41)}, WitnessFailures: []vacuity.WitnessFailure{w}}
		}, false},
		{"a sentinel with every route", func() vacuity.Mutant {
			return vacuity.Mutant{ID: mutantID("m"), Lines: []vacuity.Line{line(parseFile, 41)}, Sentinel: true, Differences: []vacuity.Difference{differs()}, WitnessFailures: []vacuity.WitnessFailure{failure}}
		}, false},
		{"a mutant identical in compiled form", func() vacuity.Mutant {
			return vacuity.Mutant{ID: mutantID("m"), Lines: []vacuity.Line{line(parseFile, 41)}, CompiledIdentical: true, Differences: []vacuity.Difference{differs()}}
		}, false},
		{"a mutant that mutates no line", func() vacuity.Mutant {
			return vacuity.Mutant{ID: mutantID("m"), Differences: []vacuity.Difference{differs()}}
		}, false},
	}
	for _, c := range cases {
		t.Run(c.name+" is checked [verifies SP-2023~1]", func(t *testing.T) {
			f := newFixture()
			id := f.mutant(c.mutant())
			if got := classify(t, f.input).Proven(id); got != c.proves {
				t.Fatalf("proven = %v, want %v", got, c.proves)
			}
		})
	}

	t.Run("routes are listed once each, in the order of the format [verifies SP-2023~1]", func(t *testing.T) {
		f := newFixture()
		f.input.Reference[killerTest] = runs(vacuity.RunPassed)
		f.input.Reference[otherTest] = runs(vacuity.RunPassed)
		id := f.mutant(vacuity.Mutant{
			ID: mutantID("m"), Lines: []vacuity.Line{line(parseFile, 41)},
			Differences:     []vacuity.Difference{differs(), differs()},
			WitnessFailures: []vacuity.WitnessFailure{failure, failure},
		})
		f.outcome(id, otherTest, runs(vacuity.RunFailed)).outcome(id, killerTest, runs(vacuity.RunFailed))
		routes := classify(t, f.input).Routes[id]
		if len(routes) != 4 || !slices.IsSortedFunc(routes, format.CompareScanRoutes) {
			t.Fatalf("routes = %+v", routes)
		}
	})
}

func TestChanges(t *testing.T) {
	t.Run("a test's changes are the proven mutants of the lines it ran, by site then id [verifies SP-2024~2]", func(t *testing.T) {
		f := newFixture().sampled(vacuousTest, line(parseFile, 41), line(totalFile, 7))
		ran := f.proven("ran", line(totalFile, 7))
		multi := f.mutant(vacuity.Mutant{ID: mutantID("multi"), Lines: []vacuity.Line{line(parseFile, 50), line(parseFile, 41)}, Differences: []vacuity.Difference{differs()}})
		f.proven("not run", line(parseFile, 42))
		f.mutant(vacuity.Mutant{ID: mutantID("unproven"), Lines: []vacuity.Line{line(parseFile, 41)}})
		f.outcome(ran, vacuousTest, runs(vacuity.RunPassed)).outcome(multi, vacuousTest, runs(vacuity.RunPassed))
		verdict := verdictOf(t, classify(t, f.input), vacuousTest)
		want := []vacuity.Change{
			{Mutant: multi, Site: line(parseFile, 41), Outcome: format.ScanSurvived},
			{Mutant: ran, Site: line(totalFile, 7), Outcome: format.ScanSurvived},
		}
		if !slices.Equal(verdict.Changes, want) {
			t.Fatalf("changes = %+v, want %+v", verdict.Changes, want)
		}
	})

	t.Run("a proven change the test ran but never ran against is inconclusive [verifies SP-2024~2]", func(t *testing.T) {
		f := newFixture().sampled(vacuousTest, line(parseFile, 41))
		f.proven("m", line(parseFile, 41))
		verdict := verdictOf(t, classify(t, f.input), vacuousTest)
		if len(verdict.Changes) != 1 || verdict.Changes[0].Outcome != format.ScanInconclusive {
			t.Fatalf("changes = %+v", verdict.Changes)
		}
	})

	pairs := []struct {
		name       string
		runs       []vacuity.Run
		owned      bool
		change     format.ScanOutcome
		notStarted bool
	}{
		{"a kill on an owned line is a change", runs(vacuity.RunFailed), true, format.ScanKilled, false},
		{"a kill on a start-up-only line is a change", runs(vacuity.RunFailed), false, format.ScanKilled, false},
		{"an inconclusive outcome on an owned line is a change", []vacuity.Run{vacuity.RunPassed, vacuity.RunErrored, vacuity.RunPassed}, true, format.ScanInconclusive, false},
		{"an inconclusive outcome on a start-up-only line is a change", []vacuity.Run{vacuity.RunPassed, vacuity.RunNotStarted, vacuity.RunPassed}, false, format.ScanInconclusive, false},
		{"a survival on an owned line is a change", runs(vacuity.RunPassed), true, format.ScanSurvived, false},
		{"a survival on a start-up-only line is ignored", runs(vacuity.RunPassed), false, "", false},
		{"runs that did not start on an owned line are listed apart", runs(vacuity.RunNotStarted), true, "", true},
		{"runs that did not start on a start-up-only line are ignored", runs(vacuity.RunNotStarted), false, "", false},
	}
	for _, c := range pairs {
		t.Run(c.name+" [verifies SP-2024~2]", func(t *testing.T) {
			f := newFixture().sampled(vacuousTest)
			at := line(parseFile, 41)
			if c.owned {
				f.owns(vacuousTest, at)
			} else {
				f.atStartUp(vacuousTest, at)
			}
			id := f.proven("pair", at)
			f.outcome(id, vacuousTest, c.runs)
			verdict := verdictOf(t, classify(t, f.input), vacuousTest)
			var want []vacuity.Change
			if c.change != "" {
				want = []vacuity.Change{{Mutant: id, Site: at, Outcome: c.change}}
			}
			var wantNotStarted []string
			if c.notStarted {
				wantNotStarted = []string{id}
			}
			if !slices.Equal(verdict.Changes, want) || !slices.Equal(verdict.NotStarted, wantNotStarted) {
				t.Fatalf("changes %+v, not started %v; want %+v, %v", verdict.Changes, verdict.NotStarted, want, wantNotStarted)
			}
		})
	}

	t.Run("a mutant with one mutated line the test owns counts as on an owned line [verifies SP-2024~2]", func(t *testing.T) {
		f := newFixture().sampled(vacuousTest).owns(vacuousTest, line(parseFile, 42)).atStartUp(vacuousTest, line(parseFile, 41))
		id := f.mutant(vacuity.Mutant{ID: mutantID("both"), Lines: []vacuity.Line{line(parseFile, 41), line(parseFile, 42)}, Differences: []vacuity.Difference{differs()}})
		f.outcome(id, vacuousTest, runs(vacuity.RunPassed))
		if verdict := verdictOf(t, classify(t, f.input), vacuousTest); len(verdict.Changes) != 1 || verdict.Changes[0].Outcome != format.ScanSurvived {
			t.Fatalf("changes = %+v", verdict.Changes)
		}
	})

	t.Run("an unproven mutant that stopped the test from starting is not listed [verifies SP-2024~2]", func(t *testing.T) {
		f := newFixture().sampled(vacuousTest, line(parseFile, 41))
		id := f.mutant(vacuity.Mutant{ID: mutantID("unproven"), Lines: []vacuity.Line{line(parseFile, 41)}})
		f.outcome(id, vacuousTest, runs(vacuity.RunNotStarted))
		if verdict := verdictOf(t, classify(t, f.input), vacuousTest); verdict.NotStarted != nil || verdict.Changes != nil {
			t.Fatalf("verdict = %+v", verdict)
		}
	})
}

func TestStartUp(t *testing.T) {
	t.Run("survivals on lines the test ran only at start-up never make it verify nothing [verifies SP-2133~1]", func(t *testing.T) {
		f := newFixture().sampled(vacuousTest)
		for index, site := range []vacuity.Line{line(parseFile, 41), line(parseFile, 41), line(parseFile, 41), line(parseFile, 44), line(parseFile, 44)} {
			id := f.proven(fmt.Sprintf("start-up/%d", index), site)
			f.outcome(id, vacuousTest, runs(vacuity.RunPassed))
		}
		f.atStartUp(vacuousTest, line(parseFile, 41), line(parseFile, 44))
		verdict := verdictOf(t, classify(t, f.input), vacuousTest)
		assertClass(t, verdict, format.ScanUnproven, format.ScanTooFewChanges)
		if len(verdict.Changes) != 0 {
			t.Fatalf("changes = %+v", verdict.Changes)
		}
	})

	t.Run("a test that verifies nothing keeps its claim beside the changes that stopped it from starting [verifies SP-2133~1]", func(t *testing.T) {
		f := newFixture().sampled(vacuousTest, line(totalFile, 3))
		f.fiveSurvivedOnTwoSites(vacuousTest)
		stopped := f.proven("stopped", line(totalFile, 3))
		f.outcome(stopped, vacuousTest, runs(vacuity.RunNotStarted))
		verdict := verdictOf(t, classify(t, f.input), vacuousTest)
		assertClass(t, verdict, format.ScanVerifiesNothing, "")
		if len(verdict.Changes) != vacuity.MinChanges || !slices.Equal(verdict.NotStarted, []string{stopped}) {
			t.Fatalf("changes %+v, not started %v", verdict.Changes, verdict.NotStarted)
		}
	})

	t.Run("an inconclusive outcome on a start-up-only line keeps the test from verifying nothing [verifies SP-2133~1]", func(t *testing.T) {
		f := newFixture().sampled(vacuousTest)
		f.fiveSurvivedOnTwoSites(vacuousTest)
		blocking := f.proven("blocking", line(totalFile, 3))
		f.atStartUp(vacuousTest, line(totalFile, 3))
		f.outcome(blocking, vacuousTest, []vacuity.Run{vacuity.RunPassed, vacuity.RunTimedOut, vacuity.RunPassed})
		assertClass(t, verdictOf(t, classify(t, f.input), vacuousTest), format.ScanUnproven, format.ScanInconclusiveOutcome)
	})

	t.Run("a kill on a start-up-only line is caught [verifies SP-2133~1]", func(t *testing.T) {
		f := newFixture().sampled(killerTest)
		killed := f.proven("killed", line(totalFile, 3))
		f.atStartUp(killerTest, line(totalFile, 3))
		f.outcome(killed, killerTest, runs(vacuity.RunFailed))
		assertClass(t, verdictOf(t, classify(t, f.input), killerTest), format.ScanCaught, "")
	})

	t.Run("a test whose coverage records no owned line owns none [verifies SP-2133~1]", func(t *testing.T) {
		f := newFixture().sampled(vacuousTest)
		f.fiveSurvivedOnTwoSites(vacuousTest)
		coverage := f.input.Coverage[vacuousTest]
		coverage.Owned = nil
		f.input.Coverage[vacuousTest] = coverage
		assertClass(t, verdictOf(t, classify(t, f.input), vacuousTest), format.ScanUnproven, format.ScanTooFewChanges)
	})
}

func TestClassification(t *testing.T) {
	t.Run("five survived changes on two sites verify nothing [verifies SP-2025~1]", func(t *testing.T) {
		f := newFixture().sampled(vacuousTest)
		f.fiveSurvivedOnTwoSites(vacuousTest)
		verdict := verdictOf(t, classify(t, f.input), vacuousTest)
		assertClass(t, verdict, format.ScanVerifiesNothing, "")
		if len(verdict.Changes) != vacuity.MinChanges || verdict.Sites() != vacuity.MinSites {
			t.Fatalf("%d changes on %d sites", len(verdict.Changes), verdict.Sites())
		}
	})

	t.Run("four survived changes are too few [verifies SP-2025~1]", func(t *testing.T) {
		f := newFixture().sampled(vacuousTest)
		ids := f.fiveSurvivedOnTwoSites(vacuousTest)
		f.input.Mutants = slices.DeleteFunc(f.input.Mutants, func(m vacuity.Mutant) bool { return m.ID == ids[0] })
		assertClass(t, verdictOf(t, classify(t, f.input), vacuousTest), format.ScanUnproven, format.ScanTooFewChanges)
	})

	t.Run("five survived changes on one site are too few [verifies SP-2025~1]", func(t *testing.T) {
		f := newFixture().sampled(vacuousTest, line(parseFile, 41))
		for index := range vacuity.MinChanges {
			id := f.proven(string(rune('a'+index)), line(parseFile, 41))
			f.outcome(id, vacuousTest, runs(vacuity.RunPassed))
		}
		assertClass(t, verdictOf(t, classify(t, f.input), vacuousTest), format.ScanUnproven, format.ScanTooFewChanges)
	})

	t.Run("one killed change makes the test caught [verifies SP-2025~1]", func(t *testing.T) {
		f := newFixture().sampled(vacuousTest)
		ids := f.fiveSurvivedOnTwoSites(vacuousTest)
		f.outcome(ids[2], vacuousTest, runs(vacuity.RunFailed))
		assertClass(t, verdictOf(t, classify(t, f.input), vacuousTest), format.ScanCaught, "")
	})

	t.Run("a test whose own kill is the only proof of a change caught it [verifies SP-2025~1]", func(t *testing.T) {
		f := newFixture().sampled(vacuousTest, line(parseFile, 41))
		id := f.mutant(vacuity.Mutant{ID: mutantID("m"), Lines: []vacuity.Line{line(parseFile, 41)}})
		f.outcome(id, vacuousTest, runs(vacuity.RunFailed))
		assertClass(t, verdictOf(t, classify(t, f.input), vacuousTest), format.ScanCaught, "")
	})

	t.Run("a timeout on one change keeps the test from verifying nothing [verifies SP-2025~1]", func(t *testing.T) {
		f := newFixture().sampled(vacuousTest)
		ids := f.fiveSurvivedOnTwoSites(vacuousTest)
		f.proven("sixth", line(parseFile, 44))
		f.outcome(mutantID("sixth"), vacuousTest, []vacuity.Run{vacuity.RunPassed, vacuity.RunTimedOut, vacuity.RunPassed})
		if len(ids) != vacuity.MinChanges {
			t.Fatal("fixture")
		}
		assertClass(t, verdictOf(t, classify(t, f.input), vacuousTest), format.ScanUnproven, format.ScanInconclusiveOutcome)
	})

	t.Run("a test that failed a sentinel is unproven, even with kills [verifies SP-2025~1]", func(t *testing.T) {
		f := newFixture().sampled(vacuousTest)
		ids := f.fiveSurvivedOnTwoSites(vacuousTest)
		f.outcome(ids[0], vacuousTest, runs(vacuity.RunFailed))
		sentinel := f.mutant(vacuity.Mutant{ID: mutantID("sentinel"), Lines: []vacuity.Line{line(parseFile, 41)}, Sentinel: true})
		f.outcome(sentinel, vacuousTest, []vacuity.Run{vacuity.RunPassed, vacuity.RunPassed, vacuity.RunFailed})
		assertClass(t, verdictOf(t, classify(t, f.input), vacuousTest), format.ScanUnproven, format.ScanKillsSentinel)
	})

	t.Run("a test that is not green has no verdict, sampled or not [verifies SP-2025~1]", func(t *testing.T) {
		f := newFixture().sampled(vacuousTest)
		f.fiveSurvivedOnTwoSites(vacuousTest)
		f.input.Reference[vacuousTest] = []vacuity.Run{vacuity.RunPassed, vacuity.RunPassed, vacuity.RunFailed}
		f.input.Sampled = append(f.input.Sampled, killerTest)
		result := classify(t, f.input)
		assertClass(t, verdictOf(t, result, vacuousTest), format.ScanNotGreen, "")
		assertClass(t, verdictOf(t, result, killerTest), format.ScanNotGreen, "")
		if verdictOf(t, result, vacuousTest).Changes != nil {
			t.Fatal("a test without verdict lists changes")
		}
	})

	t.Run("a green test the scan did not draw is not sampled [verifies SP-2025~1]", func(t *testing.T) {
		f := newFixture()
		f.input.Reference[otherTest] = runs(vacuity.RunPassed)
		f.owns(otherTest, line(parseFile, 41))
		assertClass(t, verdictOf(t, classify(t, f.input), otherTest), format.ScanNotSampled, "")
	})
}

func TestCounts(t *testing.T) {
	t.Run("the counts split the suite into the five classes [verifies SP-2026~1]", func(t *testing.T) {
		f := newFixture().sampled(vacuousTest).sampled(killerTest, line(totalFile, 3)).sampled("tests/test_a.py::test_thin")
		f.fiveSurvivedOnTwoSites(vacuousTest)
		caught := f.proven("caught", line(totalFile, 3))
		f.outcome(caught, killerTest, runs(vacuity.RunFailed))
		f.input.Reference[otherTest] = runs(vacuity.RunPassed)
		f.input.Reference["tests/test_b.py::test_flaky"] = []vacuity.Run{vacuity.RunPassed, vacuity.RunFailed, vacuity.RunPassed}
		counts := classify(t, f.input).Counts
		want := format.ScanCounts{Tests: 5, VerifiesNothing: 1, Caught: 1, Unproven: 1, NotSampled: 1, NotGreen: 1}
		if counts != want {
			t.Fatalf("counts = %+v, want %+v", counts, want)
		}
		if counts.Judged() != 3 {
			t.Fatalf("judged = %d", counts.Judged())
		}
	})
}

func TestRanking(t *testing.T) {
	t.Run("verdicts rank by class, then most changes, then test id [verifies SP-2027~1]", func(t *testing.T) {
		f := newFixture().sampled("tests/b.py::test_b").sampled("tests/a.py::test_a").sampled("tests/c.py::test_c", line(totalFile, 1))
		f.fiveSurvivedOnTwoSites("tests/b.py::test_b")
		f.fiveSurvivedOnTwoSites("tests/a.py::test_a")
		extra := f.proven("extra", line(totalFile, 9))
		f.owns("tests/b.py::test_b", line(totalFile, 9))
		f.outcome(extra, "tests/b.py::test_b", runs(vacuity.RunPassed))
		f.input.Reference["tests/0.py::test_not_sampled"] = runs(vacuity.RunPassed)
		var order []ecosystem.TestID
		for _, verdict := range classify(t, f.input).Verdicts {
			order = append(order, verdict.Test)
		}
		want := []ecosystem.TestID{"tests/b.py::test_b", "tests/a.py::test_a", "tests/c.py::test_c", "tests/0.py::test_not_sampled"}
		if !slices.Equal(order, want) {
			t.Fatalf("order = %v, want %v", order, want)
		}
	})

	t.Run("two mutants with one id are refused [verifies SP-2027~1]", func(t *testing.T) {
		f := newFixture()
		f.proven("same", line(parseFile, 1))
		f.proven("same", line(parseFile, 2))
		if _, err := vacuity.Classify(f.input); !errors.Is(err, vacuity.ErrDuplicateMutant) {
			t.Fatalf("error = %v, want ErrDuplicateMutant", err)
		}
	})
}

func TestScanTests(t *testing.T) {
	t.Run("the verdicts of the sampled green tests are written as the scan statement orders them [verifies SP-2029~2]", func(t *testing.T) {
		f := newFixture().sampled(vacuousTest).sampled(killerTest, line(totalFile, 3))
		f.fiveSurvivedOnTwoSites(vacuousTest)
		caught := f.proven("caught", line(totalFile, 3))
		f.outcome(caught, killerTest, runs(vacuity.RunFailed))
		f.input.Reference[otherTest] = runs(vacuity.RunPassed)
		tests := classify(t, f.input).ScanTests()
		if len(tests) != 2 || tests[0].ID != string(vacuousTest) || tests[1].ID != string(killerTest) {
			t.Fatalf("tests = %+v", tests)
		}
		if !slices.IsSortedFunc(tests[0].Changes, func(a, b format.ScanChange) int { return strings.Compare(a.Mutant, b.Mutant) }) {
			t.Fatalf("changes are not ordered by mutant: %+v", tests[0].Changes)
		}
	})

	t.Run("the mutants that stopped a test from starting are written apart from its changes, by id [verifies SP-2029~2]", func(t *testing.T) {
		f := newFixture().sampled(vacuousTest, line(totalFile, 3), line(totalFile, 4))
		f.fiveSurvivedOnTwoSites(vacuousTest)
		first, second := f.proven("stopped/1", line(totalFile, 3)), f.proven("stopped/2", line(totalFile, 4))
		f.outcome(first, vacuousTest, runs(vacuity.RunNotStarted)).outcome(second, vacuousTest, runs(vacuity.RunNotStarted))
		tests := classify(t, f.input).ScanTests()
		want := []string{first, second}
		slices.Sort(want)
		if len(tests) != 1 || !slices.Equal(tests[0].NotStarted, want) || len(tests[0].Changes) != vacuity.MinChanges {
			t.Fatalf("tests = %+v", tests)
		}
	})
}
