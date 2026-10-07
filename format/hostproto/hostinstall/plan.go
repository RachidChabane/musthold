// Package hostinstall plans the install and the removal of a proof module's files
// in a project: its configuration seed, its entry in Shallnot's `proofs.require`,
// its block of AGENTS.md and the agent, skill and hook files of each harness.
// Every edit of a file the project or another tool also writes merges and never
// rewrites: the lines the module does not own stay byte for byte as they were.
//
// The package is pure. The project tree is the Files port: a module's adapter
// reads and writes the disk, and a test reads and writes a map.
package hostinstall

import (
	"bytes"
	"fmt"
)

// Files is the project tree the installer reads and writes. Paths are relative to
// the project root and use forward slashes.
type Files interface {
	// Read returns the content of a file, and false when it does not exist.
	Read(path string) (content []byte, found bool, err error)
	// Exists reports whether a file or a directory exists.
	Exists(path string) bool
	// Write creates or replaces a file, creating its directories.
	Write(path string, content []byte) error
	// Remove deletes a file; deleting an absent file is not an error.
	Remove(path string) error
}

// Change says what the installer does to one file.
type Change uint8

// Changes.
const (
	ChangeNone Change = iota
	ChangeCreate
	ChangeUpdate
	ChangeDelete
)

var changeNames = []string{"unchanged", "create", "update", "delete"}

func (c Change) String() string { return changeNames[c] }

// Action is one file in the state the installer wants.
type Action struct {
	Path    string
	Change  Change
	Content []byte
}

// Plan is everything an install, an enable or a disable wants for a project.
type Plan struct {
	Actions   []Action
	NextSteps []string
}

// Pending reports whether applying the plan would change a file.
func (p Plan) Pending() bool {
	for _, action := range p.Actions {
		if action.Change != ChangeNone {
			return true
		}
	}
	return false
}

// Apply writes the files that differ from the plan.
func (p Plan) Apply(files Files) error {
	for _, action := range p.Actions {
		var err error
		switch action.Change {
		case ChangeNone:
		case ChangeDelete:
			err = files.Remove(action.Path)
		default:
			err = files.Write(action.Path, action.Content)
		}
		if err != nil {
			return fmt.Errorf("%s %s: %w", action.Change, action.Path, err)
		}
	}
	return nil
}

// provisioner is the strategy for one file: from its current content, nil when it
// is absent, it returns the content the installer wants, empty to have the file
// removed.
type provisioner struct {
	path    string
	desired func(current []byte) ([]byte, error)
}

func (p provisioner) action(files Files) (Action, error) {
	current, found, err := files.Read(p.path)
	if err != nil {
		return Action{}, err
	}
	if !found {
		current = nil
	}
	desired, err := p.desired(current)
	if err != nil {
		return Action{}, fmt.Errorf("%s: %w", p.path, err)
	}
	action := Action{Path: p.path, Content: desired}
	switch {
	case !found && len(desired) == 0:
	case len(desired) == 0:
		action.Change = ChangeDelete
	case !found:
		action.Change = ChangeCreate
	case !bytes.Equal(current, desired):
		action.Change = ChangeUpdate
	}
	return action, nil
}

func buildPlan(files Files, provisioners []provisioner, steps []string) (Plan, error) {
	plan := Plan{NextSteps: steps}
	for _, p := range provisioners {
		action, err := p.action(files)
		if err != nil {
			return Plan{}, err
		}
		plan.Actions = append(plan.Actions, action)
	}
	return plan, nil
}

// fixed is the content of a file the installer owns entirely.
func fixed(content []byte) func([]byte) ([]byte, error) {
	return func([]byte) ([]byte, error) { return content, nil }
}
