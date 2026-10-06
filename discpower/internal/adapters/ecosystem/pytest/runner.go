package pytest

import (
	"context"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/suiterun"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
)

const (
	pytestModule      = "pytest"
	noCacheProvider   = "-p"
	noCacheProviderID = "no:cacheprovider"
	configOption      = "-o"
	junitFamily       = "junit_family=xunit1"
	junitXMLFlag      = "--junitxml="
	resultsExtension  = ".xml"

	// Exit codes of pytest for an interrupted or unusable run: 2 interrupted
	// (collection errors), 3 internal error, 4 command line usage error.
	exitInterrupted = 2
	exitInternal    = 3
	exitUsage       = 4
)

type runner struct {
	deps Dependencies
}

// Run executes `py -m pytest -p no:cacheprovider -o junit_family=xunit1
// --junitxml=<f> <node ids>` with bytecode writing disabled, and reads the
// outcomes from the XML.
func (r runner) Run(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, selection ecosystem.Selection) (ecosystem.RunResult, error) {
	results := suiterun.ResultsFile(workspace, resultsExtension)
	argv := append(append([]string{}, toolchain.Command...), moduleFlag, pytestModule,
		noCacheProvider, noCacheProviderID, configOption, junitFamily, junitXMLFlag+results)
	argv = append(argv, selection.Args...)
	return suiterun.Run(ctx, r.deps.Process, r.deps.Reader, suiterun.Request{
		Command:                ports.Command{Argv: argv, Dir: workspace.Dir, Env: []string{noBytecodeVariable}, Timeout: r.deps.Timeout},
		ResultsPatterns:        []string{results},
		Format:                 ecosystem.ReportPytest,
		CollectionFailureExits: []int{exitInterrupted, exitInternal, exitUsage},
	})
}
