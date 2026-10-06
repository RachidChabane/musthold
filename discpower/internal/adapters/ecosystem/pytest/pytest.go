// Package pytest is the Strategy of the pytest ecosystem: interpreter
// resolution, build check, node-id selection and the suite runner.
package pytest

import (
	"io/fs"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
)

// DefaultTimeout bounds one build check or suite run.
const DefaultTimeout = 5 * time.Minute

const (
	rootDirectory = "."
	pythonSuffix  = ".py"

	pytestConfigFile = "pytest.ini"
	conftestFile     = "conftest.py"
	toxFile          = "tox.ini"
	setupConfigFile  = "setup.cfg"
	pyprojectFile    = "pyproject.toml"

	toxSection       = "[pytest]"
	setupSection     = "[tool:pytest]"
	pyprojectSection = "[tool.pytest"
)

var (
	// bytecodeCaches are never copied into workspaces.
	bytecodeCaches = []string{"__pycache__", ".pytest_cache"}
	// virtualEnvironments hold installed dependencies.
	virtualEnvironments = []string{".venv"}
	// testFileGlobs name the files that hold tests or their fixtures.
	testFileGlobs = []string{"test_*.py", "*_test.py", conftestFile}
)

// Dependencies are the ports a pytest Strategy works through.
type Dependencies struct {
	Process ports.ProcessRunner
	Reader  ports.ResultsReader
	Locator ports.DefinitionLocator
	// Assignments is optional; without it module assignment edits are refused.
	Assignments ports.AssignmentLocator
	// Interpreter is the configured interpreter command; it wins over discovery.
	Interpreter []string
	// LookPath finds an executable on PATH; it defaults to exec.LookPath.
	LookPath func(string) (string, error)
	// Timeout bounds one process; it defaults to DefaultTimeout.
	Timeout time.Duration
}

var _ ports.Ecosystem = (*Strategy)(nil)

// Strategy implements ports.Ecosystem for pytest.
type Strategy struct {
	deps Dependencies
}

// New builds the strategy over its ports.
func New(deps Dependencies) *Strategy {
	if deps.LookPath == nil {
		deps.LookPath = exec.LookPath
	}
	if deps.Timeout <= 0 {
		deps.Timeout = DefaultTimeout
	}
	return &Strategy{deps: deps}
}

// Name is ecosystem.Pytest.
func (*Strategy) Name() ecosystem.Name { return ecosystem.Pytest }

// Detect recognizes a pytest project by its configuration or its conftest.
func (*Strategy) Detect(root fs.FS) (ecosystem.Detection, bool) {
	var evidence []string
	if exists(root, pytestConfigFile) {
		evidence = append(evidence, pytestConfigFile)
	}
	if exists(root, conftestFile) {
		evidence = append(evidence, conftestFile)
	}
	for file, section := range map[string]string{toxFile: toxSection, setupConfigFile: setupSection, pyprojectFile: pyprojectSection} {
		if contains(root, file, section) {
			evidence = append(evidence, file)
		}
	}
	if len(evidence) == 0 {
		return ecosystem.Detection{}, false
	}
	slices.Sort(evidence)
	return ecosystem.Detection{Name: ecosystem.Pytest, Root: rootDirectory, Evidence: evidence}, true
}

// Layout excludes bytecode caches and treats the virtual environment as
// dependencies.
func (*Strategy) Layout() ecosystem.Layout {
	return ecosystem.Layout{ExcludedDirs: bytecodeCaches, DependencyDirs: virtualEnvironments, TestFileGlobs: testFileGlobs}
}

// Toolchain resolves the interpreter.
func (s *Strategy) Toolchain() ports.ToolchainResolver { return toolchainResolver{deps: s.deps} }

// Locator is the injected tree-sitter locator.
func (s *Strategy) Locator() ports.DefinitionLocator { return s.deps.Locator }

// Assignments is the injected assignment locator, nil when none is configured.
func (s *Strategy) Assignments() ports.AssignmentLocator { return s.deps.Assignments }

// BuildCheck compiles and imports the changed modules.
func (s *Strategy) BuildCheck() ports.BuildCheck { return buildCheck{deps: s.deps} }

// Selector maps bound tests to pytest node ids.
func (*Strategy) Selector() ports.TestSelector { return selector{} }

// Runner runs pytest and reads its JUnit XML.
func (s *Strategy) Runner() ports.SuiteRunner { return runner{deps: s.deps} }

func exists(root fs.FS, name string) bool {
	_, err := fs.Stat(root, name)
	return err == nil
}

func contains(root fs.FS, name, section string) bool {
	content, err := fs.ReadFile(root, name)
	return err == nil && strings.Contains(string(content), section)
}
