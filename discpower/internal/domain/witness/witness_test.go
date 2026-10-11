package witness_test

import (
	"fmt"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/witness"
	"github.com/RachidChabane/spec-proof/format"
)

const (
	verifiesSP1960 = " [verifies SP-1960~1]"
	verifiesSP1961 = " [verifies SP-1961~1]"

	refactorName   = "rename-local"
	otherRefactor  = "extract-helper"
	judgeThreshold = 0.8
	scopeVotes     = 2
	digestLength   = 64
	maxSetSize     = 12
)

var (
	criterion      = spec.Ref{ID: "SP-7", Revision: 1}
	otherCriterion = spec.Ref{ID: "SP-8", Revision: 1}
	rule           = witness.ScopeRule{Votes: scopeVotes, Threshold: judgeThreshold}

	passes   = []witness.Outcome{witness.OutcomePassed, witness.OutcomePassed, witness.OutcomePassed}
	conflict = []witness.Outcome{witness.OutcomeFailed, witness.OutcomeFailed, witness.OutcomeFailed}
	flaky    = []witness.Outcome{witness.OutcomePassed, witness.OutcomeFailed, witness.OutcomePassed}
)

func fixed(confidence float64, isolation format.Isolation) witness.ScopeVote {
	return witness.ScopeVote{Answer: witness.ScopeFixed, Confidence: confidence, Isolation: isolation}
}

func exampleWitness(name, code string) witness.Witness {
	return witness.New(criterion, name, "the fee is charged", witness.KindExample, code)
}

func setOf(examples, properties int) witness.Set {
	set := witness.Set{Criterion: criterion, Imports: "import math"}
	for index := range examples {
		set.Witnesses = append(set.Witnesses, witness.New(criterion, fmt.Sprintf("example_%d", index), "clause", witness.KindExample, "code"))
	}
	for index := range properties {
		set.Witnesses = append(set.Witnesses, witness.New(criterion, fmt.Sprintf("property_%d", index), "clause", witness.KindProperty, "code"))
	}
	return set
}

func record(reference []witness.Outcome, refactors map[string]witness.Outcome, votes ...witness.ScopeVote) witness.Record {
	return witness.Record{
		Witness:   exampleWitness("w", "code"),
		Writer:    witness.Writer{Isolation: format.IsolationRestrictedAttested},
		Reference: reference,
		Refactors: refactors,
		Scope:     votes,
	}
}

func TestReadingOf(t *testing.T) {
	for _, c := range []struct {
		name string
		runs []witness.Outcome
		want witness.Reading
	}{
		{"three passes pass", passes, witness.ReadingPasses},
		{"three failures are a reading conflict", conflict, witness.ReadingConflict},
		{"a pass and a failure are flaky", flaky, witness.ReadingFlaky},
		{"one failure among passes is flaky", []witness.Outcome{witness.OutcomeFailed, witness.OutcomePassed, witness.OutcomePassed}, witness.ReadingFlaky},
		{"an error is unrunnable", []witness.Outcome{witness.OutcomePassed, witness.OutcomeErrored, witness.OutcomePassed}, witness.ReadingUnrunnable},
		{"an error wins over a conflict", []witness.Outcome{witness.OutcomeFailed, witness.OutcomeFailed, witness.OutcomeErrored}, witness.ReadingUnrunnable},
		{"a flaky outcome is unrunnable", []witness.Outcome{witness.OutcomePassed, witness.OutcomeFlaky, witness.OutcomePassed}, witness.ReadingUnrunnable},
		{"two runs are unrunnable", passes[:2], witness.ReadingUnrunnable},
		{"one run is unrunnable", conflict[:1], witness.ReadingUnrunnable},
		{"no run is unrunnable", nil, witness.ReadingUnrunnable},
		{"more than three passes pass", append(slices.Clone(passes), witness.OutcomePassed), witness.ReadingPasses},
	} {
		t.Run(c.name+verifiesSP1960, func(t *testing.T) {
			if got := witness.ReadingOf(c.runs); got != c.want {
				t.Errorf("got %s, want %s", got, c.want)
			}
		})
	}
	outcomes := []witness.Outcome{witness.OutcomePassed, witness.OutcomeFailed, witness.OutcomeFlaky, witness.OutcomeErrored}
	t.Run("a reading follows the runs whatever they are"+verifiesSP1960, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			runs := rapid.SliceOfN(rapid.SampledFrom(outcomes), 0, witness.ReferenceRuns+2).Draw(t, "runs")
			got := witness.ReadingOf(runs)
			switch {
			case len(runs) < witness.ReferenceRuns || slices.ContainsFunc(runs, func(o witness.Outcome) bool {
				return o == witness.OutcomeErrored || o == witness.OutcomeFlaky
			}):
				if got != witness.ReadingUnrunnable {
					t.Fatalf("%v read %s", runs, got)
				}
			case !slices.Contains(runs, witness.OutcomeFailed):
				if got != witness.ReadingPasses {
					t.Fatalf("%v read %s", runs, got)
				}
			case !slices.Contains(runs, witness.OutcomePassed):
				if got != witness.ReadingConflict {
					t.Fatalf("%v read %s", runs, got)
				}
			default:
				if got != witness.ReadingFlaky {
					t.Fatalf("%v read %s", runs, got)
				}
			}
		})
	})
}

func TestRecordStatus(t *testing.T) {
	approving := []witness.ScopeVote{fixed(1, format.IsolationRestricted), fixed(judgeThreshold, format.IsolationInline)}
	failedRefactor := map[string]witness.Outcome{refactorName: witness.OutcomePassed, otherRefactor: witness.OutcomeFailed}
	for _, c := range []struct {
		name      string
		record    witness.Record
		admission witness.Admission
		want      witness.Status
	}{
		{"a record with fewer than three runs is unrunnable", record(passes[:2], nil), witness.AdmissionReferenceOnly, witness.StatusUnrunnable},
		{"an errored run is unrunnable", record([]witness.Outcome{witness.OutcomeErrored, witness.OutcomePassed, witness.OutcomePassed}, nil), witness.AdmissionWithRefactors, witness.StatusUnrunnable},
		{"disagreeing runs are flaky", record(flaky, nil), witness.AdmissionWithRefactors, witness.StatusFlaky},
		{"three failures are a reading conflict", record(conflict, nil), witness.AdmissionReferenceOnly, witness.StatusReadingConflict},
		{"a refactor failure drops the witness when refactors admit", record(passes, failedRefactor, approving...), witness.AdmissionWithRefactors, witness.StatusRefactorDropped},
		{"a refactor failure is ignored under reference-only admission", record(passes, failedRefactor, approving...), witness.AdmissionReferenceOnly, witness.StatusBlockBearing},
		{"every vote FIXED at the threshold makes it block-bearing", record(passes, nil, approving...), witness.AdmissionWithRefactors, witness.StatusBlockBearing},
		{"a passed refactor keeps it block-bearing", record(passes, map[string]witness.Outcome{refactorName: witness.OutcomePassed}, approving...), witness.AdmissionWithRefactors, witness.StatusBlockBearing},
		{"a vote below the threshold makes it escalate-only", record(passes, nil, fixed(1, format.IsolationRestricted), fixed(judgeThreshold-0.01, format.IsolationRestricted)), witness.AdmissionWithRefactors, witness.StatusEscalateOnly},
		{"a NOT_FIXED vote makes it escalate-only", record(passes, nil, fixed(1, format.IsolationRestricted), witness.ScopeVote{Answer: witness.ScopeNotFixed, Confidence: 1}), witness.AdmissionWithRefactors, witness.StatusEscalateOnly},
		{"an UNDETERMINED vote makes it escalate-only", record(passes, nil, fixed(1, format.IsolationRestricted), witness.ScopeVote{Answer: witness.ScopeUndetermined, Confidence: 1}), witness.AdmissionWithRefactors, witness.StatusEscalateOnly},
		{"too few votes make it escalate-only", record(passes, nil, fixed(1, format.IsolationRestricted)), witness.AdmissionWithRefactors, witness.StatusEscalateOnly},
		{"no vote makes it escalate-only", record(passes, nil), witness.AdmissionReferenceOnly, witness.StatusEscalateOnly},
		{"a vote of a weak isolation still counts toward a block", record(passes, nil, fixed(1, format.IsolationInline), fixed(1, format.IsolationUnrestricted)), witness.AdmissionWithRefactors, witness.StatusBlockBearing},
		{"a marked vote still counts toward a block", record(passes, nil, fixed(1, format.IsolationRestricted), witness.ScopeVote{Answer: witness.ScopeFixed, Confidence: 1, Marked: true}), witness.AdmissionWithRefactors, witness.StatusBlockBearing},
		{"more votes than asked are all required to be FIXED", record(passes, nil, fixed(1, format.IsolationRestricted), fixed(1, format.IsolationRestricted), witness.ScopeVote{Answer: witness.ScopeNotFixed, Confidence: 1}), witness.AdmissionWithRefactors, witness.StatusEscalateOnly},
	} {
		t.Run(c.name+verifiesSP1960, func(t *testing.T) {
			if got := c.record.Status(c.admission, rule); got != c.want {
				t.Errorf("got %s, want %s", got, c.want)
			}
		})
	}
	t.Run("a rule asking for no vote approves nothing"+verifiesSP1960, func(t *testing.T) {
		got := record(passes, nil, approving...).Status(witness.AdmissionReferenceOnly, witness.ScopeRule{Votes: 0, Threshold: 0})
		if got != witness.StatusEscalateOnly {
			t.Errorf("got %s", got)
		}
	})
	t.Run("a block-bearing witness is admitted and every vote is FIXED at the threshold"+verifiesSP1960, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			answers := []witness.ScopeAnswer{witness.ScopeFixed, witness.ScopeNotFixed, witness.ScopeUndetermined}
			votes := rapid.SliceOfN(rapid.Custom(func(t *rapid.T) witness.ScopeVote {
				return witness.ScopeVote{
					Answer:     rapid.SampledFrom(answers).Draw(t, "answer"),
					Confidence: rapid.Float64Range(0, 1).Draw(t, "confidence"),
				}
			}), 0, scopeVotes+1).Draw(t, "votes")
			subject := record(passes, nil, votes...)
			if subject.Status(witness.AdmissionWithRefactors, rule) != witness.StatusBlockBearing {
				return
			}
			if !subject.Admitted(witness.AdmissionWithRefactors) || len(votes) < scopeVotes {
				t.Fatal("a block-bearing witness is not admitted or lacks votes")
			}
			for _, vote := range votes {
				if vote.Answer != witness.ScopeFixed || vote.Confidence < judgeThreshold {
					t.Fatalf("the vote %+v does not approve", vote)
				}
			}
		})
	})
}

func TestAdmitted(t *testing.T) {
	passed := map[string]witness.Outcome{refactorName: witness.OutcomePassed}
	failed := map[string]witness.Outcome{refactorName: witness.OutcomePassed, otherRefactor: witness.OutcomeFailed}
	errored := map[string]witness.Outcome{refactorName: witness.OutcomeErrored}
	for _, c := range []struct {
		name      string
		record    witness.Record
		admission witness.Admission
		want      bool
	}{
		{"a passing witness is admitted by every refactor it passes", record(passes, passed), witness.AdmissionWithRefactors, true},
		{"a passing witness is admitted when no refactor is named", record(passes, nil), witness.AdmissionWithRefactors, true},
		{"a witness failing a refactor is not admitted with refactors", record(passes, failed), witness.AdmissionWithRefactors, false},
		{"a witness erroring on a refactor is not admitted with refactors", record(passes, errored), witness.AdmissionWithRefactors, false},
		{"a witness failing a refactor is admitted reference-only", record(passes, failed), witness.AdmissionReferenceOnly, true},
		{"a flaky witness is not admitted", record(flaky, nil), witness.AdmissionReferenceOnly, false},
		{"a conflicting witness is not admitted", record(conflict, nil), witness.AdmissionReferenceOnly, false},
		{"an unrunnable witness is not admitted", record(passes[:1], nil), witness.AdmissionReferenceOnly, false},
	} {
		t.Run(c.name+verifiesSP1960, func(t *testing.T) {
			if got := c.record.Admitted(c.admission); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
	t.Run("the reading and the refactor check of a record follow its runs"+verifiesSP1960, func(t *testing.T) {
		subject := record(passes, failed)
		if subject.Reading() != witness.ReadingPasses || subject.PassesRefactors() {
			t.Errorf("reading %s, passes refactors %v", subject.Reading(), subject.PassesRefactors())
		}
		if !record(passes, nil).PassesRefactors() {
			t.Error("a record with no refactor failed one")
		}
	})
}

func TestCreditEligible(t *testing.T) {
	floor := format.IsolationRestricted
	strong := fixed(1, format.IsolationRestrictedAttested)
	weak := fixed(1, format.IsolationUnrestricted)
	marked := witness.ScopeVote{Answer: witness.ScopeFixed, Confidence: 1, Isolation: format.IsolationRestricted, Marked: true}
	atFloor := fixed(1, floor)
	for _, c := range []struct {
		name   string
		writer witness.Writer
		votes  []witness.ScopeVote
		want   bool
	}{
		{"a writer and judges at the floor are eligible", witness.Writer{Isolation: floor}, []witness.ScopeVote{atFloor, strong}, true},
		{"a witness with no scope vote and a writer at the floor is eligible", witness.Writer{Isolation: floor}, nil, true},
		{"a writer below the floor is not eligible", witness.Writer{Isolation: format.IsolationInline}, []witness.ScopeVote{strong}, false},
		{"a marked writer is not eligible", witness.Writer{Isolation: floor, Marked: true}, []witness.ScopeVote{strong}, false},
		{"a judge below the floor is not eligible", witness.Writer{Isolation: floor}, []witness.ScopeVote{strong, weak}, false},
		{"a marked judge is not eligible", witness.Writer{Isolation: floor}, []witness.ScopeVote{strong, marked}, false},
		{"an unknown writer isolation is not eligible", witness.Writer{Isolation: "invented"}, nil, false},
	} {
		t.Run(c.name+verifiesSP1960, func(t *testing.T) {
			subject := record(passes, nil, c.votes...)
			subject.Writer = c.writer
			if got := subject.CreditEligible(floor); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
	t.Run("a witness not credit-eligible keeps its power to block"+verifiesSP1960, func(t *testing.T) {
		subject := record(passes, nil, fixed(1, format.IsolationInline), fixed(1, format.IsolationInline))
		subject.Writer = witness.Writer{Isolation: format.IsolationInline}
		if subject.CreditEligible(floor) || subject.Status(witness.AdmissionWithRefactors, rule) != witness.StatusBlockBearing {
			t.Error("eligibility and blocking are not independent")
		}
	})
	t.Run("a writer and a vote answer at the floor exactly when unmarked and strong enough"+verifiesSP1960, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			level := rapid.SampledFrom(format.Isolations()).Draw(t, "level")
			isMarked := rapid.Bool().Draw(t, "marked")
			want := !isMarked && level.AtLeast(floor)
			if got := (witness.Writer{Isolation: level, Marked: isMarked}).AtFloor(floor); got != want {
				t.Fatalf("writer at %s marked %v: %v", level, isMarked, got)
			}
			if got := (witness.ScopeVote{Isolation: level, Marked: isMarked}).AtFloor(floor); got != want {
				t.Fatalf("vote at %s marked %v: %v", level, isMarked, got)
			}
		})
	})
}

func TestSetDigest(t *testing.T) {
	base := setOf(witness.MinExamples, 1)
	t.Run("two sets with the same content and different writers have the same digest"+verifiesSP1961, func(t *testing.T) {
		other := base
		other.Writer = witness.Writer{RequestID: "r-2", Vote: 2, Draw: 2, Isolation: format.IsolationInline, Marked: true}
		if base.Digest() != other.Digest() {
			t.Error("the writer's provenance changed the digest")
		}
	})
	t.Run("the digest is a sha-256 and is stable"+verifiesSP1961, func(t *testing.T) {
		if len(base.Digest()) != digestLength || base.Digest() != base.Digest() {
			t.Errorf("digest %q", base.Digest())
		}
	})
	t.Run("the digest changes with the content"+verifiesSP1961, func(t *testing.T) {
		changedImports := base
		changedImports.Imports = "import decimal"
		changedCriterion := base
		changedCriterion.Criterion = otherCriterion
		changedWitness := setOf(witness.MinExamples, 1)
		changedWitness.Witnesses[0] = exampleWitness("example_0", "other code")
		extra := setOf(witness.MinExamples+1, 1)
		for name, changed := range map[string]witness.Set{"imports": changedImports, "criterion": changedCriterion, "a witness": changedWitness, "another witness": extra} {
			if changed.Digest() == base.Digest() {
				t.Errorf("a change of %s keeps the digest", name)
			}
		}
	})
	t.Run("the digest of a witness changes with each of its parts"+verifiesSP1961, func(t *testing.T) {
		original := witness.New(criterion, "n", "clause", witness.KindExample, "code")
		for name, changed := range map[string]witness.Witness{
			"criterion": witness.New(otherCriterion, "n", "clause", witness.KindExample, "code"),
			"name":      witness.New(criterion, "m", "clause", witness.KindExample, "code"),
			"clause":    witness.New(criterion, "n", "other", witness.KindExample, "code"),
			"kind":      witness.New(criterion, "n", "clause", witness.KindProperty, "code"),
			"code":      witness.New(criterion, "n", "clause", witness.KindExample, "other"),
		} {
			if changed.ID == original.ID {
				t.Errorf("a change of the %s keeps the id", name)
			}
		}
		if again := witness.New(criterion, "n", "clause", witness.KindExample, "code"); again.ID != original.ID || len(original.ID) != digestLength {
			t.Errorf("id %q is not stable", original.ID)
		}
		if original.ShortID() != original.ID[:len(original.ShortID())] || original.ShortID() == "" {
			t.Errorf("short id %q", original.ShortID())
		}
	})
	t.Run("a new witness changes its id with its code"+verifiesSP1961, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			first := rapid.String().Draw(t, "first")
			second := rapid.String().Filter(func(code string) bool { return code != first }).Draw(t, "second")
			if exampleWitness("n", first).ID == exampleWitness("n", second).ID {
				t.Fatal("two codes share an id")
			}
		})
	})
}

func TestUniverseDigest(t *testing.T) {
	sets := []witness.Set{setOf(3, 1), setOf(4, 2), setOf(5, 3)}
	sets[1].Criterion = otherCriterion
	t.Run("the digest of a universe does not depend on the order of its sets"+verifiesSP1961, func(t *testing.T) {
		reversed := slices.Clone(sets)
		slices.Reverse(reversed)
		if witness.UniverseDigest(sets) != witness.UniverseDigest(reversed) {
			t.Error("the order changed the digest")
		}
	})
	t.Run("the digest of a universe changes with its sets"+verifiesSP1961, func(t *testing.T) {
		if witness.UniverseDigest(sets) == witness.UniverseDigest(sets[:2]) {
			t.Error("a missing set keeps the digest")
		}
		if witness.UniverseDigest(nil) == witness.UniverseDigest(sets) {
			t.Error("an empty universe shares the digest")
		}
	})
	t.Run("any permutation of the sets gives one digest"+verifiesSP1961, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			count := rapid.IntRange(0, maxSetSize).Draw(t, "count")
			universe := make([]witness.Set, count)
			for index := range universe {
				universe[index] = setOf(rapid.IntRange(0, maxSetSize).Draw(t, "examples"), rapid.IntRange(0, maxSetSize).Draw(t, "properties"))
				universe[index].Imports = rapid.String().Draw(t, "imports")
			}
			shuffled := rapid.Permutation(universe).Draw(t, "permutation")
			if witness.UniverseDigest(universe) != witness.UniverseDigest(shuffled) {
				t.Fatal("a permutation changed the digest")
			}
		})
	})
}

func TestComplete(t *testing.T) {
	for examples := 0; examples <= witness.MaxExamples+1; examples++ {
		for properties := 0; properties <= witness.MaxProperties+1; properties++ {
			want := examples >= witness.MinExamples && examples <= witness.MaxExamples &&
				properties >= witness.MinProperties && properties <= witness.MaxProperties
			t.Run(fmt.Sprintf("%d examples and %d properties", examples, properties)+verifiesSP1961, func(t *testing.T) {
				set := setOf(examples, properties)
				if got := set.Complete(); got != want {
					t.Errorf("Complete() = %v, want %v", got, want)
				}
				gotExamples, gotProperties := set.Counts()
				if gotExamples != examples || gotProperties != properties {
					t.Errorf("Counts() = %d, %d", gotExamples, gotProperties)
				}
			})
		}
	}
	t.Run("a witness of an unknown kind counts as neither"+verifiesSP1961, func(t *testing.T) {
		set := setOf(witness.MinExamples, 1)
		set.Witnesses = append(set.Witnesses, witness.New(criterion, "odd", "clause", witness.Kind("fuzz"), "code"))
		if examples, properties := set.Counts(); examples != witness.MinExamples || properties != 1 {
			t.Errorf("Counts() = %d, %d", examples, properties)
		}
		if witness.Kind("fuzz").Valid() || !witness.KindExample.Valid() || !witness.KindProperty.Valid() {
			t.Error("Valid() misjudges a kind")
		}
	})
}
