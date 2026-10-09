package cli_test

import (
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/cli"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

func TestSystemOneClassifyPlan(t *testing.T) {
	closed := hostprototest.ClosedRequest()
	const (
		model  = "claude-haiku-4-5-20251001"
		sonnet = "claude-sonnet-5-5"
	)
	source := "host-classify(5,rot1):" + model + "*5"

	t.Run("classify-plan lists the five calls of each question of the closed requests that wait for their first vote [verifies SP-1910~1]", func(t *testing.T) {
		f := newHostFixture(t)
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
		f := newHostFixture(t)
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
		f := newHostFixture(t)
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
			f := newHostFixture(t)
			f.emit(closed, 0)
			if code, stdout, stderr := f.run(nil, args...); code == cli.ExitClean || stdout != "" || stderr == "" {
				t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
			}
		})
	}
}
