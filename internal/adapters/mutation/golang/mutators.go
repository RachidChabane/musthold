package golang

import (
	"go/ast"
	"go/token"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/RachidChabane/spec-proof/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/internal/domain/mutation/operator"
)

// Operator vocabularies.
var (
	relationalOperators = []token.Token{token.LSS, token.LEQ, token.GTR, token.GEQ, token.EQL, token.NEQ}
	equalityOperators   = []token.Token{token.EQL, token.NEQ}
	arithmeticOperators = []token.Token{token.ADD, token.SUB, token.MUL, token.QUO}
	logicalOperators    = []token.Token{token.LAND, token.LOR}
	minMaxFunctions     = []string{"min", "max"}
	decimalParsers      = []string{"NewFromString", "RequireFromString"}
	boolFlips           = map[string]string{"true": "false", "false": "true"}
	zeroValues          = map[string]string{
		"int": "0", "int8": "0", "int16": "0", "int32": "0", "int64": "0",
		"uint": "0", "uint8": "0", "uint16": "0", "uint32": "0", "uint64": "0",
		"byte": "0", "rune": "0", "float32": "0", "float64": "0",
		"string": `""`, "bool": "false",
	}
)

const (
	negationPrefix  = "!("
	negationSuffix  = ")"
	errorNamePrefix = "err"
	minErrorNameLen = 4
)

// collector gathers the candidates of one unit.
type collector struct {
	s         *source
	file      string
	unit      unit
	sentinels []string
	out       []operator.Candidate
}

// handler proposes mutations for one visited node.
type handler func(c *collector, node ast.Node, up []ast.Node)

// handlers maps a node type, by a kind key, to its mutators. Adding a mutator is
// adding an entry here.
var handlers = []struct {
	applies func(ast.Node) bool
	mutate  handler
}{
	{isBinary, binaryMutator},
	{isBinary, operandNegationMutator},
	{isBasicNumber, numberMutator},
	{isIdent, booleanLiteralMutator},
	{isIdent, errorSentinelMutator},
	{isIf, conditionNegationMutator},
	{isFor, conditionNegationMutator},
	{isReturn, returnMutator},
	{isCall, callMutator},
}

func isBinary(n ast.Node) bool { _, ok := n.(*ast.BinaryExpr); return ok }
func isIdent(n ast.Node) bool  { _, ok := n.(*ast.Ident); return ok }
func isIf(n ast.Node) bool     { _, ok := n.(*ast.IfStmt); return ok }
func isFor(n ast.Node) bool    { _, ok := n.(*ast.ForStmt); return ok }
func isReturn(n ast.Node) bool { _, ok := n.(*ast.ReturnStmt); return ok }
func isCall(n ast.Node) bool   { _, ok := n.(*ast.CallExpr); return ok }
func isBasicNumber(n ast.Node) bool {
	lit, ok := n.(*ast.BasicLit)
	return ok && (lit.Kind == token.INT || lit.Kind == token.FLOAT)
}

// candidates enumerates the operator mutants of every unit of the file.
func (s *source) candidates(file string) []operator.Candidate {
	sentinels := s.errorSentinels()
	var all []operator.Candidate
	for _, u := range s.units() {
		c := &collector{s: s, file: file, unit: u, sentinels: sentinels}
		walk(u.node, func(n ast.Node, up []ast.Node) bool {
			for _, h := range handlers {
				if h.applies(n) {
					h.mutate(c, n, up)
				}
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

func (c *collector) operatorSpan(e *ast.BinaryExpr) ecosystem.Span {
	start := c.s.offset(e.OpPos)
	return ecosystem.Span{Start: start, End: start + len(e.Op.String())}
}

func tokenTexts(tokens []token.Token, except token.Token) []string {
	var texts []string
	for _, t := range tokens {
		if t != except {
			texts = append(texts, t.String())
		}
	}
	return texts
}

func binaryMutator(c *collector, node ast.Node, _ []ast.Node) {
	e := node.(*ast.BinaryExpr)
	span := c.operatorSpan(e)
	switch {
	case slices.Contains(relationalOperators, e.Op):
		vocabulary := relationalOperators
		if unordered(e.X) || unordered(e.Y) {
			vocabulary = equalityOperators
		}
		c.proposeAll(span, tokenTexts(vocabulary, e.Op), operator.FamilyRelational)
	case slices.Contains(arithmeticOperators, e.Op):
		if isStringLiteral(e.X) || isStringLiteral(e.Y) {
			return
		}
		c.proposeAll(span, tokenTexts(arithmeticOperators, e.Op), operator.FamilyArithmetic)
	case slices.Contains(logicalOperators, e.Op):
		c.proposeAll(span, tokenTexts(logicalOperators, e.Op), operator.FamilyAndOr)
	}
}

// unordered tells an operand that only supports equality: nil and booleans.
func unordered(e ast.Expr) bool {
	ident, ok := e.(*ast.Ident)
	return ok && (ident.Name == "nil" || ident.Name == "true" || ident.Name == "false")
}

func isStringLiteral(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	return ok && lit.Kind == token.STRING
}

// operandNegationMutator negates each operand of && and || that is not itself a
// logical expression.
func operandNegationMutator(c *collector, node ast.Node, _ []ast.Node) {
	e := node.(*ast.BinaryExpr)
	if !slices.Contains(logicalOperators, e.Op) {
		return
	}
	for _, operand := range []ast.Expr{e.X, e.Y} {
		if inner, ok := operand.(*ast.BinaryExpr); ok && slices.Contains(logicalOperators, inner.Op) {
			continue
		}
		c.negate(operand)
	}
}

func (c *collector) negate(operand ast.Expr) {
	c.propose(c.s.spanOf(operand), negationPrefix+c.s.textOf(operand)+negationSuffix, operator.FamilyBooleanLogical)
}

func conditionNegationMutator(c *collector, node ast.Node, _ []ast.Node) {
	switch n := node.(type) {
	case *ast.IfStmt:
		c.negate(n.Cond)
	case *ast.ForStmt:
		if n.Cond != nil {
			c.negate(n.Cond)
		}
	}
}

func booleanLiteralMutator(c *collector, node ast.Node, _ []ast.Node) {
	ident := node.(*ast.Ident)
	if flipped, ok := boolFlips[ident.Name]; ok && ident.Obj == nil {
		c.propose(c.s.spanOf(ident), flipped, operator.FamilyBooleanLiteral)
	}
}

func numberMutator(c *collector, node ast.Node, up []ast.Node) {
	lit := node.(*ast.BasicLit)
	if _, isLength := nearest(up, 1).(*ast.ArrayType); isLength {
		return
	}
	c.proposeAll(c.s.spanOf(lit), operator.ShiftNumber(lit.Value), c.numberFamily(node, up))
}

// numberFamily classifies a numeric literal: an argument of a constructor call
// or a composite literal field, a module-level constant, or a plain constant.
func (c *collector) numberFamily(node ast.Node, up []ast.Node) operator.Family {
	if c.isConstructorArgument(node, up) {
		return operator.FamilyConstructorArgument
	}
	if c.unit.target == mutation.TargetModuleAssignment {
		return operator.FamilyModuleConstant
	}
	return operator.FamilyNumericConstant
}

func (c *collector) isConstructorArgument(node ast.Node, up []ast.Node) bool {
	switch parent := nearest(up, 1).(type) {
	case *ast.CallExpr:
		return parent.Fun != node && isExported(calleeName(parent))
	case *ast.CompositeLit:
		return true
	case *ast.KeyValueExpr:
		_, inLiteral := nearest(up, 2).(*ast.CompositeLit)
		return inLiteral && parent.Value == node
	}
	return false
}

// calleeName is the last name of a call's function.
func calleeName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		return fun.Sel.Name
	}
	return ""
}

func callMutator(c *collector, node ast.Node, _ []ast.Node) {
	call := node.(*ast.CallExpr)
	if ident, ok := call.Fun.(*ast.Ident); ok && ident.Obj == nil {
		for _, name := range minMaxFunctions {
			if ident.Name == name {
				c.proposeAll(c.s.spanOf(ident), operator.Alternatives(name, minMaxFunctions), operator.FamilyMinMax)
			}
		}
	}
	c.decimalMutator(call)
}

// decimalMutator shifts the string of a decimal parsing call such as
// `decimal.RequireFromString("2.50")`.
func (c *collector) decimalMutator(call *ast.CallExpr) {
	if len(call.Args) != 1 || !slices.Contains(decimalParsers, calleeName(call)) {
		return
	}
	literal, ok := call.Args[0].(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return
	}
	text, err := strconv.Unquote(literal.Value)
	if err != nil {
		return
	}
	for _, shifted := range operator.ShiftDecimalString(text) {
		c.propose(c.s.spanOf(literal), strconv.Quote(shifted), operator.FamilyDecimalString)
	}
}

// returnMutator replaces a returned value of a basic type by its zero value.
func returnMutator(c *collector, node ast.Node, up []ast.Node) {
	ret := node.(*ast.ReturnStmt)
	types := resultTypes(enclosingSignature(up))
	if len(ret.Results) != len(types) {
		return
	}
	for i, result := range ret.Results {
		zero, ok := zeroValues[types[i]]
		if ok && c.s.textOf(result) != zero {
			c.propose(c.s.spanOf(result), zero, operator.FamilyReturnValue)
		}
	}
}

func enclosingSignature(up []ast.Node) *ast.FuncType {
	for i := len(up) - 1; i >= 0; i-- {
		switch f := up[i].(type) {
		case *ast.FuncLit:
			return f.Type
		case *ast.FuncDecl:
			return f.Type
		}
	}
	return nil
}

// resultTypes lists the type name of each result, empty for a type that is not
// a plain identifier.
func resultTypes(signature *ast.FuncType) []string {
	if signature == nil || signature.Results == nil {
		return nil
	}
	var types []string
	for _, field := range signature.Results.List {
		name := ""
		if ident, ok := field.Type.(*ast.Ident); ok {
			name = ident.Name
		}
		for range max(1, len(field.Names)) {
			types = append(types, name)
		}
	}
	return types
}

// errorSentinels lists the package-level variables of the file that are named
// like error values (`ErrNotFound`, `errNegativeStay`), in source order.
func (s *source) errorSentinels() []string {
	var names []string
	for _, decl := range s.file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			if value, ok := spec.(*ast.ValueSpec); ok {
				for _, name := range value.Names {
					if isErrorName(name.Name) {
						names = append(names, name.Name)
					}
				}
			}
		}
	}
	return names
}

func isErrorName(name string) bool {
	if len(name) < minErrorNameLen || !strings.HasPrefix(strings.ToLower(name), errorNamePrefix) {
		return false
	}
	next, _ := utf8.DecodeRuneInString(name[len(errorNamePrefix):])
	return unicode.IsUpper(next)
}

// errorSentinelMutator swaps a reference to an error sentinel of the file for
// another sentinel of the file; Go has no exception classes.
func errorSentinelMutator(c *collector, node ast.Node, _ []ast.Node) {
	ident := node.(*ast.Ident)
	if c.unit.target != mutation.TargetDefinition || ident.Obj == nil || ident.Obj.Kind != ast.Var || !isErrorName(ident.Name) {
		return
	}
	if _, packageLevel := ident.Obj.Decl.(*ast.ValueSpec); !packageLevel || !slices.Contains(c.sentinels, ident.Name) {
		return
	}
	c.proposeAll(c.s.spanOf(ident), operator.Alternatives(ident.Name, c.sentinels), operator.FamilyExceptionClass)
}
