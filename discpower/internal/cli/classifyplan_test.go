package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/cli"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

// shadowConfig turns the switch to the mode that plans every request.
const shadowConfig = "systemone:\n  classify: shadow\n"

// inShadow writes the shared configuration of the project of the fixture.
func (f *hostFixture) inShadow() *hostFixture {
	f.t.Helper()
	directory := filepath.Join(f.root, ".spec-proof")
	if err := os.MkdirAll(directory, 0o750); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "config.yaml"), []byte(shadowConfig), 0o600); err != nil {
		f.t.Fatal(err)
	}
	return f
}

func TestSystemOneClassifyPlan(t *testing.T) {
	closed := hostprototest.ClosedRequest()
	const (
		model  = "claude-haiku-4-5-20251001"
		sonnet = "claude-sonnet-5-5"
	)
	source := "host-classify(5,rot1):" + model + "*5"

	t.Run("classify-plan lists the five calls of each question of the closed requests that wait for their first vote [verifies SP-1910~1]", func(t *testing.T) {
		f := newHostFixture(t).inShadow()
		f.emit(hostprototest.Request(), 0)
		ticket := f.emit(closed, 0)
		f.emit(closed, 1)
		code, stdout, stderr := f.run(nil, "systemone", "classify-plan", "--model", model)
		if code != cli.ExitClean {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		items := hostprototest.DecodeClassifyPlans(t, stdout)
		if len(items) != 1 {
			t.Fatalf("%d items: %s", len(items), stdout)
		}
		hostprototest.CheckClassifyPlan(t, items[0], ticket, closed.Closed, source)
	})

	t.Run("classify-plan with voters assigns the first voter the first calls [verifies SP-1910~1]", func(t *testing.T) {
		f := newHostFixture(t).inShadow()
		ticket := f.emit(closed, 0)
		voters := sonnet + "*3+" + model + "*2"
		code, stdout, stderr := f.run(nil, "systemone", "classify-plan", "--voters", voters)
		if code != cli.ExitClean {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		items := hostprototest.DecodeClassifyPlans(t, stdout)
		hostprototest.CheckClassifyPlan(t, items[0], ticket, closed.Closed, "host-classify(5,rot1):"+voters)
		var models []string
		for _, call := range items[0].Calls {
			models = append(models, call.Model)
		}
		if strings.Join(models, " ") != strings.Repeat(sonnet+" ", 3)+model+" "+model {
			t.Fatalf("models %v", models)
		}
	})

	t.Run("classify-plan of an empty store is an empty array [verifies SP-1910~1]", func(t *testing.T) {
		f := newHostFixture(t).inShadow()
		if code, stdout, _ := f.run(nil, "systemone", "classify-plan", "--model", model); code != cli.ExitClean || strings.TrimSpace(stdout) != "[]" {
			t.Fatalf("exit %d, stdout %q", code, stdout)
		}
	})

	refusals := map[string][]string{
		"neither a model nor voters":     {"systemone", "classify-plan"},
		"a model and voters":             {"systemone", "classify-plan", "--model", model, "--voters", model + "*5"},
		"voters that do not sum to five": {"systemone", "classify-plan", "--voters", model + "*4"},
		"an unexpected argument":         {"systemone", "classify-plan", "--model", model, "extra"},
	}
	for name, args := range refusals {
		t.Run("classify-plan with "+name+" is a refusal [verifies SP-1910~1]", func(t *testing.T) {
			f := newHostFixture(t).inShadow()
			f.emit(closed, 0)
			if code, stdout, stderr := f.run(nil, args...); code == cli.ExitClean || stdout != "" || stderr == "" {
				t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
			}
		})
	}
}

func TestImportOfAClassification(t *testing.T) {
	closed := hostprototest.ClosedRequest()
	const model = "claude-haiku-4-5-20251001"
	source := "host-classify(5,rot1):" + model + "*5"

	t.Run("a refused import of a classification is a refusal and leaves the ticket to the host votes [verifies SP-1916~1]", func(t *testing.T) {
		f := newHostFixture(t).inShadow()
		ticket := f.emit(closed, 0)
		code, stdout, stderr := f.run(nil, "systemone", "classify-plan", "--model", model)
		if code != cli.ExitClean {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		item := hostprototest.DecodeClassifyPlans(t, stdout)[0]
		report := hostprototest.ClassifyReportOf(t, item, hostprototest.Always("VIOLATES"))
		refusals := map[string][]string{
			"another nonce":            {"systemone", "import", ticket.TicketID.String(), "--nonce", staleNonce, "--source", source},
			"a source off the grammar": {"systemone", "import", ticket.TicketID.String(), "--nonce", ticket.Nonce, "--source", "host-classify(5,rot1):" + model + "*4"},
			"no ticket":                {"systemone", "import", "--nonce", ticket.Nonce, "--source", source},
		}
		for name, args := range refusals {
			if code, _, stderr := f.run(report, args...); code != refusalExit || stderr == "" {
				t.Errorf("%s: exit %d, stderr %q", name, code, stderr)
			}
		}
		if code, _, stderr := f.run([]byte(`{"calls":[]}`), "systemone", "import", ticket.TicketID.String(), "--nonce", ticket.Nonce, "--source", source); code != refusalExit || stderr == "" {
			t.Errorf("a report without a host version: exit %d, stderr %q", code, stderr)
		}
		if f.stateOf(ticket) != hostproto.StateEmitted {
			t.Fatalf("state %s", f.stateOf(ticket))
		}
	})

	t.Run("in shadow an import prints its outcome and answers nothing [verifies SP-1916~1]", func(t *testing.T) {
		f := newHostFixture(t).inShadow()
		ticket := f.emit(closed, 0)
		_, stdout, _ := f.run(nil, "systemone", "classify-plan", "--model", model)
		item := hostprototest.DecodeClassifyPlans(t, stdout)[0]
		code, printed, stderr := f.run(hostprototest.ClassifyReportOf(t, item, hostprototest.Always("VIOLATES")), "systemone", "import", ticket.TicketID.String(), "--nonce", ticket.Nonce, "--source", source)
		if code != cli.ExitClean || !strings.Contains(printed, `"outcome":"shadow"`) || f.stateOf(ticket) != hostproto.StateEmitted || !f.journaled(hostproto.EventClassifyShadow) {
			t.Fatalf("exit %d, stdout %q, stderr %q, state %s", code, printed, stderr, f.stateOf(ticket))
		}
	})
}
