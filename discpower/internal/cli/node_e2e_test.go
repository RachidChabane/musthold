package cli_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/cli"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/testkit/nodeinstall"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	nodeFixtures    = "../../fixtures/ecosystems"
	nodeReportsPath = "../../fixtures/prove"
	nodeModulesName = "node_modules"
)

// newNodeHarness copies the fixture project of a Node runner, which binds its
// three operations, and links its node_modules to the installed fixture.
func newNodeHarness(t *testing.T, name ecosystem.Name) *harness {
	t.Helper()
	installed := nodeinstall.Installed(t, filepath.Join(nodeFixtures, string(name)))
	report, err := filepath.Abs(filepath.Join(nodeReportsPath, string(name)+"-report.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(reportVariable, report)
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{
		t: t, root: filepath.Join(base, "parcel"), cache: filepath.Join(base, "cache"), state: t.TempDir(),
		now: time.Date(2026, time.October, 7, 12, 0, 0, 0, time.UTC),
	}
	copyTree(t, filepath.Join(nodeFixtures, string(name)), h.root)
	if err := os.Symlink(filepath.Join(installed, nodeModulesName), filepath.Join(h.root, nodeModulesName)); err != nil {
		t.Fatal(err)
	}
	return h
}

func TestProveATypeScriptProjectEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the fixture's tests against many mutants")
	}
	for _, name := range []ecosystem.Name{ecosystem.Jest, ecosystem.Vitest} {
		t.Run(string(name)+": the honest persona blocks the weak test and escalates the others, whose survivors exposed answers cannot clear [verifies SP-1041~2]", func(t *testing.T) {
			nodeinstall.RequireNode(t)
			h := newNodeHarness(t, name)
			code, output := h.drive(hostprototest.PersonaHonest, oracle{})
			want := map[string]string{"PAR-1~1": provedEscalate, "PAR-2~1": provedEscalate, "PAR-3~1": provedBlock}
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
}

func TestDoctorRecognizesANodeProject(t *testing.T) {
	for _, name := range []ecosystem.Name{ecosystem.Jest, ecosystem.Vitest} {
		t.Run("the "+string(name)+" project is recognized among the ecosystems [verifies SP-1042~1]", func(t *testing.T) {
			nodeinstall.RequireNode(t)
			h := newNodeHarness(t, name)
			code, output, stderr := h.run(nil, "doctor")
			if code != cli.ExitClean || !strings.Contains(output, "ok ecosystem "+string(name)) {
				t.Fatalf("exit code %d\n%s\n%s", code, output, stderr)
			}
		})
	}
}
