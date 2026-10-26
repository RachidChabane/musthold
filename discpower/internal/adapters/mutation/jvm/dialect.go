package jvm

import (
	"path"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/footprint"
)

// role says what an identifier occurrence is in its position.
type role int

const (
	// roleReference reads or writes a variable.
	roleReference role = iota
	// roleDeclaration introduces a variable or a parameter.
	roleDeclaration
	// roleOther names a method, a field, a type, a label or a named argument.
	roleOther
)

// localDeclaration is a local variable declaration the rename sentinel may
// rename: the declared name and the node whose span bounds the variable's scope.
type localDeclaration struct {
	name  *gotreesitter.Node
	scope *gotreesitter.Node
}

// dialect is the Strategy of one language of the engine: the grammar, the node
// vocabulary and the syntax-specific rules. Everything that does not depend on
// the shape of a grammar is written once, over this interface.
type dialect interface {
	extension() string
	language() *gotreesitter.Language
	// wrapUnit makes the source of one unit parse as a whole file.
	wrapUnit(unit string) string
	isComment(nodeType string) bool

	// Units.
	isDefinition(nodeType string) bool
	isContainer(nodeType string) bool
	isConstructor(nodeType string) bool
	// isMemberBody reports whether a container's child holds its members.
	isMemberBody(nodeType string) bool
	// isPassThrough reports whether the members of a node's children stay
	// members of the container.
	isPassThrough(nodeType string) bool
	// topLevelMembers reports whether declarations at the root of a file are
	// members that can be assignment units.
	topLevelMembers() bool
	// assignmentName returns the name of the constant declared by a member node
	// that is an assignment unit.
	assignmentName(p *parsed, parent, node *gotreesitter.Node) (string, bool)

	// Operator mutants.
	handlers() map[string][]handler
	// condition returns the condition expression of a branching node.
	condition(p *parsed, node *gotreesitter.Node) *gotreesitter.Node
	isStringLiteral(nodeType string) bool
	isConstructorArgument(p *parsed, up ancestry) bool

	// Sentinels.
	bodyStatements(p *parsed, definition *gotreesitter.Node) []*gotreesitter.Node
	deadLocal(name string) string
	unusedConstant(name string) (string, bool)
	localDeclaration(p *parsed, node *gotreesitter.Node, up ancestry) (localDeclaration, bool)
	isIdentifier(nodeType string) bool
	identifierRole(p *parsed, node *gotreesitter.Node, field string, up ancestry) role
	isRenameBlocked(name string) bool

	// Call graph.
	callReferences(p *parsed, node *gotreesitter.Node) []footprint.Reference
	fieldReads(p *parsed, node *gotreesitter.Node) []string
}

// dialects lists the dialects of the engine.
var dialects = []dialect{javaDialect{}, kotlinDialect{}}

// dialectFor returns the dialect of a file path by its extension.
func dialectFor(filePath string) (dialect, bool) {
	extension := path.Ext(filePath)
	for _, d := range dialects {
		if d.extension() == extension {
			return d, true
		}
	}
	return nil, false
}

// extensions lists the extensions of every dialect.
func extensions() []string {
	found := make([]string, 0, len(dialects))
	for _, d := range dialects {
		found = append(found, d.extension())
	}
	return found
}
