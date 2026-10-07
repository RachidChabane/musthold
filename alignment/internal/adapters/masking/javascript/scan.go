package javascript

import (
	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/spec-proof/alignment/internal/platform/syntax"
	"github.com/RachidChabane/spec-proof/alignment/internal/platform/textedit"
)

// occurrence is one identifier of a fragment that may name a declaration.
type occurrence struct {
	span textedit.Span
	name string
	// shorthand is true for `{ name }` of an object or a pattern, where renaming
	// the identifier must keep the property key.
	shorthand bool
}

// fragment is what a scan learns about a statement: the identifiers it holds and
// the attributes it reads from namespace objects.
type fragment struct {
	occurrences []occurrence
	// reads maps an object such as `rates` to the properties read from it.
	reads map[string]map[string]bool
}

// scan collects the fragment of a node, leaving out the subtree skip, which may
// be nil. Comments hold no code and are not scanned.
func (x *fileIndex) scan(node, skip *gotreesitter.Node) fragment {
	found := fragment{reads: map[string]map[string]bool{}}
	x.Walk(node, func(visited, _ *gotreesitter.Node, _ string) bool {
		if skip != nil && visited.StartByte() == skip.StartByte() && visited.EndByte() == skip.EndByte() {
			return false
		}
		switch x.Type(visited) {
		case typeComment:
			return false
		case typeIdentifier, typeTypeIdentifier:
			found.occurrences = append(found.occurrences, occurrence{span: syntax.Span(visited), name: x.Text(visited)})
		case typeShorthandProperty, typeShorthandPattern:
			found.occurrences = append(found.occurrences, occurrence{span: syntax.Span(visited), name: x.Text(visited), shorthand: true})
		case typeMemberExpression:
			x.noteRead(&found, visited)
		}
		return true
	})
	return found
}

func (x *fileIndex) noteRead(found *fragment, member *gotreesitter.Node) {
	object := x.Field(member, fieldObject)
	if x.Type(object) != typeIdentifier {
		return
	}
	key, property := x.Text(object), x.Text(x.Field(member, fieldProperty))
	if found.reads[key] == nil {
		found.reads[key] = map[string]bool{}
	}
	found.reads[key][property] = true
}

// names lists the identifier names the fragment holds.
func (f fragment) names() []string {
	names := make([]string, len(f.occurrences))
	for index, use := range f.occurrences {
		names[index] = use.name
	}
	return names
}
