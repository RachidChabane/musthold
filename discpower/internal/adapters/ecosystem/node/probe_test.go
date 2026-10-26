package node_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/ecosystem/node"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/process"
	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/differential"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
	"github.com/RachidChabane/musthold/discpower/internal/testkit/nodeinstall"
)

const (
	kitRoot       = "testdata/probekit"
	jestFixture   = "../../../../fixtures/ecosystems/jest"
	vitestFixture = "../../../../fixtures/ecosystems/vitest"
	nodeModules   = "node_modules"
)

// probeWorkspace copies a probe kit into a temporary workspace; modules, when
// not empty, is the node_modules directory the workspace links to.
func probeWorkspace(t *testing.T, kit, modules string) ecosystem.Workspace {
	t.Helper()
	nodeinstall.RequireNode(t)
	dir := t.TempDir()
	source := filepath.Join(kitRoot, kit)
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		relative, _ := filepath.Rel(source, path)
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		target := filepath.Join(dir, relative)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, content, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	if modules != "" {
		if err := os.Symlink(modules, filepath.Join(dir, nodeModules)); err != nil {
			t.Fatal(err)
		}
	}
	return ecosystem.Workspace{Dir: dir, ResultsDir: t.TempDir()}
}

func prober() ports.Prober {
	return node.NewJest(node.Dependencies{Process: process.New()}).Prober()
}

func integer(text string) spec.Literal { return spec.Literal{Type: spec.TypeInteger, Value: text} }
func decimal(text string) spec.Literal { return spec.Literal{Type: spec.TypeDecimal, Value: text} }
func text(value string) spec.Literal   { return spec.Literal{Type: spec.TypeString, Value: value} }
func null() spec.Literal               { return spec.Literal{Type: spec.TypeNull, Value: "null"} }

func call(target string, args map[string]spec.Literal) ports.ProbeCall {
	return ports.ProbeCall{Target: target, Args: args}
}

func args(pairs ...any) map[string]spec.Literal {
	named := map[string]spec.Literal{}
	for index := 0; index < len(pairs); index += 2 {
		named[pairs[index].(string)] = pairs[index+1].(spec.Literal)
	}
	return named
}

func invoke(t *testing.T, workspace ecosystem.Workspace, calls ...ports.ProbeCall) []spec.Observation {
	t.Helper()
	observed, err := prober().Invoke(context.Background(), workspace, ecosystem.Toolchain{}, calls)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if len(observed) != len(calls) {
		t.Fatalf("%d observations for %d calls: %v", len(observed), len(calls), observed)
	}
	return observed
}

func returns(kind spec.LiteralType, value string) spec.Observation {
	return spec.Observation{Kind: spec.Returns, Type: kind, Value: value}
}

func raises(name string) spec.Observation { return spec.Observation{Kind: spec.Raises, Value: name} }

func requireObservations(t *testing.T, got, want []spec.Observation) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("observations\n got  %v\n want %v", got, want)
	}
}

func signaturesOf(t *testing.T, workspace ecosystem.Workspace, targets ...string) map[string][]differential.Param {
	t.Helper()
	signatures, err := prober().Signatures(context.Background(), workspace, ecosystem.Toolchain{}, targets)
	if err != nil {
		t.Fatalf("Signatures: %v", err)
	}
	byTarget := map[string][]differential.Param{}
	for _, signature := range signatures {
		byTarget[signature.Target] = signature.Params
	}
	return byTarget
}

func param(name string, hint differential.Hint) differential.Param {
	return differential.Param{Name: name, Hint: hint}
}

func TestSignaturesWithoutACompiler(t *testing.T) {
	t.Run("names are snake case and hints come from the defaults of the parameters [verifies SP-1020~1]", func(t *testing.T) {
		workspace := probeWorkspace(t, "esm", "")
		got := signaturesOf(t, workspace, "ops.js:lineTotal", "ops.js:describeOrder", "ops.js:scaled")
		want := map[string][]differential.Param{
			"ops.js:lineTotal":     {param("unit_price", differential.HintUnknown), param("quantity", differential.HintUnknown)},
			"ops.js:describeOrder": {param("count", differential.HintInteger), param("label", differential.HintString), param("ratio", differential.HintDecimal), param("urgent", differential.HintBoolean)},
			"ops.js:scaled":        {param("amount", differential.HintUnknown), param("factor", differential.HintInteger)},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("signatures\n got  %v\n want %v", got, want)
		}
	})

	t.Run("a method is addressed by its class and a variadic parameter is left out [verifies SP-1020~1]", func(t *testing.T) {
		workspace := probeWorkspace(t, "esm", "")
		got := signaturesOf(t, workspace, "ops.js:Meter.bump", "ops.js:Meter.from")
		if !reflect.DeepEqual(got["ops.js:Meter.bump"], []differential.Param{param("by", differential.HintUnknown)}) ||
			!reflect.DeepEqual(got["ops.js:Meter.from"], []differential.Param{param("start", differential.HintUnknown)}) {
			t.Fatalf("signatures = %v", got)
		}
	})

	t.Run("a target that names nothing callable is an error [verifies SP-1020~1]", func(t *testing.T) {
		workspace := probeWorkspace(t, "esm", "")
		for _, target := range []string{"ops.js:absent", "ops.js:LIMIT", "ops.js", "missing.js:lineTotal"} {
			if _, err := prober().Signatures(context.Background(), workspace, ecosystem.Toolchain{}, []string{target}); !errors.Is(err, node.ErrProbeFailed) {
				t.Errorf("%s: err = %v", target, err)
			}
		}
	})
}

func TestSignaturesFromTheTypes(t *testing.T) {
	t.Run("hints come from the type annotations of the source, the project's compiler reading them [verifies SP-1020~1]", func(t *testing.T) {
		workspace := probeWorkspace(t, "typed", nodeinstall.Installed(t, jestFixture)+"/node_modules")
		got := signaturesOf(t, workspace, "ops.ts:signature", "ops.ts:Account.fee", "ops.ts:total")
		want := map[string][]differential.Param{
			"ops.ts:signature": {
				param("percent", differential.HintDecimal), param("name", differential.HintString), param("active", differential.HintBoolean),
				param("when", differential.HintDatetime), param("count", differential.HintInteger), param("maybe", differential.HintDecimal),
				param("later", differential.HintString), param("label", differential.HintString), param("ratio", differential.HintDecimal),
				param("retries", differential.HintInteger),
			},
			"ops.ts:Account.fee": {param("amount", differential.HintDecimal)},
			"ops.ts:total":       {param("amount", differential.HintDecimal), param("tax", differential.HintDecimal)},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("signatures\n got  %v\n want %v", got, want)
		}
	})
}

func TestInvokeReturnsTypedValues(t *testing.T) {
	observe := func(t *testing.T, calls ...ports.ProbeCall) []spec.Observation {
		return invoke(t, probeWorkspace(t, "esm", ""), calls...)
	}

	t.Run("integers, decimals, strings, booleans, null and times come back as typed literals [verifies SP-1021~1]", func(t *testing.T) {
		got := observe(t,
			call("ops.js:lineTotal", args("unit_price", integer("4"), "quantity", integer("3"))),
			call("ops.js:exactSum", args("first", decimal("0.1"), "second", decimal("0.2"))),
			call("ops.js:describeOrder", args("count", integer("2"), "label", text("box"), "ratio", decimal("0.25"), "urgent", spec.Literal{Type: spec.TypeBoolean, Value: "false"})),
			call("ops.js:typeOf", args("value", spec.Literal{Type: spec.TypeBoolean, Value: "true"})),
			call("ops.js:nothing", nil),
			call("ops.js:undecided", nil),
			call("ops.js:wide", args("count", integer("21"))),
			call("ops.js:asDate", args("iso", text("2026-03-01T10:00:00Z"))),
			call("ops.js:nowAtNoon", nil),
		)
		requireObservations(t, got, []spec.Observation{
			returns(spec.TypeInteger, "12"),
			returns(spec.TypeDecimal, "0.30000000000000004"),
			returns(spec.TypeString, "box:2:0.25:false:0"),
			returns(spec.TypeString, "boolean"),
			returns(spec.TypeNull, "null"),
			returns(spec.TypeNull, "null"),
			returns(spec.TypeInteger, "42"),
			returns(spec.TypeDatetime, "2026-03-01T10:00:00+00:00"),
			returns(spec.TypeDatetime, "2026-03-01T12:00:00.250+00:00"),
		})
	})

	t.Run("a number never comes back in exponent notation [verifies SP-1021~1]", func(t *testing.T) {
		got := observe(t, call("ops.js:tiny", nil), call("ops.js:huge", nil))
		requireObservations(t, got, []spec.Observation{returns(spec.TypeDecimal, "0.0000001"), returns(spec.TypeInteger, "1000000000000000000000")})
	})

	t.Run("an object comes back as canonical JSON, keys sorted, undefined left out [verifies SP-1021~1]", func(t *testing.T) {
		got := observe(t, call("ops.js:asObject", args("first", integer("1"), "second", integer("2"))))
		requireObservations(t, got, []spec.Observation{returns(spec.TypeString, `{"first":1,"nested":{"y":[1,{"a":1,"b":2}],"z":1},"second":2,"tags":["x"]}`)})
	})

	t.Run("a promise is awaited [verifies SP-1021~1]", func(t *testing.T) {
		got := observe(t, call("ops.js:asyncDouble", args("count", integer("21"))))
		requireObservations(t, got, []spec.Observation{returns(spec.TypeInteger, "42")})
	})

	t.Run("the observations come in the order of the calls [verifies SP-1021~1]", func(t *testing.T) {
		first := call("ops.js:lineTotal", args("unit_price", integer("2"), "quantity", integer("2")))
		second := call("ops.js:lineTotal", args("unit_price", integer("3"), "quantity", integer("3")))
		requireObservations(t, observe(t, second, first, second), []spec.Observation{returns(spec.TypeInteger, "9"), returns(spec.TypeInteger, "4"), returns(spec.TypeInteger, "9")})
	})
}

func TestInvokeNamesWhatWasThrown(t *testing.T) {
	t.Run("a call raises the class name of the error it throws, a promise rejection and a timer included [verifies SP-1022~1]", func(t *testing.T) {
		got := invoke(t, probeWorkspace(t, "esm", ""),
			call("ops.js:failsWithTypeError", nil),
			call("ops.js:failsWithCustomError", nil),
			call("ops.js:failsWithString", nil),
			call("ops.js:failsLater", nil),
			call("ops.js:failsFromTimer", nil),
			call("ops.js:exits", nil),
			call("ops.js:lineTotal", args("unit_price", integer("1"), "quantity", integer("1"))),
		)
		requireObservations(t, got, []spec.Observation{
			raises("TypeError"), raises("ValidationError"), raises("String"), raises("RangeError"), raises("SyntaxError"), raises("ProcessExit"),
			returns(spec.TypeInteger, "1"),
		})
	})

	t.Run("a target the module does not export is observed as an unknown target [verifies SP-1022~1]", func(t *testing.T) {
		got := invoke(t, probeWorkspace(t, "esm", ""), call("ops.js:absent", nil), call("ops.js:LIMIT", nil), call("ops.js:Meter.nothing", nil))
		requireObservations(t, got, []spec.Observation{raises("UnknownTarget"), raises("UnknownTarget"), raises("UnknownTarget")})
	})
}

func TestInvokeDecodesArguments(t *testing.T) {
	typedProject := func(t *testing.T) ecosystem.Workspace {
		return probeWorkspace(t, "typed", nodeinstall.Installed(t, jestFixture)+"/node_modules")
	}
	plainProject := func(t *testing.T) ecosystem.Workspace { return probeWorkspace(t, "esm", "") }

	t.Run("an argument is found under any spelling of the name of its parameter [verifies SP-1023~1]", func(t *testing.T) {
		plain := plainProject(t)
		for _, spelling := range [][2]string{{"unit_price", "quantity"}, {"unitPrice", "Quantity"}, {"UNIT_PRICE", "quantity"}} {
			got := invoke(t, plain, call("ops.js:lineTotal", args(spelling[0], integer("4"), spelling[1], integer("3"))))
			requireObservations(t, got, []spec.Observation{returns(spec.TypeInteger, "12")})
		}
	})

	t.Run("an argument that is missing, given twice or not expected is an ArgumentError without running the code [verifies SP-1023~1]", func(t *testing.T) {
		got := invoke(t, plainProject(t),
			call("ops.js:lineTotal", args("unit_price", integer("4"))),
			call("ops.js:lineTotal", args("unit_price", integer("4"), "unitPrice", integer("5"), "quantity", integer("3"))),
			call("ops.js:lineTotal", args("unit_price", integer("4"), "quantity", integer("3"), "surplus", integer("1"))),
		)
		requireObservations(t, got, []spec.Observation{raises("ArgumentError"), raises("ArgumentError"), raises("ArgumentError")})
	})

	t.Run("an optional parameter may be left out [verifies SP-1023~1]", func(t *testing.T) {
		got := invoke(t, plainProject(t), call("ops.js:scaled", args("amount", integer("4"))))
		requireObservations(t, got, []spec.Observation{returns(spec.TypeInteger, "8")})
	})

	t.Run("a literal is decoded into the type its parameter declares [verifies SP-1023~1]", func(t *testing.T) {
		got := invoke(t, typedProject(t),
			call("ops.ts:shifted", args("when", spec.Literal{Type: spec.TypeDatetime, Value: "2026-03-01T10:00:00Z"}, "days", integer("2"))),
			call("ops.ts:shifted", args("when", text("2026-03-01"), "days", integer("1"))),
			call("ops.ts:shifted", args("when", text("not a date"), "days", integer("1"))),
			call("ops.ts:total", args("amount", integer("5"), "tax", null())),
			call("ops.ts:total", args("amount", integer("5"), "tax", decimal("0.5"))),
		)
		requireObservations(t, got, []spec.Observation{
			returns(spec.TypeDatetime, "2026-03-03T10:00:00+00:00"),
			returns(spec.TypeDatetime, "2026-03-02T00:00:00+00:00"),
			raises("ArgumentError"),
			returns(spec.TypeInteger, "5"),
			returns(spec.TypeDecimal, "5.5"),
		})
	})
}

func TestInvokeLoadsTheModuleOfTheProject(t *testing.T) {
	t.Run("an ES module and its class, static and instance methods are called [verifies SP-1024~1]", func(t *testing.T) {
		workspace := probeWorkspace(t, "esm", "")
		got := invoke(t, workspace,
			call("ops.js:Meter.from", args("start", integer("5"))),
			call("ops.js:Meter.bump", args("by", integer("2"))),
			call("ops.js:Meter.bump", args("by", integer("2"))),
		)
		requireObservations(t, got, []spec.Observation{returns(spec.TypeInteger, "105"), returns(spec.TypeInteger, "2"), returns(spec.TypeInteger, "2")})
	})

	t.Run("a CommonJS module is called, its instance made afresh for each call [verifies SP-1024~1]", func(t *testing.T) {
		workspace := probeWorkspace(t, "cjs", "")
		got := invoke(t, workspace,
			call("ops.js:halve", args("amount", integer("9"))),
			call("ops.js:Ledger.record", args("amount", integer("1"))),
			call("ops.js:Ledger.record", args("amount", integer("1"))),
			call("ops.js:refuses", nil),
			call("ops.js:answer", nil),
		)
		requireObservations(t, got, []spec.Observation{returns(spec.TypeDecimal, "4.5"), returns(spec.TypeInteger, "1"), returns(spec.TypeInteger, "1"), raises("RangeError"), raises("UnknownTarget")})
	})

	typedCalls := []ports.ProbeCall{
		call("ops.ts:Account.fee", args("amount", integer("150"))),
		call("ops.ts:tierOf", args("name", text("premium"))),
		call("ops.ts:total", args("amount", integer("1"), "tax", integer("2"))),
	}
	typedWant := []spec.Observation{returns(spec.TypeInteger, "100"), returns(spec.TypeString, "premium"), returns(spec.TypeInteger, "3")}

	t.Run("a TypeScript module is loaded with the tsx of the project, enums and parameter properties included [verifies SP-1024~1]", func(t *testing.T) {
		workspace := probeWorkspace(t, "typed", nodeinstall.Installed(t, vitestFixture)+"/node_modules")
		requireObservations(t, invoke(t, workspace, typedCalls...), typedWant)
	})

	t.Run("without tsx or ts-node the TypeScript of the project transpiles it, relative .js imports resolved to .ts [verifies SP-1024~1]", func(t *testing.T) {
		workspace := probeWorkspace(t, "typed", nodeinstall.Installed(t, jestFixture)+"/node_modules")
		requireObservations(t, invoke(t, workspace, typedCalls...), typedWant)
	})

	t.Run("a TypeScript module is loaded with the ts-node of the project when it has no tsx [verifies SP-1024~1]", func(t *testing.T) {
		workspace := probeWorkspace(t, "tsnode", "")
		got := invoke(t, workspace, call("ops.ts:lineTotal", nil))
		requireObservations(t, got, []spec.Observation{returns(spec.TypeString, "compiled by ts-node")})
	})

	t.Run("a TypeScript module with no TypeScript tooling installed is an error that says so [verifies SP-1024~1]", func(t *testing.T) {
		workspace := probeWorkspace(t, "typed", "")
		_, err := prober().Invoke(context.Background(), workspace, ecosystem.Toolchain{}, typedCalls)
		if !errors.Is(err, node.ErrProbeFailed) || !strings.Contains(err.Error(), "no TypeScript loader") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("a module that does not load is an error that carries the message [verifies SP-1024~1]", func(t *testing.T) {
		workspace := probeWorkspace(t, "esm", "")
		if err := os.WriteFile(filepath.Join(workspace.Dir, "broken.js"), []byte("export const = ;\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := prober().Invoke(context.Background(), workspace, ecosystem.Toolchain{}, []ports.ProbeCall{call("broken.js:anything", nil)})
		if !errors.Is(err, node.ErrProbeFailed) || !strings.Contains(err.Error(), "SyntaxError") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("the workspace is left as it was [verifies SP-1024~1]", func(t *testing.T) {
		workspace := probeWorkspace(t, "esm", "")
		before := listing(t, workspace.Dir)
		invoke(t, workspace, call("ops.js:nothing", nil))
		if after := listing(t, workspace.Dir); !reflect.DeepEqual(before, after) {
			t.Fatalf("the workspace changed: %v -> %v", before, after)
		}
	})
}

func listing(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestInvokeSurvivesCallsThatDoNotEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for two call deadlines")
	}
	one := call("ops.js:lineTotal", args("unit_price", integer("1"), "quantity", integer("1")))
	two := call("ops.js:lineTotal", args("unit_price", integer("2"), "quantity", integer("1")))

	t.Run("a call that never ends, synchronously or not, is a timeout and the calls after it run in a new process [verifies SP-1025~1]", func(t *testing.T) {
		got := invoke(t, probeWorkspace(t, "esm", ""), one, call("ops.js:spins", nil), two, call("ops.js:hangs", nil), one)
		requireObservations(t, got, []spec.Observation{
			returns(spec.TypeInteger, "1"), raises("Timeout"), returns(spec.TypeInteger, "2"), raises("Timeout"), returns(spec.TypeInteger, "1"),
		})
	})

	t.Run("a call that ends the process is a crash and the calls after it run in a new process [verifies SP-1025~1]", func(t *testing.T) {
		got := invoke(t, probeWorkspace(t, "esm", ""), one, call("ops.js:crashes", nil), two, call("ops.js:crashes", nil))
		requireObservations(t, got, []spec.Observation{returns(spec.TypeInteger, "1"), raises("Crash"), returns(spec.TypeInteger, "2"), raises("Crash")})
	})
}

func TestInvokeRepeats(t *testing.T) {
	t.Run("a repeated call observes the same thing, in UTC and with the same random numbers [verifies SP-1026~1]", func(t *testing.T) {
		workspace := probeWorkspace(t, "esm", "")
		calls := []ports.ProbeCall{call("ops.js:luck", nil), call("ops.js:nowAtNoon", nil), call("ops.js:luck", nil)}
		first := invoke(t, workspace, calls...)
		second := invoke(t, workspace, calls...)
		requireObservations(t, second, first)
		if first[0] == first[2] || first[1].Value != "2026-03-01T12:00:00.250+00:00" {
			t.Fatalf("observations = %v", first)
		}
	})
}

func TestInvokeWithoutNode(t *testing.T) {
	t.Run("a machine without node is an error that says so, not a probe that raised [verifies SP-1027~1]", func(t *testing.T) {
		strategy := node.NewJest(node.Dependencies{Process: process.New(), LookPath: notFound})
		workspace := ecosystem.Workspace{Dir: t.TempDir(), ResultsDir: t.TempDir()}
		_, err := strategy.Prober().Invoke(context.Background(), workspace, ecosystem.Toolchain{}, []ports.ProbeCall{call("ops.js:lineTotal", nil)})
		if !errors.Is(err, node.ErrNoToolchain) {
			t.Fatalf("err = %v", err)
		}
	})
}
