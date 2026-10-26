package testkit

import (
	"crypto/sha1" //nolint:gosec // a fake object id, as git's own, not a security digest
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/ledger/internal/domain/ledger"
)

// MemoryStore is an in-memory TreeStore: objects are named by the digest of their
// content, so equal content has an equal id, as in git.
type MemoryStore struct {
	blobs      map[ledger.ObjectID][]byte
	trees      map[ledger.ObjectID][]ledger.TreeEntry
	WrittenIDs []ledger.ObjectID
}

// NewMemoryStore returns an empty store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{blobs: map[ledger.ObjectID][]byte{}, trees: map[ledger.ObjectID][]ledger.TreeEntry{}}
}

func identify(kind string, parts ...string) ledger.ObjectID {
	sum := sha1.Sum([]byte(kind + "\x00" + strings.Join(parts, "\x00")))
	return ledger.ObjectID(hex.EncodeToString(sum[:]))
}

// ReadTree returns the entries of a tree.
func (s *MemoryStore) ReadTree(id ledger.ObjectID) ([]ledger.TreeEntry, error) {
	entries, found := s.trees[id]
	if !found {
		return nil, fmt.Errorf("no tree %s", id)
	}
	return slices.Clone(entries), nil
}

// ReadBlob returns the content of a blob.
func (s *MemoryStore) ReadBlob(id ledger.ObjectID) ([]byte, error) {
	content, found := s.blobs[id]
	if !found {
		return nil, fmt.Errorf("no blob %s", id)
	}
	return slices.Clone(content), nil
}

// WriteBlob stores a blob.
func (s *MemoryStore) WriteBlob(content []byte) (ledger.ObjectID, error) {
	id := identify("blob", string(content))
	s.blobs[id] = slices.Clone(content)
	s.WrittenIDs = append(s.WrittenIDs, id)
	return id, nil
}

// WriteTree stores a tree; its id does not depend on the order of the entries.
func (s *MemoryStore) WriteTree(entries []ledger.TreeEntry) (ledger.ObjectID, error) {
	sorted := slices.Clone(entries)
	slices.SortFunc(sorted, func(a, b ledger.TreeEntry) int { return strings.Compare(a.Name, b.Name) })
	parts := make([]string, 0, len(sorted))
	for _, entry := range sorted {
		parts = append(parts, entry.Mode+" "+entry.Name+" "+string(entry.ID))
	}
	id := identify("tree", parts...)
	s.trees[id] = sorted
	s.WrittenIDs = append(s.WrittenIDs, id)
	return id, nil
}

// Flatten lists every file of a tree as path to content.
func (s *MemoryStore) Flatten(root ledger.ObjectID) (map[string]string, error) {
	files := map[string]string{}
	return files, s.flatten(root, "", files)
}

func (s *MemoryStore) flatten(tree ledger.ObjectID, prefix string, files map[string]string) error {
	entries, err := s.ReadTree(tree)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Kind == ledger.EntryTree {
			if err := s.flatten(entry.ID, prefix+entry.Name+"/", files); err != nil {
				return err
			}
			continue
		}
		content, err := s.ReadBlob(entry.ID)
		if err != nil {
			return err
		}
		files[prefix+entry.Name] = string(content)
	}
	return nil
}
