package pytest

import (
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
)

func TestMissingRun(t *testing.T) {
	const test = ecosystem.TestID("tests/billing/test_rates.py::test_reduced_rate[low]")
	cases := []struct {
		name     string
		failures []string
		want     vacuity.Run
	}{
		{"its own module failed to collect", []string{"tests/billing/test_rates.py"}, vacuity.RunNotStarted},
		{"the conftest of its directory failed to import", []string{"tests/billing/conftest.py"}, vacuity.RunNotStarted},
		{"a conftest of a parent directory failed to import", []string{"tests/conftest.py"}, vacuity.RunNotStarted},
		{"the root conftest failed to import", []string{"conftest.py"}, vacuity.RunNotStarted},
		{"the __init__ of its package failed to import", []string{"tests/billing/__init__.py"}, vacuity.RunNotStarted},
		{"a directory on its path failed to collect", []string{"tests/billing"}, vacuity.RunNotStarted},
		{"another module failed to collect", []string{"tests/billing/test_rates_extra.py"}, vacuity.RunErrored},
		{"a sibling directory failed to collect", []string{"tests/bill"}, vacuity.RunErrored},
		{"a conftest of another directory failed to import", []string{"tests/shipping/conftest.py"}, vacuity.RunErrored},
		{"nothing failed to collect", nil, vacuity.RunErrored},
	}
	for _, c := range cases {
		t.Run("a test the session did not report when "+c.name+" [verifies SP-2135~1]", func(t *testing.T) {
			if got := (scanReport{CollectFailures: c.failures}).missingRun(test); got != c.want {
				t.Fatalf("missing run = %s, want %s", got, c.want)
			}
		})
	}
}
