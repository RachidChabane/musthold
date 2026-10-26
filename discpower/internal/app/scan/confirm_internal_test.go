package scan

import (
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/format"
)

func TestShownRoute(t *testing.T) {
	const fragile, upheld = "tests/test_a.py::test_fragile", "tests/test_b.py::test_upheld"
	kills := []format.ScanRoute{
		{Kind: format.ScanRouteKilledByTest, Test: fragile},
		{Kind: format.ScanRouteKilledByTest, Test: upheld},
	}
	t.Run("a finding shows a kill its confirmation upheld, never one it did not [verifies SP-2144~1]", func(t *testing.T) {
		if shown := shownRoute(kills, []ecosystem.TestID{upheld}); shown.Test != upheld {
			t.Fatalf("shown %+v", shown)
		}
	})
	t.Run("a finding shows a difference first [verifies SP-2113~1]", func(t *testing.T) {
		difference := format.ScanRoute{Kind: format.ScanRouteDifferential, Operation: "invoice.totals:discount", Input: "{}", Reference: "1", Mutant: "2"}
		if shown := shownRoute(append(kills, difference), []ecosystem.TestID{upheld}); shown.Kind != format.ScanRouteDifferential {
			t.Fatalf("shown %+v", shown)
		}
	})
}
