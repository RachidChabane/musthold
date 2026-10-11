package vacuity_test

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
	"github.com/RachidChabane/spec-proof/format"
)

// Bounds of the generated scans.
const (
	generatedTests     = 6
	generatedFiles     = 2
	generatedLines     = 4
	minMutants         = 12
	maxMutants         = 20
	minMutantLines     = 1
	maxMutantLines     = 2
	minDifferences     = 1
	maxDifferences     = 2
	maxWitnessFailures = 1
	generatedWitnesses = 2
	generatedPromise   = "P1"
)

var allRuns = []vacuity.Run{vacuity.RunPassed, vacuity.RunFailed, vacuity.RunTimedOut, vacuity.RunErrored, vacuity.RunNotStarted}

func testPool() []ecosystem.TestID {
	tests := make([]ecosystem.TestID, generatedTests)
	for index := range tests {
		tests[index] = ecosystem.TestID(fmt.Sprintf("tests/test_%d.py::test_%d", index%generatedFiles, index))
	}
	return tests
}

func linePool() []vacuity.Line {
	var lines []vacuity.Line
	for file := range generatedFiles {
		for number := 1; number <= generatedLines; number++ {
			lines = append(lines, line(fmt.Sprintf("src/module_%d.py", file), number))
		}
	}
	return lines
}

func witnessPool() []string {
	pool := make([]string, generatedWitnesses)
	for index := range pool {
		pool[index] = format.SHA256Hex([]byte(fmt.Sprintf("witness %d", index)))
	}
	return pool
}

// scanEdits is the one edit that names a generated mutant: its id is the digest
// of the edit, as in a scan statement.
func scanEdits(label string) []format.ScanEdit {
	return []format.ScanEdit{{File: "src/module_0.py", Qualname: "unit", NewSource: "def unit():\n    return " + label}}
}

func generatedID(label string) string {
	id, err := format.ScanEditsDigest(scanEdits(label))
	if err != nil {
		panic(err)
	}
	return id
}

// Shares, in percent, of the generated runs: most tests pass the reference and
// survive a mutant, so that every class is drawn.
const (
	percent               = 100
	greenShare            = 85
	survivedShare         = 82
	killedShare           = 9
	notStartedShare       = 4
	ownedShare            = 80
	likelyShare           = 85
	sentinelShare         = 10
	identicalShare        = 5
	ranAgainstShare       = 95
	sampledShare          = 70
	sentinelSurvivedShare = 95
	coveredShare          = 60
	unsettledMaxRuns      = vacuity.RunsPerOutcome + 1
)

// chance draws whether an event of the given share in percent happens.
func chance(t *rapid.T, label string, share int) bool {
	return rapid.IntRange(0, percent-1).Draw(t, label) < share
}

// referenceRuns are mostly green.
func referenceRuns(t *rapid.T) []vacuity.Run {
	if chance(t, "green", greenShare) {
		return runs(vacuity.RunPassed)
	}
	return rapid.SliceOfN(rapid.SampledFrom(allRuns), 0, unsettledMaxRuns).Draw(t, "unsettled")
}

// mutantRuns mostly survive, sometimes kill, and otherwise are any runs; a
// sentinel is seldom failed.
func mutantRuns(t *rapid.T, sentinel bool) []vacuity.Run {
	if sentinel && chance(t, "sentinelSurvived", sentinelSurvivedShare) {
		return runs(vacuity.RunPassed)
	}
	switch draw := rapid.IntRange(0, percent-1).Draw(t, "outcome"); {
	case draw < survivedShare:
		return runs(vacuity.RunPassed)
	case draw < survivedShare+killedShare:
		return runs(vacuity.RunFailed)
	case draw < survivedShare+killedShare+notStartedShare:
		return runs(vacuity.RunNotStarted)
	}
	return rapid.SliceOfN(rapid.SampledFrom(allRuns), 0, unsettledMaxRuns).Draw(t, "unsettled")
}

// subsetOf draws between minLength and maxLength members of the pool, in any
// order.
func subsetOf[T any](t *rapid.T, label string, pool []T, minLength, maxLength int) []T {
	indices := rapid.SliceOfNDistinct(rapid.IntRange(0, len(pool)-1), minLength, maxLength, rapid.ID[int]).Draw(t, label)
	out := make([]T, len(indices))
	for position, index := range indices {
		out[position] = pool[index]
	}
	return out
}

// eachOf keeps each member of the pool with the given share in percent.
func eachOf[T any](t *rapid.T, label string, pool []T, share int) []T {
	var kept []T
	for _, member := range pool {
		if chance(t, label, share) {
			kept = append(kept, member)
		}
	}
	return kept
}

func mutantGenerator(label string) *rapid.Generator[vacuity.Mutant] {
	return rapid.Custom(func(t *rapid.T) vacuity.Mutant {
		mutant := vacuity.Mutant{
			ID:                generatedID(label),
			Lines:             subsetOf(t, "lines", linePool(), minMutantLines, maxMutantLines),
			Sentinel:          chance(t, "sentinel", sentinelShare),
			CompiledIdentical: chance(t, "identical", identicalShare),
		}
		for range rapid.IntRange(minDifferences, maxDifferences).Draw(t, "differences") {
			mutant.Differences = append(mutant.Differences, vacuity.Difference{
				Operation:       rapid.SampledFrom([]string{"", "split_fields", "total"}).Draw(t, "operation"),
				Input:           rapid.SampledFrom([]string{"", `"a,"`, "0", "1"}).Draw(t, "input"),
				Channel:         rapid.SampledFrom([]format.ScanChannel{format.ScanChannelReturnValue, format.ScanChannelException, format.ScanChannelArgumentMutation, "stdout"}).Draw(t, "channel"),
				Reference:       rapid.SampledFrom([]string{"1", "2", "3"}).Draw(t, "reference"),
				Mutant:          rapid.SampledFrom([]string{"1", "2", "3"}).Draw(t, "mutant"),
				Reproduced:      chance(t, "reproduced", likelyShare),
				ReferenceAgrees: chance(t, "agrees", likelyShare),
			})
		}
		for range rapid.IntRange(0, maxWitnessFailures).Draw(t, "witnessFailures") {
			mutant.WitnessFailures = append(mutant.WitnessFailures, vacuity.WitnessFailure{
				Witness:    rapid.SampledFrom(witnessPool()).Draw(t, "witness"),
				Admitted:   rapid.Bool().Draw(t, "admitted"),
				Blind:      rapid.Bool().Draw(t, "blind"),
				Reproduced: rapid.Bool().Draw(t, "witnessReproduced"),
			})
		}
		return mutant
	})
}

func inputGenerator() *rapid.Generator[vacuity.Input] {
	return rapid.Custom(func(t *rapid.T) vacuity.Input {
		tests := testPool()
		input := vacuity.Input{
			Reference: map[ecosystem.TestID][]vacuity.Run{},
			Coverage:  map[ecosystem.TestID]vacuity.Coverage{},
			Outcomes:  map[string]map[ecosystem.TestID][]vacuity.Run{},
			Sampled:   eachOf(t, "sampled", tests, sampledShare),
		}
		for _, test := range tests {
			if chance(t, "known", likelyShare) {
				input.Reference[test] = referenceRuns(t)
			}
			lines := eachOf(t, "coverage", linePool(), coveredShare)
			input.Coverage[test] = vacuity.Coverage{Lines: lines, Owned: eachOf(t, "owned", lines, ownedShare)}
		}
		for index := range rapid.IntRange(minMutants, maxMutants).Draw(t, "mutants") {
			mutant := mutantGenerator(fmt.Sprintf("m%d", index)).Draw(t, "mutant")
			input.Mutants = append(input.Mutants, mutant)
			input.Outcomes[mutant.ID] = map[ecosystem.TestID][]vacuity.Run{}
			for _, test := range eachOf(t, "ranAgainst", tests, ranAgainstShare) {
				input.Outcomes[mutant.ID][test] = mutantRuns(t, mutant.Sentinel)
			}
		}
		return input
	})
}

func mustClassify(t *rapid.T, input vacuity.Input) vacuity.Result {
	result, err := vacuity.Classify(input)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func verdictsByTest(result vacuity.Result) map[ecosystem.TestID]vacuity.Verdict {
	byTest := map[ecosystem.TestID]vacuity.Verdict{}
	for _, verdict := range result.Verdicts {
		byTest[verdict.Test] = verdict
	}
	return byTest
}

func cloneInput(input vacuity.Input) vacuity.Input {
	clone := input
	clone.Reference = maps.Clone(input.Reference)
	clone.Coverage = maps.Clone(input.Coverage)
	clone.Sampled = slices.Clone(input.Sampled)
	clone.Mutants = slices.Clone(input.Mutants)
	clone.Outcomes = map[string]map[ecosystem.TestID][]vacuity.Run{}
	for id, byTest := range input.Outcomes {
		clone.Outcomes[id] = maps.Clone(byTest)
	}
	return clone
}

func TestMonotonicity(t *testing.T) {
	t.Run("a killing outcome of a test never moves that test into verifies nothing [verifies SP-2028~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			input := inputGenerator().Draw(t, "input")
			if len(input.Mutants) == 0 {
				return
			}
			test := rapid.SampledFrom(testPool()).Draw(t, "test")
			mutant := rapid.SampledFrom(input.Mutants).Draw(t, "killed")
			before := verdictsByTest(mustClassify(t, input))[test]
			killed := cloneInput(input)
			if killed.Outcomes[mutant.ID] == nil {
				killed.Outcomes[mutant.ID] = map[ecosystem.TestID][]vacuity.Run{}
			}
			killed.Outcomes[mutant.ID][test] = runs(vacuity.RunFailed)
			after := verdictsByTest(mustClassify(t, killed))[test]
			if after.Class == format.ScanVerifiesNothing && before.Class != format.ScanVerifiesNothing {
				t.Fatalf("a kill of %s moved %s from %s into verifies nothing", mutant.ID, test, before.Class)
			}
		})
	})

	t.Run("an unproven mutant changes no verdict and no count [verifies SP-2028~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			input := inputGenerator().Draw(t, "input")
			result := mustClassify(t, input)
			var unproven []vacuity.Mutant
			for _, mutant := range input.Mutants {
				if !mutant.Sentinel && !result.Proven(mutant.ID) {
					unproven = append(unproven, mutant)
				}
			}
			if len(unproven) == 0 {
				return
			}
			dropped := rapid.SampledFrom(unproven).Draw(t, "dropped")
			without := cloneInput(input)
			without.Mutants = slices.DeleteFunc(without.Mutants, func(m vacuity.Mutant) bool { return m.ID == dropped.ID })
			delete(without.Outcomes, dropped.ID)
			if after := mustClassify(t, without); !reflect.DeepEqual(after.Verdicts, result.Verdicts) || after.Counts != result.Counts {
				t.Fatalf("dropping the unproven %s changed the result", dropped.ID)
			}
		})
	})
}

func TestDeterminism(t *testing.T) {
	t.Run("the result does not depend on the order of any list of the input [verifies SP-2027~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			input := inputGenerator().Draw(t, "input")
			shuffled := cloneInput(input)
			shuffled.Sampled = rapid.Permutation(input.Sampled).Draw(t, "sampled")
			shuffled.Mutants = rapid.Permutation(input.Mutants).Draw(t, "mutants")
			for index, mutant := range shuffled.Mutants {
				mutant.Lines = rapid.Permutation(mutant.Lines).Draw(t, "lines")
				mutant.Differences = rapid.Permutation(mutant.Differences).Draw(t, "differences")
				shuffled.Mutants[index] = mutant
			}
			for test, coverage := range input.Coverage {
				shuffled.Coverage[test] = vacuity.Coverage{
					Lines: rapid.Permutation(coverage.Lines).Draw(t, "coverage"), Owned: rapid.Permutation(coverage.Owned).Draw(t, "owned"),
				}
			}
			if !reflect.DeepEqual(mustClassify(t, input), mustClassify(t, shuffled)) {
				t.Fatal("a permutation of the input changed the result")
			}
		})
	})

	t.Run("verdicts are ranked by class, then most changes, then test id [verifies SP-2027~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			verdicts := mustClassify(t, inputGenerator().Draw(t, "input")).Verdicts
			for index := 1; index < len(verdicts); index++ {
				previous, current := verdicts[index-1], verdicts[index]
				byClass := slices.Index(format.ScanClasses(), previous.Class) - slices.Index(format.ScanClasses(), current.Class)
				switch {
				case byClass < 0:
				case byClass > 0:
					t.Fatalf("%s before %s", previous.Class, current.Class)
				case len(previous.Changes) < len(current.Changes):
					t.Fatalf("%s with fewer changes before %s", previous.Test, current.Test)
				case len(previous.Changes) == len(current.Changes) && previous.Test >= current.Test:
					t.Fatalf("%s before %s", previous.Test, current.Test)
				}
			}
		})
	})
}

func TestSoundness(t *testing.T) {
	t.Run("a test said to verify nothing is green, sampled and survives enough proven changes it ran [verifies SP-2025~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			input := inputGenerator().Draw(t, "input")
			result := mustClassify(t, input)
			for _, verdict := range result.Verdicts {
				for _, change := range verdict.Changes {
					if !result.Proven(change.Mutant) {
						t.Fatalf("%s counts the unproven %s", verdict.Test, change.Mutant)
					}
				}
				if verdict.Class != format.ScanVerifiesNothing {
					continue
				}
				if !vacuity.Green(input.Reference[verdict.Test]) || !slices.Contains(input.Sampled, verdict.Test) {
					t.Fatalf("%s is not a sampled green test", verdict.Test)
				}
				if len(verdict.Changes) < vacuity.MinChanges || verdict.Sites() < vacuity.MinSites {
					t.Fatalf("%s: %d changes on %d sites", verdict.Test, len(verdict.Changes), verdict.Sites())
				}
				for _, change := range verdict.Changes {
					if change.Outcome != format.ScanSurvived || vacuity.Settle(input.Outcomes[change.Mutant][verdict.Test]) != vacuity.SettledSurvived {
						t.Fatalf("%s did not survive %s", verdict.Test, change.Mutant)
					}
				}
			}
		})
	})

	t.Run("a mutant is proven only by a route the rules accept [verifies SP-2023~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			input := inputGenerator().Draw(t, "input")
			result := mustClassify(t, input)
			for _, mutant := range input.Mutants {
				if (mutant.Sentinel || mutant.CompiledIdentical) && result.Proven(mutant.ID) {
					t.Fatalf("the sentinel or identical %s is proven", mutant.ID)
				}
				for _, route := range result.Routes[mutant.ID] {
					if route.Kind == format.ScanRouteKilledByTest {
						killer := ecosystem.TestID(route.Test)
						if !vacuity.Green(input.Reference[killer]) || vacuity.Settle(input.Outcomes[mutant.ID][killer]) != vacuity.SettledKilled {
							t.Fatalf("%s proves %s without a green 3/3 kill", killer, mutant.ID)
						}
					}
				}
			}
		})
	})
}

func TestCountProperties(t *testing.T) {
	t.Run("the counts add up to the suite and the judged tests are the sampled green ones [verifies SP-2026~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			input := inputGenerator().Draw(t, "input")
			result := mustClassify(t, input)
			counts := result.Counts
			if counts.Tests != len(result.Verdicts) {
				t.Fatalf("%d tests counted, %d verdicts", counts.Tests, len(result.Verdicts))
			}
			green, sampledGreen := 0, 0
			for _, verdict := range result.Verdicts {
				if vacuity.Green(input.Reference[verdict.Test]) {
					green++
					if slices.Contains(input.Sampled, verdict.Test) {
						sampledGreen++
					}
				}
			}
			if counts.Judged() != sampledGreen || counts.NotSampled != green-sampledGreen || counts.NotGreen != counts.Tests-green {
				t.Fatalf("counts %+v for %d green and %d sampled green tests", counts, green, sampledGreen)
			}
		})
	})
}

func TestScanStatementOfAResult(t *testing.T) {
	t.Run("a scan statement built from any result validates in the format [verifies SP-2029~2]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			input := inputGenerator().Draw(t, "input")
			result := mustClassify(t, input)
			statement := format.NewScanStatement(strings.Repeat("a", 40), format.SHA256Hex([]byte("tree")), format.Scan{
				Module: format.Module{Name: "spec-proof-dp", Version: "0.1.0"}, ProducedBy: format.ProducedByLocal,
				Mode: format.ScanModeRepository, Runner: "pytest", Seed: "7", PromisesChecked: true,
				Counts: result.Counts, Tests: result.ScanTests(),
				Promises: []format.ScanPromise{{ID: generatedPromise, Quote: "keeps empty fields", Source: format.ScanSource{Name: "README.md", Digest: format.Digest{SHA256: format.SHA256Hex([]byte("readme"))}}}},
			})
			for index, witness := range witnessPool() {
				statement.Predicate.Witnesses = append(statement.Predicate.Witnesses, format.ScanWitness{
					ID: witness, Promise: generatedPromise, Name: fmt.Sprintf("test_witness_%d", index), Imports: "import unit", Code: "assert True", Admitted: true,
				})
			}
			slices.SortFunc(statement.Predicate.Witnesses, func(a, b format.ScanWitness) int { return strings.Compare(a.ID, b.ID) })
			for index, mutant := range input.Mutants {
				lines := slices.Clone(mutant.Lines)
				slices.SortFunc(lines, format.CompareScanLines)
				if len(lines) == 0 {
					continue
				}
				statement.Predicate.Mutants = append(statement.Predicate.Mutants, format.ScanMutant{
					ID: mutant.ID, Origin: format.ScanOriginOperator, Sentinel: mutant.Sentinel,
					Edits: scanEdits(fmt.Sprintf("m%d", index)), Lines: lines, Routes: result.Routes[mutant.ID],
				})
			}
			slices.SortFunc(statement.Predicate.Mutants, func(a, b format.ScanMutant) int { return strings.Compare(a.ID, b.ID) })
			if err := statement.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	})
}

// startUpLine is a line no generated test runs, for a mutant added on top of a
// generated scan.
var startUpLine = line("src/start_up.py", 1)

// withStartUpPair adds to the input a mutant proven by a difference on
// startUpLine and, when runs are given, the runs of the test on it and the
// line in the test's coverage, owned or not.
func withStartUpPair(input vacuity.Input, test ecosystem.TestID, runsOnIt []vacuity.Run, owned bool) (vacuity.Input, string) {
	added := cloneInput(input)
	id := generatedID("start-up")
	added.Mutants = append(added.Mutants, vacuity.Mutant{ID: id, Lines: []vacuity.Line{startUpLine}, Differences: []vacuity.Difference{differs()}})
	added.Outcomes[id] = map[ecosystem.TestID][]vacuity.Run{}
	if runsOnIt == nil {
		return added, id
	}
	added.Outcomes[id][test] = runsOnIt
	coverage := added.Coverage[test]
	coverage.Lines = append(slices.Clone(coverage.Lines), startUpLine)
	if owned {
		coverage.Owned = append(slices.Clone(coverage.Owned), startUpLine)
	}
	added.Coverage[test] = coverage
	return added, id
}

// sameJudgement reports whether two results give every test the same class,
// reason and changes, the same counts and the same routes.
func sameJudgement(a, b vacuity.Result) bool {
	if a.Counts != b.Counts || !reflect.DeepEqual(a.Routes, b.Routes) || len(a.Verdicts) != len(b.Verdicts) {
		return false
	}
	for index := range a.Verdicts {
		left, right := a.Verdicts[index], b.Verdicts[index]
		if left.Test != right.Test || left.Class != right.Class || left.Reason != right.Reason || !slices.Equal(left.Changes, right.Changes) {
			return false
		}
	}
	return true
}

func ownsOneOf(coverage vacuity.Coverage, lines []vacuity.Line) bool {
	return slices.ContainsFunc(lines, func(l vacuity.Line) bool { return slices.Contains(coverage.Owned, l) })
}

func TestStartUpProperties(t *testing.T) {
	t.Run("a survival on a line the test ran only at start-up changes no class and creates no route [verifies SP-2133~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			input := inputGenerator().Draw(t, "input")
			test := rapid.SampledFrom(testPool()).Draw(t, "test")
			without, _ := withStartUpPair(input, test, nil, false)
			with, _ := withStartUpPair(input, test, runs(vacuity.RunPassed), false)
			if !sameJudgement(mustClassify(t, without), mustClassify(t, with)) {
				t.Fatalf("a start-up-only survival of %s changed the judgement", test)
			}
		})
	})

	t.Run("runs that did not start change no class and create no route [verifies SP-2133~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			input := inputGenerator().Draw(t, "input")
			test := rapid.SampledFrom(testPool()).Draw(t, "test")
			owned := rapid.Bool().Draw(t, "owned")
			without, _ := withStartUpPair(input, test, nil, false)
			with, id := withStartUpPair(input, test, runs(vacuity.RunNotStarted), owned)
			result := mustClassify(t, with)
			if !sameJudgement(mustClassify(t, without), result) {
				t.Fatalf("runs of %s that did not start changed the judgement", test)
			}
			verdict := verdictsByTest(result)[test]
			listed := slices.Contains(verdict.NotStarted, id)
			if want := owned && slices.Contains(format.ScanVerdicts(), verdict.Class); listed != want {
				t.Fatalf("listed %v, want %v", listed, want)
			}
		})
	})

	t.Run("no survival on a line a test does not own is one of its changes [verifies SP-2133~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			input := inputGenerator().Draw(t, "input")
			byID := map[string]vacuity.Mutant{}
			for _, mutant := range input.Mutants {
				byID[mutant.ID] = mutant
			}
			for _, verdict := range mustClassify(t, input).Verdicts {
				for _, change := range verdict.Changes {
					if change.Outcome == format.ScanSurvived && !ownsOneOf(input.Coverage[verdict.Test], byID[change.Mutant].Lines) {
						t.Fatalf("%s counts the survival %s on a line it does not own", verdict.Test, change.Mutant)
					}
				}
			}
		})
	})

	t.Run("the changes of a test that verifies nothing are its survivals on the lines it owns [verifies SP-2133~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			input := inputGenerator().Draw(t, "input")
			result := mustClassify(t, input)
			for _, verdict := range result.Verdicts {
				if verdict.Class != format.ScanVerifiesNothing {
					continue
				}
				survivals := 0
				for _, mutant := range input.Mutants {
					settled := vacuity.Settle(input.Outcomes[mutant.ID][verdict.Test])
					if result.Proven(mutant.ID) && ownsOneOf(input.Coverage[verdict.Test], mutant.Lines) && settled == vacuity.SettledSurvived {
						survivals++
					}
				}
				if len(verdict.Changes) != survivals {
					t.Fatalf("%s verifies nothing on %d changes, %d owned survivals", verdict.Test, len(verdict.Changes), survivals)
				}
			}
		})
	})
}
