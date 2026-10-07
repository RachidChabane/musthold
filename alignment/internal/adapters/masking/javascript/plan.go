package javascript

import (
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// helperPrefix starts the neutral name of a declaration the test reaches.
const helperPrefix = "helper"

// piece is one statement of the unit: a declaration the test reaches, or a hook or
// a setup call of a block that encloses the test.
type piece struct {
	member   member
	fragment fragment
}

// plan is the unit: for every block from the module to the suite that holds the
// test, the pieces that block contributes, then the test.
type plan struct {
	test testCase
	// blocks holds the pieces per depth, depth zero being the module, each in
	// source order.
	blocks [][]piece
	// testFragment and headers are the fragments of the test statement and of the
	// head of each enclosing suite.
	testFragment fragment
	headers      []fragment
	// renames maps the name of every declaration of the unit to its neutral name.
	renames map[string]string
}

// planner follows the identifiers of the test to the declarations they name.
type planner struct {
	declared map[string][]*piece
	chosen   map[*piece]bool
	pending  []fragment
}

// planUnit selects what the test reaches: the hooks and setup calls of every
// enclosing block, and the declarations the test, those and the heads of the
// enclosing suites name, transitively.
func (x *fileIndex) planUnit(test testCase) plan {
	depths := len(test.suites) + 1
	blockMembers := make([][]member, depths)
	blockMembers[0] = x.membersOf(x.Root())
	for index, enclosing := range test.suites {
		blockMembers[index+1] = enclosing.members
	}

	all := make([][]*piece, depths)
	planning := &planner{declared: map[string][]*piece{}, chosen: map[*piece]bool{}}
	for depth, members := range blockMembers {
		for _, m := range members {
			candidate := &piece{member: m}
			switch m.kind {
			case memberDeclaration:
				candidate.fragment = x.scan(m.statement, nil)
				all[depth] = append(all[depth], candidate)
				for _, name := range m.names {
					planning.declared[name] = append(planning.declared[name], candidate)
				}
			case memberSetup:
				candidate.fragment = x.scan(m.statement, nil)
				all[depth] = append(all[depth], candidate)
				planning.chosen[candidate] = true
				planning.pending = append(planning.pending, candidate.fragment)
			}
		}
	}

	result := plan{test: test, testFragment: x.scan(test.test.statement, nil), renames: map[string]string{}}
	planning.pending = append(planning.pending, result.testFragment)
	for _, enclosing := range test.suites {
		head := x.scan(enclosing.call.statement, x.suiteBlock(enclosing.call))
		result.headers = append(result.headers, head)
		planning.pending = append(planning.pending, head)
	}
	planning.run()

	for _, pieces := range all {
		var kept []piece
		for _, candidate := range pieces {
			if planning.chosen[candidate] {
				kept = append(kept, *candidate)
			}
		}
		slices.SortFunc(kept, func(a, b piece) int { return int(a.member.statement.StartByte()) - int(b.member.statement.StartByte()) })
		result.blocks = append(result.blocks, kept)
	}
	result.nameHelpers()
	return result
}

func (p *planner) run() {
	for len(p.pending) > 0 {
		current := p.pending[len(p.pending)-1]
		p.pending = p.pending[:len(p.pending)-1]
		for _, use := range current.occurrences {
			for _, candidate := range p.declared[use.name] {
				if !p.chosen[candidate] {
					p.chosen[candidate] = true
					p.pending = append(p.pending, candidate.fragment)
				}
			}
		}
	}
}

// nameHelpers gives the declarations of the unit neutral names in the order the
// unit lists them.
func (p *plan) nameHelpers() {
	next := 1
	for _, pieces := range p.blocks {
		for _, chosen := range pieces {
			for _, name := range chosen.member.names {
				if _, named := p.renames[name]; !named {
					p.renames[name] = neutralName(name, next)
					next++
				}
			}
		}
	}
}

// neutralName is the helper name for the declaration at the position. A name that
// starts with a capital and is not a constant spelled in capitals keeps one,
// because the case of a name tells a component from an element in JSX and a type
// from a value.
func neutralName(original string, position int) string {
	prefix := helperPrefix
	if r, _ := utf8.DecodeRuneInString(original); unicode.IsUpper(r) && strings.ContainsFunc(original, unicode.IsLower) {
		prefix = strings.ToUpper(prefix[:1]) + prefix[1:]
	}
	return prefix + strconv.Itoa(position)
}

// fragments lists every fragment of the unit.
func (p plan) fragments() []fragment {
	all := []fragment{p.testFragment}
	all = append(all, p.headers...)
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

// namespaceReads merges what the unit reads from namespace objects.
func (p plan) namespaceReads() map[string]map[string]bool {
	merged := map[string]map[string]bool{}
	for _, f := range p.fragments() {
		for object, names := range f.reads {
			if merged[object] == nil {
				merged[object] = map[string]bool{}
			}
			for name := range names {
				merged[object][name] = true
			}
		}
	}
	return merged
}
