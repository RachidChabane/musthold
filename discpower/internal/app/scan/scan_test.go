package scan_test

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io/fs"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/scan"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/scanscope"
	"github.com/RachidChabane/spec-proof/format"
)

var updateGolden = flag.Bool("update", false, "rewrite the golden scan statement")

const (
	goldenStatement   = "testdata/scan-repository.golden.json"
	verifiesReference = " [verifies SP-2108~2]"
	verifiesPlan      = " [verifies SP-2109~1]"
	verifiesDraw      = " [verifies SP-2110~2]"
	verifiesExecute   = " [verifies SP-2111~2]"
	verifiesSearch    = " [verifies SP-2112~1]"
	verifiesRecord    = " [verifies SP-2113~1]"
	verifiesReplay    = " [verifies SP-2114~1]"
	verifiesContract  = " [verifies SP-2123~2]"
	verifiesAllocated = " [verifies SP-2125~2]"
	verifiesSentinels = " [verifies SP-2126~1]"
	verifiesCoverage  = " [verifies SP-2127~1]"

	// inFlightBudget leaves the mutants a fraction of a second, and
	// inFlightBound is how long a scan stopped by it may take.
	inFlightBudget = 300 * time.Millisecond
	inFlightBound  = 10 * time.Second
)

func run(t *testing.T, f *fixture, req scan.Request) scan.Report {
	t.Helper()
	report, err := f.service(t).Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func classOf(report scan.Report, test ecosystem.TestID) format.ScanClass {
	for _, scanned := range report.Statement.Predicate.Tests {
		if scanned.ID == string(test) {
			return scanned.Class
		}
	}
	return ""
}

// trackedDigest is the digest of the tracked files of the fake project, scan
// reports aside.
func trackedDigest(t *testing.T, f *fixture) string {
	t.Helper()
	var files []format.File
	for _, path := range []string{totals, "tests/test_discount.py", "tests/test_shipping.py"} {
		content, err := fs.ReadFile(f.workspaces.project, path)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, format.File{Path: path, Content: content})
	}
	digest, err := format.FileSetDigest(files)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func TestRepositoryScan(t *testing.T) {
	f := newFixture()
	report := run(t, f, request())
	statement := report.Statement.Predicate

	t.Run("every test of the suite ran three times on the reference and the green ones are judged"+verifiesReference, func(t *testing.T) {
		if report.Suite != (scan.SuiteCounts{Tests: 4, Green: 3, Sampled: 3}) {
			t.Fatalf("suite = %+v", report.Suite)
		}
		if statement.Counts != (format.ScanCounts{Tests: 4, VerifiesNothing: 1, Caught: 2, NotGreen: 1}) {
			t.Fatalf("counts = %+v", statement.Counts)
		}
	})
	t.Run("the test that runs discount and checks nothing verifies nothing; the tests that check catch"+verifiesRecord, func(t *testing.T) {
		if classOf(report, vacuousTest) != format.ScanVerifiesNothing || classOf(report, exactTest) != format.ScanCaught || classOf(report, feeTest) != format.ScanCaught {
			t.Fatalf("tests = %+v", statement.Tests)
		}
	})
	t.Run("every mutant mutates lines the sampled tests ran and holds its edits"+verifiesDraw, func(t *testing.T) {
		ran := map[int]bool{2: true, 3: true, 4: true, 8: true, 9: true, 10: true}
		for _, mutant := range statement.Mutants {
			if len(mutant.Edits) != 1 || mutant.Edits[0].File != totals {
				t.Fatalf("edits %+v", mutant.Edits)
			}
			if !slices.ContainsFunc(mutant.Lines, func(line format.ScanLine) bool { return ran[line.Line] }) {
				t.Fatalf("mutant %s mutates %v", mutant.ID, mutant.Lines)
			}
		}
	})
	t.Run("sentinels run and are never proven"+verifiesDraw, func(t *testing.T) {
		sentinels := 0
		for _, mutant := range statement.Mutants {
			if mutant.Sentinel {
				sentinels++
				if mutant.Proven() {
					t.Fatalf("sentinel %s proven", mutant.ID)
				}
			}
		}
		if sentinels == 0 {
			t.Fatal("no sentinel ran")
		}
	})
	t.Run("every sampled test ran a mutated line of every operator mutant drawn among its lines"+verifiesAllocated, func(t *testing.T) {
		operators := 0
		for _, mutant := range statement.Mutants {
			if !mutant.Sentinel {
				operators++
			}
		}
		want := map[ecosystem.TestID]int{vacuousTest: 0, exactTest: 0, feeTest: 0}
		lines := map[ecosystem.TestID][]int{vacuousTest: {2, 3}, exactTest: {2, 4}, feeTest: {8, 9, 10}}
		for _, mutant := range statement.Mutants {
			for test, ran := range lines {
				if !mutant.Sentinel && slices.ContainsFunc(mutant.Lines, func(line format.ScanLine) bool { return slices.Contains(ran, line.Line) }) {
					want[test]++
				}
			}
		}
		if operators == 0 || !maps.Equal(report.Candidates, want) {
			t.Fatalf("candidates %v, want %v", report.Candidates, want)
		}
	})
	t.Run("a scan runs at most the sentinel cap of sentinels"+verifiesSentinels, func(t *testing.T) {
		sentinels := 0
		for _, mutant := range statement.Mutants {
			if mutant.Sentinel {
				sentinels++
			}
		}
		if sentinels == 0 || sentinels > scanscope.SentinelCap {
			t.Fatalf("%d sentinels", sentinels)
		}
	})
	t.Run("the coverage of the reference is recorded within the bound derived from the reference runs"+verifiesCoverage, func(t *testing.T) {
		if f.suite.coverageTimeout != scan.MinCoverageTimeout {
			t.Fatalf("coverage bound %s", f.suite.coverageTimeout)
		}
	})
	t.Run("a mutant runs only against green tests that ran one of its mutated lines"+verifiesExecute, func(t *testing.T) {
		if len(f.suite.selections) == 0 {
			t.Fatal("no mutant ran")
		}
		for _, selection := range f.suite.selections {
			if slices.Contains(selection, redTest) || (slices.Contains(selection, feeTest) && slices.Contains(selection, vacuousTest)) {
				t.Fatalf("selection %v", selection)
			}
		}
	})
	t.Run("a change no test killed is proven by a reproduced difference on the bound function"+verifiesSearch, func(t *testing.T) {
		found := false
		for _, mutant := range statement.Mutants {
			for _, route := range mutant.Routes {
				if route.Kind != format.ScanRouteDifferential {
					continue
				}
				found = true
				if route.Operation != discountOp || route.Channel != format.ScanChannelReturnValue || route.Reference == route.Mutant || route.Input == "" {
					t.Fatalf("route %+v", route)
				}
			}
		}
		if !found {
			t.Fatal("no differential route")
		}
	})
	t.Run("the finding shows a proven change of the vacuous test, the files it edits and the repro"+verifiesRecord, func(t *testing.T) {
		if len(report.Findings) != 1 {
			t.Fatalf("findings = %+v", report.Findings)
		}
		finding := report.Findings[0]
		if finding.Test != vacuousTest || finding.Rank != 1 || finding.Changes < 5 || !finding.Mutant.Proven() ||
			finding.Route.Kind != format.ScanRouteDifferential || len(finding.Files) != 1 ||
			string(finding.Files[0].Before) != totalsSource || bytes.Equal(finding.Files[0].Before, finding.Files[0].After) ||
			finding.Repro[len(finding.Repro)-1] != vacuousTest {
			t.Fatalf("finding = %+v", finding)
		}
	})
	t.Run("the statement names the head commit and the digest of its tracked files, scan reports aside"+verifiesRecord, func(t *testing.T) {
		subject := report.Statement.Subject[0]
		if subject.Name != headCommit || subject.Digest.SHA256 != trackedDigest(t, f) {
			t.Fatalf("subject = %+v", subject)
		}
		if _, err := format.AdmitScan(report.Encoded); err != nil {
			t.Fatal(err)
		}
		if statement.Mode != format.ScanModeRepository || statement.Seed != "7" || statement.Runner != "pytest" || statement.Partial || statement.PromisesChecked {
			t.Fatalf("predicate = %+v", statement)
		}
	})
}

func TestScanContract(t *testing.T) {
	report := run(t, newFixture(), request())
	t.Run("a scan of the fake project records the golden statement"+verifiesContract, func(t *testing.T) {
		if *updateGolden {
			if err := os.WriteFile(goldenStatement, append(report.Encoded, '\n'), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		golden, err := os.ReadFile(goldenStatement)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(bytes.TrimSuffix(golden, []byte("\n")), report.Encoded) {
			t.Fatalf("the statement differs from %s; rerun with -update after checking the change", goldenStatement)
		}
	})
	t.Run("the scan schema accepts the statement"+verifiesContract, func(t *testing.T) {
		instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(report.Encoded))
		if err != nil {
			t.Fatal(err)
		}
		if err := scanStatementSchema(t).Validate(instance); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("the statement is no proof: decoding or admitting it as one is refused"+verifiesContract, func(t *testing.T) {
		if _, err := format.Decode(report.Encoded); !errors.Is(err, format.ErrUnknownPredicateType) {
			t.Fatalf("decode: %v", err)
		}
		if _, err := format.Admit(report.Encoded); !errors.Is(err, format.ErrUnknownPredicateType) {
			t.Fatalf("admit: %v", err)
		}
	})
	t.Run("the statement does not depend on how many workers ran the mutants"+verifiesContract, func(t *testing.T) {
		single := request()
		single.Workers = 1
		if again := run(t, newFixture(), single); !bytes.Equal(again.Encoded, report.Encoded) {
			t.Fatal("one worker records another statement")
		}
	})
}

// scanStatementSchema compiles the scan schema with the common definitions.
func scanStatementSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	common := format.Schemas()[slices.IndexFunc(format.Schemas(), func(s format.Schema) bool { return s.ID == format.CommonSchemaID })]
	for _, schema := range []format.Schema{common, format.ScanSchema()} {
		document, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema.Bytes))
		if err != nil {
			t.Fatal(err)
		}
		if err := compiler.AddResource(schema.ID, document); err != nil {
			t.Fatal(err)
		}
	}
	compiled, err := compiler.Compile(format.PredicateTypeScan + format.ScanStatementSchemaPointer)
	if err != nil {
		t.Fatal(err)
	}
	return compiled
}

func TestChangeScan(t *testing.T) {
	f := newFixture()
	f.source.changed = map[string][]int{totals: {9}, "README.md": {1}}
	req := request()
	req.Base = "main"
	report := run(t, f, req)
	statement := report.Statement.Predicate

	t.Run("a change scan judges the green tests that ran the changed function, against the base"+verifiesPlan, func(t *testing.T) {
		if statement.Mode != format.ScanModeChange || statement.Base != baseCommit || report.Suite.Sampled != 1 {
			t.Fatalf("mode %s base %s sampled %d", statement.Mode, statement.Base, report.Suite.Sampled)
		}
		if classOf(report, feeTest) != format.ScanCaught || classOf(report, vacuousTest) != "" || statement.Counts.NotSampled != 2 {
			t.Fatalf("tests %+v, counts %+v", statement.Tests, statement.Counts)
		}
	})
	t.Run("a change scan mutates only the lines of the changed function the sampled tests ran"+verifiesPlan, func(t *testing.T) {
		if len(statement.Mutants) == 0 {
			t.Fatal("no mutant")
		}
		for _, mutant := range statement.Mutants {
			for _, line := range mutant.Lines {
				if line.Line < 8 || line.Line > 10 {
					t.Fatalf("mutant %s mutates %v", mutant.ID, mutant.Lines)
				}
			}
		}
	})
}

func TestScanRefusals(t *testing.T) {
	t.Run("a work tree with changed tracked files is refused before anything runs"+verifiesReference, func(t *testing.T) {
		f := newFixture()
		f.source.dirty = true
		if _, err := f.service(t).Run(context.Background(), request()); !errors.Is(err, scan.ErrDirtyTree) || f.workspaces.files != nil {
			t.Fatalf("error %v, workspaces %v", err, f.workspaces.files)
		}
	})
	t.Run("a reference that does not build is an error"+verifiesReference, func(t *testing.T) {
		f := newFixture()
		f.broken = func(string) bool { return true }
		if _, err := f.service(t).Run(context.Background(), request()); !errors.Is(err, scan.ErrReferenceBuild) {
			t.Fatalf("error %v", err)
		}
	})
	t.Run("a suite that collects no test is an error"+verifiesReference, func(t *testing.T) {
		f := newFixture()
		f.suite.tests = map[ecosystem.TestID]fakeTest{}
		if _, err := f.service(t).Run(context.Background(), request()); !errors.Is(err, scan.ErrNoTests) {
			t.Fatalf("error %v", err)
		}
	})
}

func TestCoverageTimeout(t *testing.T) {
	t.Run("a requested coverage bound replaces the derived one"+verifiesCoverage, func(t *testing.T) {
		f := newFixture()
		req := request()
		req.CoverageTimeout = 7 * time.Minute
		run(t, f, req)
		if f.suite.coverageTimeout != req.CoverageTimeout {
			t.Fatalf("coverage bound %s", f.suite.coverageTimeout)
		}
	})
	t.Run("a coverage run past its bound fails the scan with the timeout and the bound"+verifiesCoverage, func(t *testing.T) {
		f := newFixture()
		f.suite.coverageOverruns = true
		_, err := f.service(t).Run(context.Background(), request())
		if !errors.Is(err, ports.ErrCoverageTimedOut) || !strings.Contains(err.Error(), scan.MinCoverageTimeout.String()) {
			t.Fatalf("error %v", err)
		}
	})
}

func TestMutantExecution(t *testing.T) {
	t.Run("a mutant that does not build is dropped and counted"+verifiesExecute, func(t *testing.T) {
		f := newFixture()
		f.broken = func(source string) bool { return !strings.Contains(source, sourceLine(10)) }
		report := run(t, f, request())
		if report.Mutants.DidNotBuild == 0 {
			t.Fatalf("mutants %+v", report.Mutants)
		}
		for _, mutant := range report.Statement.Predicate.Mutants {
			if !strings.Contains(mutant.Edits[0].NewSource, strings.TrimSpace(sourceLine(10))) && strings.Contains(mutant.Edits[0].NewSource, "def shipping") {
				t.Fatalf("mutant %s did not build and was recorded", mutant.ID)
			}
		}
	})
	t.Run("a mutant whose edited sites compile to the reference's form runs no test and proves nothing"+verifiesExecute, func(t *testing.T) {
		f := newFixture()
		f.compiler = true
		report := run(t, f, request())
		if report.Mutants.CompiledIdentical == 0 {
			t.Fatalf("mutants %+v", report.Mutants)
		}
		for _, mutant := range report.Statement.Predicate.Mutants {
			if strings.Contains(mutant.Edits[0].NewSource, "if percent < 0:") && mutant.Proven() {
				t.Fatalf("identical mutant %s proven", mutant.ID)
			}
		}
	})
	t.Run("a sentinel runs its tests even when it compiles to the reference's form"+verifiesExecute, func(t *testing.T) {
		f := newFixture()
		f.compiler, f.folds = true, true
		report := run(t, f, request())
		sentinels := 0
		for _, mutant := range report.Statement.Predicate.Mutants {
			if mutant.Sentinel {
				sentinels++
			}
		}
		if sentinels == 0 || len(f.suite.selections) != sentinels || report.Mutants.CompiledIdentical != report.Mutants.Ran-sentinels {
			t.Fatalf("%d sentinels, %d selections, mutants %+v", sentinels, len(f.suite.selections), report.Mutants)
		}
	})
	t.Run("once the budget has passed no mutant starts and the scan is partial"+verifiesExecute, func(t *testing.T) {
		f := newFixture()
		f.clock.step = time.Minute
		req := request()
		req.Budget = time.Minute
		report := run(t, f, req)
		statement := report.Statement.Predicate
		if !report.Partial || !statement.Partial || report.Mutants.NotRun != report.Mutants.Drawn || len(statement.Mutants) != 0 {
			t.Fatalf("partial %v, mutants %+v", report.Partial, report.Mutants)
		}
		if statement.Counts.Unproven != 3 || statement.Counts.VerifiesNothing != 0 {
			t.Fatalf("counts %+v", statement.Counts)
		}
	})
	t.Run("a mutant still running when the budget passes is stopped and counted as not run"+verifiesExecute, func(t *testing.T) {
		f := newFixture()
		f.suite.hangs = true
		req := request()
		req.Budget = inFlightBudget
		started := time.Now()
		report := run(t, f, req)
		if !report.Partial || report.Mutants.NotRun != report.Mutants.Drawn || len(report.Statement.Predicate.Mutants) != 0 || time.Since(started) > inFlightBound {
			t.Fatalf("partial %v, mutants %+v after %s", report.Partial, report.Mutants, time.Since(started))
		}
	})
	t.Run("a difference that does not reproduce on the mutant proves nothing"+verifiesSearch, func(t *testing.T) {
		f := newFixture()
		f.prober.flaky = true
		report := run(t, f, request())
		for _, mutant := range report.Statement.Predicate.Mutants {
			for _, route := range mutant.Routes {
				if route.Kind == format.ScanRouteDifferential {
					t.Fatalf("route %+v", route)
				}
			}
		}
		if classOf(report, vacuousTest) != format.ScanUnproven {
			t.Fatalf("tests %+v", report.Statement.Predicate.Tests)
		}
	})
}

func TestReplay(t *testing.T) {
	f := newFixture()
	report := run(t, f, request())
	record := scan.RecordOf(report, report.Findings[0])
	service := f.service(t)

	t.Run("a finding replays on its commit and every claim holds"+verifiesReplay, func(t *testing.T) {
		replay, err := service.Replay(context.Background(), "/work/invoicing", record)
		if err != nil || !replay.Held() || len(replay.Checks) < 4 {
			t.Fatalf("replay %+v, %v", replay, err)
		}
	})
	t.Run("a finding of another commit is refused"+verifiesReplay, func(t *testing.T) {
		other := record
		other.Commit = baseCommit
		if _, err := service.Replay(context.Background(), "/work/invoicing", other); !errors.Is(err, scan.ErrOtherCommit) {
			t.Fatalf("error %v", err)
		}
	})
	t.Run("a route that no longer holds is reported as not held"+verifiesReplay, func(t *testing.T) {
		claimed := record
		claimed.Route = format.ScanRoute{Kind: format.ScanRouteKilledByTest, Test: vacuousTest}
		replay, err := service.Replay(context.Background(), "/work/invoicing", claimed)
		if err != nil || replay.Held() {
			t.Fatalf("replay %+v, %v", replay, err)
		}
	})
	t.Run("a finding record reads back from its encoding and an empty one is refused"+verifiesReplay, func(t *testing.T) {
		if _, err := scan.DecodeFinding([]byte(`{"test": ""}`)); !errors.Is(err, scan.ErrUnreadableFinding) {
			t.Fatalf("error %v", err)
		}
	})
}
