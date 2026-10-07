package python

import (
	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/spec-proof/alignment/internal/domain/masking"
)

// Names that a method uses for the object it is called on.
var receiverNames = map[string]bool{"self": true, "cls": true}

const (
	// placeholderBody stands in for a body that held nothing but a docstring.
	placeholderBody = "..."
	lineFeed        = '\n'
	carriageReturn  = '\r'
	space           = ' '
	tab             = '\t'
)

// identifierRole tells how an identifier takes part in a fragment.
type identifierRole int

const (
	// rolePlain is a name that stands on its own: a variable, a call, a
	// parameter, a decorator.
	rolePlain identifierRole = iota
	// roleReceiverAttribute is the attribute of `self` or `cls`.
	roleReceiverAttribute
	// roleDefinitionName is the name a function or class definition gives itself.
	roleDefinitionName
)

// occurrence is one identifier of a fragment.
type occurrence struct {
	span span
	name string
	role identifierRole
}

// fragment is what a scan learns about a definition or statement: the
// identifiers it holds, the text that must not reach a judge, and the
// attributes it reads from dotted names.
type fragment struct {
	occurrences []occurrence
	removals    []edit
	// attributeReads maps a dotted object such as `parcel` or `pkg.shipping` to
	// the attributes read from it.
	attributeReads map[string]map[string]bool
}

// scanner walks a subtree once and fills a fragment.
type scanner struct {
	*parsed
	definition *gotreesitter.Node
	found      fragment
}

// scan collects the fragment of a statement or definition. Comments, docstrings
// and decorators that cite a requirement are recorded as removals, and what they
// held is not counted as a use of any name.
func (p *parsed) scan(statement *gotreesitter.Node) fragment {
	s := &scanner{parsed: p, definition: p.definitionOf(statement)}
	s.found.attributeReads = map[string]map[string]bool{}
	s.walk(statement, "", nil)
	return s.found
}

func (s *scanner) walk(node *gotreesitter.Node, field string, parent *gotreesitter.Node) {
	switch s.typeOf(node) {
	case typeComment:
		s.found.removals = append(s.found.removals, s.removeComment(node))
		return
	case typeDecorator:
		if s.citesRequirement(node) {
			s.found.removals = append(s.found.removals, s.removeLine(node))
			return
		}
	case typeFunction, typeClass:
		s.removeDocstring(node)
	case typeIdentifier:
		s.noteIdentifier(node, field, parent)
		return
	case typeAttribute:
		s.noteAttributeRead(node)
	}
	for i := 0; i < node.ChildCount(); i++ {
		s.walk(node.Child(i), node.FieldNameForChild(i, s.lang), node)
	}
}

func (s *scanner) noteIdentifier(node *gotreesitter.Node, field string, parent *gotreesitter.Node) {
	role, counted := s.roleOf(node, field, parent)
	if !counted {
		return
	}
	s.found.occurrences = append(s.found.occurrences, occurrence{span: spanOf(node), name: s.text(node), role: role})
}

// roleOf classifies an identifier. Attribute names other than those of a
// receiver, and keyword-argument names, belong to other namespaces and are not
// counted.
func (s *scanner) roleOf(node *gotreesitter.Node, field string, parent *gotreesitter.Node) (identifierRole, bool) {
	switch s.typeOf(parent) {
	case typeAttribute:
		if field != fieldAttribute {
			return rolePlain, true
		}
		return roleReceiverAttribute, s.isReceiver(s.child(parent, fieldObject))
	case typeKeywordArgument:
		return rolePlain, field != fieldName
	case typeFunction, typeClass:
		if parent == s.definition && field == fieldName {
			return roleDefinitionName, true
		}
	}
	return rolePlain, true
}

func (s *scanner) isReceiver(object *gotreesitter.Node) bool {
	return s.typeOf(object) == typeIdentifier && receiverNames[s.text(object)]
}

func (s *scanner) noteAttributeRead(attribute *gotreesitter.Node) {
	object := s.child(attribute, fieldObject)
	if kind := s.typeOf(object); kind != typeIdentifier && kind != typeAttribute {
		return
	}
	key, name := s.text(object), s.text(s.child(attribute, fieldAttribute))
	if s.found.attributeReads[key] == nil {
		s.found.attributeReads[key] = map[string]bool{}
	}
	s.found.attributeReads[key][name] = true
}

// citesRequirement reports whether a decorator is a `verifies` marker, however
// it is qualified or called.
func (s *scanner) citesRequirement(decorator *gotreesitter.Node) bool {
	statements := s.statementsOf(decorator)
	if len(statements) == 0 {
		return false
	}
	expression := statements[0]
	if s.typeOf(expression) == typeCall {
		expression = s.child(expression, fieldFunction)
	}
	switch s.typeOf(expression) {
	case typeIdentifier:
		return s.text(expression) == masking.MarkerWord
	case typeAttribute:
		return s.text(s.child(expression, fieldAttribute)) == masking.MarkerWord
	}
	return false
}

// removeLine removes a decorator with the line break and the indentation that
// follow it, so the next line takes its place.
func (s *scanner) removeLine(node *gotreesitter.Node) edit {
	end := int(node.EndByte())
	cursor := skipBlanks(s.source, end)
	if cursor < len(s.source) && s.source[cursor] == carriageReturn {
		cursor++
	}
	if cursor >= len(s.source) || s.source[cursor] != lineFeed {
		return edit{span: spanOf(node)}
	}
	return edit{span: span{start: int(node.StartByte()), end: skipBlanks(s.source, cursor+1)}}
}

func skipBlanks(source []byte, from int) int {
	for from < len(source) && (source[from] == space || source[from] == tab) {
		from++
	}
	return from
}

// removeDocstring records the removal of the docstring of a function or class.
func (s *scanner) removeDocstring(definition *gotreesitter.Node) {
	body := s.child(definition, fieldBody)
	if body == nil || s.typeOf(body) != typeBlock {
		return
	}
	statements := s.statementsOf(body)
	if len(statements) == 0 || s.typeOf(s.bare(statements[0])) != typeString {
		return
	}
	replacement := ""
	if len(statements) == 1 {
		replacement = placeholderBody
	}
	if replacement != "" {
		s.found.removals = append(s.found.removals, edit{span: spanOf(statements[0]), replacement: replacement})
		return
	}
	s.found.removals = append(s.found.removals, s.removeWholeLines(statements[0]))
}

// removeComment removes a comment, and its line when nothing else is on it.
func (s *scanner) removeComment(comment *gotreesitter.Node) edit {
	if indentationOf(s.source, int(comment.StartByte())) != blanksBefore(s.source, int(comment.StartByte())) {
		return edit{span: spanOf(comment)}
	}
	return s.removeWholeLines(comment)
}

// removeWholeLines removes the lines a node occupies, which hold nothing else.
func (s *scanner) removeWholeLines(node *gotreesitter.Node) edit {
	start := int(node.StartByte()) - indentationOf(s.source, int(node.StartByte()))
	return edit{span: span{start: start, end: s.lineEnd(int(node.EndByte()))}}
}

// lineEnd is the offset after the line break that ends the line holding offset.
func (s *scanner) lineEnd(offset int) int {
	for offset < len(s.source) && s.source[offset] != lineFeed {
		offset++
	}
	return min(offset+1, len(s.source))
}
