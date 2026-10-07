package jvm

import (
	"slices"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/operator"
)

// Operator vocabularies.
const (
	plusOperator      = "+"
	negationPrefix    = "!("
	negationSuffix    = ")"
	typeParenthesized = "parenthesized_expression"
	boolTrue          = "true"
	boolFalse         = "false"
)

var (
	orderingOperators   = []string{"<", "<=", ">", ">="}
	equalityOperators   = []string{"==", "!="}
	arithmeticOperators = []string{plusOperator, "-", "*", "/", "%"}
	logicalOperators    = []string{"&&", "||"}
)

// Swap tables: an operator and the operators it is mutated to. Ordering
// operators also become equality tests; an equality test only becomes the other
// one, which keeps mutants on references compilable.
var (
	relationalSwaps  = relationalSwapTable()
	arithmeticSwaps  = swapsAmong(arithmeticOperators)
	logicalSwaps     = swapsAmong(logicalOperators)
	booleanLiteralOf = map[string]string{boolTrue: boolFalse, boolFalse: boolTrue}
)

func relationalSwapTable() map[string][]string {
	table := map[string][]string{}
	for _, current := range orderingOperators {
		table[current] = append(operator.Alternatives(current, orderingOperators), equalityOperators...)
	}
	for _, current := range equalityOperators {
		table[current] = operator.Alternatives(current, equalityOperators)
	}
	return table
}

func swapsAmong(vocabulary []string) map[string][]string {
	table := map[string][]string{}
	for _, current := range vocabulary {
		table[current] = operator.Alternatives(current, vocabulary)
	}
	return table
}

// collector gathers the candidates of one unit.
type collector struct {
	p    *parsed
	file string
	unit unit
	out  []operator.Candidate
}

// handler proposes mutations for one visited node.
type handler func(c *collector, node *gotreesitter.Node, up ancestry)

// candidates enumerates the operator mutants of every unit of the file.
func (p *parsed) candidates(file string) []operator.Candidate {
	handlers := p.dialect.handlers()
	var all []operator.Candidate
	for _, u := range p.units() {
		c := &collector{p: p, file: file, unit: u}
		p.walk(u.node, func(node *gotreesitter.Node, _ string, up ancestry) bool {
			for _, h := range handlers[p.typeOf(node)] {
				h(c, node, up)
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

// operatorMutator swaps the operator of a binary node according to the table.
func operatorMutator(swaps map[string][]string, family operator.Family) handler {
	return func(c *collector, node *gotreesitter.Node, _ ancestry) {
		if token := c.p.operatorToken(node, swaps); token != nil {
			c.proposeAll(c.p.span(token), swaps[c.p.text(token)], family)
		}
	}
}

var (
	relationalMutator = operatorMutator(relationalSwaps, operator.FamilyRelational)
	andOrMutator      = operatorMutator(logicalSwaps, operator.FamilyAndOr)
)

// arithmeticMutator swaps the arithmetic operator of a binary node, leaving the
// `+` of a string concatenation alone.
func arithmeticMutator(c *collector, node *gotreesitter.Node, _ ancestry) {
	token := c.p.operatorToken(node, arithmeticSwaps)
	if token == nil {
		return
	}
	current := c.p.text(token)
	if current == plusOperator && c.p.isStringConcatenation(node) {
		return
	}
	c.proposeAll(c.p.span(token), arithmeticSwaps[current], operator.FamilyArithmetic)
}

// isStringConcatenation reports whether a `+` node has a string literal in its
// chain of operands, however it is parenthesized.
func (p *parsed) isStringConcatenation(node *gotreesitter.Node) bool {
	for _, operand := range p.namedChildren(node) {
		operand = p.unparenthesized(operand)
		switch {
		case p.dialect.isStringLiteral(p.typeOf(operand)):
			return true
		case p.typeOf(operand) == p.typeOf(node) && p.hasToken(operand, plusOperator) && p.isStringConcatenation(operand):
			return true
		}
	}
	return false
}

func (p *parsed) unparenthesized(node *gotreesitter.Node) *gotreesitter.Node {
	for p.typeOf(node) == typeParenthesized {
		inner := p.firstNamedChild(node)
		if inner == nil {
			break
		}
		node = inner
	}
	return node
}

// operandNegationMutator negates each operand of a logical operator that is not
// itself a logical operator.
func operandNegationMutator(c *collector, node *gotreesitter.Node, _ ancestry) {
	if c.p.operatorToken(node, logicalSwaps) == nil {
		return
	}
	for _, operand := range c.p.namedChildren(node) {
		if c.p.operatorToken(operand, logicalSwaps) == nil {
			c.negate(operand)
		}
	}
}

func (c *collector) negate(operand *gotreesitter.Node) {
	c.propose(c.p.span(operand), negationPrefix+c.p.text(operand)+negationSuffix, operator.FamilyBooleanLogical)
}

// conditionNegationMutator negates the condition of a branching node.
func conditionNegationMutator(c *collector, node *gotreesitter.Node, _ ancestry) {
	if condition := c.p.dialect.condition(c.p, node); condition != nil {
		c.negate(condition)
	}
}

func booleanLiteralMutator(c *collector, node *gotreesitter.Node, _ ancestry) {
	if flipped, ok := booleanLiteralOf[c.p.text(node)]; ok {
		c.propose(c.p.span(node), flipped, operator.FamilyBooleanLiteral)
	}
}

// swapName replaces a method or function name node by its partner in the table.
func (c *collector) swapName(name *gotreesitter.Node, partners map[string]string, family operator.Family) {
	if name == nil {
		return
	}
	if partner, ok := partners[c.p.text(name)]; ok {
		c.propose(c.p.span(name), partner, family)
	}
}

// Exception classes: the class a known JDK exception is swapped for. Every
// target is unchecked, so the mutant keeps compiling wherever the original did.
var exceptionSwaps = map[string]string{
	"IllegalArgumentException":      "IllegalStateException",
	"IllegalStateException":         "IllegalArgumentException",
	"NullPointerException":          "IllegalArgumentException",
	"UnsupportedOperationException": "IllegalStateException",
	"ArithmeticException":           "IllegalArgumentException",
	"IndexOutOfBoundsException":     "NoSuchElementException",
	"NoSuchElementException":        "IndexOutOfBoundsException",
	"RuntimeException":              "IllegalStateException",
	"Exception":                     "RuntimeException",
}

const (
	exceptionDefault = "RuntimeException"
	exceptionSuffix  = "Exception"
	errorSuffix      = "Error"
)

// swapException proposes the replacement of the class named by node.
func (c *collector) swapException(class *gotreesitter.Node) {
	if class == nil {
		return
	}
	if swapped := exceptionSwap(simpleName(c.p.text(class))); swapped != "" {
		c.propose(c.p.span(class), swapped, operator.FamilyExceptionClass)
	}
}

// simpleName is the last segment of a possibly qualified class name.
func simpleName(name string) string {
	return name[strings.LastIndex(name, qualnameSeparator)+1:]
}

// exceptionSwap names the class a thrown or caught class is swapped for; empty
// when the name does not look like an exception class.
func exceptionSwap(name string) string {
	if swapped, ok := exceptionSwaps[name]; ok {
		return swapped
	}
	if strings.HasSuffix(name, exceptionSuffix) || strings.HasSuffix(name, errorSuffix) {
		return exceptionDefault
	}
	return ""
}

// Numeric literals.
const (
	numberSuffixes = "lLfFdD"
	signOpen       = "("
	signClose      = ")"
)

// Types whose static factories and constructors take numeric constants that
// shape a domain value.
var constructorFactories = []string{
	"Duration", "Period", "LocalDate", "LocalDateTime", "LocalTime", "Instant", "ZonedDateTime", "OffsetDateTime",
}

// numberMutator shifts a numeric literal, keeping its type suffix.
func numberMutator(c *collector, node *gotreesitter.Node, up ancestry) {
	body, suffix := splitNumberSuffix(c.p.text(node))
	family := c.numberFamily(up)
	for _, shifted := range operator.ShiftNumber(body) {
		c.propose(c.p.span(node), withNumberSuffix(shifted, suffix), family)
	}
}

func splitNumberSuffix(literal string) (body, suffix string) {
	last := len(literal) - 1
	if last > 0 && strings.ContainsRune(numberSuffixes, rune(literal[last])) {
		return literal[:last], literal[last:]
	}
	return literal, ""
}

// withNumberSuffix appends the suffix to a shifted literal; a parenthesized
// negative number carries it inside the parentheses.
func withNumberSuffix(shifted, suffix string) string {
	if strings.HasPrefix(shifted, signOpen) {
		return strings.TrimSuffix(shifted, signClose) + suffix + signClose
	}
	return shifted + suffix
}

// numberFamily classifies a numeric literal: an argument of a constructor or of
// a factory of a domain value, a constant declaration, or a plain constant.
func (c *collector) numberFamily(up ancestry) operator.Family {
	if c.p.dialect.isConstructorArgument(c.p, up) {
		return operator.FamilyConstructorArgument
	}
	if c.unit.target == mutation.TargetModuleAssignment {
		return operator.FamilyModuleConstant
	}
	return operator.FamilyNumericConstant
}

// skipTransparent returns the depth, counted from the parent, of the first
// ancestor whose type is not one of the transparent types.
func (a ancestry) skipTransparent(p *parsed, transparent ...string) int {
	depth := 1
	for a.up(depth) != nil && slices.Contains(transparent, p.typeOf(a.up(depth))) {
		depth++
	}
	return depth
}

// decimalConstructor is the class whose string argument is a decimal string.
const decimalConstructor = "BigDecimal"
