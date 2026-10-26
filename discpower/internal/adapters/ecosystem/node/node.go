// Package node holds the Strategies of the Node.js test ecosystems: Jest and
// Vitest. Both resolve their tools through the package runner of the project's
// lock file, check builds with `tsc` or `node --check`, select bound tests by
// file and title regex, run the runner's JUnit reporter and probe calls of the
// project's modules in a Node process. Only what differs between the two (how
// they are recognized, how a title is selected, how the runner is started)
// lives in a framework.
package node

import (
	"io/fs"
	"os/exec"
	"time"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

// DefaultTimeout bounds one build check or suite run.
const DefaultTimeout = 5 * time.Minute

const rootDirectory = "."

var (
	// outputDirectories are never copied into workspaces: coverage reports the
	// project regenerates. The workspace manager leaves them out at every depth,
	// node_modules included, so only names no package ships a directory by are
	// listed.
	outputDirectories = []string{"coverage", ".nyc_output"}
	// dependencyDirectories hold installed packages.
	dependencyDirectories = []string{"node_modules"}
	// testFileGlobs name the files that hold tests, their setup or the tooling
	// configuration, none of which is source to mutate.
	testFileGlobs = []string{"*.test.*", "*.spec.*", "*.config.*", "*.setup.*", "setupTests.*", "*.d.ts"}
)

// Dependencies are the ports a Node Strategy works through.
type Dependencies struct {
	Process ports.ProcessRunner
	Reader  ports.ResultsReader
	Locator ports.DefinitionLocator
	// Assignments is optional; without it module assignment edits are refused.
	Assignments ports.AssignmentLocator
	// PackageRunner is the configured command that runs the project's tools; it
	// wins over the lock file.
	PackageRunner []string
	// LookPath finds an executable on PATH; it defaults to exec.LookPath.
	LookPath func(string) (string, error)
	// Timeout bounds one process; it defaults to DefaultTimeout.
	Timeout time.Duration
}

// framework is what differs between the Node test runners.
type framework interface {
	name() ecosystem.Name
	// detect recognizes the framework in the project tree.
	detect(root fs.FS) (ecosystem.Detection, bool)
	selector() ports.TestSelector
	runner(deps Dependencies) ports.SuiteRunner
	witnessFramework
	scanFramework
}

var _ ports.Ecosystem = (*Strategy)(nil)

// Strategy implements ports.Ecosystem for one Node test runner.
type Strategy struct {
	deps      Dependencies
	framework framework
	// scanCache keeps what a scan reads once of the runner's configuration.
	scanCache *scanCache
}

// NewJest builds the Jest strategy over its ports.
func NewJest(deps Dependencies) *Strategy { return newStrategy(deps, jest{}) }

// NewVitest builds the Vitest strategy over its ports.
func NewVitest(deps Dependencies) *Strategy { return newStrategy(deps, vitest{}) }

func newStrategy(deps Dependencies, framework framework) *Strategy {
	if deps.LookPath == nil {
		deps.LookPath = exec.LookPath
	}
	if deps.Timeout <= 0 {
		deps.Timeout = DefaultTimeout
	}
	return &Strategy{deps: deps, framework: framework, scanCache: newScanCache()}
}

// Name is ecosystem.Jest or ecosystem.Vitest.
func (s *Strategy) Name() ecosystem.Name { return s.framework.name() }

// Detect recognizes the framework by its configuration file or its dependency.
func (s *Strategy) Detect(root fs.FS) (ecosystem.Detection, bool) { return s.framework.detect(root) }

// Layout leaves build outputs out of workspaces, treats node_modules as
// dependencies and recognizes test, setup and configuration files.
func (*Strategy) Layout() ecosystem.Layout {
	return ecosystem.Layout{ExcludedDirs: outputDirectories, DependencyDirs: dependencyDirectories, TestFileGlobs: testFileGlobs}
}

// Toolchain resolves the package runner of the project.
func (s *Strategy) Toolchain() ports.ToolchainResolver { return toolchainResolver{deps: s.deps} }

// Locator is the injected tree-sitter locator.
func (s *Strategy) Locator() ports.DefinitionLocator { return s.deps.Locator }

// Assignments is the injected assignment locator, nil when none is configured.
func (s *Strategy) Assignments() ports.AssignmentLocator { return s.deps.Assignments }

// BuildCheck type-checks TypeScript with tsc and syntax-checks JavaScript with node.
func (s *Strategy) BuildCheck() ports.BuildCheck { return buildCheck{deps: s.deps} }

// Selector maps bound tests to the runner's title selection.
func (s *Strategy) Selector() ports.TestSelector { return s.framework.selector() }

// Runner runs the framework and reads its JUnit XML.
func (s *Strategy) Runner() ports.SuiteRunner { return s.framework.runner(s.deps) }
