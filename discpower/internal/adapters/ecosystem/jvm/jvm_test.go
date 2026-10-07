package jvm_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/ecosystem/jvm"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/sourceparse/treesitter"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/testkit/bindings"
)

const fixturesRoot = "../../../../fixtures/ecosystems"

type recorder struct {
	commands []ports.Command
	exit     int
	timedOut bool
}

func (r *recorder) Run(_ context.Context, command ports.Command) (ports.CommandResult, error) {
	r.commands = append(r.commands, command)
	return ports.CommandResult{ExitCode: r.exit, TimedOut: r.timedOut, Stdout: []byte("compilation failed")}, nil
}

type fixedReader struct {
	patterns []string
	format   ecosystem.ReportFormat
}

func (f *fixedReader) Read(patterns []string, format ecosystem.ReportFormat) (map[ecosystem.TestID]ecosystem.TestOutcome, error) {
	f.patterns, f.format = patterns, format
	return map[ecosystem.TestID]ecosystem.TestOutcome{"parcel.RatesTest::x": ecosystem.TestPassed}, nil
}

func found(name string) (string, error) { return "/opt/tools/bin/" + name, nil }

func missing(string) (string, error) { return "", errors.New("absent") }

func maven(process ports.ProcessRunner, reader ports.ResultsReader) *jvm.Strategy {
	return jvm.NewMaven(jvm.Dependencies{Process: process, Reader: reader, Locator: treesitter.NewLocator(), LookPath: found})
}

func gradle(process ports.ProcessRunner, reader ports.ResultsReader) *jvm.Strategy {
	return jvm.NewGradle(jvm.Dependencies{Process: process, Reader: reader, Locator: treesitter.NewLocator(), LookPath: found})
}

func TestDetectAndLayout(t *testing.T) {
	t.Run("a pom.xml marks a Maven project [verifies SP-1110~1]", func(t *testing.T) {
		detection, ok := maven(nil, nil).Detect(fstest.MapFS{"pom.xml": {}})
		if !ok || detection.Name != ecosystem.Maven || detection.Root != "." || !reflect.DeepEqual(detection.Evidence, []string{"pom.xml"}) {
			t.Fatalf("%+v %v", detection, ok)
		}
		if _, ok := maven(nil, nil).Detect(fstest.MapFS{"build.gradle": {}}); ok {
			t.Fatal("a Gradle tree was detected as Maven")
		}
	})

	t.Run("the build and settings scripts of Gradle, Groovy or Kotlin, mark a Gradle project [verifies SP-1110~1]", func(t *testing.T) {
		for _, file := range []string{"build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts"} {
			detection, ok := gradle(nil, nil).Detect(fstest.MapFS{file: {}})
			if !ok || detection.Name != ecosystem.Gradle || !reflect.DeepEqual(detection.Evidence, []string{file}) {
				t.Errorf("%s: %+v %v", file, detection, ok)
			}
		}
		if _, ok := gradle(nil, nil).Detect(fstest.MapFS{"pom.xml": {}}); ok {
			t.Fatal("a Maven tree was detected as Gradle")
		}
	})

	t.Run("test classes of both languages are test files and the Gradle caches are left out [verifies SP-1110~1]", func(t *testing.T) {
		layout := gradle(nil, nil).Layout()
		for _, file := range []string{"src/test/java/a/RatesTest.java", "src/test/kotlin/a/RatesTests.kt", "src/test/java/a/TestRates.java", "src/test/java/a/CartIT.java"} {
			if !layout.IsTestFile(file) {
				t.Errorf("%s is not a test file", file)
			}
		}
		if layout.IsTestFile("src/main/java/a/Rates.java") || layout.IsTestFile("src/main/kotlin/a/Rates.kt") {
			t.Error("a source file is a test file")
		}
		if !reflect.DeepEqual(layout.ExcludedDirs, []string{".gradle", ".kotlin"}) {
			t.Errorf("excluded directories = %v", layout.ExcludedDirs)
		}
		if excluded := maven(nil, nil).Layout().ExcludedDirs; len(excluded) != 0 {
			t.Errorf("Maven excludes %v", excluded)
		}
	})
}

func TestToolchain(t *testing.T) {
	resolve := func(t *testing.T, strategy *jvm.Strategy, dir string) (ecosystem.Toolchain, error) {
		t.Helper()
		return strategy.Toolchain().Resolve(context.Background(), ecosystem.Workspace{Dir: dir})
	}

	t.Run("an executable wrapper of the project wins over the tool on PATH [verifies SP-1111~1]", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "mvnw"), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		got, err := resolve(t, maven(nil, nil), dir)
		if err != nil || !reflect.DeepEqual(got.Command, []string{"./mvnw"}) {
			t.Fatalf("%v %v", got, err)
		}
	})

	t.Run("a wrapper that is not executable is not used [verifies SP-1111~1]", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "gradlew"), []byte("#!/bin/sh\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := resolve(t, gradle(nil, nil), dir)
		if err != nil || !reflect.DeepEqual(got.Command, []string{"/opt/tools/bin/gradle"}) {
			t.Fatalf("%v %v", got, err)
		}
	})

	t.Run("without a wrapper the tool is resolved from PATH and its absence is an error [verifies SP-1111~1]", func(t *testing.T) {
		got, err := resolve(t, maven(nil, nil), t.TempDir())
		if err != nil || !reflect.DeepEqual(got.Command, []string{"/opt/tools/bin/mvn"}) {
			t.Fatalf("%v %v", got, err)
		}
		absent := jvm.NewMaven(jvm.Dependencies{LookPath: missing})
		if _, err := resolve(t, absent, t.TempDir()); !errors.Is(err, jvm.ErrNoToolchain) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestBuildCheckCommand(t *testing.T) {
	workspace := ecosystem.Workspace{Dir: "/work/ref"}
	cases := []struct {
		name     string
		strategy func(ports.ProcessRunner) *jvm.Strategy
		tool     string
		want     []string
	}{
		{"maven", func(p ports.ProcessRunner) *jvm.Strategy { return maven(p, nil) }, "mvn", []string{"-o", "-q", "-B", "-T", "1", "test-compile"}},
		{"gradle", func(p ports.ProcessRunner) *jvm.Strategy { return gradle(p, nil) }, "gradle", []string{
			"--offline", "-q", "--no-daemon", "--console=plain", "-Pkotlin.compiler.execution.strategy=in-process", "testClasses",
		}},
	}
	for _, c := range cases {
		t.Run(c.name+" compiles main and test sources offline with one process [verifies SP-1112~1]", func(t *testing.T) {
			process := &recorder{}
			result, err := c.strategy(process).BuildCheck().Check(context.Background(), workspace, ecosystem.Toolchain{Command: []string{c.tool}}, []string{"src/main/java/a/Rates.java"})
			if err != nil || !result.OK {
				t.Fatalf("%+v %v", result, err)
			}
			if len(process.commands) != 1 || !reflect.DeepEqual(process.commands[0].Argv, append([]string{c.tool}, c.want...)) || process.commands[0].Dir != workspace.Dir {
				t.Fatalf("commands = %+v", process.commands)
			}
		})

		t.Run(c.name+" turns a failing build into diagnostics [verifies SP-1112~1]", func(t *testing.T) {
			result, err := c.strategy(&recorder{exit: 1}).BuildCheck().Check(context.Background(), workspace, ecosystem.Toolchain{Command: []string{c.tool}}, nil)
			if err != nil || result.OK || !strings.Contains(result.Diagnostics, "compilation failed") {
				t.Fatalf("%+v %v", result, err)
			}
			timedOut, err := c.strategy(&recorder{timedOut: true}).BuildCheck().Check(context.Background(), workspace, ecosystem.Toolchain{Command: []string{c.tool}}, nil)
			if err != nil || timedOut.OK {
				t.Fatalf("a timed out build passed: %+v %v", timedOut, err)
			}
		})
	}
}

func TestRunnerCommand(t *testing.T) {
	selection := ecosystem.Selection{Args: []string{"-Dtest=a.RatesTest#m"}, Tests: []ecosystem.TestID{"a.RatesTest::m"}}

	t.Run("maven runs Surefire directly, reads TEST-*.xml and clears the reports of an earlier run [verifies SP-1113~1]", func(t *testing.T) {
		dir := t.TempDir()
		stale := filepath.Join(dir, "target", "surefire-reports", "TEST-a.RatesTest.xml")
		if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(stale, []byte("<testsuite/>"), 0o644); err != nil {
			t.Fatal(err)
		}
		process, reader := &recorder{}, &fixedReader{}
		result, err := maven(process, reader).Runner().Run(context.Background(), ecosystem.Workspace{Dir: dir, ResultsDir: t.TempDir()}, ecosystem.Toolchain{Command: []string{"./mvnw"}}, selection)
		if err != nil || result.Outcomes["parcel.RatesTest::x"] != ecosystem.TestPassed {
			t.Fatalf("%+v %v", result, err)
		}
		want := []string{"./mvnw", "-o", "-q", "-B", "-T", "1", "surefire:test", "-Dsurefire.failIfNoSpecifiedTests=false", "-DfailIfNoTests=false", "-Dtest=a.RatesTest#m"}
		if !reflect.DeepEqual(process.commands[0].Argv, want) {
			t.Fatalf("argv = %v, want %v", process.commands[0].Argv, want)
		}
		if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("the report of the earlier run is still there: %v", err)
		}
		if reader.format != ecosystem.ReportSurefire || !reflect.DeepEqual(reader.patterns, []string{filepath.Join(dir, "target", "surefire-reports", "TEST-*.xml")}) {
			t.Fatalf("read %v as %s", reader.patterns, reader.format)
		}
	})

	t.Run("gradle cleans its test results so that an up-to-date task runs again [verifies SP-1113~1]", func(t *testing.T) {
		dir := t.TempDir()
		process, reader := &recorder{}, &fixedReader{}
		_, err := gradle(process, reader).Runner().Run(context.Background(), ecosystem.Workspace{Dir: dir, ResultsDir: t.TempDir()}, ecosystem.Toolchain{Command: []string{"./gradlew"}},
			ecosystem.Selection{Args: []string{"--tests", "a.RatesTest.m"}})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"./gradlew", "--offline", "-q", "--no-daemon", "--console=plain", "-Pkotlin.compiler.execution.strategy=in-process", "cleanTest", "test", "--tests", "a.RatesTest.m"}
		if !reflect.DeepEqual(process.commands[0].Argv, want) {
			t.Fatalf("argv = %v, want %v", process.commands[0].Argv, want)
		}
		if reader.format != ecosystem.ReportGradle || !reflect.DeepEqual(reader.patterns, []string{filepath.Join(dir, "build", "test-results", "test", "*.xml")}) {
			t.Fatalf("read %v as %s", reader.patterns, reader.format)
		}
	})

	t.Run("an empty selection runs the whole suite [verifies SP-1113~1]", func(t *testing.T) {
		process := &recorder{}
		if _, err := maven(process, &fixedReader{}).Runner().Run(context.Background(), ecosystem.Workspace{Dir: t.TempDir(), ResultsDir: t.TempDir()}, ecosystem.Toolchain{Command: []string{"mvn"}}, ecosystem.Selection{}); err != nil {
			t.Fatal(err)
		}
		for _, argument := range process.commands[0].Argv {
			if strings.HasPrefix(argument, "-Dtest=") {
				t.Fatalf("the whole suite was filtered: %v", process.commands[0].Argv)
			}
		}
	})
}

func loadBindings(t *testing.T, name ecosystem.Name) bindings.Bindings {
	t.Helper()
	loaded, err := bindings.Load(name)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

// definitionsOf locates the definitions of the files the bound tests name.
func definitionsOf(t *testing.T, fixture string, bound []ecosystem.BoundTest) map[string][]ecosystem.Definition {
	t.Helper()
	definitions := map[string][]ecosystem.Definition{}
	for _, test := range bound {
		if _, done := definitions[test.File]; done {
			continue
		}
		source, err := os.ReadFile(filepath.Join(fixturesRoot, fixture, filepath.FromSlash(test.File)))
		if err != nil {
			t.Fatal(err)
		}
		located, err := treesitter.NewLocator().Locate(test.File, source)
		if err != nil {
			t.Fatal(err)
		}
		definitions[test.File] = located
	}
	return definitions
}

func TestSelector(t *testing.T) {
	t.Run("maven names the methods of a class and the whole nested class its display name tags [verifies SP-1114~1]", func(t *testing.T) {
		bound := loadBindings(t, ecosystem.Maven).All()
		selection, err := maven(nil, nil).Selector().Select(bound, definitionsOf(t, "junit5-maven", bound))
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"-Dtest=com.example.parcel.RatesTest#multipliesUnitPriceByQuantity+takesPercentOff,com.example.parcel.RatesTest$Shipping"}
		if !reflect.DeepEqual(selection.Args, want) {
			t.Fatalf("args = %v, want %v", selection.Args, want)
		}
		if len(selection.Tests) != len(bound) {
			t.Fatalf("%d tests for %d bound", len(selection.Tests), len(bound))
		}
	})

	t.Run("gradle filters one method at a time, in Java and in Kotlin [verifies SP-1114~1]", func(t *testing.T) {
		for _, c := range []struct {
			fixture  string
			bindings ecosystem.Name
			want     []string
		}{
			{"junit5-gradle", "gradle", []string{
				"--tests", "parcel.RatesTest.multipliesUnitPriceByQuantity", "--tests", "parcel.RatesTest.takesPercentOff", "--tests", "parcel.RatesTest.isWaivedFromFifty",
			}},
			{"junit5-kotlin", "gradle-kotlin", []string{
				"--tests", "parcel.RatesTest.multipliesUnitPriceByQuantity", "--tests", "parcel.RatesTest.takesPercentOff", "--tests", "parcel.RatesTest.isWaivedFromFifty",
			}},
		} {
			bound := loadBindings(t, c.bindings).All()
			selection, err := gradle(nil, nil).Selector().Select(bound, definitionsOf(t, c.fixture, bound))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(selection.Args, c.want) {
				t.Errorf("%s: args = %v, want %v", c.fixture, selection.Args, c.want)
			}
		}
	})

	t.Run("a bound test no definition explains is refused [verifies SP-1114~1]", func(t *testing.T) {
		unmapped := []ecosystem.BoundTest{
			ecosystem.NewBoundTest("a.RatesTest", "m", "src/test/java/a/RatesTest.java", 5),
			ecosystem.NewBoundTest("a.RatesTest", "m", "tests/RatesTest.java", 5),
		}
		definitions := map[string][]ecosystem.Definition{
			"tests/RatesTest.java": {{Qualname: "RatesTest.m", Kind: ecosystem.KindMethod, StartLine: 5, EndLine: 9}},
		}
		for _, test := range unmapped {
			if _, err := maven(nil, nil).Selector().Select([]ecosystem.BoundTest{test}, definitions); !errors.Is(err, jvm.ErrUnmappedTest) {
				t.Errorf("%s: err = %v", test.File, err)
			}
		}
	})

	t.Run("no bound test selects nothing [verifies SP-1114~1]", func(t *testing.T) {
		selection, err := maven(nil, nil).Selector().Select(nil, nil)
		if err != nil || len(selection.Args) != 0 || len(selection.Tests) != 0 {
			t.Fatalf("%+v %v", selection, err)
		}
	})
}
