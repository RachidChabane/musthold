// Command musthold-verify checks signed proofs and writes the verified set. It reads files only and links no os/exec.
package main

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/RachidChabane/musthold/format/cliconv"
	"github.com/RachidChabane/musthold/verify/internal/cli"
	buildversion "github.com/RachidChabane/musthold/verify/internal/version"
)

const binaryName = "musthold-verify"

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
