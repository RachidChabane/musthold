package java

// Node types of the tree-sitter Java grammar.
const (
	typeAnnotation         = "annotation"
	typeAnnotationArgs     = "annotation_argument_list"
	typeAnnotationType     = "annotation_type_declaration"
	typeBlock              = "block"
	typeBlockComment       = "block_comment"
	typeClass              = "class_declaration"
	typeClassBody          = "class_body"
	typeConstructor        = "constructor_declaration"
	typeElementValuePair   = "element_value_pair"
	typeEnum               = "enum_declaration"
	typeEnumBody           = "enum_body"
	typeEnumConstant       = "enum_constant"
	typeEnumDeclarations   = "enum_body_declarations"
	typeField              = "field_declaration"
	typeFieldAccess        = "field_access"
	typeIdentifier         = "identifier"
	typeImport             = "import_declaration"
	typeInterface          = "interface_declaration"
	typeInterfaceBody      = "interface_body"
	typeLineComment        = "line_comment"
	typeMarkerAnnotation   = "marker_annotation"
	typeMethod             = "method_declaration"
	typeMethodInvocation   = "method_invocation"
	typeMethodReference    = "method_reference"
	typeModifiers          = "modifiers"
	typePackage            = "package_declaration"
	typeRecord             = "record_declaration"
	typeScopedIdentifier   = "scoped_identifier"
	typeScopedType         = "scoped_type_identifier"
	typeStaticInitializer  = "static_initializer"
	typeStringFragment     = "string_fragment"
	typeStringLiteral      = "string_literal"
	typeTypeIdentifier     = "type_identifier"
	typeVariableDeclarator = "variable_declarator"
	typeAsterisk           = "asterisk"
)

// Field names of the grammar.
const (
	fieldBody       = "body"
	fieldDeclarator = "declarator"
	fieldField      = "field"
	fieldKey        = "key"
	fieldName       = "name"
	fieldObject     = "object"
	fieldScope      = "scope"
	fieldValue      = "value"
	fieldArguments  = "arguments"
)

// Tokens of the grammar the masker reads by their text.
const (
	tokenStatic  = "static"
	tokenPrivate = "private"
	tokenThis    = "this"
	tokenSuper   = "super"
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

// Annotations of the methods and fields that run for every test of their class.
var setupAnnotations = []string{"BeforeEach", "BeforeAll", "AfterEach", "AfterAll", "RegisterExtension"}

// Annotations that name the test and so are not shown.
var hiddenAnnotations = []string{annotationDisplayName, annotationNested, annotationTag}
