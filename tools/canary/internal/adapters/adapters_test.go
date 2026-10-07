package adapters_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/format/hostproto/mcpstdio"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/adapters"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/app"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
)

// Modes in which the test binary stands in for another program.
const (
	helperVariable = "CANARY_TEST_HELPER"
	helperShimMCP  = "shim-mcp"
	helperRefuse   = "mcp-refuse"
	helperServe    = "mcp-serve"
	helperSilent   = "mcp-silent"
	helperErrorRPC = "mcp-rpc-error"
	helperOneTool  = "mcp-one-tool"

	recordVariable = "CANARY_TEST_RECORD"
)

func TestMain(m *testing.M) {
	switch os.Getenv(helperVariable) {
	case helperShimMCP:
		os.Exit(adapters.RunShim("/bin/echo", os.Getenv(recordVariable), []string{"mcp", "--role", "executor"}, os.Stdin, os.Stdout, os.Stderr))
	case helperRefuse, helperServe, helperSilent, helperErrorRPC, helperOneTool:
		serveFakeMCP(os.Getenv(helperVariable))
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// serveFakeMCP answers the JSON-RPC requests of the cap probe the way the
// executor server would, for the mode.
func serveFakeMCP(mode string) {
	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}
		var request mcpstdio.Message
		if json.Unmarshal(line, &request) != nil || len(request.ID) == 0 {
			continue
		}
		if mode == helperSilent {
			continue
		}
		response := map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(request.ID)}
		switch {
		case request.Method == "initialize":
			response["result"] = map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "serverInfo": map[string]string{"name": "fake"}}
		case request.Method == "tools/list":
			tools := []map[string]string{{"name": "fetch_request"}, {"name": "submit_response"}}
			if mode == helperOneTool {
				tools = tools[:1]
			}
			response["result"] = map[string]any{"tools": tools}
		case mode == helperErrorRPC:
			response["error"] = map[string]any{"code": -32602, "message": "capability required"}
		case mode == helperRefuse:
			response["result"] = map[string]any{"isError": true, "content": []map[string]string{{"type": "text", "text": "the capability is missing"}}}
		default:
			response["result"] = map[string]any{"isError": false, "content": []map[string]string{{"type": "text", "text": `{"ticket":"t","nonce":"n","responseSchema":{}}`}}}
		}
		encoded, _ := json.Marshal(response)
		fmt.Fprintf(os.Stdout, "%s\n", encoded)
	}
}

func readRecords(t *testing.T, path string) []evidence.HookCall {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines, err := evidence.Lines(path, data)
	if err != nil {
		t.Fatal(err)
	}
	var calls []evidence.HookCall
	for _, line := range lines {
		var call evidence.HookCall
		if err := json.Unmarshal(line.Raw, &call); err != nil {
			t.Fatal(err)
		}
		calls = append(calls, call)
	}
	return calls
}

func fakeBinary(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "real")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestShim(t *testing.T) {
	t.Run("a hook is run for real and its payload, answer and exit code are recorded while the harness gets the answer [verifies SP-1506~1]", func(t *testing.T) {
		real := fakeBinary(t, `cat >/dev/null; printf '{"hookSpecificOutput":{"permissionDecision":"deny"}}'; exit 3`)
		record := filepath.Join(t.TempDir(), "hooks.jsonl")
		var stdout, stderr strings.Builder
		code := adapters.RunShim(real, record, []string{"hook", "pre-tool", "--harness", "claude"}, strings.NewReader(`{"tool_name":"Agent"}`), &stdout, &stderr)
		if code != 3 || stdout.String() != `{"hookSpecificOutput":{"permissionDecision":"deny"}}` {
			t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
		}
		calls := readRecords(t, record)
		if len(calls) != 1 || calls[0].Module != "real" || calls[0].Kind() != "pre-tool" || calls[0].ExitCode != 3 {
			t.Fatalf("records = %+v", calls)
		}
		if calls[0].Payload().ToolName != "Agent" || calls[0].Decision().Permission != "deny" {
			t.Errorf("record = %+v", calls[0])
		}
	})

	t.Run("a payload that is not JSON is kept as a string [verifies SP-1506~1]", func(t *testing.T) {
		real := fakeBinary(t, `cat >/dev/null`)
		record := filepath.Join(t.TempDir(), "hooks.jsonl")
		adapters.RunShim(real, record, []string{"hook", "subagent-start"}, strings.NewReader("plain text"), &strings.Builder{}, &strings.Builder{})
		if got := string(readRecords(t, record)[0].Stdin); got != `"plain text"` {
			t.Errorf("stdin = %s", got)
		}
	})

	t.Run("a shim with no binary to stand for refuses [verifies SP-1506~1]", func(t *testing.T) {
		var stderr strings.Builder
		if code := adapters.RunShim("", "x", nil, strings.NewReader(""), &strings.Builder{}, &stderr); code == 0 || !strings.Contains(stderr.String(), "--real") {
			t.Errorf("exit %d, stderr %q", code, stderr.String())
		}
	})

	t.Run("a command that is no hook is recorded and replaced by the real binary [verifies SP-1506~1]", func(t *testing.T) {
		record := filepath.Join(t.TempDir(), "hooks.jsonl")
		command := exec.Command(os.Args[0])
		command.Env = append(os.Environ(), helperVariable+"="+helperShimMCP, recordVariable+"="+record)
		output, err := command.Output()
		if err != nil || strings.TrimSpace(string(output)) != "mcp --role executor" {
			t.Fatalf("output %q, %v", output, err)
		}
		calls := readRecords(t, record)
		if len(calls) != 1 || calls[0].Module != "echo" || calls[0].IsModuleHook() || calls[0].Args[0] != "mcp" {
			t.Fatalf("records = %+v", calls)
		}
	})
}

func TestProbe(t *testing.T) {
	t.Run("a probe records the payload under its label and exits 0 even when it cannot write [verifies SP-1506~1]", func(t *testing.T) {
		record := filepath.Join(t.TempDir(), "hooks.jsonl")
		if code := adapters.RunProbe(record, "Stop-probe", strings.NewReader(`{"hook_event_name":"Stop"}`)); code != 0 {
			t.Fatalf("exit %d", code)
		}
		calls := readRecords(t, record)
		if len(calls) != 1 || calls[0].Label != "Stop-probe" || calls[0].Payload().EventName != "Stop" {
			t.Fatalf("records = %+v", calls)
		}
		if code := adapters.RunProbe(filepath.Join(record, "no", "such", "dir"), "x", strings.NewReader("")); code != 0 {
			t.Errorf("an unwritable record changed the exit code to %d", code)
		}
	})

	t.Run("concurrent hooks append whole lines [verifies SP-1506~1]", func(t *testing.T) {
		record := filepath.Join(t.TempDir(), "hooks.jsonl")
		var wait sync.WaitGroup
		for index := range 40 {
			wait.Add(1)
			go func() {
				defer wait.Done()
				adapters.RunProbe(record, fmt.Sprintf("p%d", index), strings.NewReader(`{"a":"`+strings.Repeat("x", 2000)+`"}`))
			}()
		}
		wait.Wait()
		if calls := readRecords(t, record); len(calls) != 40 {
			t.Fatalf("%d records, want 40", len(calls))
		}
	})
}

func TestLock(t *testing.T) {
	t.Run("a second session cannot start while the first holds the lock [verifies SP-1505~1]", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "canary", "session.lock")
		first, err := adapters.FileLocker{}.Acquire(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := (adapters.FileLocker{}).Acquire(path); !errors.Is(err, adapters.ErrSessionRunning) {
			t.Fatalf("error = %v", err)
		}
		first.Release()
		again, err := adapters.FileLocker{}.Acquire(path)
		if err != nil {
			t.Fatalf("the lock was not released: %v", err)
		}
		again.Release()
	})
}

func session(t *testing.T, script string, capSteps int, timeout time.Duration) (app.SessionOutcome, string, time.Duration) {
	t.Helper()
	dir := t.TempDir()
	spec := app.SessionSpec{
		Argv: []string{"/bin/sh", "-c", script}, Dir: dir, Env: os.Environ(),
		StdoutPath: filepath.Join(dir, "stream.jsonl"), StderrPath: filepath.Join(dir, "stderr.txt"),
		Counts: func(line []byte) bool { return strings.Contains(string(line), "assistant") },
		Cap:    capSteps, Timeout: timeout,
	}
	started := time.Now()
	outcome, err := adapters.Sessions{}.Run(spec)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := os.ReadFile(spec.StdoutPath)
	if err != nil {
		t.Fatal(err)
	}
	return outcome, string(stream), time.Since(started)
}

func TestSessionRunner(t *testing.T) {
	t.Run("a session that ends by itself keeps its output and exit code [verifies SP-1505~1]", func(t *testing.T) {
		outcome, stream, _ := session(t, `echo '{"type":"assistant"}'; echo '{"type":"result"}'; exit 4`, 5, 30*time.Second)
		if outcome.ExitCode != 4 || outcome.Killed != evidence.KilledNone || stream != "{\"type\":\"assistant\"}\n{\"type\":\"result\"}\n" {
			t.Fatalf("outcome %+v, stream %q", outcome, stream)
		}
	})

	t.Run("a session past the event cap is ended with everything it started [verifies SP-1505~1]", func(t *testing.T) {
		script := `sleep 60 & for i in 1 2 3 4 5 6; do echo '{"type":"assistant"}'; sleep 0.2; done; sleep 60`
		outcome, stream, elapsed := session(t, script, 2, 60*time.Second)
		if outcome.Killed != evidence.KilledTurnCap {
			t.Fatalf("outcome %+v", outcome)
		}
		if elapsed > 20*time.Second || strings.Count(stream, "assistant") > 4 {
			t.Errorf("the session ran %s and printed %d steps", elapsed, strings.Count(stream, "assistant"))
		}
	})

	t.Run("a session past its timeout is ended [verifies SP-1505~1]", func(t *testing.T) {
		outcome, _, elapsed := session(t, `sleep 60`, 5, 300*time.Millisecond)
		if outcome.Killed != evidence.KilledTimeout || elapsed > 20*time.Second {
			t.Fatalf("outcome %+v after %s", outcome, elapsed)
		}
	})
}

func TestExec(t *testing.T) {
	t.Run("a command's output and non-zero exit are results, a timeout is an error [verifies SP-1505~1]", func(t *testing.T) {
		output, err := adapters.Exec{}.Run(context.Background(), app.CommandSpec{Argv: []string{"/bin/sh", "-c", "echo out; echo err >&2; exit 7"}, Env: os.Environ()})
		if err != nil || output.ExitCode != 7 || output.Stdout != "out\n" || output.Stderr != "err\n" {
			t.Fatalf("output %+v, %v", output, err)
		}
		_, err = adapters.Exec{}.Run(context.Background(), app.CommandSpec{Argv: []string{"/bin/sleep", "30"}, Env: os.Environ(), Timeout: 200 * time.Millisecond})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("error = %v", err)
		}
		if _, err := (adapters.Exec{}).Run(context.Background(), app.CommandSpec{Argv: []string{"/no/such/binary"}}); err == nil {
			t.Fatal("a missing binary started")
		}
	})
}

func TestCapProbe(t *testing.T) {
	probe := func(mode string) evidence.CapProbe {
		spec := app.ServerSpec{Binary: os.Args[0], Harness: "claude", Dir: t.TempDir(), Env: append(os.Environ(), helperVariable+"="+mode)}
		return adapters.McpProber{}.Probe(context.Background(), spec, "ticket-1")
	}

	t.Run("a server that answers with an error result has refused [verifies SP-1502~1]", func(t *testing.T) {
		got := probe(helperRefuse)
		if !got.Attempted || !got.Responded || !got.Refused || got.ServedTicket || !strings.Contains(got.Detail, "capability is missing") {
			t.Fatalf("probe = %+v", got)
		}
	})

	t.Run("a server that answers with a JSON-RPC error has refused [verifies SP-1502~1]", func(t *testing.T) {
		got := probe(helperErrorRPC)
		if !got.Refused || got.ServedTicket || !strings.Contains(got.Detail, "capability required") {
			t.Fatalf("probe = %+v", got)
		}
	})

	t.Run("a server that returns the ticket has served it [verifies SP-1502~1]", func(t *testing.T) {
		got := probe(helperServe)
		if got.Refused || !got.ServedTicket || !got.Responded {
			t.Fatalf("probe = %+v", got)
		}
	})

	t.Run("a binary that is not there leaves the probe unanswered [verifies SP-1502~1]", func(t *testing.T) {
		got := adapters.McpProber{}.Probe(context.Background(), app.ServerSpec{Binary: "/no/such/binary", Harness: "claude", Dir: t.TempDir()}, "t")
		if !got.Attempted || got.Responded || got.Detail == "" {
			t.Fatalf("probe = %+v", got)
		}
	})
}

func TestHandshake(t *testing.T) {
	handshake := func(mode string) evidence.ServerStart {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		spec := app.ServerSpec{Binary: os.Args[0], Harness: "codex", Dir: t.TempDir(), Env: append(os.Environ(), helperVariable+"="+mode)}
		return adapters.McpProber{}.Handshake(ctx, spec)
	}

	t.Run("a server that answers the handshake and lists both executor tools has started [verifies SP-1502~1]", func(t *testing.T) {
		got := handshake(helperServe)
		if !got.Started || !strings.Contains(got.Detail, "submit_response") || got.Binary == "" {
			t.Fatalf("start = %+v", got)
		}
	})

	t.Run("a server that lists one tool, or never answers, has not started [verifies SP-1502~1]", func(t *testing.T) {
		if got := handshake(helperOneTool); got.Started || !strings.Contains(got.Detail, "submit_response") {
			t.Errorf("start = %+v", got)
		}
		if got := handshake(helperSilent); got.Started || got.Detail == "" {
			t.Errorf("start = %+v", got)
		}
		missing := adapters.McpProber{}.Handshake(context.Background(), app.ServerSpec{Binary: "/no/such/binary", Harness: "claude", Dir: t.TempDir()})
		if missing.Started || missing.Binary != "binary" {
			t.Errorf("start = %+v", missing)
		}
	})
}

func TestFingerprint(t *testing.T) {
	t.Run("a fingerprint sees an entry that appears, one that changes and the directories above them, down to a depth [verifies SP-1505~1]", func(t *testing.T) {
		files := adapters.OSFiles{}
		root := t.TempDir()
		for _, path := range []string{"a.json", "projects/p1/s.jsonl", "projects/p1/deep/er/x"} {
			if err := files.Write(filepath.Join(root, path), []byte("1"), false); err != nil {
				t.Fatal(err)
			}
		}
		before, err := files.Fingerprint(root, 2)
		if err != nil {
			t.Fatal(err)
		}
		if _, deeper := before[filepath.Join(root, "projects", "p1", "deep", "er")]; deeper {
			t.Error("an entry below the depth was listed")
		}
		for _, path := range []string{"a.json", "projects", "projects/p1"} {
			if _, listed := before[filepath.Join(root, path)]; !listed {
				t.Errorf("%s is not in the fingerprint", path)
			}
		}
		if _, listed := before[filepath.Join(root, "projects", "p1", "s.jsonl")]; listed {
			t.Error("an entry three levels down was listed")
		}
		if err := files.Write(filepath.Join(root, "projects", "p2", "new.jsonl"), []byte("22"), false); err != nil {
			t.Fatal(err)
		}
		if err := files.Write(filepath.Join(root, "a.json"), []byte("changed content"), false); err != nil {
			t.Fatal(err)
		}
		after, _ := files.Fingerprint(root, 2)
		if after[filepath.Join(root, "a.json")] == before[filepath.Join(root, "a.json")] {
			t.Error("a changed file kept its fingerprint")
		}
		if _, appeared := after[filepath.Join(root, "projects", "p2")]; !appeared {
			t.Error("a new directory is missing")
		}
	})

	t.Run("a path that does not exist has no entries [verifies SP-1505~1]", func(t *testing.T) {
		got, err := adapters.OSFiles{}.Fingerprint(filepath.Join(t.TempDir(), "absent"), 2)
		if err != nil || len(got) != 0 {
			t.Fatalf("fingerprint = %v, %v", got, err)
		}
	})
}

func TestFiles(t *testing.T) {
	files := adapters.OSFiles{}

	t.Run("a copy leaves the interpreter and test caches out [verifies SP-1507~1]", func(t *testing.T) {
		from, to := t.TempDir(), filepath.Join(t.TempDir(), "copy")
		for path, content := range map[string]string{"a/b.py": "x", "a/__pycache__/b.pyc": "y", ".pytest_cache/n": "z", "test-results/r.xml": "w", "top.txt": "t"} {
			if err := files.Write(filepath.Join(from, path), []byte(content), false); err != nil {
				t.Fatal(err)
			}
		}
		if err := files.CopyTree(from, to); err != nil {
			t.Fatal(err)
		}
		for path, want := range map[string]bool{"a/b.py": true, "top.txt": true, "a/__pycache__/b.pyc": false, ".pytest_cache/n": false, "test-results/r.xml": false} {
			if got := files.Exists(filepath.Join(to, path)); got != want {
				t.Errorf("%s exists = %v, want %v", path, got, want)
			}
		}
	})

	t.Run("a missing file reads as nothing, and a made directory has its links resolved [verifies SP-1507~1]", func(t *testing.T) {
		data, err := files.Read(filepath.Join(t.TempDir(), "absent"))
		if data != nil || err != nil {
			t.Fatalf("read = %v, %v", data, err)
		}
		real := t.TempDir()
		link := filepath.Join(t.TempDir(), "link")
		if err := os.Symlink(real, link); err != nil {
			t.Fatal(err)
		}
		made, err := files.MakeDir(link, "run-")
		if err != nil {
			t.Fatal(err)
		}
		resolved, _ := filepath.EvalSymlinks(real)
		if !strings.HasPrefix(made, resolved) {
			t.Errorf("made %s, want it under %s", made, resolved)
		}
		matches, _ := files.Glob(filepath.Join(real, "run-*"))
		if len(matches) != 1 {
			t.Errorf("glob = %v", matches)
		}
	})
}
