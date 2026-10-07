package python

import (
	"slices"

	"github.com/odvcencio/gotreesitter"
)

// pieceKind tells whether a piece of the unit is declared at module level or in
// a class body.
type pieceKind int

const (
	modulePiece pieceKind = iota
	methodPiece
)

// piece is one definition or statement of the unit.
type piece struct {
	kind      pieceKind
	name      string
	order     int
	statement *gotreesitter.Node
	fragment  fragment
}

// target is the bound test as the file declares it.
type target struct {
	piece piece
	// class is the class the test is a method of, nil for a module-level test.
	class *classScope
}

// plan is the unit: the module-level helpers and the sibling methods the test
// reaches, each in source order, then the test.
type plan struct {
	helpers  []piece
	siblings []piece
	test     piece
}

// pieces lists the pieces of the unit in emission order.
func (p plan) pieces() []piece {
	all := make([]piece, 0, len(p.helpers)+len(p.siblings)+1)
	all = append(all, p.helpers...)
	all = append(all, p.siblings...)
	return append(all, p.test)
}

// planner follows the references of the test to the definitions they name.
type planner struct {
	index    *fileIndex
	class    *classScope
	test     piece
	helpers  map[string]piece
	siblings map[string]piece
	pending  []fragment
}

// planUnit selects the definitions the test reaches: module-level names and,
// for a method, the methods of its class, transitively. The class itself is
// never a helper, because the unit does not keep the class wrapper.
func (x *fileIndex) planUnit(t target) plan {
	p := &planner{
		index:    x,
		class:    t.class,
		test:     t.piece,
		helpers:  map[string]piece{},
		siblings: map[string]piece{},
		pending:  []fragment{t.piece.fragment},
	}
	for len(p.pending) > 0 {
		current := p.pending[len(p.pending)-1]
		p.pending = p.pending[:len(p.pending)-1]
		p.follow(current)
	}
	return plan{helpers: sortedPieces(p.helpers), siblings: sortedPieces(p.siblings), test: t.piece}
}

func (p *planner) follow(f fragment) {
	for _, use := range f.occurrences {
		switch use.role {
		case rolePlain:
			p.followName(use.name)
		case roleReceiverAttribute:
			p.followMethod(use.name)
		case roleDefinitionName:
		}
	}
}

func (p *planner) followName(name string) {
	if p.isExcluded(name) {
		return
	}
	if found, ok := p.index.declarations[name]; ok {
		p.selectHelper(found)
		return
	}
	p.followMethod(name)
}

// isExcluded reports whether a module-level name is the test or the class of the
// test, which the unit never lists among its helpers.
func (p *planner) isExcluded(name string) bool {
	if p.class != nil {
		return name == p.class.name
	}
	return name == p.test.name
}

func (p *planner) selectHelper(found declaration) {
	if _, seen := p.helpers[found.name]; seen {
		return
	}
	chosen := piece{kind: modulePiece, name: found.name, order: found.order, statement: found.statement, fragment: p.index.scan(found.statement)}
	p.helpers[found.name] = chosen
	p.pending = append(p.pending, chosen.fragment)
}

func (p *planner) followMethod(name string) {
	if p.class == nil || name == p.test.name {
		return
	}
	found, ok := p.class.method(name)
	if !ok {
		return
	}
	if _, seen := p.siblings[name]; seen {
		return
	}
	chosen := piece{kind: methodPiece, name: name, order: found.order, statement: found.statement, fragment: p.index.scan(found.statement)}
	p.siblings[name] = chosen
	p.pending = append(p.pending, chosen.fragment)
}

func sortedPieces(set map[string]piece) []piece {
	sorted := make([]piece, 0, len(set))
	for _, chosen := range set {
		sorted = append(sorted, chosen)
	}
	slices.SortFunc(sorted, func(a, b piece) int { return a.order - b.order })
	return sorted
}
