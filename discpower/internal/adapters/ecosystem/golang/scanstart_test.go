package golang_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
)

const (
	meterModule       = "testdata/meter"
	keepsPositive     = ecosystem.TestID("gauge/gauge_test.go::TestReadingKeepsPositive")
	clampsNegative    = ecosystem.TestID("gauge/gauge_test.go::TestReadingClampsNegative")
	scaleDoubles      = ecosystem.TestID("rate/rate_test.go::TestScaleDoubles")
	scaleFromWorkers  = ecosystem.TestID("rate/rate_test.go::TestScaleFromWorkers")
	unitIsOne         = ecosystem.TestID("rate/rate_test.go::TestUnitIsOne")
	launchJournal     = "gauge/launches"
	calibrationFile   = "gauge/calibration_test.go"
	verifiesStartUp   = " [verifies SP-2134~1]"
	verifiesOwnership = " [verifies SP-2101~2]"
	// startUpRunTimeout bounds one run of a selection whose start-up sleeps
	// past it.
	startUpRunTimeout = time.Second
)

// calibrationThatFails stops the binary of the gauge package at every start,
// after writing one line to its launch journal.
const calibrationThatFails = `package gauge

import "os"

var calibrated = calibrate()

func calibrate() bool {
	journal, err := os.OpenFile("launches", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		_, _ = journal.WriteString("launch\n")
		_ = journal.Close()
	}
	panic("gauge: no calibration table")
}
`

// calibrationThatFailsFirst stops the binary of the gauge package at its first
// start only.
const calibrationThatFailsFirst = `package gauge

import "os"

var calibrated = calibrate()

func calibrate() bool {
	previous, _ := os.ReadFile("launches")
	journal, err := os.OpenFile("launches", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		_, _ = journal.WriteString("launch\n")
		_ = journal.Close()
	}
	if len(previous) == 0 {
		panic("gauge: calibration table not ready")
	}
	return true
}
`

// calibrationThatHangs keeps the binary of the gauge package in its start-up
// for longer than a run may take.
const calibrationThatHangs = `package gauge

import "time"

var calibrated = calibrate()

func calibrate() bool {
	time.Sleep(8 * time.Second)
	return true
}
`

// faultReadings declares a test that panics between two that pass.
const faultReadings = `package gauge

import "testing"

func TestFirstReading(t *testing.T) {
	if Reading(1) != 1 {
		t.Fatal("first reading")
	}
}

func TestReadingOfAFault(t *testing.T) {
	var faults map[string]int
	faults["sensor"]++
}

func TestLastReading(t *testing.T) {
	if Reading(2) != 2 {
		t.Fatal("last reading")
	}
}
`

// brokenRateTest does not compile.
const brokenRateTest = `package rate

var single = Scale(1)
`

func openMeter(t *testing.T) scanProject {
	t.Helper()
	return openScanModule(t, meterModule)
}

// withFile is a clone of the project's workspace with one more file.
func (p scanProject) withFile(t *testing.T, file, content string) ecosystem.Workspace {
	t.Helper()
	clone, err := p.manager.Clone(context.Background(), p.workspace, p.layout)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.manager.WriteFile(clone, p.layout, file, []byte(content)); err != nil {
		t.Fatal(err)
	}
	return clone
}

func (p scanProject) run(t *testing.T, on ecosystem.Workspace, timeout time.Duration, tests ...ecosystem.TestID) map[ecosystem.TestID][]vacuity.Run {
	t.Helper()
	ran, err := p.suite.Run(context.Background(), on, p.toolchain, ports.ScanRunRequest{Tests: tests, Runs: vacuity.RunsPerOutcome, Timeout: timeout})
	if err != nil {
		t.Fatal(err)
	}
	return ran.Runs
}

func TestScanRunsOfABinaryThatStops(t *testing.T) {
	project := openMeter(t)

	t.Run("a test-file variable that panics stops three fresh processes, and every run did not start"+verifiesStartUp, func(t *testing.T) {
		stopping := project.withFile(t, calibrationFile, calibrationThatFails)
		runs := project.run(t, stopping, scanRunTimeout, keepsPositive, clampsNegative)
		for _, test := range []ecosystem.TestID{keepsPositive, clampsNegative} {
			if !slices.Equal(runs[test], runsOf(vacuity.RunNotStarted, vacuity.RunsPerOutcome)) {
				t.Fatalf("%s: %v", test, runs[test])
			}
		}
		journal, err := project.manager.ReadFile(stopping, launchJournal)
		if err != nil {
			t.Fatal(err)
		}
		if launches := strings.Count(string(journal), "\n"); launches != vacuity.RunsPerOutcome {
			t.Fatalf("%d processes started", launches)
		}
		if vacuity.Settle(runs[keepsPositive]) != vacuity.SettledNotStarted {
			t.Fatalf("settled as %s", vacuity.Settle(runs[keepsPositive]))
		}
	})

	t.Run("a binary that stops at its first start only settles inconclusive"+verifiesStartUp, func(t *testing.T) {
		runs := project.run(t, project.withFile(t, calibrationFile, calibrationThatFailsFirst), scanRunTimeout, keepsPositive)
		want := []vacuity.Run{vacuity.RunNotStarted, vacuity.RunPassed, vacuity.RunPassed}
		if !slices.Equal(runs[keepsPositive], want) || vacuity.Settle(runs[keepsPositive]) != vacuity.SettledInconclusive {
			t.Fatalf("runs = %v", runs[keepsPositive])
		}
	})

	t.Run("a start-up that outlives the timeout times every run out"+verifiesStartUp, func(t *testing.T) {
		runs := project.run(t, project.withFile(t, calibrationFile, calibrationThatHangs), startUpRunTimeout, keepsPositive)
		if !slices.Equal(runs[keepsPositive], runsOf(vacuity.RunTimedOut, vacuity.RunsPerOutcome)) {
			t.Fatalf("runs = %v", runs[keepsPositive])
		}
	})

	t.Run("a test that panics is killed in every run, a test before it passes and a test after it errored"+verifiesStartUp, func(t *testing.T) {
		const file = "gauge/readings_test.go"
		first, fault, last := ecosystem.TestID(file+"::TestFirstReading"), ecosystem.TestID(file+"::TestReadingOfAFault"), ecosystem.TestID(file+"::TestLastReading")
		runs := project.run(t, project.withFile(t, file, faultReadings), scanRunTimeout, first, fault, last)
		if !slices.Equal(runs[fault], runsOf(vacuity.RunFailed, vacuity.RunsPerOutcome)) || vacuity.Settle(runs[fault]) != vacuity.SettledKilled {
			t.Fatalf("panicking test: %v", runs[fault])
		}
		if !slices.Equal(runs[first], runsOf(vacuity.RunPassed, vacuity.RunsPerOutcome)) {
			t.Fatalf("test before the panic: %v", runs[first])
		}
		if !slices.Equal(runs[last], runsOf(vacuity.RunErrored, vacuity.RunsPerOutcome)) {
			t.Fatalf("test after the panic: %v", runs[last])
		}
	})

	t.Run("a test file of another package that does not compile errors that package's tests alone"+verifiesStartUp, func(t *testing.T) {
		runs := project.run(t, project.withFile(t, "rate/single_test.go", brokenRateTest), scanRunTimeout, scaleDoubles, keepsPositive)
		if !slices.Equal(runs[scaleDoubles], runsOf(vacuity.RunErrored, vacuity.RunsPerOutcome)) {
			t.Fatalf("test of the broken package: %v", runs[scaleDoubles])
		}
		if !slices.Equal(runs[keepsPositive], runsOf(vacuity.RunPassed, vacuity.RunsPerOutcome)) {
			t.Fatalf("test of the other package: %v", runs[keepsPositive])
		}
	})
}

func TestScanLineCoverageOwnership(t *testing.T) {
	project := openMeter(t)
	covered, err := project.suite.LineCoverage(context.Background(), project.workspace, project.toolchain, ports.ScanCoverageRequest{
		Tests: []ports.ScanCase{
			{ID: scaleDoubles, File: "rate/rate_test.go"}, {ID: scaleFromWorkers, File: "rate/rate_test.go"}, {ID: unitIsOne, File: "rate/rate_test.go"},
		},
		Timeout: scanRunTimeout,
		Workers: coverageWorkers,
	})
	if err != nil {
		t.Fatal(err)
	}
	multiply := vacuity.Line{File: "rate/rate.go", Line: 8}
	t.Run("a function run at start-up and in a test's body is owned by that test"+verifiesOwnership, func(t *testing.T) {
		if !slices.Contains(covered[scaleDoubles].Lines, multiply) || !slices.Contains(covered[scaleDoubles].Owned, multiply) {
			t.Fatalf("coverage = %+v", covered[scaleDoubles])
		}
	})
	t.Run("a function a test's body runs in goroutines is owned by that test"+verifiesOwnership, func(t *testing.T) {
		if !slices.Contains(covered[scaleFromWorkers].Owned, multiply) {
			t.Fatalf("coverage = %+v", covered[scaleFromWorkers])
		}
	})
	t.Run("a function run only at start-up is covered by a test that does not own it"+verifiesOwnership, func(t *testing.T) {
		if !slices.Contains(covered[unitIsOne].Lines, multiply) || slices.Contains(covered[unitIsOne].Owned, multiply) {
			t.Fatalf("coverage = %+v", covered[unitIsOne])
		}
	})
}
