package power_test

import (
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/format"
)

func entry(facts power.Facts) power.Entry { return power.Entry{Facts: facts} }

func withID(facts power.Facts, id string) power.Facts {
	facts.ID = id
	return facts
}

func ownerKilledBy(test ...string) power.Facts {
	facts := survivor(power.OriginOperator, power.Skipped)
	facts.FailedExamples = []spec.Ref{subjectRef}
	if len(test) > 0 {
		facts = killedBy(power.OriginOperator, testOwnA)
		facts.FailedExamples = []spec.Ref{subjectRef}
	}
	return facts
}

func TestEvaluate(t *testing.T) {
	killedValidE := withID(failing(killedBy(power.OriginTargeted, testOwnA), ownWitness), "killed-valid")
	survivingValidE := withID(failing(survivor(power.OriginTargeted, power.DiffersAt), ownWitness), "surviving-valid")
	unsettled := withID(survivor(power.OriginTargeted, power.DiffersAt), "unsettled")
	excluded := withID(failing(survivor(power.OriginTargeted, power.DiffersAt), otherWitness), "excluded")
	killedLow := withID(failing(killedBy(power.OriginTargeted, testOwnA), ownWitnessLow), "killed-low")
	collateral := withID(failing(survivor(power.OriginTargeted, power.DiffersAt), ownWitness, otherWitness), "collateral")
	cases := []struct {
		name     string
		standing power.Standing
		result   power.Result
		findings []power.Finding
	}{
		{"a killed credit-bearing VALID_e and nothing else passes", standing(entry(killedValidE)), power.Pass, nil},
		{"a surviving VALID_e blocks", standing(entry(killedValidE), entry(survivingValidE)), power.Block, []power.Finding{power.FindingSurvivingValid}},
		{"a surviving collateral mutant blocks", standing(entry(killedValidE), entry(collateral)), power.Block, []power.Finding{power.FindingSurvivingValid}},
		{"a surviving owner-attributed mutant blocks", standing(entry(killedValidE), entry(withID(ownerKilledBy(), "owner"))), power.Block, []power.Finding{power.FindingSurvivingValid}},
		{"a killed owner-attributed mutant earns credit", standing(entry(withID(ownerKilledBy(string(testOwnA)), "owner"))), power.Pass, nil},
		{"an unsettled survivor escalates", standing(entry(killedValidE), entry(unsettled)), power.Escalate, []power.Finding{power.FindingUnexcludedSurvivor}},
		{"an excluded survivor does not escalate", standing(entry(killedValidE), entry(excluded)), power.Pass, nil},
		{"an empty pool escalates", standing(), power.Escalate, []power.Finding{power.FindingNoValidMutant}},
		{"a VALID_e written below the floor gives no credit", standing(entry(killedLow)), power.Escalate, []power.Finding{power.FindingNoValidMutant}},
		{"a survivor and no credit escalates on both", standing(entry(unsettled)), power.Escalate, []power.Finding{power.FindingNoValidMutant, power.FindingUnexcludedSurvivor}},
		{"blocking wins over escalating", standing(entry(survivingValidE), entry(unsettled)), power.Block, []power.Finding{power.FindingNoKilledValidMutant, power.FindingSurvivingValid, power.FindingUnexcludedSurvivor}},
	}
	for _, tc := range cases {
		t.Run(tc.name+verifiesSP167, func(t *testing.T) {
			got := power.Terminal(tc.standing, rulesOf(policy.Default()))
			if got.Result != tc.result || !slices.Equal(got.Findings, tc.findings) {
				t.Errorf("outcome %+v, want %s %v", got, tc.result, tc.findings)
			}
		})
	}

	t.Run("a reference that fails a test blocks, and so does a failing example"+verifiesSP167, func(t *testing.T) {
		failing := standing(entry(killedValidE))
		failing.CorrectCodePasses = false
		if got := power.Terminal(failing, rulesOf(policy.Default())); got.Result != power.Block || !got.Has(power.FindingCorrectCodeFails) {
			t.Errorf("outcome %+v", got)
		}
		example := standing(entry(killedValidE))
		example.ExampleFailed = true
		if got := power.Terminal(example, rulesOf(policy.Default())); got.Result != power.Block || !got.Has(power.FindingExampleFailed) {
			t.Errorf("outcome %+v", got)
		}
	})

	t.Run("a test that killed a sentinel escalates and earns no credit"+verifiesSP167, func(t *testing.T) {
		tainted := standing(entry(killedValidE))
		tainted.Subject = newSubject(testOwnA)
		got := power.Terminal(tainted, rulesOf(policy.Default()))
		if got.Result != power.Escalate || !got.Has(power.FindingMutationSensitiveTest) || !got.Has(power.FindingNoKilledValidMutant) {
			t.Errorf("outcome %+v", got)
		}
	})

	t.Run("an exhausted budget and an unresponsive host escalate"+verifiesSP167, func(t *testing.T) {
		stalled := standing(entry(killedValidE))
		stalled.BudgetExhausted, stalled.HostUnresponsive = true, true
		got := power.Terminal(stalled, rulesOf(policy.Default()))
		if got.Result != power.Escalate || !got.Has(power.FindingBudgetExhausted) || !got.Has(power.FindingHostUnresponsive) {
			t.Errorf("outcome %+v", got)
		}
	})

	t.Run("a mutant whose results are errors is neither a kill nor a survival"+verifiesSP167, func(t *testing.T) {
		errored := withID(failing(survivor(power.OriginTargeted, power.DiffersAt), ownWitness), "errored")
		errored.Outcomes[testOwnA] = format.OutcomeError
		got := power.Terminal(standing(entry(killedValidE), entry(errored)), rulesOf(policy.Default()))
		if got.Result != power.Pass {
			t.Errorf("outcome %+v", got)
		}
	})

	t.Run("the decisions of the pool are returned by mutant"+verifiesSP167, func(t *testing.T) {
		evaluation := power.Evaluate(standing(entry(killedValidE), entry(unsettled)), rulesOf(policy.Default()))
		if evaluation.Decisions["killed-valid"].Class != power.ClassValidE || evaluation.Decisions["unsettled"].Class != power.ClassUnsettled {
			t.Errorf("decisions %+v", evaluation.Decisions)
		}
	})
}

func TestResultSeverity(t *testing.T) {
	t.Run("results rank from block down to pass"+verifiesSP167, func(t *testing.T) {
		if !(power.Block.Severity() > power.Escalate.Severity() && power.Escalate.Severity() > power.Pass.Severity()) {
			t.Fatal("severity order")
		}
	})
}

func TestStopSearching(t *testing.T) {
	rules := policy.Default()
	witnessed := func(id, site string) power.Facts {
		facts := failing(killedBy(power.OriginTargeted, testOwnA), ownWitness)
		facts.ID, facts.Sites = id, []string{site}
		return facts
	}
	enough := []power.Facts{witnessed("a", siteFirst), witnessed("b", siteSecond), witnessed("c", siteThird)}
	t.Run("the search stops once enough witnessed mutants cover enough sites"+verifiesSP168, func(t *testing.T) {
		if !power.StopSearching(universe(rules), subjectRef, enough, rules) {
			t.Fatal("did not stop")
		}
	})

	t.Run("the search goes on below min_valid or min_sites"+verifiesSP168, func(t *testing.T) {
		if power.StopSearching(universe(rules), subjectRef, enough[:2], rules) {
			t.Error("stopped below min_valid")
		}
		oneSite := []power.Facts{witnessed("a", siteFirst), witnessed("b", siteFirst), witnessed("c", siteFirst)}
		if power.StopSearching(universe(rules), subjectRef, oneSite, rules) {
			t.Error("stopped below min_sites")
		}
	})

	t.Run("the stop rule reads no kill vector"+verifiesSP168, func(t *testing.T) {
		survivors := make([]power.Facts, len(enough))
		for index, facts := range enough {
			facts.Outcomes = outcomesWith()
			survivors[index] = facts
		}
		if power.StopSearching(universe(rules), subjectRef, survivors, rules) != power.StopSearching(universe(rules), subjectRef, enough, rules) {
			t.Error("the kill vector changed the stop rule")
		}
	})

	t.Run("operator mutants a witness flags do not count, owner-attributed ones do"+verifiesSP168, func(t *testing.T) {
		operators := make([]power.Facts, len(enough))
		for index, facts := range enough {
			facts.Origin = power.OriginOperator
			operators[index] = facts
		}
		if power.StopSearching(universe(rules), subjectRef, operators, rules) {
			t.Error("operator mutants stopped the search")
		}
		owners := make([]power.Facts, len(operators))
		for index, facts := range operators {
			facts.FailedExamples = []spec.Ref{subjectRef}
			owners[index] = facts
		}
		if !power.StopSearching(universe(rules), subjectRef, owners, rules) {
			t.Error("owner-attributed mutants did not stop the search")
		}
	})

	t.Run("a further witness draw is asked below two VALID_e candidates"+verifiesSP168, func(t *testing.T) {
		if !power.NeedsFurtherWitnesses(universe(rules), subjectRef, enough[:1]) || power.NeedsFurtherWitnesses(universe(rules), subjectRef, enough[:2]) {
			t.Error("further witness draw")
		}
	})

	t.Run("owner-attributed, operator and compiled-identical mutants are no VALID_e candidates"+verifiesSP168, func(t *testing.T) {
		owner, operator, identical := enough[0], enough[1], enough[1]
		owner.FailedExamples = []spec.Ref{subjectRef}
		operator.Origin = power.OriginOperator
		identical.Identity = power.IdentityIdentical
		for _, pool := range [][]power.Facts{{owner, enough[1]}, {enough[0], operator}, {enough[0], identical}} {
			if !power.NeedsFurtherWitnesses(universe(rules), subjectRef, pool) {
				t.Errorf("no further draw for %+v", pool)
			}
		}
	})

	t.Run("the further draw reads no kill vector"+verifiesSP168, func(t *testing.T) {
		killed := slices.Clone(enough[:2])
		for index := range killed {
			killed[index].Outcomes = outcomesWith(testOwnA, testOtherA)
		}
		if power.NeedsFurtherWitnesses(universe(rules), subjectRef, killed) != power.NeedsFurtherWitnesses(universe(rules), subjectRef, enough[:2]) {
			t.Error("the kill vector changed the further draw")
		}
	})
}

func TestGroupFaults(t *testing.T) {
	sameA := withID(failing(survivor(power.OriginTargeted, power.DiffersAt), ownWitness), "a")
	sameB := withID(failing(survivor(power.OriginTargeted, power.DiffersAt), ownWitness), "b")
	otherSite := withID(failing(survivor(power.OriginTargeted, power.DiffersAt), ownWitness), "c")
	otherSite.Sites = []string{siteSecond}
	sentinel := withID(survivor(power.OriginSentinel, power.NoDiffFound), "s")
	excluded := withID(failing(survivor(power.OriginTargeted, power.DiffersAt), otherWitness), "x")

	t.Run("mutants on the same sites failing the same witnesses are one fault"+verifiesSP1976, func(t *testing.T) {
		evaluation := power.Evaluate(standing(entry(sameA), entry(sameB), entry(otherSite), entry(sentinel), entry(excluded)), rulesOf(policy.Default()))
		if len(evaluation.Faults) != 2 {
			t.Fatalf("faults %+v", evaluation.Faults)
		}
		var sizes []int
		for _, fault := range evaluation.Faults {
			sizes = append(sizes, len(fault.Mutants))
		}
		slices.Sort(sizes)
		if !slices.Equal(sizes, []int{1, 2}) {
			t.Errorf("sizes %v", sizes)
		}
	})

	t.Run("mutants failing no witness group by the inputs they differ on"+verifiesSP1976, func(t *testing.T) {
		first := withID(survivor(power.OriginTargeted, power.DiffersAt), "d")
		second := withID(survivor(power.OriginTargeted, power.DiffersAt), "e")
		second.Differential.Witnesses = []power.Witness{{Call: spec.Call{Operation: "op"}}}
		evaluation := power.Evaluate(standing(entry(first), entry(second)), rulesOf(policy.Default()))
		if len(evaluation.Faults) != 2 {
			t.Errorf("faults %+v", evaluation.Faults)
		}
	})
}
