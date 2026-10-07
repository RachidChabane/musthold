package python

import "strconv"

// Neutral names of the unit.
const (
	maskedTestName = "test_case"
	helperPrefix   = "helper_"
)

// renamer maps the names of the unit to neutral ones. Module-level names and
// the names of methods live in separate namespaces, as they do in Python.
type renamer struct {
	modules map[string]string
	methods map[string]string
}

// newRenamer names the test `test_case` and the helpers `helper_1`, `helper_2`,
// ... in the order the unit lists them.
func newRenamer(p plan) renamer {
	r := renamer{modules: map[string]string{}, methods: map[string]string{}}
	next := 1
	for _, helper := range p.helpers {
		r.modules[helper.name] = helperPrefix + strconv.Itoa(next)
		next++
	}
	for _, sibling := range p.siblings {
		r.methods[sibling.name] = helperPrefix + strconv.Itoa(next)
		next++
	}
	r.namespaceOf(p.test.kind)[p.test.name] = maskedTestName
	return r
}

func (r renamer) namespaceOf(kind pieceKind) map[string]string {
	if kind == methodPiece {
		return r.methods
	}
	return r.modules
}

// rename returns the neutral name of an identifier of a piece, if it has one.
func (r renamer) rename(use occurrence, kind pieceKind) (string, bool) {
	switch use.role {
	case roleReceiverAttribute:
		neutral, ok := r.methods[use.name]
		return neutral, ok
	case roleDefinitionName:
		neutral, ok := r.namespaceOf(kind)[use.name]
		return neutral, ok
	case rolePlain:
		if neutral, ok := r.modules[use.name]; ok {
			return neutral, true
		}
		if kind == methodPiece {
			neutral, ok := r.methods[use.name]
			return neutral, ok
		}
	}
	return "", false
}

// renderPiece writes a piece of the unit without what a judge must not see and
// under neutral names. A method is dedented to the level of a function.
func (x *fileIndex) renderPiece(p piece, names renamer) string {
	edits := append([]edit(nil), p.fragment.removals...)
	for _, use := range p.fragment.occurrences {
		if neutral, ok := names.rename(use, p.kind); ok {
			edits = append(edits, edit{span: use.span, replacement: neutral})
		}
	}
	text := render(x.source, spanOf(p.statement), edits)
	if p.kind == methodPiece {
		text = dedent(text, indentationOf(x.source, int(p.statement.StartByte())))
	}
	return tidy(text)
}

// usedNames lists the plain names the pieces use, which decide the imports.
func usedNames(pieces []piece) map[string]bool {
	used := map[string]bool{}
	for _, p := range pieces {
		for _, use := range p.fragment.occurrences {
			if use.role == rolePlain {
				used[use.name] = true
			}
		}
	}
	return used
}

// attributeReads merges what the pieces read from dotted objects.
func attributeReads(pieces []piece) map[string]map[string]bool {
	merged := map[string]map[string]bool{}
	for _, p := range pieces {
		for object, names := range p.fragment.attributeReads {
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
