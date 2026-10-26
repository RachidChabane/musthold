// Package jvmenv prepares the machine for the tests that run a real JVM build
// tool: it skips them, recording why, when no JVM can run, keeps the JVMs small,
// and fetches a fixture's dependencies once with the network on so that every
// build after it runs offline.
package jvmenv

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/ecosystem/jvm"
)

const (
	javaLauncher  = "java"
	mavenCommand  = "mvn"
	gradleWrapper = "./gradlew"
	heapFlag      = "-Xmx768m"
	directoryMode = 0o755
)

// heapVariables are the environment variables that cap the heap of the JVMs a
// build tool starts.
var heapVariables = []string{"MAVEN_OPTS", "GRADLE_OPTS", "JAVA_TOOL_OPTIONS"}

// Require skips the test, with the reason, when it is run in short mode or no
// java launcher can run, and keeps the JVMs of the test small.
func Require(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("JVM integration tests are skipped in short mode")
	}
	if err := exec.Command(javaLauncher, "-version").Run(); err != nil {
		t.Skipf("java is not available: %v", err)
	}
	for _, name := range heapVariables {
		if os.Getenv(name) == "" {
			t.Setenv(name, heapFlag)
		}
	}
}

// CopyFixture copies a fixture project, keeping file modes: a wrapper script
// must stay executable.
func CopyFixture(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, directoryMode)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, content, info.Mode().Perm())
	})
	if err != nil {
		t.Fatal(err)
	}
}

// FetchMaven builds a Maven fixture once with the network on, in a scratch copy,
// so that its plugins and dependencies are in the local repository.
func FetchMaven(t *testing.T, fixture string) {
	t.Helper()
	fetch(t, fixture, mavenCommand, "-B", "-q", "-T", "1", "test", jvm.DependencyPlugin, "-Dmdep.outputFile="+os.DevNull)
}

// FetchGradle builds a Gradle fixture once with the network on, in a scratch
// copy, so that its plugins and dependencies are in the Gradle home.
func FetchGradle(t *testing.T, fixture string) {
	t.Helper()
	fetch(t, fixture, gradleWrapper, "--no-daemon", "--console=plain", "-q", "test")
}

func fetch(t *testing.T, fixture string, argv ...string) {
	t.Helper()
	scratch := filepath.Join(t.TempDir(), filepath.Base(fixture))
	CopyFixture(t, fixture, scratch)
	command := exec.Command(argv[0], argv[1:]...)
	command.Dir = scratch
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Run(); err != nil {
		t.Fatalf("fetch the dependencies of %s with %v: %v\n%s", fixture, argv, err, output.String())
	}
}
