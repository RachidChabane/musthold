// Package ports declares the interfaces the application layer depends on.
// Adapters implement them; the application never imports an adapter.
package ports

import (
	"context"
	"io/fs"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

// Ecosystem is the Strategy of one test ecosystem. A Factory in
// internal/adapters/ecosystem chooses the strategy by Detect.
type Ecosystem interface {
	Name() ecosystem.Name
	// Detect recognizes the ecosystem in a project tree.
	Detect(root fs.FS) (ecosystem.Detection, bool)
	// Layout describes the excluded and dependency directories and the test files.
	Layout() ecosystem.Layout
	// Toolchain resolves the interpreter, package runner or wrapper.
	Toolchain() ToolchainResolver
	// Locator finds definitions with byte spans.
	Locator() DefinitionLocator
	// Assignments finds module-level assignments with byte spans. It is nil
	// when the ecosystem cannot locate them, which refuses the edit target kind
	// module assignment.
	Assignments() AssignmentLocator
	// BuildCheck is the compile or import smoke check; a failure turns every test
	// outcome into "error".
	BuildCheck() BuildCheck
	// Selector maps bound tests to the runner's native selection.
	Selector() TestSelector
	// Runner runs a selection and reads per-test outcomes from JUnit XML.
	Runner() SuiteRunner
}

// ToolchainResolver resolves the tools of an ecosystem inside a workspace.
type ToolchainResolver interface {
	Resolve(ctx context.Context, workspace ecosystem.Workspace) (ecosystem.Toolchain, error)
}

// DefinitionLocator finds the function and method definitions of one source file.
// It is a pure function of the file's path and bytes.
type DefinitionLocator interface {
	Locate(path string, source []byte) ([]ecosystem.Definition, error)
}

// AssignmentLocator finds the module-level assignments of one source file, as
// definitions of kind ecosystem.KindAssignment. It is a pure function of the
// file's path and bytes.
type AssignmentLocator interface {
	LocateAssignments(path string, source []byte) ([]ecosystem.Definition, error)
}

// BuildCheck compiles or imports the changed files of a workspace.
type BuildCheck interface {
	// Baseline records what the check already reports on the unmutated
	// reference workspace; it is empty for a check that cannot fail on a
	// reference whose tests run.
	Baseline(ctx context.Context, reference ecosystem.Workspace, toolchain ecosystem.Toolchain) (ecosystem.BuildBaseline, error)
	// Check fails on the diagnostics the workspace adds to the baseline.
	Check(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, changed []string, baseline ecosystem.BuildBaseline) (ecosystem.BuildResult, error)
}

// NoBuildBaseline is embedded by a build check whose reference always passes.
type NoBuildBaseline struct{}

// Baseline is empty.
func (NoBuildBaseline) Baseline(context.Context, ecosystem.Workspace, ecosystem.Toolchain) (ecosystem.BuildBaseline, error) {
	return ecosystem.BuildBaseline{}, nil
}

// TestSelector builds a runner selection from bound tests and the definitions of
// their files, so a bound test is mapped through its source location to the
// enclosing test definition.
type TestSelector interface {
	Select(bound []ecosystem.BoundTest, definitions map[string][]ecosystem.Definition) (ecosystem.Selection, error)
}

// SuiteRunner runs a selection and returns per-test outcomes.
type SuiteRunner interface {
	Run(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, selection ecosystem.Selection) (ecosystem.RunResult, error)
}
