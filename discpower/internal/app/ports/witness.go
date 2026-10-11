package ports

import (
	"context"
	"io/fs"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/differential"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/witness"
)

// WitnessFile is one witness set laid out as a test file of the project.
type WitnessFile struct {
	// Path is the file's path relative to the project root, with forward
	// slashes. It is a test file of the ecosystem.
	Path    string
	Content []byte
	// Tests holds, by witness id, the test that runs the witness, named the way
	// the ecosystem's JUnit reporter names it.
	Tests map[string]ecosystem.BoundTest
}

// WitnessBrief is what a witness writer is told about the project, test-free:
// nothing in it reads a test body or depends on which tests exist, only on the
// signatures of the helpers the test files declare.
type WitnessBrief struct {
	// Language names the language and the test framework.
	Language string
	// Conventions tells how a witness of the project is written.
	Conventions string
	// Interface lists the public signatures of the code under test, never a body.
	Interface []string
	// Helpers lists the signatures of the helpers the project's test files
	// declare, never a body.
	Helpers []string
}

// WitnessHost lays out witnesses as tests of one ecosystem.
type WitnessHost interface {
	// Brief tells a writer how to write witnesses for the project: the
	// signatures of the sources the spec text mentions, and of the helpers of
	// every test file of the project.
	Brief(project fs.FS, sources []mutation.SourceFile, specText string) (WitnessBrief, error)
	// Layout writes a witness set as one test file of the project, each witness
	// renamed so that no two sets collide.
	Layout(project fs.FS, set witness.Set) (WitnessFile, error)
}

// Witnessing is implemented by an ecosystem strategy that can run witnesses.
type Witnessing interface {
	Witnesses() WitnessHost
}

// FormCompiler compiles the units of a workspace into their canonical compiled
// forms (E0).
type FormCompiler interface {
	// Forms returns the compiled form of each site `file::qualname` it can
	// compile; a site it cannot is left out.
	Forms(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, sites []string) (differential.CompiledForms, error)
}

// Compiling is implemented by an ecosystem strategy that can compare compiled
// forms.
type Compiling interface {
	Compiler() FormCompiler
}
