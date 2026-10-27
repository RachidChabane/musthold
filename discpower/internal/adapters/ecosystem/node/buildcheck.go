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
	// diagnosticProjectSeparator follows the project a diagnostic key names.
	diagnosticProjectSeparator = ": "
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

// Baseline records the diagnostics `tsc` reports on the reference for each
// TypeScript project of the workspace (see workspaceProjects): a reference
// whose tests pass can still fail the type check, because Vitest and Jest
// strip types without checking them, and the tsconfig at the root of a
// monorepo may hold only references or not check the packages alone.
func (b buildCheck) Baseline(ctx context.Context, reference ecosystem.Workspace, toolchain ecosystem.Toolchain) (ecosystem.BuildBaseline, error) {
	var keys []string
	for _, project := range workspaceProjects(reference.Dir) {
		result, err := b.typecheck(ctx, reference, toolchain, project)
		if err != nil {
			return ecosystem.BuildBaseline{}, err
		}
		keys = append(keys, diagnosticKeys(result, reference.Dir, project)...)
	}
	return ecosystem.NewBuildBaseline(keys), nil
}

// Check type-checks, with `tsc --noEmit -p <project>`, each TypeScript project
// that owns a changed TypeScript file (see owningProjects), and checks the
// syntax of each changed JavaScript file with `node --check`. A type check
// fails on the diagnostics the workspace adds to the baseline, compared by
// project, file, code and message with positions and the workspace directory
// left out, and on a run that timed out. A failure of either turns every test
// outcome into "error".
func (b buildCheck) Check(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, changed []string, baseline ecosystem.BuildBaseline) (ecosystem.BuildResult, error) {
	for _, project := range changedProjects(workspace.Dir, changed) {
		if result, err := b.checkTypes(ctx, workspace, toolchain, project, baseline); err != nil || !result.OK {
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

// changedProjects are the TypeScript projects that own a changed TypeScript
// file, in sorted order without repetition.
func changedProjects(dir string, changed []string) []string {
	var projects []string
	for _, file := range changed {
		if slices.Contains(typescriptSuffixes, path.Ext(file)) {
			projects = append(projects, owningProjects(dir, file)...)
		}
	}
	slices.Sort(projects)
	return slices.Compact(projects)
}

func (b buildCheck) checkTypes(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, project string, baseline ecosystem.BuildBaseline) (ecosystem.BuildResult, error) {
	result, err := b.typecheck(ctx, workspace, toolchain, project)
	if err != nil {
		return ecosystem.BuildResult{}, err
	}
	if !result.TimedOut && result.ExitCode == 0 {
		return ecosystem.BuildResult{OK: true}, nil
	}
	keys := diagnosticKeys(result, workspace.Dir, project)
	introduced := baseline.Introduced(keys)
	if !result.TimedOut && len(keys) > 0 && len(introduced) == 0 {
		return ecosystem.BuildResult{OK: true}, nil
	}
	command := strings.Join(typecheckArgv(toolchain, project), " ")
	return ecosystem.BuildResult{Diagnostics: strings.TrimSpace(command + ": " + strings.Join(introduced, "\n"))}, nil
}

func (b buildCheck) typecheck(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, project string) (ports.CommandResult, error) {
	result, err := b.deps.Process.Run(ctx, ports.Command{Argv: typecheckArgv(toolchain, project), Dir: workspace.Dir, Env: quietEnvironment(), Timeout: b.deps.Timeout})
	if err != nil {
		return ports.CommandResult{}, fmt.Errorf("build check: %w", err)
	}
	return result, nil
}

func typecheckArgv(toolchain ecosystem.Toolchain, project string) []string {
	return append(append([]string{}, toolchain.Command...), typescriptTool, noEmitFlag, projectFlag, project)
}

// diagnosticKeys reads the diagnostics of a tsc run of a project, one per
// output line, each led by the project and without the workspace directory
// and the position that follows a file name.
func diagnosticKeys(result ports.CommandResult, workspaceDir, project string) []string {
	var keys []string
	for _, line := range strings.Split(string(result.Stdout)+"\n"+string(result.Stderr), "\n") {
		line = strings.TrimSpace(strings.ReplaceAll(line, workspaceDir+string(filepath.Separator), ""))
		if line != "" {
			keys = append(keys, project+diagnosticProjectSeparator+diagnosticPosition.ReplaceAllString(line, ""))
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

func isFile(name string) bool {
	info, err := os.Stat(name)
	return err == nil && info.Mode().IsRegular()
}
