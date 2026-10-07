package cli_test

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/cli"
)

const (
	fixtureProject    = "../../fixtures/prove/parcel"
	fixtureReport     = "../../fixtures/prove/parcel-report.json"
	fixtureShallnot   = "../../fixtures/prove/shallnot-recorded.sh"
	reportVariable    = "SHALLNOT_RECORDED_REPORT"
	shallnotVariable  = "SPEC_PROOF_SHALLNOT"
	stateVariable     = "XDG_STATE_HOME"
	cacheVariable     = "XDG_CACHE_HOME"
	maxWavesPerDrive  = 8
	directoryMode     = 0o755
	fileMode          = 0o644
	binaryName        = "spec-proof-dp"
	statementsPattern = ".spec-proof/cache/discriminating-power"
)

// harness runs the command line in process against a copy of the parcel project,
// with the recorded Shallnot report, and state and cache directories of its own.
type harness struct {
	t       *testing.T
	root    string
	state   string
	cache   string
	now     time.Time
	entropy io.Reader
	// waves bounds how many times drive lets the host answer; zero means the default.
	waves int
}

// newHarness copies the fixture project into a temporary directory. A shared base
// directory gives every harness built on it the same project path and the same
// cache, so that they reuse the facts executed by the first one; the host state
// is always the harness's own.
func newHarness(t *testing.T, sharedBase string) *harness {
	t.Helper()
	absolute := func(path string) string {
		resolved, err := filepath.Abs(path)
		if err != nil {
			t.Fatal(err)
		}
		return resolved
	}
	t.Setenv(reportVariable, absolute(fixtureReport))
	base := sharedBase
	if base == "" {
		base = t.TempDir()
	}
	// The command line resolves symbolic links in the project root, so the
	// project is placed at its resolved path.
	base, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{
		t: t, root: filepath.Join(base, "parcel"), cache: filepath.Join(base, "cache"), state: t.TempDir(),
		now: time.Date(2026, time.October, 7, 12, 0, 0, 0, time.UTC),
	}
	if err := os.RemoveAll(h.root); err != nil {
		t.Fatal(err)
	}
	copyTree(t, fixtureProject, h.root)
	return h
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
			if entry.Name() == "__pycache__" {
				return fs.SkipDir
			}
			return os.MkdirAll(target, directoryMode)
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

func (h *harness) env(stdin []byte) (cli.Env, *bytes.Buffer, *bytes.Buffer) {
	shallnot, err := filepath.Abs(fixtureShallnot)
	if err != nil {
		h.t.Fatal(err)
	}
	variables := map[string]string{stateVariable: h.state, cacheVariable: h.cache, shallnotVariable: shallnot}
	var stdout, stderr bytes.Buffer
	return cli.Env{
		Binary: binaryName, Stdin: bytes.NewReader(stdin), Stdout: &stdout, Stderr: &stderr,
		WorkDir: h.root, HomeDir: h.state,
		LookupEnv: func(name string) (string, bool) {
			if value, found := variables[name]; found {
				return value, true
			}
			if strings.HasPrefix(name, "SPEC_PROOF_CI_") {
				return os.LookupEnv(name)
			}
			return "", false
		},
		Now:     func() time.Time { return h.now },
		Entropy: h.entropy,
	}, &stdout, &stderr
}

// run executes the command line and returns the exit code and the output.
func (h *harness) run(stdin []byte, args ...string) (code int, stdout, stderr string) {
	h.t.Helper()
	env, out, errOut := h.env(stdin)
	code = cli.Run(env, args)
	return code, out.String(), errOut.String()
}

// Run implements hostprototest.ModuleRunner: it runs a command and fails when the
// command does.
func (h *harness) Run(args []string, stdin []byte) ([]byte, error) {
	code, stdout, stderr := h.run(stdin, args...)
	if code != cli.ExitClean {
		return []byte(stdout), &commandError{code: code, stderr: strings.TrimSpace(stderr)}
	}
	return []byte(stdout), nil
}

type commandError struct {
	code   int
	stderr string
}

func (e *commandError) Error() string { return e.stderr }

// statements lists the Statement files written for the project.
func (h *harness) statements() []string {
	var found []string
	root := filepath.Join(h.root, filepath.FromSlash(statementsPattern))
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			found = append(found, path)
		}
		return nil
	})
	return found
}
