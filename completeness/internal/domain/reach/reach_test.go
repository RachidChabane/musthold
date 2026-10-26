package reach_test

import (
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/completeness/internal/domain/code"
	"github.com/RachidChabane/musthold/completeness/internal/domain/reach"
)

func function(file, qualname, owner string, mentions ...string) code.Unit {
	name := qualname
	if owner != "" {
		name = qualname[len(owner)+1:]
	}
	return code.Unit{File: file, Qualname: qualname, Name: name, Owner: owner, Kind: code.KindFunction, Mentions: mentions}
}

func assignment(file, name string, mentions ...string) code.Unit {
	return code.Unit{File: file, Qualname: name, Name: name, Kind: code.KindAssignment, Mentions: mentions}
}

func keys(units ...code.Unit) []string {
	found := make([]string, len(units))
	for index, unit := range units {
		found[index] = unit.Key()
	}
	slices.Sort(found)
	return found
}

func sortedKeys(reached map[string]bool) []string {
	found := make([]string, 0, len(reached))
	for key := range reached {
		found = append(found, key)
	}
	slices.Sort(found)
	return found
}

func TestReached(t *testing.T) {
	entry := function("a.py", "entry", "", "helper", "RATE")
	helper := function("a.py", "helper", "", "deep")
	deep := function("b.py", "deep", "")
	rate := assignment("a.py", "RATE", "base")
	base := assignment("a.py", "base")
	unrelated := function("a.py", "unrelated", "")
	graph := reach.NewGraph([]code.Unit{entry, helper, deep, rate, base, unrelated})

	t.Run("the units an entry mentions are reached transitively, across files and through assignments [verifies SP-508~1]", func(t *testing.T) {
		got := sortedKeys(graph.Reached([]string{"entry"}))
		if want := keys(entry, helper, deep, rate, base); !slices.Equal(got, want) {
			t.Fatalf("reached %v, want %v", got, want)
		}
	})

	t.Run("a unit nobody mentions is not reached [verifies SP-508~1]", func(t *testing.T) {
		if graph.Reached([]string{"entry"})[unrelated.Key()] {
			t.Fatal("an unrelated unit was reached")
		}
		if got := graph.Reached([]string{"missing"}); len(got) != 0 {
			t.Fatalf("an unknown name reached %v", got)
		}
		if got := graph.Reached(nil); len(got) != 0 {
			t.Fatalf("no entry reached %v", got)
		}
	})

	t.Run("instantiating a class reaches its constructor and a method is reached by its own name [verifies SP-508~1]", func(t *testing.T) {
		constructor := function("c.py", "Cart.__init__", "Cart", "items")
		method := function("c.py", "Cart.total", "Cart")
		other := function("c.py", "Cart.items", "Cart")
		cart := reach.NewGraph([]code.Unit{constructor, method, other})
		if got := sortedKeys(cart.Reached([]string{"Cart"})); !slices.Equal(got, keys(constructor, other)) {
			t.Fatalf("instantiation reached %v", got)
		}
		if got := sortedKeys(cart.Reached([]string{"total"})); !slices.Equal(got, keys(method)) {
			t.Fatalf("a method call reached %v", got)
		}
	})

	t.Run("a cycle ends and reaches each unit once [verifies SP-508~1]", func(t *testing.T) {
		ping := function("d.py", "ping", "", "pong")
		pong := function("d.py", "pong", "", "ping")
		if got := sortedKeys(reach.NewGraph([]code.Unit{ping, pong}).Reached([]string{"ping"})); !slices.Equal(got, keys(ping, pong)) {
			t.Fatalf("reached %v", got)
		}
	})

	t.Run("reachability grows with the entries and is the union over them [verifies SP-508~1]", func(t *testing.T) {
		units := []code.Unit{entry, helper, deep, rate, base, unrelated}
		rapid.Check(t, func(t *rapid.T) {
			names := []string{"entry", "helper", "deep", "RATE", "base", "unrelated", "missing"}
			first := rapid.SliceOf(rapid.SampledFrom(names)).Draw(t, "first")
			second := rapid.SliceOf(rapid.SampledFrom(names)).Draw(t, "second")
			g := reach.NewGraph(units)
			union := g.Reached(slices.Concat(first, second))
			a, b := g.Reached(first), g.Reached(second)
			for key := range a {
				if !union[key] {
					t.Fatalf("%s reached by the first entries is missing from the union", key)
				}
			}
			for key := range union {
				if !a[key] && !b[key] {
					t.Fatalf("%s is in the union only", key)
				}
			}
		})
	})
}
