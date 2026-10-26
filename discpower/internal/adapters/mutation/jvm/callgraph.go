package jvm

import (
	"slices"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/footprint"
)

// callGraph builds the graph contribution of the file: one node per definition
// unit, one assignment name per constant declaration.
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
	if u.constructor && u.owner != "" {
		node.Aliases = []string{u.owner[strings.LastIndex(u.owner, qualnameSeparator)+1:]}
	}
	calls := map[footprint.Reference]bool{}
	reads := map[string]bool{}
	p.walk(u.node, func(n *gotreesitter.Node, field string, up ancestry) bool {
		for _, ref := range p.dialect.callReferences(p, n) {
			calls[ref] = true
		}
		for _, name := range p.dialect.fieldReads(p, n) {
			reads[name] = true
		}
		if p.dialect.isIdentifier(p.typeOf(n)) && p.dialect.identifierRole(p, n, field, up) == roleReference {
			name := p.text(n)
			reads[name] = true
			calls[footprint.Reference{Name: name, Kind: footprint.ReferencePlain}] = true
		}
		return true
	})
	node.Calls = sortedReferences(calls)
	node.Reads = sortedNames(reads)
	return node
}

func sortedReferences(set map[footprint.Reference]bool) []footprint.Reference {
	var refs []footprint.Reference
	for ref := range set {
		refs = append(refs, ref)
	}
	slices.SortFunc(refs, func(a, b footprint.Reference) int {
		return strings.Compare(string(a.Kind)+a.Name, string(b.Kind)+b.Name)
	})
	return refs
}

func sortedNames(set map[string]bool) []string {
	var names []string
	for name := range set {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// receiverReferences names the references of a call by the receiver it goes
// through. A call without receiver is plain, which reaches functions and
// constructors, and own, which reaches the methods of the enclosing class; a
// call through `this` or `super` is own; any other call is a member call.
func receiverReferences(p *parsed, name string, receiver *gotreesitter.Node, ownReceiverTypes ...string) []footprint.Reference {
	switch {
	case receiver == nil:
		return []footprint.Reference{
			{Name: name, Kind: footprint.ReferencePlain},
			{Name: name, Kind: footprint.ReferenceOwn},
		}
	case slices.Contains(ownReceiverTypes, p.typeOf(receiver)):
		return []footprint.Reference{{Name: name, Kind: footprint.ReferenceOwn}}
	}
	return []footprint.Reference{{Name: name, Kind: footprint.ReferenceMember}}
}
