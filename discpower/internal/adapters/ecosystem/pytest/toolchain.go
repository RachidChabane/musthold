package pytest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
)

// ErrNoInterpreter is returned when no Python interpreter can be found.
var ErrNoInterpreter = errors.New("no python interpreter found")

const (
	uvLockFile      = "uv.lock"
	uvExecutable    = "uv"
	uvRunArgument   = "run"
	python3Name     = "python3"
	pythonName      = "python"
	windowsOS       = "windows"
	virtualEnvPOSIX = ".venv/bin/python"
	virtualEnvWin   = `.venv\Scripts\python.exe`
)

type toolchainResolver struct {
	deps Dependencies
}

// Resolve picks the interpreter: the configured command, then the project's
// virtual environment, then `uv run python` for a uv project (one with a
// lock file), then python3.
func (r toolchainResolver) Resolve(_ context.Context, workspace ecosystem.Workspace) (ecosystem.Toolchain, error) {
	if len(r.deps.Interpreter) > 0 {
		return ecosystem.Toolchain{Command: append([]string{}, r.deps.Interpreter...)}, nil
	}
	if venv := filepath.Join(workspace.Dir, virtualEnvPath()); isFile(venv) {
		return ecosystem.Toolchain{Command: []string{venv}}, nil
	}
	if isFile(filepath.Join(workspace.Dir, uvLockFile)) {
		if _, err := r.deps.LookPath(uvExecutable); err == nil {
			return ecosystem.Toolchain{Command: []string{uvExecutable, uvRunArgument, pythonName}}, nil
		}
	}
	for _, name := range []string{python3Name, pythonName} {
		if path, err := r.deps.LookPath(name); err == nil {
			return ecosystem.Toolchain{Command: []string{path}}, nil
		}
	}
	return ecosystem.Toolchain{}, ErrNoInterpreter
}

func virtualEnvPath() string {
	if runtime.GOOS == windowsOS {
		return virtualEnvWin
	}
	return virtualEnvPOSIX
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
