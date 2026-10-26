package python

import (
	"slices"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/operator"
)

// Operator vocabularies.
var (
	relationalOperators  = []string{"<", "<=", ">", ">=", "==", "!="}
	arithmeticOperators  = []string{"+", "-", "*", "/", "//"}
	logicalOperators     = []string{"and", "or"}
	minMaxFunctions      = []string{"min", "max"}
	decimalConstructor   = "Decimal"
	passStatement        = "pass"
	noneLiteral          = "None"
	negationPrefix       = "not ("
	negationSuffix       = ")"
	constructorFunctions = []string{"timedelta", "time", "date", "datetime", "relativedelta"}
)

// exceptionSwaps maps a built-in exception class to the class it is swapped for.
var exceptionSwaps = map[string]string{
	"ValueError":          "TypeError",
	"TypeError":           "ValueError",
	"KeyError":            "IndexError",
	"IndexError":          "KeyError",
	"LookupError":         "ValueError",
	"AttributeError":      "KeyError",
	"ArithmeticError":     "ValueError",
	"ZeroDivisionError":   "ArithmeticError",
	"OverflowError":       "ValueError",
	"NotImplementedError": "RuntimeError",
	"AssertionError":      "RuntimeError",
	"RuntimeError":        "ValueError",
	"Exception":           "RuntimeError",
}

const (
	exceptionDefault  = "RuntimeError"
	exceptionSuffix   = "Error"
	exceptionSuffixEx = "Exception"
)

// collector gathers the candidates of one unit.
type collector struct {
	p    *parsed
	file string
	unit unit
	out  []operator.Candidate
}

// handler proposes mutations for one visited node.
type handler func(c *collector, node *gotreesitter.Node, field string, up ancestry)

// handlers maps a node type to the mutators that apply to it. Adding a mutator
// is adding an entry here.
var handlers = map[string][]handler{
	typeComparison: {relationalMutator},
	typeBinary:     {arithmeticMutator},
	typeBoolean:    {logicMutator, operandNegationMutator},
	typeInteger:    {numberMutator},
	typeFloat:      {numberMutator},
	typeTrue:       {booleanLiteralMutator},
	typeFalse:      {booleanLiteralMutator},
	typeIf:         {conditionNegationMutator},
	typeElif:       {conditionNegationMutator},
	typeWhile:      {conditionNegationMutator},
	typeTernary:    {ternaryNegationMutator},
	typeReturn:     {returnMutator},
	typeCall:       {minMaxMutator, decimalMutator},
	typeRaise:      {raiseMutator},
	typeExcept:     {exceptMutator},
}

// candidates enumerates the operator mutants of every unit of the file.
func (p *parsed) candidates(file string) []operator.Candidate {
	var all []operator.Candidate
	for _, u := range p.units() {
		c := &collector{p: p, file: file, unit: u}
		p.walk(u.node, func(node *gotreesitter.Node, field string, up ancestry) bool {
			for _, h := range handlers[p.typeOf(node)] {
				h(c, node, field, up)
			}
			return true
		})
		all = append(all, c.out...)
	}
	return all
}

// propose records one mutation: the absolute span is replaced by replacement.
func (c *collector) propose(span ecosystem.Span, replacement string, family operator.Family) {
	c.out = append(c.out, operator.Candidate{
		Site:     operator.Site{File: c.file, Qualname: c.unit.qualname, Target: c.unit.target, Span: span},
		Family:   family,
		Replaces: c.unit.span,
		Original: c.unit.text,
		Edit: mutation.Edit{
			File:      c.file,
			Qualname:  c.unit.qualname,
			Target:    c.unit.target,
			NewSource: replaceIn(c.unit.text, c.unit.span.Start, span, replacement),
		},
	})
}

func (c *collector) proposeAll(span ecosystem.Span, replacements []string, family operator.Family) {
	for _, replacement := range replacements {
		c.propose(span, replacement, family)
	}
}

func relationalMutator(c *collector, node *gotreesitter.Node, _ string, _ ancestry) {
	for i := 0; i < node.ChildCount(); i++ {
		if c.p.fieldOf(node, i) != fieldOperators {
			continue
		}
		token := node.Child(i)
		if current := c.p.text(token); slices.Contains(relationalOperators, current) {
			c.proposeAll(c.p.span(token), operator.Alternatives(current, relationalOperators), operator.FamilyRelational)
		}
	}
}

func arithmeticMutator(c *collector, node *gotreesitter.Node, _ string, _ ancestry) {
	if token := c.p.child(node, fieldOperator); token != nil {
		if current := c.p.text(token); slices.Contains(arithmeticOperators, current) {
			c.proposeAll(c.p.span(token), operator.Alternatives(current, arithmeticOperators), operator.FamilyArithmetic)
		}
	}
}

func logicMutator(c *collector, node *gotreesitter.Node, _ string, _ ancestry) {
	if token := c.p.child(node, fieldOperator); token != nil {
		if current := c.p.text(token); slices.Contains(logicalOperators, current) {
			c.proposeAll(c.p.span(token), operator.Alternatives(current, logicalOperators), operator.FamilyAndOr)
		}
	}
}

// operandNegationMutator negates each operand of a boolean operator that is not
// itself a boolean operator.
func operandNegationMutator(c *collector, node *gotreesitter.Node, _ string, _ ancestry) {
	for _, field := range []string{fieldLeft, fieldRight} {
		if operand := c.p.child(node, field); operand != nil && c.p.typeOf(operand) != typeBoolean {
			c.negate(operand)
		}
	}
}

func (c *collector) negate(operand *gotreesitter.Node) {
	c.propose(c.p.span(operand), negationPrefix+c.p.text(operand)+negationSuffix, operator.FamilyBooleanLogical)
}

func conditionNegationMutator(c *collector, node *gotreesitter.Node, _ string, _ ancestry) {
	if condition := c.p.child(node, fieldCondition); condition != nil {
		c.negate(condition)
	}
}

// ternaryNegationMutator negates the test of `value if test else other`.
func ternaryNegationMutator(c *collector, node *gotreesitter.Node, _ string, _ ancestry) {
	for i := 0; i+1 < node.ChildCount(); i++ {
		if c.p.typeOf(node.Child(i)) == typeIfToken && !node.Child(i).IsNamed() {
			c.negate(node.Child(i + 1))
			return
		}
	}
}

func booleanLiteralMutator(c *collector, node *gotreesitter.Node, _ string, _ ancestry) {
	flipped := map[string]string{"True": "False", "False": "True"}[c.p.text(node)]
	if flipped != "" {
		c.propose(c.p.span(node), flipped, operator.FamilyBooleanLiteral)
	}
}

func numberMutator(c *collector, node *gotreesitter.Node, _ string, up ancestry) {
	family := c.numberFamily(up)
	c.proposeAll(c.p.span(node), operator.ShiftNumber(c.p.text(node)), family)
}

// numberFamily classifies a numeric literal: an argument of a constructor call,
// a module-level constant, or a plain constant.
func (c *collector) numberFamily(up ancestry) operator.Family {
	if call := c.enclosingCall(up); call != nil && c.isConstructor(call) {
		return operator.FamilyConstructorArgument
	}
	if c.unit.target == mutation.TargetModuleAssignment {
		return operator.FamilyModuleConstant
	}
	return operator.FamilyNumericConstant
}

// enclosingCall returns the call whose argument the visited literal is, either
// positional or keyword.
func (c *collector) enclosingCall(up ancestry) *gotreesitter.Node {
	parent := up.parent()
	if parent == nil {
		return nil
	}
	switch c.p.typeOf(parent) {
	case typeArgumentList:
		return up.up(2)
	case typeKeywordArgument:
		if list := up.up(2); list != nil && c.p.typeOf(list) == typeArgumentList {
			return up.up(3)
		}
	}
	return nil
}

func (c *collector) isConstructor(call *gotreesitter.Node) bool {
	if c.p.typeOf(call) != typeCall {
		return false
	}
	name := c.p.calleeName(call)
	return isUpper(name) || slices.Contains(constructorFunctions, name)
}

func minMaxMutator(c *collector, node *gotreesitter.Node, _ string, _ ancestry) {
	callee := c.p.child(node, fieldFunction)
	if callee == nil || c.p.typeOf(callee) != typeIdentifier {
		return
	}
	if current := c.p.text(callee); slices.Contains(minMaxFunctions, current) {
		c.proposeAll(c.p.span(callee), operator.Alternatives(current, minMaxFunctions), operator.FamilyMinMax)
	}
}

// decimalMutator shifts the string of `Decimal("…")`.
func decimalMutator(c *collector, node *gotreesitter.Node, _ string, _ ancestry) {
	if c.p.calleeName(node) != decimalConstructor {
		return
	}
	arguments := c.p.child(node, fieldArguments)
	if arguments == nil || arguments.NamedChildCount() != 1 {
		return
	}
	literal := arguments.NamedChild(0)
	if c.p.typeOf(literal) != typeString || literal.NamedChildCount() != 3 {
		return
	}
	for i := 0; i < literal.ChildCount(); i++ {
		content := literal.Child(i)
		if c.p.typeOf(content) == typeStringContent {
			c.proposeAll(c.p.span(content), operator.ShiftDecimalString(c.p.text(content)), operator.FamilyDecimalString)
		}
	}
}

// returnMutator deletes a return that closes a branch of an if (the statement
// becomes `pass`) and replaces a returned value by None.
func returnMutator(c *collector, node *gotreesitter.Node, _ string, up ancestry) {
	if block := up.parent(); block != nil && c.p.typeOf(block) == typeBlock {
		if owner := up.up(2); owner != nil && slices.Contains([]string{typeIf, typeElif, typeElse}, c.p.typeOf(owner)) {
			c.propose(c.p.span(node), passStatement, operator.FamilyReturnValue)
		}
	}
	if node.NamedChildCount() == 1 && c.p.text(node.NamedChild(0)) != noneLiteral {
		c.propose(c.p.span(node), "return "+noneLiteral, operator.FamilyReturnValue)
	}
}

func raiseMutator(c *collector, node *gotreesitter.Node, _ string, _ ancestry) {
	if node.NamedChildCount() == 0 {
		return
	}
	raised := node.NamedChild(0)
	if c.p.typeOf(raised) == typeCall {
		raised = c.p.child(raised, fieldFunction)
	}
	c.swapException(raised)
}

func exceptMutator(c *collector, node *gotreesitter.Node, _ string, _ ancestry) {
	if node.NamedChildCount() == 0 {
		return
	}
	caught := node.NamedChild(0)
	if c.p.typeOf(caught) == typeAsPattern && caught.NamedChildCount() > 0 {
		caught = caught.NamedChild(0)
	}
	c.swapException(caught)
}

func (c *collector) swapException(class *gotreesitter.Node) {
	if class == nil || c.p.typeOf(class) != typeIdentifier {
		return
	}
	if swapped := exceptionSwap(c.p.text(class)); swapped != "" {
		c.propose(c.p.span(class), swapped, operator.FamilyExceptionClass)
	}
}

// exceptionSwap names the class a raised or caught class is swapped for; empty
// when the name does not look like an exception class.
func exceptionSwap(name string) string {
	if swapped, ok := exceptionSwaps[name]; ok {
		return swapped
	}
	if strings.HasSuffix(name, exceptionSuffix) || strings.HasSuffix(name, exceptionSuffixEx) {
		return exceptionDefault
	}
	return ""
}
