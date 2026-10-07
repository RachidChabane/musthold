package cli_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/cli"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

const (
	goProject = "../../fixtures/ecosystems/go"
	goReport  = "../../fixtures/prove/go-report.json"
)

// newGoHarness copies the Go fixture project, which binds no operation.
func newGoHarness(t *testing.T) *harness {
	t.Helper()
	report, err := filepath.Abs(goReport)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(reportVariable, report)
	base := t.TempDir()
	h := &harness{
		t: t, root: filepath.Join(base, "parcel"), cache: filepath.Join(base, "cache"), state: t.TempDir(),
		now: time.Date(2026, time.October, 7, 12, 0, 0, 0, time.UTC),
	}
	copyTree(t, goProject, h.root)
	return h
}

func TestProveWithoutAnOperation(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the Go fixture's tests against many mutants")
	}
	t.Run("a project that binds no operation asks the host for nothing and escalates [verifies SP-190~2]", func(t *testing.T) {
		h := newGoHarness(t)
		code, output, stderr := h.run(nil, proveCommand, allFlag)
		if code != cli.ExitBlocked {
			t.Fatalf("exit code %d, want %d\n%s", code, cli.ExitBlocked, stderr)
		}
		_, listing, _ := h.run(nil, hostproto.ListArgs()...)
		if !strings.Contains(listing, `"items":[]`) {
			t.Errorf("the host was asked for something: %s", listing)
		}
		for _, ref := range []string{"PAR-1~1", "PAR-2~1", "PAR-3~1"} {
			if got := verdicts(output)[ref]; got != provedEscalate {
				t.Errorf("%s: %q, want %q\n%s", ref, got, provedEscalate, output)
			}
			if !slices.Contains(findings(output, ref), "no_valid_mutant") {
				t.Errorf("%s findings %v, want no_valid_mutant", ref, findings(output, ref))
			}
		}
		if !strings.Contains(output, "differential_skipped") {
			t.Errorf("the skipped search is not reported:\n%s", output)
		}
		if len(h.statements()) != 3 {
			t.Errorf("%d Statements written, want 3", len(h.statements()))
		}
	})
}
