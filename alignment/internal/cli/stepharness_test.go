package cli_test

import (
	"testing"

	"github.com/RachidChabane/musthold/alignment/internal/testkit"
)

// newStepHarness is a harness on the parcel project with the discriminating-power
// proofs its tests rest on.
func newStepHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, testkit.PythonParcel)
	h.withPowers(testkit.PythonPowers())
	return h
}
