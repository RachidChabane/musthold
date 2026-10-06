// Package golang is the Strategy of the Go ecosystem: toolchain resolution,
// build check, subtest selection and the gotestsum-based runner.
package golang

import (
	"context"
	"errors"
	"io/fs"
	"os/exec"
	"time"

	"github.com/RachidChabane/spec-proof/internal/app/ports"
	"github.com/RachidChabane/spec-proof/internal/domain/ecosystem"
)

// DefaultTimeout bounds one build check or suite run.
const DefaultTimeout = 5 * time.Minute

// ErrNoToolchain is returned when the go command cannot be found.
var ErrNoToolchain = errors.New("go command not found")

const (
	moduleFile    = "go.mod"
	rootDirectory = "."
	goCommand     = "go"
	vendorDir     = "vendor"
	testFileGlob  = "*_test.go"
	goSuffix      = ".go"
)

// Dependencies are the ports a Go Strategy works through.
type Dependencies struct {
	Process ports.ProcessRunner
	Reader  ports.ResultsReader
	Locator ports.DefinitionLocator
	// LookPath finds an executable on PATH; it defaults to exec.LookPath.
	LookPath func(string) (string, error)
	// Timeout bounds one process; it defaults to DefaultTimeout.
	Timeout time.Duration
}

var _ ports.Ecosystem = (*Strategy)(nil)

// Strategy implements ports.Ecosystem for Go testing.
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

// Name is ecosystem.GoTesting.
func (*Strategy) Name() ecosystem.Name { return ecosystem.GoTesting }

// Detect recognizes a Go module by its go.mod.
func (*Strategy) Detect(root fs.FS) (ecosystem.Detection, bool) {
	if _, err := fs.Stat(root, moduleFile); err != nil {
		return ecosystem.Detection{}, false
	}
	return ecosystem.Detection{Name: ecosystem.GoTesting, Root: rootDirectory, Evidence: []string{moduleFile}}, true
}

// Layout treats vendor as dependencies and recognizes `_test.go` files.
func (*Strategy) Layout() ecosystem.Layout {
	return ecosystem.Layout{DependencyDirs: []string{vendorDir}, TestFileGlobs: []string{testFileGlob}}
}

// Toolchain resolves the go command.
func (s *Strategy) Toolchain() ports.ToolchainResolver { return toolchainResolver{deps: s.deps} }

// Locator is the injected go/parser locator.
func (s *Strategy) Locator() ports.DefinitionLocator { return s.deps.Locator }

// BuildCheck compiles the test binaries of the changed packages.
func (s *Strategy) BuildCheck() ports.BuildCheck { return buildCheck{deps: s.deps} }

// Selector builds an anchored -run expression from the bound subtest paths.
func (*Strategy) Selector() ports.TestSelector { return selector{} }

// Runner runs the selection through gotestsum.
func (s *Strategy) Runner() ports.SuiteRunner { return runner{deps: s.deps} }

type toolchainResolver struct {
	deps Dependencies
}

// Resolve finds the go command on PATH.
func (r toolchainResolver) Resolve(context.Context, ecosystem.Workspace) (ecosystem.Toolchain, error) {
	path, err := r.deps.LookPath(goCommand)
	if err != nil {
		return ecosystem.Toolchain{}, ErrNoToolchain
	}
	return ecosystem.Toolchain{Command: []string{path}}, nil
}
