package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/completeness/internal/cli"
	"github.com/RachidChabane/spec-proof/completeness/internal/testkit"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostbundle"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	ciRunID        = "1001"
	maxCIRounds    = 8
	ciFlag         = "--ci"
	importFlag     = "--import-host"
	issuedRelative = ".spec-proof/ci-issued"
	issuedDocument = "issued.json"
)

// extractRef unpacks the tree of a ref into dir, as the CI job that fetches the
// ref does.
func extractRef(t *testing.T, root, ref, dir string) {
	t.Helper()
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, directoryMode); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "ref.tar")
	git(t, root, "archive", "--format=tar", "-o", archive, ref)
	if output, err := exec.Command("tar", "-xf", archive, "-C", dir).CombinedOutput(); err != nil {
		t.Fatalf("tar: %v\n%s", err, output)
	}
}

func TestProveCIFlags(t *testing.T) {
	t.Run("--import-host without --ci and --ci with --record or --replay are refused before anything runs [verifies SP-790~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
		if code, _, _ := h.run(nil, proveCommand, importFlag, t.TempDir()); code != cli.ExitToolFailure {
			t.Errorf("--import-host alone exit = %d", code)
		}
		if code, _, _ := h.run(nil, proveCommand, ciFlag, "--replay", t.TempDir()); code != cli.ExitToolFailure {
			t.Errorf("--ci with --replay exit = %d", code)
		}
		if code, _, _ := h.run(nil, proveCommand, ciFlag, "--record", t.TempDir()); code != cli.ExitToolFailure {
			t.Errorf("--ci with --record exit = %d", code)
		}
		if items, err := h.store().Items(); err != nil || len(items) != 0 {
			t.Errorf("a refused run did work: %v (%v)", items, err)
		}
	})

	t.Run("--ci needs no run identifier and no repository, and leaves the requests nobody answered for the host to pull [verifies SP-790~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
		code, output, stderr := h.run(nil, proveCommand, allFlag, ciFlag)
		if code != cli.ExitAwaitingHost {
			t.Fatalf("exit %d: %s\n%s", code, stderr, output)
		}
		issued := filepath.Join(h.root, filepath.FromSlash(issuedRelative), issuedDocument)
		if _, err := os.Stat(issued); err != nil {
			t.Fatalf("the requests CI issued were not left for the job to upload: %v", err)
		}
		plain := newHarness(t, testkit.PythonLedger)
		if code, _, _ := plain.run(nil, proveCommand, allFlag); code != cli.ExitAwaitingHost {
			t.Fatalf("a plain run exited %d", code)
		}
		if _, err := os.Stat(filepath.Join(plain.root, filepath.FromSlash(issuedRelative))); !os.IsNotExist(err) {
			t.Fatalf("a run that is not CI left issued requests: %v", err)
		}
	})

	t.Run("a bundle that is not one is refused [verifies SP-790~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
		if code, _, _ := h.run(nil, proveCommand, ciFlag, importFlag, t.TempDir()); code != cli.ExitBlocked {
			t.Errorf("an empty directory exit = %d", code)
		}
	})

	t.Run("the Statements of a CI run are marked as produced by CI [verifies SP-790~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
		code, output := h.drive(hostprototest.PersonaHonest, testkit.Honest(), ciFlag)
		if code != cli.ExitClean {
			t.Fatalf("exit %d:\n%s", code, output)
		}
		if got := h.mustStatement().Predicate.ProducedBy; got != format.ProducedByCI {
			t.Errorf("produced by %q, want %q", got, format.ProducedByCI)
		}
	})
}

func TestCIRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("proves the ledger project twice and answers CI's requests")
	}
	t.Run("answers pushed from the laptop settle CI's run, and the requests CI issues come back as tickets until it ends [verifies SP-790~1]", func(t *testing.T) {
		baseline := newHarness(t, testkit.PythonLedger)
		if code, output := baseline.drive(hostprototest.PersonaHonest, testkit.Honest()); code != cli.ExitClean {
			t.Fatalf("the in-process honest host exited %d:\n%s", code, output)
		}
		want := questionLines(baseline.mustStatement())

		laptop := newHarness(t, testkit.PythonLedger)
		head := git(t, laptop.root, "rev-parse", "HEAD")
		host := hostOver(laptop, true)
		if code, _, stderr := laptop.run(nil, proveCommand, allFlag); code != cli.ExitAwaitingHost {
			t.Fatalf("the laptop's first prove exited %d: %s", code, stderr)
		}
		serveAll := func() {
			t.Helper()
			for _, item := range laptop.requestsJSON().Items {
				host.serve(item, testkit.Honest())
			}
		}
		serveAll()

		bundle := filepath.Join(t.TempDir(), "bundle")
		push := func() {
			t.Helper()
			if code, stdout, stderr := laptop.run(nil, "host", "push"); code != cli.ExitClean {
				t.Fatalf("push: %d %s%s", code, stdout, stderr)
			}
			extractRef(t, laptop.root, hostbundle.RefName(hostModule, head), bundle)
		}
		push()

		ci := newHarness(t, testkit.PythonLedger)
		var code int
		var output string
		for round := 0; round < maxCIRounds; round++ {
			var stderr string
			code, output, stderr = ci.run(nil, proveCommand, allFlag, ciFlag, importFlag, bundle)
			output += stderr
			t.Logf("CI round %d exited %d", round+1, code)
			if code != cli.ExitAwaitingHost {
				break
			}
			issued := filepath.Join(ci.root, filepath.FromSlash(issuedRelative))
			if _, err := os.Stat(filepath.Join(issued, issuedDocument)); err != nil {
				t.Fatalf("CI left requests pending and issued none: %v\n%s", err, output)
			}
			stubGH(t, issued, false)
			if code, _, stderr := laptop.run(nil, "host", "pull", "--ci-run", ciRunID); code != cli.ExitClean {
				t.Fatalf("pull: %d %s", code, stderr)
			}
			serveAll()
			push()
		}
		if code != cli.ExitClean {
			t.Fatalf("CI did not settle after %d rounds: exit %d\n%s", maxCIRounds, code, output)
		}
		statement := ci.mustStatement()
		if got := questionLines(statement); !slices.Equal(got, want) {
			t.Errorf("questions %v, want %v", got, want)
		}
		if got := statement.Predicate.ProducedBy; got != format.ProducedByCI {
			t.Errorf("produced by %q, want %q", got, format.ProducedByCI)
		}
	})
}
