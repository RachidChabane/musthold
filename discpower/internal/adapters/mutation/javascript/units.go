package javascript

import (
	"slices"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
)

// qualnameSeparator joins the names of a definition's enclosing classes, as the
// shared definition locator does.
const qualnameSeparator = "."

// nameNodeTypes are the node types the shared locator reads a name from when a
// definition has no name field.
var nameNodeTypes = []string{typeIdentifier, typeSimpleIdentifier, typeTypeIdentifier, typePropertyIdentifier}

// unit is what an edit replaces: an outermost function or method definition, or
// a module-level declarator of a single name.
type unit struct {
	qualname string
	owner    string
	name     string
	target   mutation.TargetKind
	node     *gotreesitter.Node
	span     ecosystem.Span
	text     string
}

// definitionSite is a definition the shared locator reports, outermost or
// nested in another one.
type definitionSite struct {
	unit      unit
	outermost bool
}

// units lists the units of the file in source order. A definition nested in
// another definition is part of the outermost one, which keeps every edit on the
// span the definition locator reports for its qualname. A unit whose kind and
// qualname another unit shares cannot be spliced unambiguously and is left out.
func (p *parsed) units() []unit {
	sites := p.definitionSites()
	assignments := p.assignmentUnits()
	definitionCounts := map[string]int{}
	for _, site := range sites {
		definitionCounts[site.unit.qualname]++
	}
	assignmentCounts := map[string]int{}
	for _, a := range assignments {
		assignmentCounts[a.qualname]++
	}
	var found []unit
	for _, site := range sites {
		if site.outermost && definitionCounts[site.unit.qualname] == 1 {
			found = append(found, site.unit)
		}
	}
	for _, a := range assignments {
		if assignmentCounts[a.qualname] == 1 {
			found = append(found, a)
		}
	}
	slices.SortStableFunc(found, func(a, b unit) int { return a.span.Start - b.span.Start })
	return found
}

// definitionSites walks the tree exactly as the shared locator does: a scope
// entry for every class declaration and definition that has a name, definitions
// recorded under the qualname those entries give.
func (p *parsed) definitionSites() []definitionSite {
	var found []definitionSite
	p.collectDefinitions(p.root(), nil, false, &found)
	return found
}

func (p *parsed) collectDefinitions(node *gotreesitter.Node, scope []string, insideDefinition bool, found *[]definitionSite) {
	nodeType := p.typeOf(node)
	isDefinition := nodeType == typeFunctionDeclaration || nodeType == typeMethod
	if isDefinition || nodeType == typeClassDeclaration {
		if name := p.definitionName(node); name != "" {
			if isDefinition {
				*found = append(*found, definitionSite{unit: p.definitionUnit(node, name, scope), outermost: !insideDefinition})
			}
			scope = append(scope[:len(scope):len(scope)], name)
			insideDefinition = insideDefinition || isDefinition
		}
	}
	for i := 0; i < node.NamedChildCount(); i++ {
		p.collectDefinitions(node.NamedChild(i), scope, insideDefinition, found)
	}
}

// definitionName is the name of a definition or class: its name field, else the
// first direct child that carries an identifier.
func (p *parsed) definitionName(node *gotreesitter.Node) string {
	if name := p.child(node, fieldName); name != nil {
		return p.text(name)
	}
	for i := 0; i < node.NamedChildCount(); i++ {
		if child := node.NamedChild(i); slices.Contains(nameNodeTypes, p.typeOf(child)) {
			return p.text(child)
		}
	}
	return ""
}

func (p *parsed) definitionUnit(node *gotreesitter.Node, name string, scope []string) unit {
	span := p.span(node)
	return unit{
		qualname: strings.Join(append(scope[:len(scope):len(scope)], name), qualnameSeparator),
		owner:    strings.Join(scope, qualnameSeparator),
		name:     name,
		target:   mutation.TargetDefinition,
		node:     node,
		span:     span,
		text:     p.spanText(span),
	}
}

// assignmentUnits lists the declarators of the declarations written directly in
// the program or in a program-level export, `NAME = value` and
// `NAME: type = value`, whose text holds no definition.
func (p *parsed) assignmentUnits() []unit {
	var found []unit
	for i := 0; i < p.root().NamedChildCount(); i++ {
		for _, declaration := range p.moduleDeclarations(p.root().NamedChild(i)) {
			for j := 0; j < declaration.NamedChildCount(); j++ {
				if u, ok := p.assignmentUnit(declaration.NamedChild(j)); ok {
					found = append(found, u)
				}
			}
		}
	}
	return found
}

// moduleDeclarations lists the lexical and variable declarations a program
// statement is or exports.
func (p *parsed) moduleDeclarations(statement *gotreesitter.Node) []*gotreesitter.Node {
	switch p.typeOf(statement) {
	case typeLexicalDeclaration, typeVariableDeclaration:
		return []*gotreesitter.Node{statement}
	case typeExportStatement:
		var found []*gotreesitter.Node
		for i := 0; i < statement.NamedChildCount(); i++ {
			found = append(found, p.moduleDeclarations(statement.NamedChild(i))...)
		}
		return found
	}
	return nil
}

func (p *parsed) assignmentUnit(node *gotreesitter.Node) (unit, bool) {
	if p.typeOf(node) != typeDeclarator {
		return unit{}, false
	}
	name, value := p.child(node, fieldName), p.child(node, fieldValue)
	if name == nil || value == nil || p.typeOf(name) != typeIdentifier || p.holdsDefinition(node) {
		return unit{}, false
	}
	span := p.span(node)
	return unit{
		qualname: p.text(name),
		name:     p.text(name),
		target:   mutation.TargetModuleAssignment,
		node:     node,
		span:     span,
		text:     p.spanText(span),
	}, true
}

// holdsDefinition reports whether a definition the shared locator reports lies
// inside the node.
func (p *parsed) holdsDefinition(node *gotreesitter.Node) bool {
	holds := false
	p.walk(node, func(n *gotreesitter.Node, _ string, _ ancestry) bool {
		if nodeType := p.typeOf(n); nodeType == typeFunctionDeclaration || nodeType == typeMethod {
			holds = true
		}
		return !holds
	})
	return holds
}
