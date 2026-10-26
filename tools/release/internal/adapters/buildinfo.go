package adapters

import (
	"bytes"
	"debug/buildinfo"
	"path/filepath"
	"strings"

	"github.com/RachidChabane/musthold/tools/release/internal/domain"
)

// BuildInfoInspector reads the build information a Go linker embeds in a binary.
type BuildInfoInspector struct{}

// Inspect reads the build information of a binary: the main module, the modules it
// links with their hashes, and the build settings.
func (BuildInfoInspector) Inspect(binary []byte) (domain.BuildFacts, error) {
	info, err := buildinfo.Read(bytes.NewReader(binary))
	if err != nil {
		return domain.BuildFacts{}, err
	}
	facts := domain.BuildFacts{GoVersion: info.GoVersion, MainPath: info.Main.Path, MainVersion: info.Main.Version}
	// A module replaced by another module is described as that module; one replaced
	// by a directory keeps its own path and version, since a directory has none.
	for _, dependency := range info.Deps {
		linked := dependency
		if dependency.Replace != nil && !isDirectoryPath(dependency.Replace.Path) {
			linked = dependency.Replace
		}
		facts.Dependencies = append(facts.Dependencies, domain.Dependency{Path: linked.Path, Version: linked.Version, Sum: linked.Sum})
	}
	for _, setting := range info.Settings {
		facts.Settings = append(facts.Settings, domain.BuildSetting{Key: setting.Key, Value: setting.Value})
	}
	return facts, nil
}

// isDirectoryPath reports whether a replacement path names a directory, as go.mod
// spells it: relative with a leading dot or absolute.
func isDirectoryPath(path string) bool {
	return path == "." || path == ".." || strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../") || filepath.IsAbs(path)
}

// readBinaryFacts reads what a dist check compares: the main package and the
// build settings.
func readBinaryFacts(binary []byte) (*domain.BinaryFacts, error) {
	info, err := buildinfo.Read(bytes.NewReader(binary))
	if err != nil {
		return nil, err
	}
	facts := &domain.BinaryFacts{
		MainPath: info.Path, Settings: map[string]string{},
		Contains: func(needle string) bool { return bytes.Contains(binary, []byte(needle)) },
	}
	for _, setting := range info.Settings {
		facts.Settings[setting.Key] = setting.Value
	}
	return facts, nil
}
