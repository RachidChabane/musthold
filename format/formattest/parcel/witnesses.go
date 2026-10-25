package parcel

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/RachidChabane/spec-proof/format/formattest/prompts"
)

// Languages a witness prompt names, as the module's witness hosts write them.
const (
	languageGo     = "Go, the testing package"
	languageJest   = "TypeScript, Jest"
	languageVitest = "TypeScript, Vitest"
	languageJava   = "Java, JUnit 5"
	targetHeader   = "Requirement to check:"
	languageHeader = "Language and test framework:"
	scopeFixed     = "FIXED"
	scopeConfident = 0.95
)

// Subjects of the parcel criteria, as their statements phrase them.
const (
	subjectShipping = "shipping fee"
	subjectLines    = "unit price"
	subjectDiscount = "discount"
)

var scopeLabelPattern = regexp.MustCompile(`(?m)^Witness (w\d+), for the clause`)

// parcelWitness is one witness an honest writer gives for a parcel criterion.
type parcelWitness struct {
	Name   string `json:"name"`
	Clause string `json:"clause"`
	Kind   string `json:"kind"`
	Code   string `json:"code"`
}

type witnessSet struct {
	imports   string
	witnesses []parcelWitness
}

// pythonWitnesses are the witnesses of the Python parcel project, by subject.
var pythonWitnesses = map[string]witnessSet{
	subjectLines: {"from parcel.rates import line_total\nimport random", []parcelWitness{
		{"line_of_four_at_three", "its unit price times its quantity", "example", "def line_of_four_at_three():\n    assert line_total(4, 3) == 12"},
		{"line_of_two_at_five", "its unit price times its quantity", "example", "def line_of_two_at_five():\n    assert line_total(2, 5) == 10"},
		{"line_of_no_units", "its unit price times its quantity", "example", "def line_of_no_units():\n    assert line_total(7, 0) == 0"},
		{"lines_multiply", "its unit price times its quantity", "property", "def lines_multiply():\n    draws = random.Random(1)\n    for _ in range(200):\n        price, quantity = draws.randint(0, 1000), draws.randint(0, 100)\n        assert line_total(price, quantity) == price * quantity"},
	}},
	subjectDiscount: {"from parcel.rates import discounted\nimport random", []parcelWitness{
		{"ten_percent_off", "reduce the total by that percentage", "example", "def ten_percent_off():\n    assert discounted(200, 10) == 180"},
		{"above_one_hundred_is_clamped", "limited to the range 0 to 100", "example", "def above_one_hundred_is_clamped():\n    assert discounted(80, 150) == 0"},
		{"negative_is_ignored", "limited to the range 0 to 100", "example", "def negative_is_ignored():\n    assert discounted(80, -5) == 80"},
		{"discount_reduces", "reduce the total by that percentage", "property", "def discount_reduces():\n    draws = random.Random(2)\n    for _ in range(200):\n        total, percent = draws.randint(0, 1000), draws.randint(0, 100)\n        assert discounted(total, percent) == total * (100 - percent) / 100"},
	}},
	subjectShipping: {"from parcel.rates import shipping_fee\nimport random", []parcelWitness{
		{"free_from_fifty", "reaches 50", "example", "def free_from_fifty():\n    assert shipping_fee(50) == 0"},
		{"charged_below_fifty", "reaches 50", "example", "def charged_below_fifty():\n    assert shipping_fee(49.99) == 4.5"},
		{"free_for_large_orders", "waive the shipping fee", "example", "def free_for_large_orders():\n    assert shipping_fee(100) == 0"},
		{"waived_from_fifty", "waive the shipping fee", "property", "def waived_from_fifty():\n    draws = random.Random(3)\n    for _ in range(200):\n        total = draws.randint(0, 20000) / 100\n        assert (shipping_fee(total) == 0) == (total >= 50)"},
	}},
}

// goWitnesses are the witnesses of the Go parcel project, by subject.
var goWitnesses = map[string]witnessSet{
	subjectLines: {"\"math/rand/v2\"\n\"testing\"", []parcelWitness{
		{"TestLineOfFourAtThree", "its unit price times its quantity", "example", "func TestLineOfFourAtThree(t *testing.T) {\n\tif LineTotal(4, 3) != 12 {\n\t\tt.Fatal(LineTotal(4, 3))\n\t}\n}"},
		{"TestLineOfTwoAtFive", "its unit price times its quantity", "example", "func TestLineOfTwoAtFive(t *testing.T) {\n\tif LineTotal(2, 5) != 10 {\n\t\tt.Fatal(LineTotal(2, 5))\n\t}\n}"},
		{"TestLineOfNoUnits", "its unit price times its quantity", "example", "func TestLineOfNoUnits(t *testing.T) {\n\tif LineTotal(7, 0) != 0 {\n\t\tt.Fatal(LineTotal(7, 0))\n\t}\n}"},
		{"TestLinesMultiply", "its unit price times its quantity", "property", "func TestLinesMultiply(t *testing.T) {\n\tdraws := rand.New(rand.NewPCG(1, 2))\n\tfor range 200 {\n\t\tprice, quantity := draws.IntN(1000), draws.IntN(100)\n\t\tif LineTotal(price, quantity) != price*quantity {\n\t\t\tt.Fatal(price, quantity)\n\t\t}\n\t}\n}"},
	}},
	subjectDiscount: {"\"math/rand/v2\"\n\"testing\"", []parcelWitness{
		{"TestTenPercentOff", "reduce the total by that percentage", "example", "func TestTenPercentOff(t *testing.T) {\n\tif Discounted(20000, 10) != 18000 {\n\t\tt.Fatal(Discounted(20000, 10))\n\t}\n}"},
		{"TestAboveOneHundredIsClamped", "limited to the range 0 to 100", "example", "func TestAboveOneHundredIsClamped(t *testing.T) {\n\tif Discounted(8000, 150) != 0 {\n\t\tt.Fatal(Discounted(8000, 150))\n\t}\n}"},
		{"TestNegativeIsIgnored", "limited to the range 0 to 100", "example", "func TestNegativeIsIgnored(t *testing.T) {\n\tif Discounted(8000, -5) != 8000 {\n\t\tt.Fatal(Discounted(8000, -5))\n\t}\n}"},
		{"TestDiscountReduces", "reduce the total by that percentage", "property", "func TestDiscountReduces(t *testing.T) {\n\tdraws := rand.New(rand.NewPCG(3, 4))\n\tfor range 200 {\n\t\ttotal, percent := draws.IntN(100000), draws.IntN(101)\n\t\tif Discounted(total, percent) != total*(100-percent)/100 {\n\t\t\tt.Fatal(total, percent)\n\t\t}\n\t}\n}"},
	}},
	subjectShipping: {"\"math/rand/v2\"\n\"testing\"", []parcelWitness{
		{"TestFreeFromThreshold", "reaches 50", "example", "func TestFreeFromThreshold(t *testing.T) {\n\tif ShippingFee(5000) != 0 {\n\t\tt.Fatal(ShippingFee(5000))\n\t}\n}"},
		{"TestChargedBelowThreshold", "reaches 50", "example", "func TestChargedBelowThreshold(t *testing.T) {\n\tif ShippingFee(4999) != 450 {\n\t\tt.Fatal(ShippingFee(4999))\n\t}\n}"},
		{"TestFreeForLargeOrders", "waive the shipping fee", "example", "func TestFreeForLargeOrders(t *testing.T) {\n\tif ShippingFee(10000) != 0 {\n\t\tt.Fatal(ShippingFee(10000))\n\t}\n}"},
		{"TestWaivedFromThreshold", "waive the shipping fee", "property", "func TestWaivedFromThreshold(t *testing.T) {\n\tdraws := rand.New(rand.NewPCG(5, 6))\n\tfor range 200 {\n\t\ttotal := draws.IntN(20000)\n\t\tif (ShippingFee(total) == 0) != (total >= 5000) {\n\t\t\tt.Fatal(total)\n\t\t}\n\t}\n}"},
	}},
}

// typescriptWitnesses are the witnesses of the TypeScript parcel projects, by
// subject, with the imports Jest needs; Vitest's add its `expect`.
var typescriptWitnesses = map[string]witnessSet{
	subjectLines: {"import { lineTotal } from '../src/rates';", []parcelWitness{
		{"lineOfFourAtThree", "its unit price times its quantity", "example", "function lineOfFourAtThree(): void {\n  expect(lineTotal(4, 3)).toBe(12);\n}"},
		{"lineOfTwoAtFive", "its unit price times its quantity", "example", "function lineOfTwoAtFive(): void {\n  expect(lineTotal(2, 5)).toBe(10);\n}"},
		{"lineOfNoUnits", "its unit price times its quantity", "example", "function lineOfNoUnits(): void {\n  expect(lineTotal(7, 0)).toBe(0);\n}"},
		{"linesMultiply", "its unit price times its quantity", "property", "function linesMultiply(): void {\n  let seed = 1;\n  const next = (): number => {\n    seed = (seed * 16807) % 2147483647;\n    return seed;\n  };\n  for (let draw = 0; draw < 200; draw++) {\n    const price = next() % 1000;\n    const quantity = next() % 100;\n    expect(lineTotal(price, quantity)).toBe(price * quantity);\n  }\n}"},
	}},
	subjectDiscount: {"import { discounted } from '../src/rates';", []parcelWitness{
		{"tenPercentOff", "reduce the total by that percentage", "example", "function tenPercentOff(): void {\n  expect(discounted(200, 10)).toBe(180);\n}"},
		{"aboveOneHundredIsClamped", "limited to the range 0 to 100", "example", "function aboveOneHundredIsClamped(): void {\n  expect(discounted(80, 150)).toBe(0);\n}"},
		{"negativeIsIgnored", "limited to the range 0 to 100", "example", "function negativeIsIgnored(): void {\n  expect(discounted(80, -5)).toBe(80);\n}"},
		{"discountReduces", "reduce the total by that percentage", "property", "function discountReduces(): void {\n  let seed = 2;\n  const next = (): number => {\n    seed = (seed * 16807) % 2147483647;\n    return seed;\n  };\n  for (let draw = 0; draw < 200; draw++) {\n    const total = next() % 1000;\n    const percent = next() % 101;\n    expect(discounted(total, percent)).toBeCloseTo((total * (100 - percent)) / 100, 2);\n  }\n}"},
	}},
	subjectShipping: {"import { shippingFee } from '../src/rates';", []parcelWitness{
		{"freeFromFifty", "reaches 50", "example", "function freeFromFifty(): void {\n  expect(shippingFee(50)).toBe(0);\n}"},
		{"chargedBelowFifty", "reaches 50", "example", "function chargedBelowFifty(): void {\n  expect(shippingFee(49.99)).toBe(4.5);\n}"},
		{"freeForLargeOrders", "waive the shipping fee", "example", "function freeForLargeOrders(): void {\n  expect(shippingFee(100)).toBe(0);\n}"},
		{"waivedFromFifty", "waive the shipping fee", "property", "function waivedFromFifty(): void {\n  let seed = 3;\n  const next = (): number => {\n    seed = (seed * 16807) % 2147483647;\n    return seed;\n  };\n  for (let draw = 0; draw < 200; draw++) {\n    const total = (next() % 20000) / 100;\n    expect(shippingFee(total) === 0).toBe(total >= 50);\n  }\n}"},
	}},
}

// vitestExpectImport is the import a Vitest witness file adds for `expect`.
const vitestExpectImport = "import { expect } from 'vitest';\n"

// vitestWitnesses are the TypeScript witnesses with Vitest's `expect` imported.
func vitestWitnesses() map[string]witnessSet {
	sets := map[string]witnessSet{}
	for subject, set := range typescriptWitnesses {
		sets[subject] = witnessSet{imports: vitestExpectImport + set.imports, witnesses: set.witnesses}
	}
	return sets
}

// javaImports are the imports every Java witness set of the parcel project
// starts with.
const javaImports = "import static org.junit.jupiter.api.Assertions.assertEquals;\nimport java.math.BigDecimal;"

// javaWitnesses are the witnesses of the Java parcel project, by subject.
var javaWitnesses = map[string]witnessSet{
	subjectLines: {javaImports + "\nimport java.util.Random;", []parcelWitness{
		{"lineOfFourFiftyTimesThree", "its unit price times its quantity", "example", "void lineOfFourFiftyTimesThree() {\n    assertEquals(0, new BigDecimal(\"13.50\").compareTo(Rates.lineTotal(new BigDecimal(\"4.50\"), 3)));\n}"},
		{"lineOfTwoAtFive", "its unit price times its quantity", "example", "void lineOfTwoAtFive() {\n    assertEquals(0, new BigDecimal(\"10\").compareTo(Rates.lineTotal(new BigDecimal(\"5\"), 2)));\n}"},
		{"lineOfNoUnits", "its unit price times its quantity", "example", "void lineOfNoUnits() {\n    assertEquals(0, BigDecimal.ZERO.compareTo(Rates.lineTotal(new BigDecimal(\"7.00\"), 0)));\n}"},
		{"linesMultiply", "its unit price times its quantity", "property", "void linesMultiply() {\n    Random draws = new Random(1L);\n    for (int draw = 0; draw < 200; draw++) {\n        BigDecimal price = BigDecimal.valueOf(draws.nextInt(100000), 2);\n        int quantity = draws.nextInt(100);\n        assertEquals(0, price.multiply(BigDecimal.valueOf(quantity)).compareTo(Rates.lineTotal(price, quantity)));\n    }\n}"},
	}},
	subjectDiscount: {javaImports + "\nimport java.math.RoundingMode;\nimport java.util.Random;", []parcelWitness{
		{"tenPercentOff", "reduce the total by that percentage", "example", "void tenPercentOff() {\n    assertEquals(0, new BigDecimal(\"180\").compareTo(Rates.discounted(new BigDecimal(\"200.00\"), 10)));\n}"},
		{"aboveOneHundredIsClamped", "limited to the range 0 to 100", "example", "void aboveOneHundredIsClamped() {\n    assertEquals(0, BigDecimal.ZERO.compareTo(Rates.discounted(new BigDecimal(\"80.00\"), 150)));\n}"},
		{"negativeIsIgnored", "limited to the range 0 to 100", "example", "void negativeIsIgnored() {\n    assertEquals(0, new BigDecimal(\"80\").compareTo(Rates.discounted(new BigDecimal(\"80.00\"), -5)));\n}"},
		{"discountReduces", "reduce the total by that percentage", "property", "void discountReduces() {\n    Random draws = new Random(2L);\n    for (int draw = 0; draw < 200; draw++) {\n        BigDecimal total = BigDecimal.valueOf(draws.nextInt(100000), 2);\n        int percent = draws.nextInt(101);\n        BigDecimal expected = total.multiply(BigDecimal.valueOf(100 - percent)).divide(BigDecimal.valueOf(100), 2, RoundingMode.HALF_UP);\n        assertEquals(0, expected.compareTo(Rates.discounted(total, percent)));\n    }\n}"},
	}},
	subjectShipping: {javaImports + "\nimport java.util.Random;", []parcelWitness{
		{"freeFromFifty", "reaches 50", "example", "void freeFromFifty() {\n    assertEquals(0, BigDecimal.ZERO.compareTo(Rates.shippingFee(new BigDecimal(\"50.00\"))));\n}"},
		{"chargedBelowFifty", "reaches 50", "example", "void chargedBelowFifty() {\n    assertEquals(0, new BigDecimal(\"4.50\").compareTo(Rates.shippingFee(new BigDecimal(\"49.99\"))));\n}"},
		{"freeForLargeOrders", "waive the shipping fee", "example", "void freeForLargeOrders() {\n    assertEquals(0, BigDecimal.ZERO.compareTo(Rates.shippingFee(new BigDecimal(\"100.00\"))));\n}"},
		{"waivedFromFifty", "waive the shipping fee", "property", "void waivedFromFifty() {\n    Random draws = new Random(3L);\n    for (int draw = 0; draw < 200; draw++) {\n        BigDecimal total = BigDecimal.valueOf(draws.nextInt(20000), 2);\n        assertEquals(total.compareTo(new BigDecimal(\"50\")) >= 0, Rates.shippingFee(total).signum() == 0);\n    }\n}"},
	}},
}

// witnessesFor picks the witness sets of the project's language by the language
// line of the prompt: Python's when it names none of the others.
func witnessesFor(language string) map[string]witnessSet {
	switch {
	case strings.Contains(language, languageGo):
		return goWitnesses
	case strings.Contains(language, languageJest):
		return typescriptWitnesses
	case strings.Contains(language, languageVitest):
		return vitestWitnesses()
	case strings.Contains(language, languageJava):
		return javaWitnesses
	}
	return pythonWitnesses
}

// WitnessAnswer answers a witness request the way an honest writer does: three
// examples and a property for the criterion to check, in the project's language.
func WitnessAnswer(user string) string {
	byLanguage := witnessesFor(prompts.LineAfter(user, languageHeader))
	target := prompts.SectionAfter(user, targetHeader)
	set := byLanguage[subjectDiscount]
	for _, subject := range []string{subjectShipping, subjectLines} {
		if strings.Contains(target, subject) {
			set = byLanguage[subject]
		}
	}
	encoded, _ := json.Marshal(map[string]any{"imports": set.imports, "witnesses": set.witnesses})
	return string(encoded)
}

// ScopeAnswer holds every witness of a scope request fixed by its criterion, as
// an honest judge does for the parcel witnesses.
func ScopeAnswer(user string) string {
	verdicts := map[string]any{}
	for _, match := range scopeLabelPattern.FindAllStringSubmatch(user, -1) {
		verdicts[match[1]] = map[string]any{"answer": scopeFixed, "confidence": scopeConfident}
	}
	encoded, _ := json.Marshal(map[string]any{"verdicts": verdicts})
	return string(encoded)
}
