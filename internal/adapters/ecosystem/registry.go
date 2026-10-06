// Package ecosystem holds the Factory that chooses an ecosystem Strategy by
// detection. Each strategy lives in a sub-package; the registry knows none of
// them, so a new strategy needs no change here.
package ecosystem

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"

	"github.com/RachidChabane/spec-proof/internal/app/ports"
	domain "github.com/RachidChabane/spec-proof/internal/domain/ecosystem"
)

// Errors of the registry.
var (
	// ErrDuplicateStrategy is returned when two strategies share a name.
	ErrDuplicateStrategy = errors.New("duplicate ecosystem strategy")
	// ErrUnknownEcosystem is returned when no registered strategy has the name.
	ErrUnknownEcosystem = errors.New("unknown ecosystem")
	// ErrInvalidStrategy is returned for a strategy whose name is not supported.
	ErrInvalidStrategy = errors.New("invalid ecosystem strategy")
)

// Detected pairs a strategy with what it recognized in a project tree.
type Detected struct {
	Strategy  ports.Ecosystem
	Detection domain.Detection
}

// Registry is the Factory of ecosystem strategies.
type Registry struct {
	strategies []ports.Ecosystem
}

// New registers the strategies, refusing a duplicate or unsupported name.
func New(strategies ...ports.Ecosystem) (*Registry, error) {
	seen := map[domain.Name]struct{}{}
	for _, strategy := range strategies {
		name := strategy.Name()
		if !name.Valid() {
			return nil, fmt.Errorf("%w: %q", ErrInvalidStrategy, name)
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("%w: %q", ErrDuplicateStrategy, name)
		}
		seen[name] = struct{}{}
	}
	return &Registry{strategies: slices.Clone(strategies)}, nil
}

// Names lists the registered ecosystems in registration order.
func (r *Registry) Names() []domain.Name {
	names := make([]domain.Name, 0, len(r.strategies))
	for _, strategy := range r.strategies {
		names = append(names, strategy.Name())
	}
	return names
}

// For returns the strategy registered under name.
func (r *Registry) For(name domain.Name) (ports.Ecosystem, error) {
	for _, strategy := range r.strategies {
		if strategy.Name() == name {
			return strategy, nil
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrUnknownEcosystem, name)
}

// Detect asks every strategy to recognize the tree and returns those that do,
// in registration order.
func (r *Registry) Detect(root fs.FS) []Detected {
	var detected []Detected
	for _, strategy := range r.strategies {
		if detection, ok := strategy.Detect(root); ok {
			detected = append(detected, Detected{Strategy: strategy, Detection: detection})
		}
	}
	return detected
}
