package kotlin

import (
	"slices"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/musthold/alignment/internal/platform/syntax"
	"github.com/RachidChabane/musthold/alignment/internal/platform/textedit"
)

// Qualifiers that mark an occurrence as a name that belongs to another object,
// another type or an annotation, and so is neither a declaration of the unit nor
// renamed with it.
const (
	qualifierAnnotation = "@"
	qualifierNested     = "."
)

// occurrence is one name of a fragment that may designate a declaration.
type occurrence struct {
	span textedit.Span
	name string
	// qualified is true when the name is written after a receiver, a type or in an
	// annotation; qualifier is what stands before it.
	qualified bool
	qualifier string
	// inString is true when the name is the text of a string, as the method a
	// source annotation designates.
	inString bool
	// interpolated is true for a name read into a string with `$name`.
	interpolated bool
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

// scan collects the names of a node, leaving out comments and the annotations that
// name a test.
func (x *fileIndex) scan(node *gotreesitter.Node) fragment {
	var found fragment
	x.visit(node, nil, false, &found)
	return found
}

// visit walks the node; stack holds its ancestors, nearest last.
func (x *fileIndex) visit(node *gotreesitter.Node, stack []*gotreesitter.Node, annotated bool, found *fragment) {
	switch x.Type(node) {
	case typeLineComment, typeMultilineComment:
		return
	case typeAnnotation:
		if x.hidden(node) {
			return
		}
		annotated = true
		found.occurrences = append(found.occurrences, x.sourceReferences(node)...)
	case typeSimpleIdentifier, typeTypeIdentifier, typeInterpolatedName:
		if use, counted := x.occurrenceOf(node, stack, annotated); counted {
			found.occurrences = append(found.occurrences, use)
		}
	}
	stack = append(stack, node)
	for _, child := range x.Children(node) {
		x.visit(child, stack, annotated, found)
	}
}

// hidden reports whether an annotation names a test and so is not shown.
func (x *fileIndex) hidden(node *gotreesitter.Node) bool {
	items := x.annotationItems(node)
	return len(items) == 1 && slices.Contains(hiddenAnnotations, x.annotationName(items[0]))
}

// occurrenceOf classifies an identifier by the construct it stands in.
func (x *fileIndex) occurrenceOf(node *gotreesitter.Node, stack []*gotreesitter.Node, annotated bool) (occurrence, bool) {
	use := occurrence{span: syntax.Span(node), name: strings.Trim(x.Text(node), tokenBacktick), interpolated: x.Type(node) == typeInterpolatedName}
	if annotated {
		use.qualified, use.qualifier = true, qualifierAnnotation
		return use, true
	}
	if len(stack) == 0 {
		return use, true
	}
	parent := stack[len(stack)-1]
	switch x.Type(parent) {
	case typeValueArgument:
		if x.argumentKey(parent) == use.name && same(parent.Child(0), node) {
			return use, false
		}
	case typeNavigationSuffix:
		use.qualified = true
		use.qualifier = x.receiverOf(stack)
	case typeCallableReference:
		if first := parent.Child(0); !same(first, node) {
			use.qualified, use.qualifier = true, x.Text(first)
		}
	case typeUserType:
		if first := x.childOfType(parent, typeTypeIdentifier, typeSimpleIdentifier); !same(first, node) {
			use.qualified, use.qualifier = true, qualifierNested
		}
	}
	return use, true
}

// receiverOf is the text of the expression a navigation suffix selects from.
func (x *fileIndex) receiverOf(stack []*gotreesitter.Node) string {
	if len(stack) < 2 {
		return qualifierNested
	}
	expression := stack[len(stack)-2]
	if x.Type(expression) != typeNavigationExpr {
		return qualifierNested
	}
	return x.Text(expression.Child(0))
}

// sourceReferences lists the methods a `@MethodSource` annotation designates by
// the text of its strings.
func (x *fileIndex) sourceReferences(node *gotreesitter.Node) []occurrence {
	var found []occurrence
	for _, item := range x.annotationItems(node) {
		if x.annotationName(item) != annotationMethodSource {
			continue
		}
		x.Walk(item, func(visited, _ *gotreesitter.Node, _ string) bool {
			if x.Type(visited) == typeStringContent {
				if name := x.Text(visited); name != "" && !strings.Contains(name, "#") {
					found = append(found, occurrence{span: syntax.Span(visited), name: name, inString: true})
				}
			}
			return true
		})
	}
	return found
}

// same reports whether two nodes cover the same bytes.
func same(a, b *gotreesitter.Node) bool {
	return a != nil && b != nil && a.StartByte() == b.StartByte() && a.EndByte() == b.EndByte()
}

// scanAnnotation collects the names of one annotation of a class.
func (x *fileIndex) scanAnnotation(item *gotreesitter.Node) fragment {
	var found fragment
	x.visit(item, nil, true, &found)
	return found
}
