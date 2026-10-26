package spec_test

import (
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
)

func TestExampleText(t *testing.T) {
	t.Run("an example is written as the spec writes it, its arguments in name order [verifies SP-2421~1]", func(t *testing.T) {
		for want, example := range map[string]spec.Example{
			"ticket_price(age=65) => 8": {Call: spec.Call{Operation: "ticket_price", Args: map[string]spec.Literal{"age": {Type: spec.TypeInteger, Value: "65"}}}, Expect: spec.Observation{Kind: spec.Returns, Type: spec.TypeInteger, Value: "8"}},
			`label(code="a\"b\\c", on=true) => null`: {Call: spec.Call{Operation: "label", Args: map[string]spec.Literal{
				"on": {Type: spec.TypeBoolean, Value: "true"}, "code": {Type: spec.TypeString, Value: `a"b\c`},
			}}, Expect: spec.Observation{Kind: spec.Returns, Type: spec.TypeNull}},
			"quote(entry=2026-03-02T10:00) => raises ValueError": {Call: spec.Call{Operation: "quote", Args: map[string]spec.Literal{"entry": {Type: spec.TypeDatetime, Value: "2026-03-02T10:00"}}}, Expect: spec.Observation{Kind: spec.Raises, Value: "ValueError"}},
			"total() => 0.50": {Call: spec.Call{Operation: "total"}, Expect: spec.Observation{Kind: spec.Returns, Type: spec.TypeDecimal, Value: "0.50"}},
		} {
			if got := example.String(); got != want {
				t.Errorf("got %s, want %s", got, want)
			}
		}
	})

	t.Run("a criterion finds its example by index [verifies SP-2421~1]", func(t *testing.T) {
		criterion := spec.Criterion{Examples: []spec.Example{{Index: 1, Call: spec.Call{Operation: "f"}}, {Index: 2, Call: spec.Call{Operation: "g"}}}}
		if example, found := criterion.Example(2); !found || example.Call.Operation != "g" {
			t.Errorf("example %+v, found %t", example, found)
		}
		if _, found := criterion.Example(3); found {
			t.Error("an absent index was found")
		}
	})
}
