package ledger

import (
	"bytes"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/ledger/internal/domain/refusal"
)

// TreeStore reads and writes the git objects a ledger tree is made of.
type TreeStore interface {
	ReadTree(id ObjectID) ([]TreeEntry, error)
	ReadBlob(id ObjectID) ([]byte, error)
	WriteBlob(content []byte) (ObjectID, error)
	WriteTree(entries []TreeEntry) (ObjectID, error)
}

// Outcome of applying additions: the paths that were written and the paths that
// were already in the ledger with the same bytes, or kept by policy.
type Outcome struct {
	Added     []string
	Unchanged []string
}

// ApplyAdditions returns the tree that holds the additions on top of the tree
// root, which is nil for an empty ledger. Subtrees without an addition keep their
// object, so a ledger of any size costs one tree per touched directory. An
// addition at a path that holds other bytes is refused unless its policy keeps
// the existing bytes; an addition that needs a file where a directory is, or a
// directory where a file is, is refused. When nothing is added the result is root
// itself (empty when root is nil) and Outcome.Added is empty.
func ApplyAdditions(store TreeStore, root *ObjectID, additions []Addition) (ObjectID, Outcome, error) {
	applier := treeApplier{store: store}
	pending := make([]pendingAddition, 0, len(additions))
	for _, addition := range additions {
		pending = append(pending, pendingAddition{Addition: addition, rest: addition.Path})
	}
	id, changed, err := applier.apply(root, pending)
	if err != nil {
		return "", Outcome{}, err
	}
	slices.Sort(applier.outcome.Added)
	slices.Sort(applier.outcome.Unchanged)
	if !changed && root != nil {
		return *root, applier.outcome, nil
	}
	return id, applier.outcome, nil
}

type pendingAddition struct {
	Addition
	// rest is the part of the path below the directory being applied.
	rest string
}

type treeApplier struct {
	store   TreeStore
	outcome Outcome
}

// entrySet is the entries of one tree, addressable by name.
type entrySet struct {
	entries []TreeEntry
	index   map[string]int
}

func newEntrySet(entries []TreeEntry) *entrySet {
	set := &entrySet{index: make(map[string]int, len(entries))}
	for _, entry := range entries {
		set.put(entry)
	}
	return set
}

func (s *entrySet) get(name string) (TreeEntry, bool) {
	index, exists := s.index[name]
	if !exists {
		return TreeEntry{}, false
	}
	return s.entries[index], true
}

func (s *entrySet) put(entry TreeEntry) {
	if index, exists := s.index[entry.Name]; exists {
		s.entries[index] = entry
		return
	}
	s.index[entry.Name] = len(s.entries)
	s.entries = append(s.entries, entry)
}

func (a *treeApplier) apply(tree *ObjectID, additions []pendingAddition) (ObjectID, bool, error) {
	existing, err := a.entriesOf(tree)
	if err != nil {
		return "", false, err
	}
	set := newEntrySet(existing)
	leaves, directories, err := partition(additions)
	if err != nil {
		return "", false, err
	}
	changed := false
	for _, name := range slices.Sorted(maps.Keys(leaves)) {
		entry, added, err := a.applyLeaf(leaves[name], set, name)
		if err != nil {
			return "", false, err
		}
		if added {
			set.put(entry)
			changed = true
		}
	}
	for _, name := range slices.Sorted(maps.Keys(directories)) {
		entry, subtreeChanged, err := a.applyDirectory(name, directories[name], set)
		if err != nil {
			return "", false, err
		}
		if subtreeChanged {
			set.put(entry)
			changed = true
		}
	}
	if !changed {
		return "", false, nil
	}
	id, err := a.store.WriteTree(set.entries)
	return id, true, err
}

func (a *treeApplier) entriesOf(tree *ObjectID) ([]TreeEntry, error) {
	if tree == nil {
		return nil, nil
	}
	entries, err := a.store.ReadTree(*tree)
	if err != nil {
		return nil, fmt.Errorf("read tree %s: %w", *tree, err)
	}
	return entries, nil
}

// partition splits additions by the first segment of their remaining path into
// files of this directory and additions for a subdirectory, and refuses a name
// that is both.
func partition(additions []pendingAddition) (map[string]pendingAddition, map[string][]pendingAddition, error) {
	leaves := map[string]pendingAddition{}
	directories := map[string][]pendingAddition{}
	for _, addition := range additions {
		name, rest, nested := strings.Cut(addition.rest, pathSeparator)
		if name == "" || (nested && rest == "") {
			return nil, nil, refusal.Newf("%q is not a file path", addition.Path)
		}
		if !nested {
			if _, duplicate := leaves[name]; duplicate {
				return nil, nil, refusal.Newf("%s is added twice", addition.Path)
			}
			leaves[name] = addition
			continue
		}
		addition.rest = rest
		directories[name] = append(directories[name], addition)
	}
	for name := range leaves {
		if _, clash := directories[name]; clash {
			return nil, nil, refusal.Newf("%s is both a file and a directory of the additions", name)
		}
	}
	return leaves, directories, nil
}

func (a *treeApplier) applyLeaf(addition pendingAddition, set *entrySet, name string) (TreeEntry, bool, error) {
	existing, exists := set.get(name)
	if !exists {
		id, err := a.store.WriteBlob(addition.Content)
		if err != nil {
			return TreeEntry{}, false, fmt.Errorf("write blob %s: %w", addition.Path, err)
		}
		a.outcome.Added = append(a.outcome.Added, addition.Path)
		return TreeEntry{Mode: ModeBlob, Kind: EntryBlob, ID: id, Name: name}, true, nil
	}
	if existing.Kind != EntryBlob {
		return TreeEntry{}, false, refusal.Newf("%s is a directory of the ledger", addition.Path)
	}
	if addition.Policy == PolicyKeepExisting {
		a.outcome.Unchanged = append(a.outcome.Unchanged, addition.Path)
		return TreeEntry{}, false, nil
	}
	current, err := a.store.ReadBlob(existing.ID)
	if err != nil {
		return TreeEntry{}, false, fmt.Errorf("read blob %s: %w", addition.Path, err)
	}
	if !bytes.Equal(current, addition.Content) {
		return TreeEntry{}, false, refusal.Newf("%s exists with other bytes: the ledger never rewrites a path", addition.Path)
	}
	a.outcome.Unchanged = append(a.outcome.Unchanged, addition.Path)
	return TreeEntry{}, false, nil
}

func (a *treeApplier) applyDirectory(name string, additions []pendingAddition, set *entrySet) (TreeEntry, bool, error) {
	var existing *ObjectID
	if entry, exists := set.get(name); exists {
		if entry.Kind != EntryTree {
			return TreeEntry{}, false, refusal.Newf("%s is a file of the ledger, not a directory", additions[0].Path)
		}
		existing = &entry.ID
	}
	id, changed, err := a.apply(existing, additions)
	if err != nil || !changed {
		return TreeEntry{}, false, err
	}
	return TreeEntry{Mode: ModeTree, Kind: EntryTree, ID: id, Name: name}, true, nil
}
