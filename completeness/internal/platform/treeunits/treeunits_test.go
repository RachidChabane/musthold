package treeunits_test

import (
	"slices"
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"

	"github.com/RachidChabane/spec-proof/completeness/internal/domain/code"
	"github.com/RachidChabane/spec-proof/completeness/internal/platform/treeunits"
)

var rules = treeunits.Rules{Comments: []string{"comment"}, Identifiers: []string{"identifier", "property_identifier"}}

// collect parses the source and adds one function unit per function declaration and
// one per call statement.
func collect(t *testing.T, source string) []code.Unit {
	t.Helper()
	tree, err := treeunits.Parse(grammars.JavascriptLanguage(), []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	collector := treeunits.NewCollector(tree, "src/totals.js", rules)
	for _, statement := range tree.Named(tree.Root()) {
		if tree.Type(statement) == "comment" {
			continue
		}
		name := tree.Text(tree.Field(statement, "name"))
		if name == "" {
			name = "statement"
		}
		collector.Add(treeunits.Spec{Node: statement, Qualname: name, Name: name, Kind: code.KindFunction})
	}
	return collector.Units()
}

func TestUnits(t *testing.T) {
	const source = "// the total\nfunction total(items) {\n  return items.length; // count\n}\n\nfunction total(items, tax) {\n  return items.length * tax;\n}\n"

	t.Run("a unit has the text, the lines and the sorted names of its node [verifies SP-1313~1]", func(t *testing.T) {
		units := collect(t, source)
		if len(units) != 2 {
			t.Fatalf("units %+v", units)
		}
		first := units[0]
		if first.File != "src/totals.js" || first.Text != "function total(items) {\n  return items.length; // count\n}" {
			t.Errorf("text %q of %s", first.Text, first.File)
		}
		if first.StartLine != 2 || first.EndLine != 4 {
			t.Errorf("lines %d to %d", first.StartLine, first.EndLine)
		}
		if !slices.Equal(first.Mentions, []string{"items", "length", "total"}) {
			t.Errorf("mentions %v", first.Mentions)
		}
	})

	t.Run("a second unit of the same name is named with a repeat counter [verifies SP-1313~1]", func(t *testing.T) {
		units := collect(t, source)
		if units[0].Qualname != "total" || units[1].Qualname != "total#2" || units[1].Name != "total" {
			t.Fatalf("qualnames %q and %q, name %q", units[0].Qualname, units[1].Qualname, units[1].Name)
		}
	})

	t.Run("the normal form ignores comments and formatting and nothing else [verifies SP-1313~1]", func(t *testing.T) {
		plain := collect(t, "function total(items) { return items.length; }")[0].Normalized
		spaced := collect(t, "function  total( items )\n{\n  // count\n  return items.length /* here */;\n}")[0].Normalized
		if plain != spaced {
			t.Fatalf("plain %q, spaced %q", plain, spaced)
		}
		changed := collect(t, "function total(items) { return items.size; }")[0].Normalized
		if plain == changed {
			t.Fatal("a changed name left the normal form alone")
		}
	})
}
