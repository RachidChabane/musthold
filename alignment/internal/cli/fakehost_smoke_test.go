package cli_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/alignment/internal/cli"
	"github.com/RachidChabane/musthold/alignment/internal/testkit"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
)

const (
	binaryPackage   = "./cmd/musthold-alignment"
	fakehostPackage = "./hostproto/hostprototest/cmd/fakehost"
	formatModule    = "../../../format"
	alignmentModule = "../.."
	wrapperName     = "module.sh"
	buildTags       = "grammar_subset,grammar_subset_python,grammar_subset_javascript,grammar_subset_typescript,grammar_subset_tsx,grammar_subset_java,grammar_subset_kotlin"
	judgeOnlyConfig = "policy:\n  two_signal: false\n"
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
// the in-process tests exercise, and exposed answers get the same verdicts.
func TestFakehostSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("builds two binaries")
	}
	recordings := t.TempDir()
	recorder := newHarness(t, testkit.PythonParcel)
	recorder.withConfig(judgeOnlyConfig)
	recorder.withPowers(testkit.PythonPowers())
	recorder.waves = 20
	_, want := recorder.drive(hostprototest.PersonaHonest, testkit.Oracle{}, "--record", recordings)

	bin := t.TempDir()
	module := build(t, alignmentModule, binaryPackage, bin, "musthold-alignment", buildTags)
	fakehost := build(t, formatModule, fakehostPackage, bin, "fakehost")

	h := newHarness(t, testkit.PythonParcel)
	h.withConfig(judgeOnlyConfig)
	h.withPowers(testkit.PythonPowers())
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

	t.Run("the real fake host drives the real binary to the verdicts of the in-process host [verifies SP-336~1]", func(t *testing.T) {
		var output []byte
		code := cli.ExitAwaitingHost
		for wave := 0; code == cli.ExitAwaitingHost && wave < maxWavesPerDrive*2; wave++ {
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
		got, expected := results(string(output)), results(want)
		if len(got) == 0 || len(got) != len(expected) {
			t.Fatalf("%d results, want %d\n%s", len(got), len(expected), output)
		}
		for key, entry := range expected {
			if got[key].Verdict != entry.Verdict || got[key].Reason != entry.Reason {
				t.Errorf("%s: %+v, want %+v", key, got[key], entry)
			}
		}
	})
}
