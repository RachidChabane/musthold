// Package operatorgen generates the deterministic mutants of a criterion: the
// operator mutants sampled to K_op and the sentinels, over the footprint of its
// entry functions. It never calls a model.
package operatorgen

import (
	"errors"
	"fmt"
	"path"
	"slices"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/footprint"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/operator"
)

// OperatorSource enumerates the operator mutants of a file in source order.
type OperatorSource interface {
	Candidates(file mutation.SourceFile) ([]operator.Candidate, error)
}

// SentinelSource lists the sentinels the catalogue allows for a file. The seed
// draws the identifiers the sentinels inject and the positions they use.
type SentinelSource interface {
	SentinelCandidates(file mutation.SourceFile, seed operator.Seed) ([]operator.SentinelCandidate, error)
}

// CallGraphSource reads the static call graph contribution of a file.
type CallGraphSource interface {
	CallGraph(file mutation.SourceFile) (footprint.FileGraph, error)
}

// Engine is the Strategy of one source language: the language-specific half of
// mutant generation. A new language implements it and registers itself by file
// extension; nothing shared changes.
type Engine interface {
	// Extensions lists the file extensions the engine handles, dot included.
	Extensions() []string
	// Families lists the operator families the engine can produce.
	Families() []operator.Family
	OperatorSource
	SentinelSource
	CallGraphSource
	operator.Normalizer
	// AssignmentLocator finds the module-level units the mutation splicer needs
	// to apply an edit of a module assignment.
	mutation.AssignmentLocator
}

// Errors of the registry.
var (
	ErrNoEngine           = errors.New("no mutation engine for the file")
	ErrExtensionTaken     = errors.New("extension already registered")
	ErrEngineWithoutTypes = errors.New("engine handles no extension")
	ErrNoLocator          = errors.New("engine registered without a definition locator")
)

// registration is an engine with the splicer that applies its edits.
type registration struct {
	engine  Engine
	splicer mutation.Splicer
}

// Registry chooses the engine of a file by its extension.
type Registry struct {
	registrations map[string]registration
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{registrations: map[string]registration{}} }

// Register adds an engine under each of its extensions. The definition locator
// of the language lets the registry give the splicer that applies the engine's
// edits, with the engine itself as the assignment locator.
func (r *Registry) Register(engine Engine, definitions mutation.DefinitionLocator) error {
	extensions := engine.Extensions()
	if len(extensions) == 0 {
		return ErrEngineWithoutTypes
	}
	if definitions == nil {
		return ErrNoLocator
	}
	for _, extension := range extensions {
		if _, taken := r.registrations[extension]; taken {
			return fmt.Errorf("%w: %s", ErrExtensionTaken, extension)
		}
	}
	for _, extension := range extensions {
		r.registrations[extension] = registration{
			engine:  engine,
			splicer: mutation.Splicer{Locator: definitions, Assignments: engine},
		}
	}
	return nil
}

// For returns the engine of a file path.
func (r *Registry) For(filePath string) (Engine, error) {
	entry, err := r.entry(filePath)
	return entry.engine, err
}

// SplicerFor returns the splicer that applies the edits of a file.
func (r *Registry) SplicerFor(filePath string) (mutation.Splicer, error) {
	entry, err := r.entry(filePath)
	return entry.splicer, err
}

func (r *Registry) entry(filePath string) (registration, error) {
	entry, ok := r.registrations[path.Ext(filePath)]
	if !ok {
		return registration{}, fmt.Errorf("%w: %q", ErrNoEngine, filePath)
	}
	return entry, nil
}

// Extensions lists the registered extensions, sorted.
func (r *Registry) Extensions() []string {
	extensions := make([]string, 0, len(r.registrations))
	for extension := range r.registrations {
		extensions = append(extensions, extension)
	}
	slices.Sort(extensions)
	return extensions
}
