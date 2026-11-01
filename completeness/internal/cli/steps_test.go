package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/completeness/internal/cli"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
	"strconv"
	"time"

	"github.com/RachidChabane/musthold/completeness/internal/testkit"
)

// Requirements the bounded steps verify.
const (
	verifiesStepBound   = " [verifies SP-2685~1]"
	verifiesAbandonment = " [verifies SP-2686~1]"
	verifiesStepOutput  = " [verifies SP-2687~1]"
	verifiesAnswerTally = " [verifies SP-2688~1]"

	unfinishedSpent = "unfinished budget_spent"
	unfinishedBusy  = "unfinished session_busy"
	freshStatus     = " fresh "
	pendingPath     = ".musthold/pending/completeness.json"
)

// spentContext is the context of a step whose budget already ran out.
func spentContext() context.Context {
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(hostproto.ErrStepSpent)
	return ctx
}

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

	t.Run("a step whose budget ran out keeps what is stored, writes nothing, exits 3, and the next call continues"+verifiesStepBound, func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
		code, first, _ := h.run(nil, proveCommand, allFlag)
		if code != cli.ExitAwaitingHost {
			t.Fatalf("exit code %d:\n%s", code, first)
		}
		before := h.requestsJSON()
		journal := len(h.journal())
		h.ctx = spentContext()
		code, cut, _ := h.run(nil, proveCommand, allFlag, resumeFlag)
		if code != cli.ExitAwaitingHost || !strings.Contains(cut, unfinishedSpent) {
			t.Fatalf("exit code %d:\n%s", code, cut)
		}
		if after := h.requestsJSON(); len(after.Items) != len(before.Items) || len(h.journal()) != journal {
			t.Fatalf("the cut step changed the store: %d then %d tickets, journal %d then %d", len(before.Items), len(after.Items), journal, len(h.journal()))
		}
		h.ctx = nil
		code, again, _ := h.run(nil, proveCommand, allFlag, resumeFlag)
		if code != cli.ExitAwaitingHost || strings.Contains(again, unfinishedSpent) || len(h.requestsJSON().Items) != len(before.Items) {
			t.Fatalf("the next call: exit %d:\n%s", code, again)
		}
	})

	t.Run("a step cut before it asked anything is resumed by --resume"+verifiesStepBound, func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
		h.ctx = spentContext()
		if code, cut, _ := h.run(nil, proveCommand, allFlag); code != cli.ExitAwaitingHost || !strings.Contains(cut, unfinishedSpent) {
			t.Fatalf("exit code %d:\n%s", code, cut)
		}
		h.ctx = nil
		if code, resumed, stderr := h.run(nil, proveCommand, allFlag, resumeFlag); code != cli.ExitAwaitingHost {
			t.Fatalf("exit code %d:\n%s%s", code, resumed, stderr)
		}
	})

	t.Run("a step whose session another run holds changes nothing and exits 3"+verifiesStepBound, func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
		release, err := h.store().LockRun(hostproto.SessionProve)
		if err != nil {
			t.Fatal(err)
		}
		code, busy, _ := h.run(nil, proveCommand, allFlag)
		release()
		if code != cli.ExitAwaitingHost || !strings.Contains(busy, unfinishedBusy) || len(h.journal()) != 0 {
			t.Fatalf("exit code %d, journal %d:\n%s", code, len(h.journal()), busy)
		}
		if code, _, _ := h.run(nil, proveCommand, allFlag); code != cli.ExitAwaitingHost {
			t.Fatalf("after the release: exit %d", code)
		}
	})

	t.Run("the waiting count equals the listed tickets and the progress counts the answers since a cursor"+verifiesStepOutput+verifiesAnswerTally, func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
		_, output, _ := h.run(nil, proveCommand, allFlag)
		listed := len(h.requestsJSON().Items)
		before := h.progress(0)
		if listed == 0 || !strings.Contains(output, "awaiting_host "+itoa(listed)+" tickets") || before.Launchable != listed || before.Answered != 0 {
			t.Fatalf("listed %d, progress %+v:\n%s", listed, before, output)
		}
		if _, present := h.tally(); !present {
			t.Fatal("no pending file while criteria wait")
		}
		results, err := hostprototest.PlayCLI(h, hostprototest.PersonaHonest, testkit.Honest(), func(time.Duration) {})
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
		if submitted == 0 || after.Answered != submitted || pending.Answered != 0 || after.Launchable != 0 {
			t.Fatalf("submitted %d, progress %+v, pending %+v before any execution check", submitted, after, pending)
		}
		h.run(nil, proveCommand, allFlag, resumeFlag)
		checked, _ := h.tally()
		if kept := hostproto.KeptAnswers(h.journal()); checked.Answered != kept {
			t.Fatalf("the pending file counts %d answers once checked, the journal kept %d", checked.Answered, kept)
		}
	})

	t.Run("the pending file keeps the count of answers, which never decreases, once the run finished"+verifiesAnswerTally, func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
		code, output := h.drive(hostprototest.PersonaHonest, testkit.Honest())
		pending, present := h.tally()
		if code == cli.ExitAwaitingHost || code == cli.ExitToolFailure || !present || pending.Answered == 0 || pending.Pending == nil {
			t.Fatalf("exit %d, pending %+v present %v:\n%s", code, pending, present, output)
		}
		journaled := hostproto.KeptAnswers(h.journal())
		if pending.Answered != journaled {
			t.Fatalf("the file counts %d answers, the journal %d", pending.Answered, journaled)
		}
	})

	t.Run("a verdict that rests on an abandoned ticket is never fresh and --changed asks the host again"+verifiesAbandonment, func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
		h.waves = 8
		code, output := h.drive(hostprototest.PersonaBreaker, hostprototest.SchemaAnswers{})
		if code != cli.ExitBlocked || !strings.Contains(output, "host_unresponsive") {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
		code, again, _ := h.run(nil, proveCommand, changedFlag)
		if code != cli.ExitAwaitingHost || strings.Contains(again, freshStatus) {
			t.Fatalf("exit code %d:\n%s", code, again)
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
