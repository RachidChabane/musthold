// Package nodeinstall gives the integration tests an npm-installed copy of a
// fixture project. The copy is installed once per lock file, with `npm ci` and
// the npm cache, under the temporary directory, and every test package that asks
// for the same fixture reuses it.
package nodeinstall

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const (
	cacheDirectory   = "spec-proof-test-node"
	nodeCommand      = "node"
	npmCommand       = "npm"
	niceCommand      = "nice"
	niceLevel        = "10"
	installedMarker  = ".installed"
	dependenciesDir  = "node_modules"
	manifestFile     = "package.json"
	lockFile         = "package-lock.json"
	digestLength     = 12
	directoryMode    = 0o755
	stagingSuffix    = ".staging-"
	outputsDirectory = "results"
	fileMode         = 0o644
)

// RequireNode skips the test, saying why, when it is not meant to run the Node
// toolchain: in short mode, or on a machine without node.
func RequireNode(t testing.TB) {
	t.Helper()
	if testing.Short() {
		t.Skip("toolchain integration tests are skipped in short mode")
	}
	if _, err := exec.LookPath(nodeCommand); err != nil {
		t.Skipf("node is not on PATH: %v", err)
	}
}

// Installed returns the directory of a copy of the fixture project with its
// dependencies installed from its lock file. An install that fails fails the
// test.
func Installed(t testing.TB, fixtureDir string) string {
	t.Helper()
	RequireNode(t)
	digest, err := lockDigest(fixtureDir)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(os.TempDir(), cacheDirectory, filepath.Base(fixtureDir)+"-"+digest)
	if _, err := os.Stat(filepath.Join(target, installedMarker)); err == nil {
		return target
	}
	staging := target + stagingSuffix + digest
	if err := os.RemoveAll(staging); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(staging) }()
	if err := copyProject(fixtureDir, staging); err != nil {
		t.Fatalf("copy the fixture %s: %v", fixtureDir, err)
	}
	install := exec.Command(niceCommand, "-n", niceLevel, npmCommand, "ci", "--prefer-offline", "--no-audit", "--no-fund")
	install.Dir = staging
	if output, err := install.CombinedOutput(); err != nil {
		t.Fatalf("npm ci in %s: %v\n%s", fixtureDir, err, output)
	}
	if err := os.WriteFile(filepath.Join(staging, installedMarker), nil, fileMode); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(staging, target); err != nil {
		t.Fatalf("publish the installed fixture: %v", err)
	}
	return target
}

// Project returns a fresh copy of the fixture project, in a directory of the
// test, whose node_modules links to the installed one: a project of its own to
// edit, run and clone at the cost of its sources.
func Project(t testing.TB, fixtureDir string) string {
	t.Helper()
	installed := Installed(t, fixtureDir)
	project := filepath.Join(t.TempDir(), filepath.Base(fixtureDir))
	if err := copyProject(fixtureDir, project); err != nil {
		t.Fatalf("copy the fixture %s: %v", fixtureDir, err)
	}
	if err := os.Symlink(filepath.Join(installed, dependenciesDir), filepath.Join(project, dependenciesDir)); err != nil {
		t.Fatal(err)
	}
	return project
}

// lockDigest identifies the dependencies a fixture asks for.
func lockDigest(fixtureDir string) (string, error) {
	hash := sha256.New()
	for _, name := range []string{manifestFile, lockFile} {
		content, err := os.ReadFile(filepath.Join(fixtureDir, name))
		if err != nil {
			return "", err
		}
		hash.Write(content)
	}
	return hex.EncodeToString(hash.Sum(nil))[:digestLength], nil
}

func copyProject(from, to string) error {
	return filepath.WalkDir(from, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, relative)
		if entry.IsDir() {
			if entry.Name() == dependenciesDir || entry.Name() == outputsDirectory {
				return fs.SkipDir
			}
			return os.MkdirAll(target, directoryMode)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, content, fileMode)
	})
}
