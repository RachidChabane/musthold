package jvm

import (
	"fmt"
	"slices"
	"sync"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/footprint"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/operator"
)

// Java source layout.
const (
	javaExtension  = ".java"
	javaWrapPrefix = "class X {\n"
	javaWrapSuffix = "\n}"
	javaDeadLocal  = "int %s = 0;"
	javaEmptyStmt  = ";"
)

// Node types of the tree-sitter Java grammar.
const (
	javaBlockComment       = "block_comment"
	javaMethod             = "method_declaration"
	javaConstructor        = "constructor_declaration"
	javaField              = "field_declaration"
	javaConstant           = "constant_declaration"
	javaDeclarator         = "variable_declarator"
	javaModifiers          = "modifiers"
	javaBlock              = "block"
	javaConstructorBody    = "constructor_body"
	javaExplicitConstrCall = "explicit_constructor_invocation"
	javaLocalVariable      = "local_variable_declaration"
	javaIdentifier         = "identifier"
	javaTypeIdentifier     = "type_identifier"
	javaScopedType         = "scoped_type_identifier"
	javaGenericType        = "generic_type"
	javaIf                 = "if_statement"
	javaWhile              = "while_statement"
	javaDoWhile            = "do_statement"
	javaFor                = "for_statement"
	javaTernary            = "ternary_expression"
	javaBinary             = "binary_expression"
	javaIntLiteral         = "decimal_integer_literal"
	javaFloatLiteral       = "decimal_floating_point_literal"
	javaTrue               = "true"
	javaFalse              = "false"
	javaStringLiteral      = "string_literal"
	javaTextBlock          = "text_block"
	javaStringFragment     = "string_fragment"
	javaMethodInvocation   = "method_invocation"
	javaObjectCreation     = "object_creation_expression"
	javaArgumentList       = "argument_list"
	javaUnary              = "unary_expression"
	javaReturn             = "return_statement"
	javaThrow              = "throw_statement"
	javaCatch              = "catch_clause"
	javaCatchParameter     = "catch_formal_parameter"
	javaCatchType          = "catch_type"
	javaLambda             = "lambda_expression"
	javaMethodReference    = "method_reference"
	javaFieldAccess        = "field_access"
	javaThis               = "this"
	javaSuper              = "super"
	javaIntegralType       = "integral_type"
	javaBooleanType        = "boolean_type"
	javaVoidType           = "void_type"
	javaFloatingType       = "floating_point_type"
)

// Field names of the Java grammar.
const (
	javaFieldValue      = "value"
	javaFieldField      = "field"
	javaFieldKey        = "key"
	javaFieldParameters = "parameters"
)

const (
	javaStaticModifier = "static"
	javaFinalModifier  = "final"
	javaCharType       = "char"
	javaCharDefault    = `'\0'`
	javaNumberDefault  = "0"
	javaNullDefault    = "null"
)

var javaMinMax = map[string]string{"min": "max", "max": "min"}

var (
	javaDefinitions = map[string]bool{javaMethod: true, javaConstructor: true}
	javaContainers  = map[string]bool{
		"class_declaration": true, "interface_declaration": true, "enum_declaration": true, "record_declaration": true,
	}
	javaMemberBodies = map[string]bool{"class_body": true, "interface_body": true, "enum_body": true}
	javaPassThrough  = map[string]bool{"class_body": true, "interface_body": true, "enum_body": true, "enum_body_declarations": true}
	javaComments     = map[string]bool{typeLineComment: true, javaBlockComment: true}
	javaStrings      = map[string]bool{javaStringLiteral: true, javaTextBlock: true}
	// javaDeclaringParents are the nodes whose `name` field declares a variable.
	javaDeclaringParents = map[string]bool{
		javaDeclarator: true, "formal_parameter": true, javaCatchParameter: true, "enhanced_for_statement": true,
		"instanceof_expression": true, "resource": true, "type_pattern": true, "record_pattern_component": true,
	}
	// javaNamingFields are the fields that hold a name that is not a variable.
	javaNamingFields = map[string]bool{fieldName: true, javaFieldField: true, javaFieldKey: true}
	// javaNamingParents are the nodes whose identifiers are not variables.
	javaNamingParents = map[string]bool{
		"labeled_statement": true, "break_statement": true, "continue_statement": true,
		javaMethodReference: true, "scoped_identifier": true,
	}
)

var javaLanguage = sync.OnceValue(grammars.JavaLanguage)

// javaDialect is the Java dialect of the engine.
type javaDialect struct{}

func (javaDialect) extension() string { return javaExtension }

func (javaDialect) language() *gotreesitter.Language { return javaLanguage() }

// wrapUnit puts a method or a field in a class, since neither is a compilation
// unit by itself.
func (javaDialect) wrapUnit(unit string) string { return javaWrapPrefix + unit + javaWrapSuffix }

func (javaDialect) isComment(nodeType string) bool { return javaComments[nodeType] }

func (javaDialect) isDefinition(nodeType string) bool { return javaDefinitions[nodeType] }

func (javaDialect) isContainer(nodeType string) bool { return javaContainers[nodeType] }

func (javaDialect) isConstructor(nodeType string) bool { return nodeType == javaConstructor }

func (javaDialect) isMemberBody(nodeType string) bool { return javaMemberBodies[nodeType] }

func (javaDialect) isPassThrough(nodeType string) bool { return javaPassThrough[nodeType] }

func (javaDialect) topLevelMembers() bool { return false }

// assignmentName accepts a `static final` field, or an interface constant, with
// a single declarator that has an initializer.
func (javaDialect) assignmentName(p *parsed, _, node *gotreesitter.Node) (string, bool) {
	switch p.typeOf(node) {
	case javaField:
		if !p.isStaticFinal(node) {
			return "", false
		}
	case javaConstant:
	default:
		return "", false
	}
	declarators := p.childrenOfType(node, javaDeclarator)
	if len(declarators) != 1 || p.child(declarators[0], javaFieldValue) == nil {
		return "", false
	}
	return p.text(p.child(declarators[0], fieldName)), true
}

func (p *parsed) isStaticFinal(field *gotreesitter.Node) bool {
	modifiers := p.firstChildOfType(field, javaModifiers)
	return modifiers != nil && p.hasToken(modifiers, javaStaticModifier) && p.hasToken(modifiers, javaFinalModifier)
}

func (javaDialect) handlers() map[string][]handler { return javaHandlers }

var javaHandlers = map[string][]handler{
	javaBinary:           {relationalMutator, arithmeticMutator, andOrMutator, operandNegationMutator},
	javaIf:               {conditionNegationMutator},
	javaWhile:            {conditionNegationMutator},
	javaDoWhile:          {conditionNegationMutator},
	javaFor:              {conditionNegationMutator},
	javaTernary:          {conditionNegationMutator},
	javaIntLiteral:       {numberMutator},
	javaFloatLiteral:     {numberMutator},
	javaTrue:             {booleanLiteralMutator},
	javaFalse:            {booleanLiteralMutator},
	javaMethodInvocation: {javaMinMaxMutator},
	javaObjectCreation:   {javaDecimalMutator},
	javaReturn:           {javaReturnMutator},
	javaThrow:            {javaThrowMutator},
	javaCatch:            {javaCatchMutator},
}

// condition returns the condition of an if, while, do, for or ternary node. The
// first three wrap it in parentheses, which stay outside the negation.
func (javaDialect) condition(p *parsed, node *gotreesitter.Node) *gotreesitter.Node {
	condition := p.child(node, fieldCondition)
	if condition != nil && p.typeOf(condition) == typeParenthesized {
		return p.firstNamedChild(condition)
	}
	return condition
}

func (javaDialect) isStringLiteral(nodeType string) bool { return javaStrings[nodeType] }

// isConstructorArgument reports whether a literal is an argument of `new T(...)`
// or of a static factory of a domain value type.
func (javaDialect) isConstructorArgument(p *parsed, up ancestry) bool {
	depth := up.skipTransparent(p, javaUnary)
	if list := up.up(depth); list == nil || p.typeOf(list) != javaArgumentList {
		return false
	}
	call := up.up(depth + 1)
	if call == nil {
		return false
	}
	switch p.typeOf(call) {
	case javaObjectCreation:
		return true
	case javaMethodInvocation:
		receiver := p.child(call, fieldObject)
		return receiver != nil && slices.Contains(constructorFactories, p.text(receiver))
	}
	return false
}

func javaMinMaxMutator(c *collector, node *gotreesitter.Node, _ ancestry) {
	if c.p.child(node, fieldObject) != nil {
		c.swapName(c.p.child(node, fieldName), javaMinMax, operator.FamilyMinMax)
	}
}

// javaDecimalMutator shifts the string of `new BigDecimal("…")`.
func javaDecimalMutator(c *collector, node *gotreesitter.Node, _ ancestry) {
	created := c.p.child(node, fieldType)
	arguments := c.p.child(node, fieldArguments)
	if created == nil || arguments == nil || c.p.text(created) != decimalConstructor {
		return
	}
	given := c.p.namedChildren(arguments)
	if len(given) != 1 || c.p.typeOf(given[0]) != javaStringLiteral || given[0].NamedChildCount() != 1 {
		return
	}
	if content := c.p.firstChildOfType(given[0], javaStringFragment); content != nil {
		c.proposeAll(c.p.span(content), operator.ShiftDecimalString(c.p.text(content)), operator.FamilyDecimalString)
	}
}

// javaReturnMutator turns a return that closes a branch of an if into an empty
// statement and replaces a returned value by the default of the declared type.
func javaReturnMutator(c *collector, node *gotreesitter.Node, up ancestry) {
	if closesBranch(c.p, up) {
		c.propose(c.p.span(node), javaEmptyStmt, operator.FamilyReturnValue)
	}
	value := c.p.firstNamedChild(node)
	if value == nil {
		return
	}
	if fallback, ok := javaDefaultReturn(c.p, up); ok && c.p.text(value) != fallback {
		c.propose(c.p.span(value), fallback, operator.FamilyReturnValue)
	}
}

// closesBranch reports whether a statement is the direct child of the block of
// a branch of an if, or the branch itself.
func closesBranch(p *parsed, up ancestry) bool {
	parent := up.parent()
	if parent == nil {
		return false
	}
	if p.typeOf(parent) == javaIf {
		return true
	}
	owner := up.up(2)
	return p.typeOf(parent) == javaBlock && owner != nil && p.typeOf(owner) == javaIf
}

// javaDefaultReturn is the default value of the declared return type of the
// method the return statement belongs to; a lambda has none.
func javaDefaultReturn(p *parsed, up ancestry) (string, bool) {
	for depth := 1; up.up(depth) != nil; depth++ {
		switch enclosing := up.up(depth); p.typeOf(enclosing) {
		case javaLambda, javaConstructor:
			return "", false
		case javaMethod:
			return javaDefaultOf(p, p.child(enclosing, fieldType))
		}
	}
	return "", false
}

func javaDefaultOf(p *parsed, declared *gotreesitter.Node) (string, bool) {
	if declared == nil {
		return "", false
	}
	switch p.typeOf(declared) {
	case javaVoidType:
		return "", false
	case javaBooleanType:
		return boolFalse, true
	case javaIntegralType:
		if p.text(declared) == javaCharType {
			return javaCharDefault, true
		}
		return javaNumberDefault, true
	case javaFloatingType:
		return javaNumberDefault, true
	}
	return javaNullDefault, true
}

// javaThrowMutator swaps the class of `throw new X(...)`.
func javaThrowMutator(c *collector, node *gotreesitter.Node, _ ancestry) {
	thrown := c.p.firstNamedChild(node)
	if thrown != nil && c.p.typeOf(thrown) == javaObjectCreation {
		c.swapException(c.p.javaSimpleType(c.p.child(thrown, fieldType)))
	}
}

// javaCatchMutator swaps each class of `catch (X | Y e)`.
func javaCatchMutator(c *collector, node *gotreesitter.Node, _ ancestry) {
	parameter := c.p.firstChildOfType(node, javaCatchParameter)
	if parameter == nil {
		return
	}
	if caught := c.p.firstChildOfType(parameter, javaCatchType); caught != nil {
		for _, class := range c.p.namedChildren(caught) {
			c.swapException(c.p.javaSimpleType(class))
		}
	}
}

// javaSimpleType returns the node that names a class in a type position: the
// type itself for a simple or qualified name, the raw type of a generic one.
func (p *parsed) javaSimpleType(declared *gotreesitter.Node) *gotreesitter.Node {
	if declared == nil {
		return nil
	}
	switch p.typeOf(declared) {
	case javaTypeIdentifier, javaScopedType:
		return declared
	case javaGenericType:
		return p.javaSimpleType(p.firstNamedChild(declared))
	}
	return nil
}

// bodyStatements lists the statements of a method or constructor body, leaving
// out the explicit constructor invocation that must stay first.
func (javaDialect) bodyStatements(p *parsed, definition *gotreesitter.Node) []*gotreesitter.Node {
	body := p.child(definition, fieldBody)
	if body == nil || (p.typeOf(body) != javaBlock && p.typeOf(body) != javaConstructorBody) {
		return nil
	}
	var found []*gotreesitter.Node
	for _, statement := range p.namedChildren(body) {
		if p.typeOf(statement) != javaExplicitConstrCall {
			found = append(found, statement)
		}
	}
	return found
}

func (javaDialect) deadLocal(name string) string { return fmt.Sprintf(javaDeadLocal, name) }

// unusedConstant is not offered: an addition at the end of a Java file falls
// outside the class.
func (javaDialect) unusedConstant(string) (string, bool) { return "", false }

func (javaDialect) localDeclaration(p *parsed, node *gotreesitter.Node, up ancestry) (localDeclaration, bool) {
	if p.typeOf(node) != javaLocalVariable || up.parent() == nil {
		return localDeclaration{}, false
	}
	declarators := p.childrenOfType(node, javaDeclarator)
	if len(declarators) != 1 {
		return localDeclaration{}, false
	}
	return localDeclaration{name: p.child(declarators[0], fieldName), scope: up.parent()}, true
}

func (javaDialect) isIdentifier(nodeType string) bool { return nodeType == javaIdentifier }

func (javaDialect) identifierRole(p *parsed, _ *gotreesitter.Node, field string, up ancestry) role {
	parent := up.parent()
	if parent == nil {
		return roleReference
	}
	parentType := p.typeOf(parent)
	switch {
	case field == fieldName && javaDeclaringParents[parentType]:
		return roleDeclaration
	case field == javaFieldParameters || parentType == "inferred_parameters":
		return roleDeclaration
	case javaNamingFields[field] || javaNamingParents[parentType]:
		return roleOther
	}
	return roleReference
}

func (javaDialect) isRenameBlocked(string) bool { return false }

// callReferences names the calls a node makes: an unqualified call and a call
// through `this` or `super` reach the methods of the class, any other call is
// made through a receiver or a type, and `new T()` reaches a constructor.
func (javaDialect) callReferences(p *parsed, node *gotreesitter.Node) []footprint.Reference {
	switch p.typeOf(node) {
	case javaMethodInvocation:
		return javaInvocationReferences(p, node)
	case javaObjectCreation:
		if created := p.javaSimpleType(p.child(node, fieldType)); created != nil {
			return []footprint.Reference{{Name: p.text(created), Kind: footprint.ReferencePlain}}
		}
	case javaMethodReference:
		if names := p.childrenOfType(node, javaIdentifier); len(names) > 0 {
			return []footprint.Reference{{Name: p.text(names[len(names)-1]), Kind: footprint.ReferenceMember}}
		}
	}
	return nil
}

func javaInvocationReferences(p *parsed, node *gotreesitter.Node) []footprint.Reference {
	name := p.child(node, fieldName)
	if name == nil {
		return nil
	}
	return receiverReferences(p, p.text(name), p.child(node, fieldObject), javaThis, javaSuper)
}

// fieldReads names the fields a node reads through a qualifier.
func (javaDialect) fieldReads(p *parsed, node *gotreesitter.Node) []string {
	if p.typeOf(node) != javaFieldAccess {
		return nil
	}
	if field := p.child(node, javaFieldField); field != nil {
		return []string{p.text(field)}
	}
	return nil
}
