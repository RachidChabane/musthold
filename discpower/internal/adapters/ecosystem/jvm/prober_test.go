package jvm_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/ecosystem/jvm"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/junitxml"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/process"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/sourceparse/treesitter"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/workspace"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/differential"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/discpower/internal/testkit/jvmenv"
)

const (
	probedProject = "testdata/probed"
	probedPackage = "probed."
	mavenFixture  = fixturesRoot + "/junit5-maven"
	gradleFixture = fixturesRoot + "/junit5-gradle"
	kotlinFixture = fixturesRoot + "/junit5-kotlin"
	classpathGoal = "build-classpath"
)

// countingProcess runs real processes and counts the classpath resolutions.
type countingProcess struct {
	inner      ports.ProcessRunner
	classpaths atomic.Int32
}

func (c *countingProcess) Run(ctx context.Context, command ports.Command) (ports.CommandResult, error) {
	for _, argument := range command.Argv {
		if strings.Contains(argument, classpathGoal) || argument == "specProofClasspath" {
			c.classpaths.Add(1)
		}
	}
	return c.inner.Run(ctx, command)
}

// liveProject is a project built in a real workspace with the real build tool.
type liveProject struct {
	strategy  *jvm.Strategy
	manager   *workspace.Manager
	process   *countingProcess
	workspace ecosystem.Workspace
	toolchain ecosystem.Toolchain
}

func openProject(t *testing.T, directory string, newStrategy func(jvm.Dependencies) *jvm.Strategy) liveProject {
	t.Helper()
	manager, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	counting := &countingProcess{inner: process.New()}
	strategy := newStrategy(jvm.Dependencies{Process: counting, Reader: junitxml.NewReader(), Locator: treesitter.NewLocator()})
	ctx := context.Background()
	prepared, err := manager.Prepare(ctx, directory, strategy.Layout())
	if err != nil {
		t.Fatal(err)
	}
	toolchain, err := strategy.Toolchain().Resolve(ctx, prepared)
	if err != nil {
		t.Fatal(err)
	}
	if build, err := strategy.BuildCheck().Check(ctx, prepared, toolchain, nil); err != nil || !build.OK {
		t.Fatalf("build the project: %+v %v", build, err)
	}
	return liveProject{strategy: strategy, manager: manager, process: counting, workspace: prepared, toolchain: toolchain}
}

func (p liveProject) invoke(t *testing.T, calls ...ports.ProbeCall) []spec.Observation {
	t.Helper()
	got, err := p.strategy.Prober().Invoke(context.Background(), p.workspace, p.toolchain, calls)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(calls) {
		t.Fatalf("%d observations for %d calls", len(got), len(calls))
	}
	return got
}

func call(target string, args map[string]spec.Literal) ports.ProbeCall {
	return ports.ProbeCall{Target: target, Args: args}
}

func literal(kind spec.LiteralType, value string) spec.Literal {
	return spec.Literal{Type: kind, Value: value}
}

func returns(kind spec.LiteralType, value string) spec.Observation {
	return spec.Observation{Kind: spec.Returns, Type: kind, Value: value}
}

func raises(name string) spec.Observation { return spec.Observation{Kind: spec.Raises, Value: name} }

func requireObservations(t *testing.T, got []spec.Observation, want ...spec.Observation) {
	t.Helper()
	for index := range want {
		if !got[index].Same(want[index]) {
			t.Errorf("observation %d = %+v, want %+v", index, got[index], want[index])
		}
	}
}

func TestSignatures(t *testing.T) {
	prober := func(strategy *jvm.Strategy) ports.Prober { return strategy.Prober() }
	signatures := func(t *testing.T, strategy *jvm.Strategy, fixture string, targets ...string) []ports.ProbeSignature {
		t.Helper()
		got, err := prober(strategy).Signatures(context.Background(), ecosystem.Workspace{Dir: fixture}, ecosystem.Toolchain{}, targets)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	t.Run("the parameters of a Java method are named as the spec writes them and typed from its source [verifies SP-1115~1]", func(t *testing.T) {
		got := signatures(t, maven(nil, nil), mavenFixture, "com.example.parcel.Rates:lineTotal", "com.example.parcel.Rates:shippingFee")
		want := []ports.ProbeSignature{
			{Target: "com.example.parcel.Rates:lineTotal", Params: []differential.Param{{Name: "unit_price", Hint: differential.HintDecimal}, {Name: "quantity", Hint: differential.HintInteger}}},
			{Target: "com.example.parcel.Rates:shippingFee", Params: []differential.Param{{Name: "total", Hint: differential.HintDecimal}}},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("signatures = %+v, want %+v", got, want)
		}
	})

	t.Run("the top-level functions of a Kotlin file are the methods of its file class [verifies SP-1115~1]", func(t *testing.T) {
		got := signatures(t, gradle(nil, nil), kotlinFixture, "parcel.RatesKt:discounted")
		want := []ports.ProbeSignature{{Target: "parcel.RatesKt:discounted", Params: []differential.Param{{Name: "total", Hint: differential.HintInteger}, {Name: "percent", Hint: differential.HintInteger}}}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("signatures = %+v, want %+v", got, want)
		}
	})

	refusals := []struct {
		name   string
		target string
	}{
		{"a target without a method", "com.example.parcel.Rates"},
		{"a target with an empty method", "com.example.parcel.Rates:"},
		{"a class the project does not declare", "com.example.parcel.Missing:lineTotal"},
		{"a method the class does not declare", "com.example.parcel.Rates:absent"},
		{"an overloaded method", probedPackage + "Operations:overloaded"},
	}
	for _, c := range refusals {
		t.Run(c.name+" is refused [verifies SP-1115~1]", func(t *testing.T) {
			dir := mavenFixture
			if strings.HasPrefix(c.target, probedPackage) {
				dir = probedProject
			}
			_, err := maven(nil, nil).Prober().Signatures(context.Background(), ecosystem.Workspace{Dir: dir}, ecosystem.Toolchain{}, []string{c.target})
			if !errors.Is(err, jvm.ErrInvalidTarget) {
				t.Fatalf("err = %v, want ErrInvalidTarget", err)
			}
		})
	}
}

func TestInvoke(t *testing.T) {
	jvmenv.Require(t)
	jvmenv.FetchMaven(t, probedProject)
	project := openProject(t, probedProject, jvm.NewMaven)

	t.Run("typed literals reach the method and what it returns comes back typed [verifies SP-1116~1]", func(t *testing.T) {
		got := project.invoke(t,
			call(probedPackage+"Operations:divide", map[string]spec.Literal{"dividend": literal(spec.TypeInteger, "7"), "divisor": literal(spec.TypeInteger, "2")}),
			call(probedPackage+"Operations:divide", map[string]spec.Literal{"Dividend": literal(spec.TypeInteger, "9"), "divisor": literal(spec.TypeInteger, "3")}),
			call(probedPackage+"Operations:scaled", map[string]spec.Literal{"amount": literal(spec.TypeDecimal, "2.345"), "digits": literal(spec.TypeInteger, "2")}),
			call(probedPackage+"Operations:half", map[string]spec.Literal{"value": literal(spec.TypeInteger, "5")}),
			call(probedPackage+"Operations:isPositive", map[string]spec.Literal{"value": literal(spec.TypeInteger, "-3")}),
			call(probedPackage+"Operations:echo", map[string]spec.Literal{"text": literal(spec.TypeString, "tab\there\nnew line")}),
			call(probedPackage+"Operations:keep", map[string]spec.Literal{"value": literal(spec.TypeNull, "null")}),
		)
		requireObservations(t, got,
			returns(spec.TypeInteger, "3"),
			returns(spec.TypeInteger, "3"),
			returns(spec.TypeDecimal, "2.35"),
			returns(spec.TypeDecimal, "2.5"),
			returns(spec.TypeBoolean, "false"),
			returns(spec.TypeString, "tab\there\nnew line"),
			returns(spec.TypeNull, "null"),
		)
	})

	t.Run("ISO-8601 literals become java.time values and java.time results come back as ISO-8601 [verifies SP-1116~1]", func(t *testing.T) {
		got := project.invoke(t,
			call(probedPackage+"Operations:weekday", map[string]spec.Literal{"day": literal(spec.TypeDatetime, "2026-10-07")}),
			call(probedPackage+"Operations:shift", map[string]spec.Literal{"start": literal(spec.TypeDatetime, "2026-10-07T23:30:00"), "hours": literal(spec.TypeInteger, "2")}),
			call(probedPackage+"Operations:epoch", map[string]spec.Literal{"moment": literal(spec.TypeDatetime, "2026-10-07T12:00:00+02:00")}),
		)
		requireObservations(t, got,
			returns(spec.TypeString, "WEDNESDAY"),
			returns(spec.TypeDatetime, "2026-10-08T01:30:00"),
			returns(spec.TypeDatetime, "2026-10-07T10:00:00Z"),
		)
	})

	t.Run("an exception the method throws is observed by the name of its class [verifies SP-1116~1]", func(t *testing.T) {
		got := project.invoke(t,
			call(probedPackage+"Operations:divide", map[string]spec.Literal{"dividend": literal(spec.TypeInteger, "1"), "divisor": literal(spec.TypeInteger, "0")}),
			call(probedPackage+"Operations:fail", nil),
		)
		requireObservations(t, got, raises("ArithmeticException"), raises("IllegalStateException"))
	})

	t.Run("an instance method runs on a fresh instance built with the default constructor [verifies SP-1116~1]", func(t *testing.T) {
		got := project.invoke(t, call(probedPackage+"Counter:next", nil), call(probedPackage+"Counter:next", nil))
		requireObservations(t, got, returns(spec.TypeInteger, "1"), returns(spec.TypeInteger, "1"))
	})

	t.Run("a non-public method is called as well [verifies SP-1116~1]", func(t *testing.T) {
		got := project.invoke(t, call(probedPackage+"Operations:hidden", map[string]spec.Literal{"value": literal(spec.TypeInteger, "1")}))
		requireObservations(t, got, returns(spec.TypeInteger, "2"))
	})

	t.Run("arguments that do not fit the method raise an argument error [verifies SP-1117~1]", func(t *testing.T) {
		got := project.invoke(t,
			call(probedPackage+"Operations:divide", map[string]spec.Literal{"dividend": literal(spec.TypeDecimal, "2.5"), "divisor": literal(spec.TypeInteger, "1")}),
			call(probedPackage+"Operations:divide", map[string]spec.Literal{"dividend": literal(spec.TypeInteger, "2")}),
			call(probedPackage+"Operations:divide", map[string]spec.Literal{"dividend": literal(spec.TypeInteger, "2"), "divisor": literal(spec.TypeInteger, "1"), "extra": literal(spec.TypeInteger, "1")}),
			call(probedPackage+"Operations:divide", map[string]spec.Literal{"dividend": literal(spec.TypeNull, "null"), "divisor": literal(spec.TypeInteger, "1")}),
		)
		requireObservations(t, got, raises("ArgumentError"), raises("ArgumentError"), raises("ArgumentError"), raises("ArgumentError"))
	})

	t.Run("a call that kills the JVM or never ends is an observation and the calls after it run in a fresh JVM [verifies SP-1117~1]", func(t *testing.T) {
		got := project.invoke(t,
			call(probedPackage+"Operations:echo", map[string]spec.Literal{"text": literal(spec.TypeString, "before")}),
			call(probedPackage+"Operations:crash", nil),
			call(probedPackage+"Operations:echo", map[string]spec.Literal{"text": literal(spec.TypeString, "after the crash")}),
			call(probedPackage+"Operations:spin", nil),
			call(probedPackage+"Operations:echo", map[string]spec.Literal{"text": literal(spec.TypeString, "after the timeout")}),
		)
		requireObservations(t, got,
			returns(spec.TypeString, "before"),
			raises("Crash"),
			returns(spec.TypeString, "after the crash"),
			raises("Timeout"),
			returns(spec.TypeString, "after the timeout"),
		)
	})

	t.Run("the classpath of a project is resolved once for every call and every workspace cloned from it [verifies SP-1118~1]", func(t *testing.T) {
		clone, err := project.manager.Clone(context.Background(), project.workspace, project.strategy.Layout())
		if err != nil {
			t.Fatal(err)
		}
		project.invoke(t, call(probedPackage+"Operations:fail", nil))
		cloned := liveProject{strategy: project.strategy, manager: project.manager, process: project.process, workspace: clone, toolchain: project.toolchain}
		requireObservations(t, cloned.invoke(t, call(probedPackage+"Operations:fail", nil)), raises("IllegalStateException"))
		if resolved := project.process.classpaths.Load(); resolved != 1 {
			t.Fatalf("the classpath was resolved %d times", resolved)
		}
	})

	t.Run("a target the project does not declare is refused [verifies SP-1115~1]", func(t *testing.T) {
		_, err := project.strategy.Prober().Invoke(context.Background(), project.workspace, project.toolchain, []ports.ProbeCall{call(probedPackage+"Operations:absent", nil)})
		if !errors.Is(err, jvm.ErrInvalidTarget) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestInvokeFixtures(t *testing.T) {
	jvmenv.Require(t)

	t.Run("a Maven project with decimal money answers decimals [verifies SP-1116~1]", func(t *testing.T) {
		jvmenv.FetchMaven(t, mavenFixture)
		project := openProject(t, mavenFixture, jvm.NewMaven)
		got := project.invoke(t,
			call("com.example.parcel.Rates:lineTotal", map[string]spec.Literal{"unit_price": literal(spec.TypeDecimal, "4.50"), "quantity": literal(spec.TypeInteger, "3")}),
			call("com.example.parcel.Rates:discounted", map[string]spec.Literal{"total": literal(spec.TypeInteger, "200"), "percent": literal(spec.TypeInteger, "150")}),
		)
		requireObservations(t, got, returns(spec.TypeDecimal, "13.50"), returns(spec.TypeDecimal, "0.00"))
	})

	t.Run("a Gradle project in Java and one in Kotlin answer through their own classpath [verifies SP-1116~1]", func(t *testing.T) {
		for _, c := range []struct{ fixture, target string }{
			{gradleFixture, "parcel.Rates:discounted"},
			{kotlinFixture, "parcel.RatesKt:discounted"},
		} {
			jvmenv.FetchGradle(t, c.fixture)
			project := openProject(t, c.fixture, jvm.NewGradle)
			got := project.invoke(t, call(c.target, map[string]spec.Literal{"total": literal(spec.TypeInteger, "20000"), "percent": literal(spec.TypeInteger, "10")}))
			requireObservations(t, got, returns(spec.TypeInteger, "18000"))
		}
	})
}
