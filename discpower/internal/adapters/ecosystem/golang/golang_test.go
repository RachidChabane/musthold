package golang_test

import (
	"context"
	"errors"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/ecosystem/golang"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/sourceparse/goparser"
	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/testkit/bindings"
)

type recorder struct {
	commands []ports.Command
	exit     int
}

func (r *recorder) Run(_ context.Context, command ports.Command) (ports.CommandResult, error) {
	r.commands = append(r.commands, command)
	return ports.CommandResult{ExitCode: r.exit, Stdout: []byte("build failed")}, nil
}

type fixedReader struct {
	patterns []string
	format   ecosystem.ReportFormat
}

func (f *fixedReader) Read(patterns []string, format ecosystem.ReportFormat) (map[ecosystem.TestID]ecosystem.TestOutcome, error) {
	f.patterns, f.format = patterns, format
	return nil, nil
}

func strategy(process ports.ProcessRunner, reader ports.ResultsReader, lookPath func(string) (string, error)) *golang.Strategy {
	return golang.New(golang.Dependencies{Process: process, Reader: reader, Locator: goparser.NewLocator(), LookPath: lookPath})
}

func found(name string) (string, error) { return "/usr/local/go/bin/" + name, nil }

func TestDetectAndLayout(t *testing.T) {
	t.Run("a go.mod marks a Go module [verifies SP-73~1]", func(t *testing.T) {
		detection, ok := strategy(nil, nil, found).Detect(fstest.MapFS{"go.mod": {}})
		if !ok || detection.Name != ecosystem.GoTesting || detection.Root != "." || !reflect.DeepEqual(detection.Evidence, []string{"go.mod"}) {
			t.Fatalf("%+v %v", detection, ok)
		}
		if _, ok := strategy(nil, nil, found).Detect(fstest.MapFS{"main.go": {}}); ok {
			t.Fatal("a tree without go.mod was detected")
		}
	})

	t.Run("_test.go files are test files [verifies SP-62~1]", func(t *testing.T) {
		layout := strategy(nil, nil, found).Layout()
		if !layout.IsTestFile("pkg/rates_test.go") || layout.IsTestFile("pkg/rates.go") {
			t.Fatal("test file recognition is wrong")
		}
	})

	t.Run("the go command is resolved from PATH and its absence is an error [verifies SP-75~1]", func(t *testing.T) {
		got, err := strategy(nil, nil, found).Toolchain().Resolve(context.Background(), ecosystem.Workspace{})
		if err != nil || !reflect.DeepEqual(got.Command, []string{"/usr/local/go/bin/go"}) {
			t.Fatalf("%v %v", got, err)
		}
		missing := func(string) (string, error) { return "", errors.New("absent") }
		if _, err := strategy(nil, nil, missing).Toolchain().Resolve(context.Background(), ecosystem.Workspace{}); !errors.Is(err, golang.ErrNoToolchain) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestBuildCheckCommand(t *testing.T) {
	toolchain := ecosystem.Toolchain{Command: []string{"go"}}
	workspace := ecosystem.Workspace{Dir: "/work/ref"}

	t.Run("the test binaries of the changed packages are compiled without running tests [verifies SP-75~1]", func(t *testing.T) {
		process := &recorder{}
		result, err := strategy(process, nil, found).BuildCheck().Check(context.Background(), workspace, toolchain, []string{"rates.go", "internal/tax/tax.go", "internal/tax/rate.go", "README.md"})
		want := []string{"go", "test", "-count=1", "-run", "^$", ".", "./internal/tax"}
		if err != nil || !result.OK || !reflect.DeepEqual(process.commands[0].Argv, want) || process.commands[0].Dir != workspace.Dir {
			t.Fatalf("%+v %v argv=%v", result, err, process.commands[0].Argv)
		}
	})

	t.Run("no changed Go file checks every package [verifies SP-75~1]", func(t *testing.T) {
		process := &recorder{}
		if _, err := strategy(process, nil, found).BuildCheck().Check(context.Background(), workspace, toolchain, nil); err != nil {
			t.Fatal(err)
		}
		if got := process.commands[0].Argv; got[len(got)-1] != "./..." {
			t.Fatalf("argv = %v", got)
		}
	})

	t.Run("a failing compilation is a failed build with its output [verifies SP-75~1]", func(t *testing.T) {
		process := &recorder{exit: 1}
		result, err := strategy(process, nil, found).BuildCheck().Check(context.Background(), workspace, toolchain, []string{"rates.go"})
		if err != nil || result.OK || result.Diagnostics != "build failed" {
			t.Fatalf("%+v %v", result, err)
		}
	})
}

func TestRunnerCommand(t *testing.T) {
	workspace := ecosystem.Workspace{Dir: "/work/ref", ResultsDir: "/work/results/ref"}
	toolchain := ecosystem.Toolchain{Command: []string{"go"}}

	t.Run("the gotestsum command is exactly the documented one [verifies SP-75~1]", func(t *testing.T) {
		process, reader := &recorder{}, &fixedReader{}
		selection := ecosystem.Selection{Args: []string{"-run", "^TestX$", "."}}
		if _, err := strategy(process, reader, found).Runner().Run(context.Background(), workspace, toolchain, selection); err != nil {
			t.Fatal(err)
		}
		argv := process.commands[0].Argv
		if !reflect.DeepEqual(argv[:4], []string{"go", "tool", "gotestsum", "--junitfile"}) || !reflect.DeepEqual(argv[5:], []string{"--", "-count=1", "-run", "^TestX$", "."}) {
			t.Fatalf("argv = %v", argv)
		}
		if reader.format != ecosystem.ReportGotestsum || reader.patterns[0] != argv[4] {
			t.Fatalf("reader got %v %v", reader.patterns, reader.format)
		}
	})

	t.Run("an empty selection runs every package [verifies SP-75~1]", func(t *testing.T) {
		process := &recorder{}
		if _, err := strategy(process, &fixedReader{}, found).Runner().Run(context.Background(), workspace, toolchain, ecosystem.Selection{}); err != nil {
			t.Fatal(err)
		}
		if got := process.commands[0].Argv; got[len(got)-1] != "./..." {
			t.Fatalf("argv = %v", got)
		}
	})
}

func TestSelector(t *testing.T) {
	loaded, err := bindings.Load(ecosystem.GoTesting)
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile("../../../../fixtures/ecosystems/go/rates_test.go")
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := goparser.NewLocator().Locate("rates_test.go", source)
	if err != nil {
		t.Fatal(err)
	}
	byFile := map[string][]ecosystem.Definition{"rates_test.go": definitions}
	sel := strategy(nil, nil, found).Selector()

	t.Run("subtests of every level are anchored and alternated [verifies SP-75~1]", func(t *testing.T) {
		got, err := sel.Select(loaded.All(), byFile)
		if err != nil {
			t.Fatal(err)
		}
		if got.Args[0] != "-run" || got.Args[2] != "." || len(got.Args) != 3 || len(got.Tests) != 7 {
			t.Fatalf("selection = %+v", got)
		}
		const topLevel = "^(TestLineTotal|TestDiscounted|TestShippingFee)$/^("
		expression := got.Args[1]
		if !strings.HasPrefix(expression, topLevel) || !strings.HasSuffix(expression, ")$") {
			t.Fatalf("expression = %s", expression)
		}
		for _, test := range loaded.All() {
			subtest := test.Name[strings.Index(test.Name, "/")+1:]
			if !strings.Contains(expression, regexp.QuoteMeta(subtest)) {
				t.Errorf("expression lacks %s", subtest)
			}
		}
	})

	t.Run("a bound parent test stops the expression at the top level [verifies SP-75~1]", func(t *testing.T) {
		parent := ecosystem.NewBoundTest("example.com/parcel", "TestShippingFee", "rates_test.go", 1)
		child := ecosystem.NewBoundTest("example.com/parcel", "TestLineTotal/is_zero_for_no_units", "rates_test.go", 1)
		got, err := sel.Select([]ecosystem.BoundTest{parent, child}, byFile)
		if err != nil || got.Args[1] != "^(TestShippingFee|TestLineTotal)$" {
			t.Fatalf("%+v %v", got, err)
		}
	})

	t.Run("a package outside the root is named relative to the module [verifies SP-75~1]", func(t *testing.T) {
		bound := ecosystem.NewBoundTest("example.com/parcel/tax", "TestShippingFee", "internal/tax/rates_test.go", 1)
		got, err := sel.Select([]ecosystem.BoundTest{bound}, map[string][]ecosystem.Definition{"internal/tax/rates_test.go": definitions})
		if err != nil || got.Args[2] != "./internal/tax" {
			t.Fatalf("%+v %v", got, err)
		}
	})

	t.Run("a test function the file does not declare is an error [verifies SP-75~1]", func(t *testing.T) {
		bound := ecosystem.NewBoundTest("example.com/parcel", "TestMissing/x", "rates_test.go", 1)
		if _, err := sel.Select([]ecosystem.BoundTest{bound}, byFile); !errors.Is(err, golang.ErrUnmappedTest) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("no bound test selects nothing [verifies SP-75~1]", func(t *testing.T) {
		got, err := sel.Select(nil, byFile)
		if err != nil || len(got.Args) != 0 {
			t.Fatalf("%+v %v", got, err)
		}
	})
}
