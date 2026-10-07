package testkit

import (
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/criterion"
	"github.com/RachidChabane/spec-proof/format"
)

// citationWord is spelled in two parts so that no source line reads as a requirement
// tag.
const citationWord = "verif" + "ies"

// citation spells the tag a test title carries for a reference.
func citation(ref criterion.Ref) string { return "[" + citationWord + " " + ref.String() + "]" }

// TypeScriptEvidence is what the other modules wrote for the TypeScript ledger: the
// example probe of the totals criterion with its unattributed mutants, and an
// ALIGNED Vitest test of the same criterion that calls the line amount.
func TypeScriptEvidence() (Power, Aligned) {
	code := []string{"src/totals.ts"}
	tests := []string{"test/totals.test.ts"}
	return Power{Ref: Totals, Digest: TotalsDigest, Examples: []int{0}, TestFiles: tests, CodeFiles: code, Unattributed: 2},
		Aligned{
			Ref: Totals, Digest: TotalsDigest, Test: TypeScriptAlignedTest(),
			Verdict: format.VerdictAligned, TestFiles: tests, CodeFiles: code,
		}
}

// TypeScriptAlignedTest is the label of the Vitest test the evidence calls ALIGNED:
// the file, then the titles of its suite and of the test joined with ` > `.
func TypeScriptAlignedTest() string {
	return "test/totals.test.ts.line amounts > multiply the price by the quantity " + citation(Totals)
}

// JavaEvidence is the same for the Java ledger, whose ALIGNED test is a JUnit 5
// method named by its display name.
func JavaEvidence() (Power, Aligned) {
	code := []string{"src/main/java/ledger/Totals.java"}
	tests := []string{"src/test/java/ledger/TotalsTest.java"}
	return Power{Ref: Totals, Digest: TotalsDigest, Examples: []int{0}, TestFiles: tests, CodeFiles: code, Unattributed: 2},
		Aligned{
			Ref: Totals, Digest: TotalsDigest, Test: JavaAlignedTest(),
			Verdict: format.VerdictAligned, TestFiles: tests, CodeFiles: code,
		}
}

// JavaAlignedTest is the label of the JUnit 5 test the evidence calls ALIGNED: the
// class, then the display name of the method.
func JavaAlignedTest() string {
	return "ledger.TotalsTest.multiplies the price by the quantity " + citation(Totals)
}
