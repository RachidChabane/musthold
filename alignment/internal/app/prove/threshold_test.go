package prove

import (
	"testing"

	"github.com/RachidChabane/musthold/alignment/internal/domain/policy"
	"github.com/RachidChabane/musthold/format"
)

func TestRecordedThreshold(t *testing.T) {
	const (
		routeDefault = 0.90
		routeSystem  = 0.97
		modalDefault = 0.80
		modalExact   = 0.60
		alignK       = 5
	)
	rules := policy.Default()
	rules.AlignK = alignK
	rules.TauRoute = format.Threshold{Default: routeDefault, BySource: map[format.ConfidenceSource]float64{format.SourceSystemOne: routeSystem}}
	rules.ModalMin = format.Threshold{Default: modalDefault, BySource: map[format.ConfidenceSource]float64{format.HostVotesSource(alignK): modalExact}}
	run := &subjectRun{pass: &passState{settings: Settings{Policy: rules}}}
	endpoint := sourced{source: format.SystemOneSource("nimble:9b@" + format.SHA256Hex([]byte("endpoint"))), mapping: format.MappingSystemOneChoice}
	votes := sourced{source: format.HostVotesSource(3), mapping: format.MappingModalShare}

	cases := []struct {
		name       string
		basis      format.DecisionBasis
		confidence sourced
		want       float64
	}{
		{"a System One endpoint is held to the threshold of System One", format.BasisSystemOne, endpoint, routeSystem},
		{"host votes standing in for the endpoint are held to the default route threshold", format.BasisSystemOne, votes, routeDefault},
		{"the judges are held to the exact source of their votes", format.BasisJudges, sourced{source: format.HostVotesSource(alignK)}, modalExact},
		{"a verdict the module fixed itself is held to full confidence", format.BasisExecution, sourced{source: format.SourceExecution}, format.MaxConfidence},
	}
	for _, c := range cases {
		t.Run(c.name+" [verifies SP-903~1]", func(t *testing.T) {
			if got := run.thresholdOf(c.basis, c.confidence); got != c.want {
				t.Fatalf("threshold = %v, want %v", got, c.want)
			}
		})
	}
}
