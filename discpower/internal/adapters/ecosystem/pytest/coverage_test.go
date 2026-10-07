package pytest_test

import (
	"context"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/footprint"
)

const (
	ratesFile  = "parcel/rates.py"
	testsFile  = "tests/test_rates.py"
	testModule = "tests.test_rates"
)

func TestCoverageRecorder(t *testing.T) {
	project := openLiveProject(t)
	waived := ecosystem.NewBoundTest(testModule, "test_shipping_is_waived_from_fifty", testsFile, 0)
	parametrized := ecosystem.NewBoundTest(testModule, "test_discount_reduces_total[10-200-180.0]", testsFile, 0)
	clamped := ecosystem.NewBoundTest(testModule+".TestDiscountLimits", "test_percent_above_one_hundred_is_clamped", testsFile, 0)
	neverRun := ecosystem.NewBoundTest(testModule, "test_that_does_not_exist", testsFile, 0)

	covered, err := project.strategy.Coverage().Record(context.Background(), project.workspace, project.toolchain,
		[]ecosystem.BoundTest{waived, parametrized, clamped, neverRun})
	if err != nil {
		t.Fatal(err)
	}

	executes := func(test ecosystem.BoundTest, qualname string) bool {
		for _, ref := range covered[test.ID] {
			if ref == (footprint.Ref{File: ratesFile, Qualname: qualname}) {
				return true
			}
		}
		return false
	}

	t.Run("the shipping test executes shipping_fee only [verifies SP-161~1]", func(t *testing.T) {
		if !executes(waived, "shipping_fee") || executes(waived, "discounted") {
			t.Fatalf("units = %+v", covered[waived.ID])
		}
	})
	t.Run("a parametrized discount test executes discounted and clamp_percent [verifies SP-161~1]", func(t *testing.T) {
		if !executes(parametrized, "discounted") || !executes(parametrized, "clamp_percent") {
			t.Fatalf("units = %+v", covered[parametrized.ID])
		}
	})
	t.Run("a test of a class is keyed by its class and executes the code it calls [verifies SP-161~1]", func(t *testing.T) {
		if !executes(clamped, "clamp_percent") {
			t.Fatalf("units = %+v", covered[clamped.ID])
		}
	})
	t.Run("test files are never reported as units [verifies SP-161~1]", func(t *testing.T) {
		for _, ref := range covered[waived.ID] {
			if ref.File == testsFile {
				t.Fatalf("test file reported: %+v", ref)
			}
		}
	})
	t.Run("a requested test the run never reported is absent [verifies SP-161~1]", func(t *testing.T) {
		if covered.Known(neverRun.ID) {
			t.Fatalf("unexpected entry for %s", neverRun.ID)
		}
	})
	t.Run("only the requested tests are returned [verifies SP-161~1]", func(t *testing.T) {
		if len(covered) != 3 {
			t.Fatalf("%d entries: %v", len(covered), covered)
		}
	})
}
