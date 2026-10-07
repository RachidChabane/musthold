package javascript

import (
	"strings"

	"github.com/odvcencio/gotreesitter"
)

// Spellings of an import statement.
const (
	keywordImport = "import "
	keywordType   = "type "
	keywordFrom   = " from "
	namespaceMark = "* as "
	listOpen      = "{ "
	listClose     = " }"
	listSeparator = ", "
	statementEnd  = ";"
	quote         = "'"
	defaultName   = "default"
)

// binding is one name an import brings into the file.
type binding struct {
	// local is the name the file uses, imported the name the module exports it
	// under, `default` for a default import.
	local, imported string
	// namespace is true for `* as local`.
	namespace bool
	// text is how the import spells it inside braces.
	text string
}

// importStatement is an import of a module, or a `require` of one.
type importStatement struct {
	spec     string
	typeOnly bool
	bindings []binding
}

// keep returns the statement reduced to the bindings whose local name is used.
func (s importStatement) keep(used map[string]bool) (importStatement, bool) {
	kept := importStatement{spec: s.spec, typeOnly: s.typeOnly}
	for _, candidate := range s.bindings {
		if used[candidate.local] {
			kept.bindings = append(kept.bindings, candidate)
		}
	}
	return kept, len(kept.bindings) > 0
}

// String spells the statement.
func (s importStatement) String() string {
	var heads, named []string
	for _, candidate := range s.bindings {
		switch {
		case candidate.namespace:
			heads = append(heads, namespaceMark+candidate.local)
		case candidate.imported == defaultName:
			heads = append([]string{candidate.local}, heads...)
		default:
			named = append(named, candidate.text)
		}
	}
	if len(named) > 0 {
		heads = append(heads, listOpen+strings.Join(named, listSeparator)+listClose)
	}
	prefix := keywordImport
	if s.typeOnly {
		prefix += keywordType
	}
	return prefix + strings.Join(heads, listSeparator) + keywordFrom + quote + s.spec + quote + statementEnd
}

// importStatements lists the imports of the module in source order.
func (x *fileIndex) importStatements() []importStatement {
	var found []importStatement
	for _, statement := range x.statementsOf(x.Root()) {
		if x.Type(statement) == typeImportStatement {
			found = append(found, x.importOf(statement))
		}
	}
	return found
}

func (x *fileIndex) importOf(statement *gotreesitter.Node) importStatement {
	parsed := importStatement{spec: x.stringContent(x.Field(statement, fieldSource))}
	for _, child := range x.Children(statement) {
		switch x.Type(child) {
		case tokenType:
			parsed.typeOnly = true
		case typeImportClause:
			parsed.bindings = x.clauseBindings(child)
		}
	}
	return parsed
}

func (x *fileIndex) clauseBindings(clause *gotreesitter.Node) []binding {
	var bindings []binding
	for _, part := range x.Named(clause) {
		switch x.Type(part) {
		case typeIdentifier:
			bindings = append(bindings, binding{local: x.Text(part), imported: defaultName, text: x.Text(part)})
		case typeNamespaceImport:
			if name := x.firstIdentifier(part); name != "" {
				bindings = append(bindings, binding{local: name, namespace: true})
			}
		case typeNamedImports:
			for _, specifier := range x.Named(part) {
				if x.Type(specifier) == typeImportSpecifier {
					bindings = append(bindings, x.specifierBinding(specifier))
				}
			}
		}
	}
	return bindings
}

func (x *fileIndex) specifierBinding(specifier *gotreesitter.Node) binding {
	imported := x.Text(x.Field(specifier, fieldName))
	local := imported
	if alias := x.Field(specifier, fieldAlias); alias != nil {
		local = x.Text(alias)
	}
	return binding{local: local, imported: imported, text: x.Text(specifier)}
}

func (x *fileIndex) firstIdentifier(node *gotreesitter.Node) string {
	for _, child := range x.Named(node) {
		if x.Type(child) == typeIdentifier {
			return x.Text(child)
		}
	}
	return ""
}

// requireOf reads `const a = require('m')`, `const { a, b: c } = require('m')` as
// the import it stands for.
func (x *fileIndex) requireOf(statement *gotreesitter.Node) []importStatement {
	var found []importStatement
	for _, declarator := range x.Named(x.unwrapExport(statement)) {
		if x.Type(declarator) != typeVariableDeclarator {
			continue
		}
		spec, ok := x.requiredModule(x.Field(declarator, fieldValue))
		if !ok {
			continue
		}
		bindings := x.requireBindings(x.Field(declarator, fieldName))
		if len(bindings) > 0 {
			found = append(found, importStatement{spec: spec, bindings: bindings})
		}
	}
	return found
}

func (x *fileIndex) requiredModule(value *gotreesitter.Node) (string, bool) {
	if x.Type(value) != typeCallExpression || x.Text(x.Field(value, fieldFunction)) != tokenRequire {
		return "", false
	}
	arguments := x.Named(x.Field(value, fieldArguments))
	if len(arguments) == 0 || x.Type(arguments[0]) != typeString {
		return "", false
	}
	return x.stringContent(arguments[0]), true
}

func (x *fileIndex) requireBindings(pattern *gotreesitter.Node) []binding {
	switch x.Type(pattern) {
	case typeIdentifier:
		return []binding{{local: x.Text(pattern), namespace: true}}
	case typeObjectPattern:
		var bindings []binding
		for _, child := range x.Named(pattern) {
			switch x.Type(child) {
			case typeShorthandPattern:
				bindings = append(bindings, binding{local: x.Text(child), imported: x.Text(child), text: x.Text(child)})
			case typePairPattern:
				imported, local := x.Text(x.Field(child, fieldKey)), x.Text(x.Field(child, fieldValue))
				bindings = append(bindings, binding{local: local, imported: imported, text: imported + ": " + local})
			}
		}
		return bindings
	}
	return nil
}
