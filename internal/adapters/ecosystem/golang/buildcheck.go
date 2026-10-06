package golang

import (
	"context"
	"fmt"
	"path"
	"slices"

	"github.com/RachidChabane/spec-proof/internal/app/ports"
	"github.com/RachidChabane/spec-proof/internal/domain/ecosystem"
)

const (
	testSubcommand   = "test"
	countFlag        = "-count=1"
	runFlag          = "-run"
	matchNothing     = "^$"
	localPackagePath = "./"
	allPackages      = "./..."
	parentDirectory  = "."
)

type buildCheck struct {
	deps Dependencies
}

// Check compiles the test binaries of the packages holding the changed files
// with `go test -count=1 -run '^$' <pkgs>`; no test body runs.
func (b buildCheck) Check(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, changed []string) (ecosystem.BuildResult, error) {
	argv := append(append([]string{}, toolchain.Command...), testSubcommand, countFlag, runFlag, matchNothing)
	argv = append(argv, packagesOf(changed)...)
	result, err := b.deps.Process.Run(ctx, ports.Command{Argv: argv, Dir: workspace.Dir, Timeout: b.deps.Timeout})
	if err != nil {
		return ecosystem.BuildResult{}, fmt.Errorf("build check: %w", err)
	}
	if result.TimedOut || result.ExitCode != 0 {
		return ecosystem.BuildResult{Diagnostics: string(result.Stdout) + string(result.Stderr)}, nil
	}
	return ecosystem.BuildResult{OK: true}, nil
}

// packagesOf lists the packages of the changed Go files as relative package
// paths; with no changed Go file it names every package.
func packagesOf(files []string) []string {
	var packages []string
	for _, file := range files {
		if path.Ext(file) != goSuffix {
			continue
		}
		if pkg := packagePath(path.Dir(file)); !slices.Contains(packages, pkg) {
			packages = append(packages, pkg)
		}
	}
	if len(packages) == 0 {
		return []string{allPackages}
	}
	slices.Sort(packages)
	return packages
}

func packagePath(directory string) string {
	if directory == parentDirectory {
		return parentDirectory
	}
	return localPackagePath + directory
}
