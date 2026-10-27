package golang

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/domain/vacuity"
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
// each leaf test and of each test with subtests, the tests its events name,
// what its events say of each package's test binary, and how the process
// ended.
type testProcess struct {
	leaves   map[resultKey][]vacuity.Run
	parents  map[resultKey][]vacuity.Run
	named    map[resultKey]bool
	packages map[string]binaryReport
	// buildFailed is set when an event reports a test binary that did not
	// compile.
	buildFailed bool
	timedOut    bool
	exitCode    int
	// killed is whether SIGKILL that no deadline sent ended the process.
	killed bool
	// failure describes a process that did not exit with 0; empty otherwise.
	failure string
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
	process := testProcess{
		leaves: map[resultKey][]vacuity.Run{}, parents: map[resultKey][]vacuity.Run{}, named: map[resultKey]bool{},
		packages: map[string]binaryReport{}, timedOut: timedOut, exitCode: exitCode,
	}
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
			process.parents[key] = process.leaves[key]
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
		key := resultKey{pkg: event.Package, test: event.Test}
		p.named[key] = true
		if run, ends := runOfAction(event.Action); ends {
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

// runsOf gives the test its end events, at most the wanted number, each paired
// with its ancestors' end events of the same iteration (see paired), and the
// runs the process left without one: timed out after a timeout, errored
// otherwise. It also reports whether a pass was ambiguous in some iteration.
func (p testProcess) runsOf(key resultKey, runs int) ([]vacuity.Run, bool) {
	ends := p.leaves[key]
	result := make([]vacuity.Run, 0, runs)
	ambiguous := false
	for iteration := range min(len(ends), runs) {
		run, unclear := p.paired(key, iteration, ends[iteration])
		result = append(result, run)
		ambiguous = ambiguous || unclear
	}
	for len(result) < runs {
		result = append(result, p.missingRun())
	}
	return result, ambiguous
}

// missingRun is a run the process did not give a test: timed out after a
// timeout, errored otherwise.
func (p testProcess) missingRun() vacuity.Run {
	if p.timedOut {
		return vacuity.RunTimedOut
	}
	return vacuity.RunErrored
}

// observation is the one run the process gives the test when it counts as one
// observation: its first end event, paired with its ancestors' first end
// events; timed out when the process timed out; not started when the test's
// package binary stopped before any of its tests began; errored otherwise. It
// also reports whether a pass was ambiguous.
func (p testProcess) observation(key resultKey) (vacuity.Run, bool) {
	switch {
	case len(p.leaves[key]) > 0:
		return p.paired(key, 0, p.leaves[key][0])
	case p.timedOut:
		return vacuity.RunTimedOut, false
	case p.stoppedBeforeTests(key.pkg):
		return vacuity.RunNotStarted, false
	}
	return vacuity.RunErrored, false
}

// paired reads a leaf's run of one iteration against the end events its
// ancestors gave in that iteration. A pass whose ancestors all passed stays a
// pass. A pass under an ancestor that did not pass, while no other descendant
// of that ancestor failed in the iteration, is errored, or timed out after a
// timeout: the ancestor failed on its own, so the pass is no pass, and never a
// failure. A pass under failing ancestors that each have another descendant
// that failed in the iteration is ambiguous, since they may have failed only
// through those descendants: it counts as errored, or timed out, and is
// reported ambiguous unless the process timed out, so that the leaf may run
// again without them.
func (p testProcess) paired(key resultKey, iteration int, run vacuity.Run) (vacuity.Run, bool) {
	if run != vacuity.RunPassed {
		return run, false
	}
	failing := p.failingAncestors(key, iteration)
	if len(failing) == 0 {
		return run, false
	}
	for _, ancestor := range failing {
		if !p.otherDescendantFailed(ancestor, key, iteration) {
			return p.missingRun(), false
		}
	}
	return p.missingRun(), !p.timedOut
}

// failingAncestors are the ancestors of the leaf that did not pass in the
// iteration: each test the events name whose subtest path the leaf's extends,
// with a failure, or no end event, in that iteration.
func (p testProcess) failingAncestors(key resultKey, iteration int) []resultKey {
	var failing []resultKey
	for _, ancestor := range ancestorsOf(key) {
		if !p.named[ancestor] {
			continue
		}
		if ends := p.parents[ancestor]; len(ends) <= iteration || ends[iteration] != vacuity.RunPassed {
			failing = append(failing, ancestor)
		}
	}
	return failing
}

// otherDescendantFailed reports whether a test below the ancestor, neither the
// leaf nor one of the leaf's ancestors, failed in the iteration.
func (p testProcess) otherDescendantFailed(ancestor, leaf resultKey, iteration int) bool {
	for _, ends := range []map[resultKey][]vacuity.Run{p.leaves, p.parents} {
		for other, runs := range ends {
			if other == leaf || !isAncestor(ancestor, other) || isAncestor(other, leaf) {
				continue
			}
			if len(runs) > iteration && runs[iteration] == vacuity.RunFailed {
				return true
			}
		}
	}
	return false
}

// ancestorsOf are the subtest paths the leaf's path extends, shallowest first.
func ancestorsOf(key resultKey) []resultKey {
	var ancestors []resultKey
	for index := range len(key.test) {
		if strings.HasPrefix(key.test[index:], subtestSeparator) {
			ancestors = append(ancestors, resultKey{pkg: key.pkg, test: key.test[:index]})
		}
	}
	return ancestors
}

// isAncestor reports whether the test's subtest path is extended by the other's.
func isAncestor(test, other resultKey) bool {
	return test.pkg == other.pkg && strings.HasPrefix(other.test, test.test+subtestSeparator)
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
// result is the merge of its subtests' and is read only against them.
func isParent(key resultKey, results map[resultKey][]vacuity.Run) bool {
	for other := range results {
		if isAncestor(key, other) {
			return true
		}
	}
	return false
}
