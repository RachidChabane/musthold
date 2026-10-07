package node_test

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/ecosystem/node"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/testkit/bindings"
)

func boundTests(t *testing.T, name ecosystem.Name) []ecosystem.BoundTest {
	t.Helper()
	loaded, err := bindings.Load(name)
	if err != nil {
		t.Fatal(err)
	}
	return loaded.All()
}

// patternOf reads the title regex of a selection: the argument after -t.
func patternOf(t *testing.T, selection ecosystem.Selection) *regexp.Regexp {
	t.Helper()
	for index, arg := range selection.Args {
		if arg == "-t" {
			return regexp.MustCompile(selection.Args[index+1])
		}
	}
	t.Fatalf("no -t in %v", selection.Args)
	return nil
}

func TestJestSelector(t *testing.T) {
	bound := boundTests(t, ecosystem.Jest)
	selector := jestStrategy(nil, nil).Selector()

	t.Run("the bound tests are selected by path and by an anchored title regex [verifies SP-1013~1]", func(t *testing.T) {
		selection, err := selector.Select(bound, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(selection.Args[:2], []string{"--runTestsByPath", "test/rates.test.ts"}) || selection.Args[2] != "-t" || len(selection.Tests) != len(bound) {
			t.Fatalf("selection = %+v", selection)
		}
		pattern := patternOf(t, selection)
		for _, test := range bound {
			if !pattern.MatchString(strings.TrimPrefix(test.Name, " ")) {
				t.Errorf("the regex %s misses %q", pattern, test.Name)
			}
		}
		for _, other := range []string{"line totals are zero for no units", bound[0].Name + " extra", "x" + bound[0].Name} {
			if pattern.MatchString(other) {
				t.Errorf("the regex %s selects %q", pattern, other)
			}
		}
	})

	t.Run("the leading space jest-junit gives a test outside any describe is not part of its title [verifies SP-1013~1]", func(t *testing.T) {
		top := ecosystem.NewBoundTest(" sums items", " sums items", "test/cart.test.js", 4)
		selection, err := selector.Select([]ecosystem.BoundTest{top}, nil)
		if err != nil || !patternOf(t, selection).MatchString("sums items") {
			t.Fatalf("%+v %v", selection, err)
		}
	})

	t.Run("regex characters of a title are escaped and one title is selected once [verifies SP-1013~1]", func(t *testing.T) {
		title := "applies 10% (flat) $5 off"
		a := ecosystem.NewBoundTest("describe "+title, "describe "+title, "test/a.test.js", 1)
		selection, err := selector.Select([]ecosystem.BoundTest{a, a}, nil)
		if err != nil {
			t.Fatal(err)
		}
		pattern := patternOf(t, selection)
		if !pattern.MatchString("describe "+title) || pattern.MatchString("describe applies 10% flat 5 off") {
			t.Fatalf("pattern = %s", pattern)
		}
		if len(selection.Tests) != 2 || strings.Count(selection.Args[len(selection.Args)-1], "describe") != 1 {
			t.Fatalf("selection = %+v", selection)
		}
	})

	t.Run("each file is selected once, however many tests it holds [verifies SP-1013~1]", func(t *testing.T) {
		a := ecosystem.NewBoundTest("a one", "a one", "test/a.test.js", 1)
		b := ecosystem.NewBoundTest("b one", "b one", "test/b.test.js", 1)
		selection, err := selector.Select([]ecosystem.BoundTest{a, b, a}, nil)
		if err != nil || !reflect.DeepEqual(selection.Args[:3], []string{"--runTestsByPath", "test/a.test.js", "test/b.test.js"}) {
			t.Fatalf("%+v %v", selection, err)
		}
	})

	t.Run("a bound test without a file is an error [verifies SP-1013~1]", func(t *testing.T) {
		orphan := ecosystem.NewBoundTest("x", "x", "", 0)
		if _, err := selector.Select([]ecosystem.BoundTest{orphan}, nil); !errors.Is(err, node.ErrUnmappedTest) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("no bound test selects nothing [verifies SP-1013~1]", func(t *testing.T) {
		selection, err := selector.Select(nil, nil)
		if err != nil || len(selection.Args) != 0 || len(selection.Tests) != 0 {
			t.Fatalf("%+v %v", selection, err)
		}
	})
}

func TestVitestSelector(t *testing.T) {
	bound := boundTests(t, ecosystem.Vitest)
	selector := vitestStrategy(nil, nil).Selector()

	t.Run("the bound tests are selected by an anchored title regex, then by file [verifies SP-1014~1]", func(t *testing.T) {
		selection, err := selector.Select(bound, nil)
		if err != nil {
			t.Fatal(err)
		}
		if selection.Args[0] != "-t" || selection.Args[len(selection.Args)-1] != "test/rates.test.ts" || len(selection.Tests) != len(bound) {
			t.Fatalf("selection = %+v", selection)
		}
		pattern := patternOf(t, selection)
		for _, test := range bound {
			for _, spelling := range []string{test.Name, strings.ReplaceAll(test.Name, " > ", " ")} {
				if !pattern.MatchString(spelling) {
					t.Errorf("the regex %s misses %q", pattern, spelling)
				}
			}
		}
		if pattern.MatchString("line totals > are zero for no units") || pattern.MatchString(bound[len(bound)-1].Name+" > extra") {
			t.Errorf("the regex %s selects an unbound test", pattern)
		}
	})

	t.Run("a title that holds the separator of its parents is still selected [verifies SP-1014~1]", func(t *testing.T) {
		test := ecosystem.NewBoundTest("test/a.test.ts", "group > step -> 90", "test/a.test.ts", 3)
		selection, err := selector.Select([]ecosystem.BoundTest{test}, nil)
		if err != nil || !patternOf(t, selection).MatchString("group > step -> 90") {
			t.Fatalf("%+v %v", selection, err)
		}
	})

	t.Run("a bound test without a file is an error [verifies SP-1014~1]", func(t *testing.T) {
		orphan := ecosystem.NewBoundTest("", "x", "", 0)
		if _, err := selector.Select([]ecosystem.BoundTest{orphan}, nil); !errors.Is(err, node.ErrUnmappedTest) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestRunnerCommands(t *testing.T) {
	workspace := ecosystem.Workspace{Dir: "/work/ref", ResultsDir: "/work/results/ref"}
	toolchain := ecosystem.Toolchain{Command: []string{"npx", "--no-install"}}
	selection := ecosystem.Selection{Args: []string{"--runTestsByPath", "test/a.test.ts", "-t", "^a$"}, Tests: []ecosystem.TestID{"a::a"}}

	t.Run("the Jest command is exactly the documented one [verifies SP-1013~1]", func(t *testing.T) {
		process, reader := &recorder{}, &fixedReader{}
		if _, err := jestStrategy(process, reader).Runner().Run(context.Background(), workspace, toolchain, selection); err != nil {
			t.Fatal(err)
		}
		command := process.commands[0]
		want := []string{"npx", "--no-install", "jest", "--ci", "--reporters=jest-junit", "--runInBand", "--watchman=false", "--runTestsByPath", "test/a.test.ts", "-t", "^a$"}
		if !reflect.DeepEqual(command.Argv, want) || command.Dir != workspace.Dir {
			t.Fatalf("command = %+v", command)
		}
		output := ""
		for _, entry := range command.Env {
			if strings.HasPrefix(entry, "JEST_JUNIT_OUTPUT_FILE=") {
				output = strings.TrimPrefix(entry, "JEST_JUNIT_OUTPUT_FILE=")
			}
		}
		if !strings.HasPrefix(output, workspace.ResultsDir) || len(reader.patterns) != 1 || reader.patterns[0] != output || reader.format != ecosystem.ReportJestJUnit {
			t.Fatalf("report = %q, reader got %v %v", output, reader.patterns, reader.format)
		}
	})

	t.Run("the Vitest command is exactly the documented one [verifies SP-1014~1]", func(t *testing.T) {
		process, reader := &recorder{}, &fixedReader{}
		vitestSelection := ecosystem.Selection{Args: []string{"-t", "^a$", "test/a.test.ts"}, Tests: selection.Tests}
		if _, err := vitestStrategy(process, reader).Runner().Run(context.Background(), workspace, toolchain, vitestSelection); err != nil {
			t.Fatal(err)
		}
		argv := process.commands[0].Argv
		if !reflect.DeepEqual(argv[:5], []string{"npx", "--no-install", "vitest", "run", "--reporter=junit"}) ||
			!strings.HasPrefix(argv[5], "--outputFile="+workspace.ResultsDir) || !reflect.DeepEqual(argv[6:], []string{"--maxWorkers=1", "-t", "^a$", "test/a.test.ts"}) {
			t.Fatalf("argv = %v", argv)
		}
		if len(reader.patterns) != 1 || reader.patterns[0] != strings.TrimPrefix(argv[5], "--outputFile=") || reader.format != ecosystem.ReportVitest {
			t.Fatalf("reader got %v %v", reader.patterns, reader.format)
		}
	})

	t.Run("an empty selection runs the whole suite [verifies SP-1013~1]", func(t *testing.T) {
		process := &recorder{}
		if _, err := jestStrategy(process, &fixedReader{}).Runner().Run(context.Background(), workspace, toolchain, ecosystem.Selection{}); err != nil {
			t.Fatal(err)
		}
		if got := process.commands[0].Argv; got[len(got)-1] != "--watchman=false" {
			t.Fatalf("argv = %v", got)
		}
	})

	t.Run("a failing test is not a collection failure and a timeout is reported [verifies SP-1013~1]", func(t *testing.T) {
		failing := &recorder{exit: func(ports.Command) int { return 1 }}
		reader := &fixedReader{outcomes: map[ecosystem.TestID]ecosystem.TestOutcome{"a::a": ecosystem.TestFailed}}
		got, err := jestStrategy(failing, reader).Runner().Run(context.Background(), workspace, toolchain, selection)
		if err != nil || got.CollectionFailed || got.Outcomes["a::a"] != ecosystem.TestFailed {
			t.Fatalf("%+v %v", got, err)
		}
		slow, err := vitestStrategy(&recorder{timedOut: true}, &fixedReader{}).Runner().Run(context.Background(), workspace, toolchain, selection)
		if err != nil || !slow.TimedOut {
			t.Fatalf("%+v %v", slow, err)
		}
	})
}

func TestUnselectedTestsAreNotOutcomes(t *testing.T) {
	workspace := ecosystem.Workspace{Dir: "/work/ref", ResultsDir: "/work/results/ref"}
	toolchain := ecosystem.Toolchain{Command: []string{"npx"}}
	reported := func() *fixedReader {
		return &fixedReader{outcomes: map[ecosystem.TestID]ecosystem.TestOutcome{
			"a::a": ecosystem.TestPassed, "a::skipped bound": ecosystem.TestSkipped, "a::filtered out": ecosystem.TestSkipped, "a::other": ecosystem.TestFailed,
		}}
	}

	t.Run("a test the title filter left out is dropped, the others stay [verifies SP-1015~1]", func(t *testing.T) {
		selection := ecosystem.Selection{Args: []string{"-t", "x"}, Tests: []ecosystem.TestID{"a::a", "a::skipped bound"}}
		for name, runner := range map[string]ports.SuiteRunner{"jest": jestStrategy(&recorder{}, reported()).Runner(), "vitest": vitestStrategy(&recorder{}, reported()).Runner()} {
			got, err := runner.Run(context.Background(), workspace, toolchain, selection)
			if err != nil {
				t.Fatal(err)
			}
			want := map[ecosystem.TestID]ecosystem.TestOutcome{"a::a": ecosystem.TestPassed, "a::skipped bound": ecosystem.TestSkipped, "a::other": ecosystem.TestFailed}
			if !reflect.DeepEqual(got.Outcomes, want) {
				t.Errorf("%s outcomes = %v", name, got.Outcomes)
			}
		}
	})

	t.Run("a full run keeps every outcome [verifies SP-1015~1]", func(t *testing.T) {
		got, err := jestStrategy(&recorder{}, reported()).Runner().Run(context.Background(), workspace, toolchain, ecosystem.Selection{Tests: []ecosystem.TestID{"a::a"}})
		if err != nil || len(got.Outcomes) != 4 {
			t.Fatalf("%+v %v", got, err)
		}
	})
}
