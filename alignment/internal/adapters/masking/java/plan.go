package java

import (
	"slices"
	"strconv"

	"github.com/odvcencio/gotreesitter"
)

// Neutral names of the unit.
const (
	maskedClassName  = "Case"
	maskedTestName   = "testCase"
	helperPrefix     = "helper"
	helperTypePrefix = "Helper"
)

// piece is one member of the unit: a declaration the test reaches, or a member of an
// enclosing class that runs for every test of it.
type piece struct {
	node     *gotreesitter.Node
	names    []string
	fragment fragment
}

// plan is the unit: the types of the file the test reaches, the annotations of the
// classes around the test, the members of those classes that the test reaches and
// the test.
type plan struct {
	test testCase
	// types are the other types of the file the test reaches, in source order.
	types []piece
	// annotations are the annotations of the classes around the test that stay.
	annotations []piece
	// blocks holds the members per class of the chain, outermost first, each in
	// source order.
	blocks       [][]piece
	testFragment fragment
	// classes holds the names of the classes around the test, which the unit
	// calls one class.
	classes map[string]bool
	// renames maps the name of every declaration of the unit to its neutral name.
	renames map[string]string
}

type planner struct {
	classes  map[string]bool
	declared map[string][]*piece
	chosen   map[*piece]bool
	pending  []fragment
}

// planUnit selects what the test reaches: the members of its enclosing classes that
// run for every test, and the declarations the test, those members and the
// annotations of the classes name, transitively.
func (x *fileIndex) planUnit(test testCase) plan {
	classes := map[string]bool{}
	for _, scope := range test.chain {
		classes[scope.name] = true
	}
	planning := &planner{classes: classes, declared: map[string][]*piece{}, chosen: map[*piece]bool{}}
	result := plan{test: test, classes: classes, renames: map[string]string{}, testFragment: x.scan(test.method)}
	planning.pending = append(planning.pending, result.testFragment)

	all := make([][]*piece, len(test.chain))
	for depth, scope := range test.chain {
		for _, annotation := range x.annotationsOf(scope.node) {
			if x.keepsAnnotation(annotation) {
				kept := piece{node: annotation, fragment: x.scan(annotation)}
				result.annotations = append(result.annotations, kept)
				planning.pending = append(planning.pending, kept.fragment)
			}
		}
		nested := x.nestedChain(test.chain, depth)
		for _, m := range scope.members {
			if m.node == test.method || (m.kind == memberTest) || nested[m.node] {
				continue
			}
			if m.kind == memberDeclaration || m.kind == memberSetup {
				candidate := &piece{node: m.node, names: m.names, fragment: x.scan(m.node)}
				all[depth] = append(all[depth], candidate)
				planning.register(candidate)
				if m.kind == memberSetup {
					planning.chosen[candidate] = true
					planning.pending = append(planning.pending, candidate.fragment)
				}
			}
		}
	}
	var others []*piece
	for _, top := range x.topLevelTypes() {
		if top == test.chain[0].node {
			continue
		}
		candidate := &piece{node: top, names: x.declaredNames(top), fragment: x.scan(top)}
		others = append(others, candidate)
		planning.register(candidate)
	}
	planning.run()

	result.types = chosenIn(others, planning.chosen)
	for _, pieces := range all {
		result.blocks = append(result.blocks, chosenIn(pieces, planning.chosen))
	}
	result.nameHelpers(x, test)
	return result
}

// nestedChain lists the members of the class at a depth that are the next class of
// the chain, which the unit flattens into the class rather than lists as a member.
func (x *fileIndex) nestedChain(chain []*classScope, depth int) map[*gotreesitter.Node]bool {
	nested := map[*gotreesitter.Node]bool{}
	if depth+1 < len(chain) {
		nested[chain[depth+1].node] = true
	}
	return nested
}

// keepsAnnotation reports whether an annotation of a class stays in the unit.
func (x *fileIndex) keepsAnnotation(annotation *gotreesitter.Node) bool {
	return !slices.Contains(hiddenAnnotations, x.annotationName(annotation))
}

func (p *planner) register(candidate *piece) {
	for _, name := range candidate.names {
		p.declared[name] = append(p.declared[name], candidate)
	}
}

func (p *planner) run() {
	for len(p.pending) > 0 {
		current := p.pending[len(p.pending)-1]
		p.pending = p.pending[:len(p.pending)-1]
		for _, name := range current.reached(p.classes) {
			for _, candidate := range p.declared[name] {
				if !p.chosen[candidate] {
					p.chosen[candidate] = true
					p.pending = append(p.pending, candidate.fragment)
				}
			}
		}
	}
}

func chosenIn(pieces []*piece, chosen map[*piece]bool) []piece {
	var kept []piece
	for _, candidate := range pieces {
		if chosen[candidate] {
			kept = append(kept, *candidate)
		}
	}
	slices.SortFunc(kept, func(a, b piece) int { return int(a.node.StartByte()) - int(b.node.StartByte()) })
	return kept
}

// nameHelpers gives the declarations of the unit neutral names in the order the
// unit lists them, the classes around the test one name and the test its own.
func (p *plan) nameHelpers(x *fileIndex, test testCase) {
	next := 1
	name := func(original string, prefix string) {
		if _, named := p.renames[original]; !named {
			p.renames[original] = prefix + strconv.Itoa(next)
			next++
		}
	}
	for _, chosen := range p.types {
		for _, original := range chosen.names {
			name(original, helperTypePrefix)
		}
	}
	for _, pieces := range p.blocks {
		for _, chosen := range pieces {
			for _, original := range chosen.names {
				name(original, x.prefixFor(chosen.node))
			}
		}
	}
	for class := range p.classes {
		p.renames[class] = maskedClassName
	}
	p.renames[x.Text(x.Field(test.method, fieldName))] = maskedTestName
}

// prefixFor is the neutral prefix of a declaration: types are capitalized.
func (x *fileIndex) prefixFor(node *gotreesitter.Node) string {
	switch x.Type(node) {
	case typeClass, typeInterface, typeEnum, typeRecord, typeAnnotationType:
		return helperTypePrefix
	}
	return helperPrefix
}

// fragments lists every fragment of the unit.
func (p plan) fragments() []fragment {
	all := []fragment{p.testFragment}
	for _, group := range [][]piece{p.types, p.annotations} {
		for _, chosen := range group {
			all = append(all, chosen.fragment)
		}
	}
	for _, pieces := range p.blocks {
		for _, chosen := range pieces {
			all = append(all, chosen.fragment)
		}
	}
	return all
}

// usedNames lists the names the unit uses.
func (p plan) usedNames() map[string]bool {
	used := map[string]bool{}
	for _, f := range p.fragments() {
		for _, name := range f.names() {
			used[name] = true
		}
	}
	return used
}
