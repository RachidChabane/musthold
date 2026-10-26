package verdict_test

import (
	"testing"

	"github.com/RachidChabane/musthold/alignment/internal/domain/matrix"
	"github.com/RachidChabane/musthold/alignment/internal/domain/policy"
	"github.com/RachidChabane/musthold/alignment/internal/domain/verdict"
	"github.com/RachidChabane/musthold/format"
)

func TestRouteThresholdPerSource(t *testing.T) {
	const confidence = 0.95
	endpoint := format.SystemOneSource("nimble:9b@" + format.SHA256Hex([]byte("endpoint")))
	routed := func(rules policy.Policy, source format.ConfidenceSource) verdict.Decision {
		answer := verdict.SystemOneInput{Settled: true, Supports: true, Confidence: confidence, Source: source}
		return verdict.Decide(verdict.Inputs{Matrix: result(matrix.Specific), SystemOne: answer}, rules)
	}
	strict := policy.Default()
	strict.TauRoute = format.Threshold{Default: 0.90, BySource: map[format.ConfidenceSource]float64{format.SourceSystemOne: 0.99}}

	t.Run("an endpoint below the threshold of System One goes to the judge although it clears the default [verifies SP-903~1]", func(t *testing.T) {
		if got := routed(strict, endpoint); got.Done || got.Need != verdict.NeedJudge {
			t.Fatalf("decision %+v", got)
		}
	})

	t.Run("host votes standing in for the endpoint keep the default threshold [verifies SP-903~1]", func(t *testing.T) {
		if got := routed(strict, format.HostVotesSource(3)); !got.Done || got.Verdict != format.VerdictAligned || got.Basis != format.BasisSystemOne {
			t.Fatalf("decision %+v", got)
		}
	})

	t.Run("the threshold of the exact source beats the one of its kind [verifies SP-903~1]", func(t *testing.T) {
		exact := strict
		exact.TauRoute = format.Threshold{Default: 0.99, BySource: map[format.ConfidenceSource]float64{format.SourceSystemOne: 0.99, endpoint: 0.5}}
		if got := routed(exact, endpoint); !got.Done || got.Verdict != format.VerdictAligned {
			t.Fatalf("decision %+v", got)
		}
	})
}
