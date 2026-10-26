package vacuity

import (
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/format"
)

// Claim is a test that verifies nothing, with what confirming its claim
// re-executes: each change it cites, with the mutant's lines and, for a
// mutant only kills prove, the tests whose kills prove it.
type Claim struct {
	Test    ecosystem.TestID
	Changes []CitedChange
	// coverage is what the test ran and owns in the recording of the scan.
	coverage Coverage
}

// CitedChange is one change a claim cites.
type CitedChange struct {
	Change
	// Lines are the mutant's mutated lines.
	Lines []Line
	// Killers are the tests whose kills prove the mutant, by test id, when
	// every route that proves it is a kill; empty when another route proves
	// it.
	Killers []ecosystem.TestID
}

// KillsOnly reports whether only kills prove the change.
func (c CitedChange) KillsOnly() bool { return len(c.Killers) > 0 }

// Claims lists the claims of a result classified from the input: one for each
// test that verifies nothing, by test id, its changes ordered as its verdict
// orders them.
func Claims(input Input, result Result) ([]Claim, error) {
	mutants, err := mutantsByID(input.Mutants)
	if err != nil {
		return nil, err
	}
	var claims []Claim
	for _, verdict := range result.Verdicts {
		if verdict.Class == format.ScanVerifiesNothing {
			claims = append(claims, claimOf(verdict, input.Coverage[verdict.Test], mutants, result.Routes))
		}
	}
	slices.SortFunc(claims, func(a, b Claim) int { return strings.Compare(string(a.Test), string(b.Test)) })
	return claims, nil
}

func claimOf(verdict Verdict, coverage Coverage, mutants map[string]Mutant, routes map[string][]format.ScanRoute) Claim {
	claim := Claim{Test: verdict.Test, coverage: coverage}
	for _, change := range verdict.Changes {
		claim.Changes = append(claim.Changes, CitedChange{Change: change, Lines: mutants[change.Mutant].Lines, Killers: killersOnly(routes[change.Mutant])})
	}
	return claim
}

// killersOnly lists the killing tests of the routes, by test id, when every
// route is a kill, and nothing otherwise.
func killersOnly(routes []format.ScanRoute) []ecosystem.TestID {
	var killers []ecosystem.TestID
	for _, route := range routes {
		if route.Kind != format.ScanRouteKilledByTest {
			return nil
		}
		killers = append(killers, ecosystem.TestID(route.Test))
	}
	slices.Sort(killers)
	return slices.Compact(killers)
}

// ownedAlone are the lines the test owns both in the recording of the scan
// and in the recording of it alone.
func (c Claim) ownedAlone(alone Coverage) lineSet {
	owned, ownedAlone := setOf(c.coverage.OwnedLines()), setOf(alone.OwnedLines())
	both := lineSet{}
	for line := range owned {
		if ownedAlone[line] {
			both[line] = true
		}
	}
	return both
}

// OwnedAlone keeps the cited changes with a mutated line the test owns both in
// the recording of the scan and in the recording of it alone.
func (c Claim) OwnedAlone(alone Coverage) Claim {
	owned := c.ownedAlone(alone)
	kept := c
	kept.Changes = slices.DeleteFunc(slices.Clone(c.Changes), func(change CitedChange) bool { return !owned.holdsAny(change.Lines) })
	return kept
}

// KilledAlone keeps the cited changes that a route other than a kill proves,
// and those one of whose killers is among the confirmed killers of its
// mutant.
func (c Claim) KilledAlone(confirmed map[string][]ecosystem.TestID) Claim {
	kept := c
	kept.Changes = slices.DeleteFunc(slices.Clone(c.Changes), func(change CitedChange) bool {
		return change.KillsOnly() && !slices.ContainsFunc(confirmed[change.Mutant], func(killer ecosystem.TestID) bool {
			return slices.Contains(change.Killers, killer)
		})
	})
	return kept
}

// Confirmation is what running tests alone established about the claims of a
// scan.
type Confirmation struct {
	// Tests holds, for each test that verifies nothing, what running it alone
	// established. A test without an entry was not confirmed.
	Tests map[ecosystem.TestID]Alone
	// Killers holds, by mutant id, the killers of the mutant that, each run
	// alone, passed the reference and failed the mutant on every run.
	Killers map[string][]ecosystem.TestID
}

// Alone is what running one test with no other test selected established.
type Alone struct {
	// Coverage is the recording of the test alone.
	Coverage Coverage
	// Runs holds the runs of the test alone on each mutant, by mutant id.
	Runs map[string][]Run
}

// Confirm reclassifies each test of the result that verifies nothing on the
// changes the confirmation upholds, in the order of ADR 0014:
//
//  1. a change stays only on a line the test owns both in the recording of
//     the scan and in the recording of it alone;
//  2. a change that only kills prove stays only when one of its killers is
//     confirmed;
//  3. the outcome of each change left is the test's runs alone on it:
//     survived when every run passed, inconclusive otherwise;
//
// and the test is classified again on those changes. A test the confirmation
// holds nothing for keeps its changes, each inconclusive. Confirm only
// demotes: it changes no other verdict and no route, adds no change, and
// gives a test it reclassifies no class but verifies nothing or unproven.
func Confirm(input Input, result Result, confirmation Confirmation) (Result, error) {
	mutants, err := mutantsByID(input.Mutants)
	if err != nil {
		return Result{}, err
	}
	confirmed := Result{Routes: result.Routes, Verdicts: slices.Clone(result.Verdicts)}
	for index, verdict := range confirmed.Verdicts {
		if verdict.Class != format.ScanVerifiesNothing {
			continue
		}
		claim := claimOf(verdict, input.Coverage[verdict.Test], mutants, result.Routes)
		alone, held := confirmation.Tests[verdict.Test]
		confirmed.Verdicts[index] = claim.confirmed(verdict, alone, held, confirmation.Killers, mutants)
	}
	confirmed.Counts = countsOf(confirmed.Verdicts)
	slices.SortFunc(confirmed.Verdicts, compareVerdicts)
	return confirmed, nil
}

// confirmed is the verdict of the claim's test on the changes the
// confirmation upholds.
func (c Claim) confirmed(verdict Verdict, alone Alone, held bool, killers map[string][]ecosystem.TestID, mutants map[string]Mutant) Verdict {
	reclassified := Verdict{Test: verdict.Test, NotStarted: verdict.NotStarted}
	if !held {
		for _, change := range verdict.Changes {
			reclassified.Changes = append(reclassified.Changes, Change{Mutant: change.Mutant, Site: change.Site, Outcome: format.ScanInconclusive})
		}
		reclassified.Class, reclassified.Reason = classOf(false, reclassified)
		return reclassified
	}
	owned := c.ownedAlone(alone.Coverage)
	reclassified.NotStarted = slices.DeleteFunc(slices.Clone(verdict.NotStarted), func(id string) bool { return !owned.holdsAny(mutants[id].Lines) })
	for _, change := range c.OwnedAlone(alone.Coverage).KilledAlone(killers).Changes {
		outcome := format.ScanInconclusive
		if Settle(alone.Runs[change.Mutant]) == SettledSurvived {
			outcome = format.ScanSurvived
		}
		reclassified.Changes = append(reclassified.Changes, Change{Mutant: change.Mutant, Site: change.Site, Outcome: outcome})
	}
	reclassified.Class, reclassified.Reason = classOf(false, reclassified)
	return reclassified
}
