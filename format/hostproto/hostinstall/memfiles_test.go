package hostinstall_test

import (
	"strings"
	"testing"
)

// absent is what text returns for a file the tree does not hold.
const absent = "<absent>"

// memFiles is an in-memory project tree implementing hostinstall.Files. It
// records every write and removal that went through the port.
type memFiles struct {
	data       map[string][]byte
	writes     []string
	removes    []string
	failWrites error
}

func newMemFiles() *memFiles { return &memFiles{data: map[string][]byte{}} }

func (m *memFiles) Read(path string) ([]byte, bool, error) {
	content, found := m.data[path]
	if !found {
		return nil, false, nil
	}
	return append([]byte(nil), content...), true, nil
}

// Exists reports a file, or a directory holding at least one file.
func (m *memFiles) Exists(path string) bool {
	if _, found := m.data[path]; found {
		return true
	}
	for held := range m.data {
		if strings.HasPrefix(held, path+"/") {
			return true
		}
	}
	return false
}

// Directories lists the directories directly inside a directory, as the names of
// the first segment below it of every file held.
func (m *memFiles) Directories(path string) []string {
	prefix := ""
	if path != "" {
		prefix = path + "/"
	}
	seen := map[string]bool{}
	var names []string
	for held := range m.data {
		rest, found := strings.CutPrefix(held, prefix)
		if !found {
			continue
		}
		name, _, nested := strings.Cut(rest, "/")
		if nested && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

func (m *memFiles) Write(path string, content []byte) error {
	if m.failWrites != nil {
		return m.failWrites
	}
	m.data[path] = append([]byte(nil), content...)
	m.writes = append(m.writes, path)
	return nil
}

func (m *memFiles) Remove(path string) error {
	delete(m.data, path)
	m.removes = append(m.removes, path)
	return nil
}

// put seeds a file without recording a write.
func (m *memFiles) put(t *testing.T, path, content string) {
	t.Helper()
	m.data[path] = []byte(content)
}

// text returns a file as a string, or absent when the tree does not hold it.
func (m *memFiles) text(path string) string {
	content, found := m.data[path]
	if !found {
		return absent
	}
	return string(content)
}
