package jvm

import (
	"fmt"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/operator"
)

const (
	labelSeparator = "\x00"
	positionFormat = "%d"
	constantLabel  = "constant"
	constantJoint  = "_"
)

// sentinelCandidates lists every sentinel the catalogue allows for the file.
func (p *parsed) sentinelCandidates(file string, seed operator.Seed) []operator.SentinelCandidate {
	taken := string(p.source)
	var all []operator.SentinelCandidate
	for _, u := range p.units() {
		if u.target != mutation.TargetDefinition || p.usesReflection(u) {
			continue
		}
		all = append(all, p.deadAssignments(file, u, taken, seed)...)
		all = append(all, p.renames(file, u, taken, seed)...)
	}
	if constant, ok := p.unusedConstant(file, taken, seed); ok {
		all = append(all, constant)
	}
	return all
}

// localName writes an injected local in camel case.
func localName(word, suffix string) string {
	return word + strings.ToUpper(suffix[:1]) + suffix[1:]
}

// constantName writes an injected constant in upper snake case.
func constantName(word, suffix string) string {
	return strings.ToUpper(word) + constantJoint + strings.ToUpper(suffix)
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

// unusedConstant adds a private constant that nothing references, when the
// dialect has a place for one at the end of a file.
func (p *parsed) unusedConstant(file, taken string, seed operator.Seed) (operator.SentinelCandidate, bool) {
	name := operator.NeutralName(seed, label(file, constantLabel), taken, constantName)
	declaration, ok := p.dialect.unusedConstant(name)
	if !ok {
		return operator.SentinelCandidate{}, false
	}
	end := ecosystem.Span{Start: len(p.source), End: len(p.source)}
	return operator.SentinelCandidate{
		Kind:     operator.SentinelUnusedConstant,
		Site:     operator.Site{File: file, Qualname: name, Target: mutation.TargetModuleAddition, Span: end},
		Replaces: end,
		Edit:     mutation.Edit{File: file, Qualname: name, Target: mutation.TargetModuleAddition, NewSource: declaration},
	}, true
}

// deadAssignments offers, before each statement of the body that starts on its
// own line, the insertion of a fresh local that nothing reads.
func (p *parsed) deadAssignments(file string, u unit, taken string, seed operator.Seed) []operator.SentinelCandidate {
	var all []operator.SentinelCandidate
	for _, statement := range p.dialect.bodyStatements(p, u.node) {
		start := int(statement.StartByte())
		lineStart := strings.LastIndexByte(string(p.source[:start]), '\n') + 1
		indent := string(p.source[lineStart:start])
		if strings.TrimSpace(indent) != "" {
			continue
		}
		name := operator.NeutralName(seed, label(file, u.qualname, fmt.Sprintf(positionFormat, start)), taken, localName)
		offset := start - u.span.Start
		changed := u.text[:offset] + p.dialect.deadLocal(name) + "\n" + indent + u.text[offset:]
		all = append(all, p.sentinelFor(file, u, operator.SentinelDeadAssignment, ecosystem.Span{Start: start, End: start}, changed))
	}
	return all
}

// renames renames each local variable of the unit that can be renamed safely.
func (p *parsed) renames(file string, u unit, taken string, seed operator.Seed) []operator.SentinelCandidate {
	var all []operator.SentinelCandidate
	seen := map[string]bool{}
	for _, declaration := range p.localDeclarations(u) {
		name := p.text(declaration.name)
		if seen[name] || p.dialect.isRenameBlocked(name) {
			continue
		}
		seen[name] = true
		spans, ok := p.renameSpans(u, declaration)
		if !ok {
			continue
		}
		replacement := operator.NeutralName(seed, label(file, u.qualname, name), taken, localName)
		changed := u.text
		for i := len(spans) - 1; i >= 0; i-- {
			changed = replaceIn(changed, u.span.Start, spans[i], replacement)
		}
		all = append(all, p.sentinelFor(file, u, operator.SentinelAlphaRename, spans[0], changed))
	}
	return all
}

// localDeclarations lists the local variable declarations of a unit in source
// order.
func (p *parsed) localDeclarations(u unit) []localDeclaration {
	var found []localDeclaration
	p.walk(u.node, func(node *gotreesitter.Node, _ string, up ancestry) bool {
		if declaration, ok := p.dialect.localDeclaration(p, node, up); ok {
			found = append(found, declaration)
		}
		return true
	})
	return found
}

// renameSpans returns the spans of every occurrence of a local's name when the
// rename is safe: the variable is declared once in the unit, every occurrence
// is a variable reference at or after the declaration and inside its scope.
func (p *parsed) renameSpans(u unit, declaration localDeclaration) ([]ecosystem.Span, bool) {
	name := p.text(declaration.name)
	scopeStart, scopeEnd := declaration.scope.StartByte(), declaration.scope.EndByte()
	var spans []ecosystem.Span
	declarations, safe := 0, true
	p.walk(u.node, func(node *gotreesitter.Node, field string, up ancestry) bool {
		if !p.dialect.isIdentifier(p.typeOf(node)) || p.text(node) != name {
			return true
		}
		spans = append(spans, p.span(node))
		switch p.dialect.identifierRole(p, node, field, up) {
		case roleDeclaration:
			declarations++
			safe = safe && node.StartByte() == declaration.name.StartByte()
		case roleOther:
			safe = false
		default:
			safe = safe && node.StartByte() >= declaration.name.StartByte() && node.StartByte() >= scopeStart && node.EndByte() <= scopeEnd
		}
		return true
	})
	return spans, safe && declarations == 1
}
