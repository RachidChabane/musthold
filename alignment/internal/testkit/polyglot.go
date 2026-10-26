package testkit

import "github.com/RachidChabane/musthold/alignment/internal/domain/criterion"

// Test is a test of a fixture project: its identity in the per-test results of a
// discriminating-power Statement, `classname::name`, and its label in an alignment
// Statement, `classname.name`.
type Test struct {
	ID, Label string
}

func newTest(classname, name string) Test {
	return Test{ID: classname + "::" + name, Label: classname + "." + name}
}

// Language is a fixture project written for a test runner with the roles of the
// parcel projects: two tests that check the multiplication, one that checks the
// rejection under the multiplication, one that checks the surcharge, one that
// checks nothing of it, and one that checks the rejection under its own criterion.
type Language struct {
	Fixture            string
	TestFile, CodeFile string
	Multiplies         Test
	SingleUnit         Test
	RejectedLine       Test
	Remote             Test
	Number             Test
	ZeroRejected       Test
}

// Fixture projects of the languages the module masks besides Python and Go.
const (
	TypeScriptParcel = "parcel-ts"
	JavaParcel       = "parcel-java"
)

// citation spells the tag a test title carries for a reference.
func citation(ref string) string { return "[" + citationWord + " " + ref + "]" }

func titled(title string, ref criterion.Ref) string { return title + " " + citation(ref.String()) }

// TypeScript is the Vitest project: its tests are named by the titles of their
// suites joined with ` > ` and by the file they are in.
func TypeScript() Language {
	const file = "test/pricing.test.ts"
	name := func(suite, title string, ref criterion.Ref) Test {
		return newTest(file, suite+" > "+titled(title, ref))
	}
	return Language{
		Fixture: TypeScriptParcel, TestFile: file, CodeFile: "src/pricing.ts",
		Multiplies:   name("line totals", "multiply the unit price by the quantity", Multiply),
		SingleUnit:   name("line totals", "cost the price of a single unit", Multiply),
		RejectedLine: name("line totals", "refuse a negative quantity", Multiply),
		Remote:       name("shipping", "adds the remote surcharge", Surcharge),
		Number:       name("shipping", "quotes a number", Surcharge),
		ZeroRejected: name("line totals", "refuse a zero quantity", Reject),
	}
}

// Java is the JUnit 5 project: its tests are named by their display names and by the
// class they are in.
func Java() Language {
	const class = "parcel.PricingTest"
	name := func(title string, ref criterion.Ref) Test { return newTest(class, titled(title, ref)) }
	return Language{
		Fixture: JavaParcel, TestFile: "src/test/java/parcel/PricingTest.java", CodeFile: "src/main/java/parcel/Pricing.java",
		Multiplies:   name("multiplies the unit price by the quantity", Multiply),
		SingleUnit:   name("costs the price of a single unit", Multiply),
		RejectedLine: name("refuses a negative quantity", Multiply),
		Remote:       name("adds the remote surcharge", Surcharge),
		Number:       name("quotes a number", Surcharge),
		ZeroRejected: name("refuses a zero quantity", Reject),
	}
}

// Powers returns the discriminating-power Statements of the project: every test but
// the two that check nothing of their criterion kills the mutants of its criterion.
func (l Language) Powers() []Power {
	files := func(p Power) Power {
		p.TestFiles, p.CodeFiles = []string{l.TestFile}, []string{l.CodeFile}
		return p
	}
	return []Power{
		files(Power{Ref: Multiply, StatementDigest: MultiplyDigest, Tests: []string{l.Multiplies.ID, l.RejectedLine.ID, l.SingleUnit.ID}, Mutants: []Mutant{
			{Digest: "m1", Kind: ValidJudged, Killers: []string{l.Multiplies.ID, l.SingleUnit.ID}},
			{Digest: "m2", Kind: ValidExample, Killers: []string{l.Multiplies.ID, l.SingleUnit.ID}},
			{Digest: "m3", Kind: ValidJudged, Killers: []string{l.Multiplies.ID}},
			{Digest: "s1", Kind: Sentinel},
		}}),
		files(Power{Ref: Surcharge, StatementDigest: SurchargeDigest, Tests: []string{l.Number.ID, l.Remote.ID}, Mutants: []Mutant{
			{Digest: "p1", Kind: ValidJudged, Killers: []string{l.Remote.ID}},
			{Digest: "p2", Kind: ValidJudged, Killers: []string{l.Remote.ID}},
			{Digest: "s1", Kind: Sentinel},
		}}),
		files(Power{Ref: Reject, StatementDigest: RejectDigest, Tests: []string{l.ZeroRejected.ID}, Mutants: []Mutant{
			{Digest: "q1", Kind: ValidExample, Killers: []string{l.ZeroRejected.ID}},
			{Digest: "q2", Kind: ValidJudged, Killers: []string{l.ZeroRejected.ID}},
			{Digest: "s1", Kind: Sentinel},
		}}),
	}
}

// Verdicts is what an honest host finds in the project, by `criterion label`: the
// tests that check their criterion are ALIGNED, the test that checks the rejection
// under the multiplication criterion and the test that checks nothing of the
// surcharge are MISALIGNED.
func (l Language) Verdicts() map[string]string {
	return map[string]string{
		Multiply.String() + " " + l.Multiplies.Label:   "ALIGNED",
		Multiply.String() + " " + l.SingleUnit.Label:   "ALIGNED",
		Multiply.String() + " " + l.RejectedLine.Label: "MISALIGNED",
		Surcharge.String() + " " + l.Remote.Label:      "ALIGNED",
		Surcharge.String() + " " + l.Number.Label:      "MISALIGNED",
		Reject.String() + " " + l.ZeroRejected.Label:   "ALIGNED",
	}
}
