package python

import (
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/spec-proof/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/internal/domain/mutation"
)

// qualnameSeparator joins the names of a definition's enclosing classes.
const qualnameSeparator = "."

// unit is what an edit replaces: an outermost function or method definition, or
// a module-level assignment of a single name.
type unit struct {
	qualname string
	owner    string
	name     string
	target   mutation.TargetKind
	node     *gotreesitter.Node
	span     ecosystem.Span
	text     string
}

// units lists the units of the file in source order. A definition nested in
// another definition is part of the outermost one, which keeps every edit on the
// span the definition locator reports for its qualname.
func (p *parsed) units() []unit {
	var found []unit
	p.collect(p.root(), nil, true, &found)
	return found
}

// collect walks the children of parent; atModule is true while every enclosing
// node is the module or a module-level statement wrapper.
func (p *parsed) collect(parent *gotreesitter.Node, scope []string, atModule bool, found *[]unit) {
	for i := 0; i < parent.ChildCount(); i++ {
		node := parent.Child(i)
		switch p.typeOf(node) {
		case typeClass:
			if name := p.child(node, fieldName); name != nil {
				p.collect(node, append(scope[:len(scope):len(scope)], p.text(name)), false, found)
			}
		case typeFunction:
			if u, ok := p.definitionUnit(node, scope); ok {
				*found = append(*found, u)
			}
		case typeAssignment:
			if u, ok := p.assignmentUnit(node); ok && atModule {
				*found = append(*found, u)
			}
		case typeExpressionStatement:
			p.collect(node, scope, atModule, found)
		default:
			p.collect(node, scope, false, found)
		}
	}
}

func (p *parsed) definitionUnit(node *gotreesitter.Node, scope []string) (unit, bool) {
	name := p.child(node, fieldName)
	if name == nil {
		return unit{}, false
	}
	names := append(scope[:len(scope):len(scope)], p.text(name))
	span := p.span(node)
	return unit{
		qualname: strings.Join(names, qualnameSeparator),
		owner:    strings.Join(scope, qualnameSeparator),
		name:     p.text(name),
		target:   mutation.TargetDefinition,
		node:     node,
		span:     span,
		text:     string(p.source[span.Start:span.End]),
	}, true
}

// assignmentUnit accepts `NAME = value` and `NAME: type = value` written
// directly in the module body.
func (p *parsed) assignmentUnit(node *gotreesitter.Node) (unit, bool) {
	left, right := p.child(node, fieldLeft), p.child(node, fieldRight)
	if left == nil || right == nil || p.typeOf(left) != typeIdentifier {
		return unit{}, false
	}
	span := p.span(node)
	return unit{
		qualname: p.text(left),
		name:     p.text(left),
		target:   mutation.TargetModuleAssignment,
		node:     node,
		span:     span,
		text:     string(p.source[span.Start:span.End]),
	}, true
}
