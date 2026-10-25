package scanreport_test

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/scanreport"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/scan"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/promise"
	"github.com/RachidChabane/spec-proof/format"
)

var updateGolden = flag.Bool("update", false, "rewrite the golden renders of a scan that ran with a host")

const (
	verifiesPromises  = " [verifies SP-2247~1]"
	verifiesBroken    = " [verifies SP-2248~1]"
	verifiesQuestions = " [verifies SP-2249~1]"
	verifiesUnchecked = " [verifies SP-2250~1]"
	verifiesNoPromise = " [verifies SP-2251~1]"
	verifiesFooter    = " [verifies SP-2252~1]"
	verifiesLabels    = " [verifies SP-2253~1]"
	verifiesKillers   = " [verifies SP-2254~1]"
	verifiesBrokenDir = " [verifies SP-2255~1]"

	witnessID    = "9d1c0e77a4b25f6803aa11bc22dd33ee44ff5566778899aabbccddeeff001122"
	otherWitness = "1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d5e6f708192a3b4c5d6e7f809"
	witnessPath  = "tests/test_promise_witness.py"
	witnessCode  = "from invoicing.parse import split_fields\n\n\ndef test_keeps_trailing_field():\n    assert split_fields('a,') == ['a,']\n"
)

var (
	splitPromise  = format.ScanPromise{ID: "P1", Quote: "split_fields keeps a trailing field", Source: format.ScanSource{Name: "README.md"}}
	rangePromise  = format.ScanPromise{ID: "P2", Quote: "lines shorter than two characters are skipped", Source: format.ScanSource{Name: "description"}}
	capPromise    = format.ScanPromise{ID: "P3", Quote: "parsing never allocates twice", Source: format.ScanSource{Name: "README.md"}}
	budgetPromise = format.ScanPromise{ID: "P4", Quote: "headers are case insensitive", Source: format.ScanSource{Name: "issue #12"}}
)

func goldenCompare(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *updateGolden {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, []byte(got)) {
		t.Fatalf("the render differs from %s; rerun with -update after checking the change\n%s", path, got)
	}
}

func noBannedWord(t *testing.T, text string) {
	t.Helper()
	if found := bannedWords.FindString(text); found != "" {
		t.Fatalf("the render says %q", found)
	}
}

// hostReport is the report of a scan that ran with a host: one broken promise,
// a question of each reason, promises left unchecked, asks cut and unasked and
// a finding whose test is labelled with a promise.
func hostReport(t *testing.T) scan.Report {
	t.Helper()
	r := report(t)
	witness := format.ScanWitness{ID: witnessID, Promise: splitPromise.ID, Name: "test_keeps_trailing_field", Code: witnessCode, Admitted: true, BlockBearing: true}
	laid := &scan.WitnessFile{Path: witnessPath, Content: []byte(witnessCode)}
	r.Statement.Predicate.Promises = []format.ScanPromise{splitPromise, rangePromise}
	r.Statement.Predicate.Tests[0].Promises = []string{splitPromise.ID, rangePromise.ID}
	r.Findings[0].Killers = []scan.Killer{
		{Route: r.Findings[0].Route},
		{Route: format.ScanRoute{Kind: format.ScanRouteKilledByTest, Test: "tests/test_split.py::test_trailing_comma"}, Command: []string{"python3", "-m", "pytest", "tests/test_split.py::test_trailing_comma"}},
		{Route: format.ScanRoute{Kind: format.ScanRouteWitness, Witness: witnessID}, Command: []string{"python3", "-m", "pytest", witnessPath + "::test_keeps_trailing_field"}, WitnessFile: laid},
	}
	r.Promises = &scan.PromiseReport{
		Checked: true, Promises: []format.ScanPromise{splitPromise, rangePromise},
		Dropped: []promise.Dropped{{}, {}},
		Unchecked: []scan.UncheckedPromise{
			{Promise: capPromise, Reason: promise.UncheckedOverCap},
			{Promise: budgetPromise, Reason: promise.UncheckedBudget},
		},
		Cut: []promise.Cut{
			{Ask: promise.LabelTicket(), Reason: promise.CutBudget},
			{Ask: promise.Generation(rangePromise.ID, 2), Reason: promise.CutBudget},
			{Ask: promise.Writer(budgetPromise.ID, 1), Reason: promise.CutPromiseNotKept},
		},
		Unasked: []promise.Ask{promise.Scope(splitPromise.ID, 0)},
		Broken: []scan.BrokenPromise{{
			Rank: 1, Promise: splitPromise, Witness: witness, File: []format.ScanWitness{witness}, Spec: []format.ScanPromise{splitPromise, rangePromise},
			Mutant: format.ScanMutant{ID: strings.Repeat("b", 64), Edits: []format.ScanEdit{{File: parse, Qualname: "split_fields", NewSource: afterParse}}},
			Site:   format.ScanLine{File: parse, Line: 3}, Files: r.Findings[0].Files,
			Killer: scan.Killer{Route: format.ScanRoute{Kind: format.ScanRouteWitness, Witness: witnessID}, Command: []string{"python3", "-m", "pytest", witnessPath + "::test_keeps_trailing_field"}, WitnessFile: laid},
		}},
		Questions: []scan.PromiseQuestion{
			{Promise: splitPromise, Witness: format.ScanWitness{ID: otherWitness}, Reason: promise.QuestionReferenceFails},
			{Promise: rangePromise, Witness: format.ScanWitness{ID: witnessID}, Reason: promise.QuestionUnstable},
		},
		Calls: 11, Budget: promise.Budget, WitnessesUnavailable: true,
	}
	return r
}

func TestHostText(t *testing.T) {
	text := scanreport.Text(hostReport(t))
	t.Run("a scan with a host lists its promises, broken promises, questions and unchecked entries"+verifiesPromises, func(t *testing.T) {
		goldenCompare(t, "host-text.golden", text)
		noBannedWord(t, text)
	})
	t.Run("a broken promise is ranked, quoted, proven by its witness and reproduced by a script"+verifiesBroken, func(t *testing.T) {
		for _, want := range []string{
			`#P1 "split_fields keeps a trailing field" (README.md) a change that passes the whole suite 3/3 fails a block-bearing witness written for it`,
			"   src/invoicing/parse.py:3  >= → >",
			"   proven by: witness 9d1c0e77a4b2 fails on it, reproduced",
			"   repro: .spec-proof/scan/3f2a9c1/promise-1/killer.sh",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("missing %q in\n%s", want, text)
			}
		}
	})
	t.Run("a question says whether the reference fails its witness or the witness is unstable"+verifiesQuestions, func(t *testing.T) {
		for _, want := range []string{"the reference fails it", "unstable on the reference"} {
			if !strings.Contains(text, want) {
				t.Errorf("missing %q", want)
			}
		}
	})
	t.Run("the unchecked promises, the cut and unasked asks and the dropped quotes are named with their reasons"+verifiesUnchecked, func(t *testing.T) {
		for _, want := range []string{
			"over the cap of 3 promises", "the budget of 18 calls could not cover it", "the labels ticket not asked",
			"generation ticket 3 for", "left unasked: the session reached its call cap",
			"2 extracted quotes not found byte for byte in their source", "this ecosystem writes no witness",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("missing %q", want)
			}
		}
	})
	t.Run("a finding whose test is labelled lists the promises it may break"+verifiesLabels, func(t *testing.T) {
		want := `   may break: "split_fields keeps a trailing field", "lines shorter than two characters are skipped"`
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in\n%s", want, text)
		}
		if strings.Contains(scanreport.Text(report(t)), "may break") {
			t.Fatal("a scan without a host labels a finding")
		}
	})
}

func TestHostMarkdown(t *testing.T) {
	markdown := scanreport.Markdown(hostReport(t))
	t.Run("the report has the sections Promises, Broken promises, Questions and Unchecked"+verifiesPromises, func(t *testing.T) {
		goldenCompare(t, "host-markdown.golden", markdown)
		noBannedWord(t, markdown)
		for _, heading := range []string{"### Promises\n", "### Broken promises\n", "### Questions\n", "### Unchecked\n"} {
			if !strings.Contains(markdown, heading) {
				t.Errorf("missing %q", heading)
			}
		}
	})
	t.Run("a broken promise shows its change as a diff"+verifiesBroken, func(t *testing.T) {
		if !strings.Contains(markdown, "- Reproduce: `.spec-proof/scan/3f2a9c1/promise-1/killer.sh`\n\n```diff\n--- a/src/invoicing/parse.py") {
			t.Fatalf("no diff under the broken promise:\n%s", markdown)
		}
	})
	t.Run("a finding lists the promises it may break"+verifiesLabels, func(t *testing.T) {
		if !strings.Contains(markdown, `- May break: "split_fields keeps a trailing field", "lines shorter than two characters are skipped"`) {
			t.Fatalf("no label line:\n%s", markdown)
		}
	})
}

func TestHostNoPromise(t *testing.T) {
	t.Run("a host that found no quotable promise says so in the text and the report"+verifiesNoPromise, func(t *testing.T) {
		r := report(t)
		r.Promises = &scan.PromiseReport{Checked: true, Calls: 1, Budget: promise.Budget}
		sentence := "no quotable promise in the README, description or closing issues"
		for _, render := range []string{scanreport.Text(r), scanreport.Markdown(r)} {
			if !strings.Contains(render, sentence) || !strings.Contains(render, "1 of 18 model calls · promises checked") {
				t.Errorf("render:\n%s", render)
			}
			noBannedWord(t, render)
		}
		goldenCompare(t, "host-no-promise.golden", scanreport.Text(r))
	})
	t.Run("a host that did not answer the extraction leaves the promises not checked"+verifiesNoPromise, func(t *testing.T) {
		r := report(t)
		r.Promises = &scan.PromiseReport{Calls: 2, Budget: promise.Budget}
		text := scanreport.Text(r)
		if !strings.Contains(text, "promises not checked: the host did not answer the extraction") {
			t.Fatalf("text:\n%s", text)
		}
		goldenCompare(t, "host-not-checked.golden", text)
	})
}

func TestFooter(t *testing.T) {
	t.Run("a scan without a host keeps the model-free footer"+verifiesFooter, func(t *testing.T) {
		if !strings.Contains(scanreport.Text(report(t)), " · no model used · promises not checked\n") {
			t.Fatal("the footer of a plain scan changed")
		}
	})
	t.Run("a scan with a host counts its calls and says whether its promises were checked"+verifiesFooter, func(t *testing.T) {
		r := hostReport(t)
		if !strings.Contains(scanreport.Text(r), " · 11 of 18 model calls · promises checked\n") {
			t.Fatalf("text:\n%s", scanreport.Text(r))
		}
		r.Promises.Checked = false
		if !strings.Contains(scanreport.Text(r), " · 11 of 18 model calls · promises not checked\n") {
			t.Fatalf("text:\n%s", scanreport.Text(r))
		}
	})
}

// dump writes the files whose path starts with a prefix, each under its path.
func dump(files []scanreport.File, prefix string) string {
	var out strings.Builder
	for _, file := range files {
		if strings.HasPrefix(file.Path, prefix) {
			fmt.Fprintf(&out, "=== %s (executable: %t)\n%s", file.Path, file.Executable, file.Content)
		}
	}
	return out.String()
}

func TestKillerFiles(t *testing.T) {
	r := hostReport(t)
	files, err := scanreport.Files(r)
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]scanreport.File{}
	for _, file := range files {
		byPath[file.Path] = file
	}
	const folder = ".spec-proof/scan/3f2a9c1/1/"
	t.Run("a finding folder lists its killers and holds a script for each that has a command"+verifiesKillers, func(t *testing.T) {
		goldenCompare(t, "host-finding-folder.golden", dump(files, folder))
		document := string(byPath[folder+"killers.md"].Content)
		for _, want := range []string{
			"A test that fails on this mutant: `tests/test_split.py::test_trailing_comma`, which fails 3/3. Run it with `sh killer-2.sh`.",
			"witness 9d1c0e77a4b2, in `witness/tests/test_promise_witness.py`. Run it with `sh killer-3.sh`.",
			`input line="a,", reference "['a,']", mutant raises IndexError. Re-run it with ` + "`spec-proof scan replay .spec-proof/scan/3f2a9c1/1`.",
		} {
			if !strings.Contains(document, want) {
				t.Errorf("missing %q in\n%s", want, document)
			}
		}
		if _, found := byPath[folder+"killer-1.sh"]; found {
			t.Fatal("a differential killer has a script")
		}
		noBannedWord(t, document)
	})
	t.Run("the script of a witness killer copies the witness file in before it runs the command"+verifiesKillers, func(t *testing.T) {
		script := byPath[folder+"killer-3.sh"]
		if !script.Executable || !strings.Contains(string(script.Content), `cp "$finding"/'witness/tests/test_promise_witness.py' 'tests/test_promise_witness.py'`+"\n'python3'") {
			t.Fatalf("script:\n%s", script.Content)
		}
		if string(byPath[folder+"witness/"+witnessPath].Content) != witnessCode {
			t.Fatal("the witness file is not laid out")
		}
	})
	t.Run("the reproduction script of a finding and its other files are unchanged by the killers"+verifiesKillers, func(t *testing.T) {
		plain, err := scanreport.Files(report(t))
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range plain {
			if strings.Contains(file.Path, "killer") {
				t.Fatalf("a plain finding without killers has %s", file.Path)
			}
			if got := byPath[file.Path]; strings.HasSuffix(file.Path, "repro.sh") && string(got.Content) != string(file.Content) {
				t.Fatalf("%s changed", file.Path)
			}
		}
	})
	t.Run("a broken promise has a folder with its patch, killer, witness, killers and record"+verifiesBrokenDir, func(t *testing.T) {
		const broken = ".spec-proof/scan/3f2a9c1/promise-1/"
		goldenCompare(t, "host-broken-folder.golden", dump(files, broken))
		record, err := scan.DecodeFinding(byPath[broken+"finding.json"].Content)
		if err != nil || record.Promise == nil || record.Promise.ID != splitPromise.ID || record.Route.Witness != witnessID || record.Commit != commit {
			t.Fatalf("record %+v, %v", record, err)
		}
		for _, name := range []string{"mutant.patch", "killer.sh", "killers.md", "witness/" + witnessPath} {
			if _, found := byPath[broken+name]; !found {
				t.Errorf("no %s", name)
			}
		}
		if !byPath[broken+"killer.sh"].Executable {
			t.Fatal("killer.sh is not executable")
		}
	})
}
