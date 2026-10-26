// Command musthold-alignment judges whether the tests bound to a requirement verify what it states.
package main

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/RachidChabane/musthold/alignment/internal/cli"
	buildversion "github.com/RachidChabane/musthold/alignment/internal/version"
	"github.com/RachidChabane/musthold/format/cliconv"
)

const binaryName = "musthold-alignment"

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
