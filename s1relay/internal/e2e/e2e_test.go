// Package e2e_test drives the built binaries of the relay and of the alignment
// module as black boxes: the module emits its closed requests, the relay drains
// them into a stub endpoint, and the module carries on with the answers. The
// alignment module is built from its own directory; this module imports none of
// its code.
package e2e_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/RachidChabane/spec-proof/s1relay/internal/domain/endpoint"
	"github.com/RachidChabane/spec-proof/s1relay/internal/testkit/stubendpoint"
)

const (
	alignmentBinary          = "spec-proof-alignment"
	relayBinary              = "spec-proof-s1"
	alignmentModule          = "../../../alignment"
	relayModule              = "../.."
	fixtureDir               = "../../testdata/alignment"
	projectDir               = "project"
	reportFile               = "shallnot-report.json"
	shallnotScript           = "shallnot-recorded.sh"
	recordedModel            = "tev1:0.8b"
	exitAwaitingHost         = 3
	exitToolFailure          = 2
	dirMode                  = 0o755
	fileMode                 = 0o644
	recordedShallnotVariable = "SPEC_PROOF_SHALLNOT"
	recordedReportVariable   = "SHALLNOT_RECORDED_REPORT"
)

var (
	buildOnce sync.Once
	binaries  string
	buildErr  error
)

// build compiles the two binaries once, one compiler job at a time.
func build(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		binaries, buildErr = os.MkdirTemp("", "spec-proof-s1-e2e-")
		if buildErr != nil {
			return
		}
		for module, binary := range map[string]string{alignmentModule: alignmentBinary, relayModule: relayBinary} {
			command := exec.Command("go", "build", "-p", "1", "-o", filepath.Join(binaries, binary), "./cmd/"+binary)
			command.Dir = module
			command.Env = append(os.Environ(), "GOMAXPROCS=2")
			if output, err := command.CombinedOutput(); err != nil {
				buildErr = &buildFailure{binary: binary, output: string(output), cause: err}
				return
			}
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return binaries
}

// TestMain removes the compiled binaries when the tests are done.
func TestMain(m *testing.M) {
	code := m.Run()
	if binaries != "" {
		os.RemoveAll(binaries)
	}
	os.Exit(code)
}

type buildFailure struct {
	binary, output string
	cause          error
}

func (b *buildFailure) Error() string {
	return "build " + b.binary + ": " + b.cause.Error() + "\n" + b.output
}

// project is a copy of the fixture project with the environment the alignment
// module needs, which the relay inherits.
type project struct {
	t    *testing.T
	root string
	env  []string
	bin  string
}

func newProject(t *testing.T) *project {
	t.Helper()
	bin := build(t)
	base := t.TempDir()
	root := filepath.Join(base, projectDir)
	copyTree(t, filepath.Join(fixtureDir, projectDir), root)
	script, err := filepath.Abs(filepath.Join(fixtureDir, shallnotScript))
	if err != nil {
		t.Fatal(err)
	}
	report, err := filepath.Abs(filepath.Join(fixtureDir, reportFile))
	if err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"XDG_STATE_HOME="+filepath.Join(base, "state"),
		"XDG_CACHE_HOME="+filepath.Join(base, "cache"),
		"HOME="+filepath.Join(base, "home"),
		recordedShallnotVariable+"="+script,
		recordedReportVariable+"="+report,
	)
	return &project{t: t, root: root, env: env, bin: bin}
}

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, dirMode)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, content, fileMode)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// run executes a binary of the build directory in the project and returns its
// exit code and standard output.
func (p *project) run(binary string, args ...string) (int, string, string) {
	p.t.Helper()
	command := exec.Command(filepath.Join(p.bin, binary), args...)
	command.Dir = p.root
	command.Env = p.env
	var stdout, stderr strings.Builder
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	code := 0
	if exitErr, isExit := err.(*exec.ExitError); isExit {
		code = exitErr.ExitCode()
	} else if err != nil {
		p.t.Fatalf("%s: %v", binary, err)
	}
	return code, stdout.String(), stderr.String()
}

func (p *project) exported() []struct{ Ticket string } {
	p.t.Helper()
	code, stdout, stderr := p.run(alignmentBinary, "systemone", "export")
	if code != 0 {
		p.t.Fatalf("systemone export: exit %d: %s", code, stderr)
	}
	var list []struct{ Ticket string }
	if err := json.Unmarshal([]byte(stdout), &list); err != nil {
		p.t.Fatalf("export %q: %v", stdout, err)
	}
	return list
}

// recordedUnder lists the files below dir that mention the text.
func recordedUnder(t *testing.T, dir, text string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(content), text) {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func TestDrainTheClosedRequestsOfTheAlignmentBinary(t *testing.T) {
	t.Run("the closed requests of the real alignment binary are answered by a stub endpoint and recorded under the relay source [verifies SP-615~1]", func(t *testing.T) {
		p := newProject(t)
		if code, _, stderr := p.run(alignmentBinary, "prove", "--all"); code != exitAwaitingHost {
			t.Fatalf("prove: exit %d, want %d: %s", code, exitAwaitingHost, stderr)
		}
		waiting := p.exported()
		if len(waiting) == 0 {
			t.Fatal("the alignment binary exported no closed request")
		}

		stub := stubendpoint.New(stubendpoint.Ollama, nil)
		defer stub.Close()
		code, stdout, stderr := p.run(relayBinary, "drain", "--module", "alignment", "--endpoint", stub.URL, "--model", recordedModel)
		if code != 0 {
			t.Fatalf("drain: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}

		var report struct {
			Answered       int    `json:"answered"`
			Unanswered     int    `json:"unanswered"`
			EndpointDigest string `json:"endpointDigest"`
		}
		if err := json.Unmarshal([]byte(stdout), &report); err != nil {
			t.Fatalf("report %q: %v", stdout, err)
		}
		address, err := endpoint.Parse(stub.URL)
		if err != nil {
			t.Fatal(err)
		}
		if report.Answered != len(waiting) || report.Unanswered != 0 || report.EndpointDigest != address.Digest() {
			t.Fatalf("report %+v for %d waiting requests", report, len(waiting))
		}
		if len(stub.Requests()) != len(waiting) || stub.MaxInFlight() != 1 || len(stub.Unloads()) != 1 {
			t.Fatalf("%d requests (%d waiting), %d at once, %d unloads", len(stub.Requests()), len(waiting), stub.MaxInFlight(), len(stub.Unloads()))
		}

		if left := p.exported(); len(left) != 0 {
			t.Fatalf("%d closed requests still wait for a first vote", len(left))
		}
		source := address.Source(recordedModel)
		if source != "system-one:"+recordedModel+"@"+address.Digest() {
			t.Fatalf("source %q is not system-one:<model>@<digest>", source)
		}
		stateDir := filepath.Join(filepath.Dir(p.root), "state")
		if recorded := recordedUnder(t, stateDir, source); len(recorded) < len(waiting) {
			t.Fatalf("%d records carry the source %s, want at least %d", len(recorded), source, len(waiting))
		}
		if code, _, stderr := p.run(alignmentBinary, "prove", "--all", "--resume"); code == exitToolFailure {
			t.Fatalf("prove --resume failed after the import: %s", stderr)
		}
	})

	t.Run("a relay whose endpoint is down leaves every closed request waiting for the host votes [verifies SP-615~1]", func(t *testing.T) {
		p := newProject(t)
		p.run(alignmentBinary, "prove", "--all")
		waiting := p.exported()
		stub := stubendpoint.New(stubendpoint.Ollama, nil)
		address := stub.URL
		stub.Close()
		code, stdout, _ := p.run(relayBinary, "drain", "--module", "alignment", "--endpoint", address, "--model", recordedModel, "--timeout", "2s")
		if code != 1 || !strings.Contains(stdout, `"kind":"transport"`) {
			t.Fatalf("drain: exit %d, stdout %s", code, stdout)
		}
		if left := p.exported(); len(left) != len(waiting) {
			t.Fatalf("%d requests wait after a failed drain, %d before", len(left), len(waiting))
		}
	})
}
