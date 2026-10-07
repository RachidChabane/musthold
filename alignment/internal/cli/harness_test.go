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

	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/hoststore"
	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/xdg"
	"github.com/RachidChabane/spec-proof/alignment/internal/cli"
	"github.com/RachidChabane/spec-proof/alignment/internal/testkit"
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
	directoryMode    = 0o755
	fileMode         = 0o644
	binaryName       = "spec-proof-alignment"
	proveCommand     = "prove"
	allFlag          = "--all"
	changedFlag      = "--changed"
	resumeFlag       = "--resume"
	alignmentCache   = ".spec-proof/cache/alignment"
	configFile       = ".spec-proof/alignment.yaml"
)

// harness runs the command line in process against a copy of a fixture project,
// with the recorded Shallnot report, and state and cache directories of its own.
type harness struct {
	t       *testing.T
	root    string
	state   string
	cache   string
	report  string
	now     time.Time
	entropy io.Reader
	// waves bounds how many times drive lets the host answer; zero means the default.
	waves int
	// reverse makes the host meet the pending tickets in the opposite order.
	reverse bool
}

// newHarness copies the fixture project into a temporary directory.
func newHarness(t *testing.T, fixture string) *harness {
	t.Helper()
	base := t.TempDir()
	h := &harness{
		t: t, root: filepath.Join(base, "project"), cache: filepath.Join(base, "cache"), state: filepath.Join(base, "state"),
		report: filepath.Join(testkit.FixturesDir(), fixture, testkit.ReportFile),
		now:    time.Date(2026, time.October, 7, 12, 0, 0, 0, time.UTC),
	}
	copyTree(t, filepath.Join(testkit.FixturesDir(), fixture), h.root)
	if err := os.Remove(filepath.Join(h.root, testkit.ReportFile)); err != nil {
		t.Fatal(err)
	}
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

// useReport serves another recorded report than the fixture's.
func (h *harness) useReport(path string) { h.report = path }

// withConfig writes the module's configuration file.
func (h *harness) withConfig(content string) {
	h.t.Helper()
	h.write(configFile, content)
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

// withPowers writes the discriminating-power Statements into the local cache.
func (h *harness) withPowers(powers []testkit.Power) {
	h.t.Helper()
	for _, power := range powers {
		power.Write(h.t, h.root)
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
		LookupEnv: func(name string) (string, bool) { value, found := variables[name]; return value, found },
		Now:       func() time.Time { return h.now },
		Entropy:   h.entropy,
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

// drive runs `prove` and lets the persona answer the requests between waves, the
// way a host harness would, until `prove` stops waiting or the waves run out.
func (h *harness) drive(persona string, answers hostprototest.AnswerSource, extra ...string) (code int, output string) {
	h.t.Helper()
	args := append([]string{proveCommand, allFlag}, extra...)
	code, output, stderr := h.run(nil, args...)
	for wave := 0; code == cli.ExitAwaitingHost && wave < h.waveLimit(); wave++ {
		_, err := hostprototest.PlayCLI(h, persona, answers, func(time.Duration) {})
		if err != nil && persona == hostprototest.PersonaHonest {
			h.t.Fatalf("the honest persona failed: %v", err)
		}
		code, output, stderr = h.run(nil, append(args, resumeFlag)...)
	}
	if code == cli.ExitToolFailure {
		h.t.Fatalf("prove failed: %s", stderr)
	}
	return code, output
}

// store opens the host store of the harness's project.
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

// attest makes the host start an executor agent for the ticket's role, as the
// hooks of a harness would, and returns the capability it holds.
func (h *harness) attest(item hostproto.ListingItem, agent string) string {
	h.t.Helper()
	store := h.store()
	if _, err := store.Append(hostproto.Event{Kind: hostproto.EventAgentStarted, AgentID: agent, Role: item.Role}); err != nil {
		h.t.Fatal(err)
	}
	secret, err := store.Secret()
	if err != nil {
		h.t.Fatal(err)
	}
	capability, err := hostproto.FormatCap(secret, agent)
	if err != nil {
		h.t.Fatal(err)
	}
	return capability
}

// serveAttested answers every pending ticket the way a plugin executor does: an
// agent started for the role fetches the ticket and submits with its capability.
func (h *harness) serveAttested(answers hostprototest.AnswerSource) {
	h.t.Helper()
	items := h.requestsJSON().Items
	if h.reverse {
		slices.Reverse(items)
	}
	for _, item := range items {
		capability := h.attest(item, "agent-"+strings.ReplaceAll(item.Ticket.TicketID.String(), ".", "-"))
		answer, err := answers.Answer(hostprototest.PersonaHonest, item.View)
		if err != nil {
			h.t.Fatal(err)
		}
		if code, _, stderr := h.run(nil, hostproto.CommandRequest, hostproto.SubcommandShow, item.Ticket.TicketID.String(), hostproto.FlagCap, capability); code != cli.ExitClean {
			entry, _ := h.store().GetEntry(item.Ticket.TicketID.Key())
			h.t.Fatalf("show %s: %s (state %s cap %q, mine %q)", item.Ticket.TicketID, stderr, entry.State, entry.CapDigest, hostproto.CapDigest(capability))
		}
		args := append(hostproto.SubmitArgs(item.Ticket.TicketID, item.Ticket.Nonce), hostproto.FlagCap, capability)
		if code, _, stderr := h.run([]byte(answer), args...); code != cli.ExitClean {
			h.t.Fatalf("submit %s: %s", item.Ticket.TicketID, stderr)
		}
	}
}

// driveAttested runs `prove` and answers the pending tickets as attested executors
// between waves until `prove` stops waiting or the waves run out.
func (h *harness) driveAttested(answers hostprototest.AnswerSource, extra ...string) (code int, output string) {
	h.t.Helper()
	args := []string{proveCommand}
	if !slices.Contains(extra, changedFlag) {
		args = append(args, allFlag)
	}
	args = append(args, extra...)
	code, output, stderr := h.run(nil, args...)
	for wave := 0; code == cli.ExitAwaitingHost && wave < h.waveLimit(); wave++ {
		h.serveAttested(answers)
		code, output, stderr = h.run(nil, append(args, resumeFlag)...)
	}
	if code == cli.ExitToolFailure {
		h.t.Fatalf("prove failed: %s", stderr)
	}
	return code, output
}

func (h *harness) waveLimit() int {
	if h.waves > 0 {
		return h.waves
	}
	return maxWavesPerDrive
}

// result is one line of the output of `prove`.
type result struct {
	Criterion string
	Test      string
	Status    string
	Verdict   string
	Reason    string
}

// results reads the `<criterion> <test> <status> <verdict> [reason]` lines `prove` prints.
func results(output string) map[string]result {
	found := map[string]result{}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || !strings.Contains(fields[0], format.RevisionSeparator) || strings.HasPrefix(fields[0], "awaiting") {
			continue
		}
		entry := result{Criterion: fields[0], Test: fields[1], Status: fields[2], Verdict: fields[3]}
		if len(fields) > 4 {
			entry.Reason = fields[4]
		}
		found[entry.Criterion+" "+entry.Test] = entry
	}
	return found
}

// statements reads every alignment Statement written, validated, by criterion and test.
func (h *harness) statements() map[string]format.Statement[format.Alignment] {
	h.t.Helper()
	found := map[string]format.Statement[format.Alignment]{}
	root := filepath.Join(h.root, filepath.FromSlash(alignmentCache))
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		statement, err := format.DecodeAs[format.Alignment](content)
		if err != nil {
			return err
		}
		if err := statement.Validate(); err != nil {
			h.t.Errorf("the Statement at %s is invalid: %v", path, err)
		}
		found[statement.Subject[0].Name+" "+statement.Predicate.Test] = statement
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		h.t.Fatal(err)
	}
	return found
}

// statementBytes returns the bytes of every Statement by its path under the cache.
func (h *harness) statementBytes() map[string]string {
	h.t.Helper()
	found := map[string]string{}
	root := filepath.Join(h.root, filepath.FromSlash(alignmentCache))
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

// pendingFile reads the pending file of the module, empty when there is none.
func (h *harness) pendingFile() format.Pending {
	h.t.Helper()
	content, err := os.ReadFile(filepath.Join(h.root, ".spec-proof", "pending", "alignment.json"))
	if os.IsNotExist(err) {
		return format.Pending{Module: "alignment"}
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

// journal returns the events journaled for the harness's project.
func (h *harness) journal() []hostproto.Record {
	h.t.Helper()
	records, err := h.store().Records()
	if err != nil {
		h.t.Fatal(err)
	}
	return records
}

func (h *harness) requestsJSON() hostproto.Listing {
	h.t.Helper()
	output, err := h.Run(hostproto.ListArgs(), nil)
	if err != nil {
		h.t.Fatal(err)
	}
	var listing hostproto.Listing
	if err := json.Unmarshal(output, &listing); err != nil {
		h.t.Fatal(err)
	}
	return listing
}
