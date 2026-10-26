package javascript

import (
	"slices"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/footprint"
)

const constructorMethod = "constructor"

// callGraph builds the graph contribution of the file: one node per unit that
// is a definition, one assignment name per module-level declarator.
func (p *parsed) callGraph(file string) footprint.FileGraph {
	graph := footprint.FileGraph{File: file}
	imports := p.importedNames()
	for _, u := range p.units() {
		if u.target == mutation.TargetModuleAssignment {
			graph.Assignments = append(graph.Assignments, footprint.Assignment{Name: u.name, Qualname: u.qualname})
			continue
		}
		graph.Nodes = append(graph.Nodes, p.nodeOf(u, imports))
	}
	return graph
}

// importedNames maps the local name of a renamed static import to the name the
// imported module exports it under.
func (p *parsed) importedNames() map[string]string {
	renamed := map[string]string{}
	p.walk(p.root(), func(n *gotreesitter.Node, _ string, _ ancestry) bool {
		if p.typeOf(n) != typeImportSpecifier {
			return true
		}
		if original, alias := p.child(n, fieldName), p.child(n, fieldAlias); original != nil && alias != nil {
			renamed[p.text(alias)] = p.text(original)
		}
		return true
	})
	return renamed
}

func (p *parsed) nodeOf(u unit, imports map[string]string) footprint.Node {
	node := footprint.Node{Qualname: u.qualname, Name: u.name, Owner: u.owner}
	if u.name == constructorMethod && u.owner != "" {
		node.Aliases = []string{u.owner[strings.LastIndex(u.owner, qualnameSeparator)+1:]}
	}
	calls := map[footprint.Reference]bool{}
	reads := map[string]bool{}
	p.walk(u.node, func(n *gotreesitter.Node, _ string, _ ancestry) bool {
		switch p.typeOf(n) {
		case typeCall, typeNew:
			if ref, ok := p.memberReference(n); ok {
				calls[ref] = true
			}
		case typeIdentifier, typeShorthand:
			p.recordName(p.text(n), imports, calls, reads)
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

// recordName records a mentioned name as a read and as a plain reference; the
// reference of a renamed import is to the name the module exports.
func (p *parsed) recordName(name string, imports map[string]string, calls map[footprint.Reference]bool, reads map[string]bool) {
	reads[name] = true
	referenced := name
	if original, renamed := imports[name]; renamed {
		referenced = original
		reads[original] = true
	}
	calls[footprint.Reference{Name: referenced, Kind: footprint.ReferencePlain}] = true
}

// memberReference reads the callee of a call or construction written through a
// receiver: `this.m()` is an own reference and `x.m()` a member reference.
func (p *parsed) memberReference(call *gotreesitter.Node) (footprint.Reference, bool) {
	callee := p.child(call, fieldFunction)
	if callee == nil {
		callee = p.child(call, fieldConstructor)
	}
	if callee == nil || p.typeOf(callee) != typeMember {
		return footprint.Reference{}, false
	}
	property, object := p.child(callee, fieldProperty), p.child(callee, fieldObject)
	if property == nil || object == nil {
		return footprint.Reference{}, false
	}
	kind := footprint.ReferenceMember
	if p.typeOf(object) == typeThis {
		kind = footprint.ReferenceOwn
	}
	return footprint.Reference{Name: p.text(property), Kind: kind}, true
}
