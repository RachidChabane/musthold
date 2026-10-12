package vacuity_test

import (
	"reflect"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
	"github.com/RachidChabane/spec-proof/format"
)

const verifiesOwnedLines = " [verifies SP-2133~2]"

// outcomesOf counts the changes of a verdict by outcome.
func outcomesOf(verdict vacuity.Verdict) map[format.ScanOutcome][]string {
	byOutcome := map[format.ScanOutcome][]string{}
	for _, change := range verdict.Changes {
		byOutcome[change.Outcome] = append(byOutcome[change.Outcome], change.Mutant)
	}
	return byOutcome
}

// withOwned is the input with the test's owned lines replaced.
func withOwned(input vacuity.Input, test ecosystem.TestID, owned []vacuity.Line) vacuity.Input {
	changed := cloneInput(input)
	coverage := changed.Coverage[test]
	coverage.Owned = owned
	changed.Coverage[test] = coverage
	return changed
}

func TestOwnershipProperties(t *testing.T) {
	t.Run("owning fewer lines never creates a kill, a route or an inconclusive outcome, and never makes a test verify nothing"+verifiesOwnedLines, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			input := inputGenerator().Draw(t, "input")
			test := rapid.SampledFrom(testPool()).Draw(t, "test")
			owned := input.Coverage[test].Owned
			shrunk := withOwned(input, test, subsetOf(t, "kept", owned, 0, len(owned)))
			before, after := mustClassify(t, input), mustClassify(t, shrunk)
			if !reflect.DeepEqual(before.Routes, after.Routes) {
				t.Fatalf("owning fewer lines changed the routes")
			}
			was, is := verdictsByTest(before)[test], verdictsByTest(after)[test]
			for _, outcome := range []format.ScanOutcome{format.ScanKilled, format.ScanInconclusive} {
				if !slices.Equal(outcomesOf(was)[outcome], outcomesOf(is)[outcome]) {
					t.Fatalf("owning fewer lines changed the %s changes of %s: %v, then %v", outcome, test, outcomesOf(was)[outcome], outcomesOf(is)[outcome])
				}
			}
			if is.Class == format.ScanVerifiesNothing && was.Class != format.ScanVerifiesNothing {
				t.Fatalf("owning fewer lines made %s verify nothing, from %s", test, was.Class)
			}
			for other, verdict := range verdictsByTest(after) {
				if other != test && !reflect.DeepEqual(verdict, verdictsByTest(before)[other]) {
					t.Fatalf("owning fewer lines of %s changed the verdict of %s", test, other)
				}
			}
		})
	})

	t.Run("a line a test did not run is never one it owns"+verifiesOwnedLines, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			input := inputGenerator().Draw(t, "input")
			test := rapid.SampledFrom(testPool()).Draw(t, "test")
			coverage := input.Coverage[test]
			stray := slices.DeleteFunc(linePool(), func(l vacuity.Line) bool { return slices.Contains(coverage.Lines, l) })
			widened := withOwned(input, test, append(slices.Clone(coverage.Owned), eachOf(t, "stray", stray, ownedShare)...))
			if !sameJudgement(mustClassify(t, input), mustClassify(t, widened)) {
				t.Fatalf("a line %s did not run counted as one it owns", test)
			}
			for _, line := range widened.Coverage[test].OwnedLines() {
				if !slices.Contains(coverage.Lines, line) {
					t.Fatalf("%v is owned and not run", line)
				}
			}
		})
	})

	t.Run("the order of the lines a test ran and owns changes nothing"+verifiesOwnedLines, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			input := inputGenerator().Draw(t, "input")
			reordered := cloneInput(input)
			for test, coverage := range reordered.Coverage {
				lines, owned := slices.Clone(coverage.Lines), slices.Clone(coverage.Owned)
				slices.Reverse(lines)
				slices.Reverse(owned)
				reordered.Coverage[test] = vacuity.Coverage{Lines: lines, Owned: owned}
			}
			if !sameJudgement(mustClassify(t, input), mustClassify(t, reordered)) {
				t.Fatal("reordering the lines changed the judgement")
			}
		})
	})
}
