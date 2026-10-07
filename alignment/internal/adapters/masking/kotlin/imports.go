package kotlin

import (
	"strings"

	"github.com/odvcencio/gotreesitter"
)

const pathSeparator = "."

// importDeclaration is an import of the file.
type importDeclaration struct {
	// text is the declaration as it is shown.
	text string
	// path is the imported declaration or package.
	path     string
	alias    string
	wildcard bool
}

// simple is the name the import binds in the file; a wildcard binds none.
func (d importDeclaration) simple() string {
	switch {
	case d.wildcard:
		return ""
	case d.alias != "":
		return d.alias
	}
	return d.path[strings.LastIndex(d.path, pathSeparator)+1:]
}

// keep reports whether the unit uses what the import binds; a wildcard import
// stays because it cannot be told from its names.
func (d importDeclaration) keep(used map[string]bool) bool {
	return d.wildcard || used[d.simple()]
}

// importDeclarations lists the imports of the file in source order.
func (x *fileIndex) importDeclarations() []importDeclaration {
	var found []importDeclaration
	list := x.childOfType(x.Root(), typeImportList)
	for _, header := range x.Named(list) {
		if x.Type(header) == typeImportHeader {
			found = append(found, x.importOf(header))
		}
	}
	return found
}

func (x *fileIndex) importOf(header *gotreesitter.Node) importDeclaration {
	declaration := importDeclaration{text: strings.Join(strings.Fields(x.Text(header)), " ")}
	declaration.path = x.Text(x.childOfType(header, typeIdentifier))
	declaration.wildcard = x.childOfType(header, typeWildcardImport) != nil
	if alias := x.childOfType(header, typeImportAlias); alias != nil {
		declaration.alias = x.Text(x.childOfType(alias, typeTypeIdentifier))
	}
	return declaration
}
