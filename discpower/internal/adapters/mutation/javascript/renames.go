package javascript

import (
	"slices"
	"sort"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/operator"
)

// Declaration kinds of a local.
const (
	kindConst = "const"
	kindLet   = "let"
	kindVar   = "var"
)

// renamableKinds are the declaration kinds whose locals can be renamed.
var renamableKinds = []string{kindConst, kindLet, kindVar}

// scopeTypes are the nodes that open a scope of their own inside a function.
var scopeTypes = []string{
	typeFunctionDeclaration, typeGeneratorDeclaration, typeArrowFunction, typeFunctionExpression,
	typeGeneratorFunction, typeClassExpression, typeClassDeclaration, typeAbstractClass, typeMethod,
}

// renames renames a local variable throughout a function.
func (p *parsed) renames(file string, u unit, taken string, seed operator.Seed, fileNames map[string]int) []operator.SentinelCandidate {
	scan := p.scanLocals(u)
	unitNames := p.identifierCounts(u.node)
	var all []operator.SentinelCandidate
	for _, name := range scan.declared() {
		if scan.forbidden[name] || fileNames[name] != unitNames[name] {
			continue
		}
		replacement := operator.NeutralName(seed, label(file, u.qualname, name), taken, localName)
		spans := p.occurrences(u, name)
		changed := u.text
		for i := len(spans) - 1; i >= 0; i-- {
			changed = replaceIn(changed, u.span.Start, spans[i], replacement)
		}
		all = append(all, p.sentinelFor(file, u, operator.SentinelAlphaRename, spans[0], changed))
	}
	return all
}

// occurrences lists the spans of the identifiers of a name in a unit.
func (p *parsed) occurrences(u unit, name string) []ecosystem.Span {
	var spans []ecosystem.Span
	p.walk(u.node, func(n *gotreesitter.Node, _ string, _ ancestry) bool {
		if p.typeOf(n) == typeIdentifier && p.text(n) == name {
			spans = append(spans, p.span(n))
		}
		return true
	})
	return spans
}

// localScan is what the renaming of locals needs to know about a function.
type localScan struct {
	kinds     map[string][]string
	forbidden map[string]bool
}

// declared lists the locals declared by a plain identifier, sorted.
func (s localScan) declared() []string {
	names := make([]string, 0, len(s.kinds))
	for name := range s.kinds {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (s localScan) forbid(names map[string]bool) {
	for name := range names {
		s.forbidden[name] = true
	}
}

func (s localScan) declare(name, kind string) {
	if !slices.Contains(s.kinds[name], kind) {
		s.kinds[name] = append(s.kinds[name], kind)
	}
	if len(s.kinds[name]) > 1 || !slices.Contains(renamableKinds, kind) {
		s.forbidden[name] = true
	}
}

// scanLocals finds the names declared by a plain identifier directly in the
// function and the names that cannot be renamed: the function's own name,
// parameters, names bound by patterns, catch clauses and for-in loops, names
// written as shorthand properties, names mentioned by a nested scope, and names
// declared with several kinds.
func (p *parsed) scanLocals(u unit) localScan {
	scan := localScan{kinds: map[string][]string{}, forbidden: map[string]bool{u.name: true}}
	if params := p.child(u.node, fieldParameters); params != nil {
		scan.forbid(p.mentionedNames(params))
	}
	p.walk(u.node, func(n *gotreesitter.Node, _ string, up ancestry) bool {
		switch nodeType := p.typeOf(n); {
		case slices.Contains(scopeTypes, nodeType) && p.span(n) != u.span:
			scan.forbid(p.mentionedNames(n))
			return false
		case nodeType == typeShorthand || nodeType == typeShorthandPattern:
			scan.forbidden[p.text(n)] = true
		case nodeType == typeCatch:
			p.forbidField(scan, n, fieldParameter)
		case nodeType == typeForIn:
			p.forbidField(scan, n, fieldLeft)
		case nodeType == typeDeclarator:
			p.scanDeclarator(scan, n, up.parent())
		}
		return true
	})
	return scan
}

func (p *parsed) forbidField(scan localScan, n *gotreesitter.Node, field string) {
	if bound := p.child(n, field); bound != nil {
		scan.forbid(p.mentionedNames(bound))
	}
}

func (p *parsed) scanDeclarator(scan localScan, declarator, declaration *gotreesitter.Node) {
	name := p.child(declarator, fieldName)
	if name == nil {
		return
	}
	if p.typeOf(name) != typeIdentifier {
		scan.forbid(p.mentionedNames(name))
		return
	}
	scan.declare(p.text(name), p.declarationKind(declaration))
}

// declarationKind is the keyword of a lexical or variable declaration.
func (p *parsed) declarationKind(declaration *gotreesitter.Node) string {
	if declaration == nil {
		return ""
	}
	if p.typeOf(declaration) == typeVariableDeclaration {
		return kindVar
	}
	if kind := p.child(declaration, fieldKind); kind != nil {
		return p.text(kind)
	}
	return ""
}

// mentionedNames collects the identifiers and shorthand property names below n.
func (p *parsed) mentionedNames(n *gotreesitter.Node) map[string]bool {
	found := map[string]bool{}
	p.walk(n, func(node *gotreesitter.Node, _ string, _ ancestry) bool {
		switch p.typeOf(node) {
		case typeIdentifier, typeShorthand, typeShorthandPattern:
			found[p.text(node)] = true
		}
		return true
	})
	return found
}
