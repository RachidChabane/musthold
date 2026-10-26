package python

import (
	"slices"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/footprint"
)

const (
	constructorMethod = "__init__"
	receiverSelf      = "self"
	receiverClass     = "cls"
)

// callGraph builds the graph contribution of the file: one node per unit that
// is a definition, one assignment name per module-level assignment.
func (p *parsed) callGraph(file string) footprint.FileGraph {
	graph := footprint.FileGraph{File: file}
	for _, u := range p.units() {
		if u.target == mutation.TargetModuleAssignment {
			graph.Assignments = append(graph.Assignments, footprint.Assignment{Name: u.name, Qualname: u.qualname})
			continue
		}
		graph.Nodes = append(graph.Nodes, p.nodeOf(u))
	}
	return graph
}

func (p *parsed) nodeOf(u unit) footprint.Node {
	node := footprint.Node{Qualname: u.qualname, Name: u.name, Owner: u.owner}
	if u.name == constructorMethod && u.owner != "" {
		node.Aliases = []string{u.owner[strings.LastIndex(u.owner, qualnameSeparator)+1:]}
	}
	calls := map[footprint.Reference]bool{}
	reads := map[string]bool{}
	p.walk(u.node, func(n *gotreesitter.Node, field string, up ancestry) bool {
		switch p.typeOf(n) {
		case typeCall:
			if ref, ok := p.callReference(n); ok {
				calls[ref] = true
			}
		case typeIdentifier:
			if p.isReference(field, up) {
				name := p.text(n)
				reads[name] = true
				calls[footprint.Reference{Name: name, Kind: footprint.ReferencePlain}] = true
			}
		}
		return true
	})
	for ref := range calls {
		node.Calls = append(node.Calls, ref)
	}
	slices.SortFunc(node.Calls, func(a, b footprint.Reference) int {
		return strings.Compare(string(a.Kind)+a.Name, string(b.Kind)+b.Name)
	})
	for name := range reads {
		node.Reads = append(node.Reads, name)
	}
	slices.Sort(node.Reads)
	return node
}

func (p *parsed) callReference(call *gotreesitter.Node) (footprint.Reference, bool) {
	callee := p.child(call, fieldFunction)
	if callee == nil || p.typeOf(callee) != typeAttribute {
		return footprint.Reference{}, false
	}
	attribute, object := p.child(callee, fieldAttribute), p.child(callee, fieldObject)
	if attribute == nil || object == nil {
		return footprint.Reference{}, false
	}
	kind := footprint.ReferenceMember
	if receiver := p.text(object); receiver == receiverSelf || receiver == receiverClass {
		kind = footprint.ReferenceOwn
	}
	return footprint.Reference{Name: p.text(attribute), Kind: kind}, true
}

// isReference reports whether an identifier is a name use, not an attribute
// name or a keyword argument name.
func (p *parsed) isReference(field string, up ancestry) bool {
	parent := up.parent()
	if parent == nil {
		return true
	}
	switch p.typeOf(parent) {
	case typeAttribute:
		return field != fieldAttribute
	case typeKeywordArgument:
		return field != fieldName
	}
	return true
}
