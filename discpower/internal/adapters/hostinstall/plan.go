// Package hostinstall installs and removes the module's files in a project: its
// section of the configuration, its entry in Shallnot's `proofs.require`, its
// block of AGENTS.md and the agent, skill and hook files of each harness. Every
// edit of a file the project or another tool also writes merges and never
// rewrites: the lines the module does not own stay byte for byte as they were.
package hostinstall

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Permissions of what the installer writes.
const (
	dirMode  os.FileMode = 0o755
	fileMode os.FileMode = 0o644
)

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
	Root      string
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
func (p Plan) Apply() error {
	for _, action := range p.Actions {
		path := filepath.Join(p.Root, filepath.FromSlash(action.Path))
		switch action.Change {
		case ChangeNone:
		case ChangeDelete:
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		default:
			if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
				return err
			}
			if err := os.WriteFile(path, action.Content, fileMode); err != nil {
				return err
			}
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

func (p provisioner) action(root string) (Action, error) {
	current, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p.path)))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Action{}, err
	}
	desired, err := p.desired(current)
	if err != nil {
		return Action{}, fmt.Errorf("%s: %w", p.path, err)
	}
	action := Action{Path: p.path, Content: desired}
	switch {
	case current == nil && len(desired) == 0:
	case len(desired) == 0:
		action.Change = ChangeDelete
	case current == nil:
		action.Change = ChangeCreate
	case !bytes.Equal(current, desired):
		action.Change = ChangeUpdate
	}
	return action, nil
}

func buildPlan(root string, provisioners []provisioner, steps []string) (Plan, error) {
	plan := Plan{Root: root, NextSteps: steps}
	for _, p := range provisioners {
		action, err := p.action(root)
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

// unchanged keeps whatever a file holds.
func unchanged(current []byte) ([]byte, error) { return current, nil }
