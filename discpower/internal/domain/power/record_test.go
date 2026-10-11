package power_test

import (
	"errors"
	"reflect"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/witness"
)

func referencePassing() map[ecosystem.TestID]ecosystem.TestOutcome {
	reference := map[ecosystem.TestID]ecosystem.TestOutcome{}
	for _, test := range allTests() {
		reference[test] = ecosystem.TestPassed
	}
	return reference
}

func TestRecord(t *testing.T) {
	t.Run("the outcome recomputed from a decoded record equals the one evaluated on its standing"+verifiesSP1977, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			rules := rulesOf(genRules().Draw(t, "rules"))
			current := standing(genEntries().Draw(t, "entries")...)
			var sentinels []power.Facts
			for _, item := range current.Entries {
				if item.Facts.Origin == power.OriginSentinel {
					sentinels = append(sentinels, item.Facts)
				}
			}
			current.Subject = power.NewSubject(subjectRef, bindings(), power.SentinelKillers(sentinels))
			encoded, err := power.NewRecord(current, rules, referencePassing()).Encode()
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := power.DecodeRecord(encoded)
			if err != nil {
				t.Fatal(err)
			}
			rebuilt := decoded.StandingFor(decoded.Bindings)
			want := power.Evaluate(current, rules)
			got := power.Evaluate(rebuilt, decoded.Rules(""))
			if !reflect.DeepEqual(got.Outcome, want.Outcome) {
				t.Fatalf("outcome %+v, want %+v", got.Outcome, want.Outcome)
			}
		})
	})

	t.Run("another binding of the tests gives the outcome of that test set"+verifiesSP1977, func(t *testing.T) {
		killed := withID(failing(killedBy(power.OriginTargeted, testOtherA), ownWitness), "k")
		record := power.NewRecord(standing(entry(killed)), rulesOf(policy.Default()), referencePassing())
		natural := power.Terminal(record.StandingFor(record.Bindings), record.Rules(""))
		transplanted := bindings()
		transplanted[testOtherA] = []spec.Ref{subjectRef}
		moved := power.Terminal(record.StandingFor(transplanted), record.Rules(""))
		if natural.Result == moved.Result || moved.Result != power.Pass {
			t.Errorf("natural %+v, transplanted %+v", natural, moved)
		}
	})

	t.Run("the admission can be replaced when the outcome is recomputed"+verifiesSP1977, func(t *testing.T) {
		dropped := universeRecords()
		dropped[0].Refactors = map[string]witness.Outcome{"R": witness.OutcomeFailed}
		current := standing(entry(withID(failing(killedBy(power.OriginTargeted, testOwnA), ownWitness), "k")))
		current.Witnesses = dropped
		record := power.NewRecord(current, power.Rules{Policy: policy.Default(), Admission: witness.AdmissionWithRefactors}, referencePassing())
		primary := power.Terminal(record.StandingFor(record.Bindings), record.Rules(""))
		shipped := power.Terminal(record.StandingFor(record.Bindings), record.Rules(witness.AdmissionReferenceOnly))
		if primary.Result != power.Escalate || shipped.Result != power.Pass {
			t.Errorf("primary %+v, shipped %+v", primary, shipped)
		}
	})

	t.Run("a record of another version is refused"+verifiesSP1977, func(t *testing.T) {
		if _, err := power.DecodeRecord([]byte(`{"version":99}`)); !errors.Is(err, power.ErrRecordVersion) {
			t.Errorf("error %v", err)
		}
	})

	t.Run("a test of the set that fails on the reference blocks the recomputed outcome"+verifiesSP1977, func(t *testing.T) {
		reference := referencePassing()
		reference[testOwnB] = ecosystem.TestFailed
		record := power.NewRecord(standing(), rulesOf(policy.Default()), reference)
		if got := power.Terminal(record.StandingFor(record.Bindings), record.Rules("")); !got.Has(power.FindingCorrectCodeFails) {
			t.Errorf("outcome %+v", got)
		}
	})
}
