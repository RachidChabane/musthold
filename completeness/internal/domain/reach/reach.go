// Package reach closes a set of entry names over a name-based static graph of the
// units of a project. The graph is conservative: it never misses a reference the
// source states and may include a unit of the same name that the reference does
// not reach, so a unit it calls reached is one that executing the entries may
// run.
package reach

import (
	"slices"

	"github.com/RachidChabane/spec-proof/completeness/internal/domain/code"
)

// constructorName is the method a class instantiation reaches.
const constructorName = "__init__"

// Graph indexes the units of a project by the names they answer to.
type Graph struct {
	units  []code.Unit
	byName map[string][]int
}

// NewGraph indexes the units. A unit answers to its own name; the constructor of a
// class also answers to the class name, which is how code instantiates it.
func NewGraph(units []code.Unit) Graph {
	graph := Graph{units: slices.Clone(units), byName: map[string][]int{}}
	for index, unit := range graph.units {
		graph.byName[unit.Name] = append(graph.byName[unit.Name], index)
		if unit.Kind == code.KindFunction && unit.Name == constructorName && unit.Owner != "" {
			graph.byName[unit.Owner] = append(graph.byName[unit.Owner], index)
		}
	}
	return graph
}

// Reached returns the keys of the units that executing the entry names may run:
// the units the names designate, and, transitively, the units those mention.
func (g Graph) Reached(entries []string) map[string]bool {
	reached := map[string]bool{}
	queue := slices.Clone(entries)
	seenNames := map[string]bool{}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if seenNames[name] {
			continue
		}
		seenNames[name] = true
		for _, index := range g.byName[name] {
			unit := g.units[index]
			if reached[unit.Key()] {
				continue
			}
			reached[unit.Key()] = true
			queue = append(queue, unit.Mentions...)
		}
	}
	return reached
}
