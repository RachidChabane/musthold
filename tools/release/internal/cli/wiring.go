package cli

import (
	"io"

	"github.com/RachidChabane/musthold/tools/release/internal/adapters"
)

// Main runs the command against the real repository, toolchain and file system.
func Main(args []string, stdout, stderr io.Writer) int {
	return Run(realServices, args, stdout, stderr)
}

func realServices(root string) Services {
	git := adapters.NewGit(root)
	toolchain := adapters.NewGoToolchain(root)
	return Services{
		Repository: git,
		History:    git,
		Files:      adapters.NewWorkspace(root),
		Checks:     releaseChecks{GoToolchain: toolchain, PluginGenerator: adapters.NewPluginGenerator(root)},
		Compiler:   toolchain,
		Archiver:   adapters.ArchivePacker{},
		Inspector:  adapters.BuildInfoInspector{},
		Disk:       adapters.NewDisk(root),
		Dist:       adapters.DistDirectory{},
	}
}

// releaseChecks joins the Go toolchain and the plugin generator into the checks of
// the pre-flight.
type releaseChecks struct {
	adapters.GoToolchain
	adapters.PluginGenerator
}
