package javascript

import (
	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/spec-proof/alignment/internal/platform/syntax"
	"github.com/RachidChabane/spec-proof/alignment/internal/platform/textedit"
)

// fileIndex is a parsed file with the edits that remove its comments.
type fileIndex struct {
	*syntax.Tree
	file     string
	comments []textedit.Edit
}

func indexFile(file string, source []byte) (*fileIndex, error) {
	lang, ok := grammarOf(file)
	if !ok {
		return nil, errNoGrammar
	}
	tree, err := syntax.Parse(lang, source)
	if err != nil {
		return nil, err
	}
	index := &fileIndex{Tree: tree, file: file}
	tree.Walk(tree.Root(), func(node, _ *gotreesitter.Node, _ string) bool {
		if tree.Type(node) == typeComment {
			index.comments = append(index.comments, textedit.RemoveText(source, syntax.Span(node)))
			return false
		}
		return true
	})
	return index, nil
}

// statementsOf lists the named children of a program or a statement block that are
// not comments.
func (x *fileIndex) statementsOf(container *gotreesitter.Node) []*gotreesitter.Node {
	var statements []*gotreesitter.Node
	for _, child := range x.Named(container) {
		if x.Type(child) != typeComment {
			statements = append(statements, child)
		}
	}
	return statements
}

// unwrapExport returns the declaration an export statement carries, and the
// statement itself otherwise.
func (x *fileIndex) unwrapExport(statement *gotreesitter.Node) *gotreesitter.Node {
	if x.Type(statement) != typeExportStatement {
		return statement
	}
	if declaration := x.Field(statement, fieldDeclaration); declaration != nil {
		return declaration
	}
	return statement
}

// declaredNames lists the names a top-level or block-level statement binds.
func (x *fileIndex) declaredNames(statement *gotreesitter.Node) []string {
	declaration := x.unwrapExport(statement)
	switch x.Type(declaration) {
	case typeFunctionDecl, typeGeneratorDecl, typeClassDeclaration, typeAbstractClass,
		typeInterface, typeTypeAlias, typeEnumDeclaration, typeFunctionSignature:
		if name := x.Field(declaration, fieldName); name != nil {
			return []string{x.Text(name)}
		}
	case typeLexical, typeVariable:
		var names []string
		for _, declarator := range x.Named(declaration) {
			if x.Type(declarator) == typeVariableDeclarator {
				names = append(names, x.boundNames(x.Field(declarator, fieldName))...)
			}
		}
		return names
	case typeExpressionStatment:
		return x.namespaceNames(declaration)
	}
	return nil
}

// namespaceNames names the TypeScript namespace an expression statement declares.
func (x *fileIndex) namespaceNames(statement *gotreesitter.Node) []string {
	for _, child := range x.Named(statement) {
		if t := x.Type(child); t == typeInternalModule || t == typeModule {
			if name := x.Field(child, fieldName); name != nil {
				return []string{x.Text(name)}
			}
		}
	}
	return nil
}

// boundNames lists the identifiers a binding pattern introduces.
func (x *fileIndex) boundNames(pattern *gotreesitter.Node) []string {
	if pattern == nil {
		return nil
	}
	var names []string
	x.Walk(pattern, func(node, _ *gotreesitter.Node, field string) bool {
		if field == fieldDefault {
			return false
		}
		switch x.Type(node) {
		case typeIdentifier, typeShorthandPattern:
			names = append(names, x.Text(node))
		}
		return true
	})
	return names
}
