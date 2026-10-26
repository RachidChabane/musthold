package golang

import (
	"context"
	"errors"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/ecosystem/golang/probeharness"
	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/differential"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
)

func TestParseProbeTarget(t *testing.T) {
	valid := []struct {
		text string
		want probeTarget
	}{
		{"example.com/parcel:LineTotal", probeTarget{Text: "example.com/parcel:LineTotal", ImportPath: "example.com/parcel", Name: "LineTotal"}},
		{"example.com/parcel/internal/tax:Rate", probeTarget{Text: "example.com/parcel/internal/tax:Rate", ImportPath: "example.com/parcel/internal/tax", Name: "Rate"}},
		{"example.com/parcel:Counter.Add", probeTarget{Text: "example.com/parcel:Counter.Add", ImportPath: "example.com/parcel", Receiver: "Counter", Name: "Add"}},
		{"example.com/v2.1/parcel:Quote", probeTarget{Text: "example.com/v2.1/parcel:Quote", ImportPath: "example.com/v2.1/parcel", Name: "Quote"}},
	}
	for _, tt := range valid {
		t.Run("reads "+tt.text+" [verifies SP-950~1]", func(t *testing.T) {
			got, err := parseProbeTarget(tt.text)
			if err != nil || got != tt.want {
				t.Fatalf("%+v %v", got, err)
			}
		})
	}
	for _, text := range []string{"", "LineTotal", ":LineTotal", "example.com/parcel:", "example.com/parcel:lineTotal", "example.com/parcel:counter.Add", "example.com/parcel:Counter.add", "example.com/parcel:Counter."} {
		t.Run("refuses "+text+" [verifies SP-950~1]", func(t *testing.T) {
			if _, err := parseProbeTarget(text); !errors.Is(err, ErrInvalidTarget) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestSnakeCase(t *testing.T) {
	tests := map[string]string{
		"unitPrice": "unit_price", "UnitPrice": "unit_price", "total": "total", "parseURLPath": "parse_url_path",
		"ID": "id", "userID": "user_id", "x2Value": "x2_value", "HTTPServer": "http_server",
	}
	for name, want := range tests {
		t.Run(name+" [verifies SP-950~1]", func(t *testing.T) {
			if got := snakeCase(name); got != want {
				t.Fatalf("snakeCase(%q) = %q, want %q", name, got, want)
			}
		})
	}
	t.Run("a snake case name differs from its Go name in case and underscores only [verifies SP-950~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			name := rapid.StringMatching(`[A-Za-z][A-Za-z0-9]{0,12}`).Draw(t, "name")
			snake := snakeCase(name)
			if strings.ToLower(strings.ReplaceAll(snake, "_", "")) != strings.ToLower(name) || strings.ToLower(snake) != snake {
				t.Fatalf("%q became %q", name, snake)
			}
		})
	})
	t.Run("an unnamed parameter is named by its position [verifies SP-950~1]", func(t *testing.T) {
		if parameterName("", 1) != "arg1" || parameterName("_", 0) != "arg0" || parameterName("unitPrice", 3) != "unit_price" {
			t.Fatal("parameter names are wrong")
		}
	})
}

// checked type-checks a package made of source, importing the standard library
// from its source.
func checked(t *testing.T, source string) *types.Package {
	t.Helper()
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "kinds.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := (&types.Config{Importer: importer.ForCompiler(fileSet, "source", nil)}).Check("kinds", fileSet, []*ast.File{file}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}

func TestClassifyParameterTypes(t *testing.T) {
	pkg := checked(t, `package kinds

import (
	"context"
	"math/big"
	"time"
)

type Cents int64
type Level string
type Reader struct{}

func (r *Reader) UnmarshalText(text []byte) error { return nil }

type Plain struct{}

func Bool(a bool) {}
func Int(a int8) {}
func Uint(a uint64) {}
func Float(a float32) {}
func String(a string) {}
func Named(a Cents, b Level) {}
func Time(a time.Time, b *time.Time) {}
func Big(a *big.Int, b *big.Rat, c *big.Float, d big.Int) {}
func Text(a Reader, b *Reader) {}
func Any(a any, b interface{}) {}
func Pointers(a *int, b *string, c **int, d *any) {}
func Unsupported(a []int, b map[string]int, c Plain, d func(), e chan int, f complex128, g error, h interface{ Close() error }) {}
func Context(ctx context.Context, a int) {}
`)
	kindsOf := func(name string) []string {
		signature := pkg.Scope().Lookup(name).Type().(*types.Signature)
		var kinds []string
		for index := 0; index < signature.Params().Len(); index++ {
			kind, pointer := classify(signature.Params().At(index).Type())
			text := string(kind)
			if pointer {
				text = "*" + text
			}
			kinds = append(kinds, text)
		}
		return kinds
	}
	tests := []struct {
		function string
		want     []string
	}{
		{"Bool", []string{"bool"}},
		{"Int", []string{"int"}},
		{"Uint", []string{"uint"}},
		{"Float", []string{"float"}},
		{"String", []string{"string"}},
		{"Named", []string{"int", "string"}},
		{"Time", []string{"time", "*time"}},
		{"Big", []string{"*integer", "*decimal", "*decimal", "integer"}},
		{"Text", []string{"text", "*text"}},
		{"Any", []string{"any", "any"}},
		{"Pointers", []string{"*int", "*string", "unsupported", "unsupported"}},
		{"Unsupported", []string{"unsupported", "unsupported", "unsupported", "unsupported", "unsupported", "unsupported", "unsupported", "unsupported"}},
	}
	for _, tt := range tests {
		t.Run(tt.function+" [verifies SP-950~1]", func(t *testing.T) {
			if got := kindsOf(tt.function); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("kinds = %v, want %v", got, tt.want)
			}
		})
	}
	t.Run("a context is recognized by its package and name [verifies SP-950~1]", func(t *testing.T) {
		signature := pkg.Scope().Lookup("Context").Type().(*types.Signature)
		if !isContext(signature.Params().At(0).Type()) || isContext(signature.Params().At(1).Type()) {
			t.Fatal("context recognition is wrong")
		}
	})
	t.Run("a kind with no distribution to draw from has no hint [verifies SP-950~1]", func(t *testing.T) {
		hints := map[probeharness.Kind]differential.Hint{
			probeharness.KindBool: differential.HintBoolean, probeharness.KindInt: differential.HintInteger,
			probeharness.KindUint: differential.HintInteger, probeharness.KindInteger: differential.HintInteger,
			probeharness.KindFloat: differential.HintDecimal, probeharness.KindDecimal: differential.HintDecimal,
			probeharness.KindString: differential.HintString, probeharness.KindText: differential.HintString,
			probeharness.KindTime: differential.HintDatetime, probeharness.KindAny: differential.HintUnknown,
			probeharness.KindUnsupported: differential.HintUnknown,
		}
		for kind, want := range hints {
			if got := hintOf(probeharness.Param{Kind: kind}); got != want {
				t.Errorf("hint of %s = %s, want %s", kind, got, want)
			}
		}
	})
}

func TestGroupOperations(t *testing.T) {
	workspace := ecosystem.Workspace{Dir: "/w"}
	at := func(dir string) operation {
		return operation{Target: probeTarget{Text: dir}, Dir: filepath.Join("/w", dir)}
	}
	dirs := func(groups []programGroup) map[string][]string {
		found := map[string][]string{}
		for _, group := range groups {
			for _, operation := range group.Operations {
				found[group.Dir] = append(found[group.Dir], operation.Target.Text)
			}
		}
		return found
	}

	t.Run("packages that no internal rule separates share one program [verifies SP-953~1]", func(t *testing.T) {
		got := dirs(groupOperations(workspace, []operation{at("."), at("pkg/tax"), at("internal/ledger")}))
		if len(got) != 1 || len(got["/w"]) != 3 {
			t.Fatalf("groups = %v", got)
		}
	})

	t.Run("a program placed where an internal package of a subtree is visible serves the packages above it [verifies SP-953~1]", func(t *testing.T) {
		got := dirs(groupOperations(workspace, []operation{at("."), at("nested/internal/vault"), at("nested/other")}))
		if len(got) != 1 || len(got["/w/nested"]) != 3 {
			t.Fatalf("groups = %v", got)
		}
	})

	t.Run("sibling subtrees with internal packages need programs of their own [verifies SP-953~1]", func(t *testing.T) {
		got := dirs(groupOperations(workspace, []operation{at("a/internal/x"), at("b/internal/y")}))
		if len(got) != 2 || len(got["/w/a"]) != 1 || len(got["/w/b"]) != 1 {
			t.Fatalf("groups = %v", got)
		}
	})

	t.Run("the root a package is visible from is the parent of its innermost internal directory [verifies SP-953~1]", func(t *testing.T) {
		tests := map[string]string{"pkg": "/w", "internal/x": "/w", "a/internal/b": "/w/a", "a/internal/b/internal/c": "/w/a/internal/b", "a/internal": "/w/a", "internals/x": "/w"}
		for relative, want := range tests {
			if got := visibilityRoot(workspace, filepath.Join("/w", relative)); got != want {
				t.Errorf("visibilityRoot(%s) = %s, want %s", relative, got, want)
			}
		}
	})
}

func TestRenderTable(t *testing.T) {
	operations := []operation{
		{Target: probeTarget{Text: "example.com/m:Add", ImportPath: "example.com/m", Name: "Add"}, Dir: "/w", Params: []probeharness.Param{{Name: "left", Kind: probeharness.KindInt}, {Name: "right", Kind: probeharness.KindInt, Pointer: true}}, ReturnsError: true, Sentinels: []string{"ErrBad"}},
		{Target: probeTarget{Text: "example.com/m:Counter.Add", ImportPath: "example.com/m", Receiver: "Counter", Name: "Add"}, Dir: "/w", Context: true},
		{Target: probeTarget{Text: "example.com/m/sub:Run", ImportPath: "example.com/m/sub", Name: "Run"}, Dir: "/w/sub"},
	}
	rendered, err := renderTable(operations)
	if err != nil {
		t.Fatal(err)
	}
	source := string(rendered)

	t.Run("the table is Go source that imports each package once [verifies SP-953~1]", func(t *testing.T) {
		file, err := parser.ParseFile(token.NewFileSet(), "main.go", rendered, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("%v\n%s", err, source)
		}
		var imports []string
		for _, spec := range file.Imports {
			imports = append(imports, spec.Path.Value)
		}
		if !reflect.DeepEqual(imports, []string{`"os"`, `"example.com/m"`, `"example.com/m/sub"`}) {
			t.Fatalf("imports = %v", imports)
		}
	})

	t.Run("each operation is an entry of the table with its function, kinds and error variables [verifies SP-953~1]", func(t *testing.T) {
		for _, want := range []string{
			`Target:       "example.com/m:Add"`, `Func:         target0.Add`,
			`{Name: "right", Kind: "int", Pointer: true}`, `ReturnsError: true`,
			`{Name: "ErrBad", Value: target0.ErrBad}`,
			`func() any { var receiver target0.Counter; return receiver.Add }()`,
			`Context:      true`, `Func:         target1.Run`,
		} {
			if !strings.Contains(source, want) {
				t.Errorf("the table lacks %q:\n%s", want, source)
			}
		}
	})
}

// scriptedProcess answers the build command and plays the probe program from a
// script: each run of the program is a function that writes the response file
// and tells how the process ended.
type scriptedProcess struct {
	builds int
	runs   int
	build  ports.CommandResult
	script []func(request, response string) ports.CommandResult
}

func (s *scriptedProcess) Run(_ context.Context, command ports.Command) (ports.CommandResult, error) {
	if len(command.Argv) > 1 && command.Argv[1] == buildSubcommand {
		s.builds++
		return s.build, nil
	}
	run := s.script[s.runs]
	s.runs++
	return run(command.Argv[1], command.Argv[2]), nil
}

func writeLines(t *testing.T, file string, lines ...string) {
	t.Helper()
	if err := os.WriteFile(file, []byte(strings.Join(lines, "")), 0o600); err != nil {
		t.Fatal(err)
	}
}

const (
	answerOne   = `{"kind":"returns","type":"integer","value":"1"}` + "\n"
	answerTwo   = `{"kind":"returns","type":"integer","value":"2"}` + "\n"
	answerThree = `{"kind":"returns","type":"integer","value":"3"}` + "\n"
	timeoutLine = `{"kind":"raises","value":"Timeout"}` + "\n"
)

func TestRunGroupContainsWhatKillsTheProgram(t *testing.T) {
	calls := make([]ports.ProbeCall, 3)
	for index := range calls {
		calls[index] = ports.ProbeCall{Target: "example.com/m:F"}
	}
	run := func(t *testing.T, process *scriptedProcess, count int) ([]spec.Observation, error) {
		t.Helper()
		dir := t.TempDir()
		workspace := ecosystem.Workspace{Dir: dir, ResultsDir: t.TempDir()}
		group := programGroup{Dir: dir, Operations: []operation{{Target: probeTarget{Text: "example.com/m:F", ImportPath: "example.com/m", Name: "F"}, Dir: dir}}}
		p := prober{deps: Dependencies{Process: process}}
		return p.runGroup(context.Background(), workspace, ecosystem.Toolchain{Command: []string{"go"}}, group, calls[:count])
	}
	finish := func(lines ...string) func(string, string) ports.CommandResult {
		return func(_, response string) ports.CommandResult {
			if err := os.WriteFile(response, []byte(strings.Join(lines, "")), 0o600); err != nil {
				t.Fatal(err)
			}
			return ports.CommandResult{}
		}
	}
	crashing := func(lines ...string) func(string, string) ports.CommandResult {
		return func(_, response string) ports.CommandResult {
			if err := os.WriteFile(response, []byte(strings.Join(lines, "")), 0o600); err != nil {
				t.Fatal(err)
			}
			return ports.CommandResult{ExitCode: 2}
		}
	}
	values := func(observed []spec.Observation) []string {
		var got []string
		for _, observation := range observed {
			got = append(got, string(observation.Kind)+":"+observation.Value)
		}
		return got
	}

	t.Run("every call answered by one process takes one run [verifies SP-954~1]", func(t *testing.T) {
		process := &scriptedProcess{script: []func(string, string) ports.CommandResult{finish(answerOne, answerTwo, answerThree)}}
		observed, err := run(t, process, 3)
		if err != nil || process.runs != 1 || process.builds != 1 || !reflect.DeepEqual(values(observed), []string{"returns:1", "returns:2", "returns:3"}) {
			t.Fatalf("%v after %d runs and %d builds (%v)", values(observed), process.runs, process.builds, err)
		}
	})

	t.Run("a call that kills the process is a crash and the calls after it run again [verifies SP-954~1]", func(t *testing.T) {
		process := &scriptedProcess{script: []func(string, string) ports.CommandResult{crashing(answerOne), finish(answerThree)}}
		observed, err := run(t, process, 3)
		want := []string{"returns:1", "raises:Crash", "returns:3"}
		if err != nil || !reflect.DeepEqual(values(observed), want) {
			t.Fatalf("%v (%v), want %v", values(observed), err, want)
		}
	})

	t.Run("a process killed in the middle of a line loses that line only [verifies SP-954~1]", func(t *testing.T) {
		process := &scriptedProcess{script: []func(string, string) ports.CommandResult{crashing(answerOne, `{"kind":"retu`), finish(answerThree)}}
		observed, err := run(t, process, 3)
		want := []string{"returns:1", "raises:Crash", "returns:3"}
		if err != nil || !reflect.DeepEqual(values(observed), want) {
			t.Fatalf("%v (%v), want %v", values(observed), err, want)
		}
	})

	t.Run("a call that outlives its deadline ends the run after its timeout and the calls after it run again [verifies SP-954~1]", func(t *testing.T) {
		process := &scriptedProcess{script: []func(string, string) ports.CommandResult{finish(answerOne, timeoutLine), finish(answerThree)}}
		observed, err := run(t, process, 3)
		want := []string{"returns:1", "raises:Timeout", "returns:3"}
		if err != nil || !reflect.DeepEqual(values(observed), want) {
			t.Fatalf("%v (%v), want %v", values(observed), err, want)
		}
	})

	t.Run("a process the deadline kills is a timeout of the call it was in [verifies SP-954~1]", func(t *testing.T) {
		killed := func(_, response string) ports.CommandResult {
			writeLines(t, response, answerOne)
			return ports.CommandResult{ExitCode: -1, TimedOut: true}
		}
		process := &scriptedProcess{script: []func(string, string) ports.CommandResult{killed, finish(answerThree)}}
		observed, err := run(t, process, 3)
		want := []string{"returns:1", "raises:Timeout", "returns:3"}
		if err != nil || !reflect.DeepEqual(values(observed), want) {
			t.Fatalf("%v (%v), want %v", values(observed), err, want)
		}
	})

	t.Run("a harness that cannot work is an error with its message [verifies SP-954~1]", func(t *testing.T) {
		broken := func(string, string) ports.CommandResult {
			return ports.CommandResult{ExitCode: probeharness.ExitFailure, Stderr: []byte("cannot read the request")}
		}
		process := &scriptedProcess{script: []func(string, string) ports.CommandResult{broken}}
		if _, err := run(t, process, 3); !errors.Is(err, ErrProbeFailed) || !strings.Contains(err.Error(), "cannot read the request") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("a process that answers nothing and ends cleanly is an error rather than a loop [verifies SP-954~1]", func(t *testing.T) {
		process := &scriptedProcess{script: []func(string, string) ports.CommandResult{func(string, string) ports.CommandResult { return ports.CommandResult{} }}}
		if _, err := run(t, process, 3); !errors.Is(err, ErrProbeFailed) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("a program that does not build is an error with the compiler's message [verifies SP-953~1]", func(t *testing.T) {
		process := &scriptedProcess{build: ports.CommandResult{ExitCode: 1, Stderr: []byte("undefined: Missing")}}
		if _, err := run(t, process, 3); !errors.Is(err, ErrProbeFailed) || !strings.Contains(err.Error(), "undefined: Missing") || process.runs != 0 {
			t.Fatalf("err = %v after %d runs", err, process.runs)
		}
	})

	t.Run("more observations than calls is an error [verifies SP-954~1]", func(t *testing.T) {
		process := &scriptedProcess{script: []func(string, string) ports.CommandResult{finish(answerOne, answerTwo)}}
		if _, err := run(t, process, 1); !errors.Is(err, ErrProbeFailed) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestReadObservations(t *testing.T) {
	file := filepath.Join(t.TempDir(), "response.jsonl")

	t.Run("a response that was never written holds no observation [verifies SP-954~1]", func(t *testing.T) {
		if got, err := readObservations(file); err != nil || len(got) != 0 {
			t.Fatalf("%v %v", got, err)
		}
	})
	t.Run("a line that is not an observation is an error [verifies SP-954~1]", func(t *testing.T) {
		writeLines(t, file, "not json\n")
		if _, err := readObservations(file); !errors.Is(err, ErrProbeFailed) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("a response that cannot be read is an error [verifies SP-954~1]", func(t *testing.T) {
		if _, err := readObservations(t.TempDir()); err == nil {
			t.Fatal("no error")
		}
	})
}
