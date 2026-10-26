package jvm

import (
	"slices"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/operator"
)

const (
	javaPath   = "src/T.java"
	kotlinPath = "src/T.kt"
)

// javaFile puts one member in a class.
func javaFile(member string) string { return "class T {\n    " + member + "\n}\n" }

// kotlinFile puts one declaration in a file.
func kotlinFile(declaration string) string { return declaration + "\n" }

func sourceFile(path, source string) mutation.SourceFile {
	return mutation.SourceFile{Path: path, Content: []byte(source)}
}

func candidatesOf(t testing.TB, path, source string) []operator.Candidate {
	t.Helper()
	got, err := New().Candidates(sourceFile(path, source))
	if err != nil {
		t.Fatalf("Candidates: %v", err)
	}
	return got
}

func sentinelsOf(t testing.TB, path, source string, seed operator.Seed) []operator.SentinelCandidate {
	t.Helper()
	got, err := New().SentinelCandidates(sourceFile(path, source), seed)
	if err != nil {
		t.Fatalf("SentinelCandidates: %v", err)
	}
	return got
}

// sourcesOf lists the sorted new sources of the candidates of one family.
func sourcesOf(candidates []operator.Candidate, family operator.Family) []string {
	var out []string
	for _, c := range candidates {
		if c.Family == family {
			out = append(out, c.Edit.NewSource)
		}
	}
	slices.Sort(out)
	return out
}

// unitsOf lists the units of a source.
func unitsOf(t testing.TB, path, source string) []unit {
	t.Helper()
	parsedFile, err := parseChecked(path, []byte(source))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer parsedFile.release()
	return parsedFile.units()
}

func qualnamesOf(units []unit, target mutation.TargetKind) []string {
	var out []string
	for _, u := range units {
		if u.target == target {
			out = append(out, u.qualname)
		}
	}
	return out
}

// Node types the test-local definition locator reads; they restate the rule
// the production units follow, independently of its tables.
var (
	locatorDefinitions = map[string]map[string]bool{
		javaExtension:   {"method_declaration": true, "constructor_declaration": true},
		kotlinExtension: {"function_declaration": true},
	}
	locatorContainers = map[string]map[string]bool{
		javaExtension:   {"class_declaration": true, "interface_declaration": true, "enum_declaration": true, "record_declaration": true},
		kotlinExtension: {"class_declaration": true, "object_declaration": true},
	}
	locatorNameTypes = map[string]bool{"identifier": true, "simple_identifier": true, "type_identifier": true, "property_identifier": true}
)

// testLocator is a definition locator over the tree-sitter grammars that
// reports every definition, nested ones included, like the production locator.
type testLocator struct{}

func (testLocator) Locate(path string, source []byte) ([]ecosystem.Definition, error) {
	extension := path[strings.LastIndex(path, "."):]
	lang := grammars.JavaLanguage()
	if extension == kotlinExtension {
		lang = grammars.KotlinLanguage()
	}
	tree, err := gotreesitter.NewParser(lang).Parse(source)
	if err != nil {
		return nil, err
	}
	defer tree.Release()
	walk := locatorWalk{lang: lang, source: source, extension: extension}
	walk.visit(tree.RootNode(), nil)
	return walk.found, nil
}

type locatorWalk struct {
	lang      *gotreesitter.Language
	source    []byte
	extension string
	found     []ecosystem.Definition
}

func (w *locatorWalk) visit(node *gotreesitter.Node, scope []string) {
	nodeType := node.Type(w.lang)
	definition, container := locatorDefinitions[w.extension][nodeType], locatorContainers[w.extension][nodeType]
	if definition || container {
		if name := w.nameOf(node); name != "" {
			scope = append(scope[:len(scope):len(scope)], name)
			if definition {
				span := ecosystem.Span{Start: int(node.StartByte()), End: int(node.EndByte())}
				w.found = append(w.found, ecosystem.Definition{Qualname: strings.Join(scope, "."), Kind: ecosystem.KindMethod, Span: span})
			}
		}
	}
	for i := 0; i < node.NamedChildCount(); i++ {
		w.visit(node.NamedChild(i), scope)
	}
}

func (w *locatorWalk) nameOf(node *gotreesitter.Node) string {
	if named := node.ChildByFieldName("name", w.lang); named != nil {
		return named.Text(w.source)
	}
	for i := 0; i < node.NamedChildCount(); i++ {
		if child := node.NamedChild(i); locatorNameTypes[child.Type(w.lang)] {
			return child.Text(w.source)
		}
	}
	return ""
}

// splice applies an edit with the production splicer, over the test-local
// definition locator and this engine as the assignment locator.
func splice(path string, source []byte, edit mutation.Edit) ([]byte, error) {
	splicer := mutation.Splicer{Locator: testLocator{}, Assignments: New()}
	return splicer.Splice(path, source, edit)
}
