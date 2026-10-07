package node

import (
	"context"
	"io/fs"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/suiterun"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
)

const (
	vitestPackage         = "vitest"
	vitestConfigPrefix    = "vitest.config."
	vitestWorkspacePrefix = "vitest.workspace."

	vitestTool          = "vitest"
	vitestRunCommand    = "run"
	vitestReporterFlag  = "--reporter=junit"
	vitestOutputFlag    = "--outputFile="
	vitestOneWorkerFlag = "--maxWorkers=1"
	vitestTitleFlag     = "-t"

	// vitestTitleSeparator joins the titles of the enclosing describe blocks and
	// the test in the name Vitest's JUnit reporter writes. Its title filter
	// joins them with the same separator in recent releases and with a space in
	// older ones, so a selection accepts both.
	vitestTitleSeparator = " > "
)

type vitest struct{}

func (vitest) name() ecosystem.Name { return ecosystem.Vitest }

// detect recognizes Vitest by vitest.config.*, vitest.workspace.* or a vitest
// dependency.
func (vitest) detect(root fs.FS) (ecosystem.Detection, bool) {
	evidence := configFiles(root, vitestConfigPrefix, vitestWorkspacePrefix)
	if manifest, ok := readManifest(root); ok && manifest.depends(vitestPackage) {
		evidence = append(evidence, packageFile)
	}
	return detection(ecosystem.Vitest, evidence)
}

func (vitest) selector() ports.TestSelector { return vitestSelector{} }

func (vitest) runner(deps Dependencies) ports.SuiteRunner { return vitestRunner{deps: deps} }

type vitestSelector struct{}

// Select selects the bound tests by an anchored regex over their full titles,
// then by file. Vitest names a test as its describe titles and its own title
// joined by " > ".
func (vitestSelector) Select(bound []ecosystem.BoundTest, _ map[string][]ecosystem.Definition) (ecosystem.Selection, error) {
	quote := func(name string) string { return quoteTitle(name, vitestTitleSeparator) }
	return selectByTitle(bound, quote, func(titles titleSelection) []string {
		return append([]string{vitestTitleFlag, titles.expression}, titles.files...)
	})
}

type vitestRunner struct {
	deps Dependencies
}

// Run executes `vitest run --reporter=junit --outputFile=<f> --maxWorkers=1
// <selection>` and reads the outcomes from the XML. Tests the title filter left
// out are reported as skipped and dropped, as for Jest.
func (r vitestRunner) Run(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, selection ecosystem.Selection) (ecosystem.RunResult, error) {
	results := suiterun.ResultsFile(workspace, resultsExtension)
	argv := append(append([]string{}, toolchain.Command...), vitestTool, vitestRunCommand, vitestReporterFlag, vitestOutputFlag+results, vitestOneWorkerFlag)
	argv = append(argv, selection.Args...)
	result, err := suiterun.Run(ctx, r.deps.Process, r.deps.Reader, suiterun.Request{
		Command:         ports.Command{Argv: argv, Dir: workspace.Dir, Env: quietEnvironment(), Timeout: r.deps.Timeout},
		ResultsPatterns: []string{results},
		Format:          ecosystem.ReportVitest,
	})
	return dropUnselected(result, selection), err
}
