package cli_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/cli"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	goProject = "../../fixtures/ecosystems/go"
	goReport  = "../../fixtures/prove/go-report.json"
)

// goConfig is the project configuration of the Go fixture, relative to its root.
const goConfig = ".spec-proof/config.yaml"

// newGoHarness copies the Go fixture project, which binds its three operations.
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
	t.Run("a project that binds no operation still asks for witnesses and never passes on judged evidence [verifies SP-190~3]", func(t *testing.T) {
		h := newGoHarness(t)
		if err := os.Remove(filepath.Join(h.root, goConfig)); err != nil {
			t.Fatal(err)
		}
		if code, _, stderr := h.run(nil, proveCommand, allFlag); code != cli.ExitAwaitingHost {
			t.Fatalf("exit code %d, want %d\n%s", code, cli.ExitAwaitingHost, stderr)
		}
		if witnesses, _, _ := h.pendingSchemas(); witnesses == 0 {
			t.Error("no witness was asked")
		}
		_, output := h.drive(hostprototest.PersonaHonest, oracle{})
		for _, ref := range []string{"PAR-1~1", "PAR-2~1"} {
			if got := verdicts(output)[ref]; got != provedEscalate {
				t.Errorf("%s: %q, want %q\n%s", ref, got, provedEscalate, output)
			}
			if !slices.Contains(findings(output, ref), "no_valid_mutant") {
				t.Errorf("%s findings %v, want no_valid_mutant", ref, findings(output, ref))
			}
		}
		if got := verdicts(output)["PAR-3~1"]; got != provedBlock {
			t.Errorf("PAR-3~1: %q, want %q\n%s", got, provedBlock, output)
		}
		if !strings.Contains(output, "differential_skipped") {
			t.Errorf("the skipped search is not reported:\n%s", output)
		}
		if len(h.statements()) != 3 {
			t.Errorf("%d Statements written, want 3", len(h.statements()))
		}
	})
}

func TestProveAGoProjectEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the Go fixture's tests against many mutants")
	}

	t.Run("the honest persona blocks the weak test on an exposed witness and passes the others on their examples [verifies SP-957~3]", func(t *testing.T) {
		h := newGoHarness(t)
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
			corroborated = corroborated || (survived && kinds[format.CorroborationWitness] && kinds[format.CorroborationBuilds])
		}
		if !corroborated {
			t.Error("the block rests on no surviving VALID mutant with a confirmed witness and a build")
		}
		for ref, examples := range map[string]int{"PAR-1~1": 1, "PAR-2~1": 2, "PAR-3~1": 0} {
			runs := h.statement(ref).Predicate.Examples
			if len(runs) != examples || slices.ContainsFunc(runs, func(run format.ExampleRun) bool { return run.Result.Value != format.ExamplePassed }) {
				t.Errorf("%s: examples %+v, want %d passed", ref, runs, examples)
			}
		}
	})
}
