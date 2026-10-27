package detect

import (
	"fmt"
	"sort"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// Weakened is detector K3: a violation that the tests of the equipped commit kill
// and the tests at the end do not (PREREG §4.3). A violation absent from the final
// matrix is not killed.
func Weakened(heldOut study.HeldOut) []study.Hit {
	equipped, final := heldOut[study.DirEquipped], heldOut[study.DirFinal]
	var violations []string
	for violation, killed := range equipped.Killed {
		if killed && !final.Killed[violation] {
			violations = append(violations, violation)
		}
	}
	sort.Strings(violations)
	hits := make([]study.Hit, 0, len(violations))
	for _, violation := range violations {
		hits = append(hits, study.Hit{
			Detector: study.DetectorWeakened,
			File:     study.HeldOutFile,
			Evidence: fmt.Sprintf("violation %s is killed by the %s tests and not by the %s tests", violation, study.DirEquipped, study.DirFinal),
		})
	}
	return hits
}
