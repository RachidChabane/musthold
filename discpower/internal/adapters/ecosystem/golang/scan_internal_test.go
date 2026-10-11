package golang

import (
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
)

const (
	tallyModule  = "example.com/tally"
	tallyPackage = "example.com/tally"
)

func TestProfileCoverage(t *testing.T) {
	profile := strings.Join([]string{
		"mode: atomic",
		"example.com/tally/tally.go:18.38,19.18 1 3",
		"example.com/tally/tally.go:19.18,21.3 1 1",
		"example.com/tally/tally.go:25.2,25.42 1 0",
		"example.com/tally/internal/rate/rate.go:3.20,5.2 1 1",
		"example.com/tally/internal/rate/rate.go:3.20,5.2 1 1",
		"example.com/other/x.go:1.1,2.2 1 1",
		"example.com/tally/broken.go:x 1 1",
	}, "\n")
	startUp := readBlockCounts([]byte(strings.Join([]string{
		"mode: atomic",
		"example.com/tally/tally.go:18.38,19.18 1 1",
		"example.com/tally/tally.go:19.18,21.3 1 1",
		"example.com/tally/tally.go:25.2,25.42 1 0",
		"example.com/tally/internal/rate/rate.go:3.20,5.2 1 1",
	}, "\n")))
	t.Run("the blocks a test ran give their lines, relative to the module, and a block it did not run gives none [verifies SP-2101~2]", func(t *testing.T) {
		got := profileCoverage([]byte(profile), startUp, tallyModule).Lines
		want := []vacuity.Line{
			{File: "internal/rate/rate.go", Line: 3}, {File: "internal/rate/rate.go", Line: 4}, {File: "internal/rate/rate.go", Line: 5},
			{File: "tally.go", Line: 18}, {File: "tally.go", Line: 19}, {File: "tally.go", Line: 20}, {File: "tally.go", Line: 21},
		}
		if !slices.Equal(got, want) {
			t.Fatalf("lines = %v", got)
		}
	})
	t.Run("a test owns the blocks it ran more often than the start-up, a repeated block counted once per listing [verifies SP-2101~2]", func(t *testing.T) {
		got := profileCoverage([]byte(profile), startUp, tallyModule).Owned
		want := []vacuity.Line{
			{File: "internal/rate/rate.go", Line: 3}, {File: "internal/rate/rate.go", Line: 4}, {File: "internal/rate/rate.go", Line: 5},
			{File: "tally.go", Line: 18}, {File: "tally.go", Line: 19},
		}
		if !slices.Equal(got, want) {
			t.Fatalf("owned = %v", got)
		}
	})
	t.Run("a test whose binary gave no start-up counts owns no line [verifies SP-2101~2]", func(t *testing.T) {
		if coverage := profileCoverage([]byte(profile), nil, tallyModule); len(coverage.Lines) == 0 || coverage.Owned != nil {
			t.Fatalf("coverage = %+v", coverage)
		}
	})
}

func TestReadProcess(t *testing.T) {
	output := strings.Join([]string{
		`{"Action":"start","Package":"example.com/tally"}`,
		`{"Action":"run","Package":"example.com/tally","Test":"TestShipping"}`,
		`{"Action":"pass","Package":"example.com/tally","Test":"TestShipping/waived"}`,
		`{"Action":"fail","Package":"example.com/tally","Test":"TestShipping/charged"}`,
		`{"Action":"fail","Package":"example.com/tally","Test":"TestShipping"}`,
		`{"Action":"pass","Package":"example.com/tally","Test":"TestShipping/waived"}`,
		`{"Action":"skip","Package":"example.com/tally","Test":"TestLater"}`,
		`{"Action":"output","Package":"example.com/tally","Output":"ok\n"}`,
		`{"Action":"pass","Package":"example.com/tally"}`,
	}, "\n")
	t.Run("each leaf test has its results in order; a parent and package events are not tests [verifies SP-2102~3]", func(t *testing.T) {
		got := readProcess([]byte(output), 1, false).leaves
		want := map[resultKey][]vacuity.Run{
			{pkg: tallyPackage, test: "TestShipping/waived"}:  {vacuity.RunPassed, vacuity.RunPassed},
			{pkg: tallyPackage, test: "TestShipping/charged"}: {vacuity.RunFailed},
			{pkg: tallyPackage, test: "TestLater"}:            {vacuity.RunErrored},
		}
		if len(got) != len(want) {
			t.Fatalf("results = %v", got)
		}
		for key, runs := range want {
			if !slices.Equal(got[key], runs) {
				t.Fatalf("%v: %v", key, got[key])
			}
		}
	})

	stopped := strings.Join([]string{
		`{"Action":"start","Package":"example.com/tally"}`,
		`{"Action":"output","Package":"example.com/tally","Output":"panic: rate out of range\n"}`,
		`{"Action":"fail","Package":"example.com/tally","Elapsed":0.3}`,
	}, "\n")
	buildFailed := strings.Join([]string{
		`{"ImportPath":"example.com/tally [example.com/tally.test]","Action":"build-fail"}`,
		`{"Action":"start","Package":"example.com/tally"}`,
		`{"Action":"fail","Package":"example.com/tally","FailedBuild":"example.com/tally [example.com/tally.test]"}`,
	}, "\n")
	key := resultKey{pkg: tallyPackage, test: "TestShipping/waived"}
	cases := []struct {
		name     string
		output   string
		exitCode int
		timedOut bool
		want     vacuity.Run
	}{
		{"a binary that stopped with no test event, exiting non-zero, did not start", stopped, 1, false, vacuity.RunNotStarted},
		{"a binary that stopped and timed out timed out", stopped, 1, true, vacuity.RunTimedOut},
		{"a binary that did not compile errored", buildFailed, 1, false, vacuity.RunErrored},
		{"a process with no event of the package errored", "", 1, false, vacuity.RunErrored},
		{"a test with end events takes its first", output, 1, false, vacuity.RunPassed},
	}
	for _, c := range cases {
		t.Run(c.name+" [verifies SP-2134~1]", func(t *testing.T) {
			if got := readProcess([]byte(c.output), c.exitCode, c.timedOut).observation(key); got != c.want {
				t.Fatalf("observation = %s, want %s", got, c.want)
			}
		})
	}
	t.Run("a test a started binary did not reach errored [verifies SP-2134~1]", func(t *testing.T) {
		process := readProcess([]byte(output), 1, false)
		if got := process.observation(resultKey{pkg: tallyPackage, test: "TestUnreached"}); got != vacuity.RunErrored {
			t.Fatalf("observation = %s", got)
		}
	})
	t.Run("a process cut short is one observation only when it exited non-zero without timing out [verifies SP-2134~1]", func(t *testing.T) {
		keys := []resultKey{key, {pkg: tallyPackage, test: "TestShipping/charged"}}
		if !readProcess([]byte(output), 1, false).cutShort(keys, 2) {
			t.Fatal("a test with one end event of two was not cut short")
		}
		for _, process := range []testProcess{readProcess([]byte(output), 0, false), readProcess([]byte(output), 1, true)} {
			if process.cutShort(keys, 2) {
				t.Fatalf("cut short: exit %d, timed out %v", process.exitCode, process.timedOut)
			}
		}
		if readProcess([]byte(output), 1, false).cutShort(keys[:1], 2) {
			t.Fatal("a test with every end event was cut short")
		}
	})
}
