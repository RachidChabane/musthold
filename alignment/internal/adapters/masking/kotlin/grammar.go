package kotlin

// Node types of the tree-sitter Kotlin grammar.
const (
	typeAnnotation          = "annotation"
	typeAnonymousInit       = "anonymous_initializer"
	typeCallableReference   = "callable_reference"
	typeClass               = "class_declaration"
	typeClassBody           = "class_body"
	typeCompanion           = "companion_object"
	typeConstructorCall     = "constructor_invocation"
	typeEnumBody            = "enum_class_body"
	typeEnumEntry           = "enum_entry"
	typeFunction            = "function_declaration"
	typeFunctionBody        = "function_body"
	typeGetter              = "getter"
	typeIdentifier          = "identifier"
	typeImportAlias         = "import_alias"
	typeImportHeader        = "import_header"
	typeImportList          = "import_list"
	typeInterpolatedName    = "interpolated_identifier"
	typeLineComment         = "line_comment"
	typeModifiers           = "modifiers"
	typeMultiVariable       = "multi_variable_declaration"
	typeMultilineComment    = "multiline_comment"
	typeNavigationExpr      = "navigation_expression"
	typeNavigationSuffix    = "navigation_suffix"
	typeObject              = "object_declaration"
	typePackageHeader       = "package_header"
	typeProperty            = "property_declaration"
	typePropertyDelegate    = "property_delegate"
	typeSetter              = "setter"
	typeSimpleIdentifier    = "simple_identifier"
	typeStringContent       = "string_content"
	typeStringLiteral       = "string_literal"
	typeTypeIdentifier      = "type_identifier"
	typeUserType            = "user_type"
	typeValueArgument       = "value_argument"
	typeValueArguments      = "value_arguments"
	typeVariableDeclaration = "variable_declaration"
	typeWildcardImport      = "wildcard_import"
)

// Tokens of the grammar the masker reads by their text.
const (
	tokenAssign   = "="
	tokenPrivate  = "private"
	tokenThis     = "this"
	tokenBacktick = "`"
)

// Annotations of JUnit 5 and of its extensions by their simple names.
const (
	annotationTest          = "Test"
	annotationParameterized = "ParameterizedTest"
	annotationRepeated      = "RepeatedTest"
	annotationFactory       = "TestFactory"
	annotationTemplate      = "TestTemplate"
	annotationDisplayName   = "DisplayName"
	annotationNested        = "Nested"
	annotationTag           = "Tag"
	annotationMethodSource  = "MethodSource"
	elementName             = "name"
)

// Annotations of the members that run for every test of their class.
var setupAnnotations = []string{"BeforeEach", "BeforeAll", "AfterEach", "AfterAll", "RegisterExtension"}

// Annotations that name the test and so are not shown.
var hiddenAnnotations = []string{annotationDisplayName, annotationNested, annotationTag}
