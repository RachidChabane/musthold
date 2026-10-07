package jvm

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
)

// ErrNoToolchain is returned when neither the project's wrapper nor the build
// tool can be found.
var ErrNoToolchain = errors.New("no build tool found")

const (
	wrapperPrefix  = "./"
	executableBits = 0o111
)

type toolchainResolver struct {
	s *Strategy
}

// Resolve picks the project's wrapper when the workspace holds an executable
// one, as `./mvnw` or `./gradlew`, else the build tool found on PATH.
func (r toolchainResolver) Resolve(_ context.Context, workspace ecosystem.Workspace) (ecosystem.Toolchain, error) {
	wrapper := r.s.tool.wrapperName()
	if info, err := os.Stat(filepath.Join(workspace.Dir, wrapper)); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&executableBits != 0 {
		return ecosystem.Toolchain{Command: []string{wrapperPrefix + wrapper}}, nil
	}
	if found, err := r.s.deps.LookPath(r.s.tool.executableName()); err == nil {
		return ecosystem.Toolchain{Command: []string{found}}, nil
	}
	return ecosystem.Toolchain{}, ErrNoToolchain
}
