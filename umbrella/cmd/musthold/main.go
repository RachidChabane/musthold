// Command musthold is the dispatcher of the musthold modules: each command
// runs in the binary of the module that owns it.
package main

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime/debug"

	"github.com/RachidChabane/musthold/format/cliconv"
	"github.com/RachidChabane/musthold/umbrella/internal/dispatch"
)

// version is set at link time by a release build: -ldflags "-X main.version=<semver>".
var version string

// executableBits are the permission bits of a file anyone may execute.
const executableBits = 0o111

func main() {
	info, _ := debug.ReadBuildInfo()
	executable, _ := os.Executable()
	os.Exit(dispatch.Run(dispatch.Env{
		Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
		Executable: executable, IsExecutable: isExecutable, LookPath: exec.LookPath,
		Process: process{}, Version: cliconv.ResolveVersion(version, info),
	}, os.Args[1:]))
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&executableBits != 0
}

// process runs a binary on the dispatcher's own streams.
type process struct{}

func (process) Run(path string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	command := exec.Command(path, args...)
	command.Stdin, command.Stdout, command.Stderr = stdin, stdout, stderr
	err := command.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), nil
	}
	if err != nil {
		return 0, err
	}
	return 0, nil
}
