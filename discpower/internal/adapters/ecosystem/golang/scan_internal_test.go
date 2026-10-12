package golang

import (
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
)

const (
	tallyModule       = "example.com/tally"
	tallyPackage      = "example.com/tally"
	verifiesOwnership = " [verifies SP-2101~3]"
	verifiesAncestors = " [verifies SP-2141~1]"
	verifiesSelection = " [verifies SP-2102~4]"
	// actionRun starts one run of a test.
	actionRun = "run"
)

// tallySource is the source of tally.go that the profiles below count.
var tallySource = []string{
	"package tally",
	"",
	"func Fee(c bool) int {",
	"\tif c { charge() }",
	"\tif c {",
	"\t\treturn 1",
	"\t}",
	"\treturn 0",
	"}",
	"func Kind(c bool) int {",
	"\tswitch {",
	"\tcase c:",
	"\t\treturn 1",
	"\t}",
	"\treturn 0",
	"}",
}

func tallyLines(file string, line int) (string, bool) {
	if file != "tally.go" || line < 1 || line > len(tallySource) {
		return "", false
	}
	return tallySource[line-1], true
}

func profileOf(lines ...string) blockCounts {
	return readBlockCounts([]byte(strings.Join(append([]string{"mode: atomic"}, lines...), "\n")))
}

// runOf is what a run that counted the profile ran.
func runOf(lines ...string) runCounts { return countsRun(profileOf(lines...)) }

// stableBaseline is a baseline whose runs all counted the profile.
func stableBaseline(lines ...string) baseline {
	run := baselineRun{counts: countsRun(profileOf(lines...)), usable: true}
	return baselineOf(slices.Repeat([]baselineRun{run}, baselineRunsPerPath))
}

func line(number int) vacuity.Line { return vacuity.Line{File: "tally.go", Line: number} }

func TestLeafCoverage(t *testing.T) {
	startUp := stableBaseline(
		"example.com/tally/tally.go:3.22,4.7 1 1",
		"example.com/tally/tally.go:4.7,4.19 1 0",
		"example.com/tally/tally.go:5.2,5.7 1 1",
		"example.com/tally/tally.go:5.7,7.3 1 1",
		"example.com/tally/tally.go:8.2,8.10 1 0",
	)
	t.Run("a test runs every line of the blocks it ran, relative to the module, and none of a file outside it"+verifiesOwnership, func(t *testing.T) {
		profile := profileOf(
			"example.com/tally/tally.go:3.22,4.7 1 1",
			"example.com/tally/tally.go:4.7,4.19 1 1",
			"example.com/tally/tally.go:8.2,8.10 1 0",
			"example.com/other/x.go:1.1,2.2 1 1",
			"example.com/tally/broken.go:x 1 1",
		)
		got := leafCoverage(profile, baselineChain{startUp}, tallyLines, tallyModule).Lines
		if want := []vacuity.Line{line(3), line(4)}; !slices.Equal(got, want) {
			t.Fatalf("lines = %v", got)
		}
	})
	t.Run("a one-line condition whose condition ran only at start-up is not owned, though the test owns its body"+verifiesOwnership, func(t *testing.T) {
		profile := profileOf(
			"example.com/tally/tally.go:3.22,4.7 1 1",
			"example.com/tally/tally.go:4.7,4.19 1 1",
		)
		coverage := leafCoverage(profile, baselineChain{startUp}, tallyLines, tallyModule)
		if slices.Contains(coverage.Owned, line(4)) || !slices.Contains(coverage.Lines, line(4)) {
			t.Fatalf("coverage = %+v", coverage)
		}
	})
	t.Run("a condition line the test ran itself is owned though its body ran at start-up, and the body is not"+verifiesOwnership, func(t *testing.T) {
		profile := profileOf(
			"example.com/tally/tally.go:3.22,4.7 1 2",
			"example.com/tally/tally.go:5.2,5.7 1 2",
			"example.com/tally/tally.go:5.7,7.3 1 1",
		)
		coverage := leafCoverage(profile, baselineChain{startUp}, tallyLines, tallyModule)
		if !slices.Contains(coverage.Owned, line(5)) || slices.Contains(coverage.Owned, line(6)) {
			t.Fatalf("owned = %v", coverage.Owned)
		}
		if slices.Contains(coverage.Owned, line(3)) {
			t.Fatalf("a line whose only block holds its brace is owned: %v", coverage.Owned)
		}
	})
	t.Run("a case line, whose condition no block holds, is run and not owned, though the test owns the case's body"+verifiesOwnership, func(t *testing.T) {
		profile := profileOf(
			"example.com/tally/tally.go:10.23,11.9 1 1",
			"example.com/tally/tally.go:12.9,13.11 1 1",
		)
		coverage := leafCoverage(profile, baselineChain{startUp}, tallyLines, tallyModule)
		if !slices.Contains(coverage.Lines, line(12)) || slices.Contains(coverage.Owned, line(12)) || !slices.Contains(coverage.Owned, line(13)) {
			t.Fatalf("coverage = %+v", coverage)
		}
	})
	t.Run("a block a run of a baseline counted differently is owned by no test under it, and the immediate baseline sets the count to beat"+verifiesOwnership, func(t *testing.T) {
		unsteady := baselineOf([]baselineRun{
			{counts: runOf("example.com/tally/tally.go:8.2,8.10 1 0", "example.com/tally/tally.go:5.2,5.7 1 0"), usable: true},
			{counts: runOf("example.com/tally/tally.go:8.2,8.10 1 1", "example.com/tally/tally.go:5.2,5.7 1 0"), usable: true},
			{counts: runOf("example.com/tally/tally.go:8.2,8.10 1 0"), usable: true},
		})
		parent := stableBaseline("example.com/tally/tally.go:5.2,5.7 1 2", "example.com/tally/tally.go:8.2,8.10 1 3")
		chain := baselineChain{unsteady, parent}
		profile := profileOf("example.com/tally/tally.go:8.2,8.10 1 9", "example.com/tally/tally.go:5.2,5.7 1 2")
		if coverage := leafCoverage(profile, chain, tallyLines, tallyModule); coverage.Owned != nil {
			t.Fatalf("owned = %v", coverage.Owned)
		}
		if owned := leafCoverage(profileOf("example.com/tally/tally.go:5.2,5.7 1 3"), chain, tallyLines, tallyModule).Owned; !slices.Equal(owned, []vacuity.Line{line(5)}) {
			t.Fatalf("owned = %v", owned)
		}
	})
	t.Run("a test with a missing baseline above it owns no line"+verifiesOwnership, func(t *testing.T) {
		failing := baselineOf([]baselineRun{{counts: runOf(), usable: true}, {counts: countsRun(profileOf())}, {counts: runOf(), usable: true}})
		profile := profileOf("example.com/tally/tally.go:8.2,8.10 1 1")
		coverage := leafCoverage(profile, baselineChain{startUp, failing}, tallyLines, tallyModule)
		if len(coverage.Lines) == 0 || coverage.Owned != nil {
			t.Fatalf("coverage = %+v", coverage)
		}
		if coverage := leafCoverage(profile, nil, tallyLines, tallyModule); coverage.Owned != nil {
			t.Fatalf("a test with no baseline owns %v", coverage.Owned)
		}
	})
	t.Run("a block on a line that cannot be read holds code there"+verifiesOwnership, func(t *testing.T) {
		unreadable := func(string, int) (string, bool) { return "", false }
		profile := profileOf("example.com/tally/tally.go:3.22,4.7 1 2")
		if owned := leafCoverage(profile, baselineChain{startUp}, unreadable, tallyModule).Owned; !slices.Equal(owned, []vacuity.Line{line(3), line(4)}) {
			t.Fatalf("owned = %v", owned)
		}
	})
	t.Run("a block listed twice counts the sum of its listings"+verifiesOwnership, func(t *testing.T) {
		profile := profileOf("example.com/tally/tally.go:8.2,8.10 1 1", "example.com/tally/tally.go:8.2,8.10 1 1")
		if got := profile["example.com/tally/tally.go:8.2,8.10"].count; got != 2 {
			t.Fatalf("count = %d", got)
		}
	})
}

func TestBaselineSelection(t *testing.T) {
	run := coveredRun{dir: "fees", elements: []string{"TestFee", "waived", "from_fifty"}}
	keys := run.baselineKeys()
	want := []baselineKey{{dir: "fees"}, {dir: "fees", ancestor: "TestFee"}, {dir: "fees", ancestor: "TestFee/waived"}}
	t.Run("a leaf's baselines are the start-up and each of its ancestors"+verifiesOwnership, func(t *testing.T) {
		if !slices.Equal(keys, want) {
			t.Fatalf("keys = %v", keys)
		}
	})
	t.Run("a baseline runs no test, or its ancestor with no subtest"+verifiesOwnership, func(t *testing.T) {
		got := []string{keys[0].selection(), keys[1].selection(), keys[2].selection()}
		if !slices.Equal(got, []string{"^$", "^TestFee$/^$", "^TestFee$/^waived$/^$"}) {
			t.Fatalf("selections = %v", got)
		}
	})
	t.Run("each distinct baseline is planned once, the start-up of every binary first"+verifiesOwnership, func(t *testing.T) {
		binary := &coverageBinary{dir: "fees"}
		sibling := coveredRun{dir: "fees", binary: binary, elements: []string{"TestFee", "waived", "below_fifty"}}
		run.binary = binary
		plan := planCoverageRuns([]*coverageBinary{binary}, []coveredRun{run, sibling})
		if !slices.Equal(plan.baselineKeys, want) || plan.baselineJobs() != len(want)*baselineRunsPerPath {
			t.Fatalf("plan = %v", plan.baselineKeys)
		}
	})
	t.Run("the runs of a scan select exactly their subtest paths"+verifiesSelection, func(t *testing.T) {
		got := exactExpression([][]string{{"TestFee", "waived"}, {"TestRate"}, {"TestFee", "a|b"}, {"TestRate"}})
		if got != `^TestFee$/^waived$|^TestRate$|^TestFee$/^a\|b$` {
			t.Fatalf("expression = %s", got)
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
	t.Run("each leaf test has its results in order; a parent and package events are not tests [verifies SP-2102~4]", func(t *testing.T) {
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
		{"a test with end events takes its first, read against its parent's", output, 1, false, vacuity.RunErrored},
	}
	for _, c := range cases {
		t.Run(c.name+" [verifies SP-2134~1]", func(t *testing.T) {
			if got, _ := readProcess([]byte(c.output), c.exitCode, c.timedOut).observation(key); got != c.want {
				t.Fatalf("observation = %s, want %s", got, c.want)
			}
		})
	}
	t.Run("a test a started binary did not reach errored [verifies SP-2134~1]", func(t *testing.T) {
		process := readProcess([]byte(output), 1, false)
		if got, _ := process.observation(resultKey{pkg: tallyPackage, test: "TestUnreached"}); got != vacuity.RunErrored {
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

// iteration is the end events of one iteration of a `-count` process, each a
// test and its action.
func iteration(ends ...string) []string {
	events := make([]string, 0, len(ends)/2)
	for index := 0; index+1 < len(ends); index += 2 {
		events = append(events, `{"Action":"`+ends[index+1]+`","Package":"example.com/tally","Test":"`+ends[index]+`"}`)
	}
	return events
}

func processOf(exitCode int, timedOut bool, iterations ...[]string) testProcess {
	var events []string
	for _, ends := range iterations {
		events = append(events, ends...)
	}
	return readProcess([]byte(strings.Join(events, "\n")), exitCode, timedOut)
}

func TestAncestorPairing(t *testing.T) {
	waived := resultKey{pkg: tallyPackage, test: "TestShipping/waived"}
	deep := resultKey{pkg: tallyPackage, test: "TestShipping/zone/far"}
	alone := iteration("TestShipping/waived", actionPass, "TestShipping/charged", actionPass, "TestShipping", actionFail)
	sibling := iteration("TestShipping/waived", actionPass, "TestShipping/charged", actionFail, "TestShipping", actionFail)
	passing := iteration("TestShipping/waived", actionPass, "TestShipping/charged", actionPass, "TestShipping", actionPass)
	cases := []struct {
		name          string
		process       testProcess
		key           resultKey
		want          []vacuity.Run
		wantAmbiguous bool
	}{
		{
			"a pass under a parent that failed on its own is errored, never a failure",
			processOf(1, false, alone, alone, alone), waived,
			[]vacuity.Run{vacuity.RunErrored, vacuity.RunErrored, vacuity.RunErrored}, false,
		},
		{
			"a pass under a parent a failing sibling failed is ambiguous",
			processOf(1, false, sibling, sibling, sibling), waived,
			[]vacuity.Run{vacuity.RunErrored, vacuity.RunErrored, vacuity.RunErrored}, true,
		},
		{
			"each iteration pairs with its parent's end event of that iteration",
			processOf(1, false, sibling, alone, passing), waived,
			[]vacuity.Run{vacuity.RunErrored, vacuity.RunErrored, vacuity.RunPassed}, true,
		},
		{
			"a pass whose ancestors all passed stays a pass",
			processOf(0, false, passing, passing, passing), waived,
			[]vacuity.Run{vacuity.RunPassed, vacuity.RunPassed, vacuity.RunPassed}, false,
		},
		{
			"a failure stays a failure whatever its parent did",
			processOf(1, false, sibling, sibling, sibling), resultKey{pkg: tallyPackage, test: "TestShipping/charged"},
			[]vacuity.Run{vacuity.RunFailed, vacuity.RunFailed, vacuity.RunFailed}, false,
		},
		{
			"a failing intermediate ancestor with no other failing descendant makes a deep pass errored, whatever its siblings did",
			processOf(1, false, iteration("TestShipping/zone/far", actionPass, "TestShipping/zone", actionFail, "TestShipping/charged", actionFail, "TestShipping", actionFail)), deep,
			[]vacuity.Run{vacuity.RunErrored}, false,
		},
		{
			"a pass whose parent never ended in a process that timed out timed out, and is not run again",
			processOf(1, true, iteration("TestShipping", actionRun, "TestShipping/waived", actionPass, "TestShipping/charged", actionFail)), waived,
			[]vacuity.Run{vacuity.RunTimedOut}, false,
		},
	}
	for _, c := range cases {
		t.Run(c.name+verifiesAncestors, func(t *testing.T) {
			got, ambiguous := c.process.runsOf(c.key, len(c.want))
			if !slices.Equal(got, c.want) || ambiguous != c.wantAmbiguous {
				t.Fatalf("runs = %v, ambiguous = %v", got, ambiguous)
			}
		})
	}
	t.Run("a subtest whose name holds a slash has no ancestor of that name"+verifiesAncestors, func(t *testing.T) {
		process := processOf(0, false, iteration("TestShipping/a/b", actionPass, "TestShipping", actionPass))
		if got, _ := process.runsOf(resultKey{pkg: tallyPackage, test: "TestShipping/a/b"}, 1); !slices.Equal(got, []vacuity.Run{vacuity.RunPassed}) {
			t.Fatalf("runs = %v", got)
		}
	})
}
