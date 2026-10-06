package pytest_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/ecosystem/pytest"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/sourceparse/treesitter"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/testkit/bindings"
)

// recorder is a ProcessRunner that records commands and answers by exit code.
type recorder struct {
	commands []ports.Command
	exit     func(ports.Command) int
	timedOut bool
}

func (r *recorder) Run(_ context.Context, command ports.Command) (ports.CommandResult, error) {
	r.commands = append(r.commands, command)
	code := 0
	if r.exit != nil {
		code = r.exit(command)
	}
	return ports.CommandResult{ExitCode: code, Stderr: []byte("SyntaxError"), TimedOut: r.timedOut}, nil
}

type fixedReader struct {
	outcomes map[ecosystem.TestID]ecosystem.TestOutcome
	patterns []string
	format   ecosystem.ReportFormat
}

func (f *fixedReader) Read(patterns []string, format ecosystem.ReportFormat) (map[ecosystem.TestID]ecosystem.TestOutcome, error) {
	f.patterns, f.format = patterns, format
	return f.outcomes, nil
}

func strategy(process ports.ProcessRunner, reader ports.ResultsReader, lookPath func(string) (string, error)) *pytest.Strategy {
	return pytest.New(pytest.Dependencies{Process: process, Reader: reader, Locator: treesitter.NewLocator(), LookPath: lookPath})
}

func found(name string) (string, error) { return "/usr/bin/" + name, nil }

func notFound(string) (string, error) { return "", errors.New("not found") }

func TestDetect(t *testing.T) {
	cases := []struct {
		name  string
		files fstest.MapFS
		want  bool
	}{
		{"pytest.ini", fstest.MapFS{"pytest.ini": {}}, true},
		{"conftest.py", fstest.MapFS{"conftest.py": {}}, true},
		{"tox.ini section", fstest.MapFS{"tox.ini": {Data: []byte("[pytest]\n")}}, true},
		{"setup.cfg section", fstest.MapFS{"setup.cfg": {Data: []byte("[tool:pytest]\n")}}, true},
		{"pyproject section", fstest.MapFS{"pyproject.toml": {Data: []byte("[tool.pytest.ini_options]\n")}}, true},
		{"unrelated python project", fstest.MapFS{"pyproject.toml": {Data: []byte("[project]\n")}, "app.py": {}}, false},
		{"empty tree", fstest.MapFS{}, false},
	}
	for _, c := range cases {
		t.Run(c.name+" [verifies SP-73~1]", func(t *testing.T) {
			detection, ok := strategy(nil, nil, found).Detect(c.files)
			if ok != c.want || (ok && (detection.Name != ecosystem.Pytest || detection.Root != "." || len(detection.Evidence) == 0)) {
				t.Fatalf("detection = %+v, %v", detection, ok)
			}
		})
	}
}

func TestToolchainResolution(t *testing.T) {
	ctx := context.Background()
	workspace := func(files ...string) ecosystem.Workspace {
		dir := t.TempDir()
		for _, file := range files {
			full := filepath.Join(dir, file)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, nil, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		return ecosystem.Workspace{Dir: dir}
	}
	resolve := func(w ecosystem.Workspace, deps pytest.Dependencies) (ecosystem.Toolchain, error) {
		return pytest.New(deps).Toolchain().Resolve(ctx, w)
	}

	t.Run("a configured interpreter wins [verifies SP-74~1]", func(t *testing.T) {
		got, err := resolve(workspace(".venv/bin/python"), pytest.Dependencies{Interpreter: []string{"conda", "run", "python"}, LookPath: found})
		if err != nil || !reflect.DeepEqual(got.Command, []string{"conda", "run", "python"}) {
			t.Fatalf("%v %v", got, err)
		}
	})

	t.Run("then the project's virtual environment [verifies SP-74~1]", func(t *testing.T) {
		w := workspace(".venv/bin/python", "uv.lock")
		got, err := resolve(w, pytest.Dependencies{LookPath: found})
		if err != nil || len(got.Command) != 1 || got.Command[0] != filepath.Join(w.Dir, ".venv/bin/python") {
			t.Fatalf("%v %v", got, err)
		}
	})

	t.Run("then uv run python for a uv project [verifies SP-74~1]", func(t *testing.T) {
		got, err := resolve(workspace("uv.lock"), pytest.Dependencies{LookPath: found})
		if err != nil || !reflect.DeepEqual(got.Command, []string{"uv", "run", "python"}) {
			t.Fatalf("%v %v", got, err)
		}
	})

	t.Run("a project without a lock file does not use uv [verifies SP-74~1]", func(t *testing.T) {
		got, err := resolve(workspace(), pytest.Dependencies{LookPath: found})
		if err != nil || !reflect.DeepEqual(got.Command, []string{"/usr/bin/python3"}) {
			t.Fatalf("%v %v", got, err)
		}
	})

	t.Run("python3 is the last resort and its absence is an error [verifies SP-74~1]", func(t *testing.T) {
		if _, err := resolve(workspace(), pytest.Dependencies{LookPath: notFound}); !errors.Is(err, pytest.ErrNoInterpreter) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestBuildCheckCommands(t *testing.T) {
	ctx := context.Background()
	workspace := ecosystem.Workspace{Dir: "/work/ref"}
	toolchain := ecosystem.Toolchain{Command: []string{"py"}}

	t.Run("changed files are compiled, then each module is imported [verifies SP-74~1]", func(t *testing.T) {
		process := &recorder{}
		result, err := strategy(process, nil, found).BuildCheck().Check(ctx, workspace, toolchain, []string{"parcel/rates.py", "src/shop/__init__.py", "README.md"})
		if err != nil || !result.OK || len(process.commands) != 3 {
			t.Fatalf("%+v %v commands=%v", result, err, process.commands)
		}
		if want := []string{"py", "-m", "py_compile", "parcel/rates.py", "src/shop/__init__.py"}; !reflect.DeepEqual(process.commands[0].Argv, want) {
			t.Fatalf("compile argv = %v", process.commands[0].Argv)
		}
		if got := process.commands[1].Argv; got[len(got)-1] != "parcel.rates" || got[1] != "-c" {
			t.Fatalf("import argv = %v", got)
		}
		if got := process.commands[2].Argv; got[len(got)-1] != "shop" {
			t.Fatalf("package import argv = %v", got)
		}
		env := strings.Join(process.commands[1].Env, " ")
		if !strings.Contains(env, "PYTHONDONTWRITEBYTECODE=1") || !strings.Contains(env, "PYTHONPATH=/work/ref") {
			t.Fatalf("env = %v", process.commands[1].Env)
		}
	})

	t.Run("a compile failure is a failed build with diagnostics and no import [verifies SP-74~1]", func(t *testing.T) {
		process := &recorder{exit: func(ports.Command) int { return 1 }}
		result, err := strategy(process, nil, found).BuildCheck().Check(ctx, workspace, toolchain, []string{"parcel/rates.py"})
		if err != nil || result.OK || !strings.Contains(result.Diagnostics, "SyntaxError") || len(process.commands) != 1 {
			t.Fatalf("%+v %v", result, err)
		}
	})

	t.Run("an import failure is a failed build [verifies SP-74~1]", func(t *testing.T) {
		process := &recorder{exit: func(c ports.Command) int {
			if c.Argv[1] == "-c" {
				return 1
			}
			return 0
		}}
		result, err := strategy(process, nil, found).BuildCheck().Check(ctx, workspace, toolchain, []string{"parcel/rates.py"})
		if err != nil || result.OK {
			t.Fatalf("%+v %v", result, err)
		}
	})

	t.Run("no Python file changed means nothing to check [verifies SP-74~1]", func(t *testing.T) {
		process := &recorder{}
		result, err := strategy(process, nil, found).BuildCheck().Check(ctx, workspace, toolchain, []string{"notes.md"})
		if err != nil || !result.OK || len(process.commands) != 0 {
			t.Fatalf("%+v %v", result, err)
		}
	})
}

func TestRunnerCommand(t *testing.T) {
	workspace := ecosystem.Workspace{Dir: "/work/ref", ResultsDir: "/work/results/ref"}
	toolchain := ecosystem.Toolchain{Command: []string{"py"}}
	selection := ecosystem.Selection{Args: []string{"tests/test_rates.py::test_a", "tests/test_rates.py::test_b[1]"}}

	t.Run("the pytest command is exactly the documented one [verifies SP-74~1]", func(t *testing.T) {
		process, reader := &recorder{}, &fixedReader{}
		if _, err := strategy(process, reader, found).Runner().Run(context.Background(), workspace, toolchain, selection); err != nil {
			t.Fatal(err)
		}
		argv := process.commands[0].Argv
		if len(argv) != 10 || !reflect.DeepEqual(argv[:7], []string{"py", "-m", "pytest", "-p", "no:cacheprovider", "-o", "junit_family=xunit1"}) ||
			!strings.HasPrefix(argv[7], "--junitxml="+workspace.ResultsDir) || !reflect.DeepEqual(argv[8:], selection.Args) {
			t.Fatalf("argv = %v", argv)
		}
		if !reflect.DeepEqual(process.commands[0].Env, []string{"PYTHONDONTWRITEBYTECODE=1"}) || process.commands[0].Dir != workspace.Dir {
			t.Fatalf("command = %+v", process.commands[0])
		}
		if reader.format != ecosystem.ReportPytest || len(reader.patterns) != 1 || !strings.HasSuffix(reader.patterns[0], ".xml") {
			t.Fatalf("reader got %v %v", reader.patterns, reader.format)
		}
	})

	t.Run("interrupted, internal and usage exits are collection failures [verifies SP-74~1]", func(t *testing.T) {
		for _, code := range []int{2, 3, 4} {
			process := &recorder{exit: func(ports.Command) int { return code }}
			got, err := strategy(process, &fixedReader{}, found).Runner().Run(context.Background(), workspace, toolchain, selection)
			if err != nil || !got.CollectionFailed {
				t.Fatalf("exit %d: %+v %v", code, got, err)
			}
		}
	})

	t.Run("failing tests are not a collection failure [verifies SP-74~1]", func(t *testing.T) {
		process := &recorder{exit: func(ports.Command) int { return 1 }}
		reader := &fixedReader{outcomes: map[ecosystem.TestID]ecosystem.TestOutcome{"x": ecosystem.TestFailed}}
		got, err := strategy(process, reader, found).Runner().Run(context.Background(), workspace, toolchain, selection)
		if err != nil || got.CollectionFailed || got.Outcomes["x"] != ecosystem.TestFailed {
			t.Fatalf("%+v %v", got, err)
		}
	})
}

func TestSelector(t *testing.T) {
	loaded, err := bindings.Load(ecosystem.Pytest)
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile("../../../../fixtures/ecosystems/pytest/tests/test_rates.py")
	if err != nil {
		t.Fatal(err)
	}
	locator := treesitter.NewLocator()
	definitions, err := locator.Locate("tests/test_rates.py", source)
	if err != nil {
		t.Fatal(err)
	}
	byFile := map[string][]ecosystem.Definition{"tests/test_rates.py": definitions}
	sel := strategy(nil, nil, found).Selector()

	t.Run("a plain test, a parametrized test and a class method map to node ids [verifies SP-74~1]", func(t *testing.T) {
		got, err := sel.Select(loaded.All(), byFile)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{
			"tests/test_rates.py::test_line_total_multiplies_price_by_quantity",
			"tests/test_rates.py::test_discount_reduces_total[0-19.99-19.99]",
			"tests/test_rates.py::test_discount_reduces_total[10-200-180.0]",
			"tests/test_rates.py::test_discount_reduces_total[50-80-40.0]",
			"tests/test_rates.py::TestDiscountLimits::test_negative_percent_is_ignored",
			"tests/test_rates.py::TestDiscountLimits::test_percent_above_one_hundred_is_clamped",
			"tests/test_rates.py::test_shipping_is_waived_from_fifty",
		}
		if len(got.Args) != len(want) || len(got.Tests) != len(want) {
			t.Fatalf("selection = %+v", got)
		}
		for _, node := range want {
			if !contains(got.Args, node) {
				t.Errorf("missing %s in %v", node, got.Args)
			}
		}
	})

	t.Run("a test whose classname is foreign is mapped through its source line [verifies SP-65~1]", func(t *testing.T) {
		line := definitionLine(definitions, "test_shipping_is_waived_from_fifty")
		bound := ecosystem.NewBoundTest("renamed.module", "display name", "tests/test_rates.py", line-1)
		got, err := sel.Select([]ecosystem.BoundTest{bound}, byFile)
		if err != nil || !reflect.DeepEqual(got.Args, []string{"tests/test_rates.py::test_shipping_is_waived_from_fifty"}) {
			t.Fatalf("%+v %v", got, err)
		}
	})

	t.Run("the same node is selected once [verifies SP-74~1]", func(t *testing.T) {
		bound := ecosystem.NewBoundTest("tests.test_rates", "test_shipping_is_waived_from_fifty", "tests/test_rates.py", 1)
		got, err := sel.Select([]ecosystem.BoundTest{bound, bound}, byFile)
		if err != nil || len(got.Args) != 1 || len(got.Tests) != 2 {
			t.Fatalf("%+v %v", got, err)
		}
	})

	t.Run("a bound test no definition explains is an error [verifies SP-74~1]", func(t *testing.T) {
		bound := ecosystem.NewBoundTest("other", "missing", "tests/absent.py", 3)
		if _, err := sel.Select([]ecosystem.BoundTest{bound}, byFile); !errors.Is(err, pytest.ErrUnmappedTest) {
			t.Fatalf("err = %v", err)
		}
	})
}

func contains(items []string, item string) bool {
	for _, candidate := range items {
		if candidate == item {
			return true
		}
	}
	return false
}

func definitionLine(definitions []ecosystem.Definition, qualname string) int {
	for _, d := range definitions {
		if d.Qualname == qualname {
			return d.StartLine
		}
	}
	return 0
}

func TestLayout(t *testing.T) {
	layout := strategy(nil, nil, found).Layout()
	t.Run("bytecode caches are excluded and test files recognized [verifies SP-68~1]", func(t *testing.T) {
		if !contains(layout.ExcludedDirs, "__pycache__") || !contains(layout.ExcludedDirs, ".pytest_cache") {
			t.Fatalf("excluded = %v", layout.ExcludedDirs)
		}
		if !layout.IsTestFile("tests/test_rates.py") || !layout.IsTestFile("conftest.py") || layout.IsTestFile("parcel/rates.py") {
			t.Fatal("test file recognition is wrong")
		}
	})
}
