// Package hunk finds what a change did to the units of a file: one hunk per
// function, method or module-level assignment that is new or whose normal form
// differs from the unit of the same name before the change. A change that leaves
// the normal form alone, such as a comment, a docstring or formatting, is no hunk.
package hunk

import (
	"cmp"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/completeness/internal/domain/code"
	"github.com/RachidChabane/spec-proof/format"
)

// Hunk is one changed unit.
type Hunk struct {
	// Unit is the unit as it is after the change.
	Unit code.Unit
	// Before is the unit's text before the change; empty for a new unit.
	Before string
	// New is true when no unit of that name existed before the change.
	New bool
}

// ID identifies the hunk by the file, the unit and the code after the change, so
// that the same change always has the same identity.
func (h Hunk) ID() string {
	return format.SHA256Hex([]byte(strings.Join([]string{h.Unit.File, h.Unit.Qualname, h.Unit.Normalized}, "\x00")))
}

// Between lists the hunks of a file: its units after the change that are new or
// differ in normal form from the unit of the same qualname before it, sorted by
// start line. A deleted unit is no hunk, because it adds no behaviour.
func Between(before, after []code.Unit) []Hunk {
	previous := make(map[string]code.Unit, len(before))
	for _, unit := range before {
		previous[unit.Qualname] = unit
	}
	var found []Hunk
	for _, unit := range after {
		old, existed := previous[unit.Qualname]
		switch {
		case !existed:
			found = append(found, Hunk{Unit: unit, New: true})
		case old.Normalized != unit.Normalized:
			found = append(found, Hunk{Unit: unit, Before: old.Text})
		}
	}
	slices.SortFunc(found, func(a, b Hunk) int {
		return cmp.Or(cmp.Compare(a.Unit.StartLine, b.Unit.StartLine), strings.Compare(a.Unit.Qualname, b.Unit.Qualname))
	})
	return found
}
