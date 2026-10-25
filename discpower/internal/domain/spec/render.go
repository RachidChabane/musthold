package spec

import (
	"slices"
	"strings"
)

// Tokens of the example grammar, as an example is written in a spec.
const (
	callOpen        = "("
	callClose       = ")"
	argumentSplit   = ", "
	argumentBinding = "="
	outcomeArrow    = " => "
	raisesKeyword   = "raises "
	stringQuote     = `"`
	stringEscape    = `\`
	nullKeyword     = "null"
)

// String writes the literal as the example grammar does: a string quoted with
// its quotes and backslashes escaped, null as its keyword, every other type as
// its text.
func (l Literal) String() string {
	switch l.Type {
	case TypeNull:
		return nullKeyword
	case TypeString:
	default:
		return l.Value
	}
	escaped := strings.NewReplacer(stringEscape, stringEscape+stringEscape, stringQuote, stringEscape+stringQuote).Replace(l.Value)
	return stringQuote + escaped + stringQuote
}

// String writes the call as the example grammar does, its arguments in name
// order.
func (c Call) String() string {
	names := make([]string, 0, len(c.Args))
	for name := range c.Args {
		names = append(names, name)
	}
	slices.Sort(names)
	arguments := make([]string, len(names))
	for index, name := range names {
		arguments[index] = name + argumentBinding + c.Args[name].String()
	}
	return c.Operation + callOpen + strings.Join(arguments, argumentSplit) + callClose
}

// String writes the outcome an example expects: the value it returns, or the
// exception it raises.
func (o Observation) String() string {
	if o.Kind == Raises {
		return raisesKeyword + o.Value
	}
	return Literal{Type: o.Type, Value: o.Value}.String()
}

// String writes the example as the example grammar does: the call, then what it
// must give.
func (e Example) String() string { return e.Call.String() + outcomeArrow + e.Expect.String() }

// Example returns the criterion's example of the index.
func (c Criterion) Example(index int) (Example, bool) {
	for _, example := range c.Examples {
		if example.Index == index {
			return example, true
		}
	}
	return Example{}, false
}
