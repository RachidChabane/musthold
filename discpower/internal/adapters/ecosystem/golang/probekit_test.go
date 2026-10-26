package golang_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/ecosystem/golang"
	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/differential"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
)

const (
	kitFixture   = "testdata/probekit"
	kitPackage   = "example.com/kit:"
	ledgerTarget = "example.com/kit/internal/ledger:Net"
	vaultTarget  = "example.com/kit/nested/internal/vault:Seal"
)

func argumentsOf(pairs ...any) map[string]spec.Literal {
	args := map[string]spec.Literal{}
	for index := 0; index < len(pairs); index += 2 {
		args[pairs[index].(string)] = pairs[index+1].(spec.Literal)
	}
	return args
}

func kitCall(function string, pairs ...any) ports.ProbeCall {
	return ports.ProbeCall{Target: kitPackage + function, Args: argumentsOf(pairs...)}
}

func text(value string) spec.Literal     { return literal(spec.TypeString, value) }
func boolean(value string) spec.Literal  { return literal(spec.TypeBoolean, value) }
func decimal(value string) spec.Literal  { return literal(spec.TypeDecimal, value) }
func datetime(value string) spec.Literal { return literal(spec.TypeDatetime, value) }
func null() spec.Literal                 { return literal(spec.TypeNull, "null") }

// kitCase is a call of the probekit project and what it is observed to do.
type kitCase struct {
	requirement string
	name        string
	call        ports.ProbeCall
	want        spec.Observation
}

// Requirements the cases of the project verify.
const (
	observationRequirement = "SP-951~1"
	argumentRequirement    = "SP-952~1"
	placementRequirement   = "SP-953~1"
	containmentRequirement = "SP-954~1"
)

func returnsInt(value string) spec.Observation { return returnsOf(spec.TypeInteger, value) }

func returnsText(value string) spec.Observation { return returnsOf(spec.TypeString, value) }

func returnsDecimal(value string) spec.Observation { return returnsOf(spec.TypeDecimal, value) }

func returnsDatetime(value string) spec.Observation { return returnsOf(spec.TypeDatetime, value) }

var returnsNull = returnsOf(spec.TypeNull, "null")

// kitCases cover what a call returns or raises and how arguments are decoded.
// The calls that kill or stall the program come last: the calls after them are
// served by a fresh process.
func kitCases() []kitCase {
	return []kitCase{
		{observationRequirement, "an error that is nil leaves the result", kitCall("Sqrt", "value", integer("16")), returnsInt("4")},
		{observationRequirement, "an error variable of the package is named by its variable", kitCall("Sqrt", "value", integer("-1")), raisesOf("ErrNegative")},
		{observationRequirement, "an error of a type is named by its type", kitCall("Clamp", "value", integer("5"), "low", integer("1"), "high", integer("3")), raisesOf("RangeError")},
		{observationRequirement, "a wrapped error variable is named by its variable", kitCall("Limit", "value", integer("11")), raisesOf("ErrLimit")},
		{observationRequirement, "a function that returns only a nil error returns null", kitCall("Limit", "value", integer("3")), returnsNull},
		{observationRequirement, "several results are one string of their texts", kitCall("Divmod", "dividend", integer("7"), "divisor", integer("2")), returnsText(`["3","1"]`)},
		{observationRequirement, "a result of no literal type is its JSON text", kitCall("Records", "count", integer("2")), returnsText(`["r0","r1"]`)},
		{observationRequirement, "a panic with an error is named as that error", kitCall("Refuse", "value", integer("1")), raisesOf("ErrNegative")},
		{observationRequirement, "a panic with a value is a panic", kitCall("Boom", "value", integer("1")), raisesOf("Panic")},
		{argumentRequirement, "an integer literal is a float argument", kitCall("Half", "value", integer("5")), returnsDecimal("2.5")},
		{argumentRequirement, "a decimal literal is a float argument", kitCall("Half", "value", decimal("19.99")), returnsDecimal("9.995")},
		{argumentRequirement, "a boolean and a string literal are read as such", kitCall("Greet", "name", text("ann"), "loud", boolean("true")), returnsText("HELLO ANN")},
		{argumentRequirement, "a datetime without an offset is UTC", kitCall("NextDay", "day", datetime("2026-01-01T10:00:00")), returnsDatetime("2026-01-02T10:00:00+00:00")},
		{argumentRequirement, "a date is midnight UTC", kitCall("NextDay", "day", datetime("2026-01-31")), returnsDatetime("2026-02-01T00:00:00+00:00")},
		{argumentRequirement, "a datetime keeps its offset", kitCall("NextDay", "day", datetime("2026-01-01T10:00:00+02:00")), returnsDatetime("2026-01-02T10:00:00+02:00")},
		{argumentRequirement, "a decimal literal is read by a library number type", kitCall("Scale", "rate", decimal("1.5"), "factor", integer("4")), returnsInt("6")},
		{argumentRequirement, "a fraction a library number type returns keeps its value", kitCall("Scale", "rate", decimal("0.1"), "factor", integer("3")), returnsDecimal("0.3")},
		{argumentRequirement, "null is a nil pointer", kitCall("Fallback", "value", null()), returnsInt("-1")},
		{argumentRequirement, "a value is the pointee of a pointer", kitCall("Fallback", "value", integer("7")), returnsInt("7")},
		{argumentRequirement, "an empty interface takes the natural value of an integer", kitCall("Echo", "value", integer("3")), returnsInt("3")},
		{argumentRequirement, "an empty interface takes the natural value of a decimal", kitCall("Echo", "value", decimal("2.5")), returnsDecimal("2.5")},
		{argumentRequirement, "an empty interface takes the natural value of a string", kitCall("Echo", "value", text("x")), returnsText("x")},
		{argumentRequirement, "an empty interface takes the natural value of a boolean", kitCall("Echo", "value", boolean("true")), returnsOf(spec.TypeBoolean, "true")},
		{argumentRequirement, "an empty interface takes null", kitCall("Echo", "value", null()), returnsNull},
		{argumentRequirement, "a named integer type is an integer", kitCall("Cents", "amount", integer("5")), returnsInt("500")},
		{argumentRequirement, "an unsigned integer wraps as the code does", kitCall("Narrow", "value", integer("255")), returnsInt("0")},
		{argumentRequirement, "an integer too large for its type is an argument error", kitCall("Narrow", "value", integer("256")), raisesOf("ArgumentError")},
		{argumentRequirement, "a variadic parameter is left out", kitCall("Join", "separator", text(",")), returnsText("")},
		{argumentRequirement, "a function without parameters is called without arguments", kitCall("Version"), returnsText("1.0")},
		{argumentRequirement, "a context is supplied and is no parameter", kitCall("Deadline", "seconds", integer("3")), returnsInt("3")},
		{argumentRequirement, "a method of a pointer receiver is called on a zero value", kitCall("Counter.Add", "amount", integer("5")), returnsInt("5")},
		{argumentRequirement, "a method of a value receiver is called on a zero value", kitCall("Counter.Double", "amount", integer("5")), returnsInt("10")},
		{placementRequirement, "a package under an internal directory is reached", ports.ProbeCall{Target: ledgerTarget, Args: argumentsOf("gross", integer("10"), "tax", integer("3"))}, returnsInt("7")},
		{placementRequirement, "its error variable is named", ports.ProbeCall{Target: ledgerTarget, Args: argumentsOf("gross", integer("1"), "tax", integer("3"))}, raisesOf("ErrUnbalanced")},
		{placementRequirement, "a package under an internal directory of a subtree is reached", ports.ProbeCall{Target: vaultTarget, Args: argumentsOf("code", integer("6"))}, returnsInt("42")},
		{containmentRequirement, "a call that exits the program is a crash", kitCall("Quit", "value", integer("1")), raisesOf("Crash")},
		{containmentRequirement, "the call after a crash is answered", kitCall("Sqrt", "value", integer("25")), returnsInt("5")},
		{containmentRequirement, "a call that never ends is a timeout", kitCall("Spin", "value", integer("1")), raisesOf("Timeout")},
		{containmentRequirement, "the call after a timeout is answered", kitCall("Sqrt", "value", integer("36")), returnsInt("6")},
	}
}

func TestProberOverAProjectOfManyTypes(t *testing.T) {
	project := openProject(t, kitFixture)
	cases := kitCases()
	calls := make([]ports.ProbeCall, len(cases))
	for index, tt := range cases {
		calls[index] = tt.call
	}
	got := project.invoke(t, calls...)
	for index, tt := range cases {
		t.Run(fmt.Sprintf("%s [%s %s]", tt.name, "verifies", tt.requirement), func(t *testing.T) {
			requireObservations(t, got[index:index+1], tt.want)
		})
	}

	t.Run("a call whose arguments do not fit its parameters is an argument error and the others are served [verifies SP-952~1]", func(t *testing.T) {
		misfits := project.invoke(t,
			kitCall("Greet", "name", text("ann")),
			kitCall("Greet", "name", text("ann"), "loud", integer("1")),
			kitCall("Greet", "name", null(), "loud", boolean("true")),
			kitCall("NextDay", "day", datetime("last tuesday")),
			kitCall("Scale", "rate", text("many"), "factor", integer("4")),
			kitCall("Half", "value", text("x")),
			kitCall("Sqrt", "value", decimal("2.5")),
			kitCall("Sqrt", "value", integer("9")))
		for index, observation := range misfits[:len(misfits)-1] {
			if observation.Kind != spec.Raises || observation.Value != "ArgumentError" {
				t.Errorf("call %d observed %+v", index, observation)
			}
		}
		requireObservations(t, misfits[len(misfits)-1:], returnsInt("3"))
	})

	t.Run("a call is observed the same whenever it runs [verifies SP-955~1]", func(t *testing.T) {
		repeat := []ports.ProbeCall{kitCall("Roll", "sides", integer("1000000")), kitCall("Zone", "value", integer("0"))}
		first, second := project.invoke(t, repeat...), project.invoke(t, repeat...)
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("%+v then %+v", first, second)
		}
		requireObservations(t, first[1:], returnsText("UTC"))
	})
}

func TestProberSignaturesOverAProjectOfManyTypes(t *testing.T) {
	project := openProject(t, kitFixture)
	prober := project.strategy.Prober()

	t.Run("each kind of parameter has its hint and what the code does not take from a spec is left out [verifies SP-950~1]", func(t *testing.T) {
		targets := []string{
			kitPackage + "Greet", kitPackage + "NextDay", kitPackage + "Scale", kitPackage + "Fallback", kitPackage + "Echo",
			kitPackage + "Cents", kitPackage + "Narrow", kitPackage + "Half", kitPackage + "Join", kitPackage + "Deadline",
			kitPackage + "Version", kitPackage + "Counter.Add", kitPackage + "Records", ledgerTarget,
		}
		got, err := prober.Signatures(context.Background(), project.workspace, project.toolchain, targets)
		if err != nil {
			t.Fatal(err)
		}
		param := func(name string, hint differential.Hint) differential.Param {
			return differential.Param{Name: name, Hint: hint}
		}
		want := map[string][]differential.Param{
			kitPackage + "Greet":       {param("name", differential.HintString), param("loud", differential.HintBoolean)},
			kitPackage + "NextDay":     {param("day", differential.HintDatetime)},
			kitPackage + "Scale":       {param("rate", differential.HintDecimal), param("factor", differential.HintInteger)},
			kitPackage + "Fallback":    {param("value", differential.HintInteger)},
			kitPackage + "Echo":        {param("value", differential.HintUnknown)},
			kitPackage + "Cents":       {param("amount", differential.HintInteger)},
			kitPackage + "Narrow":      {param("value", differential.HintInteger)},
			kitPackage + "Half":        {param("value", differential.HintDecimal)},
			kitPackage + "Join":        {param("separator", differential.HintString)},
			kitPackage + "Deadline":    {param("seconds", differential.HintInteger)},
			kitPackage + "Version":     {},
			kitPackage + "Counter.Add": {param("amount", differential.HintInteger)},
			kitPackage + "Records":     {param("count", differential.HintInteger)},
			ledgerTarget:               {param("gross", differential.HintInteger), param("tax", differential.HintInteger)},
		}
		if len(got) != len(targets) {
			t.Fatalf("%d signatures for %d targets", len(got), len(targets))
		}
		for _, signature := range got {
			if !reflect.DeepEqual(signature.Params, want[signature.Target]) {
				t.Errorf("%s: %+v, want %+v", signature.Target, signature.Params, want[signature.Target])
			}
		}
	})

	refusals := []struct{ name, target string }{
		{"a generic function", kitPackage + "Generic"},
		{"a method the type lacks", kitPackage + "Counter.Missing"},
		{"a type the package lacks", kitPackage + "Absent.Add"},
		{"a variable", kitPackage + "ErrLimit"},
	}
	for _, tt := range refusals {
		t.Run("a target naming "+tt.name+" is refused [verifies SP-950~1]", func(t *testing.T) {
			_, err := prober.Signatures(context.Background(), project.workspace, project.toolchain, []string{tt.target})
			if !errors.Is(err, golang.ErrUnsupportedTarget) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}
