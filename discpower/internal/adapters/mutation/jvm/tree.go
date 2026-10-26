package jvm

import (
	"fmt"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

// Field names shared by the Java and Kotlin grammars.
const (
	fieldName      = "name"
	fieldType      = "type"
	fieldBody      = "body"
	fieldCondition = "condition"
	fieldObject    = "object"
	fieldArguments = "arguments"
)

// Node types shared by both grammars.
const (
	typeLineComment = "line_comment"
)

// Names a definition or container node may carry when the grammar has no `name`
// field; the definition locator reads them in the same way.
var nameNodeTypes = map[string]bool{
	"identifier":          true,
	"simple_identifier":   true,
	"type_identifier":     true,
	"property_identifier": true,
}

// ancestry is the chain of nodes from the root down to the visited node, the
// visited node excluded.
type ancestry []*gotreesitter.Node

func (a ancestry) parent() *gotreesitter.Node { return a.up(1) }

// up returns the n-th ancestor, 1 being the parent.
func (a ancestry) up(n int) *gotreesitter.Node {
	if n > len(a) {
		return nil
	}
	return a[len(a)-n]
}

// parsed is a parsed source of one dialect.
type parsed struct {
	dialect dialect
	lang    *gotreesitter.Language
	tree    *gotreesitter.Tree
	source  []byte
}

func parse(d dialect, source []byte) (*parsed, error) {
	lang := d.language()
	tree, err := gotreesitter.NewParser(lang).Parse(source)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return &parsed{dialect: d, lang: lang, tree: tree, source: source}, nil
}

func (p *parsed) release() { p.tree.Release() }

func (p *parsed) root() *gotreesitter.Node { return p.tree.RootNode() }

func (p *parsed) typeOf(n *gotreesitter.Node) string { return n.Type(p.lang) }

func (p *parsed) text(n *gotreesitter.Node) string { return n.Text(p.source) }

func (p *parsed) fieldOf(parent *gotreesitter.Node, index int) string {
	return parent.FieldNameForChild(index, p.lang)
}

func (p *parsed) child(n *gotreesitter.Node, field string) *gotreesitter.Node {
	return n.ChildByFieldName(field, p.lang)
}

func (p *parsed) span(n *gotreesitter.Node) ecosystem.Span {
	return ecosystem.Span{Start: int(n.StartByte()), End: int(n.EndByte())}
}

// firstChildOfType returns the first direct child of the given node type.
func (p *parsed) firstChildOfType(n *gotreesitter.Node, nodeType string) *gotreesitter.Node {
	for i := 0; i < n.ChildCount(); i++ {
		if p.typeOf(n.Child(i)) == nodeType {
			return n.Child(i)
		}
	}
	return nil
}

// childrenOfType returns the direct children of the given node type.
func (p *parsed) childrenOfType(n *gotreesitter.Node, nodeType string) []*gotreesitter.Node {
	var found []*gotreesitter.Node
	for i := 0; i < n.ChildCount(); i++ {
		if p.typeOf(n.Child(i)) == nodeType {
			found = append(found, n.Child(i))
		}
	}
	return found
}

// namedChildren lists the named children of n that are not comments.
func (p *parsed) namedChildren(n *gotreesitter.Node) []*gotreesitter.Node {
	var found []*gotreesitter.Node
	for i := 0; i < n.NamedChildCount(); i++ {
		if child := n.NamedChild(i); !p.dialect.isComment(p.typeOf(child)) {
			found = append(found, child)
		}
	}
	return found
}

// firstNamedChild returns the first named child of n that is not a comment.
func (p *parsed) firstNamedChild(n *gotreesitter.Node) *gotreesitter.Node {
	if children := p.namedChildren(n); len(children) > 0 {
		return children[0]
	}
	return nil
}

// unnamedTokens lists the unnamed children of n, the operators and keywords.
func (p *parsed) unnamedTokens(n *gotreesitter.Node) []*gotreesitter.Node {
	var found []*gotreesitter.Node
	for i := 0; i < n.ChildCount(); i++ {
		if child := n.Child(i); !child.IsNamed() {
			found = append(found, child)
		}
	}
	return found
}

// operatorToken returns the first unnamed child of n whose text is one of the
// operators, or nil.
func (p *parsed) operatorToken(n *gotreesitter.Node, operators map[string][]string) *gotreesitter.Node {
	for _, token := range p.unnamedTokens(n) {
		if _, ok := operators[p.text(token)]; ok {
			return token
		}
	}
	return nil
}

// hasToken reports whether n has an unnamed child with the given text.
func (p *parsed) hasToken(n *gotreesitter.Node, text string) bool {
	for _, token := range p.unnamedTokens(n) {
		if p.text(token) == text {
			return true
		}
	}
	return false
}

// nameOf is the name a definition or container carries: its `name` field, else
// its first child that is a name node.
func (p *parsed) nameOf(n *gotreesitter.Node) string {
	if named := p.child(n, fieldName); named != nil {
		return p.text(named)
	}
	for i := 0; i < n.NamedChildCount(); i++ {
		if child := n.NamedChild(i); nameNodeTypes[p.typeOf(child)] {
			return p.text(child)
		}
	}
	return ""
}

// walk visits every node below n, depth first, with its field name in its
// parent and its ancestors. When visit returns false the node's children are
// skipped.
func (p *parsed) walk(n *gotreesitter.Node, visit func(node *gotreesitter.Node, field string, up ancestry) bool) {
	p.walkFrom(n, "", nil, visit)
}

func (p *parsed) walkFrom(n *gotreesitter.Node, field string, up ancestry, visit func(*gotreesitter.Node, string, ancestry) bool) {
	if !visit(n, field, up) {
		return
	}
	down := append(up[:len(up):len(up)], n)
	for i := 0; i < n.ChildCount(); i++ {
		p.walkFrom(n.Child(i), p.fieldOf(n, i), down, visit)
	}
}

// containsAny reports whether a node whose type satisfies the predicate is at or
// below n.
func (p *parsed) containsAny(n *gotreesitter.Node, matches func(string) bool) bool {
	found := false
	p.walk(n, func(node *gotreesitter.Node, _ string, _ ancestry) bool {
		if matches(p.typeOf(node)) {
			found = true
		}
		return !found
	})
	return found
}

// leafNames collects the text of the name nodes at or below n.
func (p *parsed) leafNames(n *gotreesitter.Node) map[string]bool {
	found := map[string]bool{}
	p.walk(n, func(node *gotreesitter.Node, _ string, _ ancestry) bool {
		if nameNodeTypes[p.typeOf(node)] {
			found[p.text(node)] = true
		}
		return true
	})
	return found
}

// replaceIn returns unit with the absolute span replaced; unitStart is the
// absolute offset of the unit.
func replaceIn(unit string, unitStart int, span ecosystem.Span, replacement string) string {
	return unit[:span.Start-unitStart] + replacement + unit[span.End-unitStart:]
}

func isUpper(name string) bool {
	return name != "" && strings.ToUpper(name[:1]) == name[:1] && strings.ToLower(name[:1]) != name[:1]
}
