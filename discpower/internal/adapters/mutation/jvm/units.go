package jvm

import (
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
)

// qualnameSeparator joins the names of a definition's enclosing containers.
const qualnameSeparator = "."

// unit is what an edit replaces: an outermost method, constructor or function
// declaration, or the declaration of one constant.
type unit struct {
	qualname    string
	owner       string
	name        string
	target      mutation.TargetKind
	constructor bool
	node        *gotreesitter.Node
	span        ecosystem.Span
	text        string
}

// scope is the position of a visited node: the names of its enclosing
// containers and whether it sits directly in a container body.
type scope struct {
	names  []string
	member bool
}

func (s scope) within(name string) scope {
	return scope{names: append(s.names[:len(s.names):len(s.names)], name)}
}

func (s scope) owner() string { return strings.Join(s.names, qualnameSeparator) }

func (s scope) qualify(name string) string {
	return strings.Join(append(s.names[:len(s.names):len(s.names)], name), qualnameSeparator)
}

// units lists the units of the file in source order. A definition nested in
// another definition is part of the outermost one, which keeps every edit on the
// span the definition locator reports for its qualname. A unit whose qualname
// another unit of the same kind shares is left out: the splicer could not tell
// the two apart.
func (p *parsed) units() []unit {
	var found []unit
	p.collectChildren(p.root(), scope{member: p.dialect.topLevelMembers()}, &found)
	return withoutAmbiguous(found)
}

func (p *parsed) collectChildren(parent *gotreesitter.Node, sc scope, found *[]unit) {
	member := sc.member && p.dialect.isPassThrough(p.typeOf(parent))
	for i := 0; i < parent.ChildCount(); i++ {
		p.visit(parent, parent.Child(i), scope{names: sc.names, member: member}, found)
	}
}

func (p *parsed) visit(parent, node *gotreesitter.Node, sc scope, found *[]unit) {
	nodeType := p.typeOf(node)
	switch {
	case p.dialect.isDefinition(nodeType):
		if u, ok := p.definitionUnit(node, sc); ok {
			*found = append(*found, u)
			return
		}
	case p.dialect.isContainer(nodeType):
		if name := p.nameOf(node); name != "" {
			p.collectContainer(node, sc.within(name), found)
			return
		}
	}
	if sc.member {
		if u, ok := p.assignmentUnit(parent, node, sc); ok {
			*found = append(*found, u)
		}
	}
	p.collectChildren(node, sc, found)
}

// collectContainer visits the children of a class-like declaration; the ones
// that hold its members start at member level.
func (p *parsed) collectContainer(container *gotreesitter.Node, inner scope, found *[]unit) {
	for i := 0; i < container.ChildCount(); i++ {
		child := container.Child(i)
		p.visit(container, child, scope{names: inner.names, member: p.dialect.isMemberBody(p.typeOf(child))}, found)
	}
}

func (p *parsed) definitionUnit(node *gotreesitter.Node, sc scope) (unit, bool) {
	name := p.nameOf(node)
	if name == "" {
		return unit{}, false
	}
	return p.newUnit(node, sc.qualify(name), sc.owner(), name, mutation.TargetDefinition), true
}

// assignmentUnit accepts a constant declaration written directly in a container
// body or at the root of a file. A declaration whose text holds a definition
// is left out: an edit of it would change that definition too.
func (p *parsed) assignmentUnit(parent, node *gotreesitter.Node, sc scope) (unit, bool) {
	name, ok := p.dialect.assignmentName(p, parent, node)
	if !ok || p.containsAny(node, p.dialect.isDefinition) {
		return unit{}, false
	}
	return p.newUnit(node, sc.qualify(name), sc.owner(), name, mutation.TargetModuleAssignment), true
}

func (p *parsed) newUnit(node *gotreesitter.Node, qualname, owner, name string, target mutation.TargetKind) unit {
	span := p.span(node)
	return unit{
		qualname:    qualname,
		owner:       owner,
		name:        name,
		target:      target,
		constructor: p.dialect.isConstructor(p.typeOf(node)),
		node:        node,
		span:        span,
		text:        string(p.source[span.Start:span.End]),
	}
}

type unitKey struct {
	target   mutation.TargetKind
	qualname string
}

func withoutAmbiguous(units []unit) []unit {
	counts := map[unitKey]int{}
	for _, u := range units {
		counts[unitKey{u.target, u.qualname}]++
	}
	unique := units[:0:0]
	for _, u := range units {
		if counts[unitKey{u.target, u.qualname}] == 1 {
			unique = append(unique, u)
		}
	}
	return unique
}
