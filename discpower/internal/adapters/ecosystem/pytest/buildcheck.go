package pytest

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
)

const (
	moduleFlag          = "-m"
	compileModule       = "py_compile"
	codeFlag            = "-c"
	importScript        = "import importlib, sys; importlib.import_module(sys.argv[1])"
	srcLayoutDirectory  = "src"
	packageInit         = "__init__"
	moduleSeparator     = "."
	noBytecodeVariable  = "PYTHONDONTWRITEBYTECODE=1"
	pythonPathVariable  = "PYTHONPATH="
	compileFailedFormat = "%s failed: %s"
)

type buildCheck struct {
	deps Dependencies
}

// Check compiles the changed files with `py -m py_compile`, then imports each
// changed module. A failure of either turns every test outcome into "error".
func (b buildCheck) Check(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, changed []string) (ecosystem.BuildResult, error) {
	sources := pythonSources(changed)
	if len(sources) == 0 {
		return ecosystem.BuildResult{OK: true}, nil
	}
	compile := append(append([]string{}, toolchain.Command...), moduleFlag, compileModule)
	compile = append(compile, sources...)
	if result, err := b.run(ctx, workspace, compile, nil); err != nil || !result.OK {
		return result, err
	}
	for _, source := range sources {
		importCommand := append(append([]string{}, toolchain.Command...), codeFlag, importScript, moduleName(source))
		env := []string{pythonPathVariable + importPath(workspace)}
		if result, err := b.run(ctx, workspace, importCommand, env); err != nil || !result.OK {
			return result, err
		}
	}
	return ecosystem.BuildResult{OK: true}, nil
}

func (b buildCheck) run(ctx context.Context, workspace ecosystem.Workspace, argv, env []string) (ecosystem.BuildResult, error) {
	result, err := b.deps.Process.Run(ctx, ports.Command{
		Argv:    argv,
		Dir:     workspace.Dir,
		Env:     append([]string{noBytecodeVariable}, env...),
		Timeout: b.deps.Timeout,
	})
	if err != nil {
		return ecosystem.BuildResult{}, fmt.Errorf("build check: %w", err)
	}
	if result.TimedOut || result.ExitCode != 0 {
		return ecosystem.BuildResult{Diagnostics: fmt.Sprintf(compileFailedFormat, strings.Join(argv, " "), result.Stderr)}, nil
	}
	return ecosystem.BuildResult{OK: true}, nil
}

func pythonSources(changed []string) []string {
	var sources []string
	for _, file := range changed {
		if path.Ext(file) == pythonSuffix {
			sources = append(sources, file)
		}
	}
	return sources
}

// moduleName turns a source path into the dotted name it is imported by, from
// the project root or from the `src` directory of a src layout.
func moduleName(source string) string {
	name := strings.TrimSuffix(strings.TrimPrefix(source, srcLayoutDirectory+"/"), pythonSuffix)
	name = strings.ReplaceAll(name, "/", moduleSeparator)
	return strings.TrimSuffix(strings.TrimSuffix(name, moduleSeparator+packageInit), packageInit)
}

// importPath lists the directories modules are imported from: the project root
// and its `src` directory.
func importPath(workspace ecosystem.Workspace) string {
	return strings.Join([]string{workspace.Dir, filepath.Join(workspace.Dir, srcLayoutDirectory)}, string(filepath.ListSeparator))
}
