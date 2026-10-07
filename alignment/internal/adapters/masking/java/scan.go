package java

import (
	"slices"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/spec-proof/alignment/internal/platform/syntax"
	"github.com/RachidChabane/spec-proof/alignment/internal/platform/textedit"
)

// Qualifiers that mark an occurrence as a name that belongs to another object,
// another type or a package, and so is neither a declaration of the unit nor
// renamed with it.
const (
	qualifierAnnotation = "@"
	qualifierNested     = "."
)

// occurrence is one name of a fragment that may designate a declaration.
type occurrence struct {
	span textedit.Span
	name string
	// qualified is true when the name is written after a receiver, a type or a
	// package; qualifier is what stands before it.
	qualified bool
	qualifier string
	// inString is true when the name is the text of a string, as the method a
	// source annotation designates.
	inString bool
}

// fragment is what a scan learns about a node: the names it holds.
type fragment struct{ occurrences []occurrence }

func (f fragment) names() []string {
	names := make([]string, len(f.occurrences))
	for index, use := range f.occurrences {
		names[index] = use.name
	}
	return names
}

// scan collects the names of a node, leaving out comments.
func (x *fileIndex) scan(node *gotreesitter.Node) fragment {
	var found fragment
	x.Walk(node, func(visited, parent *gotreesitter.Node, field string) bool {
		switch x.Type(visited) {
		case typeLineComment, typeBlockComment:
			return false
		case typeIdentifier, typeTypeIdentifier:
			if use, counted := x.occurrenceOf(visited, parent, field); counted {
				found.occurrences = append(found.occurrences, use)
			}
		case typeAnnotation, typeMarkerAnnotation:
			if slices.Contains(hiddenAnnotations, x.annotationName(visited)) {
				return false
			}
			found.occurrences = append(found.occurrences, x.sourceReferences(visited)...)
		}
		return true
	})
	return found
}

// occurrenceOf classifies an identifier by the construct it stands in.
func (x *fileIndex) occurrenceOf(node, parent *gotreesitter.Node, field string) (occurrence, bool) {
	use := occurrence{span: syntax.Span(node), name: x.Text(node)}
	switch x.Type(parent) {
	case typeElementValuePair:
		return use, field != fieldKey
	case typeAnnotation, typeMarkerAnnotation:
		use.qualified, use.qualifier = true, qualifierAnnotation
	case typeMethodInvocation:
		if object := x.Field(parent, fieldObject); field == fieldName && object != nil {
			use.qualified, use.qualifier = true, x.Text(object)
		}
	case typeFieldAccess:
		if field == fieldField {
			use.qualified, use.qualifier = true, x.Text(x.Field(parent, fieldObject))
		}
	case typeScopedIdentifier, typeScopedType, typeMethodReference:
		if first := parent.Child(0); first.StartByte() != node.StartByte() {
			use.qualified, use.qualifier = true, x.qualifierOf(parent, node)
		}
	}
	return use, true
}

// qualifierOf is the text of what precedes the node in a qualified name.
func (x *fileIndex) qualifierOf(parent, node *gotreesitter.Node) string {
	if x.Type(parent) == typeMethodReference {
		return x.Text(parent.Child(0))
	}
	return qualifierNested
}

// sourceReferences lists the methods a `@MethodSource` annotation designates by
// the text of its strings.
func (x *fileIndex) sourceReferences(annotation *gotreesitter.Node) []occurrence {
	if x.annotationName(annotation) != annotationMethodSource {
		return nil
	}
	var found []occurrence
	x.Walk(annotation, func(visited, _ *gotreesitter.Node, _ string) bool {
		if x.Type(visited) == typeStringFragment {
			name := x.Text(visited)
			if !strings.ContainsAny(name, "#") && name != "" {
				found = append(found, occurrence{span: syntax.Span(visited), name: name, inString: true})
			}
		}
		return true
	})
	return found
}

// reaches reports whether the occurrence designates a member of the unit's own
// classes: it stands alone, after `this`, or after the name of one of them.
func (use occurrence) reaches(classes map[string]bool) bool {
	return !use.qualified || use.qualifier == tokenThis || classes[use.qualifier]
}

// reached lists the names of the fragment that designate a member of the unit's own
// classes, without repeats.
func (f fragment) reached(classes map[string]bool) []string {
	var names []string
	for _, use := range f.occurrences {
		if use.reaches(classes) && !slices.Contains(names, use.name) {
			names = append(names, use.name)
		}
	}
	return names
}
