package node

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
)

// ErrNoToolchain is returned when the package runner of the project cannot be found.
var ErrNoToolchain = errors.New("no package runner found")

// packageRunner is the command that runs the executables a project installed,
// without downloading anything.
type packageRunner struct {
	executable string
	arguments  []string
}

// lockfileRunners maps a lock file to the package runner of the package manager
// that wrote it, in the order they are looked for.
var lockfileRunners = []struct {
	lockfiles []string
	runner    packageRunner
}{
	{[]string{"pnpm-lock.yaml"}, packageRunner{"pnpm", []string{"exec"}}},
	{[]string{"yarn.lock"}, packageRunner{"yarn", nil}},
	{[]string{"bun.lock", "bun.lockb"}, packageRunner{"bunx", []string{"--no-install"}}},
	{[]string{"package-lock.json", "npm-shrinkwrap.json"}, packageRunner{"npx", []string{"--no-install"}}},
}

// defaultRunner serves a project without a lock file.
var defaultRunner = packageRunner{"npx", []string{"--no-install"}}

type toolchainResolver struct {
	deps Dependencies
}

// Resolve picks the package runner: the configured command, then the runner of
// the package manager whose lock file the project holds, then npx.
func (r toolchainResolver) Resolve(_ context.Context, workspace ecosystem.Workspace) (ecosystem.Toolchain, error) {
	if len(r.deps.PackageRunner) > 0 {
		return ecosystem.Toolchain{Command: append([]string{}, r.deps.PackageRunner...)}, nil
	}
	runner := runnerOfLockfile(workspace.Dir)
	path, err := r.deps.LookPath(runner.executable)
	if err != nil {
		return ecosystem.Toolchain{}, ErrNoToolchain
	}
	return ecosystem.Toolchain{Command: append([]string{path}, runner.arguments...)}, nil
}

func runnerOfLockfile(dir string) packageRunner {
	for _, candidate := range lockfileRunners {
		for _, lockfile := range candidate.lockfiles {
			if info, err := os.Stat(filepath.Join(dir, lockfile)); err == nil && info.Mode().IsRegular() {
				return candidate.runner
			}
		}
	}
	return defaultRunner
}
