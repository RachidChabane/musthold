// Package operations reads the binding of spec example operations to the
// functions that implement them, from `.musthold/config.yaml`:
//
//	operations:
//	  quote:
//	    python: "parkfee:quote"
//	    go: "parkfee.Quote"
package operations

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/RachidChabane/musthold/completeness/internal/app/ports"
)

// Location of the file inside a project.
const (
	stateDirectory = ".musthold"
	fileName       = "config.yaml"
)

var _ ports.Operations = Binding{}

// Binding implements ports.Operations over the decoded file.
type Binding struct {
	byOperation map[string][]string
}

// document is the part of the project configuration the module reads; the other
// members belong to other modules and are ignored.
type document struct {
	Operations map[string]map[string]string `yaml:"operations"`
}

// Decode reads the bindings from the YAML text of the project configuration.
func Decode(data []byte) (Binding, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var doc document
	if err := decoder.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return Binding{}, fmt.Errorf("decode the project configuration: %w", err)
	}
	binding := Binding{byOperation: map[string][]string{}}
	for operation, perLanguage := range doc.Operations {
		targets := make([]string, 0, len(perLanguage))
		for _, target := range perLanguage {
			targets = append(targets, target)
		}
		slices.Sort(targets)
		binding.byOperation[operation] = targets
	}
	return binding, nil
}

// Load reads the project configuration at the root; a project without one binds
// nothing.
func Load(root string) (Binding, error) {
	data, err := os.ReadFile(filepath.Join(root, stateDirectory, fileName))
	if errors.Is(err, fs.ErrNotExist) {
		return Binding{byOperation: map[string][]string{}}, nil
	}
	if err != nil {
		return Binding{}, fmt.Errorf("read the project configuration: %w", err)
	}
	return Decode(data)
}

// Bound lists what the operation is bound to, in any language, sorted.
func (b Binding) Bound(operation string) []string { return slices.Clone(b.byOperation[operation]) }
