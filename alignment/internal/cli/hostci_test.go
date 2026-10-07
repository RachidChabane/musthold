package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/RachidChabane/spec-proof/alignment/internal/cli"
	"github.com/RachidChabane/spec-proof/alignment/internal/testkit"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostbundle"
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
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "ref.tar")
	git(t, root, "archive", "--format=tar", "-o", archive, ref)
	if output, err := exec.Command("tar", "-xf", archive, "-C", dir).CombinedOutput(); err != nil {
		t.Fatalf("tar: %v\n%s", err, output)
	}
}

func TestProveCIFlags(t *testing.T) {
	t.Run("--import-host without --ci and --ci with --record or --replay are refused before anything runs [verifies SP-750~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
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

	t.Run("--ci needs no run identifier and no repository, and leaves the requests nobody answered for the host to pull [verifies SP-750~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		h.withPowers(testkit.PythonPowers())
		code, output, stderr := h.run(nil, proveCommand, allFlag, ciFlag)
		if code != cli.ExitAwaitingHost {
			t.Fatalf("exit %d: %s\n%s", code, stderr, output)
		}
		issued := filepath.Join(h.root, filepath.FromSlash(issuedRelative), issuedDocument)
		if _, err := os.Stat(issued); err != nil {
			t.Fatalf("the requests CI issued were not left for the job to upload: %v", err)
		}
		plain := newHarness(t, testkit.PythonParcel)
		plain.withPowers(testkit.PythonPowers())
		if code, _, _ := plain.run(nil, proveCommand, allFlag); code != cli.ExitAwaitingHost {
			t.Fatalf("a plain run exited %d", code)
		}
		if _, err := os.Stat(filepath.Join(plain.root, filepath.FromSlash(issuedRelative))); !os.IsNotExist(err) {
			t.Fatalf("a run that is not CI left issued requests: %v", err)
		}
	})

	t.Run("a bundle that is not one is refused [verifies SP-750~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		if code, _, _ := h.run(nil, proveCommand, ciFlag, importFlag, t.TempDir()); code != cli.ExitBlocked {
			t.Errorf("an empty directory exit = %d", code)
		}
	})

	t.Run("the Statements of a CI run are marked as produced by CI [verifies SP-750~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		h.withPowers(testkit.PythonPowers())
		code, _ := h.driveAttested(testkit.Oracle{}, ciFlag)
		if code == cli.ExitAwaitingHost || code == cli.ExitToolFailure {
			t.Fatalf("exit %d", code)
		}
		statements := h.statements()
		if len(statements) != len(parcelVerdicts) {
			t.Fatalf("%d Statements, want %d", len(statements), len(parcelVerdicts))
		}
		for key, statement := range statements {
			if got := statement.Predicate.ProducedBy; got != format.ProducedByCI {
				t.Errorf("%s produced by %q, want %q", key, got, format.ProducedByCI)
			}
		}
	})
}

func TestCIRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("proves the parcel project twice and answers CI's requests")
	}
	t.Run("answers pushed from the laptop settle CI's run, and the requests CI issues come back as tickets until it ends [verifies SP-750~1]", func(t *testing.T) {
		laptop := newHarness(t, testkit.PythonParcel)
		laptop.withPowers(testkit.PythonPowers())
		head := commitProject(t, laptop.root)
		if code, _, stderr := laptop.run(nil, proveCommand, allFlag); code != cli.ExitAwaitingHost {
			t.Fatalf("the laptop's first prove exited %d: %s", code, stderr)
		}
		laptop.serveAttested(testkit.Oracle{})

		bundle := filepath.Join(t.TempDir(), "bundle")
		push := func() {
			t.Helper()
			if code, stdout, stderr := laptop.run(nil, "host", "push"); code != cli.ExitClean {
				t.Fatalf("push: %d %s%s", code, stdout, stderr)
			}
			extractRef(t, laptop.root, hostbundle.RefName(hostModule, head), bundle)
		}
		push()

		ci := newHarness(t, testkit.PythonParcel)
		ci.withPowers(testkit.PythonPowers())
		commitProject(t, ci.root)
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
			laptop.serveAttested(testkit.Oracle{})
			push()
		}
		if code == cli.ExitAwaitingHost || code == cli.ExitToolFailure {
			t.Fatalf("CI still waited after %d rounds: exit %d\n%s", maxCIRounds, code, output)
		}
		got := verdictsOf(ci, output)
		for key, want := range parcelVerdicts {
			if got[key] != want {
				t.Errorf("%s: %q, want %q", key, got[key], want)
			}
		}
		statements := ci.statements()
		if len(statements) != len(parcelVerdicts) {
			t.Fatalf("%d Statements written, want %d; exit %d\n%s", len(statements), len(parcelVerdicts), code, output)
		}
		for key, statement := range statements {
			if got := statement.Predicate.ProducedBy; got != format.ProducedByCI {
				t.Errorf("%s produced by %q, want %q", key, got, format.ProducedByCI)
			}
		}
	})
}
