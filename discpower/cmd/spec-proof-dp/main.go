// Command spec-proof-dp measures the discriminating power of the tests bound to each requirement, by executing them against mutants.
package main

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/RachidChabane/spec-proof/discpower/internal/cli"
	buildversion "github.com/RachidChabane/spec-proof/discpower/internal/version"
	"github.com/RachidChabane/spec-proof/format/cliconv"
)

const binaryName = "spec-proof-dp"

// version and commit are set at link time by a release build:
// -ldflags "-X main.version=<semver> -X main.commit=<sha>".
var (
	version string
	commit  string
)

func main() {
	info, _ := debug.ReadBuildInfo()
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, info))
}

// run stamps the version, answers a lone --version with the bare version and
// otherwise runs the command line.
func run(args []string, stdout, stderr io.Writer, info *debug.BuildInfo) int {
	buildversion.Stamp(version, commit, info)
	if cliconv.IsVersionRequest(args) {
		fmt.Fprintln(stdout, buildversion.Version)
		return cli.ExitClean
	}
	return cli.Main(binaryName, args, stdout, stderr)
}
