package treesitter

import (
	"path"
	"sync"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// languageSpec is the per-language data the Locator needs: how to load the
// grammar and which node types are definitions and containers.
type languageSpec struct {
	load        func() *gotreesitter.Language
	definitions map[string]struct{}
	containers  map[string]struct{}
}

func newSpec(load func() *gotreesitter.Language, definitions, containers []string) languageSpec {
	return languageSpec{
		load:        sync.OnceValue(load),
		definitions: toSet(definitions),
		containers:  toSet(containers),
	}
}

func toSet(items []string) map[string]struct{} {
	set := make(map[string]struct{}, len(items))
	for _, item := range items {
		set[item] = struct{}{}
	}
	return set
}

// nameNodeTypes lists the node types that carry the name of a definition when
// the grammar has no `name` field.
var nameNodeTypes = toSet([]string{"identifier", "simple_identifier", "type_identifier", "property_identifier"})

// nameField is the grammar field that holds the name of a definition.
const nameField = "name"

// specsByExtension maps a file extension, dot included, to its language.
var specsByExtension = buildSpecs()

func buildSpecs() map[string]languageSpec {
	python := newSpec(grammars.PythonLanguage,
		[]string{"function_definition"},
		[]string{"class_definition"})
	java := newSpec(grammars.JavaLanguage,
		[]string{"method_declaration", "constructor_declaration"},
		[]string{"class_declaration", "interface_declaration", "enum_declaration", "record_declaration"})
	kotlin := newSpec(grammars.KotlinLanguage,
		[]string{"function_declaration"},
		[]string{"class_declaration", "object_declaration"})
	typescript := newSpec(grammars.TypescriptLanguage,
		[]string{"function_declaration", "method_definition"},
		[]string{"class_declaration"})
	tsx := newSpec(grammars.TsxLanguage,
		[]string{"function_declaration", "method_definition"},
		[]string{"class_declaration"})
	javascript := newSpec(grammars.JavascriptLanguage,
		[]string{"function_declaration", "method_definition"},
		[]string{"class_declaration"})

	return map[string]languageSpec{
		".py":   python,
		".java": java,
		".kt":   kotlin,
		".kts":  kotlin,
		".ts":   typescript,
		".tsx":  tsx,
		".js":   javascript,
		".jsx":  javascript,
		".mjs":  javascript,
		".cjs":  javascript,
	}
}

func specFor(filePath string) (languageSpec, bool) {
	spec, ok := specsByExtension[path.Ext(filePath)]
	return spec, ok
}
