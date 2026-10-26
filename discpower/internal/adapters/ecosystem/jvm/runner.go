package jvm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/app/suiterun"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

type runner struct {
	s *Strategy
}

// Run clears the reports of an earlier run, executes the selection through the
// build tool and reads the outcomes from the JUnit XML it wrote. An empty
// selection runs the whole suite.
func (r runner) Run(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, selection ecosystem.Selection) (ecosystem.RunResult, error) {
	reports := r.s.tool.reports()
	directory := filepath.Join(workspace.Dir, filepath.FromSlash(reports.directory))
	if err := os.RemoveAll(directory); err != nil {
		return ecosystem.RunResult{}, fmt.Errorf("clear the reports of an earlier run: %w", err)
	}
	argv := append(append([]string{}, toolchain.Command...), r.s.tool.runArgs(selection.Args)...)
	return suiterun.Run(ctx, r.s.deps.Process, r.s.deps.Reader, suiterun.Request{
		Command:         ports.Command{Argv: argv, Dir: workspace.Dir, Timeout: r.s.deps.Timeout},
		ResultsPatterns: []string{filepath.Join(directory, reports.pattern)},
		Format:          reports.format,
	})
}
