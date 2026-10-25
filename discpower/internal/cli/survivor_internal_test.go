package cli

import (
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/prove"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/format"
)

const survivorDigest = "10edb8fdacce9a3f00000000000000000000000000000000000000000000ffff"

func TestSurvivorLines(t *testing.T) {
	index := 1
	example := spec.Example{Index: 1, Call: spec.Call{Operation: "ticket_price", Args: map[string]spec.Literal{"age": {Type: spec.TypeInteger, Value: "65"}}},
		Expect: spec.Observation{Kind: spec.Returns, Type: spec.TypeInteger, Value: "8"}}

	t.Run("a survivor that fails an example names its reference, its slot and the example as the spec writes it [verifies SP-2421~1]", func(t *testing.T) {
		lines := survivorLines(prove.Survivor{Mutant: survivorDigest, Counterexample: format.Counterexample{Origin: format.OriginTargeted, Slot: format.SlotBoundary, Example: &index}, Example: &example})
		want := "  survivor 10edb8fdacce: a targeted mutant of slot S2 (boundary), which every test of the criterion passes, fails its example 1: ticket_price(age=65) => 8"
		if len(lines) != 1 || lines[0] != want {
			t.Errorf("lines = %q\nwant %q", lines, want)
		}
	})

	t.Run("a survivor that fails a witness names its family and shows the witness, indented [verifies SP-2421~1]", func(t *testing.T) {
		witness := format.BoundedWitness("free_from_fifty", "reaches 50", "def free_from_fifty():\n    assert shipping_fee(50) == 0\n")
		lines := survivorLines(prove.Survivor{Mutant: survivorDigest, Counterexample: format.Counterexample{Origin: format.OriginOperator, Family: "relational", Witness: &witness}})
		want := []string{
			`  survivor 10edb8fdacce: an operator mutant of family relational, which every test of the criterion passes, fails the witness free_from_fifty, which checks "reaches 50":`,
			"    def free_from_fifty():",
			"        assert shipping_fee(50) == 0",
		}
		if strings.Join(lines, "\n") != strings.Join(want, "\n") {
			t.Errorf("lines =\n%s\nwant\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
		}
	})

	t.Run("a witness cut at the bound of a host view says so [verifies SP-2421~1]", func(t *testing.T) {
		witness := format.BoundedWitness("w", "c", strings.Repeat("assert True\n", format.SourceViewCap))
		lines := survivorLines(prove.Survivor{Mutant: survivorDigest, Counterexample: format.Counterexample{Origin: format.OriginOperator, Family: "relational", Witness: &witness}})
		if last := lines[len(lines)-1]; last != "    (the witness is cut at 49152 bytes)" {
			t.Errorf("last line %q", last)
		}
		if shown := strings.Join(lines[1:len(lines)-1], "\n"); len(shown) > format.SourceViewCap+len(lines)*len(survivorIndent) {
			t.Errorf("%d bytes of witness were shown", len(shown))
		}
	})
}
