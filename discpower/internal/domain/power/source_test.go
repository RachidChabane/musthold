package power_test

import (
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/format"
)

const verifiesSP406 = " [verifies SP-406~1]"

func TestDecisionConfidenceSource(t *testing.T) {
	restricted := format.IsolationRestricted

	t.Run("a sentinel and a spec example rest on execution and name no mapping"+verifiesSP406, func(t *testing.T) {
		sentinel := power.Decide(newSubject(), survivor(power.OriginSentinel, power.NoDiffFound), nil, policy.Default())
		facts := killedBy(power.OriginOperator, testOwnA)
		facts.FailedExamples = []spec.Ref{subjectRef}
		attributed := power.Decide(newSubject(), facts, nil, policy.Default())
		for name, decision := range map[string]power.Decision{"sentinel": sentinel, "example": attributed} {
			if decision.Source != format.SourceExecution || decision.Mapping != "" {
				t.Errorf("%s: source %q, mapping %q", name, decision.Source, decision.Mapping)
			}
		}
	})

	t.Run("a judged decision rests on the host votes and maps the weakest stated confidence"+verifiesSP406, func(t *testing.T) {
		votes := repeated(confirmedVote(fullConfidence, restricted), judgeCount)
		got := power.Decide(newSubject(), survivor(power.OriginTargeted, power.Skipped), votes, policy.Default())
		if got.Class != power.ClassValid {
			t.Fatalf("class %q", got.Class)
		}
		if got.Source != format.HostVotesSource(policy.Default().JudgeK) || got.Mapping != format.MappingWeakestStated {
			t.Errorf("source %q, mapping %q", got.Source, got.Mapping)
		}
		if !got.Source.IsKnown() {
			t.Errorf("%q is not a known confidence source", got.Source)
		}
	})

	t.Run("a mutant no judge could classify is undetermined by execution, not by a vote"+verifiesSP406, func(t *testing.T) {
		got := power.Decide(newSubject(), survivor(power.OriginTargeted, power.Skipped), nil, policy.Default())
		if got.Class != power.ClassUndetermined || got.Source != format.SourceExecution || got.Confidence != format.MinConfidence {
			t.Errorf("decision %+v", got)
		}
	})
}
