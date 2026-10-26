package mutation

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/format"
)

// TargetKind says what an edit's qualname designates.
type TargetKind string

// Target kinds.
//
//   - TargetDefinition: a function or method definition located by its
//     qualname; new_source replaces the whole definition.
//   - TargetModuleAssignment: a module-level assignment (Python) or a
//     package-level const or var spec (Go) located by its assigned name;
//     new_source replaces the whole statement or spec.
//   - TargetModuleAddition: a new module-level statement; the qualname is the
//     name it declares and new_source is appended after the last statement.
const (
	TargetDefinition       TargetKind = "definition"
	TargetModuleAssignment TargetKind = "module_assignment"
	TargetModuleAddition   TargetKind = "module_addition"
)

var allTargets = []TargetKind{TargetDefinition, TargetModuleAssignment, TargetModuleAddition}

// Valid reports whether k is a known target kind.
func (k TargetKind) Valid() bool { return slices.Contains(allTargets, k) }

// Edit replaces one unit of one file. An empty Target stands for a definition,
// the kind of every edit that predates target kinds.
type Edit struct {
	File      string
	Qualname  string
	Target    TargetKind
	NewSource string
}

// Kind is the target kind of the edit: the single place where an empty Target
// reads as a definition.
func (e Edit) Kind() TargetKind {
	if e.Target == "" {
		return TargetDefinition
	}
	return e.Target
}

// editWire is the JSON form of an Edit: the target is omitted for a definition,
// so edits of producers that know only definitions stay valid.
type editWire struct {
	File      string     `json:"file"`
	Qualname  string     `json:"qualname"`
	Target    TargetKind `json:"target,omitempty"`
	NewSource string     `json:"new_source"`
}

// MarshalJSON writes the wire form.
func (e Edit) MarshalJSON() ([]byte, error) {
	wire := editWire{File: e.File, Qualname: e.Qualname, Target: e.Kind(), NewSource: e.NewSource}
	if wire.Target == TargetDefinition {
		wire.Target = ""
	}
	return json.Marshal(wire)
}

// UnmarshalJSON reads the wire form; an absent target is a definition.
func (e *Edit) UnmarshalJSON(data []byte) error {
	var wire editWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*e = Edit{File: wire.File, Qualname: wire.Qualname, Target: wire.Target, NewSource: wire.NewSource}
	e.Target = e.Kind()
	return nil
}

// EditSet is the mutant: a set of edits applied together.
type EditSet struct {
	Edits []Edit `json:"edits"`
}

// Files lists the distinct files the set edits, in first-seen order.
func (s EditSet) Files() []string {
	var files []string
	for _, edit := range s.Edits {
		if !slices.Contains(files, edit.File) {
			files = append(files, edit.File)
		}
	}
	return files
}

// EditsOf returns the edits of one file, in order.
func (s EditSet) EditsOf(file string) []Edit {
	var edits []Edit
	for _, edit := range s.Edits {
		if edit.File == file {
			edits = append(edits, edit)
		}
	}
	return edits
}

// ErrInvalidEditSet is returned for an edit set that cannot name a mutant.
var ErrInvalidEditSet = errors.New("invalid edit set")

// Validate rejects an empty set, an unknown target kind, an empty file or
// qualname, and two edits of the same unit.
func (s EditSet) Validate() error {
	if len(s.Edits) == 0 {
		return fmt.Errorf("%w: no edit", ErrInvalidEditSet)
	}
	seen := map[string]bool{}
	for _, edit := range s.Edits {
		if !edit.Kind().Valid() || edit.File == "" || edit.Qualname == "" {
			return fmt.Errorf("%w: %+v", ErrInvalidEditSet, edit)
		}
		key := unitKey(edit)
		if seen[key] {
			return fmt.Errorf("%w: unit edited twice: %s", ErrInvalidEditSet, key)
		}
		seen[key] = true
	}
	return nil
}

func unitKey(e Edit) string {
	return strings.Join([]string{e.File, string(e.Kind()), e.Qualname}, "\x00")
}

// Canonical returns the edits with their target spelled out, ordered by file,
// target and qualname.
func (s EditSet) Canonical() EditSet {
	edits := slices.Clone(s.Edits)
	for i := range edits {
		edits[i].Target = edits[i].Kind()
	}
	slices.SortFunc(edits, func(a, b Edit) int { return strings.Compare(unitKey(a), unitKey(b)) })
	return EditSet{Edits: edits}
}

// ID is the mutant id: the SHA-256 of the RFC 8785 canonical form of the
// canonically ordered edit set.
func (s EditSet) ID() (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	canonical, err := format.Canonicalize(s.Canonical())
	if err != nil {
		return "", err
	}
	return format.SHA256Hex(canonical), nil
}

// ScanEdits writes the set in the wire form of a scan statement: in canonical
// order, the target of a definition omitted. Their digest is the mutant id.
func (s EditSet) ScanEdits() []format.ScanEdit {
	canonical := s.Canonical()
	edits := make([]format.ScanEdit, len(canonical.Edits))
	for index, edit := range canonical.Edits {
		target := string(edit.Kind())
		if edit.Kind() == TargetDefinition {
			target = ""
		}
		edits[index] = format.ScanEdit{File: edit.File, Qualname: edit.Qualname, Target: target, NewSource: edit.NewSource}
	}
	return edits
}

// EditSetOf reads the wire form of a scan mutant's edits back as an edit set:
// an omitted target is a definition.
func EditSetOf(edits []format.ScanEdit) EditSet {
	set := EditSet{Edits: make([]Edit, len(edits))}
	for index, edit := range edits {
		set.Edits[index] = Edit{File: edit.File, Qualname: edit.Qualname, Target: TargetKind(edit.Target), NewSource: edit.NewSource}
		set.Edits[index].Target = set.Edits[index].Kind()
	}
	return set
}
