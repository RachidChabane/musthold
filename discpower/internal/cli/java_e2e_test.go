package cli_test

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/cli"
	"github.com/RachidChabane/spec-proof/discpower/internal/testkit/jvmenv"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	mavenProject = "../../fixtures/ecosystems/junit5-maven"
	mavenReport  = "../../fixtures/prove/maven-report.json"
)

// newJavaHarness copies the Maven fixture project, which binds its three
// operations, and fetches its dependencies once so that every build after it runs
// offline.
func newJavaHarness(t *testing.T) *harness {
	t.Helper()
	jvmenv.Require(t)
	jvmenv.FetchMaven(t, mavenProject)
	report, err := filepath.Abs(mavenReport)
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
	jvmenv.CopyFixture(t, mavenProject, h.root)
	return h
}

func TestProveAJavaProjectEndToEnd(t *testing.T) {
	t.Run("the honest persona blocks the weak test on an exposed witness and passes the others on their examples on Maven [verifies SP-1122~3]", func(t *testing.T) {
		h := newJavaHarness(t)
		code, output := h.drive(hostprototest.PersonaHonest, javaOracle{})
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
