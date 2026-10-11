package e2e_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
)

// The projects a scan runs on, and what it finds in them.
const (
	pytestScanFixture = "fixtures/scan/pytest"
	goScanFixture     = "fixtures/scan/go"
	vacuousPytest     = "tests/test_discount.py::test_discount_accepts_any_percent"
	vacuousGo         = "discount_test.go::TestDiscountAcceptsAnyPercent"
	verifiesScanE2E   = " [verifies SP-2122~2]"
	verifiesReplay    = " [verifies SP-2114~1]"
	verifiesCommand   = " [verifies SP-2119~2]"
)

// scanned is a copy of a scan fixture committed in a repository of its own.
type scanned struct {
	t      *testing.T
	root   string
	binary string
	tmp    string
}

func newScanned(t *testing.T, fixture string) *scanned {
	t.Helper()
	guardMemory(t)
	binary := productBinary(t)
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &scanned{t: t, root: filepath.Join(base, "project"), binary: binary, tmp: filepath.Join(base, "tmp")}
	if err := os.MkdirAll(s.tmp, directoryMode); err != nil {
		t.Fatal(err)
	}
	copyTree(t, filepath.Join(abs(t, discpowerDir), fixture), s.root)
	for _, args := range [][]string{
		{"init", "-q"}, {"add", "-A"},
		{"-c", "user.name=scan", "-c", "user.email=scan@example.com", "-c", "commit.gpgsign=false", "commit", "-qm", "first"},
	} {
		if result := s.run("git", args...); result.code != 0 {
			t.Fatalf("git %v: %s", args, result.output())
		}
	}
	return s
}

func (s *scanned) run(name string, args ...string) result {
	s.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, "nice", append([]string{"-n", "10", name}, args...)...)
	command.Dir = s.root
	command.Env = append(os.Environ(), "TMPDIR="+s.tmp, "GOMAXPROCS=2")
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	code := 0
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		code = exit.ExitCode()
	case err != nil:
		s.t.Fatalf("%s %v: %v", name, args, err)
	}
	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func (s *scanned) commit() string {
	s.t.Helper()
	return strings.TrimSpace(s.run("git", "rev-parse", "HEAD").stdout)
}

func TestScanFindsTheTestThatVerifiesNothing(t *testing.T) {
	for _, c := range []struct {
		name, fixture, vacuous string
		tool                   string
	}{
		{"pytest", pytestScanFixture, vacuousPytest, "python3"},
		{"go", goScanFixture, vacuousGo, "go"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := exec.LookPath(c.tool); err != nil {
				t.Skipf("no %s: %v", c.tool, err)
			}
			s := newScanned(t, c.fixture)
			short := s.commit()[:7]
			scan := s.run(s.binary, "scan", "--budget", "0")
			t.Run("the scan names the test that runs code and asserts nothing, and exits 0"+verifiesScanE2E, func(t *testing.T) {
				if scan.code != 0 || !strings.Contains(scan.stdout, "#1 "+c.vacuous+" passes on all ") {
					t.Fatalf("exit %d:\n%s", scan.code, scan.output())
				}
				for _, want := range []string{"1/5 verify nothing", "Not a pass", "no model used", "promises not checked"} {
					if !strings.Contains(scan.stdout, want) {
						t.Errorf("missing %q:\n%s", want, scan.stdout)
					}
				}
			})
			t.Run("the work tree only gains the scan's reports"+verifiesScanE2E, func(t *testing.T) {
				status := s.run("git", "status", "--porcelain", "--untracked-files=all")
				for _, line := range strings.Split(strings.TrimSpace(status.stdout), "\n") {
					if !strings.HasPrefix(line, "?? .spec-proof/scan/"+short+"/") {
						t.Errorf("unexpected change %q", line)
					}
				}
			})
			t.Run("the repro script passes the test on the mutant"+verifiesScanE2E, func(t *testing.T) {
				if repro := s.run("sh", ".spec-proof/scan/"+short+"/1/repro.sh"); repro.code != 0 {
					t.Fatalf("exit %d:\n%s", repro.code, repro.output())
				}
			})
			t.Run("a replay of the scan's findings holds every claim with no model"+verifiesReplay, func(t *testing.T) {
				replay := s.run(s.binary, "scan", "replay", ".spec-proof/scan/"+short)
				if replay.code != 0 || !strings.Contains(replay.stdout, c.vacuous) || strings.Contains(replay.stdout, "NOT held") {
					t.Fatalf("exit %d:\n%s", replay.code, replay.output())
				}
			})
			t.Run("--format json prints the scan statement, which is never admitted as a proof"+verifiesCommand, func(t *testing.T) {
				statement := s.run(s.binary, "scan", "--format", "json")
				raw := bytes.TrimSpace([]byte(statement.stdout))
				admitted, err := format.AdmitScan(raw)
				if statement.code != 0 || err != nil || admitted.Subject[0].Name != s.commit() {
					t.Fatalf("exit %d, %v:\n%s", statement.code, err, statement.output())
				}
				if _, err := format.Admit(raw); !errors.Is(err, format.ErrUnknownPredicateType) {
					t.Fatalf("admitted as a proof: %v", err)
				}
			})
		})
	}
}

func TestScanIsQuickByDefault(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("no python3: %v", err)
	}
	s := newScanned(t, pytestScanFixture)
	scan := s.run(s.binary, "scan")
	t.Run("a scan with no flag runs quickly, prints its depth line and exits 0"+verifiesScanE2E, func(t *testing.T) {
		for _, want := range []string{"quick scan · 1m30s budget · ", "5 of 5 green tests sampled", "run with --full for the whole suite", "Not a pass"} {
			if scan.code != 0 || !strings.Contains(scan.stdout, want) {
				t.Fatalf("exit %d, missing %q:\n%s", scan.code, want, scan.output())
			}
		}
	})
}

func TestScanRefusesADirtyTree(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("no python3: %v", err)
	}
	s := newScanned(t, pytestScanFixture)
	if err := os.WriteFile(filepath.Join(s.root, "invoice", "totals.py"), []byte("def line_total(a, b):\n    return 0\n"), fileMode); err != nil {
		t.Fatal(err)
	}
	t.Run("a scan of a tree whose tracked files changed fails with a reason and writes nothing"+verifiesCommand, func(t *testing.T) {
		scan := s.run(s.binary, "scan")
		if scan.code == 0 || !strings.Contains(scan.stderr, "commit or stash") {
			t.Fatalf("exit %d:\n%s", scan.code, scan.output())
		}
		if _, err := os.Stat(filepath.Join(s.root, ".spec-proof")); !os.IsNotExist(err) {
			t.Fatalf("reports written: %v", err)
		}
	})
}
