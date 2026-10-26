package node

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/vacuity"
)

const (
	verifiesRuns    = " [verifies SP-2292~1]"
	verifiesWitness = " [verifies SP-2296~1]"
	verifiesProbes  = " [verifies SP-2297~1]"
)

// encodeVLQ writes one value of a source map's mappings.
func encodeVLQ(value int) string {
	encoded := value << 1
	if value < 0 {
		encoded = (-value << 1) | 1
	}
	var out strings.Builder
	for {
		digit := encoded & vlqMask
		encoded >>= vlqShift
		if encoded > 0 {
			digit |= vlqContinuation
		}
		out.WriteByte(vlqAlphabet[digit])
		if encoded == 0 {
			return out.String()
		}
	}
}

// mapping is one segment of a generated line: its column and, when mapped,
// the zero-based line of the first source it comes from.
type mapping struct {
	column, line int
	mapped       bool
}

// sourceMapOf encodes a source map of one source whose generated lines hold
// the segments.
func sourceMapOf(t *testing.T, source string, lines [][]mapping) string {
	t.Helper()
	var generated []string
	previousLine := 0
	for _, segments := range lines {
		column := 0
		var encoded []string
		for _, segment := range segments {
			text := encodeVLQ(segment.column - column)
			column = segment.column
			if segment.mapped {
				text += encodeVLQ(0) + encodeVLQ(segment.line-previousLine) + encodeVLQ(0)
				previousLine = segment.line
			}
			encoded = append(encoded, text)
		}
		generated = append(generated, strings.Join(encoded, ","))
	}
	raw, err := json.Marshal(map[string]any{"version": 3, "sources": []string{source}, "mappings": strings.Join(generated, ";")})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestSourceMaps(t *testing.T) {
	raw := sourceMapOf(t, "totals.ts", [][]mapping{{{column: 0, line: 2, mapped: true}, {column: 6}, {column: 9, line: 4, mapped: true}}, {}, {{column: 2, line: 7, mapped: true}}})
	decoded, err := parseSourceMap(raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("a generated position maps to the line of the last segment at or before its column, and to nothing before the first or on a segment of no source"+verifiesOwnership, func(t *testing.T) {
		for _, c := range []struct {
			line, column, want int
			mapped             bool
		}{
			{0, 0, 2, true}, {0, 5, 2, true}, {0, 7, 0, false}, {0, 30, 4, true},
			{1, 0, 0, false}, {2, 1, 0, false}, {2, 2, 7, true}, {5, 0, 0, false},
		} {
			_, line, mapped := decoded.lookup(c.line, c.column)
			if mapped != c.mapped || (mapped && line != c.want) {
				t.Errorf("lookup(%d, %d) = %d %v", c.line, c.column, line, mapped)
			}
		}
	})
	t.Run("a map of another version, one with a digit outside base64 and one that maps outside its sources are refused"+verifiesOwnership, func(t *testing.T) {
		for _, bad := range []string{
			`{"version":2,"sources":["a.ts"],"mappings":"AAAA"}`,
			`{"version":3,"sources":["a.ts"],"mappings":"A*AA"}`,
			`{"version":3,"sources":["a.ts"],"mappings":"ACAA"}`,
			`not a map`,
		} {
			if _, err := parseSourceMap(bad); err == nil {
				t.Errorf("%s was decoded", bad)
			}
		}
	})
}

func TestGeometryOfAModule(t *testing.T) {
	const original = "export function total(a: number): number {\n  return a * 2;\n}\n"
	files := identityFiles(t, map[string]string{"src/totals.ts": original, "src/plain.js": "const a = 1;\n{\n}\n"})
	t.Run("a module behind the runner's wrapper maps its code, past the wrapper, through its source map"+verifiesOwnership, func(t *testing.T) {
		wrapper := "'use strict';async (__vite_ssr_exports__,module)=>{{"
		code := "function total(a) {\n  return a * 2;\n}\n"
		raw := sourceMapOf(t, "totals.ts", [][]mapping{{{column: 0, line: 0, mapped: true}}, {{column: 2, line: 1, mapped: true}}, {{column: 0, line: 2, mapped: true}}})
		shape := computeGeometry(scriptRecord{file: "src/totals.ts", source: wrapper + code + wrapperSuffix, sourceMap: raw, prefix: len(wrapper)}, files)
		first, holds := shape.at(len(wrapper))
		second, holdsSecond := shape.at(len(wrapper) + strings.Index(code, "return"))
		if !holds || first.Line != 1 || !holdsSecond || second.Line != 2 {
			t.Fatalf("at the start %v %v, at the return %v %v", first, holds, second, holdsSecond)
		}
		if _, inWrapper := shape.at(3); inWrapper {
			t.Fatal("the wrapper maps to a line")
		}
		if _, brace := shape.at(len(wrapper) + strings.Index(code, "}")); brace {
			t.Fatal("a brace holds code")
		}
	})
	t.Run("a module with no map maps by its own position when its code is its file byte for byte, and maps nothing otherwise"+verifiesOwnership, func(t *testing.T) {
		same := computeGeometry(scriptRecord{file: "src/plain.js", source: "const a = 1;\n{\n}\n"}, files)
		if line, holds := same.at(0); !holds || line.Line != 1 {
			t.Fatalf("identity %v %v", line, holds)
		}
		if _, holds := same.at(len("const a = 1;\n")); holds {
			t.Fatal("a line of braces holds code")
		}
		changed := computeGeometry(scriptRecord{file: "src/plain.js", source: "const a = 2;\n"}, files)
		if len(changed.lines) != 0 {
			t.Fatalf("a changed module maps %v", changed.lines)
		}
	})
	t.Run("a source outside the project or installed in it maps to nothing"+verifiesOwnership, func(t *testing.T) {
		for _, source := range []string{"../../elsewhere.ts", "../node_modules/lib/index.ts", "webpack://lib/x.ts"} {
			if _, inside := files.relative("src/totals.ts", "", source); inside {
				t.Errorf("%s is a project file", source)
			}
		}
		if relative, inside := files.relative("src/totals.ts", "", filepath.Join(files.roots[0], "src", "totals.ts")); !inside || relative != "src/totals.ts" {
			t.Errorf("an absolute source gives %q %v", relative, inside)
		}
	})
}

// runOf assembles a run of the given processes and reporter results.
func runOf(processes []processRecord, reported ...recordLine) runRecord {
	return runRecord{processes: processes, reported: reported}
}

func tested(id, outcome string) testRecord {
	return testRecord{id: ecosystem.TestID(id), file: fileOfTest(ecosystem.TestID(id)), outcome: outcome}
}

func TestOutcomesOfARun(t *testing.T) {
	t.Run("an id two tests share is errored, the others keep their outcome"+verifiesRuns, func(t *testing.T) {
		run := outcomesOf(runOf([]processRecord{{tests: []testRecord{tested("a.test.ts::x", outcomePassed), tested("a.test.ts::x", outcomePassed), tested("a.test.ts::y", outcomeFailed)}}}), vitest{}, false, false)
		if run.outcomes["a.test.ts::x"] != ecosystem.TestErrored || run.outcomes["a.test.ts::y"] != ecosystem.TestFailed || len(run.cases) != 2 {
			t.Fatalf("outcomes %v, cases %v", run.outcomes, run.cases)
		}
	})
	t.Run("ids equal ignoring case are errored for Jest, whose title filter ignores case, and kept for Vitest"+verifiesRuns, func(t *testing.T) {
		process := processRecord{tests: []testRecord{tested("a.test.ts::Total", outcomePassed), tested("a.test.ts::total", outcomePassed)}}
		jestRun := outcomesOf(runOf([]processRecord{process}), jest{}, false, false)
		vitestRun := outcomesOf(runOf([]processRecord{process}), vitest{}, false, false)
		if jestRun.outcomes["a.test.ts::Total"] != ecosystem.TestErrored || vitestRun.outcomes["a.test.ts::Total"] != ecosystem.TestPassed {
			t.Fatalf("jest %v, vitest %v", jestRun.outcomes, vitestRun.outcomes)
		}
	})
	t.Run("a test only the reporter saw keeps its status in a run of the whole suite and is errored in a selection"+verifiesRuns, func(t *testing.T) {
		reported := recordLine{Kind: lineResult, File: "b.test.ts", Tests: []reportedResult{{ID: "b.test.ts::z", Status: jestStatusPassed, Invocations: 1}}}
		whole := outcomesOf(runOf(nil, reported), jest{}, false, false)
		selected := outcomesOf(runOf(nil, reported), jest{}, true, false)
		if whole.outcomes["b.test.ts::z"] != ecosystem.TestPassed || selected.outcomes["b.test.ts::z"] != ecosystem.TestErrored {
			t.Fatalf("whole %v, selected %v", whole.outcomes, selected.outcomes)
		}
	})
	t.Run("a test of a file that stopped before any test began, or of a run whose global setup failed, did not start; any other missing test is errored or timed out"+verifiesRuns, func(t *testing.T) {
		stopped := outcomesOf(runOf([]processRecord{{failedFiles: []string{"c.test.ts"}}}), jest{}, true, false)
		if stopped.missingRun("c.test.ts::w") != vacuity.RunNotStarted || stopped.missingRun("d.test.ts::w") != vacuity.RunErrored {
			t.Fatal("a stopped file")
		}
		started := outcomesOf(runOf([]processRecord{{failedFiles: []string{"c.test.ts"}, tests: []testRecord{tested("c.test.ts::v", outcomeFailed)}}}), jest{}, true, false)
		if started.missingRun("c.test.ts::w") != vacuity.RunErrored {
			t.Fatal("a file one test of which began is no evidence")
		}
		setUp := outcomesOf(runRecord{globalSetupFailed: true}, vitest{}, true, false)
		timedOut := outcomesOf(runRecord{}, vitest{}, true, true)
		if setUp.missingRun("e.test.ts::v") != vacuity.RunNotStarted || timedOut.missingRun("e.test.ts::v") != vacuity.RunTimedOut {
			t.Fatal("a failed global setup or a timeout")
		}
	})
	t.Run("the outcomes the shipped scripts give map to the runs of a scan"+verifiesRuns, func(t *testing.T) {
		for outcome, want := range map[string]ecosystem.TestOutcome{outcomePassed: ecosystem.TestPassed, outcomeFailed: ecosystem.TestFailed, outcomeSkipped: ecosystem.TestSkipped, outcomeErrored: ecosystem.TestErrored, "anything": ecosystem.TestErrored} {
			if got := outcomeOf(outcome); got != want {
				t.Errorf("%s gives %s", outcome, got)
			}
		}
	})
}

func TestReadingARecord(t *testing.T) {
	dir := t.TempDir()
	lines := []string{
		`{"k":"process","runner":"vitest","recording":true,"capabilities":{"inspector":true,"synchronousTake":true,"retention":true}}`,
		`{"k":"capabilities","capabilities":{"inspector":true,"synchronousTake":true,"asyncHooks":true,"evaluation":true,"retention":true}}`,
		`{"k":"capabilities","capabilities":{"inspector":true,"synchronousTake":true,"asyncHooks":false,"evaluation":true,"retention":true}}`,
		`{"k":"script","id":"3","file":"src/a.ts","source":"x","map":"","prefix":0}`,
		`{"k":"take","at":"open","t":"a.test.ts::x","c":{"3":[[[0,1,1]]]}}`,
		`{"k":"test","id":"a.test.ts::x","file":"a.test.ts","outcome":"passed"}`,
		`{"k":"file","file":"b.test.ts","state":"failed"}`,
		`{"k":"take","at":"end","c":{"3":[[[0,`,
	}
	if err := os.WriteFile(filepath.Join(dir, "41.jsonl"), []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "42.jsonl"), []byte(`{"k":"take","at":"start","c":{}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, globalSetupFailure), []byte("boom"), 0o644); err != nil {
		t.Fatal(err)
	}
	record, err := readRunRecord(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("a record keeps its complete lines, ands the capabilities of the realms it checked, and a file with no process line is no record"+verifiesRuns, func(t *testing.T) {
		if len(record.processes) != 1 || !record.globalSetupFailed {
			t.Fatalf("%d processes, global setup failed %v", len(record.processes), record.globalSetupFailed)
		}
		process := record.processes[0]
		if process.capabilities.AsyncHooks || !process.capabilities.Evaluation || len(process.takes) != 1 || len(process.tests) != 1 || !slices.Equal(process.failedFiles, []string{"b.test.ts"}) {
			t.Fatalf("%+v", process)
		}
	})
	t.Run("a recording whose realm was never checked shows none of the realm's preconditions"+verifiesFallbacks, func(t *testing.T) {
		unchecked, _ := processOf([]recordLine{{Kind: lineProcess, Recording: true, Capabilities: &capabilities{Inspector: true, AsyncHooks: true, Evaluation: true}}})
		if unchecked.capabilities.AsyncHooks || unchecked.capabilities.Evaluation {
			t.Fatalf("%+v", unchecked.capabilities)
		}
	})
	t.Run("a missing record directory is an empty run"+verifiesRuns, func(t *testing.T) {
		empty, err := readRunRecord(filepath.Join(dir, "absent"))
		if err != nil || len(empty.processes) != 0 {
			t.Fatalf("%v %v", empty, err)
		}
	})
}

func TestScanIDOfAWitness(t *testing.T) {
	t.Run("a witness laid out in the witness suite is named by its file and the suite's and its own titles, whichever separator its runner reports"+verifiesWitness, func(t *testing.T) {
		for _, name := range []string{witnessSuite + jestTitleSeparator + "w_1", witnessSuite + vitestTitleSeparator + "w_1"} {
			got := scanIDOfBound(ecosystem.BoundTest{File: "test/musthold_1.test.ts", Name: name})
			if got != "test/musthold_1.test.ts::"+witnessSuite+" > w_1" {
				t.Errorf("%q gives %q", name, got)
			}
		}
	})
}

func TestScanOperationAndRepro(t *testing.T) {
	suite := scanSuite{framework: jest{}}
	t.Run("a top-level function of a source module binds to its probe target; a method, a test file and a declaration file do not"+verifiesProbes, func(t *testing.T) {
		if target, ok := suite.Operation(ecosystem.Workspace{}, "src/totals.ts", "discount"); !ok || target != "src/totals.ts:discount" {
			t.Fatalf("%q %v", target, ok)
		}
		for _, c := range [][2]string{{"src/totals.ts", "Cart.total"}, {"test/totals.test.ts", "helper"}, {"src/types.d.ts", "f"}, {"README.md", "f"}, {"src/totals.ts", "9lives"}} {
			if _, ok := suite.Operation(ecosystem.Workspace{}, c[0], c[1]); ok {
				t.Errorf("%v binds", c)
			}
		}
	})
	t.Run("the repro runs the one test by its file and an anchored pattern over its full title"+verifiesProbes, func(t *testing.T) {
		jestRepro := suite.Repro("test/discount.test.ts::discount > accepts (any) percent")
		vitestRepro := scanSuite{framework: vitest{}}.Repro("test/discount.test.ts::discount > accepts (any) percent")
		pattern := `^discount(?: > | )accepts \(any\) percent$`
		if !slices.Equal(jestRepro, []string{"npx", "--no-install", "jest", "--ci", "--runInBand", "--runTestsByPath", "test/discount.test.ts", "-t", pattern}) {
			t.Fatalf("jest %q", jestRepro)
		}
		if !slices.Equal(vitestRepro, []string{"npx", "--no-install", "vitest", "run", "test/discount.test.ts", "-t", pattern}) {
			t.Fatalf("vitest %q", vitestRepro)
		}
	})
}
