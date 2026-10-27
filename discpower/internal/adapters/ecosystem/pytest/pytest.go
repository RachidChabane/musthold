// Package pytest is the Strategy of the pytest ecosystem: interpreter
// resolution, build check, node-id selection and the suite runner.
package pytest

import (
	"io/fs"
	"os/exec"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

// DefaultTimeout bounds one build check or suite run.
const DefaultTimeout = 5 * time.Minute

const (
	rootDirectory = "."
	pythonSuffix  = ".py"

	conftestFile    = "conftest.py"
	toxFile         = "tox.ini"
	setupConfigFile = "setup.cfg"
	pyprojectFile   = "pyproject.toml"
	// virtualEnvironmentMarker marks the root of a virtual environment, which
	// pytest does not collect from by default.
	virtualEnvironmentMarker = "pyvenv.cfg"

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
	// configFiles are the files pytest takes as its configuration, even empty.
	configFiles = []string{"pytest.toml", ".pytest.toml", "pytest.ini", ".pytest.ini"}
	// defaultNoRecurseDirs are pytest's default `norecursedirs` patterns.
	defaultNoRecurseDirs = []string{"*.egg", ".*", "_darcs", "build", "CVS", "dist", "node_modules", "venv", "{arch}"}
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

// Detect recognizes a pytest project by its configuration or a conftest.py at
// its root, else, by convention, by a test file or a conftest.py that pytest
// collects from the root with its default settings.
func (*Strategy) Detect(root fs.FS) (ecosystem.Detection, bool) {
	var evidence []string
	for _, file := range append(slices.Clone(configFiles), conftestFile) {
		if exists(root, file) {
			evidence = append(evidence, file)
		}
	}
	for file, section := range map[string]string{toxFile: toxSection, setupConfigFile: setupSection, pyprojectFile: pyprojectSection} {
		if contains(root, file, section) {
			evidence = append(evidence, file)
		}
	}
	if len(evidence) > 0 {
		slices.Sort(evidence)
		return ecosystem.Detection{Name: ecosystem.Pytest, Root: rootDirectory, Evidence: evidence}, true
	}
	if file, found := firstCollected(root); found {
		return ecosystem.Detection{Name: ecosystem.Pytest, Root: rootDirectory, Evidence: []string{file}, ByConvention: true}, true
	}
	return ecosystem.Detection{}, false
}

// firstCollected is the first file of the tree, in lexical walk order, that
// pytest collects by default: a test file or a conftest.py outside the
// directories its default `norecursedirs` and its virtual environment rule
// leave out.
func firstCollected(root fs.FS) (string, bool) {
	found := ""
	_ = fs.WalkDir(root, rootDirectory, func(name string, entry fs.DirEntry, walkErr error) error {
		switch {
		case walkErr != nil:
			return fs.SkipDir
		case entry.IsDir():
			if name != rootDirectory && (matchesAny(entry.Name(), defaultNoRecurseDirs) || exists(root, path.Join(name, virtualEnvironmentMarker))) {
				return fs.SkipDir
			}
			return nil
		case matchesAny(entry.Name(), testFileGlobs):
			found = name
			return fs.SkipAll
		}
		return nil
	})
	return found, found != ""
}

// matchesAny reports whether the name matches one of the glob patterns.
func matchesAny(name string, patterns []string) bool {
	return slices.ContainsFunc(patterns, func(pattern string) bool {
		matched, err := path.Match(pattern, name)
		return err == nil && matched
	})
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
