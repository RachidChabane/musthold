package java

import (
	"strings"

	"github.com/odvcencio/gotreesitter"
)

const pathSeparator = "."

// importDeclaration is an import of the file.
type importDeclaration struct {
	// text is the declaration as it is shown.
	text string
	// path is the imported class, member or package.
	path     string
	static   bool
	wildcard bool
}

// simple is the name the import binds; a wildcard binds none.
func (d importDeclaration) simple() string {
	if d.wildcard {
		return ""
	}
	return d.path[strings.LastIndex(d.path, pathSeparator)+1:]
}

// owner is what holds the imported name: the package of a class, the class of a
// static member.
func (d importDeclaration) owner() string {
	if d.wildcard {
		return d.path
	}
	return d.path[:max(strings.LastIndex(d.path, pathSeparator), 0)]
}

// keep reports whether the unit uses what the import binds; a wildcard import
// stays because it cannot be told from its names.
func (d importDeclaration) keep(used map[string]bool) bool {
	return d.wildcard || used[d.simple()]
}

// importDeclarations lists the imports of the file in source order.
func (x *fileIndex) importDeclarations() []importDeclaration {
	var found []importDeclaration
	for _, child := range x.childrenOf(x.Root()) {
		if x.Type(child) == typeImport {
			found = append(found, x.importOf(child))
		}
	}
	return found
}

func (x *fileIndex) importOf(node *gotreesitter.Node) importDeclaration {
	declaration := importDeclaration{text: strings.Join(strings.Fields(x.Text(node)), " ")}
	for _, child := range x.Children(node) {
		switch x.Type(child) {
		case typeScopedIdentifier, typeIdentifier:
			declaration.path = x.Text(child)
		case typeAsterisk:
			declaration.wildcard = true
		default:
			declaration.static = declaration.static || x.Text(child) == tokenStatic
		}
	}
	return declaration
}
