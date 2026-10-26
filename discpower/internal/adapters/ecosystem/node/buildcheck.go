package node

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

const (
	tsconfigFile   = "tsconfig.json"
	typescriptTool = "tsc"
	noEmitFlag     = "--noEmit"
	projectFlag    = "-p"
	nodeCommand    = "node"
	checkFlag      = "--check"
)

var (
	// typescriptSuffixes are the sources tsc checks.
	typescriptSuffixes = []string{".ts", ".tsx", ".mts", ".cts"}
	// checkableSuffixes are the sources node can check the syntax of.
	checkableSuffixes = []string{".js", ".mjs", ".cjs"}
)

type buildCheck struct {
	deps Dependencies
}

// Check type-checks the project with `tsc --noEmit -p tsconfig.json` when a
// TypeScript file changed and the project has a tsconfig, and checks the syntax
// of each changed JavaScript file with `node --check`. A failure of either turns
// every test outcome into "error".
func (b buildCheck) Check(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, changed []string) (ecosystem.BuildResult, error) {
	if hasSuffix(changed, typescriptSuffixes) && isFile(filepath.Join(workspace.Dir, tsconfigFile)) {
		argv := append(append([]string{}, toolchain.Command...), typescriptTool, noEmitFlag, projectFlag, tsconfigFile)
		if result, err := b.run(ctx, workspace, argv); err != nil || !result.OK {
			return result, err
		}
	}
	for _, file := range changed {
		if !slices.Contains(checkableSuffixes, path.Ext(file)) {
			continue
		}
		node, err := b.deps.LookPath(nodeCommand)
		if err != nil {
			return ecosystem.BuildResult{}, fmt.Errorf("build check: %w", err)
		}
		if result, err := b.run(ctx, workspace, []string{node, checkFlag, file}); err != nil || !result.OK {
			return result, err
		}
	}
	return ecosystem.BuildResult{OK: true}, nil
}

func (b buildCheck) run(ctx context.Context, workspace ecosystem.Workspace, argv []string) (ecosystem.BuildResult, error) {
	result, err := b.deps.Process.Run(ctx, ports.Command{Argv: argv, Dir: workspace.Dir, Env: quietEnvironment(), Timeout: b.deps.Timeout})
	if err != nil {
		return ecosystem.BuildResult{}, fmt.Errorf("build check: %w", err)
	}
	if result.TimedOut || result.ExitCode != 0 {
		return ecosystem.BuildResult{Diagnostics: strings.TrimSpace(strings.Join(argv, " ") + ": " + string(result.Stdout) + string(result.Stderr))}, nil
	}
	return ecosystem.BuildResult{OK: true}, nil
}

func hasSuffix(files, suffixes []string) bool {
	return slices.ContainsFunc(files, func(file string) bool { return slices.Contains(suffixes, path.Ext(file)) })
}

func isFile(name string) bool {
	info, err := os.Stat(name)
	return err == nil && info.Mode().IsRegular()
}
