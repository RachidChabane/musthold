package golang

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
)

// The actions of `go test -json` a scan reads.
const (
	// actionPass, actionFail and actionSkip end one run of a test, or, on an
	// event without a test, the run of a package's test binary.
	actionPass = "pass"
	actionFail = "fail"
	actionSkip = "skip"
	// actionBuildFail reports that a test binary did not compile.
	actionBuildFail = "build-fail"
)

// maxEventLine bounds one line of `go test -json` output.
const maxEventLine = 16 << 20

// testEvent is one event of `go test -json`.
type testEvent struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
	// FailedBuild names the test binary that did not compile, on the event
	// that ends a package whose build failed.
	FailedBuild string `json:"FailedBuild"`
}

// resultKey names a test by its package import path and its subtest path.
type resultKey struct {
	pkg  string
	test string
}

// testProcess is what one `go test -json` process reported: the end events of
// each leaf test, what its events say of each package's test binary, and how
// the process ended.
type testProcess struct {
	leaves   map[resultKey][]vacuity.Run
	packages map[string]binaryReport
	// buildFailed is set when an event reports a test binary that did not
	// compile.
	buildFailed bool
	timedOut    bool
	exitCode    int
}

// binaryReport is what the events of one package say of its test binary.
type binaryReport struct {
	// testStarted is set when an event of the package names a test.
	testStarted bool
	// failed is set when the package's binary ended in a failure.
	failed bool
}

// readProcess reads the events of one process and how it ended.
func readProcess(output []byte, exitCode int, timedOut bool) testProcess {
	process := testProcess{leaves: map[resultKey][]vacuity.Run{}, packages: map[string]binaryReport{}, timedOut: timedOut, exitCode: exitCode}
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), maxEventLine)
	for scanner.Scan() {
		var event testEvent
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			continue
		}
		process.read(event)
	}
	for key := range process.leaves {
		if isParent(key, process.leaves) {
			delete(process.leaves, key)
		}
	}
	return process
}

func (p *testProcess) read(event testEvent) {
	if event.Action == actionBuildFail || event.FailedBuild != "" {
		p.buildFailed = true
	}
	if event.Package == "" {
		return
	}
	report := p.packages[event.Package]
	if event.Test != "" {
		report.testStarted = true
		if run, ends := runOfAction(event.Action); ends {
			key := resultKey{pkg: event.Package, test: event.Test}
			p.leaves[key] = append(p.leaves[key], run)
		}
	} else if event.Action == actionFail {
		report.failed = true
	}
	p.packages[event.Package] = report
}

// cutShort reports whether the process exited non-zero, without timing out,
// before every one of the tests had the wanted number of end events: it then
// gives each test one observation, not that many runs.
func (p testProcess) cutShort(keys []resultKey, runs int) bool {
	if p.timedOut || p.exitCode == 0 {
		return false
	}
	for _, key := range keys {
		if len(p.leaves[key]) < runs {
			return true
		}
	}
	return false
}

// runsOf gives the test its end events, at most the wanted number, and the
// runs the process left without one: timed out after a timeout, errored
// otherwise.
func (p testProcess) runsOf(key resultKey, runs int) []vacuity.Run {
	result := append([]vacuity.Run(nil), p.leaves[key][:min(len(p.leaves[key]), runs)]...)
	missing := vacuity.RunErrored
	if p.timedOut {
		missing = vacuity.RunTimedOut
	}
	for len(result) < runs {
		result = append(result, missing)
	}
	return result
}

// observation is the one run the process gives the test when it counts as one
// observation: its first end event; timed out when the process timed out; not
// started when the test's package binary stopped before any of its tests
// began; errored otherwise.
func (p testProcess) observation(key resultKey) vacuity.Run {
	switch {
	case len(p.leaves[key]) > 0:
		return p.leaves[key][0]
	case p.timedOut:
		return vacuity.RunTimedOut
	case p.stoppedBeforeTests(key.pkg):
		return vacuity.RunNotStarted
	}
	return vacuity.RunErrored
}

// stoppedBeforeTests reports the positive evidence that a package's test
// binary stopped before any test began: the process exited non-zero without
// timing out, the binary of the package ended in a failure, no event of the
// package names a test, and no event reports a build failure.
func (p testProcess) stoppedBeforeTests(pkg string) bool {
	report, ran := p.packages[pkg]
	return ran && p.exitCode != 0 && !p.timedOut && !p.buildFailed && report.failed && !report.testStarted
}

func runOfAction(action string) (vacuity.Run, bool) {
	switch action {
	case actionPass:
		return vacuity.RunPassed, true
	case actionFail:
		return vacuity.RunFailed, true
	case actionSkip:
		return vacuity.RunErrored, true
	}
	return "", false
}

// isParent reports whether a subtest of the test has results: a parent's own
// result is the merge of its subtests' and is never read.
func isParent(key resultKey, results map[resultKey][]vacuity.Run) bool {
	for other := range results {
		if other.pkg == key.pkg && strings.HasPrefix(other.test, key.test+subtestSeparator) {
			return true
		}
	}
	return false
}
