package node_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/ecosystem/node"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/process"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/sourceparse/treesitter"
	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/footprint"
	"github.com/RachidChabane/musthold/discpower/internal/domain/vacuity"
	"github.com/RachidChabane/musthold/discpower/internal/testkit/nodeinstall"
)

const (
	scanKit        = "testdata/scankit"
	ledgerFile     = "src/ledger.ts"
	scanRunTimeout = 2 * time.Minute
	scanWorkers    = 2
	// withoutVariable switches off, by name, preconditions the shipped agent
	// otherwise checks for itself.
	withoutVariable = "MUSTHOLD_SCAN_WITHOUT"

	verifiesScanRuns      = " [verifies SP-2292~1]"
	verifiesScanOwnership = " [verifies SP-2293~1]"
	verifiesScanFallbacks = " [verifies SP-2294~1]"
	verifiesScanSegments  = " [verifies SP-2295~1]"
	verifiesScanWitnesses = " [verifies SP-2296~1]"
	verifiesScanBrowser   = " [verifies SP-2297~1]"
	// verifiesReferenceEnding cites how a scan tells a suite with no test
	// from a runner that failed.
	verifiesReferenceEnding = " [verifies SP-2108~3]"
	verifiesRecordingsRead  = " [verifies SP-2656~1]"
)

// The lines of the ledger module the kit's tests run.
const (
	creditLine   = 2
	routeLine    = 6
	expressLine  = 10
	standardLine = 14
	tickLine     = 18
	lateLine     = 22
	auditLine    = 26
)

// Tests of the kit.
const (
	credits             = "test/ownership.test.ts::ownership > credits"
	routesUrgent        = "test/ownership.test.ts::ownership > routes urgent"
	waitsPastATimer     = "test/ownership.test.ts::ownership > waits past a timer"
	leavesWorkBehind    = "test/ownership.test.ts::ownership > leaves work behind"
	loadsLazily         = "test/ownership.test.ts::ownership > loads lazily"
	creditsConcurrently = "test/ownership.test.ts::ownership > credits concurrently"
	timerItDidNotSet    = "test/timers.test.ts::fake timers set before the test > advances a timer it did not set"
	itsOwnTimer         = "test/timers.test.ts::fake timers of the test > advances its own timer"
	legacyTimer         = "test/legacy.test.ts::legacy fake timers set before the test > advances a legacy timer it did not set"
	readsAfterSetup     = "test/setup.test.ts::setup > reads after setup"
	neverRuns           = "test/outcomes.test.ts::a failing beforeAll > never runs"
	passesItsBody       = "test/outcomes.test.ts::a failing afterEach > passes its body"
	fails               = "test/outcomes.test.ts::fails"
	twin                = "test/outcomes.test.ts::twin"
	upperCase           = "test/outcomes.test.ts::Case"
	flaky               = "test/flaky.test.ts::passes on its second attempt"
	readsTheArchive     = "test/stopped.test.ts::reads the archive"
)

// scanRunner is a Node runner a scan runs, with the installed project whose
// node_modules its kit links to.
type scanRunner struct {
	name      ecosystem.Name
	installed string
	strategy  func(node.Dependencies) *node.Strategy
}

func scanRunners() []scanRunner {
	return []scanRunner{
		{name: ecosystem.Jest, installed: jestFixture, strategy: node.NewJest},
		{name: ecosystem.Vitest, installed: vitestFixture, strategy: node.NewVitest},
	}
}

// scanned is a copy of the runner's scan kit in a workspace of the test.
type scanned struct {
	strategy  *node.Strategy
	suite     ports.ScanSuite
	workspace ecosystem.Workspace
	toolchain ecosystem.Toolchain
	cases     map[ecosystem.TestID]ports.ScanCase
}

func scanProject(t *testing.T, runner scanRunner) *scanned {
	t.Helper()
	modules := filepath.Join(nodeinstall.Installed(t, runner.installed), nodeModules)
	source := filepath.Join(scanKit, string(runner.name))
	dir := t.TempDir()
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		relative, _ := filepath.Rel(source, path)
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		target := filepath.Join(dir, relative)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, content, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(modules, filepath.Join(dir, nodeModules)); err != nil {
		t.Fatal(err)
	}
	workspace := ecosystem.Workspace{Dir: dir, ResultsDir: t.TempDir()}
	strategy := runner.strategy(node.Dependencies{Process: process.New(), Locator: treesitter.NewLocator()})
	toolchain, err := strategy.Toolchain().Resolve(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	return &scanned{strategy: strategy, suite: strategy.Scan(), workspace: workspace, toolchain: toolchain}
}

func (s *scanned) run(t *testing.T, tests ...ecosystem.TestID) ports.ScanRuns {
	t.Helper()
	ran, err := s.suite.Run(context.Background(), s.workspace, s.toolchain, ports.ScanRunRequest{Tests: tests, Runs: vacuity.RunsPerOutcome, Timeout: scanRunTimeout})
	if err != nil {
		t.Fatal(err)
	}
	return ran
}

// coverage records the tests, the owners alone.
func (s *scanned) coverage(t *testing.T, tests []ecosystem.TestID, owners ...ecosystem.TestID) map[ecosystem.TestID]vacuity.Coverage {
	t.Helper()
	cases := make([]ports.ScanCase, 0, len(tests))
	for _, test := range tests {
		cases = append(cases, ports.ScanCase{ID: test, File: string(test)[:slices.Index([]byte(test), ':')]})
	}
	covered, err := s.suite.LineCoverage(context.Background(), s.workspace, s.toolchain, ports.ScanCoverageRequest{Tests: cases, Timeout: scanRunTimeout, Workers: scanWorkers, Owners: owners})
	if err != nil {
		t.Fatal(err)
	}
	return covered
}

// alone records each test alone.
func (s *scanned) alone(t *testing.T, tests ...ecosystem.TestID) map[ecosystem.TestID]vacuity.Coverage {
	t.Helper()
	return s.coverage(t, tests, tests...)
}

func ledger(line int) vacuity.Line { return vacuity.Line{File: ledgerFile, Line: line} }

func expectLines(t *testing.T, name string, lines []vacuity.Line, holds, lacks []int) {
	t.Helper()
	for _, line := range holds {
		if !slices.Contains(lines, ledger(line)) {
			t.Errorf("%s lacks line %d: %v", name, line, lines)
		}
	}
	for _, line := range lacks {
		if slices.Contains(lines, ledger(line)) {
			t.Errorf("%s holds line %d: %v", name, line, lines)
		}
	}
}

func TestScanRunsOfJestAndVitest(t *testing.T) {
	for _, runner := range scanRunners() {
		t.Run(string(runner.name), func(t *testing.T) {
			s := scanProject(t, runner)
			ran := s.run(t)
			runs := func(test ecosystem.TestID) []vacuity.Run { return ran.Runs[test] }
			all := func(run vacuity.Run) []vacuity.Run { return slices.Repeat([]vacuity.Run{run}, vacuity.RunsPerOutcome) }
			t.Run("a run of the whole suite names each test by its file and titles, and a passing test passes each of its three runs"+verifiesScanRuns, func(t *testing.T) {
				if !slices.Equal(runs(credits), all(vacuity.RunPassed)) || !slices.Equal(runs(readsAfterSetup), all(vacuity.RunPassed)) {
					t.Fatalf("runs %v", ran.Runs)
				}
			})
			t.Run("a failing test function fails; a hook failure, a retried test and a shared id are errored"+verifiesScanRuns, func(t *testing.T) {
				if !slices.Equal(runs(fails), all(vacuity.RunFailed)) {
					t.Errorf("fails: %v", runs(fails))
				}
				for _, test := range []ecosystem.TestID{neverRuns, passesItsBody, flaky, twin} {
					if !slices.Equal(runs(test), all(vacuity.RunErrored)) {
						t.Errorf("%s: %v", test, runs(test))
					}
				}
			})
			t.Run("ids equal ignoring case are errored for Jest only"+verifiesScanRuns, func(t *testing.T) {
				want := vacuity.RunPassed
				if runner.name == ecosystem.Jest {
					want = vacuity.RunErrored
				}
				if !slices.Equal(runs(upperCase), all(want)) {
					t.Fatalf("%s: %v", upperCase, runs(upperCase))
				}
			})
			t.Run("a test of a file whose import throws did not start, and a selection runs its tests alone"+verifiesScanRuns, func(t *testing.T) {
				selected := s.run(t, readsTheArchive, credits)
				if !slices.Equal(selected.Runs[readsTheArchive], all(vacuity.RunNotStarted)) || !slices.Equal(selected.Runs[credits], all(vacuity.RunPassed)) {
					t.Fatalf("runs %v", selected.Runs)
				}
				if _, ran := selected.Runs[fails]; ran {
					t.Fatalf("a test outside the selection has runs: %v", selected.Runs)
				}
			})
			t.Run("a test of a run whose global setup throws did not start"+verifiesScanRuns, func(t *testing.T) {
				t.Setenv("SCANKIT_FAIL_SETUP", "1")
				failed := s.run(t, credits)
				if !slices.Equal(failed.Runs[credits], all(vacuity.RunNotStarted)) {
					t.Fatalf("runs %v", failed.Runs)
				}
			})
		})
	}
}

func TestScanRunnerEndingsOnJestAndVitest(t *testing.T) {
	for _, runner := range scanRunners() {
		t.Run(string(runner.name), func(t *testing.T) {
			t.Run("a suite with no test file reports no test and no failure"+verifiesReferenceEnding, func(t *testing.T) {
				s := scanProject(t, runner)
				testFiles, err := filepath.Glob(filepath.Join(s.workspace.Dir, "test", "*.test.ts"))
				if err != nil || len(testFiles) == 0 {
					t.Fatalf("test files %v: %v", testFiles, err)
				}
				for _, file := range testFiles {
					if err := os.Remove(file); err != nil {
						t.Fatal(err)
					}
				}
				ran := s.run(t)
				if len(ran.Cases) != 0 || ran.Failure != "" || ran.Killed {
					t.Fatalf("cases %v, failure %q, killed %v", ran.Cases, ran.Failure, ran.Killed)
				}
			})
			t.Run("a run of the whole suite whose global setup throws reports no test and how the runner ended"+verifiesReferenceEnding, func(t *testing.T) {
				t.Setenv("SCANKIT_FAIL_SETUP", "1")
				ran := scanProject(t, runner).run(t)
				if len(ran.Cases) != 0 || !strings.Contains(ran.Failure, "exited 1") || ran.Killed {
					t.Fatalf("cases %v, failure %q, killed %v", ran.Cases, ran.Failure, ran.Killed)
				}
			})
		})
	}
}

func TestScanRecordingKilledFromOutside(t *testing.T) {
	t.Run("a recording of a Vitest test whose runner SIGKILL ended from outside fails as a killed runner [verifies SP-2658~1]", func(t *testing.T) {
		suite := node.NewVitest(node.Dependencies{Process: process.New(), Locator: treesitter.NewLocator()}).Scan()
		workspace := ecosystem.Workspace{Dir: t.TempDir(), ResultsDir: t.TempDir()}
		killed := ecosystem.Toolchain{Command: []string{"sh", "-c", "kill -KILL $$", "npx"}}
		request := ports.ScanCoverageRequest{Tests: []ports.ScanCase{{ID: credits, File: "test/ownership.test.ts"}}, Timeout: scanRunTimeout, Workers: 1}
		if _, err := suite.LineCoverage(context.Background(), workspace, killed, request); !errors.Is(err, ports.ErrRunnerKilled) {
			t.Fatalf("error %v", err)
		}
	})
}

func TestScanLetsGoOfItsRecordings(t *testing.T) {
	for _, runner := range scanRunners() {
		t.Run(string(runner.name)+": a recording of tests alone keeps their lines and leaves no record behind"+verifiesRecordingsRead, func(t *testing.T) {
			s := scanProject(t, runner)
			covered := s.alone(t, credits, routesUrgent)
			expectLines(t, credits, covered[credits].Lines, []int{creditLine}, nil)
			var records []string
			err := filepath.WalkDir(s.workspace.ResultsDir, func(path string, entry fs.DirEntry, walkErr error) error {
				if walkErr == nil && !entry.IsDir() && filepath.Ext(path) == ".jsonl" {
					records = append(records, path)
				}
				return walkErr
			})
			if err != nil || len(records) != 0 {
				t.Fatalf("records left %v: %v", records, err)
			}
		})
	}
}

func TestScanOwnershipOnJestAndVitest(t *testing.T) {
	for _, runner := range scanRunners() {
		t.Run(string(runner.name), func(t *testing.T) {
			s := scanProject(t, runner)
			covered := s.alone(t, credits, routesUrgent, waitsPastATimer, leavesWorkBehind, loadsLazily, creditsConcurrently, timerItDidNotSet, itsOwnTimer, readsAfterSetup)
			t.Run("a test owns the lines its body and its beforeEach ran, never those only afterEach ran"+verifiesScanOwnership, func(t *testing.T) {
				expectLines(t, "credits owned", covered[credits].Owned, []int{creditLine}, []int{auditLine, standardLine, routeLine})
				expectLines(t, "credits ran", covered[credits].Lines, []int{creditLine, auditLine, standardLine}, nil)
				expectLines(t, "reads after setup owned", covered[readsAfterSetup].Owned, []int{creditLine, expressLine}, nil)
			})
			t.Run("a line with a branch only afterEach takes is not owned"+verifiesScanOwnership, func(t *testing.T) {
				expectLines(t, "routes urgent owned", covered[routesUrgent].Owned, []int{expressLine}, []int{routeLine, standardLine})
			})
			t.Run("a timer beforeAll set that fires in the window is cut out of it, and work left for after the window is only run"+verifiesScanOwnership, func(t *testing.T) {
				expectLines(t, "waits past a timer owned", covered[waitsPastATimer].Owned, []int{creditLine}, []int{tickLine})
				expectLines(t, "waits past a timer ran", covered[waitsPastATimer].Lines, []int{tickLine}, nil)
				expectLines(t, "leaves work behind owned", covered[leavesWorkBehind].Owned, []int{creditLine}, []int{lateLine})
				expectLines(t, "leaves work behind ran", covered[leavesWorkBehind].Lines, []int{lateLine}, nil)
			})
			t.Run("a test that evaluates a module in its window owns nothing"+verifiesScanOwnership, func(t *testing.T) {
				if owned := covered[loadsLazily].Owned; len(owned) != 0 {
					t.Fatalf("owned %v", owned)
				}
			})
			t.Run("a concurrent test recorded alone owns its body"+verifiesScanOwnership, func(t *testing.T) {
				expectLines(t, "credits concurrently owned", covered[creditsConcurrently].Owned, []int{creditLine}, nil)
			})
			t.Run("a fake timer pending when the window opens is cut out of it, one the body sets and advances is owned"+verifiesScanOwnership, func(t *testing.T) {
				expectLines(t, "timer it did not set owned", covered[timerItDidNotSet].Owned, []int{creditLine}, []int{tickLine})
				expectLines(t, "its own timer owned", covered[itsOwnTimer].Owned, []int{tickLine}, nil)
			})
			t.Run("a test whose lines are read without its ownership gets its file's segment and owns nothing"+verifiesScanSegments, func(t *testing.T) {
				together := s.coverage(t, []ecosystem.TestID{credits, fails}, credits)
				expectLines(t, "fails ran", together[fails].Lines, []int{creditLine}, nil)
				if len(together[fails].Owned) != 0 {
					t.Fatalf("owned %v", together[fails].Owned)
				}
				for _, line := range together[credits].Lines {
					if line.File != ledgerFile {
						t.Fatalf("a test file is reported: %v", line)
					}
				}
			})
		})
	}
}

func TestScanCutsOutJestLegacyFakeTimers(t *testing.T) {
	t.Run("a legacy fake timer pending when the window opens is cut out of it"+verifiesScanOwnership, func(t *testing.T) {
		s := scanProject(t, scanRunners()[0])
		expectLines(t, "legacy timer owned", s.alone(t, legacyTimer)[legacyTimer].Owned, []int{creditLine}, []int{tickLine})
	})
}

func TestScanFallbacksOnJestAndVitest(t *testing.T) {
	for _, runner := range scanRunners() {
		t.Run(string(runner.name), func(t *testing.T) {
			s := scanProject(t, runner)
			t.Run("without an inspector session a test runs the whole of each file it ran and owns nothing"+verifiesScanFallbacks, func(t *testing.T) {
				t.Setenv(withoutVariable, "inspector")
				covered := s.alone(t, credits)[credits]
				expectLines(t, "credits ran", covered.Lines, []int{creditLine, lateLine}, nil)
				if len(covered.Owned) != 0 {
					t.Fatalf("owned %v", covered.Owned)
				}
			})
			t.Run("without synchronous takes the window opens at the test function, so beforeEach is not owned, and a test whose realm fakes timers when its window opens owns nothing"+verifiesScanFallbacks, func(t *testing.T) {
				t.Setenv(withoutVariable, "synchronousTake")
				covered := s.alone(t, readsAfterSetup, timerItDidNotSet)
				expectLines(t, "reads after setup owned", covered[readsAfterSetup].Owned, []int{expressLine}, []int{creditLine})
				if owned := covered[timerItDidNotSet].Owned; len(owned) != 0 {
					t.Fatalf("owned %v", owned)
				}
			})
			t.Run("where the async hooks are blind a line is owned when it ran more often than in hollow runs of the test"+verifiesScanFallbacks, func(t *testing.T) {
				t.Setenv(withoutVariable, "asyncHooks")
				expectLines(t, "reads after setup owned", s.alone(t, readsAfterSetup)[readsAfterSetup].Owned, []int{expressLine}, []int{creditLine})
			})
			for _, missing := range []string{"evaluation", "retention", "maps"} {
				t.Run("without "+missing+" a test owns nothing"+verifiesScanFallbacks, func(t *testing.T) {
					t.Setenv(withoutVariable, missing)
					if owned := s.alone(t, credits)[credits].Owned; len(owned) != 0 {
						t.Fatalf("owned %v", owned)
					}
				})
			}
			t.Run("without enumerable fake timers a test whose realm fakes timers when its window opens owns nothing"+verifiesScanFallbacks, func(t *testing.T) {
				t.Setenv(withoutVariable, "fakeTimers")
				covered := s.alone(t, timerItDidNotSet, credits)
				if owned := covered[timerItDidNotSet].Owned; len(owned) != 0 {
					t.Fatalf("owned %v", owned)
				}
				expectLines(t, "credits owned", covered[credits].Owned, []int{creditLine}, nil)
			})
			t.Run("without bounded windows a concurrent test owns nothing"+verifiesScanFallbacks, func(t *testing.T) {
				t.Setenv(withoutVariable, "bounds")
				covered := s.alone(t, creditsConcurrently, credits)
				if owned := covered[creditsConcurrently].Owned; len(owned) != 0 {
					t.Fatalf("owned %v", owned)
				}
				expectLines(t, "credits owned", covered[credits].Owned, []int{creditLine}, nil)
			})
		})
	}
}

func TestScanWitnessCoverageOnJestAndVitest(t *testing.T) {
	for _, runner := range scanRunners() {
		t.Run(string(runner.name)+": a witness records the definitions it runs in its window, alone"+verifiesScanWitnesses, func(t *testing.T) {
			s := scanProject(t, runner)
			test := ecosystem.BoundTest{ID: "credits", File: "test/ownership.test.ts", Name: "ownership > credits"}
			recorded, err := s.strategy.WitnessCoverage().Record(context.Background(), s.workspace, s.toolchain, []ecosystem.BoundTest{test})
			if err != nil {
				t.Fatal(err)
			}
			units := recorded[test.ID]
			if !slices.Contains(units, footprint.Ref{File: ledgerFile, Qualname: "credit"}) || slices.Contains(units, footprint.Ref{File: ledgerFile, Qualname: "audit"}) {
				t.Fatalf("units %v", units)
			}
		})
	}
}

func TestScanRefusesVitestBrowserMode(t *testing.T) {
	t.Run("a Vitest project that runs its tests in a browser is refused with the reason"+verifiesScanBrowser, func(t *testing.T) {
		s := scanProject(t, scanRunners()[1])
		t.Setenv("SCANKIT_BROWSER", "1")
		_, err := s.suite.Run(context.Background(), s.workspace, s.toolchain, ports.ScanRunRequest{Runs: 1, Timeout: scanRunTimeout})
		if !errors.Is(err, node.ErrBrowserMode) {
			t.Fatalf("err %v", err)
		}
	})
}
