package pytest

import (
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/vacuity"
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
			index := runIndex{sessions: []scanReport{{CollectFailures: c.failures}}}
			if got := index.missingRun(test); got != c.want {
				t.Fatalf("missing run = %s, want %s", got, c.want)
			}
		})
	}
}

func TestRunIndexOfSeveralSessions(t *testing.T) {
	const (
		rate    = ecosystem.TestID("tests/billing/test_rates.py::test_reduced_rate")
		ship    = ecosystem.TestID("tests/shipping/test_ship.py::test_ship")
		vanish  = ecosystem.TestID("tests/shipping/test_ship.py::test_ship_by_zone[far]")
		pending = ecosystem.TestID("tests/zones/test_zones.py::test_zone")
	)
	index := runIndex{sessions: []scanReport{
		{CollectFailures: []string{"tests/billing/conftest.py"}, StoppedBeforeCollection: true},
		{Outcomes: map[string]string{string(ship): string(ecosystem.TestPassed)}},
	}}
	t.Run("a run index gives a test covered by an earlier session's failure a run that did not start [verifies SP-2142~1]", func(t *testing.T) {
		if got := index.missingRun(rate); got != vacuity.RunNotStarted {
			t.Fatalf("missing run = %s", got)
		}
	})
	t.Run("a run index gives a test the outcome of the session that ran it [verifies SP-2142~1]", func(t *testing.T) {
		if got := index.outcomes()[ship]; got != ecosystem.TestPassed {
			t.Fatalf("outcome = %q", got)
		}
	})
	t.Run("a run index gives a test absent from a module that collected an error [verifies SP-2142~1]", func(t *testing.T) {
		if _, reported := index.outcomes()[vanish]; reported || index.missingRun(vanish) != vacuity.RunErrored {
			t.Fatalf("missing run = %s", index.missingRun(vanish))
		}
	})
	t.Run("a run index whose last session timed out gives the tests no failure covers a timeout [verifies SP-2142~1]", func(t *testing.T) {
		stopped := runIndex{sessions: index.sessions[:1], timedOut: true}
		if stopped.missingRun(pending) != vacuity.RunTimedOut || stopped.missingRun(rate) != vacuity.RunNotStarted {
			t.Fatalf("missing runs = %s, %s", stopped.missingRun(pending), stopped.missingRun(rate))
		}
	})
	t.Run("a session of selected tests names each of their files once, in the order of their first test [verifies SP-2102~4]", func(t *testing.T) {
		files := filesOf([]ecosystem.TestID{ship, rate, vanish})
		if len(files) != 2 || files[0] != "tests/shipping/test_ship.py" || files[1] != "tests/billing/test_rates.py" {
			t.Fatalf("files = %v", files)
		}
	})
}
