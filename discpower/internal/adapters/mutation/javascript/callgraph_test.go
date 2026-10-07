package javascript

import (
	"os"
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/footprint"
)

const (
	goldenSourcePath = "testdata/parkfee/fees.ts"
	goldenFilePath   = "testdata/parkfee/golden.json"
	goldenName       = "parkfee/fees.ts"
)

func graphOf(t testing.TB, path, source string) footprint.FileGraph {
	t.Helper()
	graph, err := New().CallGraph(fileAt(path, source))
	if err != nil {
		t.Fatal(err)
	}
	return graph
}

func nodesByQualname(graph footprint.FileGraph) map[string]footprint.Node {
	nodes := map[string]footprint.Node{}
	for _, n := range graph.Nodes {
		nodes[n.Qualname] = n
	}
	return nodes
}

func TestCallGraph(t *testing.T) {
	content, err := os.ReadFile(goldenSourcePath)
	if err != nil {
		t.Fatal(err)
	}
	graph := graphOf(t, goldenName, string(content))

	t.Run("nodes carry owner, name, aliases and references [verifies SP-1004~1]", func(t *testing.T) {
		nodes := nodesByQualname(graph)
		want := []string{"Tariff.constructor", "Tariff.breakdown", "Tariff.atNight", "Tariff.label", "validatedFree", "charge"}
		if len(nodes) != len(want) {
			t.Fatalf("nodes = %v", nodes)
		}
		for _, qualname := range want {
			if _, ok := nodes[qualname]; !ok {
				t.Fatalf("%s missing from %v", qualname, nodes)
			}
		}
		constructor := nodes["Tariff.constructor"]
		if constructor.Owner != "Tariff" || !slices.Equal(constructor.Aliases, []string{"Tariff"}) || !slices.Contains(constructor.Reads, "HOURLY_RATE") {
			t.Fatalf("constructor = %+v", constructor)
		}
		charge := nodes["charge"]
		if !slices.Contains(charge.Calls, footprint.Reference{Name: "breakdown", Kind: footprint.ReferenceMember}) ||
			!slices.Contains(charge.Calls, footprint.Reference{Name: "Tariff", Kind: footprint.ReferencePlain}) {
			t.Fatalf("charge calls = %+v", charge.Calls)
		}
	})

	t.Run("module-level declarators are listed with their names [verifies SP-1004~1]", func(t *testing.T) {
		var names []string
		for _, a := range graph.Assignments {
			names = append(names, a.Name)
		}
		want := []string{"GRACE_MINUTES", "HOURLY_RATE", "DAILY_CAP", "NIGHT_START", "VALIDATION_WINDOW", "LABEL", "PENALTY", "capOf"}
		if !slices.Equal(names, want) {
			t.Fatalf("assignments = %v", names)
		}
	})

	t.Run("a helper reachable only through a call stays in the footprint [verifies SP-1004~1]", func(t *testing.T) {
		closed := footprint.Close([]footprint.FileGraph{graph}, []footprint.Ref{{File: goldenName, Qualname: "charge"}})
		for _, qualname := range []string{"charge", "Tariff.constructor", "Tariff.breakdown", "validatedFree"} {
			if !closed.ContainsDefinition(footprint.Ref{File: goldenName, Qualname: qualname}) {
				t.Errorf("%s missing from %v", qualname, closed.Definitions)
			}
		}
		if closed.ContainsDefinition(footprint.Ref{File: goldenName, Qualname: "Tariff.atNight"}) || closed.ContainsAssignment(footprint.Ref{File: goldenName, Qualname: "NIGHT_START"}) {
			t.Fatal("an unreached member is in the footprint")
		}
		if !closed.ContainsAssignment(footprint.Ref{File: goldenName, Qualname: "VALIDATION_WINDOW"}) {
			t.Fatal("a referenced module constant is missing")
		}
	})

	t.Run("a call through this is an own reference and a call through another receiver a member reference [verifies SP-1004~1]", func(t *testing.T) {
		nodes := nodesByQualname(graphOf(t, "m.ts", "class A {\n  m() {\n    return this.n() + other.n() + new ns.B();\n  }\n  n() {\n    return 1;\n  }\n}\n"))
		calls := nodes["A.m"].Calls
		for _, want := range []footprint.Reference{
			{Name: "n", Kind: footprint.ReferenceOwn},
			{Name: "n", Kind: footprint.ReferenceMember},
			{Name: "B", Kind: footprint.ReferenceMember},
		} {
			if !slices.Contains(calls, want) {
				t.Errorf("%+v missing from %+v", want, calls)
			}
		}
	})

	t.Run("a renamed static import is referenced by its exported name [verifies SP-1004~1]", func(t *testing.T) {
		source := "import { a as b, c } from './x';\nimport * as ns from './y';\nimport d from './z';\nfunction f() {\n  return b() + c() + ns.g() + d();\n}\n"
		node := nodesByQualname(graphOf(t, "m.ts", source))["f"]
		for _, want := range []footprint.Reference{
			{Name: "a", Kind: footprint.ReferencePlain},
			{Name: "c", Kind: footprint.ReferencePlain},
			{Name: "d", Kind: footprint.ReferencePlain},
			{Name: "g", Kind: footprint.ReferenceMember},
		} {
			if !slices.Contains(node.Calls, want) {
				t.Errorf("%+v missing from %+v", want, node.Calls)
			}
		}
		if slices.Contains(node.Calls, footprint.Reference{Name: "b", Kind: footprint.ReferencePlain}) {
			t.Errorf("the local alias is referenced: %+v", node.Calls)
		}
		if !slices.Contains(node.Reads, "b") || !slices.Contains(node.Reads, "a") {
			t.Errorf("reads = %v", node.Reads)
		}
	})

	t.Run("the graph resolves a renamed import to the definition of another file [verifies SP-1004~1]", func(t *testing.T) {
		caller := graphOf(t, "caller.ts", "import { price as cost } from './tariff';\nexport function total() {\n  return cost();\n}\n")
		callee := graphOf(t, "tariff.ts", "export function price() {\n  return 1;\n}\n")
		closed := footprint.Close([]footprint.FileGraph{caller, callee}, []footprint.Ref{{File: "caller.ts", Qualname: "total"}})
		if !closed.ContainsDefinition(footprint.Ref{File: "tariff.ts", Qualname: "price"}) {
			t.Fatalf("footprint = %+v", closed)
		}
	})

	t.Run("a source with a syntax error has no graph [verifies SP-1004~1]", func(t *testing.T) {
		if _, err := New().CallGraph(mutation.SourceFile{Path: "m.ts", Content: []byte("function f(:")}); err == nil {
			t.Fatal("no error")
		}
	})
}
