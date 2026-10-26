package e2e_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/format"
)

// Answers of the hosts the end-to-end scans run with.
const (
	scopeFixed      = "FIXED"
	scopeConfidence = 0.9
	firstSlot       = "S1"
	// propertyFrom, propertyTo and propertyStep are the totals a property
	// witness runs over.
	propertyFrom = 0
	propertyTo   = 500
	propertyStep = 37
	// unknownFunction is a function id no labels request shows.
	unknownFunction = "f999"
	// unknownWitness is a witness label no scope request shows.
	unknownWitness = "w999"
	// unknownUnit is a qualified name no code holds.
	unknownUnit = "nowhere"
)

// check is one witness a writer writes: the stem of its name, the call it
// makes and the value it wants, and whether it runs over a range of totals.
type check struct {
	stem, call, want string
	property         bool
}

// promiseScript is one promise the host extracts and how it witnesses it.
type promiseScript struct {
	quote  string
	source string
	checks []check
}

// dialect is how the witnesses of one ecosystem are written.
type dialect struct {
	imports  string
	name     func(vote int, stem string) string
	example  func(name, call, want string) string
	property func(name, call, want string) string
}

// pythonDialect writes pytest witnesses that import from the module.
func pythonDialect(imports string) dialect {
	return dialect{
		imports: imports,
		name:    func(vote int, stem string) string { return fmt.Sprintf("test_v%d_%s", vote, stem) },
		example: func(name, call, want string) string {
			return fmt.Sprintf("def %s():\n    assert %s == %s\n", name, call, want)
		},
		property: func(name, call, want string) string {
			return fmt.Sprintf("def %s():\n    for total in range(%d, %d, %d):\n        assert %s == %s\n", name, propertyFrom, propertyTo, propertyStep, call, want)
		},
	}
}

// goDialect writes Go witnesses of the package under test.
func goDialect() dialect {
	return dialect{
		imports: `"testing"`,
		name: func(vote int, stem string) string {
			var name strings.Builder
			fmt.Fprintf(&name, "TestV%d", vote)
			for _, part := range strings.Split(stem, "_") {
				name.WriteString(strings.ToUpper(part[:1]) + part[1:])
			}
			return name.String()
		},
		example: func(name, call, want string) string {
			return fmt.Sprintf("func %s(t *testing.T) {\n\tif got := %s; got != %s {\n\t\tt.Fatalf(\"got %%v\", got)\n\t}\n}\n", name, call, want)
		},
		property: func(name, call, want string) string {
			return fmt.Sprintf("func %s(t *testing.T) {\n\tfor total := %d; total < %d; total += %d {\n\t\tif got := %s; got != %s {\n\t\t\tt.Fatalf(\"total %%d: got %%v\", total, got)\n\t\t}\n\t}\n}\n",
				name, propertyFrom, propertyTo, propertyStep, call, want)
		},
	}
}

// typeScriptDialect writes witnesses of a Jest or a Vitest project: plain
// functions that throw through expect, with the imports given, Vitest's expect
// among them.
func typeScriptDialect(imports string) dialect {
	return dialect{
		imports: imports,
		name:    func(vote int, stem string) string { return fmt.Sprintf("witness_v%d_%s", vote, stem) },
		example: func(name, call, want string) string {
			return fmt.Sprintf("function %s() {\n  expect(%s).toBe(%s);\n}\n", name, call, want)
		},
		property: func(name, call, want string) string {
			return fmt.Sprintf("function %s() {\n  for (let total = %d; total < %d; total += %d) {\n    expect(%s).toBe(%s);\n  }\n}\n",
				name, propertyFrom, propertyTo, propertyStep, call, want)
		},
	}
}

// vitestExpect imports Vitest's expect into a witness file.
const vitestExpect = "import { expect } from 'vitest';\n"

// witnessAnswer is the answer of one writer: the checks of the promise in the
// dialect, named after the vote so that the two writers differ.
func (d dialect) witnessAnswer(t *testing.T, vote int, checks []check) string {
	type item struct {
		Name   string `json:"name"`
		Clause string `json:"clause"`
		Kind   string `json:"kind"`
		Code   string `json:"code"`
	}
	items := make([]item, 0, len(checks))
	for _, one := range checks {
		name := d.name(vote, one.stem)
		code, kind := d.example(name, one.call, one.want), "example"
		if one.property {
			code, kind = d.property(name, one.call, one.want), "property"
		}
		items = append(items, item{Name: name, Clause: strings.ReplaceAll(one.stem, "_", " "), Kind: kind, Code: code})
	}
	return encode(t, map[string]any{"imports": d.imports, "witnesses": items})
}

// mutation is the change a generation answer makes to the unit it targets.
type mutation struct {
	// unit is the qualified name of the unit, as the code shows it.
	unit string
	// from is replaced by to, once.
	from, to string
	// mechanism says what the change does.
	mechanism string
}

// scenario is what an honest host knows of one project: the promises it
// extracts, how it writes their witnesses and the change it proposes for the
// promise it breaks.
type scenario struct {
	promises []promiseScript
	dialect  dialect
	// breaks is the quote whose generation tickets propose the mutation.
	breaks   string
	mutation mutation
	// label is the function the broken promise labels, as a signature names
	// it.
	label string
}

// checksOf are the checks of the promise whose quote a prompt asks about.
func (s scenario) checksOf(t *testing.T, user string) []check {
	t.Helper()
	quote := quoteOf(targetQuote, user)
	for _, one := range s.promises {
		if one.quote == quote {
			return one.checks
		}
	}
	t.Fatalf("a writer was asked about %q:\n%s", quote, user)
	return nil
}

func (s scenario) extraction(t *testing.T) string {
	type extracted struct {
		Quote  string `json:"quote"`
		Source string `json:"source"`
	}
	items := make([]extracted, len(s.promises))
	for index, one := range s.promises {
		items[index] = extracted{Quote: one.quote, Source: one.source}
	}
	return encode(t, map[string]any{"promises": items})
}

// honestScope holds every witness a scope request shows fixed.
func honestScope(t *testing.T, user string) string {
	verdicts := map[string]any{}
	for _, match := range scopeWitness.FindAllStringSubmatch(user, -1) {
		verdicts[match[1]] = map[string]any{"answer": scopeFixed, "confidence": scopeConfidence}
	}
	return encode(t, map[string]any{"verdicts": verdicts})
}

// generation proposes the scenario's mutation of the unit the ticket shows,
// its source ending with a line break as a model writes code; a ticket that
// shows no such unit gets no mutant.
func (s scenario) generation(t *testing.T, user string) string {
	mutants := []any{}
	for _, unit := range shownUnits(user) {
		if unit.qualname != s.mutation.unit || !strings.Contains(unit.source, s.mutation.from) {
			continue
		}
		mutants = append(mutants, map[string]any{
			"slot": firstSlot, "mechanism": s.mutation.mechanism,
			"edits": []any{map[string]any{"file": unit.file, "qualname": unit.qualname, "new_source": strings.Replace(unit.source, s.mutation.from, s.mutation.to, 1) + "\n"}},
		})
	}
	return encode(t, map[string]any{"mutants": mutants})
}

// labels labels the broken promise with the function the scenario names.
func (s scenario) labels(t *testing.T, user string) string {
	labelled := map[string][]string{}
	for _, promise := range labelPromise.FindAllStringSubmatch(user, -1) {
		if promise[2] != s.breaks {
			continue
		}
		for _, function := range labelFunction.FindAllStringSubmatch(user, -1) {
			if strings.Contains(function[2], s.label) {
				labelled[promise[1]] = append(labelled[promise[1]], function[1])
			}
		}
	}
	return encode(t, map[string]any{"labels": labelled})
}

// honest answers every ticket as a careful host would.
func (s scenario) honest(t *testing.T) persona {
	return func(ask ticketAsk) string {
		switch ask.kind {
		case answerPromises:
			return s.extraction(t)
		case answerWitnesses:
			return s.dialect.witnessAnswer(t, ask.vote, s.checksOf(t, ask.user))
		case answerScope:
			return honestScope(t, ask.user)
		case answerGeneration:
			return s.generation(t, ask.user)
		case answerLabels:
			return s.labels(t, ask.user)
		}
		t.Fatalf("a %s ticket", ask.kind)
		return ""
	}
}

// rejecting answers every ticket with an answer the schema accepts and the
// scan rejects once it reads or runs it: a writer's first set has no
// property, a scope answer judges no witness it was shown, a generation
// answer edits a unit no code holds and a labels answer names a function it
// was not shown. A writer asked again answers honestly.
func (s scenario) rejecting(t *testing.T) persona {
	honest := s.honest(t)
	return func(ask ticketAsk) string {
		switch ask.kind {
		case answerWitnesses:
			if ask.times > 0 {
				return honest(ask)
			}
			var examples []check
			for _, one := range s.checksOf(t, ask.user) {
				one.property = false
				examples = append(examples, one)
			}
			return s.dialect.witnessAnswer(t, ask.vote, append(examples, check{stem: "again", call: examples[0].call, want: examples[0].want}))
		case answerScope:
			return encode(t, map[string]any{"verdicts": map[string]any{unknownWitness: map[string]any{"answer": scopeFixed, "confidence": scopeConfidence}}})
		case answerGeneration:
			units := shownUnits(ask.user)
			return encode(t, map[string]any{"mutants": []any{map[string]any{
				"slot": firstSlot, "mechanism": s.mutation.mechanism,
				"edits": []any{map[string]any{"file": units[0].file, "qualname": unknownUnit, "new_source": units[0].source}},
			}}})
		case answerLabels:
			promise := labelPromise.FindStringSubmatch(ask.user)
			return encode(t, map[string]any{"labels": map[string][]string{promise[1]: {unknownFunction}}})
		}
		return honest(ask)
	}
}

// The promises of the README of the host fixtures.
const (
	orderQuote   = "The total of an order is the sum of its line totals."
	fortyQuote   = "Orders of forty or more ship free."
	refundsQuote = "Refunds are paid back within a day."
)

// invoiceScenario is the pytest invoice project: an order total the suite
// runs and never asserts, shipping the README misstates and a quote the
// README does not hold.
func invoiceScenario() scenario {
	return scenario{
		promises: []promiseScript{
			{quote: orderQuote, source: format.ScanSourceReadme, checks: []check{
				{stem: "one_line", call: "order_total([(4, 3)])", want: "12"},
				{stem: "two_lines", call: "order_total([(4, 3), (2, 5)])", want: "22"},
				{stem: "no_line", call: "order_total([])", want: "0"},
				{stem: "any_lines", call: "order_total([(total, 1), (1, total)])", want: "2 * total", property: true},
			}},
			{quote: fortyQuote, source: format.ScanSourceReadme, checks: fortyChecks("shipping")},
			{quote: refundsQuote, source: format.ScanSourceReadme},
		},
		dialect:  pythonDialect("from invoice.orders import order_total\nfrom invoice.totals import shipping"),
		breaks:   orderQuote,
		mutation: mutation{unit: "order_total", from: "unit_price * quantity", to: "unit_price + quantity", mechanism: "add the quantity to the unit price"},
		label:    "def order_total(",
	}
}

// fortyChecks witness free shipping from forty with the shipping function.
func fortyChecks(shipping string) []check {
	return []check{
		{stem: "forty_ships_free", call: shipping + "(40)", want: "0"},
		{stem: "forty_five_ships_free", call: shipping + "(45)", want: "0"},
		{stem: "forty_nine_ships_free", call: shipping + "(49)", want: "0"},
		{stem: "from_forty_ships_free", call: shipping + "(40 + total % 10)", want: "0", property: true},
	}
}

// tallyScenario is the Go tally module, with the promises of the invoice
// project.
func tallyScenario() scenario {
	s := invoiceScenario()
	s.promises[0].checks = []check{
		{stem: "one_line", call: "OrderTotal([]Line{{UnitPrice: 4, Quantity: 3}})", want: "12"},
		{stem: "two_lines", call: "OrderTotal([]Line{{UnitPrice: 4, Quantity: 3}, {UnitPrice: 2, Quantity: 5}})", want: "22"},
		{stem: "no_line", call: "OrderTotal(nil)", want: "0"},
		{stem: "any_lines", call: "OrderTotal([]Line{{UnitPrice: total, Quantity: 1}, {UnitPrice: 1, Quantity: total}})", want: "2 * total", property: true},
	}
	s.promises[1].checks = fortyChecks("Shipping")
	s.dialect = goDialect()
	s.mutation = mutation{unit: "tally.OrderTotal", from: "line.UnitPrice * line.Quantity", to: "line.UnitPrice + line.Quantity", mechanism: s.mutation.mechanism}
	s.label = "func OrderTotal("
	return s
}

// invoiceTypeScriptScenario is the TypeScript invoice project of a runner,
// with the promises of the pytest one; preamble is what the witness file
// imports before the project's modules.
func invoiceTypeScriptScenario(preamble string) scenario {
	s := invoiceScenario()
	s.promises[0].checks = []check{
		{stem: "one_line", call: "orderTotal([[4, 3]])", want: "12"},
		{stem: "two_lines", call: "orderTotal([[4, 3], [2, 5]])", want: "22"},
		{stem: "no_line", call: "orderTotal([])", want: "0"},
		{stem: "any_lines", call: "orderTotal([[total, 1], [1, total]])", want: "2 * total", property: true},
	}
	s.promises[1].checks = fortyChecks("shipping")
	s.dialect = typeScriptDialect(preamble + "import { orderTotal } from '../src/orders';\nimport { shipping } from '../src/totals';")
	s.mutation = mutation{unit: "orderTotal", from: "unitPrice * quantity", to: "unitPrice + quantity", mechanism: s.mutation.mechanism}
	s.label = "function orderTotal("
	return s
}

// The promises of the README the parcel projects get for a scan beside
// prove: the pytest project prices in units, the Go project in cents.
const (
	parcelQuote   = "Orders of fifty or more ship free."
	parcelGoQuote = "Orders of 5000 cents or more ship free."
)

// readmeOf is the README of a parcel project whose shipping promise is the
// quote.
func readmeOf(quote string) string {
	return "# parcel\n\nPrices a parcel order: line totals, discounts and shipping.\n\n## Shipping\n\n" + quote + "\nSmaller orders pay a flat fee.\n"
}

// parcelScenario is the parcel project of prove: free shipping from fifty,
// which the suite checks at a hundred and at ten only.
func parcelScenario() scenario {
	return scenario{
		promises: []promiseScript{{quote: parcelQuote, source: format.ScanSourceReadme, checks: []check{
			{stem: "fifty_ships_free", call: "shipping_fee(50)", want: "0"},
			{stem: "seventy_five_ships_free", call: "shipping_fee(75)", want: "0"},
			{stem: "a_hundred_ships_free", call: "shipping_fee(100)", want: "0"},
			{stem: "from_fifty_ships_free", call: "shipping_fee(50 + total)", want: "0", property: true},
		}}},
		dialect:  pythonDialect("from parcel.rates import shipping_fee"),
		breaks:   parcelQuote,
		mutation: mutation{unit: "shipping_fee", from: "total >= FREE_SHIPPING_FROM", to: "total > FREE_SHIPPING_FROM", mechanism: "charge an order of exactly fifty"},
		label:    "def shipping_fee(",
	}
}

// parcelGoScenario is the Go parcel project of prove, with the promise of
// the pytest one in cents.
func parcelGoScenario() scenario {
	return scenario{
		promises: []promiseScript{{quote: parcelGoQuote, source: format.ScanSourceReadme, checks: []check{
			{stem: "fifty_ships_free", call: "ShippingFee(5000)", want: "0"},
			{stem: "seventy_five_ships_free", call: "ShippingFee(7500)", want: "0"},
			{stem: "a_hundred_ships_free", call: "ShippingFee(10000)", want: "0"},
			{stem: "from_fifty_ships_free", call: "ShippingFee(5000 + total)", want: "0", property: true},
		}}},
		dialect:  goDialect(),
		breaks:   parcelGoQuote,
		mutation: mutation{unit: "parcel.ShippingFee", from: "total >= FreeShippingFrom", to: "total > FreeShippingFrom", mechanism: "charge an order of exactly 5000 cents"},
		label:    "func ShippingFee(",
	}
}

// parcelTypeScriptScenario is the TypeScript parcel project of a runner of
// prove, with the promise of the pytest one.
func parcelTypeScriptScenario(preamble string) scenario {
	return scenario{
		promises: []promiseScript{{quote: parcelQuote, source: format.ScanSourceReadme, checks: []check{
			{stem: "fifty_ships_free", call: "shippingFee(50)", want: "0"},
			{stem: "seventy_five_ships_free", call: "shippingFee(75)", want: "0"},
			{stem: "a_hundred_ships_free", call: "shippingFee(100)", want: "0"},
			{stem: "from_fifty_ships_free", call: "shippingFee(50 + total)", want: "0", property: true},
		}}},
		dialect:  typeScriptDialect(preamble + "import { shippingFee } from '../src/rates';"),
		breaks:   parcelQuote,
		mutation: mutation{unit: "shippingFee", from: "total >= FREE_SHIPPING_FROM", to: "total > FREE_SHIPPING_FROM", mechanism: "charge an order of exactly fifty"},
		label:    "function shippingFee(",
	}
}

// The sources of the pull request a scan reads through gh.
const (
	pullNumber      = 12
	issueNumber     = 7
	pullTitle       = "Document the discount"
	pullBody        = "Documents what `discount` does with its percent.\n\nA discount of zero percent leaves the total as it is.\n"
	issueTitle      = "What does a discount do?"
	issueBody       = "The README says nothing about shipping. Shipping is free from fifty.\n"
	zeroQuote       = "A discount of zero percent leaves the total as it is."
	fiftyQuote      = "Shipping is free from fifty."
	documentedTotal = "def discount(total: int, percent: int) -> int:\n    \"\"\"Takes percent off total, nothing at zero percent or less.\"\"\"\n"
)

// pullScenario is the invoice project changed by a pull request that
// documents the discount: one promise of its description and one of the
// issue it closes.
func pullScenario() scenario {
	return scenario{
		promises: []promiseScript{
			{quote: zeroQuote, source: format.ScanSourceDescription, checks: []check{
				{stem: "keeps_two_hundred", call: "discount(200, 0)", want: "200"},
				{stem: "keeps_seven", call: "discount(7, 0)", want: "7"},
				{stem: "keeps_nothing", call: "discount(0, 0)", want: "0"},
				{stem: "keeps_any_total", call: "discount(total, 0)", want: "total", property: true},
			}},
			{quote: fiftyQuote, source: format.ScanSourceIssue(issueNumber), checks: []check{
				{stem: "fifty_ships_free", call: "shipping(50)", want: "0"},
				{stem: "sixty_ships_free", call: "shipping(60)", want: "0"},
				{stem: "a_hundred_ships_free", call: "shipping(100)", want: "0"},
				{stem: "from_fifty_ships_free", call: "shipping(50 + total)", want: "0", property: true},
			}},
		},
		dialect:  pythonDialect("from invoice.totals import discount, shipping"),
		breaks:   zeroQuote,
		mutation: mutation{unit: "discount", from: "        return total\n", to: "        return total + 1\n", mechanism: "add one to the total kept at zero percent"},
		label:    "def discount(",
	}
}
