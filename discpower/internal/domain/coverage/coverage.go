// Package coverage holds what a reference run learned about which units each test
// executes: the basis of the footprint entries and of the selection of the tests
// a mutant is run against.
package coverage

import (
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/footprint"
)

// nestedSeparator joins a nested definition to its enclosing qualname.
const nestedSeparator = "."

// Map records, per test, the units (functions and methods) it executed.
type Map map[ecosystem.TestID][]footprint.Ref

// Known reports whether the map holds the test.
func (m Map) Known(test ecosystem.TestID) bool {
	_, known := m[test]
	return known
}

// EntriesOf lists the units executed by any of the tests, sorted and without
// duplicates: the entry functions of a footprint.
func (m Map) EntriesOf(tests []ecosystem.TestID) []footprint.Ref {
	var entries []footprint.Ref
	for _, test := range tests {
		for _, ref := range m[test] {
			if !slices.Contains(entries, ref) {
				entries = append(entries, ref)
			}
		}
	}
	slices.SortFunc(entries, compareRefs)
	return entries
}

// Covering lists, in the order of the candidates, the tests that executed one of
// the units or a definition nested in one of them.
func (m Map) Covering(candidates []ecosystem.TestID, units []footprint.Ref) []ecosystem.TestID {
	var covering []ecosystem.TestID
	for _, test := range candidates {
		if slices.ContainsFunc(m[test], func(executed footprint.Ref) bool { return coversAny(executed, units) }) {
			covering = append(covering, test)
		}
	}
	return covering
}

func coversAny(executed footprint.Ref, units []footprint.Ref) bool {
	return slices.ContainsFunc(units, func(unit footprint.Ref) bool {
		return executed.File == unit.File &&
			(executed.Qualname == unit.Qualname || strings.HasPrefix(executed.Qualname, unit.Qualname+nestedSeparator))
	})
}

func compareRefs(a, b footprint.Ref) int {
	if byFile := strings.Compare(a.File, b.File); byFile != 0 {
		return byFile
	}
	return strings.Compare(a.Qualname, b.Qualname)
}
