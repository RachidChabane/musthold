package node

import (
	"encoding/json"
	"io/fs"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

const packageFile = "package.json"

// projectManifest is the part of package.json that tells which test runner a
// project uses.
type projectManifest struct {
	Dependencies    map[string]json.RawMessage `json:"dependencies"`
	DevDependencies map[string]json.RawMessage `json:"devDependencies"`
	Jest            json.RawMessage            `json:"jest"`
}

func (m projectManifest) depends(name string) bool {
	_, runtime := m.Dependencies[name]
	_, development := m.DevDependencies[name]
	return runtime || development
}

func readManifest(root fs.FS) (projectManifest, bool) {
	content, err := fs.ReadFile(root, packageFile)
	if err != nil {
		return projectManifest{}, false
	}
	var manifest projectManifest
	if json.Unmarshal(content, &manifest) != nil {
		return projectManifest{}, false
	}
	return manifest, true
}

// configFiles lists the files of the project root whose name starts with one of
// the prefixes.
func configFiles(root fs.FS, prefixes ...string) []string {
	entries, err := fs.ReadDir(root, rootDirectory)
	if err != nil {
		return nil
	}
	var found []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		for _, prefix := range prefixes {
			if strings.HasPrefix(entry.Name(), prefix) {
				found = append(found, entry.Name())
				break
			}
		}
	}
	return found
}

// detection builds the detection of a framework from its evidence, sorted.
func detection(name ecosystem.Name, evidence []string) (ecosystem.Detection, bool) {
	if len(evidence) == 0 {
		return ecosystem.Detection{}, false
	}
	slices.Sort(evidence)
	return ecosystem.Detection{Name: name, Root: rootDirectory, Evidence: evidence}, true
}
