package syntax_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"

	"github.com/RachidChabane/musthold/alignment/internal/platform/syntax"
)

const source = "const total = price(2);\nfunction price(n) {\n  return n * 2;\n}\n"

func parse(t *testing.T) *syntax.Tree {
	t.Helper()
	tree, err := syntax.Parse(grammars.JavascriptLanguage(), []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tree.Release)
	return tree
}

func TestTree(t *testing.T) {
	t.Run("a node is read by its type, its text, its fields and its children [verifies SP-1215~1]", func(t *testing.T) {
		tree := parse(t)
		statements := tree.Named(tree.Root())
		if len(statements) != 2 || tree.Type(statements[0]) != "lexical_declaration" || tree.Type(statements[1]) != "function_declaration" {
			t.Fatalf("statements %v", statements)
		}
		if name := tree.Text(tree.Field(statements[1], "name")); name != "price" {
			t.Errorf("name %q", name)
		}
		if body := tree.Field(statements[1], "body"); tree.Type(body) != "statement_block" {
			t.Errorf("body %q", tree.Type(body))
		}
		if tree.Type(nil) != "" || tree.Text(nil) != "" || tree.Field(nil, "name") != nil {
			t.Error("nil is not read as empty")
		}
		if found := tree.FirstOfType(statements[0], "variable_declarator"); tree.Text(found) != "total = price(2)" {
			t.Errorf("first declarator %q", tree.Text(found))
		}
		if tree.FirstOfType(statements[0], "nothing") != nil {
			t.Error("a type that is absent was found")
		}
	})

	t.Run("a node knows its bytes and its lines [verifies SP-1215~1]", func(t *testing.T) {
		tree := parse(t)
		function := tree.Named(tree.Root())[1]
		span := syntax.Span(function)
		if got := source[span.Start:span.End]; got != "function price(n) {\n  return n * 2;\n}" {
			t.Errorf("span %q", got)
		}
		if syntax.Line(function) != 2 || syntax.EndLine(function) != 4 {
			t.Errorf("lines %d to %d", syntax.Line(function), syntax.EndLine(function))
		}
	})

	t.Run("a walk visits the nodes in source order with their parents and fields, and skips what the visitor refuses [verifies SP-1215~1]", func(t *testing.T) {
		tree := parse(t)
		var identifiers []string
		tree.Walk(tree.Root(), func(node, parent *gotreesitter.Node, field string) bool {
			if tree.Type(node) == "function_declaration" {
				return false
			}
			if tree.Type(node) == "identifier" {
				identifiers = append(identifiers, field+":"+tree.Text(node)+"<"+tree.Type(parent))
			}
			return true
		})
		if len(identifiers) != 2 || identifiers[0] != "name:total<variable_declarator" || identifiers[1] != "function:price<call_expression" {
			t.Fatalf("identifiers %v", identifiers)
		}
	})
}
