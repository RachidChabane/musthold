package java_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/units/java"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/code"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/hunk"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/testlabel"
	"github.com/RachidChabane/spec-proof/completeness/internal/testkit"
)

// marker is the word that opens a citation; the tests write it as `cites` so that
// none of their lines reads as a requirement tag.
const marker = "verif" + "ies"

func tagged(text string) string { return strings.ReplaceAll(text, "cites", marker) }

func fixture(t *testing.T, side, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(testkit.FixturesDir(), testkit.JavaLedger, side, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func index(t *testing.T, file, source string) code.File {
	t.Helper()
	indexed, err := java.New().Index(file, []byte(source))
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

const cart = `package shop;

import java.util.List;

public class Cart implements Iterable<Item> {
    public static final int LIMIT = 10, FLOOR = 1;
    static final String NAME = "cart";
    private static int counter = 0;
    private final List<Item> items = new ArrayList<>();
    int size;

    static {
        counter = 1;
    }

    public Cart(String owner) {
        this.items.clear();
    }

    public void add(Item item) {
        items.add(item);
    }

    public void add(Item item, int times) {
        for (int i = 0; i < times; i++) {
            add(item);
        }
    }

    public interface Visitor {
        int MAX = 3;
        void visit(Cart cart);
    }

    public enum Mode {
        LOCAL, REMOTE;

        int rank() {
            return ordinal();
        }
    }

    public static class Receipt {
        public Receipt() {
        }

        String text() {
            return "r";
        }
    }

    public record Point(int x, int y) {
        public Point {
            if (x < 0) {
                throw new IllegalArgumentException();
            }
        }
    }
}
`

func TestIndexesUnits(t *testing.T) {
	t.Run("a unit is listed per method, constructor and static initializer, and per constant, qualified by the types around it [verifies SP-1303~1]", func(t *testing.T) {
		got := index(t, "src/main/java/shop/Cart.java", cart).Units
		want := []string{
			"Cart.LIMIT", "Cart.FLOOR", "Cart.NAME", "Cart.<clinit>", "Cart.<init>", "Cart.add", "Cart.add#2",
			"Cart.Visitor.MAX", "Cart.Visitor.visit", "Cart.Mode.LOCAL", "Cart.Mode.REMOTE", "Cart.Mode.rank",
			"Cart.Receipt.<init>", "Cart.Receipt.text", "Cart.Point.<init>",
		}
		names := qualnames(got)
		slices.Sort(names)
		slices.Sort(want)
		if !slices.Equal(names, want) {
			t.Fatalf("units\n%v\nwant\n%v", names, want)
		}
	})

	t.Run("a field that is not a static final constant is no unit [verifies SP-1303~1]", func(t *testing.T) {
		names := qualnames(index(t, "Cart.java", cart).Units)
		for _, absent := range []string{"Cart.counter", "Cart.items", "Cart.size"} {
			if slices.Contains(names, absent) {
				t.Errorf("%s is a unit", absent)
			}
		}
	})

	t.Run("a constructor and a static initializer answer to the name of their class, a method to its own [verifies SP-1303~1]", func(t *testing.T) {
		units := index(t, "Cart.java", cart).Units
		for _, qualname := range []string{"Cart.<init>", "Cart.<clinit>"} {
			if unit := find(t, units, qualname); unit.Name != "Cart" || unit.Owner != "Cart" || unit.Kind != code.KindFunction {
				t.Errorf("%s: %+v", qualname, unit)
			}
		}
		if unit := find(t, units, "Cart.Receipt.text"); unit.Name != "text" || unit.Owner != "Receipt" {
			t.Errorf("text: %+v", unit)
		}
		if unit := find(t, units, "Cart.LIMIT"); unit.Kind != code.KindAssignment || unit.Name != "LIMIT" {
			t.Errorf("LIMIT: %+v", unit)
		}
	})

	t.Run("a unit mentions the names in its code, calls and types included [verifies SP-1303~1]", func(t *testing.T) {
		units := index(t, "Totals.java", fixture(t, "change", "src/main/java/ledger/Totals.java")).Units
		invoice := find(t, units, "Totals.invoiceTotal")
		for _, want := range []string{"lineAmount", "TAX_RATE", "BigDecimal", "RoundingMode", "Line"} {
			if !slices.Contains(invoice.Mentions, want) {
				t.Errorf("invoiceTotal does not mention %q: %v", want, invoice.Mentions)
			}
		}
	})

	t.Run("a file that does not parse still gives the units it could read [verifies SP-1303~1]", func(t *testing.T) {
		broken := index(t, "Broken.java", "class Broken {\n    void ok() { }\n}\nclass Oops {\n    void oops( {\n")
		if !slices.Contains(qualnames(broken.Units), "Broken.ok") {
			t.Fatalf("units %v", qualnames(broken.Units))
		}
	})
}

func TestChangedUnits(t *testing.T) {
	before := index(t, "Totals.java", fixture(t, "base", "src/main/java/ledger/Totals.java")).Units
	after := index(t, "Totals.java", fixture(t, "change", "src/main/java/ledger/Totals.java")).Units

	t.Run("a unit that is new or whose code differs is a hunk and a Javadoc is no change [verifies SP-1305~1]", func(t *testing.T) {
		var changed []string
		for _, found := range hunk.Between(before, after) {
			changed = append(changed, found.Unit.Qualname)
		}
		if !slices.Equal(changed, []string{"Totals.TAX_RATE", "Totals.lineAmount", "Totals.invoiceTotal", "Totals.applyDiscount"}) {
			t.Fatalf("hunks %v", changed)
		}
	})

	t.Run("a new class makes every unit a hunk [verifies SP-1305~1]", func(t *testing.T) {
		receipts := index(t, "Receipts.java", fixture(t, "change", "src/main/java/ledger/Receipts.java")).Units
		got := hunk.Between(nil, receipts)
		if len(got) != 1 || got[0].Unit.Qualname != "Receipts.receiptLine" || !got[0].New {
			t.Fatalf("hunks %+v", got)
		}
	})

	t.Run("formatting and comments are no change and a literal is [verifies SP-1305~1]", func(t *testing.T) {
		original := index(t, "A.java", "class A { int f(int x) { return x + 1; } }").Units
		reformatted := index(t, "A.java", "class A {\n    /** Adds one. */\n    int f(int x) {\n        // add\n        return x + 1; /* done */\n    }\n}\n").Units
		if got := hunk.Between(original, reformatted); len(got) != 0 {
			t.Fatalf("hunks %+v", got)
		}
		literal := index(t, "A.java", "class A { int f(int x) { return x + 2; } }").Units
		if got := hunk.Between(original, literal); len(got) != 1 {
			t.Fatalf("hunks %+v", got)
		}
	})
}

func TestServesTestFiles(t *testing.T) {
	t.Run("the files below a test directory and those named as tests hold tests [verifies SP-1304~1]", func(t *testing.T) {
		indexer := java.New()
		for _, file := range []string{
			"src/test/java/ledger/Anything.java", "service/src/test/java/a/B.java", "ledger/TotalsTest.java", "ledger/TotalsTests.java",
			"ledger/TotalsIT.java", "ledger/TotalsTestCase.java", "ledger/TestTotals.java", "src/integrationTest/java/A.java",
		} {
			if !indexer.IsTest(file) {
				t.Errorf("%s is not a test file", file)
			}
		}
		for _, file := range []string{"src/main/java/ledger/Totals.java", "ledger/Contest.java", "ledger/Test.java", "ledger/Latest.java", "ledger/Attestation.java"} {
			if indexer.IsTest(file) {
				t.Errorf("%s is a test file", file)
			}
		}
	})

	t.Run("only Java files are served [verifies SP-1304~1]", func(t *testing.T) {
		if !java.New().Handles("A.java") || java.New().Handles("A.kt") || java.New().Handles("a.ts") || java.New().Handles("a.py") {
			t.Fatal("the extensions served are wrong")
		}
	})
}

const suite = `package ledger;

@ExtendWith(MockitoExtension.class)
class TotalsTest {
    private final Cart cart = new Cart();
    private static final int UNIT = 4;

    @BeforeEach
    void open() {
        cart.reset();
    }

    @Test
    @DisplayName("adds the lines [cites INV-1~1]")
    void addsTheLines() {
        assertEquals(1, Totals.invoiceTotal(cart));
    }

    @ParameterizedTest(name = "{0} kilos cost {1} [cites INV-2~1]")
    @MethodSource("rows")
    void costs(int kilos, int cost) {
        assertEquals(cost, Rates.cost(kilos));
    }

    @ParameterizedTest
    @ValueSource(ints = {1, 2})
    void anyWeight(int kilos) {
        assertTrue(Rates.cost(kilos) > 0);
    }

    @RepeatedTest(2)
    void again() {
        Totals.lineAmount(null, 1);
    }

    static Stream<Arguments> rows() {
        return Stream.of(Arguments.of(1, 2));
    }

    @Nested
    @DisplayName("an invoice [cites INV-3~1]")
    class Invoice {
        @Test
        void plain() {
            Totals.formatTotal(null);
        }
    }
}
`

func TestIndexesTests(t *testing.T) {
	file := index(t, "src/test/java/ledger/TotalsTest.java", tagged(suite))
	tests := map[string]code.Unit{}
	for _, unit := range file.Units {
		if unit.Kind == code.KindTest {
			tests[unit.Qualname] = unit
		}
	}

	t.Run("every annotated method is a test unit and the helpers of the class stay functions [verifies SP-1304~1]", func(t *testing.T) {
		names := make([]string, 0, len(tests))
		for name := range tests {
			names = append(names, name)
		}
		slices.Sort(names)
		want := []string{"TotalsTest.addsTheLines", "TotalsTest.again", "TotalsTest.anyWeight", "TotalsTest.costs", "TotalsTest.Invoice.plain"}
		slices.Sort(want)
		if !slices.Equal(names, want) {
			t.Fatalf("tests %v, want %v", names, want)
		}
		for _, helper := range []string{"TotalsTest.open", "TotalsTest.rows"} {
			if unit := find(t, file.Units, helper); unit.Kind != code.KindFunction {
				t.Errorf("%s is %s", helper, unit.Kind)
			}
		}
	})

	t.Run("a test mentions what it calls and what the classes around it run for every test [verifies SP-1304~1]", func(t *testing.T) {
		adds := tests["TotalsTest.addsTheLines"]
		for _, want := range []string{"invoiceTotal", "assertEquals", "reset", "Cart"} {
			if !slices.Contains(adds.Mentions, want) {
				t.Errorf("the test does not mention %q: %v", want, adds.Mentions)
			}
		}
		if slices.Contains(adds.Mentions, "Rates") {
			t.Errorf("a sibling test's code is mentioned: %v", adds.Mentions)
		}
		nested := tests["TotalsTest.Invoice.plain"]
		if !slices.Contains(nested.Mentions, "reset") || !slices.Contains(nested.Mentions, "formatTotal") {
			t.Errorf("the nested test does not mention the setup of the class around it: %v", nested.Mentions)
		}
	})

	t.Run("a runner's label matches the test by its display name, its expanded pattern, its method name or its default name [verifies SP-1304~1]", func(t *testing.T) {
		cases := []struct {
			test  string
			label string
		}{
			{"TotalsTest.addsTheLines", tagged("ledger.TotalsTest.adds the lines [cites INV-1~1]")},
			{"TotalsTest.addsTheLines", "ledger.TotalsTest.addsTheLines()"},
			{"TotalsTest.costs", tagged("ledger.TotalsTest.7 kilos cost 9 [cites INV-2~1]")},
			{"TotalsTest.anyWeight", "ledger.TotalsTest.[2] 2"},
			{"TotalsTest.anyWeight", "ledger.TotalsTest.anyWeight(int)[2]"},
			{"TotalsTest.again", "ledger.TotalsTest.repetition 1 of 2"},
			{"TotalsTest.Invoice.plain", "ledger.TotalsTest$Invoice.plain()"},
			{"TotalsTest.Invoice.plain", tagged("TotalsTest an invoice [cites INV-3~1].plain")},
		}
		for _, c := range cases {
			if !testlabel.Matches(tests[c.test].Labels, c.label) {
				t.Errorf("%s does not match %q (%v)", c.test, c.label, tests[c.test].Labels)
			}
		}
		if testlabel.Matches(tests["TotalsTest.addsTheLines"].Labels, "ledger.TotalsTest.other") || testlabel.Matches(tests["TotalsTest.again"].Labels, "ledger.TotalsTest.[1] 1") {
			t.Error("a label of another test matched")
		}
	})
}
