package construct_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
)

// fakeCodex stands in for the host: it answers `codex --version` and
// `codex exec`, writing the final message where -o says.
type fakeCodex struct {
	t       *testing.T
	mu      sync.Mutex
	argvs   [][]string
	message string
	exit    int
	// emptyAtCall and schemaAtCall are what the call found on disk.
	emptyAtCall  []os.DirEntry
	schemaAtCall string
	cdAtCall     string
}

func (f *fakeCodex) Run(_ context.Context, argv []string) (construct.ProcessResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.argvs = append(f.argvs, argv)
	if len(argv) == 2 && argv[1] == "--version" {
		return construct.ProcessResult{Stdout: []byte(fakeVersion + "\n")}, nil
	}
	flag := func(name string) string {
		for index, arg := range argv {
			if arg == name && index+1 < len(argv) {
				return argv[index+1]
			}
		}
		f.t.Fatalf("no %s in %v", name, argv)
		return ""
	}
	f.cdAtCall = flag("--cd")
	f.emptyAtCall, _ = os.ReadDir(f.cdAtCall)
	schema, _ := os.ReadFile(flag("--output-schema"))
	f.schemaAtCall = string(schema)
	if err := os.WriteFile(flag("-o"), []byte(f.message), 0o600); err != nil {
		f.t.Fatal(err)
	}
	return construct.ProcessResult{Stderr: []byte("stderr text"), ExitCode: f.exit}, nil
}

func newCodex(t *testing.T) (*fakeCodex, construct.CodexModel) {
	fake := &fakeCodex{t: t, message: `{"files":[{"path":"a.py","content":"x = 1\n"}]}`}
	tick := time.Date(2026, time.October, 27, 8, 0, 0, 0, time.UTC)
	return fake, construct.CodexModel{Process: fake, Now: func() time.Time { tick = tick.Add(time.Minute); return tick }}
}

func schemaFor(t *testing.T) construct.Schema {
	schema, err := construct.FilesSchema()
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func TestCodexAdapter(t *testing.T) {
	t.Run("a call runs exactly the codex exec command of the protocol, in an empty directory with the schema file [verifies SP-2541~1]", func(t *testing.T) {
		fake, model := newCodex(t)
		reply, err := model.Complete(t.Context(), construct.Request{Prompt: "the prompt", Schema: schemaFor(t)})
		if err != nil {
			t.Fatal(err)
		}
		exec := fake.argvs[1]
		want := []string{
			"codex", "exec", "-m", "gpt-5.6-terra", "-c", "model_reasoning_effort=high", "--sandbox", "read-only",
			"--ignore-user-config", "--ephemeral", "--skip-git-repo-check",
			"--cd", exec[12], "--output-schema", exec[14], "-o", exec[16], "the prompt",
		}
		if !reflect.DeepEqual(exec, want) {
			t.Fatalf("argv\n%v\nwant\n%v", exec, want)
		}
		if len(fake.emptyAtCall) != 0 || fake.schemaAtCall != string(schemaFor(t).JSON) {
			t.Errorf("the directory held %d entries, the schema file %q", len(fake.emptyAtCall), fake.schemaAtCall)
		}
		if len(reply.Files) != 1 || reply.Files[0].Path != "a.py" || string(reply.Output) != fake.message {
			t.Errorf("reply %+v", reply)
		}
		if _, err := os.Stat(filepath.Dir(fake.cdAtCall)); !os.IsNotExist(err) {
			t.Errorf("the scratch directory remains: %v", err)
		}
	})

	t.Run("the Codex version is read once per call and returned with the call's times and exit [verifies SP-2541~1]", func(t *testing.T) {
		fake, model := newCodex(t)
		for range 2 {
			reply, err := model.Complete(t.Context(), construct.Request{Prompt: "p", Schema: schemaFor(t)})
			if err != nil || reply.CodexVersion != fakeVersion || !reply.End.After(reply.Start) || reply.Start.Location() != time.UTC {
				t.Fatalf("%v %+v", err, reply)
			}
		}
		versions := 0
		for _, argv := range fake.argvs {
			if len(argv) == 2 && argv[1] == "--version" {
				versions++
			}
		}
		if versions != 2 {
			t.Fatalf("%d version reads for two calls", versions)
		}
	})

	t.Run("a failed call and a malformed final message are errors that still carry what the call produced [verifies SP-2541~1]", func(t *testing.T) {
		fake, model := newCodex(t)
		fake.exit = 3
		reply, err := model.Complete(t.Context(), construct.Request{Prompt: "p", Schema: schemaFor(t)})
		if !errors.Is(err, construct.ErrCodexExit) || reply.Exit != 3 || string(reply.Stderr) != "stderr text" || reply.CodexVersion == "" {
			t.Fatalf("%v %+v", err, reply)
		}
		fake.exit, fake.message = 0, `{"files":[{"path":"a.py"}],"extra":1}`
		reply, err = model.Complete(t.Context(), construct.Request{Prompt: "p", Schema: schemaFor(t)})
		if !errors.Is(err, construct.ErrBadReply) || string(reply.Output) != fake.message {
			t.Fatalf("%v %+v", err, reply)
		}
	})

	t.Run("the host process runs with its standard input closed [verifies SP-2541~1]", func(t *testing.T) {
		done := make(chan construct.ProcessResult, 1)
		go func() {
			result, _ := construct.ExecProcess{}.Run(t.Context(), []string{"cat"})
			done <- result
		}()
		select {
		case result := <-done:
			if result.ExitCode != 0 || len(result.Stdout) != 0 {
				t.Fatalf("%+v", result)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("cat is still waiting for input")
		}
		if _, err := (construct.ExecProcess{}).Run(t.Context(), []string{"/nonexistent/program"}); err == nil {
			t.Error("a program that cannot start was not an error")
		}
		result, err := construct.ExecProcess{}.Run(t.Context(), []string{"sh", "-c", "exit 4"})
		if err != nil || result.ExitCode != 4 {
			t.Errorf("%v %+v", err, result)
		}
	})
}

func TestTheTestWriterPromptThroughTheAdapter(t *testing.T) {
	t.Run("the prompt that reaches codex for the test writer holds the spec and the interface and no line of the project [verifies SP-2544~1]", func(t *testing.T) {
		fake, model := newCodex(t)
		w := newWorld(t, domains("d1"), nil)
		w.markBoxofficeBuilt()
		w.constructor.Target = 1
		answers := map[records.Role]func() string{
			records.RoleAuthor: func() string { return filesJSON(authorFiles(w.sample, nil)) },
			records.RoleOracle: func() string { return filesJSON(oracleFiles(w.sample)) },
		}
		model.Process = processFunc(func(argv []string) (construct.ProcessResult, error) {
			if len(argv) == 2 {
				return fake.Run(t.Context(), argv)
			}
			role := records.RoleAuthor
			if strings.Contains(argv[len(argv)-1], "Write pytest tests") {
				role = records.RoleOracle
			}
			fake.message = answers[role]()
			return fake.Run(t.Context(), argv)
		})
		w.constructor.Model = model
		if _, err := w.constructor.Run(t.Context()); err != nil {
			t.Fatal(err)
		}
		var testPrompts, authorPrompts []string
		for _, argv := range fake.argvs {
			if len(argv) > 2 {
				prompt := argv[len(argv)-1]
				if strings.Contains(prompt, "Write pytest tests") {
					testPrompts = append(testPrompts, prompt)
				} else {
					authorPrompts = append(authorPrompts, prompt)
				}
			}
		}
		if len(testPrompts) != 1 || len(authorPrompts) != 1 {
			t.Fatalf("%d test prompts, %d author prompts", len(testPrompts), len(authorPrompts))
		}
		for _, source := range []string{"SHORT_LIMIT", "return 14", "test_short_trip_costs_two", "BREAKS", "fare_for(distance=3) == 2"} {
			if strings.Contains(testPrompts[0], source) {
				t.Errorf("the test writer's prompt holds %q", source)
			}
		}
		if !strings.Contains(testPrompts[0], "fare.zones") || !strings.Contains(authorPrompts[0], "ticket_price") {
			t.Error("the prompts lack their inputs")
		}
	})
}

type processFunc func(argv []string) (construct.ProcessResult, error)

func (f processFunc) Run(_ context.Context, argv []string) (construct.ProcessResult, error) {
	return f(argv)
}

func filesJSON(files []construct.File) string {
	encoded, _ := json.Marshal(struct {
		Files []construct.File `json:"files"`
	}{files})
	return string(encoded)
}
