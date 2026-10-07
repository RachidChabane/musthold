package kotlin_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/units/kotlin"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/code"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/hunk"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/testlabel"
)

// marker is the word that opens a citation; the tests write it as `cites` so that
// none of their lines reads as a requirement tag.
const marker = "verif" + "ies"

func tagged(text string) string { return strings.ReplaceAll(text, "cites", marker) }

func index(t *testing.T, file, source string) code.File {
	t.Helper()
	indexed, err := kotlin.New().Index(file, []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	return indexed
}

func qualnames(units []code.Unit) []string {
	names := make([]string, len(units))
	for position, unit := range units {
		names[position] = unit.Qualname
	}
	return names
}

func find(t *testing.T, units []code.Unit, qualname string) code.Unit {
	t.Helper()
	for _, unit := range units {
		if unit.Qualname == qualname {
			return unit
		}
	}
	t.Fatalf("no unit %q among %v", qualname, qualnames(units))
	return code.Unit{}
}

const cart = `package shop

import java.math.BigDecimal

const val LIMIT = 10
val RATE: BigDecimal = BigDecimal("0.2")
var counter = 0
val (first, second) = pair

/** Totals a line. */
fun total(items: List<Int>): Int = items.size

private fun hidden() {}

enum class Mode { LOCAL, REMOTE }

class Cart(private val owner: String) : Iterable<Int> {
    private val items = mutableListOf<Int>()

    constructor() : this("nobody")

    init {
        items.clear()
    }

    fun add(item: Int) {
        items.add(item)
    }

    val size: Int
        get() = items.size

    companion object {
        const val EMPTY = 0
        fun of(owner: String): Cart = Cart(owner)
    }

    class Receipt {
        fun text(): String = "r"
    }
}

object Rates {
    const val BASE = 1
    val EXTRA = 2
    fun base(): Int = BASE
}
`

func TestIndexesUnits(t *testing.T) {
	t.Run("a unit is listed per function, constructor and initializer and per constant, qualified by the types around it [verifies SP-1306~1]", func(t *testing.T) {
		got := index(t, "src/main/kotlin/shop/Cart.kt", cart).Units
		want := []string{
			"LIMIT", "RATE", "counter", "first", "second", "total", "hidden", "Mode.LOCAL", "Mode.REMOTE",
			"Cart.<init>", "Cart.<init>#2", "Cart.<init>#3", "Cart.add", "Cart.Companion.EMPTY", "Cart.Companion.of", "Cart.Receipt.text",
			"Rates.BASE", "Rates.EXTRA", "Rates.base",
		}
		names := qualnames(got)
		slices.Sort(names)
		slices.Sort(want)
		if !slices.Equal(names, want) {
			t.Fatalf("units\n%v\nwant\n%v", names, want)
		}
	})

	t.Run("a property of a class is no unit and a property of an object or a file is one [verifies SP-1306~1]", func(t *testing.T) {
		units := index(t, "Cart.kt", cart).Units
		names := qualnames(units)
		for _, absent := range []string{"Cart.items", "Cart.size"} {
			if slices.Contains(names, absent) {
				t.Errorf("%s is a unit", absent)
			}
		}
		if unit := find(t, units, "Rates.EXTRA"); unit.Kind != code.KindAssignment || unit.Owner != "Rates" {
			t.Errorf("EXTRA: %+v", unit)
		}
		if unit := find(t, units, "LIMIT"); unit.Kind != code.KindAssignment || unit.Owner != "" {
			t.Errorf("LIMIT: %+v", unit)
		}
	})

	t.Run("a constructor and an initializer answer to the name of their class, a function to its own [verifies SP-1306~1]", func(t *testing.T) {
		units := index(t, "Cart.kt", cart).Units
		for _, qualname := range []string{"Cart.<init>", "Cart.<init>#2", "Cart.<init>#3"} {
			if unit := find(t, units, qualname); unit.Name != "Cart" || unit.Kind != code.KindFunction {
				t.Errorf("%s: %+v", qualname, unit)
			}
		}
		if unit := find(t, units, "Cart.Companion.of"); unit.Name != "of" || unit.Owner != "Cart" {
			t.Errorf("of: %+v", unit)
		}
	})

	t.Run("a unit mentions the names in its code [verifies SP-1306~1]", func(t *testing.T) {
		units := index(t, "Cart.kt", cart).Units
		add := find(t, units, "Cart.add")
		for _, want := range []string{"items", "add", "Int"} {
			if !slices.Contains(add.Mentions, want) {
				t.Errorf("add does not mention %q: %v", want, add.Mentions)
			}
		}
	})

	t.Run("a class that several annotations precede still gives its units [verifies SP-1306~1]", func(t *testing.T) {
		got := index(t, "A.kt", "@ExtendWith(MockitoExtension::class)\n@DisplayName(\"a\")\nclass A {\n    fun f() {}\n}\n").Units
		if !slices.Equal(qualnames(got), []string{"A.f"}) {
			t.Fatalf("units %v", qualnames(got))
		}
	})

	t.Run("the lines of a unit are those of the source whatever the annotations before its class [verifies SP-1306~1]", func(t *testing.T) {
		got := index(t, "A.kt", "@ExtendWith(MockitoExtension::class)\n@DisplayName(\"a\")\nclass A {\n    fun f() {\n    }\n\n    fun g() {}\n}\n").Units
		if f := find(t, got, "A.f"); f.StartLine != 4 || f.EndLine != 5 {
			t.Errorf("f lines %d to %d", f.StartLine, f.EndLine)
		}
		if g := find(t, got, "A.g"); g.StartLine != 7 {
			t.Errorf("g starts on line %d", g.StartLine)
		}
	})

	t.Run("a file that does not parse still gives the units it could read [verifies SP-1306~1]", func(t *testing.T) {
		broken := index(t, "Broken.kt", "class Broken {\n    fun ok() {}\n}\nclass Oops {\n    fun oops( {\n")
		if !slices.Contains(qualnames(broken.Units), "Broken.ok") {
			t.Fatalf("units %v", qualnames(broken.Units))
		}
	})
}

func TestChangedUnits(t *testing.T) {
	const before = "package a\n\nconst val RATE = 1\n\nfun f(x: Int): Int = x + 1\n\nfun g(x: Int) = x\n"
	original := index(t, "A.kt", before).Units

	t.Run("a unit that is new or whose code differs is a hunk, a comment or a KDoc is no change [verifies SP-1308~1]", func(t *testing.T) {
		after := index(t, "A.kt", "package a\n\nconst val RATE = 2\n\n/** Adds one. */\nfun f(x: Int): Int =\n    x + 1 // add\n\nfun g(x: Int) = x\n\nfun h() {}\n").Units
		var changed []string
		for _, found := range hunk.Between(original, after) {
			changed = append(changed, found.Unit.Qualname)
		}
		if !slices.Equal(changed, []string{"RATE", "h"}) {
			t.Fatalf("hunks %v", changed)
		}
	})

	t.Run("a literal in a function is a change [verifies SP-1308~1]", func(t *testing.T) {
		after := index(t, "A.kt", strings.Replace(before, "x + 1", "x + 2", 1)).Units
		got := hunk.Between(original, after)
		if len(got) != 1 || got[0].Unit.Qualname != "f" {
			t.Fatalf("hunks %+v", got)
		}
	})
}

func TestServesTestFiles(t *testing.T) {
	t.Run("the files below a test directory and those named as tests hold tests [verifies SP-1307~1]", func(t *testing.T) {
		indexer := kotlin.New()
		for _, file := range []string{"src/test/kotlin/shop/Any.kt", "shop/PricingTest.kt", "shop/PricingTests.kt", "shop/PricingIT.kt", "shop/TestPricing.kt"} {
			if !indexer.IsTest(file) {
				t.Errorf("%s is not a test file", file)
			}
		}
		for _, file := range []string{"src/main/kotlin/shop/Pricing.kt", "shop/Contest.kt", "shop/Test.kt"} {
			if indexer.IsTest(file) {
				t.Errorf("%s is a test file", file)
			}
		}
		if !indexer.Handles("A.kt") || indexer.Handles("A.java") || indexer.Handles("a.ts") {
			t.Error("the extensions served are wrong")
		}
	})
}

const suite = `package shop

@ExtendWith(MockitoExtension::class)
@DisplayName("totals")
class TotalsTest {
    private val cart = Cart()

    @BeforeEach
    fun open() {
        cart.reset()
    }

    @Test
    @DisplayName("adds the lines [cites INV-1~1]")
    fun addsTheLines() {
        assertEquals(1, Totals.invoiceTotal(cart))
    }

    @Test
    fun ` + "`the plain one`" + `() {
        Totals.lineAmount(null, 1)
    }

    @ParameterizedTest(name = "{0} kilos cost {1} [cites INV-2~1]")
    @MethodSource("rows")
    fun costs(kilos: Int, cost: Int) {
        assertEquals(cost, Rates.cost(kilos))
    }

    @Nested
    @DisplayName("an invoice")
    inner class Invoice {
        @Test
        fun plain() {
            Totals.formatTotal(null)
        }
    }
}
`

func TestIndexesTests(t *testing.T) {
	file := index(t, "src/test/kotlin/shop/TotalsTest.kt", tagged(suite))
	tests := map[string]code.Unit{}
	for _, unit := range file.Units {
		if unit.Kind == code.KindTest {
			tests[unit.Qualname] = unit
		}
	}

	t.Run("every annotated function is a test unit and the helpers stay functions [verifies SP-1307~1]", func(t *testing.T) {
		names := make([]string, 0, len(tests))
		for name := range tests {
			names = append(names, name)
		}
		slices.Sort(names)
		want := []string{"TotalsTest.addsTheLines", "TotalsTest.the plain one", "TotalsTest.costs", "TotalsTest.Invoice.plain"}
		slices.Sort(want)
		if !slices.Equal(names, want) {
			t.Fatalf("tests %v, want %v", names, want)
		}
		if unit := find(t, file.Units, "TotalsTest.open"); unit.Kind != code.KindFunction {
			t.Errorf("open is %s", unit.Kind)
		}
	})

	t.Run("a test mentions what it calls and what the classes around it run for every test [verifies SP-1307~1]", func(t *testing.T) {
		adds := tests["TotalsTest.addsTheLines"]
		for _, want := range []string{"invoiceTotal", "assertEquals", "reset"} {
			if !slices.Contains(adds.Mentions, want) {
				t.Errorf("the test does not mention %q: %v", want, adds.Mentions)
			}
		}
		if slices.Contains(adds.Mentions, "Rates") {
			t.Errorf("a sibling test's code is mentioned: %v", adds.Mentions)
		}
		if nested := tests["TotalsTest.Invoice.plain"]; !slices.Contains(nested.Mentions, "reset") {
			t.Errorf("the nested test does not mention the setup of the class around it: %v", nested.Mentions)
		}
	})

	t.Run("a runner's label matches the test by its display name, its sentence, its expanded pattern or its class [verifies SP-1307~1]", func(t *testing.T) {
		cases := []struct {
			test  string
			label string
		}{
			{"TotalsTest.addsTheLines", tagged("shop.TotalsTest.adds the lines [cites INV-1~1]")},
			{"TotalsTest.addsTheLines", "shop.TotalsTest.addsTheLines()"},
			{"TotalsTest.the plain one", "shop.TotalsTest.the plain one()"},
			{"TotalsTest.costs", tagged("shop.TotalsTest.7 kilos cost 9 [cites INV-2~1]")},
			{"TotalsTest.Invoice.plain", "shop.TotalsTest$Invoice.plain()"},
			{"TotalsTest.Invoice.plain", "TotalsTest an invoice.plain"},
		}
		for _, c := range cases {
			if !testlabel.Matches(tests[c.test].Labels, c.label) {
				t.Errorf("%s does not match %q (%v)", c.test, c.label, tests[c.test].Labels)
			}
		}
		if testlabel.Matches(tests["TotalsTest.addsTheLines"].Labels, "shop.TotalsTest.other") {
			t.Error("a label of another test matched")
		}
	})
}
