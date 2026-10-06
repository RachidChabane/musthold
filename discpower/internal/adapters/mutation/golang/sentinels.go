package golang

import (
	"fmt"
	"go/ast"
	"go/token"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/operator"
)

const (
	labelSeparator = "\x00"
	positionFormat = "%d"
)

// introspectionPackages are the packages whose use makes a function unsafe for
// sentinels: they observe the running program or its values by layout.
var introspectionPackages = []string{"reflect", "runtime", "unsafe"}

// sentinelCandidates lists every sentinel the catalogue allows for the file.
func (s *source) sentinelCandidates(file string, seed operator.Seed) []operator.SentinelCandidate {
	taken := string(s.text)
	var all []operator.SentinelCandidate
	for _, u := range s.units() {
		if u.function == nil || usesIntrospection(u.function) {
			continue
		}
		all = append(all, s.deadAssignments(file, u, taken, seed)...)
		all = append(all, s.renames(file, u, taken, seed)...)
	}
	return append(all, s.unusedConstant(file, taken, seed))
}

func usesIntrospection(d *ast.FuncDecl) bool {
	found := false
	ast.Inspect(d, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok && ident.Obj == nil && slices.Contains(introspectionPackages, ident.Name) {
			found = true
		}
		return !found
	})
	return found
}

// camelName writes an injected identifier in lower camel case.
func camelName(word, suffix string) string {
	return word + strings.ToUpper(suffix[:1]) + suffix[1:]
}

func label(parts ...string) string { return strings.Join(parts, labelSeparator) }

func (s *source) sentinelFor(file string, u unit, kind operator.SentinelKind, span ecosystem.Span, newSource string) operator.SentinelCandidate {
	return operator.SentinelCandidate{
		Kind:     kind,
		Site:     operator.Site{File: file, Qualname: u.qualname, Target: u.target, Span: span},
		Replaces: u.span,
		Original: u.text,
		Edit:     mutation.Edit{File: file, Qualname: u.qualname, Target: u.target, NewSource: newSource},
	}
}

// unusedConstant adds a private package-level constant that nothing references.
func (s *source) unusedConstant(file, taken string, seed operator.Seed) operator.SentinelCandidate {
	name := operator.NeutralName(seed, label(file, "constant"), taken, camelName)
	qualname := s.file.Name.Name + qualnameSeparator + name
	end := ecosystem.Span{Start: len(s.text), End: len(s.text)}
	return operator.SentinelCandidate{
		Kind:     operator.SentinelUnusedConstant,
		Site:     operator.Site{File: file, Qualname: qualname, Target: mutation.TargetModuleAddition, Span: end},
		Replaces: end,
		Edit:     mutation.Edit{File: file, Qualname: qualname, Target: mutation.TargetModuleAddition, NewSource: "const " + name + " = 0"},
	}
}

// deadAssignments offers, before each statement of the body that starts on its
// own line, the insertion of `fresh := 0` and `_ = fresh`. A function with a goto
// or a label only offers the first position, since a jump over a declaration
// does not compile.
func (s *source) deadAssignments(file string, u unit, taken string, seed operator.Seed) []operator.SentinelCandidate {
	statements := u.function.Body.List
	if len(statements) > 0 && hasJump(u.function) {
		statements = statements[:1]
	}
	var all []operator.SentinelCandidate
	for _, statement := range statements {
		start := s.offset(statement.Pos())
		lineStart := strings.LastIndexByte(string(s.text[:start]), '\n') + 1
		indent := string(s.text[lineStart:start])
		if strings.TrimSpace(indent) != "" {
			continue
		}
		name := operator.NeutralName(seed, label(file, u.qualname, fmt.Sprintf(positionFormat, start)), taken, camelName)
		offset := start - u.span.Start
		inserted := name + " := 0\n" + indent + "_ = " + name + "\n" + indent
		changed := u.text[:offset] + inserted + u.text[offset:]
		all = append(all, s.sentinelFor(file, u, operator.SentinelDeadAssignment, ecosystem.Span{Start: start, End: start}, changed))
	}
	return all
}

func hasJump(d *ast.FuncDecl) bool {
	found := false
	ast.Inspect(d, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.LabeledStmt:
			found = true
		case *ast.BranchStmt:
			found = found || stmt.Tok == token.GOTO
		}
		return !found
	})
	return found
}

// renames renames a local variable throughout a function, following the
// identifier resolution of the parser.
func (s *source) renames(file string, u unit, taken string, seed operator.Seed) []operator.SentinelCandidate {
	keys := compositeKeys(u.function)
	var all []operator.SentinelCandidate
	for _, local := range s.locals(u.function) {
		if keys[local.Name] {
			continue
		}
		replacement := operator.NeutralName(seed, label(file, u.qualname, local.Name), taken, camelName)
		var spans []ecosystem.Span
		ast.Inspect(u.function, func(n ast.Node) bool {
			if ident, ok := n.(*ast.Ident); ok && ident.Obj == local.Obj {
				spans = append(spans, s.spanOf(ident))
			}
			return true
		})
		changed := u.text
		for i := len(spans) - 1; i >= 0; i-- {
			changed = replaceIn(changed, u.span.Start, spans[i], replacement)
		}
		all = append(all, s.sentinelFor(file, u, operator.SentinelAlphaRename, spans[0], changed))
	}
	return all
}

// compositeKeys lists the identifiers used as keys of composite literals, which
// may be struct fields rather than variables.
func compositeKeys(d *ast.FuncDecl) map[string]bool {
	keys := map[string]bool{}
	ast.Inspect(d, func(n ast.Node) bool {
		if literal, ok := n.(*ast.CompositeLit); ok {
			for _, element := range literal.Elts {
				if pair, ok := element.(*ast.KeyValueExpr); ok {
					if key, ok := pair.Key.(*ast.Ident); ok {
						keys[key.Name] = true
					}
				}
			}
		}
		return true
	})
	return keys
}

// locals lists the variables a function declares with `:=` or `var` directly in
// a statement list, in source order. Parameters and results are not included.
func (s *source) locals(d *ast.FuncDecl) []*ast.Ident {
	var found []*ast.Ident
	walk(d.Body, func(n ast.Node, up []ast.Node) bool {
		if !isStatementList(nearest(up, 1)) {
			return true
		}
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			if stmt.Tok == token.DEFINE {
				for _, lhs := range stmt.Lhs {
					found = appendLocal(found, lhs, stmt)
				}
			}
		case *ast.DeclStmt:
			if gen, ok := stmt.Decl.(*ast.GenDecl); ok && gen.Tok == token.VAR {
				for _, spec := range gen.Specs {
					for _, name := range spec.(*ast.ValueSpec).Names {
						found = appendLocal(found, name, spec)
					}
				}
			}
		}
		return true
	})
	return found
}

func isStatementList(parent ast.Node) bool {
	switch parent.(type) {
	case *ast.BlockStmt, *ast.CaseClause, *ast.CommClause:
		return true
	}
	return false
}

// appendLocal adds an identifier that declares an object of its own: a name
// that the statement redeclares keeps the object of the first declaration and is
// added only at that first declaration.
func appendLocal(found []*ast.Ident, expr ast.Expr, declaration ast.Node) []*ast.Ident {
	ident, ok := expr.(*ast.Ident)
	if !ok || ident.Name == "_" || ident.Obj == nil || ident.Obj.Decl != declaration {
		return found
	}
	return append(found, ident)
}
