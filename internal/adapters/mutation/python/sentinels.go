package python

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/spec-proof/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/internal/domain/mutation/operator"
)

const (
	zeroLiteral    = "0"
	labelSeparator = "\x00"
	positionFormat = "%d"
)

// introspectionNames are the names whose use makes a function unsafe for
// sentinels: they observe local variables or run code built from text.
var introspectionNames = []string{"locals", "vars", "globals", "eval", "exec", "inspect"}

// sentinelCandidates lists every sentinel the catalogue allows for the file.
func (p *parsed) sentinelCandidates(file string, seed operator.Seed) []operator.SentinelCandidate {
	taken := string(p.source)
	var all []operator.SentinelCandidate
	for _, u := range p.units() {
		if u.target != mutation.TargetDefinition || p.usesIntrospection(u) {
			continue
		}
		all = append(all, p.deadAssignments(file, u, taken, seed)...)
		all = append(all, p.renames(file, u, taken, seed)...)
	}
	return append(all, p.unusedConstant(file, taken, seed))
}

func (p *parsed) usesIntrospection(u unit) bool {
	for name := range p.identifiers(u.node) {
		if slices.Contains(introspectionNames, name) {
			return true
		}
	}
	return false
}

// localName writes an injected local in snake case.
func localName(word, suffix string) string { return word + "_" + suffix }

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

// unusedConstant adds a private module constant that nothing references.
func (p *parsed) unusedConstant(file, taken string, seed operator.Seed) operator.SentinelCandidate {
	name := operator.NeutralName(seed, label(file, "constant"), taken, constantName)
	end := ecosystem.Span{Start: len(p.source), End: len(p.source)}
	return operator.SentinelCandidate{
		Kind:     operator.SentinelUnusedConstant,
		Site:     operator.Site{File: file, Qualname: name, Target: mutation.TargetModuleAddition, Span: end},
		Replaces: end,
		Edit:     mutation.Edit{File: file, Qualname: name, Target: mutation.TargetModuleAddition, NewSource: name + " = " + zeroLiteral},
	}
}

// deadAssignments offers, before each statement of the function body that
// starts on its own line (after a docstring), the insertion of `fresh = 0`.
func (p *parsed) deadAssignments(file string, u unit, taken string, seed operator.Seed) []operator.SentinelCandidate {
	body := p.child(u.node, fieldBody)
	if body == nil || p.typeOf(body) != typeBlock {
		return nil
	}
	var all []operator.SentinelCandidate
	for _, statement := range p.statements(body) {
		start := int(statement.StartByte())
		lineStart := strings.LastIndexByte(string(p.source[:start]), '\n') + 1
		indent := string(p.source[lineStart:start])
		if strings.TrimSpace(indent) != "" {
			continue
		}
		name := operator.NeutralName(seed, label(file, u.qualname, fmt.Sprintf(positionFormat, start)), taken, localName)
		offset := start - u.span.Start
		changed := u.text[:offset] + name + " = " + zeroLiteral + "\n" + indent + u.text[offset:]
		all = append(all, p.sentinelFor(file, u, operator.SentinelDeadAssignment, ecosystem.Span{Start: start, End: start}, changed))
	}
	return all
}

// statements lists the statements of a block after its docstring, comments
// left out.
func (p *parsed) statements(body *gotreesitter.Node) []*gotreesitter.Node {
	var found []*gotreesitter.Node
	skippedDocstring := false
	for i := 0; i < body.NamedChildCount(); i++ {
		statement := body.NamedChild(i)
		switch {
		case p.typeOf(statement) == typeComment:
		case !skippedDocstring && p.isDocstring(statement):
			skippedDocstring = true
		default:
			found = append(found, statement)
		}
	}
	return found
}

// renames renames a local variable throughout a function.
func (p *parsed) renames(file string, u unit, taken string, seed operator.Seed) []operator.SentinelCandidate {
	scan := p.scanLocals(u)
	if scan.declaresScope {
		return nil
	}
	var all []operator.SentinelCandidate
	for _, name := range scan.assigned {
		if scan.forbidden[name] {
			continue
		}
		replacement := operator.NeutralName(seed, label(file, u.qualname, name), taken, localName)
		var spans []ecosystem.Span
		p.walk(u.node, func(n *gotreesitter.Node, field string, up ancestry) bool {
			if p.typeOf(n) == typeIdentifier && p.text(n) == name && p.isReference(field, up) {
				spans = append(spans, p.span(n))
			}
			return true
		})
		changed := u.text
		for i := len(spans) - 1; i >= 0; i-- {
			changed = replaceIn(changed, u.span.Start, spans[i], replacement)
		}
		all = append(all, p.sentinelFor(file, u, operator.SentinelAlphaRename, spans[0], changed))
	}
	return all
}

// localScan is what the renaming of locals needs to know about a function.
type localScan struct {
	declaresScope bool
	assigned      []string
	forbidden     map[string]bool
}

// scanLocals finds the names assigned directly in the function and the names
// that cannot be renamed: parameters, names mentioned by a nested scope, and
// comprehension targets.
func (p *parsed) scanLocals(u unit) localScan {
	scan := localScan{forbidden: map[string]bool{}}
	if params := p.child(u.node, fieldParams); params != nil {
		for name := range p.identifiers(params) {
			scan.forbidden[name] = true
		}
	}
	p.walk(u.node, func(n *gotreesitter.Node, field string, up ancestry) bool {
		switch p.typeOf(n) {
		case typeGlobal, typeNonlocal:
			scan.declaresScope = true
		case typeFunction, typeLambda, typeClass:
			if p.span(n) != u.span {
				for name := range p.identifiers(n) {
					scan.forbidden[name] = true
				}
				return false
			}
		case typeForInClause:
			if left := p.child(n, fieldLeft); left != nil {
				for name := range p.identifiers(left) {
					scan.forbidden[name] = true
				}
			}
		case typeAssignment, typeFor:
			if left := p.child(n, fieldLeft); left != nil && p.typeOf(left) == typeIdentifier {
				if name := p.text(left); !slices.Contains(scan.assigned, name) {
					scan.assigned = append(scan.assigned, name)
				}
			}
		}
		return true
	})
	sort.Strings(scan.assigned)
	return scan
}
