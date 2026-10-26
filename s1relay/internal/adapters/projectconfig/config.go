// Package projectconfig reads the System One settings of `.musthold/config.yaml`.
// The file is shared with the other modules; the relay reads only `systemone`.
package projectconfig

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Location of the configuration file under the project root.
const (
	Directory = ".musthold"
	File      = "config.yaml"
)

// SystemOne is the `systemone` section of the configuration. An absent member is
// empty.
type SystemOne struct {
	Endpoint string `yaml:"endpoint"`
	Model    string `yaml:"model"`
}

type document struct {
	SystemOne SystemOne `yaml:"systemone"`
}

// Load reads the section from the project at root. A project without a
// configuration file has an empty section.
func Load(root string) (SystemOne, error) {
	path := filepath.Join(root, Directory, File)
	content, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return SystemOne{}, nil
	}
	if err != nil {
		return SystemOne{}, fmt.Errorf("read %s: %w", path, err)
	}
	var parsed document
	if err := yaml.Unmarshal(content, &parsed); err != nil {
		return SystemOne{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return parsed.SystemOne, nil
}
