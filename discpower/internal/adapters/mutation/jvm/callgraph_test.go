package jvm

import (
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/footprint"
)

const javaGraphSource = `class Tariff {
    static final int HOURLY = 3;
    static final int NIGHT = 9;
    private final int base;

    Tariff(int base) {
        this.base = base;
    }

    int charge(int hours) {
        return Rates.perHour(hours) + surcharge(hours) + this.fee() + HOURLY + Config.CAP;
    }

    int surcharge(int hours) {
        return hours * NIGHT;
    }

    int fee() {
        return base;
    }

    int unrelated() {
        return NIGHT;
    }

    class Inner {
        int n() {
            return fee();
        }
    }
}

class Rates {
    static int perHour(int hours) {
        return hours;
    }
}

class Checkout {
    Tariff open() {
        return new Tariff(1);
    }
}
`

const kotlinGraphSource = `const val HOURLY = 3
const val NIGHT = 9

fun entry(hours: Int): Int {
    return Rates.perHour(hours) + helper(hours) + HOURLY + Config.CAP
}

fun helper(hours: Int): Int = hours * 2

object Rates {
    fun perHour(hours: Int): Int = hours
}

class Tariff {
    val base = 1

    fun charge(hours: Int): Int {
        return this.fee() + surcharge(hours)
    }

    fun fee(): Int = base

    fun surcharge(hours: Int): Int = hours * NIGHT

    companion object {
        const val CAP = 100
        fun make(): Tariff = Tariff()
    }
}
`

func graphOf(t *testing.T, path, source string) footprint.FileGraph {
	t.Helper()
	graph, err := New().CallGraph(sourceFile(path, source))
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

func reference(name string, kind footprint.ReferenceKind) footprint.Reference {
	return footprint.Reference{Name: name, Kind: kind}
}

func TestJavaCallGraph(t *testing.T) {
	graph := graphOf(t, "src/Tariff.java", javaGraphSource)
	nodes := nodesByQualname(graph)

	t.Run("a node per definition carries owner and name, nested units included [verifies SP-1103~1]", func(t *testing.T) {
		want := []string{"Tariff.Tariff", "Tariff.charge", "Tariff.surcharge", "Tariff.fee", "Tariff.unrelated", "Tariff.Inner.n", "Rates.perHour", "Checkout.open"}
		var got []string
		for _, n := range graph.Nodes {
			got = append(got, n.Qualname)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("nodes %v", got)
		}
		if n := nodes["Tariff.Inner.n"]; n.Owner != "Tariff.Inner" || n.Name != "n" {
			t.Fatalf("nested node %+v", n)
		}
	})

	t.Run("a static call is a member call of the callee name [verifies SP-1103~1]", func(t *testing.T) {
		if !slices.Contains(nodes["Tariff.charge"].Calls, reference("perHour", footprint.ReferenceMember)) {
			t.Fatalf("calls %+v", nodes["Tariff.charge"].Calls)
		}
	})

	t.Run("an unqualified call is plain and own, a call through this is own [verifies SP-1103~1]", func(t *testing.T) {
		calls := nodes["Tariff.charge"].Calls
		for _, want := range []footprint.Reference{
			reference("surcharge", footprint.ReferencePlain), reference("surcharge", footprint.ReferenceOwn), reference("fee", footprint.ReferenceOwn),
		} {
			if !slices.Contains(calls, want) {
				t.Errorf("%+v missing from %+v", want, calls)
			}
		}
		if slices.Contains(calls, reference("fee", footprint.ReferenceMember)) {
			t.Error("a call through this is a member call")
		}
	})

	t.Run("a constructor is reached through its class name alias [verifies SP-1103~1]", func(t *testing.T) {
		if n := nodes["Tariff.Tariff"]; n.Owner != "Tariff" || !slices.Equal(n.Aliases, []string{"Tariff"}) {
			t.Fatalf("constructor %+v", n)
		}
		if !slices.Contains(nodes["Checkout.open"].Calls, reference("Tariff", footprint.ReferencePlain)) {
			t.Fatalf("calls %+v", nodes["Checkout.open"].Calls)
		}
		if len(nodes["Tariff.charge"].Aliases) != 0 {
			t.Fatal("a method has no alias")
		}
	})

	t.Run("constants are read by bare and qualified names [verifies SP-1103~1]", func(t *testing.T) {
		reads := nodes["Tariff.charge"].Reads
		for _, want := range []string{"HOURLY", "CAP", "hours"} {
			if !slices.Contains(reads, want) {
				t.Errorf("%s missing from %v", want, reads)
			}
		}
		var names []string
		for _, a := range graph.Assignments {
			names = append(names, a.Qualname)
		}
		if !slices.Equal(names, []string{"Tariff.HOURLY", "Tariff.NIGHT"}) {
			t.Fatalf("assignments %v", names)
		}
	})

	t.Run("the footprint of an entry follows static and instance calls and the constants they read [verifies SP-1103~1]", func(t *testing.T) {
		const file = "src/Tariff.java"
		closed := footprint.Close([]footprint.FileGraph{graph}, []footprint.Ref{{File: file, Qualname: "Checkout.open"}, {File: file, Qualname: "Tariff.charge"}})
		for _, qualname := range []string{"Checkout.open", "Tariff.Tariff", "Tariff.charge", "Rates.perHour", "Tariff.surcharge", "Tariff.fee"} {
			if !closed.ContainsDefinition(footprint.Ref{File: file, Qualname: qualname}) {
				t.Errorf("%s missing from %v", qualname, closed.Definitions)
			}
		}
		if closed.ContainsDefinition(footprint.Ref{File: file, Qualname: "Tariff.unrelated"}) {
			t.Error("an unreached method is in the footprint")
		}
		if !closed.ContainsAssignment(footprint.Ref{File: file, Qualname: "Tariff.HOURLY"}) || !closed.ContainsAssignment(footprint.Ref{File: file, Qualname: "Tariff.NIGHT"}) {
			t.Errorf("constants missing from %v", closed.Assignments)
		}
	})

	t.Run("a call from a nested class reaches the method of the enclosing class [verifies SP-1103~1]", func(t *testing.T) {
		const file = "src/Tariff.java"
		closed := footprint.Close([]footprint.FileGraph{graph}, []footprint.Ref{{File: file, Qualname: "Tariff.Inner.n"}})
		if !closed.ContainsDefinition(footprint.Ref{File: file, Qualname: "Tariff.fee"}) {
			t.Fatalf("footprint %v", closed.Definitions)
		}
	})

	t.Run("a definition nested in another belongs to it [verifies SP-1103~1]", func(t *testing.T) {
		source := "class T {\n  int m() {\n    Runnable r = new Runnable() { public void run() { helper(); } };\n    return 1;\n  }\n  void helper() {}\n}\n"
		nodes := nodesByQualname(graphOf(t, javaPath, source))
		if len(nodes) != 2 || !slices.Contains(nodes["T.m"].Calls, reference("helper", footprint.ReferenceOwn)) {
			t.Fatalf("nodes %+v", nodes)
		}
	})
}

func TestKotlinCallGraph(t *testing.T) {
	graph := graphOf(t, "src/Tariff.kt", kotlinGraphSource)
	nodes := nodesByQualname(graph)

	t.Run("a node per function carries owner and name [verifies SP-1103~1]", func(t *testing.T) {
		want := []string{"entry", "helper", "Rates.perHour", "Tariff.charge", "Tariff.fee", "Tariff.surcharge", "Tariff.make"}
		var got []string
		for _, n := range graph.Nodes {
			got = append(got, n.Qualname)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("nodes %v", got)
		}
		if n := nodes["Tariff.make"]; n.Owner != "Tariff" || n.Name != "make" {
			t.Fatalf("companion node %+v", n)
		}
	})

	t.Run("a static call is a member call and a top-level call is plain [verifies SP-1103~1]", func(t *testing.T) {
		calls := nodes["entry"].Calls
		for _, want := range []footprint.Reference{reference("perHour", footprint.ReferenceMember), reference("helper", footprint.ReferencePlain)} {
			if !slices.Contains(calls, want) {
				t.Errorf("%+v missing from %+v", want, calls)
			}
		}
	})

	t.Run("a call through this is own and an unqualified call is plain and own [verifies SP-1103~1]", func(t *testing.T) {
		calls := nodes["Tariff.charge"].Calls
		for _, want := range []footprint.Reference{
			reference("fee", footprint.ReferenceOwn), reference("surcharge", footprint.ReferencePlain), reference("surcharge", footprint.ReferenceOwn),
		} {
			if !slices.Contains(calls, want) {
				t.Errorf("%+v missing from %+v", want, calls)
			}
		}
	})

	t.Run("a constructor call is a plain call of the class name [verifies SP-1103~1]", func(t *testing.T) {
		if !slices.Contains(nodes["Tariff.make"].Calls, reference("Tariff", footprint.ReferencePlain)) {
			t.Fatalf("calls %+v", nodes["Tariff.make"].Calls)
		}
	})

	t.Run("constants are read by bare and qualified names [verifies SP-1103~1]", func(t *testing.T) {
		reads := nodes["entry"].Reads
		for _, want := range []string{"HOURLY", "CAP"} {
			if !slices.Contains(reads, want) {
				t.Errorf("%s missing from %v", want, reads)
			}
		}
		var names []string
		for _, a := range graph.Assignments {
			names = append(names, a.Qualname)
		}
		if !slices.Equal(names, []string{"HOURLY", "NIGHT", "Tariff.base", "Tariff.CAP"}) {
			t.Fatalf("assignments %v", names)
		}
	})

	t.Run("the footprint follows calls and the constants they read [verifies SP-1103~1]", func(t *testing.T) {
		const file = "src/Tariff.kt"
		closed := footprint.Close([]footprint.FileGraph{graph}, []footprint.Ref{{File: file, Qualname: "entry"}, {File: file, Qualname: "Tariff.charge"}})
		for _, qualname := range []string{"entry", "helper", "Rates.perHour", "Tariff.charge", "Tariff.fee", "Tariff.surcharge"} {
			if !closed.ContainsDefinition(footprint.Ref{File: file, Qualname: qualname}) {
				t.Errorf("%s missing from %v", qualname, closed.Definitions)
			}
		}
		if closed.ContainsDefinition(footprint.Ref{File: file, Qualname: "Tariff.make"}) {
			t.Error("an unreached function is in the footprint")
		}
		for _, qualname := range []string{"HOURLY", "NIGHT", "Tariff.CAP", "Tariff.base"} {
			if !closed.ContainsAssignment(footprint.Ref{File: file, Qualname: qualname}) {
				t.Errorf("%s missing from %v", qualname, closed.Assignments)
			}
		}
	})

	t.Run("a local function and an object literal belong to the enclosing function [verifies SP-1103~1]", func(t *testing.T) {
		source := "fun m(): Int {\n  fun inner() = helper()\n  val o = object : Runnable { override fun run() { other() } }\n  return inner()\n}\nfun helper() = 1\nfun other() = 2\n"
		nodes := nodesByQualname(graphOf(t, kotlinPath, source))
		if len(nodes) != 3 || !slices.Contains(nodes["m"].Calls, reference("helper", footprint.ReferencePlain)) || !slices.Contains(nodes["m"].Calls, reference("other", footprint.ReferencePlain)) {
			t.Fatalf("nodes %+v", nodes)
		}
	})
}
