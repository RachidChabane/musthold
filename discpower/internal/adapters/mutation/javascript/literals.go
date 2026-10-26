package javascript

import (
	"slices"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/operator"
)

// Vocabularies of the literal and call mutators.
var (
	minMaxMethods       = []string{"min", "max"}
	mathObject          = "Math"
	decimalConstructors = []string{"Decimal", "Big", "BigNumber"}
	flippedBoolean      = map[string]string{"true": "false", "false": "true"}
	parenthesisOpen     = "("
	negativeOpen        = parenthesisOpen + "-"
	negativeClose       = ")"
	functionValueTypes  = []string{typeArrowFunction, typeFunctionExpression, typeGeneratorFunction, typeClassExpression}
)

// inTypePosition reports whether the visited node lies in a type, where a
// literal names a type instead of a value.
func inTypePosition(up ancestry, p *parsed) bool {
	return slices.ContainsFunc(up, func(n *gotreesitter.Node) bool { return p.typeOf(n) == typeLiteralType })
}

func numberMutator(c *collector, node *gotreesitter.Node, _ string, up ancestry) {
	if inTypePosition(up, c.p) {
		return
	}
	span, family := c.p.span(node), c.numberFamily(up)
	for _, literal := range operator.ShiftNumber(c.p.text(node)) {
		if !c.propose(span, literal, family) {
			c.proposeUnparenthesized(span, literal, family)
		}
	}
}

// proposeUnparenthesized proposes a negative literal without its parentheses,
// the form some grammars read where they refuse the parenthesized one, as in
// `a < (-1)`.
func (c *collector) proposeUnparenthesized(span ecosystem.Span, literal string, family operator.Family) {
	if strings.HasPrefix(literal, negativeOpen) && strings.HasSuffix(literal, negativeClose) {
		c.propose(span, strings.TrimSuffix(strings.TrimPrefix(literal, parenthesisOpen), negativeClose), family)
	}
}

// numberFamily classifies a numeric literal: an argument of a constructor, a
// constant of a module-level declarator whose value is not a function, or a
// plain constant.
func (c *collector) numberFamily(up ancestry) operator.Family {
	if c.isConstructorArgument(up) {
		return operator.FamilyConstructorArgument
	}
	if c.unit.target == mutation.TargetModuleAssignment && !c.insideFunctionValue(up) {
		return operator.FamilyModuleConstant
	}
	return operator.FamilyNumericConstant
}

func (c *collector) insideFunctionValue(up ancestry) bool {
	return slices.ContainsFunc(up, func(n *gotreesitter.Node) bool {
		return slices.Contains(functionValueTypes, c.p.typeOf(n))
	})
}

// isConstructorArgument reports whether the literal is a direct argument of a
// construction or of a capitalised call, or the value of a property of an
// object literal that is such an argument.
func (c *collector) isConstructorArgument(up ancestry) bool {
	parent := up.parent()
	if parent == nil {
		return false
	}
	switch c.p.typeOf(parent) {
	case typeArguments:
		return c.isConstructorCall(up.up(2))
	case typePair:
		object, arguments := up.up(2), up.up(3)
		return object != nil && arguments != nil && c.p.typeOf(object) == typeObject &&
			c.p.typeOf(arguments) == typeArguments && c.isConstructorCall(up.up(4))
	}
	return false
}

func (c *collector) isConstructorCall(call *gotreesitter.Node) bool {
	if call == nil {
		return false
	}
	switch c.p.typeOf(call) {
	case typeNew:
		return true
	case typeCall:
		return isUpper(c.p.calleeName(call))
	}
	return false
}

func booleanLiteralMutator(c *collector, node *gotreesitter.Node, _ string, up ancestry) {
	if inTypePosition(up, c.p) {
		return
	}
	c.propose(c.p.span(node), flippedBoolean[c.p.text(node)], operator.FamilyBooleanLiteral)
}

// minMaxMutator swaps the method of `Math.min(...)` and `Math.max(...)`.
func minMaxMutator(c *collector, node *gotreesitter.Node, _ string, _ ancestry) {
	callee := c.p.child(node, fieldFunction)
	if callee == nil || c.p.typeOf(callee) != typeMember {
		return
	}
	object, property := c.p.child(callee, fieldObject), c.p.child(callee, fieldProperty)
	if object == nil || property == nil || c.p.text(object) != mathObject {
		return
	}
	if current := c.p.text(property); slices.Contains(minMaxMethods, current) {
		c.proposeAll(c.p.span(property), operator.Alternatives(current, minMaxMethods), operator.FamilyMinMax)
	}
}

// decimalMutator shifts the string of `new Decimal("…")` and `Decimal("…")`,
// and of the same call of Big and BigNumber.
func decimalMutator(c *collector, node *gotreesitter.Node, _ string, _ ancestry) {
	if !slices.Contains(decimalConstructors, c.p.calleeName(node)) {
		return
	}
	arguments := c.p.child(node, fieldArguments)
	if arguments == nil || arguments.NamedChildCount() != 1 {
		return
	}
	literal := arguments.NamedChild(0)
	if c.p.typeOf(literal) != typeString || literal.NamedChildCount() != 1 {
		return
	}
	if content := literal.NamedChild(0); c.p.typeOf(content) == typeStringFragment {
		c.proposeAll(c.p.span(content), operator.ShiftDecimalString(c.p.text(content)), operator.FamilyDecimalString)
	}
}
