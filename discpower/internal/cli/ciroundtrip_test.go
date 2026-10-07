package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/cli"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	ciRunID        = "1001"
	maxCIRounds    = 4
	issuedRelative = ".spec-proof/ci-issued"
	seedsRelative  = ".spec-proof/cache/seeds"
)

// extractRef unpacks the tree of a ref into dir, as the CI job that fetches the
// ref does.
func extractRef(t *testing.T, root, ref, dir string) {
	t.Helper()
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "ref.tar")
	git(t, root, "archive", "--format=tar", "-o", archive, ref)
	if output, err := exec.Command("tar", "-xf", archive, "-C", dir).CombinedOutput(); err != nil {
		t.Fatalf("tar: %v\n%s", err, output)
	}
}

func TestCIRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("proves the parcel project twice and answers CI's requests")
	}
	t.Run("answers pushed from the laptop settle CI's run, and the requests CI issues come back as tickets until it ends [verifies SP-249~1]", func(t *testing.T) {
		shared := t.TempDir()
		laptop := newHarness(t, shared)
		commitProject(t, laptop.root)
		laptop.drive(hostprototest.PersonaHonest, oracle{})
		t.Setenv("SPEC_PROOF_CI_RUN_ID", ciRunID)

		bundle := filepath.Join(t.TempDir(), "bundle")
		push := func() {
			t.Helper()
			code, stdout, stderr := laptop.run(nil, "host", "push")
			if code != cli.ExitClean {
				t.Fatalf("push: %d %s%s", code, stdout, stderr)
			}
			head := git(t, laptop.root, "rev-parse", "HEAD")
			extractRef(t, laptop.root, "refs/spec-proof/host/discpower/"+head, bundle)
		}
		push()

		ci := newHarness(t, shared)
		commitProject(t, ci.root)
		var code int
		var output string
		for round := 0; round < maxCIRounds; round++ {
			var stderr string
			code, output, stderr = ci.run(nil, proveCommand, allFlag, "--ci", "--import-host", bundle)
			output += stderr
			t.Logf("CI round %d exited %d", round+1, code)
			if code != cli.ExitAwaitingHost {
				break
			}
			issued := filepath.Join(ci.root, filepath.FromSlash(issuedRelative))
			if _, err := os.Stat(filepath.Join(issued, "issued.json")); err != nil {
				t.Fatalf("CI left requests pending and issued none: %v\n%s", err, output)
			}
			stubGH(t, issued, false)
			if code, _, stderr := laptop.run(nil, "host", "pull", "--ci-run", ciRunID); code != cli.ExitClean {
				t.Fatalf("pull: %d %s", code, stderr)
			}
			if _, err := hostprototest.PlayCLI(laptop, hostprototest.PersonaHonest, oracle{}, func(time.Duration) {}); err != nil {
				t.Fatalf("the host could not answer CI's requests: %v", err)
			}
			push()
		}
		if code == cli.ExitAwaitingHost || code == cli.ExitToolFailure {
			t.Fatalf("CI still waited after %d rounds: exit %d\n%s", maxCIRounds, code, output)
		}
		statements := ci.statements()
		if len(statements) != 3 {
			t.Fatalf("%d Statements written, want 3; exit %d\n%s", len(statements), code, output)
		}
		for _, criterion := range []string{"PAR-1~1", "PAR-2~1", "PAR-3~1"} {
			if got := ci.statement(criterion).Predicate.ProducedBy; got != format.ProducedByCI {
				t.Errorf("%s produced by %q, want %q", criterion, got, format.ProducedByCI)
			}
		}
		seeds, err := os.ReadDir(filepath.Join(ci.root, filepath.FromSlash(seedsRelative)))
		if err != nil || len(seeds) != 1 || !strings.HasSuffix(seeds[0].Name(), ".json") {
			t.Fatalf("seeds = %v, %v", seeds, err)
		}
	})
}
