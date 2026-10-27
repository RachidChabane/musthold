package adapters

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/RachidChabane/musthold/tools/release/internal/domain"
)

const (
	goBinary = "go"
	// testTimeout is the per-package limit of discpower's own test command (its
	// shallnot.yaml), the module with the longest packages: its end-to-end
	// packages run for more than an hour on a hosted runner.
	testTimeout     = "240m"
	testParallelism = "1"
)

// GoToolchain builds and tests the modules of a repository with the go command.
type GoToolchain struct{ root string }

// NewGoToolchain returns a toolchain for the repository rooted at root.
func NewGoToolchain(root string) GoToolchain { return GoToolchain{root: root} }

func (g GoToolchain) moduleRunner(module string) commandRunner {
	return commandRunner{dir: filepath.Join(g.root, module)}
}

// Build reports how `go build ./...` ends in the module outside the workspace.
func (g GoToolchain) Build(module string) domain.Outcome {
	output, err := g.moduleRunner(module).run(domain.CheckEnvironment(), goBinary, "build", "-o", os.DevNull, "./...")
	return outcomeOf(output, err)
}

// Test reports how `go test ./...` ends in the module outside the workspace. It
// runs one package at a time.
func (g GoToolchain) Test(module string) domain.Outcome {
	output, err := g.moduleRunner(module).run(domain.CheckEnvironment(), goBinary,
		"test", "-count=1", "-p", testParallelism, "-timeout", testTimeout, "./...")
	return outcomeOf(output, err)
}

// Compile cross-builds one binary as the release does.
func (g GoToolchain) Compile(spec domain.BuildSpec) error {
	output, err := g.moduleRunner(spec.Tag.Module.Name).run(spec.Environment(), goBinary, spec.Arguments()...)
	if err != nil {
		return fmt.Errorf("go build for %s: %s", spec.Target, failureText(output, err))
	}
	return nil
}

func outcomeOf(output string, err error) domain.Outcome {
	if err != nil {
		return domain.Outcome{Ran: true, Failure: failureText(output, err)}
	}
	return domain.Outcome{Ran: true}
}
