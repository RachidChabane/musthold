package scanreport_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/scanreport"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/scan"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/format"
)

const (
	verifiesText     = " [verifies SP-2115~4]"
	verifiesMarkdown = " [verifies SP-2116~3]"
	verifiesGaps     = " [verifies SP-2129~2]"
	verifiesFolders  = " [verifies SP-2117~1]"
	verifiesStake    = " [verifies SP-2132~1]"

	stakeLine = "7/40 sampled tests stay green on every proven change in lines they ran: on those lines, their green is no evidence the code is right."

	commit = "3f2a9c1d0b8e7f6a5c4d3e2f1a0b9c8d7e6f5a4b"
	parse  = "src/invoicing/parse.py"
)

var (
	beforeParse = "def split_fields(line):\n    fields = []\n    if len(line) >= 2:\n        fields.append(line)\n    return fields\n"
	afterParse  = "def split_fields(line):\n    fields = []\n    if len(line) > 2:\n        fields.append(line)\n    return fields\n"
	// bannedWords may never appear in a report's own vocabulary.
	bannedWords = regexp.MustCompile(`(?i:\bbugs?\b)|\bPASS\b|(?i:\bscore\b)`)
)

func encoded(t *testing.T, value any) string {
	t.Helper()
	raw, err := format.Canonicalize(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func report(t *testing.T) scan.Report {
	t.Helper()
	tests := []format.ScanTest{
		{ID: "tests/test_parse.py::test_parse_ok", Class: format.ScanVerifiesNothing},
		{ID: "tests/test_parse.py::test_two", Class: format.ScanUnproven, Reason: format.ScanTooFewChanges},
		{ID: "tests/test_parse.py::test_three", Class: format.ScanUnproven, Reason: format.ScanTooFewChanges},
		{ID: "tests/test_split.py::test_slow", Class: format.ScanUnproven, Reason: format.ScanInconclusiveOutcome},
		{ID: "tests/test_split.py::test_trailing_comma", Class: format.ScanCaught},
	}
	differential := format.ScanRoute{
		Kind: format.ScanRouteDifferential, Operation: "invoicing.parse:split_fields", Channel: format.ScanChannelReturnValue,
		Input:     encoded(t, map[string]spec.Literal{"line": {Type: spec.TypeString, Value: "a,"}}),
		Reference: encoded(t, spec.Observation{Kind: spec.Returns, Type: spec.TypeString, Value: "['a,']"}),
		Mutant:    encoded(t, spec.Observation{Kind: spec.Raises, Value: "IndexError"}),
	}
	killed := format.ScanRoute{Kind: format.ScanRouteKilledByTest, Test: "tests/test_split.py::test_trailing_comma"}
	files := []scan.FileChange{{Path: parse, Before: []byte(beforeParse), After: []byte(afterParse)}}
	return scan.Report{
		Statement: format.ScanStatement{Predicate: format.Scan{
			Counts: format.ScanCounts{Tests: 412, VerifiesNothing: 7, Caught: 21, Unproven: 12, NotSampled: 369, NotGreen: 3},
			Tests:  tests,
		}},
		Encoded:    []byte(`{"predicateType":"` + format.PredicateTypeScan + `"}`),
		Repository: "acme/invoicing", Commit: commit, Runner: ecosystem.Pytest, Mode: format.ScanModeRepository, Seed: 7,
		Suite:   scan.SuiteCounts{Tests: 412, Green: 409, Sampled: 40},
		Mutants: scan.MutantCounts{Drawn: 152, Ran: 150, Proven: 61, CompiledIdentical: 4, DidNotBuild: 2},
		Findings: []scan.Finding{
			{Rank: 1, Test: "tests/test_parse.py::test_parse_ok", Changes: 9, Site: format.ScanLine{File: parse, Line: 3},
				Route: differential, Files: files, Repro: []string{"python3", "-m", "pytest", "tests/test_parse.py::test_parse_ok"},
				Mutant: format.ScanMutant{ID: strings.Repeat("a", 64), Edits: []format.ScanEdit{{File: parse, Qualname: "split_fields", NewSource: afterParse}}}},
			{Rank: 2, Test: "tests/test_parse.py::test_it's_fine", Changes: 5, Site: format.ScanLine{File: parse, Line: 3},
				Route: killed, Files: files, Repro: []string{"python3", "-m", "pytest", "tests/test_parse.py::test_it's_fine"}},
		},
		Elapsed: 4*time.Minute + 12*time.Second,
		Phases:  scan.Phases{Reference: time.Minute, Coverage: 30 * time.Second, Draw: 2 * time.Second, Mutants: 2*time.Minute + 40*time.Second},
		Depth:   scan.DepthQuick, Budget: 90 * time.Second, Workers: 2,
		Unproven: []scan.UnprovenTest{
			{Test: "tests/test_parse.py::test_three", Reason: format.ScanTooFewChanges},
			{Test: "tests/test_parse.py::test_two", Reason: format.ScanTooFewChanges, Drawn: 3, Proven: 1, Gaps: map[string]scan.Gap{"m1": scan.GapNoDifference, "m2": scan.GapUnbound}},
			{Test: "tests/test_split.py::test_slow", Reason: format.ScanInconclusiveOutcome, Drawn: 7, Proven: 6, Gaps: map[string]scan.Gap{"m1": scan.GapNoDifference}},
		},
	}
}

func TestGapNoun(t *testing.T) {
	t.Run("one unproven mutant is named in the singular"+verifiesText, func(t *testing.T) {
		single := report(t)
		single.Unproven = []scan.UnprovenTest{{Test: "t::one", Reason: format.ScanTooFewChanges, Drawn: 1, Gaps: map[string]scan.Gap{"m1": scan.GapUnbound}}}
		if !strings.Contains(scanreport.Text(single), "they lack: 1 unproven mutant on their lines: 1 with no function to call") {
			t.Fatalf("text:\n%s", scanreport.Text(single))
		}
	})
}

func TestText(t *testing.T) {
	text := scanreport.Text(report(t))
	lines := strings.Split(text, "\n")
	t.Run("the first line names the repository, commit, runner, suite, sample and seed"+verifiesText, func(t *testing.T) {
		if want := "acme/invoicing @ 3f2a9c1 · pytest · 412 tests (409 green 3/3) · 40 sampled · seed 7"; lines[0] != want {
			t.Fatalf("header %q", lines[0])
		}
	})
	t.Run("the second line names the depth, the budget, the time, the sample of the green tests and the workers, and how to judge more"+verifiesText, func(t *testing.T) {
		if want := "  quick scan · 1m30s budget · 4m12s · 40 of 409 green tests sampled · 2 workers — run with --full for the whole suite"; lines[1] != want {
			t.Fatalf("depth %q", lines[1])
		}
		full := report(t)
		full.Depth, full.Budget, full.Workers = scan.DepthFull, 0, 1
		if want := "  full scan · no budget · 4m12s · 40 of 409 green tests sampled · 1 worker"; strings.Split(scanreport.Text(full), "\n")[1] != want {
			t.Fatalf("depth %q", strings.Split(scanreport.Text(full), "\n")[1])
		}
	})
	t.Run("the four counts stand over their denominators: the judged tests, and the green tests for the unsampled"+verifiesText, func(t *testing.T) {
		if want := "  7/40 verify nothing   21/40 caught ≥ 1 change   12/40 unproven   369/409 not sampled"; lines[2] != want {
			t.Fatalf("split %q", lines[2])
		}
	})
	t.Run("the unproven tests are broken down by reason and by what their lines lack, and the result is never a pass"+verifiesText, func(t *testing.T) {
		if want := "  unproven: 2 with < 5 proven changes or < 2 sites, 1 with an inconclusive outcome"; lines[3] != want {
			t.Fatalf("reasons %q", lines[3])
		}
		if want := "  they lack: 1 ran no drawn mutant · 2 unproven mutants on their lines: 1 searched without a difference, 1 with no function to call · no witness without a host"; lines[4] != want {
			t.Fatalf("gaps %q", lines[4])
		}
		if lines[5] != "  Not a pass: unproven and unsampled tests are unjudged." {
			t.Fatalf("line %q", lines[5])
		}
	})
	t.Run("a finding shows the literal sentence, the change, its input and both results, the route and the repro"+verifiesText, func(t *testing.T) {
		for _, want := range []string{
			"#1 tests/test_parse.py::test_parse_ok passes on all 9 proven changes in lines it ran",
			`   src/invoicing/parse.py:3  >= → >   input line="a,": reference "['a,']", mutant raises IndexError`,
			"   proven by: a differential run of invoicing.parse:split_fields, reproduced, on a reference that agrees with itself",
			"   repro: .spec-proof/scan/3f2a9c1/1/repro.sh",
			"   proven by: tests/test_split.py::test_trailing_comma kills it 3/3",
		} {
			if !strings.Contains(text, want+"\n") {
				t.Errorf("missing %q in\n%s", want, text)
			}
		}
	})
	t.Run("the last line counts the mutants and says no model ran and no promise was checked"+verifiesText, func(t *testing.T) {
		want := "150 mutants (61 proven, 4 identical when compiled, 2 did not build or start) · 4m12s (reference 1m0s, coverage 30s, draw 2s, mutants 2m40s) · no model used · promises not checked"
		if lines[len(lines)-3] != want {
			t.Fatalf("footer %q", lines[len(lines)-3])
		}
	})
	t.Run("a scan with a test that verifies nothing ends on what its green leaves unshown, over the sampled tests"+verifiesStake, func(t *testing.T) {
		if lines[len(lines)-2] != stakeLine {
			t.Fatalf("closing %q", lines[len(lines)-2])
		}
	})
	t.Run("a scan with no test that verifies nothing ends on the footer"+verifiesStake, func(t *testing.T) {
		none := report(t)
		none.Statement.Predicate.Counts.VerifiesNothing, none.Statement.Predicate.Counts.Caught = 0, 28
		none.Findings = nil
		lines := strings.Split(scanreport.Text(none), "\n")
		if !strings.HasSuffix(lines[len(lines)-2], "promises not checked") {
			t.Fatalf("closing %q", lines[len(lines)-2])
		}
		if markdown := scanreport.Markdown(none); strings.Contains(markdown, "stay green") {
			t.Fatalf("markdown:\n%s", markdown)
		}
	})
	t.Run("the headline never says bug, PASS or score"+verifiesText, func(t *testing.T) {
		if found := bannedWords.FindString(text); found != "" {
			t.Fatalf("the headline says %q", found)
		}
	})
	t.Run("a partial scan says the budget stopped it"+verifiesText, func(t *testing.T) {
		partial := report(t)
		partial.Partial = true
		if !strings.Contains(scanreport.Text(partial), "partial: the budget stopped the scan before every mutant ran") {
			t.Fatal("no partial note")
		}
	})
	t.Run("the mutant count names the mutants the budget left unrun, in the text and the Markdown"+verifiesText, func(t *testing.T) {
		partial := report(t)
		partial.Partial, partial.Mutants.NotRun = true, 21
		want := "150 mutants (61 proven, 4 identical when compiled, 2 did not build or start, 21 not run within the budget) · "
		if text, markdown := scanreport.Text(partial), scanreport.Markdown(partial); !strings.Contains(text, want) || !strings.Contains(markdown, want) {
			t.Fatalf("missing %q in\n%s", want, text)
		}
		if strings.Contains(scanreport.Text(report(t)), "not run within the budget") {
			t.Fatal("a scan that ran every mutant names unrun ones")
		}
	})
	t.Run("a scan with no finding still prints the split and the note"+verifiesText, func(t *testing.T) {
		empty := report(t)
		empty.Findings = nil
		text := scanreport.Text(empty)
		if !strings.Contains(text, "7/40 verify nothing") || !strings.Contains(text, "Not a pass") || strings.Contains(text, "#1") {
			t.Fatalf("text:\n%s", text)
		}
	})
}

// stoppedReport is the report with changes that stopped tests from starting:
// three for the unproven test_two, one for test_three, and two for the first
// finding.
func stoppedReport(t *testing.T) scan.Report {
	t.Helper()
	stopped := report(t)
	tests := stopped.Statement.Predicate.Tests
	for index := range tests {
		switch tests[index].ID {
		case "tests/test_parse.py::test_two":
			tests[index].NotStarted = []string{"m3", "m4", "m5"}
		case "tests/test_parse.py::test_three":
			tests[index].NotStarted = []string{"m6"}
		case "tests/test_parse.py::test_parse_ok":
			tests[index].NotStarted = []string{"m7", "m8"}
		}
	}
	stopped.Findings[0].NotStarted = 2
	stopped.Mutants.StoppedSentinels = 1
	return stopped
}

func TestNotStarted(t *testing.T) {
	stopped := stoppedReport(t)
	text, markdown := scanreport.Text(stopped), scanreport.Markdown(stopped)
	t.Run("a test that changes in its lines stopped from starting says how many, and that they count neither way"+verifiesText, func(t *testing.T) {
		for _, want := range []string{
			"  tests/test_parse.py::test_two: 3 proven changes in lines it ran stopped it from starting; they count neither way\n",
			"  tests/test_parse.py::test_three: 1 proven change in lines it ran stopped it from starting; it counts neither way\n",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("missing %q in\n%s", want, text)
			}
		}
		if strings.Contains(text, "test_parse_ok: ") {
			t.Fatalf("a finding has its own line:\n%s", text)
		}
	})
	t.Run("the sentence of a test that verifies nothing keeps its number and adds the changes that stopped it from starting"+verifiesText, func(t *testing.T) {
		if want := "#1 tests/test_parse.py::test_parse_ok passes on all 9 proven changes in lines it ran; 2 more stopped it from starting\n"; !strings.Contains(text, want) {
			t.Fatalf("missing %q in\n%s", want, text)
		}
	})
	t.Run("the Markdown report lists the same lines and sentence"+verifiesMarkdown, func(t *testing.T) {
		for _, want := range []string{
			"- tests/test_parse.py::test_two: 3 proven changes in lines it ran stopped it from starting; they count neither way.\n",
			"### 1. `tests/test_parse.py::test_parse_ok` passes on all 9 proven changes in lines it ran; 2 more stopped it from starting\n",
		} {
			if !strings.Contains(markdown, want) {
				t.Errorf("missing %q in\n%s", want, markdown)
			}
		}
	})
	t.Run("the sentinels that stopped a test binary are counted with the mutants"+verifiesGaps, func(t *testing.T) {
		if want := "2 did not build or start, 1 sentinel stopped a test binary)"; !strings.Contains(text, want) || !strings.Contains(markdown, want) {
			t.Fatalf("missing %q in\n%s", want, text)
		}
	})
	t.Run("a scan where nothing stopped a test from starting prints none of it"+verifiesText, func(t *testing.T) {
		if plain := scanreport.Text(report(t)); strings.Contains(plain, "stopped") {
			t.Fatalf("text:\n%s", plain)
		}
	})
}

func TestMarkdown(t *testing.T) {
	markdown := scanreport.Markdown(report(t))
	t.Run("the report holds the split as a table, the note, each finding with its diff, and no banned word"+verifiesMarkdown, func(t *testing.T) {
		for _, want := range []string{
			"| verify nothing | 7/40 |", "| not sampled | 369/409 |", "Not a pass: unproven and unsampled tests are unjudged.",
			"### 1. `tests/test_parse.py::test_parse_ok` passes on all 9 proven changes in lines it ran",
			"```diff\n--- a/src/invoicing/parse.py\n+++ b/src/invoicing/parse.py\n",
			"-    if len(line) >= 2:\n+    if len(line) > 2:\n",
			"promises not checked",
			"quick scan · 1m30s budget · 4m12s · 40 of 409 green tests sampled · 2 workers — run with --full for the whole suite",
			"They lack: 1 ran no drawn mutant · 2 unproven mutants on their lines",
			"| `tests/test_parse.py::test_two` | with < 5 proven changes or < 2 sites | 1 | 1 searched without a difference, 1 with no function to call |",
			"| `tests/test_parse.py::test_three` | with < 5 proven changes or < 2 sites | 0 | none |",
		} {
			if !strings.Contains(markdown, want) {
				t.Errorf("missing %q in\n%s", want, markdown)
			}
		}
		if found := bannedWords.FindString(markdown); found != "" {
			t.Fatalf("the report says %q", found)
		}
	})
	t.Run("the report ends on what the green of the tests that verify nothing leaves unshown"+verifiesStake, func(t *testing.T) {
		if !strings.HasSuffix(markdown, "promises not checked\n\n"+stakeLine+"\n") {
			t.Fatalf("markdown ends\n%s", markdown[max(0, len(markdown)-300):])
		}
	})
}

func TestFiles(t *testing.T) {
	r := report(t)
	files, err := scanreport.Files(r)
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]scanreport.File{}
	for _, file := range files {
		byPath[file.Path] = file
	}
	t.Run("the reports of a commit hold the Markdown report, the statement and one folder per finding"+verifiesFolders, func(t *testing.T) {
		for _, path := range []string{
			".spec-proof/scan/3f2a9c1/report.md", ".spec-proof/scan/3f2a9c1/scan.json",
			".spec-proof/scan/3f2a9c1/1/mutant.patch", ".spec-proof/scan/3f2a9c1/1/repro.sh", ".spec-proof/scan/3f2a9c1/1/finding.json",
			".spec-proof/scan/3f2a9c1/2/repro.sh",
		} {
			if _, found := byPath[path]; !found {
				t.Errorf("no %s", path)
			}
		}
		if string(byPath[".spec-proof/scan/3f2a9c1/scan.json"].Content) != string(r.Encoded) {
			t.Fatal("the statement is not the encoded one")
		}
	})
	t.Run("the finding record names the commit, the test, the mutant and the route"+verifiesFolders, func(t *testing.T) {
		record, err := scan.DecodeFinding(byPath[".spec-proof/scan/3f2a9c1/1/finding.json"].Content)
		if err != nil || record.Commit != commit || record.Test != "tests/test_parse.py::test_parse_ok" || record.Route.Kind != format.ScanRouteDifferential {
			t.Fatalf("record %+v, %v", record, err)
		}
		var raw map[string]any
		content := byPath[".spec-proof/scan/3f2a9c1/1/finding.json"].Content
		if json.Unmarshal(content, &raw) != nil || !strings.Contains(string(content), "len(line) > 2") {
			t.Fatalf("the record escapes its source:\n%s", content)
		}
	})
	t.Run("the repro script checks the commit out elsewhere, applies the patch and runs the one test, quoted"+verifiesFolders, func(t *testing.T) {
		script := string(byPath[".spec-proof/scan/3f2a9c1/2/repro.sh"].Content)
		for _, want := range []string{
			"#!/bin/sh\n", "set -eu\n", "git -C \"$root\" archive " + commit + " | tar -x -C \"$work\"\n",
			"git apply \"$finding/mutant.patch\"\n", `'python3' '-m' 'pytest' 'tests/test_parse.py::test_it'\''s_fine'` + "\n",
		} {
			if !strings.Contains(script, want) {
				t.Errorf("missing %q in\n%s", want, script)
			}
		}
		if !byPath[".spec-proof/scan/3f2a9c1/2/repro.sh"].Executable {
			t.Fatal("the script is not executable")
		}
	})
	t.Run("writing replaces the earlier reports of the commit and makes the script executable"+verifiesFolders, func(t *testing.T) {
		root := t.TempDir()
		stale := filepath.Join(root, ".spec-proof", "scan", "3f2a9c1", "9", "repro.sh")
		if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(stale, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := scanreport.Write(root, r, files); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(stale); !os.IsNotExist(err) {
			t.Fatalf("stale finding kept: %v", err)
		}
		info, err := os.Stat(filepath.Join(root, ".spec-proof", "scan", "3f2a9c1", "1", "repro.sh"))
		if err != nil || info.Mode().Perm()&0o100 == 0 {
			t.Fatalf("repro.sh %v, %v", info, err)
		}
	})
}
