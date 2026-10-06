// Package treesitter locates function and method definitions in Python, Java,
// Kotlin, TypeScript, TSX and JavaScript sources with the pure-Go tree-sitter
// runtime.
package treesitter

import (
	"errors"
	"fmt"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/spec-proof/internal/app/ports"
	"github.com/RachidChabane/spec-proof/internal/domain/ecosystem"
)

// ErrUnsupportedLanguage is returned for a file whose extension no grammar covers.
var ErrUnsupportedLanguage = errors.New("unsupported language")

// qualnameSeparator joins the enclosing names of a definition.
const qualnameSeparator = "."

var _ ports.DefinitionLocator = (*Locator)(nil)

// Locator implements ports.DefinitionLocator with tree-sitter grammars chosen
// by file extension.
type Locator struct{}

// NewLocator returns a Locator.
func NewLocator() *Locator { return &Locator{} }

// scopeEntry is one enclosing definition or container of the node being visited.
type scopeEntry struct {
	name      string
	container bool
}

// walker collects the definitions of one parsed file.
type walker struct {
	lang        *gotreesitter.Language
	spec        languageSpec
	source      []byte
	definitions []ecosystem.Definition
}

// Locate returns every function and method definition of source, in source
// order. The grammar is chosen from the extension of path. A source with syntax
// errors still yields the definitions the parser recovered.
func (*Locator) Locate(path string, source []byte) ([]ecosystem.Definition, error) {
	spec, ok := specFor(path)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedLanguage, path)
	}
	lang := spec.load()
	tree, err := gotreesitter.NewParser(lang).Parse(source)
	if err != nil {
		return nil, fmt.Errorf("parse %q: %w", path, err)
	}
	defer tree.Release()

	w := &walker{lang: lang, spec: spec, source: source}
	w.visit(tree.RootNode(), nil)
	return w.definitions, nil
}

func (w *walker) visit(node *gotreesitter.Node, scope []scopeEntry) {
	nodeType := node.Type(w.lang)
	_, isDefinition := w.spec.definitions[nodeType]
	_, isContainer := w.spec.containers[nodeType]

	if isDefinition || isContainer {
		if name := w.nameOf(node); name != "" {
			if isDefinition {
				w.record(node, name, scope)
			}
			scope = append(scope[:len(scope):len(scope)], scopeEntry{name: name, container: isContainer})
		}
	}
	for i := 0; i < node.NamedChildCount(); i++ {
		w.visit(node.NamedChild(i), scope)
	}
}

func (w *walker) record(node *gotreesitter.Node, name string, scope []scopeEntry) {
	kind := ecosystem.KindFunction
	if len(scope) > 0 && scope[len(scope)-1].container {
		kind = ecosystem.KindMethod
	}
	names := make([]string, 0, len(scope)+1)
	for _, entry := range scope {
		names = append(names, entry.name)
	}
	names = append(names, name)
	w.definitions = append(w.definitions, ecosystem.Definition{
		Qualname:  strings.Join(names, qualnameSeparator),
		Kind:      kind,
		Span:      ecosystem.Span{Start: int(node.StartByte()), End: int(node.EndByte())},
		StartLine: int(node.StartPoint().Row) + 1,
		EndLine:   int(node.EndPoint().Row) + 1,
	})
}

func (w *walker) nameOf(node *gotreesitter.Node) string {
	if named := node.ChildByFieldName(nameField, w.lang); named != nil {
		return named.Text(w.source)
	}
	for i := 0; i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if _, ok := nameNodeTypes[child.Type(w.lang)]; ok {
			return child.Text(w.source)
		}
	}
	return ""
}
