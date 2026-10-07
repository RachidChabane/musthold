package cli_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/cli"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	proveCommand   = "prove"
	allFlag        = "--all"
	changedFlag    = "--changed"
	resumeFlag     = "--resume"
	provedPass     = "proved PASS"
	provedBlock    = "proved BLOCK"
	provedEscalate = "proved ESCALATE"
	awaiting       = "awaiting_host"
	freshStatus    = "fresh"
)

// verdicts reads the `<criterion> <status> <result>` lines `prove` prints.
func verdicts(output string) map[string]string {
	found := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.Contains(fields[0], format.RevisionSeparator) && !strings.HasPrefix(line, " ") {
			found[fields[0]] = strings.Join(fields[1:], " ")
		}
	}
	return found
}

// findings reads the `finding <name>` lines printed under a criterion.
func findings(output, criterion string) []string {
	var names []string
	inCriterion := false
	for _, line := range strings.Split(output, "\n") {
		switch {
		case strings.HasPrefix(line, criterion+" "):
			inCriterion = true
		case !strings.HasPrefix(line, "  finding "):
			inCriterion = false
		case inCriterion:
			names = append(names, strings.TrimPrefix(line, "  finding "))
		}
	}
	return names
}

// drive runs `prove` and lets the persona answer the requests between waves, the
// way a host harness would, until `prove` stops waiting or the waves run out.
func (h *harness) drive(persona string, answers hostprototest.AnswerSource, extra ...string) (code int, output string) {
	h.t.Helper()
	args := append([]string{proveCommand, allFlag}, extra...)
	code, output, stderr := h.run(nil, args...)
	for wave := 0; code == cli.ExitAwaitingHost && wave < maxWavesPerDrive; wave++ {
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

// statement reads and validates the Statement written for a criterion.
func (h *harness) statement(criterion string) format.Statement[format.DiscriminatingPower] {
	h.t.Helper()
	directory := filepath.Join(h.root, filepath.FromSlash(statementsPattern), criterion)
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		h.t.Fatalf("Statements of %s: %v (%d files)", criterion, err, len(entries))
	}
	content, err := os.ReadFile(filepath.Join(directory, entries[0].Name()))
	if err != nil {
		h.t.Fatal(err)
	}
	statement, err := format.DecodeAs[format.DiscriminatingPower](content)
	if err != nil {
		h.t.Fatal(err)
	}
	if err := statement.Validate(); err != nil {
		h.t.Fatalf("the Statement of %s is invalid: %v", criterion, err)
	}
	return statement
}

// statementBytes returns the bytes of every Statement by criterion.
func (h *harness) statementBytes() map[string]string {
	h.t.Helper()
	found := map[string]string{}
	for _, path := range h.statements() {
		content, err := os.ReadFile(path)
		if err != nil {
			h.t.Fatal(err)
		}
		found[filepath.Base(filepath.Dir(path))] = string(content)
	}
	return found
}

// reversing lists the pending tickets in the opposite order, as another host
// would meet them.
type reversing struct{ *harness }

// Run implements hostprototest.ModuleRunner.
func (r reversing) Run(args []string, stdin []byte) ([]byte, error) {
	output, err := r.harness.Run(args, stdin)
	if err != nil || !slices.Equal(args, hostproto.ListArgs()) {
		return output, err
	}
	var listing hostproto.Listing
	if err := json.Unmarshal(output, &listing); err != nil {
		return nil, err
	}
	slices.Reverse(listing.Items)
	return json.Marshal(listing)
}

// driveReversed is drive with a host that answers in the opposite order.
func (h *harness) driveReversed(answers hostprototest.AnswerSource) (code int, output string) {
	h.t.Helper()
	args := []string{proveCommand, allFlag}
	code, output, _ = h.run(nil, args...)
	for wave := 0; code == cli.ExitAwaitingHost && wave < maxWavesPerDrive; wave++ {
		if _, err := hostprototest.PlayCLI(reversing{h}, hostprototest.PersonaHonest, answers, func(time.Duration) {}); err != nil {
			h.t.Fatal(err)
		}
		code, output, _ = h.run(nil, append(args, resumeFlag)...)
	}
	return code, output
}

// rewrite replaces a text in a file of the project.
func (h *harness) rewrite(file, from, to string) {
	h.t.Helper()
	path := filepath.Join(h.root, filepath.FromSlash(file))
	content, err := os.ReadFile(path)
	if err != nil {
		h.t.Fatal(err)
	}
	if !strings.Contains(string(content), from) {
		h.t.Fatalf("%s does not contain %q", file, from)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(content), from, to, 1)), fileMode); err != nil {
		h.t.Fatal(err)
	}
}

// useReport replaces the recorded Shallnot report by the report the edit makes.
func (h *harness) useReport(edit func(requirements []any)) {
	h.t.Helper()
	content, err := os.ReadFile(fixtureReport)
	if err != nil {
		h.t.Fatal(err)
	}
	var report map[string]any
	if err := json.Unmarshal(content, &report); err != nil {
		h.t.Fatal(err)
	}
	edit(report["requirements"].([]any))
	edited, err := json.Marshal(report)
	if err != nil {
		h.t.Fatal(err)
	}
	path := filepath.Join(h.t.TempDir(), "report.json")
	if err := os.WriteFile(path, edited, fileMode); err != nil {
		h.t.Fatal(err)
	}
	h.t.Setenv(reportVariable, path)
}

// mutantBodies lists the mutant bodies kept in the cache directory.
func (h *harness) mutantBodies() []string {
	var found []string
	_ = filepath.WalkDir(h.cache, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && filepath.Base(filepath.Dir(path)) == "mutants" {
			found = append(found, path)
		}
		return nil
	})
	return found
}

func TestProveEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the parcel project's tests against many mutants")
	}
	shared := t.TempDir()

	t.Run("prove waits for the host, repeats its requests and refuses to resume nothing [verifies SP-181~1]", func(t *testing.T) {
		h := newHarness(t, shared)
		if code, _, _ := h.run(nil, proveCommand, resumeFlag); code != cli.ExitToolFailure {
			t.Errorf("--resume before any request exited %d, want %d", code, cli.ExitToolFailure)
		}
		code, first, _ := h.run(nil, proveCommand, allFlag)
		_, listed, _ := h.run(nil, hostproto.ListArgs()...)
		code2, second, _ := h.run(nil, proveCommand, allFlag, resumeFlag)
		_, listedAgain, _ := h.run(nil, hostproto.ListArgs()...)
		if code != cli.ExitAwaitingHost || code2 != cli.ExitAwaitingHost {
			t.Fatalf("exit codes %d and %d, want %d", code, code2, cli.ExitAwaitingHost)
		}
		if !strings.Contains(first, "wave 1") || !strings.Contains(second, "wave 2") {
			t.Errorf("waves: %q then %q", firstLine(first), firstLine(second))
		}
		if requestIDs(t, listed) != requestIDs(t, listedAgain) {
			t.Error("running prove again changed the requests")
		}
		for ref, verdict := range verdicts(first) {
			if verdict != awaiting+" " && verdict != awaiting {
				t.Errorf("%s: %q while the host has not answered", ref, verdict)
			}
		}
	})

	t.Run("the honest persona blocks the weak test and passes the adequate ones [verifies SP-182~1]", func(t *testing.T) {
		h := newHarness(t, shared)
		code, output := h.drive(hostprototest.PersonaHonest, oracle{})
		want := map[string]string{"PAR-1~1": provedPass, "PAR-2~1": provedPass, "PAR-3~1": provedBlock}
		for ref, verdict := range want {
			if got := verdicts(output)[ref]; got != verdict {
				t.Errorf("%s: %q, want %q\n%s", ref, got, verdict, output)
			}
		}
		if code != cli.ExitBlocked {
			t.Errorf("exit code %d, want %d", code, cli.ExitBlocked)
		}
		blocked := h.statement("PAR-3~1")
		corroborated := false
		for _, mutant := range blocked.Predicate.Mutants {
			if mutant.Validity.Value != format.ValidityValid {
				continue
			}
			kinds := map[format.CorroborationKind]bool{}
			for _, evidence := range mutant.Validity.CorroboratedBy {
				kinds[evidence.Kind] = true
			}
			survived := !slices.ContainsFunc(mutant.Results, func(r format.TestResult) bool { return r.Outcome.Value == format.OutcomeKilled })
			corroborated = corroborated || (survived && kinds[format.CorroborationWitness] && kinds[format.CorroborationBuilds] && kinds[format.CorroborationOtherCriteriaPass])
		}
		if !corroborated {
			t.Error("the block rests on no surviving VALID mutant with a confirmed witness, a build and the other criteria's tests")
		}
		if blocked.Predicate.Executor.Isolation != format.IsolationInline {
			t.Errorf("executor isolation %q, want inline for a command-line host", blocked.Predicate.Executor.Isolation)
		}
	})

	t.Run("the other personas never yield a pass [verifies SP-183~1]", func(t *testing.T) {
		for _, persona := range []string{hostprototest.PersonaForger, hostprototest.PersonaOmitter, hostprototest.PersonaStale, hostprototest.PersonaBreaker} {
			t.Run(persona, func(t *testing.T) {
				h := newHarness(t, shared)
				code, output := h.drive(persona, hostprototest.SchemaAnswers{})
				if code == cli.ExitClean {
					t.Errorf("exit code %d under the %s persona", code, persona)
				}
				for ref, verdict := range verdicts(output) {
					if strings.Contains(verdict, "PASS") {
						t.Errorf("%s passed under the %s persona: %s", ref, persona, verdict)
					}
				}
			})
		}
	})

	t.Run("a Statement is validated, written under the cache directory and its mutant bodies stay outside the work tree [verifies SP-189~1]", func(t *testing.T) {
		h := newHarness(t, shared)
		h.drive(hostprototest.PersonaHonest, oracle{})
		if len(h.statements()) != 3 {
			t.Fatalf("%d Statements written, want 3", len(h.statements()))
		}
		for _, criterion := range []string{"PAR-1~1", "PAR-2~1", "PAR-3~1"} {
			statement := h.statement(criterion)
			if statement.Subject[0].Name != criterion {
				t.Errorf("subject %q, want %q", statement.Subject[0].Name, criterion)
			}
		}
		if len(h.mutantBodies()) == 0 {
			t.Error("no mutant body was kept in the cache directory")
		}
		_ = filepath.WalkDir(h.root, func(path string, entry fs.DirEntry, err error) error {
			if err == nil && !entry.IsDir() && strings.Contains(path, "mutants") {
				t.Errorf("a mutant body lies in the work tree: %s", path)
			}
			return nil
		})
	})

	t.Run("answers consumed in another order give the same Statements [verifies SP-185~1]", func(t *testing.T) {
		first := newHarness(t, shared)
		first.drive(hostprototest.PersonaHonest, oracle{})
		second := newHarness(t, shared)
		second.now = second.now.Add(24 * time.Hour)
		second.driveReversed(oracle{})
		want, got := first.statementBytes(), second.statementBytes()
		if len(want) != 3 {
			t.Fatalf("%d Statements, want 3", len(want))
		}
		for criterion, content := range want {
			if got[criterion] != content {
				t.Errorf("the Statement of %s differs when the answers come in another order", criterion)
			}
		}
	})

	t.Run("recorded answers replay to the same Statements and a missing recording is a hard error [verifies SP-180~1]", func(t *testing.T) {
		recordings := t.TempDir()
		recorded := newHarness(t, shared)
		recorded.drive(hostprototest.PersonaHonest, oracle{}, "--record", recordings)
		want := recorded.statementBytes()
		replayed := newHarness(t, shared)
		code, output, stderr := replayed.run(nil, proveCommand, allFlag, "--replay", recordings)
		if code != cli.ExitBlocked || verdicts(output)["PAR-3~1"] != provedBlock {
			t.Fatalf("replay exited %d: %s\n%s", code, output, stderr)
		}
		for criterion, content := range want {
			if got := replayed.statementBytes()[criterion]; got != content {
				t.Errorf("the replayed Statement of %s differs from the recorded run", criterion)
			}
		}
		missing := newHarness(t, shared)
		if code, _, stderr := missing.run(nil, proveCommand, allFlag, "--replay", t.TempDir()); code != cli.ExitToolFailure || !strings.Contains(stderr, "no recorded answer") {
			t.Errorf("a replay miss exited %d: %s", code, stderr)
		}
	})

	t.Run("a criterion whose tickets run out escalates and is never passed [verifies SP-187~1]", func(t *testing.T) {
		h := newHarness(t, shared)
		h.rewrite(".spec-proof/config.yaml", "operations:", "policy:\n  max_host_tickets: 1\noperations:")
		_, output := h.drive(hostprototest.PersonaHonest, oracle{})
		if got := verdicts(output)["PAR-3~1"]; got != provedEscalate {
			t.Errorf("PAR-3~1: %q, want %q\n%s", got, provedEscalate, output)
		}
		if !slices.Contains(findings(output, "PAR-3~1"), "budget_exhausted") {
			t.Errorf("PAR-3~1 findings %v, want budget_exhausted", findings(output, "PAR-3~1"))
		}
		undetermined := false
		for _, mutant := range h.statement("PAR-3~1").Predicate.Mutants {
			undetermined = undetermined || (mutant.Extensions != nil && mutant.Extensions.Decision != nil && mutant.Extensions.Decision.EscalationReason == "budget_exhausted")
		}
		if !undetermined {
			t.Error("no mutant records the reason budget_exhausted")
		}
		for ref, verdict := range verdicts(output) {
			if ref == "PAR-3~1" && strings.Contains(verdict, "PASS") {
				t.Errorf("%s passed with no tickets left", ref)
			}
		}
	})

	t.Run("an unchanged criterion is reported fresh and not proved again [verifies SP-188~1]", func(t *testing.T) {
		h := newHarness(t, shared)
		h.drive(hostprototest.PersonaHonest, oracle{})
		before := h.statementBytes()
		code, output, _ := h.run(nil, proveCommand, changedFlag)
		for ref, verdict := range verdicts(output) {
			if !strings.HasPrefix(verdict, freshStatus) {
				t.Errorf("%s: %q, want fresh", ref, verdict)
			}
		}
		if verdicts(output)["PAR-3~1"] != freshStatus+" BLOCK" || code != cli.ExitBlocked {
			t.Errorf("the stored outcome was not reported: %q, exit %d", verdicts(output)["PAR-3~1"], code)
		}
		after := h.statementBytes()
		for criterion, content := range before {
			if after[criterion] != content {
				t.Errorf("the Statement of %s was written again", criterion)
			}
		}
	})
}

func TestProveCleanRun(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the parcel project's tests against many mutants")
	}
	h := newHarness(t, t.TempDir())
	h.rewrite("tests/test_rates.py", "assert shipping_fee(100) == 0", "assert shipping_fee(100) == 0 and shipping_fee(50) == 0")
	h.rewrite("tests/test_rates.py", "assert shipping_fee(10) == 4.5", "assert shipping_fee(10) == 4.5 and shipping_fee(49.99) == 4.5")
	h.useReport(func(requirements []any) {
		third := requirements[2].(map[string]any)
		third["examples"] = []any{map[string]any{
			"index": 0, "operation": "shipping_fee",
			"args":   map[string]any{"total": map[string]any{"type": "integer", "value": "50"}},
			"expect": map[string]any{"kind": "returns", "type": "integer", "value": "0"},
		}}
	})
	code, output := h.drive(hostprototest.PersonaHonest, oracle{})
	for _, ref := range []string{"PAR-1~1", "PAR-2~1", "PAR-3~1"} {
		if got := verdicts(output)[ref]; got != provedPass {
			t.Errorf("%s: %q, want %q\n%s", ref, got, provedPass, output)
		}
	}
	t.Run("a run whose criteria all pass exits clean [verifies SP-181~1]", func(t *testing.T) {
		if code != cli.ExitClean {
			t.Errorf("exit code %d, want %d", code, cli.ExitClean)
		}
	})
	if code != cli.ExitClean {
		for _, mutant := range h.statement("PAR-3~1").Predicate.Mutants {
			t.Logf("%s %v %v", mutant.Digest[:8], mutant.Validity.Value, mutant.Extensions.Decision)
			for _, body := range h.mutantBodies() {
				if strings.Contains(body, mutant.Digest) {
					content, _ := os.ReadFile(body)
					t.Logf("%s", content)
				}
			}
		}
	}
}

func firstLine(text string) string { return strings.SplitN(text, "\n", 2)[0] }

func requestIDs(t *testing.T, listing string) string {
	t.Helper()
	var parsed hostproto.Listing
	if err := json.Unmarshal([]byte(listing), &parsed); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, len(parsed.Items))
	for index, item := range parsed.Items {
		ids[index] = item.Ticket.TicketID.String()
	}
	slices.Sort(ids)
	return strings.Join(ids, ",")
}
