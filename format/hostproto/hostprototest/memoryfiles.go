package hostprototest

import (
	"fmt"
	"io/fs"
)

// MemoryFiles is a hostproto.FileReader over files held in memory, by name.
type MemoryFiles map[string][]byte

// ReadFile implements hostproto.FileReader.
func (m MemoryFiles) ReadFile(name string, limit int) ([]byte, error) {
	content, found := m[name]
	if !found {
		return nil, fs.ErrNotExist
	}
	if len(content) > limit {
		return nil, fmt.Errorf("%s is %d bytes, over %d", name, len(content), limit)
	}
	return content, nil
}
