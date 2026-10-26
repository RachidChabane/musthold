package e2e_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Requirements the closed-loop tests verify.
const (
	verifiesHelpOfBinaries = " [verifies SP-1670~1]"
	verifiesLocalSetup     = " [verifies SP-1671~1]"
	verifiesEmptyCache     = " [verifies SP-1672~1]"

	helpRequirement       = "SP-1670~1"
	localSetupRequirement = "SP-1671~1"
	emptyCacheRequirement = "SP-1672~1"

	helpRequestedText = "help requested"
	usageText         = "usage:"
	versionCommand    = "version"
	configPath        = ".musthold/config.yaml"
	gitignorePath     = ".gitignore"
	operationsExample = "# operations:\n#   line_total:\n"

	// emptyResultsFile is the JUnit file the project's configuration reads; no test ran.
	emptyResultsFile = "test-results/pytest.xml"
	emptyResults     = "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<testsuites></testsuites>\n"
)

var (
	helpFlags       = []string{"-h", "--help"}
	localStateLines = []string{".musthold/cache/", ".musthold/verified/", ".musthold/pending/"}
)

// helpRun is what a binary printed and exited with.
type helpRun struct {
	code           int
	stdout, stderr string
}

// runHelp executes a built binary in an empty directory with an empty home.
func runHelp(t *testing.T, directory, binary string, arguments ...string) helpRun {
	t.Helper()
	home := t.TempDir()
	command := exec.Command(filepath.Join(directory, binary), arguments...)
	command.Dir = home
	command.Env = append(os.Environ(), homeVariable+"="+home,
		stateVariable+"="+filepath.Join(home, emptyStateDirectory), cacheVariable+"="+filepath.Join(home, emptyCacheDirectory))
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	var exit *exec.ExitError
	code := 0
	switch {
	case errors.As(err, &exit):
		code = exit.ExitCode()
	case err != nil:
		t.Fatalf("%s %v: %v", binary, arguments, err)
	}
	return helpRun{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

// assertHelp fails unless the arguments gave a usage on the output stream and exit 0.
func assertHelp(t *testing.T, directory, binary string, arguments ...string) {
	t.Helper()
	got := runHelp(t, directory, binary, arguments...)
	if got.code != 0 || got.stderr != "" || !strings.Contains(got.stdout, usageText) || strings.Contains(got.stdout, helpRequestedText) {
		t.Errorf("%s %v: exit %d, stdout %q, stderr %q", binary, arguments, got.code, got.stdout, got.stderr)
	}
}

// TestEveryBinaryAnswersHelp checks the help of the six built binaries: the binary,
// each of its commands and each verb of a command that takes one.
func TestEveryBinaryAnswersHelp(t *testing.T) {
	requirements := []string{helpRequirement}
	if testing.Short() {
		skipRequirements(t, requirements, "builds the binary of every module")
	}
	requireMemory(t, requirements)
	directory := productBinaries(t)

	for _, binary := range documentedBinaries {
		t.Run(binary+" answers -h, --help and help for itself, its commands and their verbs"+verifiesHelpOfBinaries, func(t *testing.T) {
			for _, request := range append(slices.Clone(helpFlags), "help") {
				assertHelp(t, directory, binary, request)
			}
			for _, command := range commandsOf(t, directory, binary) {
				if command == versionCommand || command == "help" {
					continue
				}
				for _, flag := range helpFlags {
					assertHelp(t, directory, binary, command, flag)
				}
				assertHelp(t, directory, binary, "help", command)
				if !slices.Contains(verbCommands, command) && !(binary == ledgerBinary && command == "ledger") {
					continue
				}
				if command == "hook" {
					continue
				}
				for _, verb := range verbsOf(t, directory, binary, command) {
					for _, flag := range helpFlags {
						assertHelp(t, directory, binary, command, verb, flag)
					}
				}
			}
		})
	}

	t.Run("a verb lists only the flags it defines, and a flag another verb owns is a usage error"+verifiesHelpOfBinaries, func(t *testing.T) {
		for _, binary := range []string{dpBinary, alignmentBinary, completenessBinary} {
			show := runHelp(t, directory, binary, "request", "show", "-h")
			if !strings.Contains(show.stdout, "--cap") || strings.Contains(show.stdout, "--nonce") || strings.Contains(show.stdout, "--json") {
				t.Errorf("%s request show -h:\n%s", binary, show.stdout)
			}
			push := runHelp(t, directory, binary, "host", "push", "-h")
			if !strings.Contains(push.stdout, "--remote") || strings.Contains(push.stdout, "--ci-run") {
				t.Errorf("%s host push -h:\n%s", binary, push.stdout)
			}
			if refused := runHelp(t, directory, binary, "request", "show", "--nonce", "x"); refused.code != 2 || !strings.Contains(refused.stderr, usageText) {
				t.Errorf("%s request show --nonce: exit %d, stderr %q", binary, refused.code, refused.stderr)
			}
		}
		drain := runHelp(t, directory, s1Binary, "drain", "-h")
		if !strings.Contains(drain.stdout, "--module") || !strings.Contains(drain.stdout, "--endpoint") {
			t.Errorf("drain -h:\n%s", drain.stdout)
		}
		verify := runHelp(t, directory, verifyBinary, "-h")
		if !strings.Contains(verify.stdout, "--accept-local") || !strings.Contains(verify.stdout, "--out") {
			t.Errorf("verify -h:\n%s", verify.stdout)
		}
	})
}

// TestInitAndEnableLeaveTheLocalLoopReady checks what the modules leave in a project
// that had no musthold state: the policy of the local loop, the local state kept
// out of version control and the commented operations example.
func TestInitAndEnableLeaveTheLocalLoopReady(t *testing.T) {
	requirements := []string{localSetupRequirement}
	if testing.Short() {
		skipRequirements(t, requirements, "builds the binaries and Shallnot")
	}
	p := newParcelProject(t, requirements)
	if err := os.RemoveAll(filepath.Join(p.root, ".musthold")); err != nil {
		t.Fatal(err)
	}
	p.equip()

	t.Run("enable leaves the local trust, the source root and both requirements and nothing else to write"+verifiesLocalSetup, func(t *testing.T) {
		if got := p.read("shallnot.yaml"); !strings.Contains(got, enabledPolicy) {
			t.Fatalf("shallnot.yaml:\n%s", got)
		}
	})

	t.Run("init ignores the cache, the verified set and the pending files"+verifiesLocalSetup, func(t *testing.T) {
		ignored := strings.Split(p.read(gitignorePath), "\n")
		for _, line := range localStateLines {
			if !slices.Contains(ignored, line) {
				t.Errorf(".gitignore lacks %s:\n%s", line, strings.Join(ignored, "\n"))
			}
		}
	})

	t.Run("init writes a commented operations example that a second init leaves alone"+verifiesLocalSetup, func(t *testing.T) {
		config := p.read(configPath)
		if strings.Count(config, operationsExample) != 1 || strings.Contains(config, "\noperations:") {
			t.Fatalf("config.yaml:\n%s", config)
		}
		for _, run := range []func(string, ...string) result{p.dp, p.alignment} {
			if r := run("", "init", "--hooks", "claude", "--check"); r.code != exitClean {
				t.Errorf("a second init would change something: %s", r.output())
			}
		}
	})
}

// TestAcceptLocalOnAnEmptyCache checks that the pinned Shallnot reads the manifest
// the verifier writes for a local run that has no statement yet.
func TestAcceptLocalOnAnEmptyCache(t *testing.T) {
	requirements := []string{emptyCacheRequirement}
	if testing.Short() {
		skipRequirements(t, requirements, "builds the binaries and Shallnot")
	}
	p := newParcelProject(t, requirements)
	p.equip()
	p.write(emptyResultsFile, emptyResults)
	p.verifyLocal()

	var manifest struct {
		Trust             string `json:"trust"`
		TrustedRootDigest string `json:"trustedRootDigest"`
		PolicyDigest      string `json:"policyDigest"`
	}
	if err := json.Unmarshal([]byte(p.read(verifiedPath+"/"+manifestFileName)), &manifest); err != nil {
		t.Fatal(err)
	}
	report := p.check()

	t.Run("the manifest of an empty local cache states trust local and no digest of a root or a policy"+verifiesEmptyCache, func(t *testing.T) {
		if manifest.Trust != "local" || manifest.TrustedRootDigest != "" || manifest.PolicyDigest != "" || len(p.manifest()) != 0 {
			t.Fatalf("manifest %+v", manifest)
		}
	})

	t.Run("Shallnot under trust local reads it and finds every proof missing, not the manifest malformed"+verifiesEmptyCache, func(t *testing.T) {
		for _, f := range report.findings {
			if f.Category == "malformed_proof" {
				t.Errorf("finding %+v\n%s", f, report.output)
			}
		}
		if report.code == exitToolFailed || report.code == exitClean {
			t.Errorf("shallnot check exited %d on a project with no proof\n%s", report.code, report.output)
		}
	})
}
