package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/tools/release/internal/adapters"
	"github.com/RachidChabane/spec-proof/tools/release/internal/domain"
	"github.com/RachidChabane/spec-proof/tools/release/internal/testkit"
)

const (
	verifiesInstaller = " [verifies SP-1821~1]"
	installerVersion  = "0.1.0"
	installerModule   = "discpower"
	installerBinary   = "spec-proof-dp"
	exitUsage         = 2
	exitNoVerifier    = 3
	exitChecksum      = 6
	exitVersion       = 7
	exitUnsupported   = 8
)

// hostTarget is the release target of the machine running the test.
func hostTarget(t *testing.T) domain.Target {
	t.Helper()
	target, err := domain.ParseTarget(runtime.GOOS + "/" + runtime.GOARCH)
	if err != nil || target.OS == domain.OSWindows {
		t.Skipf("the installer test runs a shell script as the binary: %s is not a unix target of the matrix", runtime.GOOS+"/"+runtime.GOARCH)
	}
	return target
}

// releaseDirectory writes a release directory holding one archive for the host
// whose binary is a shell script that prints reported, with its SHA256SUMS.
func releaseDirectory(t *testing.T, reported string) (directory, archive string) {
	t.Helper()
	target := hostTarget(t)
	directory = t.TempDir()
	script := "#!/bin/sh\necho " + reported + "\n"
	entries := domain.ArchiveEntries(target, installerBinary, domain.ArchiveContent{Binary: []byte(script), License: []byte("licence\n"), Readme: []byte("readme\n")})
	packed, err := adapters.ArchivePacker{}.Pack(target.Format(), entries, time.Unix(1_700_000_000, 0))
	if err != nil {
		t.Fatal(err)
	}
	version := domain.Version{Major: 0, Minor: 1, Patch: 0}
	archive = target.ArchiveName(installerBinary, version)
	sums := []domain.Checksum{{Digest: domain.DigestOf(packed), Name: archive}}
	for name, data := range map[string][]byte{archive: packed, domain.ChecksumsFileName: []byte(domain.RenderChecksums(sums))} {
		if err := os.WriteFile(filepath.Join(directory, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return directory, archive
}

// install runs script/install with the arguments and returns its output and exit
// code.
func install(t *testing.T, environment []string, arguments ...string) (string, int) {
	t.Helper()
	command := exec.Command("bash", append([]string{filepath.Join(testkit.RepositoryRoot(t), "script", "install")}, arguments...)...)
	command.Env = append(os.Environ(), environment...)
	output, err := command.CombinedOutput()
	if exit, isExit := err.(*exec.ExitError); isExit {
		return string(output), exit.ExitCode()
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(output), 0
}

func nothingInstalled(t *testing.T, prefix string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(prefix, "bin"))
	if err == nil && len(entries) > 0 {
		t.Errorf("the refused install left %d file(s) in %s/bin", len(entries), prefix)
	}
}

func TestInstaller(t *testing.T) {
	t.Run("a release directory whose binary reports the version is installed and runs"+verifiesInstaller, func(t *testing.T) {
		directory, _ := releaseDirectory(t, installerVersion)
		prefix := t.TempDir()
		output, code := install(t, nil, installerModule, installerVersion, "--from", directory, "--prefix", prefix)
		if code != 0 {
			t.Fatalf("exit %d: %s", code, output)
		}
		reported, err := exec.Command(filepath.Join(prefix, "bin", installerBinary), "--version").Output()
		if err != nil || strings.TrimSpace(string(reported)) != installerVersion {
			t.Fatalf("installed binary reports %q, error %v", reported, err)
		}
	})

	t.Run("a binary that reports another version is refused and nothing is installed"+verifiesInstaller, func(t *testing.T) {
		directory, _ := releaseDirectory(t, "9.9.9")
		prefix := t.TempDir()
		output, code := install(t, nil, installerModule, installerVersion, "--from", directory, "--prefix", prefix)
		if code != exitVersion || !strings.Contains(output, "reports version 9.9.9") {
			t.Fatalf("exit %d: %s", code, output)
		}
		nothingInstalled(t, prefix)
	})

	t.Run("a tampered archive is refused and nothing is installed"+verifiesInstaller, func(t *testing.T) {
		directory, archive := releaseDirectory(t, installerVersion)
		path := filepath.Join(directory, archive)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		data[len(data)/2] ^= 0xff
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		prefix := t.TempDir()
		output, code := install(t, nil, installerModule, installerVersion, "--from", directory, "--prefix", prefix)
		if code != exitChecksum || !strings.Contains(output, "lists") {
			t.Fatalf("exit %d: %s", code, output)
		}
		nothingInstalled(t, prefix)
	})

	t.Run("a checksum list that does not name the archive is refused"+verifiesInstaller, func(t *testing.T) {
		directory, _ := releaseDirectory(t, installerVersion)
		if err := os.WriteFile(filepath.Join(directory, domain.ChecksumsFileName), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, code := install(t, nil, installerModule, installerVersion, "--from", directory, "--prefix", t.TempDir()); code != exitUnsupported {
			t.Fatalf("exit %d", code)
		}
	})

	t.Run("a platform outside the matrix is refused"+verifiesInstaller, func(t *testing.T) {
		directory, _ := releaseDirectory(t, installerVersion)
		prefix := t.TempDir()
		for _, flags := range [][]string{{"--os", "plan9"}, {"--arch", "riscv64"}} {
			arguments := append([]string{installerModule, installerVersion, "--from", directory, "--prefix", prefix}, flags...)
			if output, code := install(t, nil, arguments...); code != exitUnsupported {
				t.Errorf("%v: exit %d: %s", flags, code, output)
			}
		}
		nothingInstalled(t, prefix)
	})

	t.Run("a release with no archive for the platform asked for is refused"+verifiesInstaller, func(t *testing.T) {
		directory, _ := releaseDirectory(t, installerVersion)
		other := "arm64"
		if runtime.GOARCH == "arm64" {
			other = "amd64"
		}
		if output, code := install(t, nil, installerModule, installerVersion, "--from", directory, "--prefix", t.TempDir(), "--arch", other); code != exitUnsupported {
			t.Fatalf("exit %d: %s", code, output)
		}
	})

	t.Run("without a release directory, the absence of both cosign and gh refuses the install"+verifiesInstaller, func(t *testing.T) {
		emptyPath := t.TempDir()
		prefix := t.TempDir()
		output, code := install(t, []string{"PATH=" + emptyPath}, installerModule, installerVersion, "--prefix", prefix)
		if code != exitNoVerifier || !strings.Contains(output, "neither cosign nor gh") {
			t.Fatalf("exit %d: %s", code, output)
		}
		nothingInstalled(t, prefix)
	})

	t.Run("a module, a version or an option that does not parse is a usage error"+verifiesInstaller, func(t *testing.T) {
		for name, arguments := range map[string][]string{
			"no argument":      {},
			"one argument":     {installerModule},
			"a v in front":     {installerModule, "v0.1.0"},
			"a path as module": {"../x", installerVersion},
			"an unknown flag":  {installerModule, installerVersion, "--force"},
			"a leading zero":   {installerModule, "01.2.3"},
		} {
			if output, code := install(t, nil, arguments...); code != exitUsage {
				t.Errorf("%s: exit %d: %s", name, code, output)
			}
		}
	})
}
