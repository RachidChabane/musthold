package footprint

import (
	"slices"
	"testing"
)

func graphs() []FileGraph {
	return []FileGraph{
		{
			File: "pkg/fees.py",
			Nodes: []Node{
				{Qualname: "charge", Name: "charge", Calls: []Reference{{Name: "breakdown", Kind: ReferenceMember}, {Name: "Tariff", Kind: ReferencePlain}}},
				{Qualname: "Tariff.__init__", Name: "__init__", Owner: "Tariff", Aliases: []string{"Tariff"}, Reads: []string{"HOURLY_RATE"}},
				{Qualname: "Tariff.breakdown", Name: "breakdown", Owner: "Tariff", Calls: []Reference{{Name: "_free", Kind: ReferencePlain}, {Name: "_round", Kind: ReferenceOwn}}, Reads: []string{"GRACE"}},
				{Qualname: "Tariff._round", Name: "_round", Owner: "Tariff"},
				{Qualname: "_free", Name: "_free", Reads: []string{"WINDOW"}},
				{Qualname: "unrelated", Name: "unrelated", Reads: []string{"OTHER"}},
			},
			Assignments: []Assignment{{Name: "HOURLY_RATE", Qualname: "HOURLY_RATE"}, {Name: "GRACE", Qualname: "GRACE"}, {Name: "OTHER", Qualname: "OTHER"}},
		},
		{
			File:        "pkg/limits.py",
			Nodes:       []Node{{Qualname: "cap", Name: "cap"}},
			Assignments: []Assignment{{Name: "WINDOW", Qualname: "WINDOW"}},
		},
	}
}

func TestClose(t *testing.T) {
	entry := []Ref{{File: "pkg/fees.py", Qualname: "charge"}}

	t.Run("calls are followed through plain, member and own references and constructors [verifies SP-90~1]", func(t *testing.T) {
		got := Close(graphs(), entry)
		want := []Ref{
			{File: "pkg/fees.py", Qualname: "Tariff.__init__"},
			{File: "pkg/fees.py", Qualname: "Tariff._round"},
			{File: "pkg/fees.py", Qualname: "Tariff.breakdown"},
			{File: "pkg/fees.py", Qualname: "_free"},
			{File: "pkg/fees.py", Qualname: "charge"},
		}
		if !slices.Equal(got.Definitions, want) {
			t.Fatalf("definitions = %v", got.Definitions)
		}
	})

	t.Run("module-level assignments read by the footprint are included across files [verifies SP-90~1]", func(t *testing.T) {
		got := Close(graphs(), entry)
		want := []Ref{
			{File: "pkg/fees.py", Qualname: "GRACE"},
			{File: "pkg/fees.py", Qualname: "HOURLY_RATE"},
			{File: "pkg/limits.py", Qualname: "WINDOW"},
		}
		if !slices.Equal(got.Assignments, want) {
			t.Fatalf("assignments = %v", got.Assignments)
		}
		if got.ContainsAssignment(Ref{File: "pkg/fees.py", Qualname: "OTHER"}) || got.ContainsDefinition(Ref{File: "pkg/fees.py", Qualname: "unrelated"}) {
			t.Fatal("unreached members are in the footprint")
		}
		if !slices.Equal(got.Files(), []string{"pkg/fees.py", "pkg/limits.py"}) {
			t.Fatalf("files = %v", got.Files())
		}
	})

	t.Run("an entry nested in a node designates the node [verifies SP-90~1]", func(t *testing.T) {
		got := Close(graphs(), []Ref{{File: "pkg/fees.py", Qualname: "_free.inner"}})
		if !got.ContainsDefinition(Ref{File: "pkg/fees.py", Qualname: "_free"}) || !got.ContainsAssignment(Ref{File: "pkg/limits.py", Qualname: "WINDOW"}) {
			t.Fatalf("footprint = %+v", got)
		}
	})

	t.Run("an unknown entry gives an empty footprint [verifies SP-90~1]", func(t *testing.T) {
		got := Close(graphs(), []Ref{{File: "nope.py", Qualname: "f"}})
		if len(got.Definitions) != 0 || len(got.Assignments) != 0 || len(got.Files()) != 0 {
			t.Fatalf("footprint = %+v", got)
		}
	})

	t.Run("a cycle terminates and the closure is stable [verifies SP-90~1]", func(t *testing.T) {
		cyclic := []FileGraph{{File: "a.py", Nodes: []Node{
			{Qualname: "f", Name: "f", Calls: []Reference{{Name: "g", Kind: ReferencePlain}}},
			{Qualname: "g", Name: "g", Calls: []Reference{{Name: "f", Kind: ReferencePlain}}},
		}}}
		first := Close(cyclic, []Ref{{File: "a.py", Qualname: "f"}})
		second := Close(cyclic, first.Definitions)
		if len(first.Definitions) != 2 || !slices.Equal(first.Definitions, second.Definitions) {
			t.Fatalf("closure = %v then %v", first.Definitions, second.Definitions)
		}
	})
}
