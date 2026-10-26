package jvm

import (
	"context"
	"fmt"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

type buildCheck struct {
	ports.NoBuildBaseline
	s *Strategy
}

// Check compiles the main and test sources of the project with the build tool,
// offline; the changed files are compiled with it, since both tools compile
// incrementally by what changed. A failure turns every test outcome into
// "error".
func (b buildCheck) Check(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, _ []string, _ ecosystem.BuildBaseline) (ecosystem.BuildResult, error) {
	argv := append(append([]string{}, toolchain.Command...), b.s.tool.buildCheckArgs()...)
	result, err := b.s.deps.Process.Run(ctx, ports.Command{Argv: argv, Dir: workspace.Dir, Timeout: b.s.deps.Timeout})
	if err != nil {
		return ecosystem.BuildResult{}, fmt.Errorf("build check: %w", err)
	}
	if result.TimedOut || result.ExitCode != 0 {
		return ecosystem.BuildResult{Diagnostics: string(result.Stdout) + string(result.Stderr)}, nil
	}
	return ecosystem.BuildResult{OK: true}, nil
}
