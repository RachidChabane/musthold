package node

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
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

// diagnosticPosition is the `(line,column)` tsc prints after a file name; it
// moves when a mutant changes the lines above a diagnostic, so a diagnostic's
// key leaves it out.
var diagnosticPosition = regexp.MustCompile(`\(\d+,\d+\)`)

var (
	// typescriptSuffixes are the sources tsc checks.
	typescriptSuffixes = []string{".ts", ".tsx", ".mts", ".cts"}
	// checkableSuffixes are the sources node can check the syntax of.
	checkableSuffixes = []string{".js", ".mjs", ".cjs"}
)

type buildCheck struct {
	deps Dependencies
}

// Baseline records the diagnostics `tsc` reports on the reference, when the
// project has a tsconfig: a reference whose tests pass can still fail the type
// check, because Vitest and Jest strip types without checking them.
func (b buildCheck) Baseline(ctx context.Context, reference ecosystem.Workspace, toolchain ecosystem.Toolchain) (ecosystem.BuildBaseline, error) {
	if !isFile(filepath.Join(reference.Dir, tsconfigFile)) {
		return ecosystem.BuildBaseline{}, nil
	}
	result, err := b.typecheck(ctx, reference, toolchain)
	if err != nil {
		return ecosystem.BuildBaseline{}, err
	}
	return ecosystem.NewBuildBaseline(diagnosticKeys(result, reference.Dir)), nil
}

// Check type-checks the project with `tsc --noEmit -p tsconfig.json` when a
// TypeScript file changed and the project has a tsconfig, and checks the syntax
// of each changed JavaScript file with `node --check`. The type check fails on
// the diagnostics the workspace adds to the baseline, compared by file, code and
// message with positions and the workspace directory left out, and on a run that
// timed out. A failure of either turns every test outcome into "error".
func (b buildCheck) Check(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, changed []string, baseline ecosystem.BuildBaseline) (ecosystem.BuildResult, error) {
	if hasSuffix(changed, typescriptSuffixes) && isFile(filepath.Join(workspace.Dir, tsconfigFile)) {
		if result, err := b.checkTypes(ctx, workspace, toolchain, baseline); err != nil || !result.OK {
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

func (b buildCheck) checkTypes(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, baseline ecosystem.BuildBaseline) (ecosystem.BuildResult, error) {
	result, err := b.typecheck(ctx, workspace, toolchain)
	if err != nil {
		return ecosystem.BuildResult{}, err
	}
	if !result.TimedOut && result.ExitCode == 0 {
		return ecosystem.BuildResult{OK: true}, nil
	}
	keys := diagnosticKeys(result, workspace.Dir)
	introduced := baseline.Introduced(keys)
	if !result.TimedOut && len(keys) > 0 && len(introduced) == 0 {
		return ecosystem.BuildResult{OK: true}, nil
	}
	command := strings.Join(typecheckArgv(toolchain), " ")
	return ecosystem.BuildResult{Diagnostics: strings.TrimSpace(command + ": " + strings.Join(introduced, "\n"))}, nil
}

func (b buildCheck) typecheck(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain) (ports.CommandResult, error) {
	result, err := b.deps.Process.Run(ctx, ports.Command{Argv: typecheckArgv(toolchain), Dir: workspace.Dir, Env: quietEnvironment(), Timeout: b.deps.Timeout})
	if err != nil {
		return ports.CommandResult{}, fmt.Errorf("build check: %w", err)
	}
	return result, nil
}

func typecheckArgv(toolchain ecosystem.Toolchain) []string {
	return append(append([]string{}, toolchain.Command...), typescriptTool, noEmitFlag, projectFlag, tsconfigFile)
}

// diagnosticKeys reads the diagnostics of a tsc run, one per output line, each
// without the workspace directory and the position that follows a file name.
func diagnosticKeys(result ports.CommandResult, workspaceDir string) []string {
	var keys []string
	for _, line := range strings.Split(string(result.Stdout)+"\n"+string(result.Stderr), "\n") {
		line = strings.TrimSpace(strings.ReplaceAll(line, workspaceDir+string(filepath.Separator), ""))
		if line != "" {
			keys = append(keys, diagnosticPosition.ReplaceAllString(line, ""))
		}
	}
	return keys
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
