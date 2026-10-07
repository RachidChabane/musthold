package cli_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/cli"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	binaryPackage   = "./cmd/spec-proof-dp"
	fakehostPackage = "./hostproto/hostprototest/cmd/fakehost"
	formatModule    = "../../../format"
	discpowerModule = "../.."
	wrapperName     = "module.sh"
)

// build compiles a package of a module into a binary in dir.
func build(t *testing.T, module, pkg, dir, name string) string {
	t.Helper()
	output := filepath.Join(dir, name)
	command := exec.Command("go", "build", "-o", output, pkg)
	command.Dir = module
	command.Env = append(os.Environ(), "GOMAXPROCS=2")
	if combined, err := command.CombinedOutput(); err != nil {
		t.Fatalf("go build %s: %v\n%s", pkg, err, combined)
	}
	return output
}

// TestFakehostSmoke drives the real binary with the real fake host, serving the
// answers an honest host recorded earlier, once: the command line contract is the
// same one the in-process tests exercise.
func TestFakehostSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("builds two binaries and runs the parcel project's tests against many mutants")
	}
	shared := t.TempDir()
	recordings := t.TempDir()
	recorder := newHarness(t, shared)
	recorder.drive(hostprototest.PersonaHonest, oracle{}, "--record", recordings)

	bin := t.TempDir()
	module := build(t, discpowerModule, binaryPackage, bin, "spec-proof-dp")
	fakehost := build(t, formatModule, fakehostPackage, bin, "fakehost")

	h := newHarness(t, shared)
	shallnot, err := filepath.Abs(fixtureShallnot)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(stateVariable, h.state)
	t.Setenv(cacheVariable, h.cache)
	t.Setenv(shallnotVariable, shallnot)
	wrapper := filepath.Join(bin, wrapperName)
	script := fmt.Sprintf("#!/bin/sh\ncd %q && exec %q \"$@\"\n", h.root, module)
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("the real fake host drives the real binary to the recorded verdicts [verifies SP-182~1]", func(t *testing.T) {
		var output []byte
		code := cli.ExitAwaitingHost
		for wave := 0; code == cli.ExitAwaitingHost && wave < maxWavesPerDrive; wave++ {
			if wave > 0 {
				host := exec.Command(fakehost, "-persona", hostprototest.PersonaHonest, "-module-bin", wrapper, "-recordings", recordings)
				if combined, err := host.CombinedOutput(); err != nil {
					t.Fatalf("fakehost: %v\n%s", err, combined)
				}
			}
			run := exec.Command(wrapper, proveCommand, allFlag)
			var runErr error
			output, runErr = run.Output()
			code = 0
			if exitErr, isExit := runErr.(*exec.ExitError); isExit {
				code = exitErr.ExitCode()
			} else if runErr != nil {
				t.Fatal(runErr)
			}
		}
		got := verdicts(string(output))
		want := map[string]string{"PAR-1~1": provedPass, "PAR-2~1": provedPass, "PAR-3~1": provedBlock}
		for ref, verdict := range want {
			if !strings.EqualFold(got[ref], verdict) {
				t.Errorf("%s: %q, want %q\n%s", ref, got[ref], verdict, output)
			}
		}
	})
}
