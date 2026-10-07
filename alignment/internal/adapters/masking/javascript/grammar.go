package javascript

import (
	"path"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Extensions of the files the masker serves, by grammar.
var (
	javascriptExtensions = []string{".js", ".jsx", ".mjs", ".cjs"}
	typescriptExtensions = []string{".ts", ".mts", ".cts"}
	tsxExtensions        = []string{".tsx"}
)

// grammarOf picks the tree-sitter grammar of a file by its extension.
func grammarOf(file string) (*gotreesitter.Language, bool) {
	extension := path.Ext(file)
	switch {
	case contains(javascriptExtensions, extension):
		return grammars.JavascriptLanguage(), true
	case contains(typescriptExtensions, extension):
		return grammars.TypescriptLanguage(), true
	case contains(tsxExtensions, extension):
		return grammars.TsxLanguage(), true
	}
	return nil, false
}

func contains(list []string, item string) bool {
	for _, candidate := range list {
		if candidate == item {
			return true
		}
	}
	return false
}

// sourceExtensions lists every extension a module specifier may resolve to, in
// the order they are tried.
var sourceExtensions = []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".mts", ".cts"}

// Node types of the tree-sitter JavaScript and TypeScript grammars.
const (
	typeAbstractClass      = "abstract_class_declaration"
	typeArguments          = "arguments"
	typeArrowFunction      = "arrow_function"
	typeCallExpression     = "call_expression"
	typeClassDeclaration   = "class_declaration"
	typeComment            = "comment"
	typeEnumDeclaration    = "enum_declaration"
	typeExportClause       = "export_clause"
	typeExportSpecifier    = "export_specifier"
	typeExportStatement    = "export_statement"
	typeExpressionStatment = "expression_statement"
	typeFunctionDecl       = "function_declaration"
	typeFunctionExpression = "function_expression"
	typeFunctionOld        = "function"
	typeFunctionSignature  = "function_signature"
	typeGeneratorDecl      = "generator_function_declaration"
	typeIdentifier         = "identifier"
	typeImportClause       = "import_clause"
	typeImportSpecifier    = "import_specifier"
	typeImportStatement    = "import_statement"
	typeInterface          = "interface_declaration"
	typeInternalModule     = "internal_module"
	typeLexical            = "lexical_declaration"
	typeMemberExpression   = "member_expression"
	typeMethodDefinition   = "method_definition"
	typeModule             = "module"
	typeNamedImports       = "named_imports"
	typeNamespaceImport    = "namespace_import"
	typeObjectPattern      = "object_pattern"
	typePairPattern        = "pair_pattern"
	typePrivateProperty    = "private_property_identifier"
	typePublicField        = "public_field_definition"
	typeFieldDefinition    = "field_definition"
	typeShorthandProperty  = "shorthand_property_identifier"
	typeShorthandPattern   = "shorthand_property_identifier_pattern"
	typeStatementBlock     = "statement_block"
	typeString             = "string"
	typeTemplateString     = "template_string"
	typeTypeAlias          = "type_alias_declaration"
	typeTypeIdentifier     = "type_identifier"
	typeVariable           = "variable_declaration"
	typeVariableDeclarator = "variable_declarator"
	typeAccessibility      = "accessibility_modifier"
)

// Field names of the grammars.
const (
	fieldAlias       = "alias"
	fieldArguments   = "arguments"
	fieldBody        = "body"
	fieldDeclaration = "declaration"
	fieldFunction    = "function"
	fieldName        = "name"
	fieldObject      = "object"
	fieldProperty    = "property"
	fieldSource      = "source"
	fieldType        = "type"
	fieldValue       = "value"
	fieldKey         = "key"
	// fieldDefault is the field an assignment pattern keeps its default value in.
	fieldDefault = "right"
)

// Tokens of the grammars the masker reads by their text.
const (
	tokenPrivate = "private"
	tokenType    = "type"
	tokenDefault = "default"
	tokenStar    = "*"
	tokenRequire = "require"
)
