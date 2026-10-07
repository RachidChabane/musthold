package modulecli_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/s1relay/internal/adapters/modulecli"
	"github.com/RachidChabane/spec-proof/s1relay/internal/domain/failure"
	"github.com/RachidChabane/spec-proof/s1relay/internal/domain/question"
)

const (
	helperVariable = "S1RELAY_HELPER_PROCESS"
	helperTest     = "^TestHelperProcess$"
	exportOutput   = `[{"ticket":"aa.0.1","nonce":"n1","closed":{"state":"S","questions":[{"id":"q1","type":"choice","instructions":"Q?","options":[{"name":"A","description":"a"},{"name":"B","description":"b"}]}]}},{"ticket":"bb.0.1","nonce":"n2","closed":{"state":"S","questions":[{"id":"q1","type":"noul","instructions":"Is it?"}]}}]`
)

type call struct {
	dir   string
	name  string
	args  []string
	stdin string
}

// fakeRunner records the calls and serves one canned result.
type fakeRunner struct {
	calls  []call
	stdout string
	stderr string
	err    error
}

func (f *fakeRunner) Run(_ context.Context, dir, name string, args []string, stdin []byte) ([]byte, []byte, error) {
	f.calls = append(f.calls, call{dir: dir, name: name, args: args, stdin: string(stdin)})
	return []byte(f.stdout), []byte(f.stderr), f.err
}

func TestBinaryName(t *testing.T) {
	t.Run("a module is run as spec-proof- and its name [verifies SP-602~1]", func(t *testing.T) {
		for _, module := range []string{"dp", "alignment", "completeness", "a-1"} {
			got, err := modulecli.BinaryName(module)
			if err != nil || got != "spec-proof-"+module {
				t.Errorf("BinaryName(%q) = %q, %v", module, got, err)
			}
		}
	})

	t.Run("a name that could reach another executable is refused [verifies SP-602~1]", func(t *testing.T) {
		for _, module := range []string{"", "../dp", "dp/x", `dp\x`, "DP", "dp x", "-dp", "dp;rm", "dp.exe"} {
			if _, err := modulecli.BinaryName(module); !errors.Is(err, modulecli.ErrModuleName) {
				t.Errorf("BinaryName(%q) accepted", module)
			}
			if _, err := modulecli.NewGateway(module, ".", &fakeRunner{}); !errors.Is(err, modulecli.ErrModuleName) {
				t.Errorf("NewGateway(%q) accepted", module)
			}
		}
	})
}

func TestExport(t *testing.T) {
	t.Run("export runs the module binary from the project root and reads its closed requests [verifies SP-600~2]", func(t *testing.T) {
		runner := &fakeRunner{stdout: exportOutput}
		gateway, err := modulecli.NewGateway("dp", "/project", runner)
		if err != nil {
			t.Fatal(err)
		}
		exported, err := gateway.Export(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		want := call{dir: "/project", name: "spec-proof-dp", args: []string{"systemone", "export"}}
		if len(runner.calls) != 1 || runner.calls[0].dir != want.dir || runner.calls[0].name != want.name || strings.Join(runner.calls[0].args, " ") != "systemone export" {
			t.Fatalf("calls %+v", runner.calls)
		}
		if len(exported) != 2 || exported[0].Ticket != "aa.0.1" || exported[0].Nonce != "n1" || exported[0].Problem != nil || exported[0].Set.Questions[0].Options[1].Name != "B" {
			t.Fatalf("exported %+v", exported)
		}
	})

	t.Run("a request whose question set cannot be read is exported with its problem [verifies SP-616~1]", func(t *testing.T) {
		gateway, _ := modulecli.NewGateway("dp", "/project", &fakeRunner{stdout: exportOutput})
		exported, _ := gateway.Export(context.Background())
		if exported[1].Ticket != "bb.0.1" || !errors.Is(exported[1].Problem, question.ErrUnsupportedType) {
			t.Fatalf("second request %+v", exported[1])
		}
	})

	t.Run("a binary that fails or prints something else than a list is a tool failure [verifies SP-602~1]", func(t *testing.T) {
		for name, runner := range map[string]*fakeRunner{
			"a failing binary":   {err: errors.New("exit status 2"), stderr: "no project here"},
			"output that is not": {stdout: "not json"},
		} {
			gateway, _ := modulecli.NewGateway("dp", "/project", runner)
			if _, err := gateway.Export(context.Background()); err == nil {
				t.Errorf("%s: no error", name)
			}
		}
		gateway, _ := modulecli.NewGateway("dp", "/project", &fakeRunner{err: errors.New("exit status 2"), stderr: "no project here"})
		if _, err := gateway.Export(context.Background()); !strings.Contains(err.Error(), "no project here") {
			t.Errorf("the module's stderr is lost: %v", err)
		}
	})
}

func TestImport(t *testing.T) {
	reply := []byte(`{"model":"m","answers":{"q1":{"type":"choice","choice":"A","confidence":0.5}}}`)

	t.Run("import gives the ticket, nonce and source and the reply of the endpoint on stdin [verifies SP-600~2]", func(t *testing.T) {
		runner := &fakeRunner{}
		gateway, _ := modulecli.NewGateway("alignment", "/project", runner)
		if err := gateway.Import(context.Background(), "aa.0.1", "n1", "system-one:m@d", reply); err != nil {
			t.Fatal(err)
		}
		got := runner.calls[0]
		if got.name != "spec-proof-alignment" || strings.Join(got.args, " ") != "systemone import aa.0.1 --nonce n1 --source system-one:m@d" {
			t.Fatalf("call %+v", got)
		}
		if got.stdin != string(reply) {
			t.Fatalf("stdin %q", got.stdin)
		}
	})

	t.Run("a module that refuses the answers is an import failure that quotes its reason [verifies SP-614~1]", func(t *testing.T) {
		runner := &fakeRunner{err: errors.New("exit status 1"), stderr: "nonce mismatch"}
		gateway, _ := modulecli.NewGateway("dp", "/project", runner)
		err := gateway.Import(context.Background(), "aa.0.1", "bad", "system-one:m@d", reply)
		if failure.KindOf(err) != failure.KindImport || !strings.Contains(failure.DetailOf(err), "nonce mismatch") {
			t.Fatalf("error %v", err)
		}
	})
}

// TestHelperProcess is the program ExecRunner runs in TestExecRunner; it does
// nothing in a normal test run.
func TestHelperProcess(t *testing.T) {
	mode := os.Getenv(helperVariable)
	if mode == "" {
		return
	}
	switch mode {
	case "echo":
		stdin, _ := io.ReadAll(os.Stdin)
		dir, _ := os.Getwd()
		fmt.Printf("%s|%s", dir, stdin)
	case "fail":
		fmt.Fprint(os.Stderr, "the module refused")
		os.Exit(3)
	}
	os.Exit(0)
}

func TestExecRunner(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"-test.run=" + helperTest}

	t.Run("a subprocess runs in the given directory with the given standard input [verifies SP-600~2]", func(t *testing.T) {
		t.Setenv(helperVariable, "echo")
		stdout, _, err := modulecli.ExecRunner{}.Run(context.Background(), dir, os.Args[0], args, []byte("answers"))
		if err != nil {
			t.Fatal(err)
		}
		got := string(stdout)
		resolved, _ := filepath.EvalSymlinks(strings.SplitN(got, "|", 2)[0])
		if resolved != dir || !strings.HasSuffix(got, "|answers") {
			t.Fatalf("stdout %q", got)
		}
	})

	t.Run("a subprocess that exits non-zero reports its stderr and its error [verifies SP-614~1]", func(t *testing.T) {
		t.Setenv(helperVariable, "fail")
		_, stderr, err := modulecli.ExecRunner{}.Run(context.Background(), dir, os.Args[0], args, nil)
		if err == nil || string(stderr) != "the module refused" {
			t.Fatalf("stderr %q, error %v", stderr, err)
		}
	})

	t.Run("an executable that does not exist is an error [verifies SP-602~1]", func(t *testing.T) {
		gateway, err := modulecli.NewGateway("no-such-module-on-this-machine", dir, modulecli.ExecRunner{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := gateway.Export(context.Background()); err == nil {
			t.Fatal("a missing binary exported")
		}
	})
}
