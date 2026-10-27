// Package dockertest gives the container tests of the loop-1 packages the
// study's runner on the study's engine: the image of the product's manifest, the
// product directory and a scratch directory under the work directory, the
// engine's only read-write mount. A container test runs only when Gate is 1,
// with the engine started (experiments/loop-1/script/engine start) and the
// product built (experiments/loop-1/script/build).
package dockertest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box/docker"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/product"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// Gate enables the container tests when it is set to 1.
const Gate = "MUSTHOLD_LOOP1_CONTAINERS"

const scratchMode = 0o755

// World is the study's engine as a container test sees it.
type World struct {
	Runner *docker.Runner
	// Repo is the repository root; Client the docker command that names the
	// study's engine.
	Repo, Client string
	Manifest     product.Manifest
	// Product is the product directory and Work a scratch directory under the
	// work directory, removed when the test ends; both have their symbolic links
	// resolved, as the engine mounts them.
	Product, Work string
}

// Enabled reports whether the container tests run.
func Enabled() bool { return os.Getenv(Gate) == "1" }

// Skip skips a test while the container tests are not enabled.
func Skip(t testing.TB) {
	t.Helper()
	if !Enabled() {
		t.Skipf("set %s=1, with the study's engine running and the product built, to run the container tests", Gate)
	}
}

// New readies the world, or skips the test while the container tests are not
// enabled. A missing product fails the test.
func New(t testing.TB) World {
	t.Helper()
	Skip(t)
	repo := RepoRoot(t)
	productDir, err := filepath.EvalSymlinks(study.DefaultProduct())
	if err != nil {
		t.Fatalf("no product: %v (run %s/script/build)", err, study.StudyDir)
	}
	data, err := os.ReadFile(filepath.Join(productDir, product.ManifestFile))
	if err != nil {
		t.Fatalf("no product: %v (run %s/script/build)", err, study.StudyDir)
	}
	manifest, err := product.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := docker.StudyRunner(repo, productDir, manifest.Images.Built)
	if err != nil {
		t.Fatal(err)
	}
	return World{
		Runner: runner, Repo: repo, Client: filepath.Join(repo, study.DockerClient), Manifest: manifest,
		Product: productDir, Work: Scratch(t, "dockertest-"),
	}
}

// Scratch is a fresh directory under the work directory, removed when the test
// ends, with its symbolic links resolved.
func Scratch(t testing.TB, prefix string) string {
	t.Helper()
	base := study.DefaultWork()
	if err := os.MkdirAll(base, scratchMode); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(base, prefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

// RepoRoot is the repository root above the test's directory: the directory
// that holds the study's configuration.
func RepoRoot(t testing.TB) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for ; dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(study.ConfigFile))); err == nil {
			return dir
		}
	}
	t.Fatal("no repository root above the test's directory")
	return ""
}
