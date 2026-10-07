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

// Kotlin source layout.
const (
	kotlinExtension        = ".kt"
	kotlinDeadLocal        = "val %s = 0"
	kotlinUnusedConstant   = "private const val %s = 0"
	kotlinValKeyword       = "val"
	kotlinAssignToken      = "="
	kotlinOpenBraceToken   = "{"
	kotlinReturnToken      = "return"
	kotlinThrowToken       = "throw"
	kotlinNullDefault      = "null"
	kotlinIntegerDefault   = "0"
	kotlinDoubleDefault    = "0.0"
	kotlinFloatDefault     = "0.0f"
	kotlinBooleanDefault   = "false"
	kotlinMultilineComment = "multiline_comment"
)

// Node types of the tree-sitter Kotlin grammar.
const (
	kotlinSourceFile       = "source_file"
	kotlinFunction         = "function_declaration"
	kotlinProperty         = "property_declaration"
	kotlinPropertyKind     = "binding_pattern_kind"
	kotlinVariable         = "variable_declaration"
	kotlinDelegate         = "property_delegate"
	kotlinGetter           = "getter"
	kotlinSetter           = "setter"
	kotlinStatements       = "statements"
	kotlinFunctionBody     = "function_body"
	kotlinLoopBody         = "control_structure_body"
	kotlinIf               = "if_expression"
	kotlinWhile            = "while_statement"
	kotlinDoWhile          = "do_while_statement"
	kotlinIntLiteral       = "integer_literal"
	kotlinRealLiteral      = "real_literal"
	kotlinUnsignedLiteral  = "unsigned_literal"
	kotlinLongLiteral      = "long_literal"
	kotlinBooleanLiteral   = "boolean_literal"
	kotlinStringLiteral    = "string_literal"
	kotlinStringContent    = "string_content"
	kotlinCall             = "call_expression"
	kotlinCallSuffix       = "call_suffix"
	kotlinArguments        = "value_arguments"
	kotlinArgument         = "value_argument"
	kotlinNavigation       = "navigation_expression"
	kotlinNavigationSuffix = "navigation_suffix"
	kotlinPrefix           = "prefix_expression"
	kotlinJump             = "jump_expression"
	kotlinCatch            = "catch_block"
	kotlinUserType         = "user_type"
	kotlinNullableType     = "nullable_type"
	kotlinTypeIdentifier   = "type_identifier"
	kotlinIdentifier       = "simple_identifier"
	kotlinInterpolated     = "interpolated_identifier"
	kotlinThis             = "this_expression"
	kotlinSuper            = "super_expression"
	kotlinLabel            = "label"
	kotlinCallableRef      = "callable_reference"
	kotlinLambda           = "lambda_literal"
	kotlinAnonymousFn      = "anonymous_function"
)

// kotlinImplicitNames are names the language binds without a declaration.
var kotlinImplicitNames = []string{"it", "field"}

var kotlinMinMax = map[string]string{"min": "max", "max": "min", "minOf": "maxOf", "maxOf": "minOf"}

// kotlinDefaults maps a declared return type to the value a returned value is
// replaced by.
var kotlinDefaults = map[string]string{
	"Int": kotlinIntegerDefault, "Long": kotlinIntegerDefault, "Short": kotlinIntegerDefault, "Byte": kotlinIntegerDefault,
	"Double": kotlinDoubleDefault, "Float": kotlinFloatDefault, "Boolean": kotlinBooleanDefault,
}

var (
	kotlinDefinitions = map[string]bool{kotlinFunction: true}
	kotlinContainers  = map[string]bool{"class_declaration": true, "object_declaration": true}
	kotlinMemberBody  = map[string]bool{"class_body": true, "enum_class_body": true}
	kotlinPassThrough = map[string]bool{
		kotlinSourceFile: true, "class_body": true, "enum_class_body": true, "companion_object": true,
	}
	kotlinComments = map[string]bool{typeLineComment: true, kotlinMultilineComment: true}
	// kotlinDeclaringParents are the nodes whose identifier declares a variable.
	kotlinDeclaringParents = map[string]bool{
		kotlinVariable: true, "parameter": true, "class_parameter": true, "parameter_with_optional_type": true, kotlinCatch: true,
	}
	// kotlinNamingParents are the nodes whose identifier names something that is
	// not a variable.
	kotlinNamingParents = map[string]bool{
		kotlinFunction: true, kotlinNavigationSuffix: true, kotlinCallableRef: true, "identifier": true,
		"class_declaration": true, "object_declaration": true, "type_parameter": true, "enum_entry": true,
	}
	// kotlinOpaqueScopes end the search for the function a return leaves.
	kotlinOpaqueScopes = []string{kotlinLambda, kotlinAnonymousFn, kotlinGetter, kotlinSetter}
)

var kotlinLanguage = sync.OnceValue(grammars.KotlinLanguage)

// kotlinDialect is the Kotlin dialect of the engine.
type kotlinDialect struct{}

func (kotlinDialect) extension() string { return kotlinExtension }

func (kotlinDialect) language() *gotreesitter.Language { return kotlinLanguage() }

// wrapUnit returns the unit as it is: a function or a property is a top-level
// declaration of a Kotlin file.
func (kotlinDialect) wrapUnit(unit string) string { return unit }

func (kotlinDialect) isComment(nodeType string) bool { return kotlinComments[nodeType] }

func (kotlinDialect) isDefinition(nodeType string) bool { return kotlinDefinitions[nodeType] }

func (kotlinDialect) isContainer(nodeType string) bool { return kotlinContainers[nodeType] }

func (kotlinDialect) isConstructor(string) bool { return false }

func (kotlinDialect) isMemberBody(nodeType string) bool { return kotlinMemberBody[nodeType] }

func (kotlinDialect) isPassThrough(nodeType string) bool { return kotlinPassThrough[nodeType] }

func (kotlinDialect) topLevelMembers() bool { return true }

// assignmentName accepts a `val` property with an initializer and no accessor
// or delegate.
func (kotlinDialect) assignmentName(p *parsed, parent, node *gotreesitter.Node) (string, bool) {
	if p.typeOf(node) != kotlinProperty || !p.isPlainValProperty(node) {
		return "", false
	}
	if next := p.nextNamedSibling(parent, node); next != nil && (p.typeOf(next) == kotlinGetter || p.typeOf(next) == kotlinSetter) {
		return "", false
	}
	variables := p.childrenOfType(node, kotlinVariable)
	if len(variables) != 1 {
		return "", false
	}
	name := p.firstChildOfType(variables[0], kotlinIdentifier)
	if name == nil {
		return "", false
	}
	return p.text(name), true
}

func (p *parsed) isPlainValProperty(property *gotreesitter.Node) bool {
	kind := p.firstChildOfType(property, kotlinPropertyKind)
	if kind == nil || p.text(kind) != kotlinValKeyword || !p.hasToken(property, kotlinAssignToken) {
		return false
	}
	for _, accessor := range []string{kotlinDelegate, kotlinGetter, kotlinSetter} {
		if p.firstChildOfType(property, accessor) != nil {
			return false
		}
	}
	return true
}

// nextNamedSibling returns the named child of parent that follows node.
func (p *parsed) nextNamedSibling(parent, node *gotreesitter.Node) *gotreesitter.Node {
	siblings := p.namedChildren(parent)
	for i, sibling := range siblings {
		if sibling.StartByte() == node.StartByte() && sibling.EndByte() == node.EndByte() && i+1 < len(siblings) {
			return siblings[i+1]
		}
	}
	return nil
}

func (kotlinDialect) handlers() map[string][]handler { return kotlinHandlers }

var kotlinHandlers = map[string][]handler{
	"comparison_expression":     {relationalMutator},
	"equality_expression":       {relationalMutator},
	"additive_expression":       {arithmeticMutator},
	"multiplicative_expression": {arithmeticMutator},
	"conjunction_expression":    {andOrMutator, operandNegationMutator},
	"disjunction_expression":    {andOrMutator, operandNegationMutator},
	kotlinIf:                    {conditionNegationMutator},
	kotlinWhile:                 {conditionNegationMutator},
	kotlinDoWhile:               {conditionNegationMutator},
	kotlinIntLiteral:            {kotlinNumberMutator},
	kotlinRealLiteral:           {numberMutator},
	kotlinBooleanLiteral:        {booleanLiteralMutator},
	kotlinCall:                  {kotlinMinMaxMutator, kotlinDecimalMutator},
	kotlinJump:                  {kotlinReturnMutator, kotlinThrowMutator},
	kotlinCatch:                 {kotlinCatchMutator},
}

// condition returns the condition of an if or of a loop. A loop has no field
// for it: it is the named child that is not the body.
func (kotlinDialect) condition(p *parsed, node *gotreesitter.Node) *gotreesitter.Node {
	if p.typeOf(node) == kotlinIf {
		return p.child(node, fieldCondition)
	}
	for _, child := range p.namedChildren(node) {
		if p.typeOf(child) != kotlinLoopBody {
			return child
		}
	}
	return nil
}

func (kotlinDialect) isStringLiteral(nodeType string) bool { return nodeType == kotlinStringLiteral }

// isConstructorArgument reports whether a literal is an argument of `T(...)`
// with T capitalized, or of a static factory of a domain value type.
func (kotlinDialect) isConstructorArgument(p *parsed, up ancestry) bool {
	depth := up.skipTransparent(p, kotlinPrefix, kotlinLongLiteral)
	argument := up.up(depth)
	if argument == nil || p.typeOf(argument) != kotlinArgument {
		return false
	}
	call := up.up(depth + 3)
	if call == nil || p.typeOf(call) != kotlinCall || p.typeOf(up.up(depth+2)) != kotlinCallSuffix {
		return false
	}
	callee := call.Child(0)
	switch p.typeOf(callee) {
	case kotlinIdentifier:
		return isUpper(p.text(callee))
	case kotlinNavigation:
		receiver := callee.Child(0)
		return p.typeOf(receiver) == kotlinIdentifier && slices.Contains(constructorFactories, p.text(receiver))
	}
	return false
}

// kotlinNumberMutator shifts an integer literal other than the digits of an
// unsigned literal; the `L` of a long literal stays outside the node.
func kotlinNumberMutator(c *collector, node *gotreesitter.Node, up ancestry) {
	if parent := up.parent(); parent != nil && c.p.typeOf(parent) == kotlinUnsignedLiteral {
		return
	}
	numberMutator(c, node, up)
}

// kotlinCallee splits a call into the node naming the function and the
// receiver of a qualified call.
func (p *parsed) kotlinCallee(call *gotreesitter.Node) (name, receiver *gotreesitter.Node) {
	callee := call.Child(0)
	switch p.typeOf(callee) {
	case kotlinIdentifier:
		return callee, nil
	case kotlinNavigation:
		suffixes := p.childrenOfType(callee, kotlinNavigationSuffix)
		if len(suffixes) == 0 {
			return nil, nil
		}
		return p.firstChildOfType(suffixes[len(suffixes)-1], kotlinIdentifier), callee.Child(0)
	}
	return nil, nil
}

// kotlinArgumentNodes lists the arguments of a call.
func (p *parsed) kotlinArgumentNodes(call *gotreesitter.Node) []*gotreesitter.Node {
	suffix := p.firstChildOfType(call, kotlinCallSuffix)
	if suffix == nil {
		return nil
	}
	arguments := p.firstChildOfType(suffix, kotlinArguments)
	if arguments == nil {
		return nil
	}
	return p.childrenOfType(arguments, kotlinArgument)
}

// kotlinMinMaxMutator swaps `minOf`/`maxOf`, `min`/`max` and `Math.min`/`Math.max`
// calls; a call through a receiver needs an argument to be a binary function.
func kotlinMinMaxMutator(c *collector, node *gotreesitter.Node, _ ancestry) {
	name, receiver := c.p.kotlinCallee(node)
	if receiver != nil && len(c.p.kotlinArgumentNodes(node)) == 0 {
		return
	}
	c.swapName(name, kotlinMinMax, operator.FamilyMinMax)
}

// kotlinDecimalMutator shifts the string of `BigDecimal("…")`.
func kotlinDecimalMutator(c *collector, node *gotreesitter.Node, _ ancestry) {
	name, receiver := c.p.kotlinCallee(node)
	if receiver != nil || name == nil || c.p.text(name) != decimalConstructor {
		return
	}
	arguments := c.p.kotlinArgumentNodes(node)
	if len(arguments) != 1 {
		return
	}
	given := c.p.namedChildren(arguments[0])
	literal := given[len(given)-1]
	if c.p.typeOf(literal) != kotlinStringLiteral || literal.NamedChildCount() != 1 {
		return
	}
	if content := c.p.firstChildOfType(literal, kotlinStringContent); content != nil {
		c.proposeAll(c.p.span(content), operator.ShiftDecimalString(c.p.text(content)), operator.FamilyDecimalString)
	}
}

// isJump reports whether a jump expression starts with the keyword.
func (p *parsed) isJump(node *gotreesitter.Node, keyword string) bool {
	return node.ChildCount() > 0 && p.typeOf(node.Child(0)) == keyword
}

// kotlinReturnMutator replaces a returned value by the default of the declared
// return type of the function.
func kotlinReturnMutator(c *collector, node *gotreesitter.Node, up ancestry) {
	if !c.p.isJump(node, kotlinReturnToken) {
		return
	}
	value := c.p.firstNamedChild(node)
	if value == nil {
		return
	}
	if fallback, ok := kotlinDefaultReturn(c.p, up); ok && c.p.text(value) != fallback {
		c.propose(c.p.span(value), fallback, operator.FamilyReturnValue)
	}
}

// kotlinDefaultReturn is the default value of the declared return type of the
// function a return leaves; a lambda or an accessor has none.
func kotlinDefaultReturn(p *parsed, up ancestry) (string, bool) {
	for depth := 1; up.up(depth) != nil; depth++ {
		enclosing := up.up(depth)
		if slices.Contains(kotlinOpaqueScopes, p.typeOf(enclosing)) {
			return "", false
		}
		if p.typeOf(enclosing) == kotlinFunction {
			return kotlinDefaultOf(p, enclosing)
		}
	}
	return "", false
}

func kotlinDefaultOf(p *parsed, function *gotreesitter.Node) (string, bool) {
	if p.firstChildOfType(function, kotlinNullableType) != nil {
		return kotlinNullDefault, true
	}
	declared := p.firstChildOfType(function, kotlinUserType)
	if declared == nil {
		return "", false
	}
	fallback, ok := kotlinDefaults[p.text(declared)]
	return fallback, ok
}

// kotlinThrowMutator swaps the class of `throw X(...)`.
func kotlinThrowMutator(c *collector, node *gotreesitter.Node, _ ancestry) {
	if !c.p.isJump(node, kotlinThrowToken) {
		return
	}
	thrown := c.p.firstNamedChild(node)
	if thrown == nil || c.p.typeOf(thrown) != kotlinCall {
		return
	}
	if name, receiver := c.p.kotlinCallee(thrown); receiver == nil {
		c.swapException(name)
	}
}

// kotlinCatchMutator swaps the class of `catch (e: X)`.
func kotlinCatchMutator(c *collector, node *gotreesitter.Node, _ ancestry) {
	if caught := c.p.firstChildOfType(node, kotlinUserType); caught != nil {
		c.swapException(c.p.firstChildOfType(caught, kotlinTypeIdentifier))
	}
}

// bodyStatements lists the statements of a block-bodied function, leaving out
// the statements a label is attached to.
func (kotlinDialect) bodyStatements(p *parsed, definition *gotreesitter.Node) []*gotreesitter.Node {
	body := p.firstChildOfType(definition, kotlinFunctionBody)
	if body == nil || !p.hasToken(body, kotlinOpenBraceToken) {
		return nil
	}
	statements := p.firstChildOfType(body, kotlinStatements)
	if statements == nil {
		return nil
	}
	var found []*gotreesitter.Node
	previousIsLabel := false
	for _, statement := range p.namedChildren(statements) {
		if !previousIsLabel {
			found = append(found, statement)
		}
		previousIsLabel = p.typeOf(statement) == kotlinLabel
	}
	return found
}

func (kotlinDialect) deadLocal(name string) string { return fmt.Sprintf(kotlinDeadLocal, name) }

func (kotlinDialect) unusedConstant(name string) (string, bool) {
	return fmt.Sprintf(kotlinUnusedConstant, name), true
}

func (kotlinDialect) localDeclaration(p *parsed, node *gotreesitter.Node, up ancestry) (localDeclaration, bool) {
	parent := up.parent()
	if p.typeOf(node) != kotlinProperty || parent == nil || p.typeOf(parent) != kotlinStatements {
		return localDeclaration{}, false
	}
	variables := p.childrenOfType(node, kotlinVariable)
	if len(variables) != 1 {
		return localDeclaration{}, false
	}
	name := p.firstChildOfType(variables[0], kotlinIdentifier)
	return localDeclaration{name: name, scope: parent}, name != nil
}

func (kotlinDialect) isIdentifier(nodeType string) bool {
	return nodeType == kotlinIdentifier || nodeType == kotlinInterpolated
}

func (kotlinDialect) identifierRole(p *parsed, node *gotreesitter.Node, _ string, up ancestry) role {
	parent := up.parent()
	if parent == nil || p.typeOf(node) == kotlinInterpolated {
		return roleReference
	}
	parentType := p.typeOf(parent)
	switch {
	case kotlinDeclaringParents[parentType]:
		return roleDeclaration
	case parentType == kotlinCall && parent.Child(0).StartByte() == node.StartByte():
		return roleOther
	case parentType == kotlinArgument && p.isNamedArgument(parent, node):
		return roleOther
	case kotlinNamingParents[parentType]:
		return roleOther
	}
	return roleReference
}

// isNamedArgument reports whether the identifier is the name of `name = value`.
func (p *parsed) isNamedArgument(argument, identifier *gotreesitter.Node) bool {
	for i := 0; i+1 < argument.ChildCount(); i++ {
		if argument.Child(i).StartByte() == identifier.StartByte() {
			return p.text(argument.Child(i+1)) == kotlinAssignToken
		}
	}
	return false
}

func (kotlinDialect) isRenameBlocked(name string) bool {
	return slices.Contains(kotlinImplicitNames, name)
}

// callReferences names the calls a node makes: an unqualified call and a call
// through `this` or `super` reach the declarations of the class, any other call
// is made through a receiver or a type, and `T(...)` reaches a constructor.
func (kotlinDialect) callReferences(p *parsed, node *gotreesitter.Node) []footprint.Reference {
	switch p.typeOf(node) {
	case kotlinCall:
		name, receiver := p.kotlinCallee(node)
		if name == nil {
			return nil
		}
		return receiverReferences(p, p.text(name), receiver, kotlinThis, kotlinSuper)
	case kotlinCallableRef:
		if names := p.childrenOfType(node, kotlinIdentifier); len(names) > 0 {
			return []footprint.Reference{{Name: p.text(names[len(names)-1]), Kind: footprint.ReferenceMember}}
		}
	}
	return nil
}

// fieldReads names the members a node reads through a qualifier.
func (kotlinDialect) fieldReads(p *parsed, node *gotreesitter.Node) []string {
	if p.typeOf(node) != kotlinNavigationSuffix {
		return nil
	}
	if name := p.firstChildOfType(node, kotlinIdentifier); name != nil {
		return []string{p.text(name)}
	}
	return nil
}
