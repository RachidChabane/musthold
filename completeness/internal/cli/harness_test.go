package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/hoststore"
	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/xdg"
	"github.com/RachidChabane/spec-proof/completeness/internal/cli"
	"github.com/RachidChabane/spec-proof/completeness/internal/testkit"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	reportVariable   = "SHALLNOT_RECORDED_REPORT"
	shallnotVariable = "SPEC_PROOF_SHALLNOT"
	stateVariable    = "XDG_STATE_HOME"
	cacheVariable    = "XDG_CACHE_HOME"
	maxWavesPerDrive = 12
	binaryName       = "spec-proof-completeness"
	proveCommand     = "prove"
	allFlag          = "--all"
	changedFlag      = "--changed"
	resumeFlag       = "--resume"
	completenessDir  = ".spec-proof/cache/completeness"
	configFile       = ".spec-proof/completeness.yaml"
	fileMode         = 0o644
	directoryMode    = 0o755
)

// harness runs the command line in process against a copy of a fixture project
// kept in a repository, with the recorded Shallnot report, and state and cache
// directories of its own.
type harness struct {
	t       *testing.T
	root    string
	state   string
	cache   string
	report  string
	now     time.Time
	entropy io.Reader
	// admission is the table of classifications the binary answers with; nil keeps
	// the shipped, empty one.
	admission *hostproto.ClassifyAdmission
	// waves bounds how many times drive lets the host answer; zero means the default.
	waves int
	// reverse makes the host meet the pending tickets in the opposite order.
	reverse bool
}

func newHarness(t *testing.T, fixture string) *harness {
	t.Helper()
	base := t.TempDir()
	return &harness{
		t: t, root: testkit.Materialize(t, fixture), cache: filepath.Join(base, "cache"), state: filepath.Join(base, "state"),
		report: testkit.ReportPath(fixture), now: time.Date(2026, time.October, 7, 12, 0, 0, 0, time.UTC),
	}
}

func (h *harness) write(relative, content string) {
	h.t.Helper()
	path := filepath.Join(h.root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), directoryMode); err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), fileMode); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) withConfig(content string) {
	h.t.Helper()
	h.write(configFile, content)
}

// withEvidence writes the Statements the other modules would have written.
func (h *harness) withEvidence(power testkit.Power, aligned ...testkit.Aligned) {
	h.t.Helper()
	power.Write(h.t, h.root)
	for _, item := range aligned {
		item.Write(h.t, h.root)
	}
}

func (h *harness) env(stdin []byte) (cli.Env, *bytes.Buffer, *bytes.Buffer) {
	shallnot, err := filepath.Abs(filepath.Join(testkit.FixturesDir(), testkit.RecordedShallnot))
	if err != nil {
		h.t.Fatal(err)
	}
	variables := map[string]string{stateVariable: h.state, cacheVariable: h.cache, shallnotVariable: shallnot}
	var stdout, stderr bytes.Buffer
	return cli.Env{
		Binary: binaryName, Stdin: bytes.NewReader(stdin), Stdout: &stdout, Stderr: &stderr,
		WorkDir: h.root, HomeDir: h.state,
		LookupEnv:         func(name string) (string, bool) { value, found := variables[name]; return value, found },
		Now:               func() time.Time { return h.now },
		Entropy:           h.entropy,
		ClassifyAdmission: h.admission,
	}, &stdout, &stderr
}

// run executes the command line and returns the exit code and the output.
func (h *harness) run(stdin []byte, args ...string) (code int, stdout, stderr string) {
	h.t.Helper()
	h.t.Setenv(reportVariable, h.report)
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

func (h *harness) waveLimit() int {
	if h.waves > 0 {
		return h.waves
	}
	return maxWavesPerDrive
}

// drive runs `prove` and lets the persona answer the requests between waves, the
// way a host harness would, until `prove` stops waiting or the waves run out.
func (h *harness) drive(persona string, answers hostprototest.AnswerSource, extra ...string) (code int, output string) {
	h.t.Helper()
	args := append([]string{proveCommand, allFlag}, extra...)
	code, output, stderr := h.run(nil, args...)
	for wave := 0; code == cli.ExitAwaitingHost && wave < h.waveLimit(); wave++ {
		if _, err := hostprototest.PlayCLI(h, persona, answers, func(time.Duration) {}); err != nil && persona == hostprototest.PersonaHonest {
			h.t.Fatalf("the honest persona failed: %v", err)
		}
		code, output, stderr = h.run(nil, append(args, resumeFlag)...)
	}
	if code == cli.ExitToolFailure {
		h.t.Fatalf("prove failed: %s", stderr)
	}
	return code, output
}

// serve answers every pending ticket as an honest executor would, in the order
// the harness is set to meet them.
func (h *harness) serve(answers hostprototest.AnswerSource) {
	h.t.Helper()
	items := h.requestsJSON().Items
	if h.reverse {
		slices.Reverse(items)
	}
	for _, item := range items {
		answer, err := answers.Answer(hostprototest.PersonaHonest, item.View)
		if err != nil {
			h.t.Fatal(err)
		}
		if code, _, stderr := h.run(nil, hostproto.CommandRequest, hostproto.SubcommandShow, item.Ticket.TicketID.String()); code != cli.ExitClean {
			h.t.Fatalf("show %s: %s", item.Ticket.TicketID, stderr)
		}
		args := hostproto.SubmitArgs(item.Ticket.TicketID, item.Ticket.Nonce)
		if code, _, stderr := h.run([]byte(answer), args...); code != cli.ExitClean {
			h.t.Fatalf("submit %s: %s", item.Ticket.TicketID, stderr)
		}
	}
}

// driveInOrder runs `prove` and answers the pending tickets in the harness's
// order between waves until `prove` stops waiting or the waves run out.
func (h *harness) driveInOrder(answers hostprototest.AnswerSource, extra ...string) (code int, output string) {
	h.t.Helper()
	args := append([]string{proveCommand, allFlag}, extra...)
	code, output, stderr := h.run(nil, args...)
	for wave := 0; code == cli.ExitAwaitingHost && wave < h.waveLimit(); wave++ {
		h.serve(answers)
		code, output, stderr = h.run(nil, append(args, resumeFlag)...)
	}
	if code == cli.ExitToolFailure {
		h.t.Fatalf("prove failed: %s", stderr)
	}
	return code, output
}

func (h *harness) store() *hoststore.Store {
	h.t.Helper()
	root, err := filepath.Abs(h.root)
	if err != nil {
		h.t.Fatal(err)
	}
	lookup := func(name string) (string, bool) { return h.state, name == stateVariable }
	store, err := hoststore.Open(xdg.HostDir(lookup, h.state, root))
	if err != nil {
		h.t.Fatal(err)
	}
	return store
}

func (h *harness) requestsJSON() hostproto.Listing {
	h.t.Helper()
	output, err := h.Run(hostproto.ListArgs(hostproto.SessionProve), nil)
	if err != nil {
		h.t.Fatal(err)
	}
	var listing hostproto.Listing
	if err := json.Unmarshal(output, &listing); err != nil {
		h.t.Fatal(err)
	}
	return listing
}

// statementBytes returns the bytes of every Statement by its path under the cache.
func (h *harness) statementBytes() map[string]string {
	h.t.Helper()
	found := map[string]string{}
	root := filepath.Join(h.root, filepath.FromSlash(completenessDir))
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		relative, _ := filepath.Rel(root, path)
		found[relative] = string(content)
		return nil
	})
	return found
}

// statement reads the Statement of the fixture spec, validated.
func (h *harness) statement() (format.Statement[format.Completeness], bool) {
	h.t.Helper()
	var found format.Statement[format.Completeness]
	ok := false
	for _, content := range h.statementBytes() {
		statement, err := format.DecodeAs[format.Completeness]([]byte(content))
		if err != nil {
			h.t.Fatal(err)
		}
		if err := statement.Validate(); err != nil {
			h.t.Fatalf("the Statement is invalid: %v", err)
		}
		found, ok = statement, true
	}
	return found, ok
}

func (h *harness) mustStatement() format.Statement[format.Completeness] {
	h.t.Helper()
	statement, ok := h.statement()
	if !ok {
		h.t.Fatal("no Statement was written")
	}
	return statement
}

// pendingFile reads the pending file of the module, empty when there is none.
func (h *harness) pendingFile() format.Pending {
	h.t.Helper()
	content, err := os.ReadFile(filepath.Join(h.root, ".spec-proof", "pending", "completeness.json"))
	if os.IsNotExist(err) {
		return format.Pending{Module: "completeness"}
	}
	if err != nil {
		h.t.Fatal(err)
	}
	pending, err := format.DecodePending(content)
	if err != nil {
		h.t.Fatal(err)
	}
	return pending
}

func (h *harness) journal() []hostproto.Record {
	h.t.Helper()
	records, err := h.store().Records()
	if err != nil {
		h.t.Fatal(err)
	}
	return records
}

// questionLines lists the questions of a Statement as `dimension: text` lines.
func questionLines(statement format.Statement[format.Completeness]) []string {
	lines := make([]string, len(statement.Predicate.Questions))
	for index, q := range statement.Predicate.Questions {
		lines[index] = q.Dimension + ": " + q.Question
	}
	return lines
}

// questionsOf lists the questions of a dimension.
func questionsOf(statement format.Statement[format.Completeness], dimension string) []format.Question {
	var found []format.Question
	for _, q := range statement.Predicate.Questions {
		if q.Dimension == dimension {
			found = append(found, q)
		}
	}
	return found
}
