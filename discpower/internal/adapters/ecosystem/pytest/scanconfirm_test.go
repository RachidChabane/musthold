package pytest_test

import (
	"context"
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
)

// The alerts project: a test mutes a channel, and a later test routes an alert
// of that channel.
const (
	alertsFixture       = "../../../../fixtures/scan/pytest-alerts"
	levelsFile          = "alerts/levels.py"
	routingTests        = "tests/test_routing.py"
	mutingTests         = "tests/test_muting.py"
	neverRaisesALevel   = ecosystem.TestID(routingTests + "::test_route_never_raises_a_level")
	silencesOps         = ecosystem.TestID(mutingTests + "::test_mute_silences_the_ops_channel")
	verifiesConfirmed   = " [verifies SP-2144~1]"
	routeCondition      = 16
	routeQuiet          = 17
	routeAsIs           = 18
	quietCondition      = 22
	quietLowered        = 23
	sessionAndAloneRuns = 2
)

func numbersOf(t *testing.T, lines []vacuity.Line) []int {
	t.Helper()
	var numbers []int
	for _, line := range lines {
		if line.File != levelsFile {
			t.Fatalf("a line of %s: %v", line.File, lines)
		}
		numbers = append(numbers, line.Line)
	}
	return numbers
}

func TestScanLineCoverageAlone(t *testing.T) {
	for _, tracer := range []string{"", settraceTracer} {
		name := "sys.monitoring"
		if tracer != "" {
			name = tracer
			t.Setenv(tracerVariable, tracer)
		}
		t.Run(name, func(t *testing.T) {
			project := openScanFixture(t, alertsFixture)
			cases := []ports.ScanCase{{ID: silencesOps, File: mutingTests}, {ID: neverRaisesALevel, File: routingTests}}
			recordings := make([]map[ecosystem.TestID]vacuity.Coverage, 0, sessionAndAloneRuns)
			for _, alone := range []bool{false, true} {
				covered, err := project.suite.LineCoverage(context.Background(), project.workspace, project.toolchain, ports.ScanCoverageRequest{Tests: cases, Timeout: scanRunTimeout, Alone: alone})
				if err != nil {
					t.Fatal(err)
				}
				recordings = append(recordings, covered)
			}
			session, alone := recordings[0][neverRaisesALevel], recordings[1][neverRaisesALevel]

			t.Run("in the session a test owns the branch an earlier test's state opens to it"+verifiesConfirmed, func(t *testing.T) {
				if got := numbersOf(t, session.Owned); !slices.Equal(got, []int{routeCondition, routeQuiet, quietCondition, quietLowered}) {
					t.Fatalf("owned in the session %v", got)
				}
			})
			t.Run("alone the test runs and owns only the branch its own call takes"+verifiesConfirmed, func(t *testing.T) {
				if got := numbersOf(t, alone.Owned); !slices.Equal(got, []int{routeCondition, routeAsIs}) || !slices.Equal(numbersOf(t, alone.Lines), []int{routeCondition, routeAsIs}) {
					t.Fatalf("alone %+v", alone)
				}
			})
			t.Run("each test recorded alone is recorded"+verifiesConfirmed, func(t *testing.T) {
				if _, recorded := recordings[1][silencesOps]; !recorded || len(recordings[1]) != len(cases) {
					t.Fatalf("recorded alone %+v", recordings[1])
				}
			})
		})
	}
}
