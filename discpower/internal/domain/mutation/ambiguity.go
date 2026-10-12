package mutation

import "github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"

// UnitKey names a unit the way an edit targets it: by its kind class and its
// qualified name. Several units of one file may share a key when the source
// defines a name more than once.
type UnitKey struct {
	Assignment bool
	Qualname   string
}

// AmbiguousUnits is the set of unit keys that more than one unit of a file
// shares. It holds exactly the targets a splice refuses as ambiguous.
type AmbiguousUnits map[UnitKey]bool

// Ambiguous reads the ambiguous unit keys of a source.
func (s Splicer) Ambiguous(file string, source []byte) (AmbiguousUnits, error) {
	units, err := s.units(file, source)
	if err != nil {
		return nil, err
	}
	return ambiguousIn(units), nil
}

func ambiguousIn(units []ecosystem.Definition) AmbiguousUnits {
	counts := map[UnitKey]int{}
	for _, u := range units {
		counts[keyOf(u)]++
	}
	ambiguous := AmbiguousUnits{}
	for key, count := range counts {
		if count > 1 {
			ambiguous[key] = true
		}
	}
	return ambiguous
}

// Targets returns the key of the unit the edit replaces and whether it names
// one: a module addition declares a new name and replaces nothing.
func (e Edit) Targets() (UnitKey, bool) {
	if e.Kind() == TargetModuleAddition {
		return UnitKey{}, false
	}
	return UnitKey{Assignment: e.Kind() == TargetModuleAssignment, Qualname: e.Qualname}, true
}

// Covers reports whether the edit targets an ambiguous unit, which is the
// condition under which a splice of the edit returns ErrAmbiguousDefinition.
func (a AmbiguousUnits) Covers(edit Edit) bool {
	key, ok := edit.Targets()
	return ok && a[key]
}

func keyOf(u ecosystem.Definition) UnitKey {
	return UnitKey{Assignment: u.Kind == ecosystem.KindAssignment, Qualname: u.Qualname}
}
