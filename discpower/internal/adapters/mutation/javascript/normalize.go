package javascript

import (
	"errors"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
)

// ErrSyntax is returned for a source that does not parse.
var ErrSyntax = errors.New("javascript syntax error")

// wrapper is a context a unit is parsed in. Some units are not statements of a
// program: a method needs its class and a declarator needs its declaration.
type wrapper struct{ prefix, suffix string }

var (
	bareUnit         = wrapper{}
	classWrapper     = wrapper{prefix: "class __Unit {", suffix: "}"}
	constWrapper     = wrapper{prefix: "const ", suffix: ";"}
	wrappersByTarget = map[mutation.TargetKind][]wrapper{
		mutation.TargetDefinition:       {bareUnit, classWrapper},
		mutation.TargetModuleAssignment: {constWrapper},
		mutation.TargetModuleAddition:   {bareUnit},
	}
)

// Normalize reduces a unit to its canonical form, ignoring comments and layout.
// The grammar is chosen from the extension of the file path. A bare method does
// not parse at module level, so a definition that does not parse as a statement
// is parsed once more inside a class. It fails with ErrSyntax when the unit
// does not parse.
func (*Engine) Normalize(filePath string, target mutation.TargetKind, source string) (string, error) {
	parsedUnit, err := parseUnit(filePath, target, source)
	if err != nil {
		return "", err
	}
	canonical := parsedUnit.canonical()
	parsedUnit.release()
	return canonical, nil
}

// parseUnit parses a unit in the first context of its target kind it parses in.
func parseUnit(filePath string, target mutation.TargetKind, source string) (*parsed, error) {
	for _, w := range wrappersByTarget[mutation.Edit{Target: target}.Kind()] {
		parsedUnit, err := parseChecked(filePath, []byte(w.prefix+source+w.suffix))
		if !errors.Is(err, ErrSyntax) {
			return parsedUnit, err
		}
	}
	return nil, ErrSyntax
}

// unitParses reports whether a unit parses in a context of its target kind.
func (p *parsed) unitParses(target mutation.TargetKind, source string) bool {
	parsedUnit, err := parseUnit(p.path, target, source)
	if err != nil {
		return false
	}
	parsedUnit.release()
	return true
}

// canonical serializes a tree as nested lists of node types with the text of the
// leaves, leaving out comments and statement terminators. Layout does not appear
// in it, and block structure does.
func (p *parsed) canonical() string {
	var out strings.Builder
	p.writeCanonical(&out, p.root())
	return out.String()
}

func (p *parsed) writeCanonical(out *strings.Builder, node *gotreesitter.Node) {
	children := p.canonicalChildren(node)
	out.WriteByte('(')
	out.WriteString(p.typeOf(node))
	if len(children) == 0 {
		out.WriteByte(' ')
		out.WriteString(p.text(node))
	}
	for _, child := range children {
		p.writeCanonical(out, child)
	}
	out.WriteByte(')')
}

func (p *parsed) canonicalChildren(node *gotreesitter.Node) []*gotreesitter.Node {
	var children []*gotreesitter.Node
	for i := 0; i < node.ChildCount(); i++ {
		child := node.Child(i)
		if p.typeOf(child) == typeComment || (!child.IsNamed() && p.typeOf(child) == typeSemicolon) {
			continue
		}
		children = append(children, child)
	}
	return children
}
