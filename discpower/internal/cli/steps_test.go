package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/hoststore"
	"github.com/RachidChabane/musthold/discpower/internal/cli"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
	"strconv"
	"time"
)

// Requirements the bounded steps verify.
const (
	verifiesStepBound   = " [verifies SP-2670~1]"
	verifiesAbandonment = " [verifies SP-2673~1]"
	verifiesStepOutput  = " [verifies SP-2674~1]"
	verifiesAnswerTally = " [verifies SP-2675~1]"
	verifiesBusyStep    = " [verifies SP-2672~1]"
	verifiesCutFacts    = " [verifies SP-2671~1]"

	firstStepBudget = 2 * time.Second
	maxSteps        = 400

	unfinishedSpent = "unfinished budget_spent"
	unfinishedBusy  = "unfinished session_busy"
	freshLine       = " fresh "
	pendingPath     = ".musthold/pending/discpower.json"
)

// progress runs `request progress --json` for the prove session.
func (h *harness) progress(since int) hostproto.Progress {
	h.t.Helper()
	output, err := h.Run(hostproto.ProgressArgs(hostproto.OnlySession(hostproto.SessionProve), since), nil)
	if err != nil {
		h.t.Fatal(err)
	}
	var progress hostproto.Progress
	if err := json.Unmarshal(output, &progress); err != nil {
		h.t.Fatal(err)
	}
	return progress
}

// tally reads the count of answers the pending file keeps and whether the file exists.
func (h *harness) tally() (format.Pending, bool) {
	h.t.Helper()
	content, err := os.ReadFile(filepath.Join(h.root, filepath.FromSlash(pendingPath)))
	if os.IsNotExist(err) {
		return format.Pending{}, false
	}
	if err != nil {
		h.t.Fatal(err)
	}
	pending, err := format.DecodePending(content)
	if err != nil {
		h.t.Fatal(err)
	}
	return pending, true
}

func TestBoundedSteps(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the parcel project's tests against many mutants")
	}
	shared := t.TempDir()

	t.Run("a step whose budget ran out in its executions keeps what is stored, writes nothing, exits 3, and the next call continues"+verifiesStepBound, func(t *testing.T) {
		h := newHarness(t, "")
		if code, first, _ := h.run(nil, proveCommand, allFlag); code != cli.ExitAwaitingHost {
			t.Fatalf("exit code %d:\n%s", code, first)
		}
		if _, err := hostprototest.PlayCLI(h, hostprototest.PersonaHonest, oracle{}, func(time.Duration) {}); err != nil {
			t.Fatal(err)
		}
		statements := len(h.statements())
		journal := len(h.journal())
		cut := ""
		for budget := firstStepBudget / 4; cut == "" && budget <= firstStepBudget*4; budget *= 2 {
			ctx, cancel := context.WithTimeoutCause(context.Background(), budget, hostproto.ErrStepSpent)
			h.ctx = ctx
			code, output, stderr := h.run(nil, proveCommand, allFlag, resumeFlag)
			cancel()
			h.ctx = nil
			switch {
			case code == cli.ExitAwaitingHost && strings.Contains(output, unfinishedSpent):
				cut = output
			case code == cli.ExitToolFailure && strings.Contains(stderr, "host.step_seconds"):
			default:
				t.Skipf("the step ended before its budget ran out (exit %d), so it cannot be cut here", code)
			}
		}
		if cut == "" {
			t.Fatal("no budget cut the step in its executions")
		}
		if len(h.statements()) != statements || len(h.journal()) < journal {
			t.Fatalf("the cut step wrote %d Statements over %d, journal %d then %d", len(h.statements()), statements, journal, len(h.journal()))
		}
		code, again, stderr := h.run(nil, proveCommand, allFlag, resumeFlag)
		if code == cli.ExitToolFailure || strings.Contains(again, unfinishedSpent) {
			t.Fatalf("the next call: exit %d:\n%s%s", code, again, stderr)
		}
	})

	t.Run("a step whose session another run holds changes nothing and exits 3"+verifiesBusyStep, func(t *testing.T) {
		h := newHarness(t, shared)
		release, err := h.store().LockRun(hostproto.SessionProve)
		if err != nil {
			t.Fatal(err)
		}
		code, busy, _ := h.run(nil, proveCommand, allFlag)
		release()
		if code != cli.ExitAwaitingHost || !strings.Contains(busy, unfinishedBusy) || len(h.journal()) != 0 {
			t.Fatalf("exit code %d, journal %d:\\n%s", code, len(h.journal()), busy)
		}
		if code, _, _ := h.run(nil, proveCommand, allFlag); code != cli.ExitAwaitingHost {
			t.Fatalf("after the release: exit %d", code)
		}
	})

	t.Run("the waiting count equals the listed tickets and the progress counts the answers since a cursor"+verifiesStepOutput+verifiesAnswerTally, func(t *testing.T) {
		h := newHarness(t, shared)
		_, output, _ := h.run(nil, proveCommand, allFlag)
		listed := len(h.requestsJSON().Items)
		before := h.progress(0)
		if listed == 0 || !strings.Contains(output, "awaiting_host "+itoa(listed)+" tickets") || before.Launchable != listed || before.Answered != 0 {
			t.Fatalf("listed %d, progress %+v:\\n%s", listed, before, output)
		}
		if _, present := h.tally(); !present {
			t.Fatal("no pending file while criteria wait")
		}
		results, err := hostprototest.PlayCLI(h, hostprototest.PersonaHonest, oracle{}, func(time.Duration) {})
		if err != nil {
			t.Fatal(err)
		}
		submitted := 0
		for _, result := range results {
			if result.Submitted {
				submitted++
			}
		}
		after := h.progress(before.Cursor)
		pending, _ := h.tally()
		if submitted == 0 || after.Answered != submitted || pending.Answered != submitted || after.Launchable != 0 {
			t.Fatalf("submitted %d, progress %+v, pending %+v", submitted, after, pending)
		}
	})

	t.Run("the pending file keeps the count of answers, which never decreases, once the run finished"+verifiesAnswerTally, func(t *testing.T) {
		h := newHarness(t, shared)
		code, output := h.drive(hostprototest.PersonaHonest, oracle{})
		pending, present := h.tally()
		if code == cli.ExitAwaitingHost || code == cli.ExitToolFailure || !present || pending.Answered == 0 || pending.Pending == nil {
			t.Fatalf("exit %d, pending %+v present %v:\\n%s", code, pending, present, output)
		}
		journaled := hostproto.AcceptedAnswers(h.journal())
		if pending.Answered != journaled {
			t.Fatalf("the file counts %d answers, the journal %d", pending.Answered, journaled)
		}
	})

	t.Run("a verdict that rests on an abandoned ticket is never fresh and --changed asks the host again"+verifiesAbandonment, func(t *testing.T) {
		h := newHarness(t, shared)
		h.waves = 30
		code, output := h.drive(hostprototest.PersonaBreaker, hostprototest.SchemaAnswers{})
		if code != cli.ExitBlocked || !strings.Contains(output, "host_unresponsive") {
			t.Fatalf("exit code %d:\\n%s", code, output)
		}
		code, again, _ := h.run(nil, proveCommand, changedFlag)
		if code != cli.ExitAwaitingHost || strings.Contains(again, freshLine) {
			t.Fatalf("exit code %d:\\n%s", code, again)
		}
		renewed := false
		for _, item := range h.requestsJSON().Items {
			renewed = renewed || item.Ticket.Emission > hostproto.DefaultMaxEmissions
		}
		if !renewed {
			t.Fatal("--changed issued no abandoned ticket again")
		}
	})
}

func itoa(number int) string { return strconv.Itoa(number) }

// store is the host store of the harness's project.
func (h *harness) store() *hoststore.Store { return h.hostStore() }

// requestsJSON is the listing of the prove session's tickets.
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

// driveInSteps drives `prove` as a harness whose tool calls time out does: every
// call gets a budget, a cut call is run again, and the host answers between
// waves. A budget too short to store any fact is doubled. It returns the last
// output and how many calls were cut.
func (h *harness) driveInSteps(answers hostprototest.AnswerSource) (code int, output string, cuts int) {
	h.t.Helper()
	budget := firstStepBudget
	args := []string{proveCommand, allFlag}
	for range maxSteps {
		ctx, cancel := context.WithTimeoutCause(context.Background(), budget, hostproto.ErrStepSpent)
		h.ctx = ctx
		var stderr string
		code, output, stderr = h.run(nil, args...)
		cancel()
		h.ctx = nil
		switch {
		case code == cli.ExitToolFailure && strings.Contains(stderr, "host.step_seconds"):
			budget *= 2
			continue
		case code == cli.ExitToolFailure:
			h.t.Fatalf("prove failed: %s", stderr)
		case code != cli.ExitAwaitingHost:
			return code, output, cuts
		case strings.Contains(output, unfinishedSpent):
			cuts++
		default:
			if _, err := hostprototest.PlayCLI(h, hostprototest.PersonaHonest, answers, func(time.Duration) {}); err != nil {
				h.t.Fatal(err)
			}
		}
		args = []string{proveCommand, allFlag, resumeFlag}
	}
	h.t.Fatalf("prove did not end in %d steps", maxSteps)
	return code, output, cuts
}

func TestStepsMatchOneRun(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the parcel project's tests against many mutants, twice")
	}
	t.Run("a run cut into steps of a short budget writes the Statements of a run in one step, byte for byte"+verifiesStepBound+verifiesCutFacts, func(t *testing.T) {
		whole := newHarness(t, "")
		wholeCode, _ := whole.drive(hostprototest.PersonaHonest, oracle{})
		stepped := newHarness(t, "")
		steppedCode, output, cuts := stepped.driveInSteps(oracle{})
		if cuts == 0 {
			t.Fatalf("no call was cut, so the test checks nothing:\n%s", output)
		}
		if wholeCode != steppedCode {
			t.Fatalf("exit %d in one step, %d in %d cut steps", wholeCode, steppedCode, cuts)
		}
		want, got := whole.statementBytes(), stepped.statementBytes()
		if len(want) == 0 || len(got) != len(want) {
			t.Fatalf("%d Statements in one step, %d in steps", len(want), len(got))
		}
		for criterion, statement := range want {
			if got[criterion] != statement {
				t.Errorf("the Statement of %s differs once the run is cut into %d steps", criterion, cuts)
			}
		}
	})

	t.Run("a step too short to store any executed fact fails and names the setting"+verifiesCutFacts, func(t *testing.T) {
		h := newHarness(t, "")
		ctx, cancel := context.WithTimeoutCause(context.Background(), time.Millisecond, hostproto.ErrStepSpent)
		defer cancel()
		h.ctx = ctx
		code, _, stderr := h.run(nil, proveCommand, allFlag)
		if code != cli.ExitToolFailure || !strings.Contains(stderr, "host.step_seconds") {
			t.Fatalf("exit %d: %s", code, stderr)
		}
	})
}
