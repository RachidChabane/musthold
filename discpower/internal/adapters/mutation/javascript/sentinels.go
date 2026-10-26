package javascript

import (
	"fmt"
	"slices"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/operator"
)

const (
	zeroLiteral                 = "0"
	labelSeparator              = "\x00"
	positionFormat              = "%d"
	deadDeclaration             = "const %s = " + zeroLiteral + "; void %s;"
	privateConstantDeclaration  = "const %s = " + zeroLiteral + ";"
	exportedConstantDeclaration = "export " + privateConstantDeclaration
)

// dynamicScopeNames are the identifiers whose use makes a function unsafe for
// sentinels: they observe the variables of a scope or run code built from text.
var dynamicScopeNames = []string{"eval", "Function", "arguments"}

// sentinelCandidates lists every sentinel the catalogue allows for the file.
func (p *parsed) sentinelCandidates(file string, seed operator.Seed) []operator.SentinelCandidate {
	taken := string(p.source)
	fileNames := p.identifierCounts(p.root())
	var all []operator.SentinelCandidate
	for _, u := range p.units() {
		if u.target != mutation.TargetDefinition || p.usesDynamicScope(u) {
			continue
		}
		all = append(all, p.deadAssignments(file, u, taken, seed)...)
		all = append(all, p.renames(file, u, taken, seed, fileNames)...)
	}
	return append(all, p.unusedConstant(file, taken, seed))
}

// usesDynamicScope reports whether the unit mentions eval, Function or
// arguments, or contains a with statement.
func (p *parsed) usesDynamicScope(u unit) bool {
	dynamic := false
	p.walk(u.node, func(n *gotreesitter.Node, _ string, _ ancestry) bool {
		switch p.typeOf(n) {
		case typeWith:
			dynamic = true
		case typeIdentifier:
			dynamic = dynamic || slices.Contains(dynamicScopeNames, p.text(n))
		}
		return !dynamic
	})
	return dynamic
}

// identifierCounts counts the identifier occurrences below n by name.
func (p *parsed) identifierCounts(n *gotreesitter.Node) map[string]int {
	counts := map[string]int{}
	p.walk(n, func(node *gotreesitter.Node, _ string, _ ancestry) bool {
		if p.typeOf(node) == typeIdentifier {
			counts[p.text(node)]++
		}
		return true
	})
	return counts
}

// localName writes an injected local in camel case.
func localName(word, suffix string) string { return word + strings.ToUpper(suffix) }

// constantName writes an injected private module constant in upper snake case.
func constantName(word, suffix string) string {
	return "_" + strings.ToUpper(word) + "_" + strings.ToUpper(suffix)
}

func label(parts ...string) string { return strings.Join(parts, labelSeparator) }

func (p *parsed) sentinelFor(file string, u unit, kind operator.SentinelKind, span ecosystem.Span, newSource string) operator.SentinelCandidate {
	return operator.SentinelCandidate{
		Kind:     kind,
		Site:     operator.Site{File: file, Qualname: u.qualname, Target: u.target, Span: span},
		Replaces: u.span,
		Original: u.text,
		Edit:     mutation.Edit{File: file, Qualname: u.qualname, Target: u.target, NewSource: newSource},
	}
}

// unusedConstant adds a private module constant that nothing references. In a
// module it is exported so that unused-local checks stay quiet.
func (p *parsed) unusedConstant(file, taken string, seed operator.Seed) operator.SentinelCandidate {
	name := operator.NeutralName(seed, label(file, "constant"), taken, constantName)
	end := ecosystem.Span{Start: len(p.source), End: len(p.source)}
	declaration := privateConstantDeclaration
	if p.isModule() {
		declaration = exportedConstantDeclaration
	}
	return operator.SentinelCandidate{
		Kind:     operator.SentinelUnusedConstant,
		Site:     operator.Site{File: file, Qualname: name, Target: mutation.TargetModuleAddition, Span: end},
		Replaces: end,
		Edit:     mutation.Edit{File: file, Qualname: name, Target: mutation.TargetModuleAddition, NewSource: fmt.Sprintf(declaration, name)},
	}
}

// isModule reports whether the file imports or exports something.
func (p *parsed) isModule() bool {
	for i := 0; i < p.root().NamedChildCount(); i++ {
		if nodeType := p.typeOf(p.root().NamedChild(i)); nodeType == typeImportStatement || nodeType == typeExportStatement {
			return true
		}
	}
	return false
}

// deadAssignments offers, before each statement of the function body that
// starts on its own line, the insertion of a fresh local that is declared and
// read by `void`.
func (p *parsed) deadAssignments(file string, u unit, taken string, seed operator.Seed) []operator.SentinelCandidate {
	body := p.child(u.node, fieldBody)
	if body == nil || p.typeOf(body) != typeStatementBlock {
		return nil
	}
	var all []operator.SentinelCandidate
	for _, statement := range p.insertableStatements(body) {
		start := int(statement.StartByte())
		lineStart := strings.LastIndexByte(string(p.source[:start]), '\n') + 1
		indent := string(p.source[lineStart:start])
		if strings.TrimSpace(indent) != "" {
			continue
		}
		name := operator.NeutralName(seed, label(file, u.qualname, fmt.Sprintf(positionFormat, start)), taken, localName)
		offset := start - u.span.Start
		changed := u.text[:offset] + fmt.Sprintf(deadDeclaration, name, name) + "\n" + indent + u.text[offset:]
		all = append(all, p.sentinelFor(file, u, operator.SentinelDeadAssignment, ecosystem.Span{Start: start, End: start}, changed))
	}
	return all
}

// insertableStatements lists the statements of a block a declaration can be
// inserted before: comments, directives and calls of `super` are left out.
func (p *parsed) insertableStatements(body *gotreesitter.Node) []*gotreesitter.Node {
	var found []*gotreesitter.Node
	for i := 0; i < body.NamedChildCount(); i++ {
		statement := body.NamedChild(i)
		if p.typeOf(statement) != typeComment && !p.isDirective(statement) && !p.isSuperCall(statement) {
			found = append(found, statement)
		}
	}
	return found
}

func (p *parsed) isDirective(statement *gotreesitter.Node) bool {
	if p.typeOf(statement) != typeExpressionStatement || statement.NamedChildCount() != 1 {
		return false
	}
	return p.typeOf(statement.NamedChild(0)) == typeString
}

func (p *parsed) isSuperCall(statement *gotreesitter.Node) bool {
	if p.typeOf(statement) != typeExpressionStatement || statement.NamedChildCount() != 1 {
		return false
	}
	call := statement.NamedChild(0)
	if p.typeOf(call) != typeCall {
		return false
	}
	callee := p.child(call, fieldFunction)
	return callee != nil && p.typeOf(callee) == typeSuper
}
