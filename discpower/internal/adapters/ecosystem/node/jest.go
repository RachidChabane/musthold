package node

import (
	"context"
	"io/fs"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/app/suiterun"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

const (
	jestPackage      = "jest"
	jestConfigPrefix = "jest.config."

	jestTool         = "jest"
	jestCIFlag       = "--ci"
	jestReporterFlag = "--reporters=jest-junit"
	// jestSingleProcessFlag runs the tests in the Jest process, so one run
	// starts no worker pool.
	jestSingleProcessFlag = "--runInBand"
	// jestNoWatchmanFlag keeps Jest from starting a watchman daemon.
	jestNoWatchmanFlag = "--watchman=false"
	jestByPathFlag     = "--runTestsByPath"
	jestTitleFlag      = "-t"
	jestOutputVariable = "JEST_JUNIT_OUTPUT_FILE="

	// jestTitleSeparator joins the titles of the enclosing describe blocks and
	// the test in the name jest-junit reports.
	jestTitleSeparator = " "
)

type jest struct{}

func (jest) name() ecosystem.Name { return ecosystem.Jest }

// detect recognizes Jest by jest.config.*, a jest key or a jest dependency.
func (jest) detect(root fs.FS) (ecosystem.Detection, bool) {
	evidence := configFiles(root, jestConfigPrefix)
	if manifest, ok := readManifest(root); ok && (manifest.depends(jestPackage) || len(manifest.Jest) > 0) {
		evidence = append(evidence, packageFile)
	}
	return detection(ecosystem.Jest, evidence)
}

func (jest) selector() ports.TestSelector { return jestSelector{} }

func (jest) runner(deps Dependencies) ports.SuiteRunner { return jestRunner{deps: deps} }

type jestSelector struct{}

// Select selects the bound tests by path and by an anchored regex over their
// full titles. jest-junit reports a test as its describe titles and its own
// title joined by spaces, with a leading space when no describe encloses it;
// `-t` matches the same text without that space.
func (jestSelector) Select(bound []ecosystem.BoundTest, _ map[string][]ecosystem.Definition) (ecosystem.Selection, error) {
	quote := func(name string) string { return quoteTitle(strings.TrimPrefix(name, jestTitleSeparator), "") }
	return selectByTitle(bound, quote, func(titles titleSelection) []string {
		args := append([]string{jestByPathFlag}, titles.files...)
		return append(args, jestTitleFlag, titles.expression)
	})
}

type jestRunner struct {
	deps Dependencies
}

// Run executes `jest --ci --reporters=jest-junit --runInBand --watchman=false
// <selection>` with the report path in JEST_JUNIT_OUTPUT_FILE and reads the
// outcomes from the XML. A selected run reports the tests its title filter left
// out as skipped; those are dropped, so only the selection has outcomes.
func (r jestRunner) Run(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, selection ecosystem.Selection) (ecosystem.RunResult, error) {
	results := suiterun.ResultsFile(workspace, resultsExtension)
	argv := append(append([]string{}, toolchain.Command...), jestTool, jestCIFlag, jestReporterFlag, jestSingleProcessFlag, jestNoWatchmanFlag)
	argv = append(argv, selection.Args...)
	result, err := suiterun.Run(ctx, r.deps.Process, r.deps.Reader, suiterun.Request{
		Command:         ports.Command{Argv: argv, Dir: workspace.Dir, Env: append(quietEnvironment(), jestOutputVariable+results), Timeout: r.deps.Timeout},
		ResultsPatterns: []string{results},
		Format:          ecosystem.ReportJestJUnit,
	})
	return dropUnselected(result, selection), err
}
