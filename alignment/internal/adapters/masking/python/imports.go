package python

import (
	"strings"

	"github.com/odvcencio/gotreesitter"
)

// Keywords and separators of import statements.
const (
	keywordImport = "import"
	keywordFrom   = "from"
	keywordAs     = "as"
	wordSeparator = " "
	listSeparator = ", "
)

// importedName is one alias of an import statement.
type importedName struct {
	// module is the dotted module of an `import module` alias, or the module a
	// `from module import name` alias reads from, with its leading dots.
	module string
	// name is the imported name of a from-import, empty for a plain import.
	name  string
	alias string
}

func (n importedName) fromImport() bool { return n.name != "" }

// bound is the name the alias binds in the importing module.
func (n importedName) bound() string {
	switch {
	case n.alias != "":
		return n.alias
	case n.fromImport():
		return n.name
	default:
		return strings.SplitN(n.module, classnameSeparator, 2)[0]
	}
}

// accessKey is how the importing module spells the imported module when it reads
// an attribute from it: `shipping.cost` or `pkg.shipping.cost`.
func (n importedName) accessKey() string {
	switch {
	case n.alias != "":
		return n.alias
	case n.fromImport():
		return n.name
	default:
		return n.module
	}
}

func (n importedName) String() string {
	spelled := n.module
	if n.fromImport() {
		spelled = n.name
	}
	if n.alias == "" {
		return spelled
	}
	return strings.Join([]string{spelled, keywordAs, n.alias}, wordSeparator)
}

// importStatement is a top-level import statement.
type importStatement struct {
	fromModule string
	from       bool
	names      []importedName
}

// keep returns the statement reduced to the aliases whose bound name is used.
func (s importStatement) keep(used map[string]bool) (importStatement, bool) {
	kept := importStatement{from: s.from, fromModule: s.fromModule}
	for _, name := range s.names {
		if used[name.bound()] {
			kept.names = append(kept.names, name)
		}
	}
	return kept, len(kept.names) > 0
}

func (s importStatement) String() string {
	spelled := make([]string, len(s.names))
	for index, name := range s.names {
		spelled[index] = name.String()
	}
	list := strings.Join(spelled, listSeparator)
	if s.from {
		return strings.Join([]string{keywordFrom, s.fromModule, keywordImport, list}, wordSeparator)
	}
	return keywordImport + wordSeparator + list
}

// importStatements lists the top-level imports of the file in source order.
func (x *fileIndex) importStatements() []importStatement {
	var found []importStatement
	for _, statement := range x.statementsOf(x.root()) {
		switch x.typeOf(statement) {
		case typeImport:
			found = append(found, x.plainImport(statement))
		case typeImportFrom:
			found = append(found, x.fromImport(statement))
		}
	}
	return found
}

func (x *fileIndex) plainImport(statement *gotreesitter.Node) importStatement {
	parsedImport := importStatement{}
	for _, node := range x.fieldChildren(statement, fieldName) {
		module, alias := x.aliasOf(node)
		parsedImport.names = append(parsedImport.names, importedName{module: module, alias: alias})
	}
	return parsedImport
}

func (x *fileIndex) fromImport(statement *gotreesitter.Node) importStatement {
	module := x.text(x.child(statement, fieldModule))
	parsedImport := importStatement{from: true, fromModule: module}
	for _, node := range x.fieldChildren(statement, fieldName) {
		name, alias := x.aliasOf(node)
		parsedImport.names = append(parsedImport.names, importedName{module: module, name: name, alias: alias})
	}
	return parsedImport
}

// aliasOf reads a dotted name, with or without an `as` alias.
func (x *fileIndex) aliasOf(node *gotreesitter.Node) (name, alias string) {
	if x.typeOf(node) != typeAliasedImport {
		return x.text(node), ""
	}
	return x.text(x.child(node, fieldName)), x.text(x.child(node, fieldAlias))
}
