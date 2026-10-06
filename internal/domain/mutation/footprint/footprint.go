// Package footprint closes a set of entry functions over a static call graph.
// The graph is name-based and conservative: it never misses a call that the
// source states, and may include a definition of the same name that the call
// does not reach.
package footprint

import (
	"slices"
	"strings"
)

// ReferenceKind says how a call names its callee.
type ReferenceKind string

// Reference kinds.
const (
	// ReferencePlain is a bare name: `helper()`.
	ReferencePlain ReferenceKind = "plain"
	// ReferenceMember is a call through any receiver or package: `x.helper()`.
	ReferenceMember ReferenceKind = "member"
	// ReferenceOwn is a call through the receiver of the enclosing method:
	// `self.helper()`.
	ReferenceOwn ReferenceKind = "own"
)

// Reference is one call a definition makes.
type Reference struct {
	Name string
	Kind ReferenceKind
}

// Node is a top-level function or a method; definitions nested in it belong to
// it, so their calls and reads are its own.
type Node struct {
	Qualname string
	// Name is the last segment of the qualname.
	Name string
	// Owner is the enclosing class or receiver type, empty for a function.
	Owner string
	// Aliases are the other names a call reaches the node by, such as the class
	// name for `Class.__init__`.
	Aliases []string
	Calls   []Reference
	// Reads are the identifiers the body mentions; those naming a module-level
	// assignment make that assignment part of the footprint.
	Reads []string
}

// FileGraph is the call graph contribution of one source file.
type FileGraph struct {
	File        string
	Nodes       []Node
	Assignments []Assignment
}

// Assignment is a module-level assignment: the identifier code reads it by and
// the qualname its edits carry.
type Assignment struct {
	Name     string
	Qualname string
}

// Ref designates a definition or a module-level assignment of a file.
type Ref struct {
	File     string
	Qualname string
}

// Footprint is the closure: definitions and the module-level assignments they
// reference, each sorted by file then qualname.
type Footprint struct {
	Definitions []Ref
	Assignments []Ref
}

// Files lists the files of the footprint, sorted.
func (f Footprint) Files() []string {
	var files []string
	for _, ref := range slices.Concat(f.Definitions, f.Assignments) {
		if !slices.Contains(files, ref.File) {
			files = append(files, ref.File)
		}
	}
	slices.Sort(files)
	return files
}

// ContainsDefinition reports whether the footprint holds the definition.
func (f Footprint) ContainsDefinition(ref Ref) bool { return slices.Contains(f.Definitions, ref) }

// ContainsAssignment reports whether the footprint holds the assignment.
func (f Footprint) ContainsAssignment(ref Ref) bool { return slices.Contains(f.Assignments, ref) }

type nodeKey = Ref

type index struct {
	nodes       map[nodeKey]Node
	order       []nodeKey
	assignments map[string][]Ref
}

func buildIndex(graphs []FileGraph) index {
	idx := index{nodes: map[nodeKey]Node{}, assignments: map[string][]Ref{}}
	for _, graph := range graphs {
		for _, node := range graph.Nodes {
			key := nodeKey{File: graph.File, Qualname: node.Qualname}
			idx.nodes[key] = node
			idx.order = append(idx.order, key)
		}
		for _, assignment := range graph.Assignments {
			idx.assignments[assignment.Name] = append(idx.assignments[assignment.Name], Ref{File: graph.File, Qualname: assignment.Qualname})
		}
	}
	return idx
}

// Close returns the footprint of the entries: the entries themselves, every
// definition reachable from them through calls, and the module-level
// assignments any of those definitions reads. An entry that names a definition
// nested in a node designates that node. Entries that match nothing are ignored.
func Close(graphs []FileGraph, entries []Ref) Footprint {
	idx := buildIndex(graphs)
	visited := map[nodeKey]bool{}
	reached := map[Ref]bool{}
	var queue []nodeKey
	push := func(key nodeKey) {
		if !visited[key] {
			visited[key] = true
			queue = append(queue, key)
		}
	}
	for _, entry := range entries {
		if key, ok := idx.enclosing(entry); ok {
			push(key)
		}
	}
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		node := idx.nodes[key]
		for _, call := range node.Calls {
			for _, callee := range idx.resolve(key, node, call) {
				push(callee)
			}
		}
		for _, name := range node.Reads {
			for _, ref := range preferSameFile(idx.assignments[name], key.File) {
				reached[ref] = true
			}
		}
	}

	result := Footprint{}
	for key := range visited {
		result.Definitions = append(result.Definitions, key)
	}
	for ref := range reached {
		result.Assignments = append(result.Assignments, ref)
	}
	sortRefs(result.Definitions)
	sortRefs(result.Assignments)
	return result
}

func sortRefs(refs []Ref) {
	slices.SortFunc(refs, func(a, b Ref) int {
		if c := strings.Compare(a.File, b.File); c != 0 {
			return c
		}
		return strings.Compare(a.Qualname, b.Qualname)
	})
}

func (idx index) enclosing(entry Ref) (nodeKey, bool) {
	if _, ok := idx.nodes[entry]; ok {
		return entry, true
	}
	for key := range idx.nodes {
		if key.File == entry.File && strings.HasPrefix(entry.Qualname, key.Qualname+nestedSeparator) {
			return key, true
		}
	}
	return nodeKey{}, false
}

// nestedSeparator joins a nested definition to its enclosing qualname.
const nestedSeparator = "."

func (idx index) resolve(caller nodeKey, node Node, call Reference) []nodeKey {
	switch call.Kind {
	case ReferencePlain:
		return preferSameFile(idx.match(func(n Node) bool {
			return n.Owner == "" && n.Name == call.Name || slices.Contains(n.Aliases, call.Name)
		}), caller.File)
	case ReferenceOwn:
		own := idx.matchIn(caller.File, func(n Node) bool { return n.Owner == node.Owner && n.Name == call.Name })
		if len(own) > 0 {
			return own
		}
	}
	return idx.match(func(n Node) bool { return n.Name == call.Name || slices.Contains(n.Aliases, call.Name) })
}

func (idx index) match(predicate func(Node) bool) []nodeKey {
	var found []nodeKey
	for _, key := range idx.order {
		if predicate(idx.nodes[key]) {
			found = append(found, key)
		}
	}
	return found
}

func (idx index) matchIn(file string, predicate func(Node) bool) []nodeKey {
	return slices.DeleteFunc(idx.match(predicate), func(key nodeKey) bool { return key.File != file })
}

// preferSameFile keeps the references in file when there are any, else all.
func preferSameFile(refs []Ref, file string) []Ref {
	local := slices.DeleteFunc(slices.Clone(refs), func(ref Ref) bool { return ref.File != file })
	if len(local) > 0 {
		return local
	}
	return refs
}
