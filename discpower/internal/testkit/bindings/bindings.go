// Package bindings loads the tests Shallnot binds to each criterion of the
// ecosystem fixtures, recorded from a real `shallnot check` run of each fixture.
package bindings

import (
	"embed"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

//go:embed testdata/*.json
var recorded embed.FS

const fileFormat = "testdata/%s-bound-tests.json"

type entry struct {
	Name      string `json:"name"`
	Classname string `json:"classname"`
	File      string `json:"file"`
	Line      int    `json:"line"`
}

// Bindings maps a requirement reference (`ID~REV`) to its bound tests.
type Bindings map[string][]ecosystem.BoundTest

// Load reads the recorded bindings of an ecosystem fixture.
func Load(name ecosystem.Name) (Bindings, error) {
	raw, err := recorded.ReadFile(fmt.Sprintf(fileFormat, name))
	if err != nil {
		return nil, fmt.Errorf("read the bindings of %q: %w", name, err)
	}
	var entries map[string][]entry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("decode the bindings of %q: %w", name, err)
	}
	loaded := Bindings{}
	for ref, tests := range entries {
		for _, test := range tests {
			loaded[ref] = append(loaded[ref], ecosystem.NewBoundTest(test.Classname, test.Name, test.File, test.Line))
		}
	}
	return loaded, nil
}

// Refs lists the requirement references in sorted order.
func (b Bindings) Refs() []string {
	refs := make([]string, 0, len(b))
	for ref := range b {
		refs = append(refs, ref)
	}
	slices.Sort(refs)
	return refs
}

// All lists every bound test of every criterion.
func (b Bindings) All() []ecosystem.BoundTest {
	var all []ecosystem.BoundTest
	for _, ref := range b.Refs() {
		all = append(all, b[ref]...)
	}
	return all
}
