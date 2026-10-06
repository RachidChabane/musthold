package golang

import (
	"context"

	"github.com/RachidChabane/spec-proof/internal/app/ports"
	"github.com/RachidChabane/spec-proof/internal/app/suiterun"
	"github.com/RachidChabane/spec-proof/internal/domain/ecosystem"
)

const (
	toolSubcommand   = "tool"
	gotestsumTool    = "gotestsum"
	junitFileFlag    = "--junitfile"
	flagTerminator   = "--"
	resultsExtension = ".xml"
)

type runner struct {
	deps Dependencies
}

// Run executes `go tool gotestsum --junitfile <f> -- -count=1 -run <re> <pkgs>`
// and reads the outcomes from the XML. An empty selection runs every package.
func (r runner) Run(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, selection ecosystem.Selection) (ecosystem.RunResult, error) {
	results := suiterun.ResultsFile(workspace, resultsExtension)
	args := selection.Args
	if len(args) == 0 {
		args = []string{allPackages}
	}
	argv := append(append([]string{}, toolchain.Command...), toolSubcommand, gotestsumTool, junitFileFlag, results, flagTerminator, countFlag)
	argv = append(argv, args...)
	return suiterun.Run(ctx, r.deps.Process, r.deps.Reader, suiterun.Request{
		Command:         ports.Command{Argv: argv, Dir: workspace.Dir, Timeout: r.deps.Timeout},
		ResultsPatterns: []string{results},
		Format:          ecosystem.ReportGotestsum,
	})
}
