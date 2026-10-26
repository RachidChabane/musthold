package javascript

import (
	"slices"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/operator"
)

// Operator vocabularies. Strict and loose equality draw their alternatives from
// their own set, so a mutant never trades one for the other.
var (
	strictRelationalOperators = []string{"<", "<=", ">", ">=", "===", "!=="}
	looseRelationalOperators  = []string{"<", "<=", ">", ">=", "==", "!="}
	looseEqualityOperators    = []string{"==", "!="}
	arithmeticOperators       = []string{"+", "-", "*", "/", "%"}
	augmentedOperators        = []string{"+=", "-=", "*=", "/=", "%="}
	logicalOperators          = []string{"&&", "||"}
	instanceOfOperator        = "instanceof"
	negationPrefix            = "!("
	negationSuffix            = ")"
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
	typeBinary:    {relationalMutator, arithmeticMutator, logicMutator, operandNegationMutator, instanceOfMutator},
	typeAugmented: {augmentedMutator},
	typeNumber:    {numberMutator},
	typeTrue:      {booleanLiteralMutator},
	typeFalse:     {booleanLiteralMutator},
	typeIf:        {conditionNegationMutator},
	typeWhile:     {conditionNegationMutator},
	typeDoWhile:   {conditionNegationMutator},
	typeFor:       {conditionNegationMutator},
	typeTernary:   {ternaryNegationMutator},
	typeReturn:    {returnMutator},
	typeCall:      {minMaxMutator, decimalMutator},
	typeNew:       {decimalMutator},
	typeThrow:     {throwMutator},
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
// A mutation whose unit does not parse is not recorded, so every candidate
// re-parses whatever the grammar makes of the text; it reports whether the
// mutation was recorded.
func (c *collector) propose(span ecosystem.Span, replacement string, family operator.Family) bool {
	newSource := replaceIn(c.unit.text, c.unit.span.Start, span, replacement)
	if !c.p.unitParses(c.unit.target, newSource) {
		return false
	}
	c.out = append(c.out, operator.Candidate{
		Site:     operator.Site{File: c.file, Qualname: c.unit.qualname, Target: c.unit.target, Span: span},
		Family:   family,
		Replaces: c.unit.span,
		Original: c.unit.text,
		Edit:     mutation.Edit{File: c.file, Qualname: c.unit.qualname, Target: c.unit.target, NewSource: newSource},
	})
	return true
}

func (c *collector) proposeAll(span ecosystem.Span, replacements []string, family operator.Family) {
	for _, replacement := range replacements {
		c.propose(span, replacement, family)
	}
}

// operatorOf is the operator token of a binary or augmented expression and its
// text; the token is nil when the node has none.
func (c *collector) operatorOf(node *gotreesitter.Node) (*gotreesitter.Node, string) {
	token := c.p.child(node, fieldOperator)
	if token == nil {
		return nil, ""
	}
	return token, c.p.text(token)
}

// swapOperator replaces the operator token by each other member of the
// vocabulary when the operator belongs to it.
func (c *collector) swapOperator(node *gotreesitter.Node, vocabulary []string, family operator.Family) {
	if token, current := c.operatorOf(node); token != nil && slices.Contains(vocabulary, current) {
		c.proposeAll(c.p.span(token), operator.Alternatives(current, vocabulary), family)
	}
}

func relationalMutator(c *collector, node *gotreesitter.Node, _ string, _ ancestry) {
	_, current := c.operatorOf(node)
	vocabulary := strictRelationalOperators
	if slices.Contains(looseEqualityOperators, current) {
		vocabulary = looseRelationalOperators
	}
	c.swapOperator(node, vocabulary, operator.FamilyRelational)
}

func arithmeticMutator(c *collector, node *gotreesitter.Node, _ string, _ ancestry) {
	c.swapOperator(node, arithmeticOperators, operator.FamilyArithmetic)
}

func augmentedMutator(c *collector, node *gotreesitter.Node, _ string, _ ancestry) {
	c.swapOperator(node, augmentedOperators, operator.FamilyArithmetic)
}

func logicMutator(c *collector, node *gotreesitter.Node, _ string, _ ancestry) {
	c.swapOperator(node, logicalOperators, operator.FamilyAndOr)
}

func (c *collector) isLogical(node *gotreesitter.Node) bool {
	if c.p.typeOf(node) != typeBinary {
		return false
	}
	_, current := c.operatorOf(node)
	return slices.Contains(logicalOperators, current)
}

// operandNegationMutator negates each operand of a logical operator that is not
// itself a logical expression.
func operandNegationMutator(c *collector, node *gotreesitter.Node, _ string, _ ancestry) {
	if !c.isLogical(node) {
		return
	}
	for _, field := range []string{fieldLeft, fieldRight} {
		if operand := c.p.child(node, field); operand != nil && !c.isLogical(operand) {
			c.negate(operand)
		}
	}
}

func (c *collector) negate(operand *gotreesitter.Node) {
	c.propose(c.p.span(operand), negationPrefix+c.p.text(operand)+negationSuffix, operator.FamilyBooleanLogical)
}

// conditionNegationMutator negates the condition of an if, while, do-while or
// for statement inside the parentheses the statement already has.
func conditionNegationMutator(c *collector, node *gotreesitter.Node, _ string, _ ancestry) {
	if inner := c.conditionExpression(c.p.child(node, fieldCondition)); inner != nil {
		c.negate(inner)
	}
}

// conditionExpression is the expression a statement condition holds: the
// content of its parentheses, or of the expression statement of a for loop.
func (c *collector) conditionExpression(condition *gotreesitter.Node) *gotreesitter.Node {
	if condition == nil {
		return nil
	}
	switch c.p.typeOf(condition) {
	case typeParenthesized, typeExpressionStatement:
		return c.p.firstNamed(condition)
	case typeEmptyStatement:
		return nil
	}
	return condition
}

// ternaryNegationMutator negates the test of `test ? value : other`.
func ternaryNegationMutator(c *collector, node *gotreesitter.Node, _ string, _ ancestry) {
	if condition := c.p.child(node, fieldCondition); condition != nil {
		c.negate(condition)
	}
}
