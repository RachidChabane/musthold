package power_test

import (
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
)

func entryOf(facts power.Facts, class power.Class) power.Entry {
	return power.Entry{Facts: facts, Decision: power.Decision{Class: class}}
}

func validKilled() power.Entry {
	return entryOf(killedBy(power.OriginTargeted, testOwnA), power.ClassValid)
}

func validKilledAt(site string) power.Entry {
	entry := validKilled()
	entry.Facts.Sites = []string{site}
	return entry
}

func validSurviving() power.Entry {
	return entryOf(survivor(power.OriginTargeted, power.Skipped), power.ClassValid)
}

func undeterminedSurvivor() power.Entry {
	return entryOf(survivor(power.OriginTargeted, power.Skipped), power.ClassUndetermined)
}

func healthy(entries ...power.Entry) power.Standing {
	return power.Standing{Subject: newSubject(), CorrectCodePasses: true, Entries: entries}
}

func TestTerminal(t *testing.T) {
	collateralKilled := entryOf(killedBy(power.OriginTargeted, testOwnB), power.ClassCollateral)
	collateralSurviving := entryOf(survivor(power.OriginTargeted, power.Skipped), power.ClassCollateral)
	survivingSentinel := entryOf(survivor(power.OriginSentinel, power.NoDiffFound), power.ClassEquivalent)
	undeterminedSentinel := entryOf(survivor(power.OriginSentinel, power.NoDiffFound), power.ClassUndetermined)
	survivingEquivalent := entryOf(survivor(power.OriginTargeted, power.NoDiffFound), power.ClassEquivalent)
	survivingOffTarget := entryOf(survivor(power.OriginOperator, power.Skipped), power.ClassOffTarget)
	survivingUnattributed := entryOf(survivor(power.OriginTargeted, power.Skipped), power.ClassUnattributed)
	killedEquivalent := entryOf(killedBy(power.OriginTargeted, testOwnA), power.ClassEquivalent)
	killedUndetermined := entryOf(killedBy(power.OriginTargeted, testOwnA), power.ClassUndetermined)

	cases := []struct {
		name     string
		standing power.Standing
		result   power.Result
		findings []power.Finding
	}{
		{"a killed VALID mutant and nothing else", healthy(validKilled()), power.Pass, nil},
		{"a killed collateral mutant is a credited VALID mutant", healthy(collateralKilled), power.Pass, nil},
		{"a credited VALID mutant with excluded survivors", healthy(validKilled(), survivingEquivalent, survivingOffTarget, survivingUnattributed, survivingSentinel), power.Pass, nil},
		{"a killed mutant of any decision is not a survivor", healthy(validKilled(), killedEquivalent, killedUndetermined), power.Pass, nil},
		{"a failing reference", power.Standing{Subject: newSubject(), CorrectCodePasses: false, Entries: []power.Entry{validKilled()}}, power.Block, []power.Finding{power.FindingCorrectCodeFails}},
		{"a failing example", power.Standing{Subject: newSubject(), CorrectCodePasses: true, ExampleFailed: true, Entries: []power.Entry{validKilled()}}, power.Block, []power.Finding{power.FindingExampleFailed}},
		{"a failing reference and a failing example", power.Standing{Subject: newSubject(), ExampleFailed: true}, power.Block, []power.Finding{power.FindingCorrectCodeFails, power.FindingExampleFailed}},
		{"a failing reference leaves out the credit findings", power.Standing{Subject: newSubject()}, power.Block, []power.Finding{power.FindingCorrectCodeFails}},
		{"a surviving VALID mutant", healthy(validKilled(), validSurviving()), power.Block, []power.Finding{power.FindingSurvivingValid}},
		{"a surviving collateral mutant", healthy(validKilled(), collateralSurviving), power.Block, []power.Finding{power.FindingSurvivingValid}},
		{"a surviving VALID mutant with nothing credited", healthy(validSurviving()), power.Block, []power.Finding{power.FindingNoKilledValidMutant, power.FindingSurvivingValid}},
		{"a surviving VALID mutant blocks before an undetermined survivor escalates", healthy(validKilled(), validSurviving(), undeterminedSurvivor()), power.Block, []power.Finding{power.FindingSurvivingValid, power.FindingUndeterminedSurvivor}},
		{"a block blocks before an exhausted budget escalates", power.Standing{Subject: newSubject(), CorrectCodePasses: true, Entries: []power.Entry{validKilled(), validSurviving()}, BudgetExhausted: true}, power.Block, []power.Finding{power.FindingBudgetExhausted, power.FindingSurvivingValid}},
		{"an undetermined survivor", healthy(validKilled(), undeterminedSurvivor()), power.Escalate, []power.Finding{power.FindingUndeterminedSurvivor}},
		{"an undetermined survivor of the operators", healthy(validKilled(), entryOf(survivor(power.OriginOperator, power.Skipped), power.ClassUndetermined)), power.Escalate, []power.Finding{power.FindingUndeterminedSurvivor}},
		{"no mutant at all", healthy(), power.Escalate, []power.Finding{power.FindingNoValidMutant}},
		{"only excluded mutants", healthy(survivingEquivalent, survivingOffTarget), power.Escalate, []power.Finding{power.FindingNoValidMutant}},
		{"a sentinel is no VALID mutant", healthy(survivingSentinel), power.Escalate, []power.Finding{power.FindingNoValidMutant}},
		{"an undetermined sentinel raises no finding", healthy(validKilled(), undeterminedSentinel), power.Pass, nil},
		{"a killed VALID mutant whose killer also killed a sentinel", power.Standing{Subject: newSubject(testOtherA), CorrectCodePasses: true, Entries: []power.Entry{entryOf(killedBy(power.OriginTargeted, testOwnA, testOtherA), power.ClassValid)}}, power.Pass, nil},
		{"a killed VALID mutant killed only by a sentinel killer", power.Standing{Subject: newSubject(testOwnA), CorrectCodePasses: true, Entries: []power.Entry{validKilled()}}, power.Escalate, []power.Finding{power.FindingMutationSensitiveTest, power.FindingNoKilledValidMutant}},
		{"a test of the criterion that killed a sentinel", power.Standing{Subject: newSubject(testOwnB), CorrectCodePasses: true, Entries: []power.Entry{validKilled()}}, power.Escalate, []power.Finding{power.FindingMutationSensitiveTest}},
		{"a sentinel killer outside the criterion", power.Standing{Subject: newSubject(testOtherA), CorrectCodePasses: true, Entries: []power.Entry{validKilled()}}, power.Pass, nil},
		{"an exhausted budget", power.Standing{Subject: newSubject(), CorrectCodePasses: true, Entries: []power.Entry{validKilled()}, BudgetExhausted: true}, power.Escalate, []power.Finding{power.FindingBudgetExhausted}},
		{"an unresponsive host", power.Standing{Subject: newSubject(), CorrectCodePasses: true, Entries: []power.Entry{validKilled()}, HostUnresponsive: true}, power.Escalate, []power.Finding{power.FindingHostUnresponsive}},
		{"every ground to escalate", power.Standing{Subject: newSubject(testOwnB), CorrectCodePasses: true, Entries: []power.Entry{undeterminedSurvivor()}, BudgetExhausted: true, HostUnresponsive: true}, power.Escalate, []power.Finding{
			power.FindingBudgetExhausted, power.FindingHostUnresponsive, power.FindingMutationSensitiveTest, power.FindingNoValidMutant, power.FindingUndeterminedSurvivor}},
	}
	for _, tc := range cases {
		t.Run(tc.name+verifiesSP167, func(t *testing.T) {
			got := power.Terminal(tc.standing)
			if got.Result != tc.result {
				t.Fatalf("result %q (%v), want %q", got.Result, got.Findings, tc.result)
			}
			if !slices.Equal(got.Findings, tc.findings) {
				t.Errorf("findings %v, want %v", got.Findings, tc.findings)
			}
			for _, finding := range tc.findings {
				if !got.Has(finding) {
					t.Errorf("Has(%q) = false", finding)
				}
			}
			if got.Has("unlisted") {
				t.Error("Has reported an unknown finding")
			}
		})
	}
	t.Run("findings are sorted and without duplicates"+verifiesSP167, func(t *testing.T) {
		got := power.Terminal(healthy(validSurviving(), validSurviving(), undeterminedSurvivor(), undeterminedSurvivor()))
		if !slices.IsSorted(got.Findings) || len(slices.Compact(slices.Clone(got.Findings))) != len(got.Findings) {
			t.Errorf("findings %v", got.Findings)
		}
	})
	t.Run("a pass carries no finding"+verifiesSP167, func(t *testing.T) {
		if got := power.Terminal(healthy(validKilled())); len(got.Findings) != 0 {
			t.Errorf("findings %v", got.Findings)
		}
	})
}

func TestResultSeverity(t *testing.T) {
	t.Run("results rank from block down to pass"+verifiesSP167, func(t *testing.T) {
		if !(power.Block.Severity() > power.Escalate.Severity() && power.Escalate.Severity() > power.Pass.Severity()) {
			t.Errorf("severities %d %d %d", power.Block.Severity(), power.Escalate.Severity(), power.Pass.Severity())
		}
	})
	t.Run("an unknown result ranks as a pass"+verifiesSP167, func(t *testing.T) {
		if power.Result("OTHER").Severity() != power.Pass.Severity() {
			t.Error("an unknown result outranks a pass")
		}
	})
}

func TestStopSearching(t *testing.T) {
	rules := policy.Default()
	cases := []struct {
		name     string
		standing power.Standing
		rules    policy.Policy
		stop     bool
	}{
		{"an empty pool", healthy(), rules, false},
		{"a surviving VALID mutant", healthy(validSurviving()), rules, true},
		{"a surviving collateral mutant", healthy(entryOf(survivor(power.OriginTargeted, power.Skipped), power.ClassCollateral)), rules, true},
		{"a surviving VALID mutant among killed ones that are too few", healthy(validKilledAt(siteFirst), validSurviving()), rules, true},
		{"a failing reference", power.Standing{Subject: newSubject(), CorrectCodePasses: false}, rules, true},
		{"a failing example", power.Standing{Subject: newSubject(), CorrectCodePasses: true, ExampleFailed: true}, rules, true},
		{"too few VALID mutants", healthy(validKilledAt(siteFirst), validKilledAt(siteSecond)), rules, false},
		{"enough VALID mutants on one site only", healthy(validKilledAt(siteFirst), validKilledAt(siteFirst), validKilledAt(siteFirst)), rules, false},
		{"min_valid VALID mutants on min_sites sites with all killed", healthy(validKilledAt(siteFirst), validKilledAt(siteFirst), validKilledAt(siteSecond)), rules, true},
		{"more than enough on many sites", healthy(validKilledAt(siteFirst), validKilledAt(siteSecond), validKilledAt(siteThird), validKilledAt(siteThird)), rules, true},
		{"enough killed mutants whose other decisions do not count", healthy(validKilledAt(siteFirst), validKilledAt(siteSecond), undeterminedSurvivor(), entryOf(survivor(power.OriginTargeted, power.NoDiffFound), power.ClassEquivalent)), rules, false},
		{"a mutant editing several sites counts every one", healthy(func() power.Entry {
			entry := validKilled()
			entry.Facts.Sites = []string{siteFirst, siteSecond}
			return entry
		}(), validKilledAt(siteFirst), validKilledAt(siteFirst)), rules, true},
		{"a lowered min_valid", healthy(validKilledAt(siteFirst), validKilledAt(siteSecond)), func() policy.Policy {
			lowered := policy.Default()
			lowered.MinValid = 2
			return lowered
		}(), true},
		{"a raised min_sites", healthy(validKilledAt(siteFirst), validKilledAt(siteSecond), validKilledAt(siteSecond)), func() policy.Policy {
			raised := policy.Default()
			raised.MinSites = 3
			return raised
		}(), false},
		{"a block even with nothing else", power.Standing{Subject: newSubject(), CorrectCodePasses: false, Entries: []power.Entry{validKilledAt(siteFirst)}}, rules, true},
	}
	for _, tc := range cases {
		t.Run(tc.name+verifiesSP168, func(t *testing.T) {
			if got := power.StopSearching(tc.standing, tc.rules); got != tc.stop {
				t.Errorf("StopSearching() = %v, want %v", got, tc.stop)
			}
		})
	}
	t.Run("a pool of VALID mutants with none killed cannot reach the quota without blocking"+verifiesSP168, func(t *testing.T) {
		standing := healthy(validSurviving(), validSurviving(), validSurviving())
		if !power.StopSearching(standing, rules) || power.Terminal(standing).Result != power.Block {
			t.Error("the search continues past a block")
		}
	})
}

func TestTerminalAgreesWithStop(t *testing.T) {
	t.Run("a pool that stops the search by its quota passes when nothing remains"+verifiesSP168, func(t *testing.T) {
		standing := healthy(validKilledAt(siteFirst), validKilledAt(siteFirst), validKilledAt(siteSecond))
		if !power.StopSearching(standing, policy.Default()) || power.Terminal(standing).Result != power.Pass {
			t.Error("the quota did not end in a pass")
		}
	})
	t.Run("sentinel decisions never count towards the quota"+verifiesSP168, func(t *testing.T) {
		sentinel := entryOf(killedBy(power.OriginSentinel, testOwnA), power.ClassEquivalent)
		standing := healthy(sentinel, sentinel, sentinel)
		if power.StopSearching(standing, policy.Default()) {
			t.Error("sentinels ended the search")
		}
	})
}
