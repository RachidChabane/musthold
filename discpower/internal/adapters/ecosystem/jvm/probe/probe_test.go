package probe_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/ecosystem/jvm/probe"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/differential"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
)

const javaSource = `package a.b;

import java.math.BigDecimal;

public final class Rates {
    public static BigDecimal lineTotal(BigDecimal unitPrice, int quantity) { return unitPrice; }

    static String join(String first, java.time.LocalDate day, String... rest) { return first; }

    static int twice(int value) { return value; }

    static int twice(long value) { return 0; }

    static class Inner {
        long widen(int amount, Map<String, Integer> index, double[] weights) { return amount; }
    }
}
`

const kotlinSource = `package a.b

fun lineTotal(unitPrice: Int, quantity: Int?): Int = unitPrice * (quantity ?: 0)

class Cart {
    fun total(items: List<Int>, rate: java.math.BigDecimal, flag: Boolean): Int { return 1 }
}

object Pricing {
    fun discount(total: Long): Long = total
}
`

func TestParameters(t *testing.T) {
	cases := []struct {
		name    string
		file    string
		source  string
		classes []string
		method  string
		want    []probe.Parameter
	}{
		{"java static method", "Rates.java", javaSource, []string{"Rates"}, "lineTotal",
			[]probe.Parameter{{Name: "unitPrice", Type: "BigDecimal"}, {Name: "quantity", Type: "int"}}},
		{"java qualified type and varargs", "Rates.java", javaSource, []string{"Rates"}, "join",
			[]probe.Parameter{{Name: "first", Type: "String"}, {Name: "day", Type: "LocalDate"}, {Name: "rest", Type: "String[]"}}},
		{"java nested class and generic type", "Rates.java", javaSource, []string{"Rates", "Inner"}, "widen",
			[]probe.Parameter{{Name: "amount", Type: "int"}, {Name: "index", Type: "Map"}, {Name: "weights", Type: "double[]"}}},
		{"kotlin top-level function and nullable type", "Rates.kt", kotlinSource, nil, "lineTotal",
			[]probe.Parameter{{Name: "unitPrice", Type: "Int"}, {Name: "quantity", Type: "Int"}}},
		{"kotlin class member", "Rates.kt", kotlinSource, []string{"Cart"}, "total",
			[]probe.Parameter{{Name: "items", Type: "List"}, {Name: "rate", Type: "BigDecimal"}, {Name: "flag", Type: "Boolean"}}},
		{"kotlin object member", "Rates.kt", kotlinSource, []string{"Pricing"}, "discount",
			[]probe.Parameter{{Name: "total", Type: "Long"}}},
	}
	for _, c := range cases {
		t.Run(c.name+" [verifies SP-1115~1]", func(t *testing.T) {
			got, err := probe.Parameters(c.file, []byte(c.source), c.classes, c.method)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("parameters = %+v, want %+v", got, c.want)
			}
		})
	}

	refusals := []struct {
		name    string
		file    string
		classes []string
		method  string
		want    error
	}{
		{"an overloaded method", "Rates.java", []string{"Rates"}, "twice", probe.ErrOverloaded},
		{"an unknown method", "Rates.java", []string{"Rates"}, "absent", probe.ErrNoMethod},
		{"an unknown class", "Rates.java", []string{"Missing"}, "lineTotal", probe.ErrNoMethod},
		{"a file of another language", "rates.py", []string{"Rates"}, "lineTotal", probe.ErrUnsupportedSource},
	}
	for _, c := range refusals {
		t.Run(c.name+" is refused [verifies SP-1115~1]", func(t *testing.T) {
			if _, err := probe.Parameters(c.file, []byte(javaSource), c.classes, c.method); !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestHintOf(t *testing.T) {
	t.Run("declared types are hinted for the literals the probe converts [verifies SP-1115~1]", func(t *testing.T) {
		cases := map[string]differential.Hint{
			"int": differential.HintInteger, "Long": differential.HintInteger, "Int": differential.HintInteger,
			"BigDecimal": differential.HintDecimal, "double": differential.HintDecimal,
			"String": differential.HintString, "Boolean": differential.HintBoolean,
			"LocalDate": differential.HintDatetime, "Instant": differential.HintDatetime,
			"List": differential.HintUnknown, "": differential.HintUnknown,
		}
		for declared, want := range cases {
			if got := probe.HintOf(declared); got != want {
				t.Errorf("HintOf(%q) = %q, want %q", declared, got, want)
			}
		}
	})
}

func TestEncode(t *testing.T) {
	t.Run("a call is one tab separated line with its arguments typed [verifies SP-1116~1]", func(t *testing.T) {
		text := "a\tb\nc\\d"
		got := string(probe.Encode([]probe.Call{{
			Class: "a.b.Rates", Method: "lineTotal",
			Args: []*spec.Literal{{Type: spec.TypeDecimal, Value: "4.50"}, nil, {Type: spec.TypeString, Value: text}},
		}}))
		want := "CALL\ta.b.Rates\tlineTotal\t3\tdecimal\t4.50\tmissing\t\tstring\ta\\tb\\nc\\\\d\n"
		if got != want {
			t.Fatalf("encoded %q, want %q", got, want)
		}
		if strings.Count(got, "\n") != 1 {
			t.Fatal("a call spans several lines")
		}
	})
}
