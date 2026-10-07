package differential_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/differential"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
)

const (
	verifiesSP170 = " [verifies SP-170~1]"
	verifiesSP171 = " [verifies SP-171~1]"

	operation   = "price"
	seed        = uint64(42)
	manyDraws   = 500
	otherSeed   = uint64(7)
	maxParams   = 3
	maxSources  = 4
	smallDraws  = 12
	boolOptions = 4
)

func integerParam(name string) differential.Param {
	return differential.Param{Name: name, Hint: differential.HintInteger}
}

func signatureOf(params ...differential.Param) differential.Signature {
	return differential.Signature{Operation: operation, Params: params}
}

func exampleCall(args map[string]spec.Literal) spec.Call {
	return spec.Call{Operation: operation, Args: args}
}

func integer(value string) spec.Literal { return spec.Literal{Type: spec.TypeInteger, Value: value} }

func key(call spec.Call) string {
	encoded, err := json.Marshal(call)
	if err != nil {
		return call.Operation
	}
	return string(encoded)
}

func valuesOf(calls []spec.Call, name string) []string {
	var values []string
	for _, call := range calls {
		values = append(values, call.Args[name].Value)
	}
	return values
}

func distinctValues(calls []spec.Call, name string) []string {
	var values []string
	for _, value := range valuesOf(calls, name) {
		if !slices.Contains(values, value) {
			values = append(values, value)
		}
	}
	return values
}

func TestDrawDeterminism(t *testing.T) {
	sources := []string{"if amount > 100: return 0.5", `label = "gold"`}
	examples := []spec.Call{exampleCall(map[string]spec.Literal{"amount": integer("7"), "tier": {Type: spec.TypeString, Value: "silver"}})}
	signature := signatureOf(integerParam("amount"), differential.Param{Name: "tier", Hint: differential.HintString})
	t.Run("the same pool, signature and seed give the same inputs"+verifiesSP170, func(t *testing.T) {
		first := differential.NewPool(sources, examples).Draw(signature, seed, differential.DefaultDraws)
		second := differential.NewPool(sources, examples).Draw(signature, seed, differential.DefaultDraws)
		if len(first) == 0 || !slices.EqualFunc(first, second, func(a, b spec.Call) bool { return key(a) == key(b) }) {
			t.Error("the draws differ between two runs")
		}
	})
	t.Run("the order of a draw does not depend on a previous draw from the pool"+verifiesSP170, func(t *testing.T) {
		pool := differential.NewPool(sources, examples)
		first := pool.Draw(signature, seed, differential.DefaultDraws)
		pool.Draw(signatureOf(integerParam("other")), otherSeed, smallDraws)
		second := pool.Draw(signature, seed, differential.DefaultDraws)
		if !slices.EqualFunc(first, second, func(a, b spec.Call) bool { return key(a) == key(b) }) {
			t.Error("a draw changed the pool")
		}
	})
	t.Run("another seed gives other random combinations after the same sweep"+verifiesSP170, func(t *testing.T) {
		pool := differential.NewPool(sources, examples)
		first := pool.Draw(signature, seed, differential.DefaultDraws)
		second := pool.Draw(signature, otherSeed, differential.DefaultDraws)
		same := slices.EqualFunc(first, second, func(a, b spec.Call) bool { return key(a) == key(b) })
		if same {
			t.Error("two seeds drew the same inputs")
		}
		sweep := len(distinctValues(pool.Draw(signatureOf(integerParam("amount")), seed, manyDraws), "amount"))
		for index := range sweep {
			if key(first[index]) != key(second[index]) {
				t.Fatalf("the sweep differs at %d", index)
			}
		}
	})
	t.Run("another operation name draws another random stream"+verifiesSP170, func(t *testing.T) {
		pool := differential.NewPool(sources, examples)
		renamed := signature
		renamed.Operation = "refund"
		first := pool.Draw(signature, seed, differential.DefaultDraws)
		second := pool.Draw(renamed, seed, differential.DefaultDraws)
		if len(first) != len(second) {
			t.Fatalf("the counts differ: %d and %d", len(first), len(second))
		}
		for index := range second {
			if second[index].Operation != "refund" {
				t.Fatalf("the call %d names %q", index, second[index].Operation)
			}
		}
	})
}

func TestDrawLimits(t *testing.T) {
	pool := differential.NewPool([]string{"limit = 50"}, nil)
	signature := signatureOf(integerParam("a"), integerParam("b"))
	for _, draws := range []int{1, 2, 5, 13, differential.DefaultDraws} {
		t.Run(fmt.Sprintf("a request for %d inputs never", draws)+" exceeds the number asked and never repeats"+verifiesSP170, func(t *testing.T) {
			calls := pool.Draw(signature, seed, draws)
			if len(calls) == 0 || len(calls) > draws {
				t.Fatalf("%d calls for %d asked", len(calls), draws)
			}
			seen := map[string]bool{}
			for _, call := range calls {
				if seen[key(call)] {
					t.Fatalf("the call %s is repeated", key(call))
				}
				seen[key(call)] = true
			}
		})
	}
	for _, draws := range []int{0, -1} {
		t.Run("a request for no input gives none"+verifiesSP170, func(t *testing.T) {
			if got := pool.Draw(signature, seed, draws); got != nil {
				t.Errorf("got %v", got)
			}
		})
	}
	t.Run("an operation without parameter gives no input"+verifiesSP170, func(t *testing.T) {
		if got := pool.Draw(signatureOf(), seed, differential.DefaultDraws); got != nil {
			t.Errorf("got %v", got)
		}
	})
	t.Run("a parameter with no candidate gives no input"+verifiesSP170, func(t *testing.T) {
		datetime := differential.Param{Name: "when", Hint: differential.HintDatetime}
		if got := pool.Draw(signatureOf(datetime), seed, differential.DefaultDraws); got != nil {
			t.Errorf("got %v", got)
		}
	})
	t.Run("a datetime parameter draws the datetimes of the examples"+verifiesSP170, func(t *testing.T) {
		moment := spec.Literal{Type: spec.TypeDatetime, Value: "2026-01-01T00:00:00Z"}
		withExample := differential.NewPool(nil, []spec.Call{exampleCall(map[string]spec.Literal{"when": moment})})
		calls := withExample.Draw(signatureOf(differential.Param{Name: "when", Hint: differential.HintDatetime}), seed, differential.DefaultDraws)
		if len(calls) != 1 || calls[0].Args["when"] != moment {
			t.Errorf("got %v", calls)
		}
	})
	t.Run("a finite input space is exhausted without repeats"+verifiesSP170, func(t *testing.T) {
		booleans := differential.NewPool(nil, nil).Draw(signatureOf(
			differential.Param{Name: "x", Hint: differential.HintBoolean},
			differential.Param{Name: "y", Hint: differential.HintBoolean}), seed, differential.DefaultDraws)
		if len(booleans) != boolOptions {
			t.Errorf("got %d calls, want %d", len(booleans), boolOptions)
		}
	})
	t.Run("every drawn call names the operation and every parameter"+verifiesSP170, func(t *testing.T) {
		for _, call := range pool.Draw(signature, seed, differential.DefaultDraws) {
			if call.Operation != operation || len(call.Args) != 2 {
				t.Fatalf("the call %s is incomplete", key(call))
			}
		}
	})
}

func TestDrawBoundaries(t *testing.T) {
	t.Run("an integer in the code is drawn on both sides"+verifiesSP170, func(t *testing.T) {
		pool := differential.NewPool([]string{"if quantity >= 42:"}, nil)
		values := valuesOf(pool.Draw(signatureOf(integerParam("quantity")), seed, differential.DefaultDraws), "quantity")
		for _, want := range []string{"41", "42", "43"} {
			if !slices.Contains(values, want) {
				t.Errorf("%s is missing from %v", want, values)
			}
		}
	})
	t.Run("a decimal in the code is drawn with its neighbours of one unit and one step"+verifiesSP170, func(t *testing.T) {
		pool := differential.NewPool([]string{"rate = 2.50"}, nil)
		values := valuesOf(pool.Draw(signatureOf(differential.Param{Name: "rate", Hint: differential.HintDecimal}), seed, manyDraws), "rate")
		for _, want := range []string{"2.50", "2.51", "2.49", "3.50", "1.50"} {
			if !slices.Contains(values, want) {
				t.Errorf("%s is missing from %v", want, values)
			}
		}
	})
	t.Run("the basic values are always drawn"+verifiesSP170, func(t *testing.T) {
		values := valuesOf(differential.NewPool(nil, nil).Draw(signatureOf(integerParam("n")), seed, manyDraws), "n")
		for _, want := range []string{"0", "1", "-1", "2", "10", "100"} {
			if !slices.Contains(values, want) {
				t.Errorf("%s is missing from %v", want, values)
			}
		}
	})
	t.Run("an integer parameter draws integers only"+verifiesSP170, func(t *testing.T) {
		pool := differential.NewPool([]string{"x = 2.5"}, nil)
		for _, call := range pool.Draw(signatureOf(integerParam("n")), seed, manyDraws) {
			if call.Args["n"].Type != spec.TypeInteger {
				t.Fatalf("drew %v", call.Args["n"])
			}
		}
	})
	t.Run("short string literals of the code are drawn, long ones are not"+verifiesSP170, func(t *testing.T) {
		long := "this literal is longer than twenty four characters"
		pool := differential.NewPool([]string{`mode = "fast"` + "\n" + `other = 'slow'` + "\n" + `text = "` + long + `"`}, nil)
		values := valuesOf(pool.Draw(signatureOf(differential.Param{Name: "mode", Hint: differential.HintString}), seed, manyDraws), "mode")
		for _, want := range []string{"", "a", "fast", "slow"} {
			if !slices.Contains(values, want) {
				t.Errorf("%q is missing from %v", want, values)
			}
		}
		if slices.Contains(values, long) {
			t.Error("a long literal was drawn")
		}
	})
	t.Run("the arguments of the examples are drawn first for their parameter, with their neighbours"+verifiesSP170, func(t *testing.T) {
		example := exampleCall(map[string]spec.Literal{"amount": integer("7")})
		calls := differential.NewPool(nil, []spec.Call{example}).Draw(signatureOf(integerParam("amount")), seed, manyDraws)
		values := valuesOf(calls, "amount")
		if values[0] != "7" {
			t.Errorf("the first value is %s", values[0])
		}
		for _, want := range []string{"6", "8"} {
			if !slices.Contains(values, want) {
				t.Errorf("%s is missing from %v", want, values)
			}
		}
	})
	t.Run("the example values of a parameter do not leak to another parameter"+verifiesSP170, func(t *testing.T) {
		example := exampleCall(map[string]spec.Literal{"amount": {Type: spec.TypeString, Value: "special-value"}})
		calls := differential.NewPool(nil, []spec.Call{example}).Draw(signatureOf(differential.Param{Name: "label", Hint: differential.HintString}), seed, manyDraws)
		if slices.Contains(valuesOf(calls, "label"), "special-value") {
			t.Error("a value of another parameter was drawn")
		}
	})
	t.Run("a parameter of unknown type draws numbers and strings"+verifiesSP170, func(t *testing.T) {
		pool := differential.NewPool([]string{`x = 9 if y == "tag" else 3`}, nil)
		calls := pool.Draw(signatureOf(differential.Param{Name: "v", Hint: differential.HintUnknown}), seed, manyDraws)
		var types []spec.LiteralType
		for _, call := range calls {
			if !slices.Contains(types, call.Args["v"].Type) {
				types = append(types, call.Args["v"].Type)
			}
		}
		if !slices.Contains(types, spec.TypeInteger) || !slices.Contains(types, spec.TypeString) {
			t.Errorf("drew only %v", types)
		}
	})
	t.Run("boundary inputs come before random combinations: each parameter is swept while the others stay on their anchor"+verifiesSP170, func(t *testing.T) {
		pool := differential.NewPool([]string{"limit = 50"}, nil)
		first := pool.Draw(signatureOf(integerParam("a")), seed, manyDraws)
		sweep := valuesOf(first, "a")
		calls := pool.Draw(signatureOf(integerParam("a"), integerParam("b")), seed, differential.DefaultDraws)
		anchor := calls[0].Args["b"]
		for index, want := range sweep {
			if calls[index].Args["a"].Value != want || calls[index].Args["b"] != anchor {
				t.Fatalf("call %d is %s, want a=%s with b on its anchor", index, key(calls[index]), want)
			}
		}
	})
	t.Run("the second parameter is swept right after the first"+verifiesSP170, func(t *testing.T) {
		pool := differential.NewPool([]string{"limit = 50"}, nil)
		sweepA := len(pool.Draw(signatureOf(integerParam("a")), seed, manyDraws))
		sweepB := valuesOf(pool.Draw(signatureOf(integerParam("b")), seed, manyDraws), "b")
		calls := pool.Draw(signatureOf(integerParam("a"), integerParam("b")), seed, manyDraws)
		anchorA := calls[0].Args["a"]
		for offset, want := range sweepB[1:] {
			call := calls[sweepA+offset]
			if call.Args["b"].Value != want || call.Args["a"] != anchorA {
				t.Fatalf("call %d is %s, want b=%s with a on its anchor", sweepA+offset, key(call), want)
			}
		}
	})
}

func TestDrawProperties(t *testing.T) {
	hints := []differential.Hint{differential.HintInteger, differential.HintDecimal, differential.HintString, differential.HintBoolean, differential.HintUnknown}
	genSignature := rapid.Custom(func(t *rapid.T) differential.Signature {
		count := rapid.IntRange(1, maxParams).Draw(t, "params")
		params := make([]differential.Param, count)
		for index := range params {
			params[index] = differential.Param{Name: string(rune('a' + index)), Hint: rapid.SampledFrom(hints).Draw(t, "hint")}
		}
		return differential.Signature{Operation: rapid.StringMatching(`[a-z]{1,8}`).Draw(t, "operation"), Params: params}
	})
	genSources := rapid.SliceOfN(rapid.StringMatching(`[a-z0-9 ="'.<>()-]{0,40}`), 0, maxSources)
	t.Run("a draw is deterministic, never exceeds the count and never repeats a call"+verifiesSP170, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			sources := genSources.Draw(t, "sources")
			signature := genSignature.Draw(t, "signature")
			randomSeed := rapid.Uint64().Draw(t, "seed")
			draws := rapid.IntRange(0, differential.DefaultDraws).Draw(t, "draws")
			first := differential.NewPool(sources, nil).Draw(signature, randomSeed, draws)
			second := differential.NewPool(sources, nil).Draw(signature, randomSeed, draws)
			if len(first) > draws {
				t.Fatalf("%d calls for %d asked", len(first), draws)
			}
			seen := map[string]bool{}
			for _, call := range first {
				if seen[key(call)] {
					t.Fatalf("the call %s is repeated", key(call))
				}
				seen[key(call)] = true
			}
			if !slices.EqualFunc(first, second, func(a, b spec.Call) bool { return key(a) == key(b) }) {
				t.Fatal("two draws with the same pool, signature and seed differ")
			}
		})
	})
	t.Run("every number of the code is drawn on both sides for an integer parameter"+verifiesSP170, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			number := rapid.IntRange(2, 9999).Draw(t, "number")
			source := "if x == " + strconv.Itoa(number)
			values := valuesOf(differential.NewPool([]string{source}, nil).Draw(signatureOf(integerParam("x")), seed, manyDraws), "x")
			for _, want := range []int{number - 1, number, number + 1} {
				if !slices.Contains(values, strconv.Itoa(want)) {
					t.Fatalf("%d is missing from %v", want, values)
				}
			}
		})
	})
}
