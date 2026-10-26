package scanscope_test

import (
	"bytes"
	"fmt"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/operator"
	"github.com/RachidChabane/musthold/discpower/internal/domain/scanscope"
	"github.com/RachidChabane/musthold/discpower/internal/domain/vacuity"
)

const (
	verifiesSample    = " [verifies SP-2104~1]"
	verifiesSeed      = " [verifies SP-2105~1]"
	verifiesLines     = " [verifies SP-2106~1]"
	verifiesNever     = " [verifies SP-2107~2]"
	verifiesFunctions = " [verifies SP-2109~1]"

	maxFiles        = 5
	maxTestsPerFile = 6
	maxSampleSize   = 12
)

func cases(t *rapid.T) []scanscope.Case {
	files := rapid.IntRange(1, maxFiles).Draw(t, "files")
	var all []scanscope.Case
	for file := range files {
		for test := range rapid.IntRange(1, maxTestsPerFile).Draw(t, fmt.Sprintf("tests of %d", file)) {
			path := fmt.Sprintf("tests/test_%d.py", file)
			all = append(all, scanscope.Case{ID: ecosystem.TestID(fmt.Sprintf("%s::test_%d", path, test)), File: path})
		}
	}
	return all
}

func TestSample(t *testing.T) {
	t.Run("the sample is the same for the same seed whatever the order of the cases"+verifiesSample, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			all := cases(t)
			size := rapid.IntRange(0, maxSampleSize).Draw(t, "size")
			seed := rapid.Uint64().Draw(t, "seed")
			shuffled := slices.Clone(all)
			slices.Reverse(shuffled)
			if first, second := scanscope.Sample(all, size, seed), scanscope.Sample(shuffled, size, seed); !slices.Equal(first, second) {
				t.Fatalf("%v != %v", first, second)
			}
		})
	})

	t.Run("the sample holds min(size, tests) distinct tests, sorted, each of the cases"+verifiesSample, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			all := cases(t)
			size := rapid.IntRange(0, maxSampleSize).Draw(t, "size")
			sample := scanscope.Sample(all, size, rapid.Uint64().Draw(t, "seed"))
			if len(sample) != min(size, len(all)) || !slices.IsSorted(sample) || len(slices.Compact(slices.Clone(sample))) != len(sample) {
				t.Fatalf("sample %v of %d cases at size %d", sample, len(all), size)
			}
			for _, test := range sample {
				if !slices.ContainsFunc(all, func(c scanscope.Case) bool { return c.ID == test }) {
					t.Fatalf("%s is no case", test)
				}
			}
		})
	})

	t.Run("no file gives a second test before every file gave one"+verifiesSample, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			all := cases(t)
			files := map[string]int{}
			for _, c := range all {
				files[c.File]++
			}
			size := rapid.IntRange(1, len(files)).Draw(t, "size")
			sample := scanscope.Sample(all, size, rapid.Uint64().Draw(t, "seed"))
			seen := map[string]bool{}
			for _, test := range sample {
				file := fileOf(all, test)
				if seen[file] {
					t.Fatalf("%s drawn twice in a sample of %d over %d files", file, size, len(files))
				}
				seen[file] = true
			}
		})
	})

	t.Run("another seed may draw another sample"+verifiesSample, func(t *testing.T) {
		var all []scanscope.Case
		for test := range 20 {
			all = append(all, scanscope.Case{ID: ecosystem.TestID(fmt.Sprintf("t.py::test_%02d", test)), File: "t.py"})
		}
		if slices.Equal(scanscope.Sample(all, 3, 1), scanscope.Sample(all, 3, 2)) && slices.Equal(scanscope.Sample(all, 3, 1), scanscope.Sample(all, 3, 3)) {
			t.Fatal("three seeds drew the same sample")
		}
	})
}

func fileOf(all []scanscope.Case, test ecosystem.TestID) string {
	for _, c := range all {
		if c.ID == test {
			return c.File
		}
	}
	return ""
}

func TestDrawSeed(t *testing.T) {
	t.Run("a scan seed gives one operator seed, never the local proof seed"+verifiesSeed, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			seed := rapid.Uint64().Draw(t, "seed")
			drawn := scanscope.DrawSeed(seed)
			if drawn != scanscope.DrawSeed(seed) || drawn == operator.LocalSeed || uint64(drawn) == seed {
				t.Fatalf("seed %d draws %d", seed, drawn)
			}
		})
	})
}

func TestSpanLines(t *testing.T) {
	source := []byte("a = 1\nif a >= 2:\n    b = 3\n")
	line := func(n int) vacuity.Line { return vacuity.Line{File: "m.py", Line: n} }
	cases := []struct {
		name string
		span ecosystem.Span
		want []vacuity.Line
	}{
		{"a token on one line", ecosystem.Span{Start: 11, End: 13}, []vacuity.Line{line(2)}},
		{"a span over two lines", ecosystem.Span{Start: 6, End: 21}, []vacuity.Line{line(2), line(3)}},
		{"a span that ends on a line break", ecosystem.Span{Start: 0, End: 6}, []vacuity.Line{line(1)}},
		{"an insertion", ecosystem.Span{Start: 17, End: 17}, []vacuity.Line{line(3)}},
		{"a span outside the file", ecosystem.Span{Start: 10, End: 99}, nil},
	}
	for _, c := range cases {
		t.Run(c.name+verifiesLines, func(t *testing.T) {
			if got := scanscope.SpanLines("m.py", source, c.span); !slices.Equal(got, c.want) {
				t.Fatalf("lines = %v, want %v", got, c.want)
			}
		})
	}
	t.Run("the lines of a span inside the file are consecutive and hold its first byte"+verifiesLines, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			content := []byte(rapid.StringMatching(`([a-z =]{0,6}\n){0,6}`).Draw(t, "content"))
			start := rapid.IntRange(0, len(content)).Draw(t, "start")
			end := rapid.IntRange(start, len(content)).Draw(t, "end")
			lines := scanscope.SpanLines("f", content, ecosystem.Span{Start: start, End: end})
			if len(lines) == 0 {
				t.Fatal("no line")
			}
			for index := 1; index < len(lines); index++ {
				if lines[index].Line != lines[index-1].Line+1 {
					t.Fatalf("lines %v", lines)
				}
			}
			if first := 1 + bytes.Count(content[:start], []byte("\n")); lines[0].Line != first {
				t.Fatalf("first line %d, want %d", lines[0].Line, first)
			}
		})
	})
}

func TestNeverMutated(t *testing.T) {
	layout := ecosystem.Layout{TestFileGlobs: []string{"test_*.py", "*_test.go"}}
	cases := []struct {
		name    string
		path    string
		content string
		mutable bool
	}{
		{"a source file", "src/parse.py", "def f():\n    return 1\n", true},
		{"a test file", "src/test_parse.py", "def test_f():\n    pass\n", false},
		{"a helper of a test directory", "tests/helpers.py", "def make():\n    return 1\n", false},
		{"a module of a __tests__ directory", "src/__tests__/helpers.ts", "export const make = () => 1;\n", false},
		{"a manual mock", "src/__mocks__/clock.ts", "export const now = () => 0;\n", false},
		{"a fixture module", "src/__fixtures__/orders.ts", "export const orders = [];\n", false},
		{"a TypeScript source file", "src/totals.ts", "export const total = 1;\n", true},
		{"a vendored package", "vendor/lib/lib.go", "package lib\n", false},
		{"an installed package", ".venv/lib/site-packages/x.py", "x = 1\n", false},
		{"a migration", "app/migrations/0001_initial.py", "x = 1\n", false},
		{"generated Go code", "api/api.pb.go", "// Code generated by protoc-gen-go. DO NOT EDIT.\npackage api\n", false},
		{"generated Python code", "api/schema.py", "# Code generated by tool. DO NOT EDIT.\nx = 1\n", false},
		{"code marked as generated", "api/client.py", "# @generated\nx = 1\n", false},
	}
	for _, c := range cases {
		t.Run(c.name+verifiesNever, func(t *testing.T) {
			if got := scanscope.Mutable(c.path, []byte(c.content), layout); got != c.mutable {
				t.Fatalf("Mutable(%s) = %v", c.path, got)
			}
		})
	}
	sinks := map[string]bool{
		`    print("total", total)`:             true,
		`    logging.info("total %s", total)`:   true,
		`    logger.warning("late")`:            true,
		`	log.Printf("total %d", total)`:        true,
		`	fmt.Println(total)`:                   true,
		`	slog.Info("total", "value", total)`:   true,
		`    metrics.increment("orders")`:       true,
		`  console.log('total', total);`:        true,
		`  console.error(error);`:               true,
		`  return consoleTotal(total);`:         false,
		`    return total * rate`:               false,
		`	if total >= limit {`:                  false,
		`    blueprint(total)`:                  false,
		`    catalog.find(total)`:               false,
		`    fingerprint = digest(total)`:       false,
		`	logical := total > 0 && rate < limit`: false,
	}
	for line, sink := range sinks {
		t.Run(fmt.Sprintf("%q calls a sink: %v", line, sink)+verifiesNever, func(t *testing.T) {
			if got := scanscope.SinkLine(line); got != sink {
				t.Fatalf("SinkLine = %v", got)
			}
		})
	}
}

func TestDefinitionLines(t *testing.T) {
	definitions := []ecosystem.Definition{
		{Qualname: "f", Kind: ecosystem.KindFunction, StartLine: 1, EndLine: 3},
		{Qualname: "g", Kind: ecosystem.KindFunction, StartLine: 5, EndLine: 6},
		{Qualname: "LIMIT", Kind: ecosystem.KindAssignment, StartLine: 8, EndLine: 8},
	}
	t.Run("a changed line marks every line of the functions that hold it, and no assignment"+verifiesFunctions, func(t *testing.T) {
		got := scanscope.DefinitionLines("m.py", definitions, []int{2, 8})
		want := []vacuity.Line{{File: "m.py", Line: 1}, {File: "m.py", Line: 2}, {File: "m.py", Line: 3}}
		if !slices.Equal(got, want) {
			t.Fatalf("lines = %v", got)
		}
	})
}
