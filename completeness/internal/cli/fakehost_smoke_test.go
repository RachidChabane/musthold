package cli_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/completeness/internal/cli"
	"github.com/RachidChabane/spec-proof/completeness/internal/testkit"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	binaryPackage      = "./cmd/spec-proof-completeness"
	fakehostPackage    = "./hostproto/hostprototest/cmd/fakehost"
	formatModule       = "../../../format"
	completenessModule = "../.."
	wrapperName        = "module.sh"
	buildTags          = "grammar_subset,grammar_subset_python,grammar_subset_javascript,grammar_subset_typescript,grammar_subset_tsx,grammar_subset_java,grammar_subset_kotlin"
)

// build compiles a package of a module into a binary in dir.
func build(t *testing.T, module, pkg, dir, name string, tags ...string) string {
	t.Helper()
	output := filepath.Join(dir, name)
	args := []string{"build", "-o", output}
	if len(tags) > 0 {
		args = append(args, "-tags", strings.Join(tags, ","))
	}
	command := exec.Command("nice", append([]string{"-n", "10", "go"}, append(args, pkg)...)...)
	command.Dir = module
	command.Env = append(os.Environ(), "GOMAXPROCS=2", "GOPROXY=off")
	if combined, err := command.CombinedOutput(); err != nil {
		t.Fatalf("go build %s: %v\n%s", pkg, err, combined)
	}
	return output
}

// TestFakehostSmoke drives the real binary with the real fake host, serving the
// answers an honest host recorded earlier: the command-line contract is the one
// the in-process tests exercise, and exposed answers give the same Statement.
func TestFakehostSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("builds two binaries")
	}
	recordings := t.TempDir()
	recorder := newHarness(t, testkit.PythonLedger)
	power, aligned := testkit.PythonEvidence()
	recorder.withEvidence(power, aligned)
	recorder.drive(hostprototest.PersonaHonest, testkit.Honest(), "--record", recordings)
	want := recorder.statementBytes()

	bin := t.TempDir()
	module := build(t, completenessModule, binaryPackage, bin, "spec-proof-completeness", buildTags)
	fakehost := build(t, formatModule, fakehostPackage, bin, "fakehost")

	h := newHarness(t, testkit.PythonLedger)
	h.withEvidence(power, aligned)
	shallnot, err := filepath.Abs(filepath.Join(testkit.FixturesDir(), testkit.RecordedShallnot))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(stateVariable, h.state)
	t.Setenv(cacheVariable, h.cache)
	t.Setenv(shallnotVariable, shallnot)
	t.Setenv(reportVariable, h.report)
	wrapper := filepath.Join(bin, wrapperName)
	script := fmt.Sprintf("#!/bin/sh\ncd %q && exec %q \"$@\"\n", h.root, module)
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("the real fake host drives the real binary to the Statement of the in-process host [verifies SP-545~1]", func(t *testing.T) {
		code := cli.ExitAwaitingHost
		for wave := 0; code == cli.ExitAwaitingHost && wave < maxWavesPerDrive*2; wave++ {
			if wave > 0 {
				host := exec.Command(fakehost, "-persona", hostprototest.PersonaHonest, "-module-bin", wrapper, "-recordings", recordings)
				if combined, err := host.CombinedOutput(); err != nil {
					t.Fatalf("fakehost: %v\n%s", err, combined)
				}
			}
			run := exec.Command(wrapper, proveCommand, allFlag)
			code = 0
			if _, runErr := run.Output(); runErr != nil {
				exitErr, isExit := runErr.(*exec.ExitError)
				if !isExit {
					t.Fatal(runErr)
				}
				code = exitErr.ExitCode()
			}
		}
		if code != cli.ExitClean {
			t.Fatalf("exit code %d", code)
		}
		got := h.statementBytes()
		if len(got) != 1 || len(want) != 1 {
			t.Fatalf("%d Statements, want %d", len(got), len(want))
		}
		for path, content := range want {
			if got[path] != content {
				t.Errorf("%s differs between the real binary and the in-process host", path)
			}
		}
	})
}
