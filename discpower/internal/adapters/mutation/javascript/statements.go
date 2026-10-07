package javascript

import (
	"slices"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/operator"
)

const (
	undefinedLiteral  = "undefined"
	emptyStatement    = ";"
	exceptionDefault  = "Error"
	exceptionSuffix   = "Error"
	exceptionSuffixEx = "Exception"
)

// branchTypes are the nodes whose body is a branch of a conditional.
var branchTypes = []string{typeIf, typeElse}

// exceptionSwaps maps a built-in error class to the class it is swapped for.
var exceptionSwaps = map[string]string{
	"Error":          "TypeError",
	"TypeError":      "RangeError",
	"RangeError":     "TypeError",
	"ReferenceError": "Error",
	"SyntaxError":    "Error",
	"EvalError":      "Error",
	"URIError":       "Error",
	"AggregateError": "Error",
}

// returnMutator deletes a return that is a branch of an if or closes one (the
// statement becomes an empty statement) and replaces a returned value by
// undefined.
func returnMutator(c *collector, node *gotreesitter.Node, _ string, up ancestry) {
	if c.closesBranch(up) {
		c.propose(c.p.span(node), emptyStatement, operator.FamilyReturnValue)
	}
	if value := c.p.firstNamed(node); value != nil && c.p.text(value) != undefinedLiteral {
		c.propose(c.p.span(value), undefinedLiteral, operator.FamilyReturnValue)
	}
}

// closesBranch reports whether a statement is the body of an if or else branch,
// directly or as the last statement of a block that is one.
func (c *collector) closesBranch(up ancestry) bool {
	parent := up.parent()
	if parent == nil {
		return false
	}
	if c.p.typeOf(parent) == typeStatementBlock {
		parent = up.up(2)
	}
	return parent != nil && slices.Contains(branchTypes, c.p.typeOf(parent))
}

// throwMutator swaps the class of `throw new X(...)` and `throw X(...)`.
func throwMutator(c *collector, node *gotreesitter.Node, _ string, _ ancestry) {
	thrown := c.p.firstNamed(node)
	if thrown == nil {
		return
	}
	switch c.p.typeOf(thrown) {
	case typeNew:
		c.swapException(c.p.child(thrown, fieldConstructor))
	case typeCall:
		c.swapException(c.p.child(thrown, fieldFunction))
	}
}

// instanceOfMutator swaps the class on the right of `instanceof`.
func instanceOfMutator(c *collector, node *gotreesitter.Node, _ string, _ ancestry) {
	if _, current := c.operatorOf(node); current == instanceOfOperator {
		c.swapException(c.p.child(node, fieldRight))
	}
}

func (c *collector) swapException(class *gotreesitter.Node) {
	if class == nil || c.p.typeOf(class) != typeIdentifier {
		return
	}
	if swapped := exceptionSwap(c.p.text(class)); swapped != "" {
		c.propose(c.p.span(class), swapped, operator.FamilyExceptionClass)
	}
}

// exceptionSwap names the class a thrown or tested class is swapped for; empty
// when the name does not look like an error class.
func exceptionSwap(name string) string {
	if swapped, ok := exceptionSwaps[name]; ok {
		return swapped
	}
	if strings.HasSuffix(name, exceptionSuffix) || strings.HasSuffix(name, exceptionSuffixEx) {
		return exceptionDefault
	}
	return ""
}
