package prove

import (
	"encoding/json"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/execution"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/coverage"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/footprint"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostbridge"
)

const (
	sourceFile = "parcel/rates.py"
	testFile   = "tests/test_rates.py"
	testName   = "tests.test_rates::test_shipping"
	operation  = "shipping_fee"
)

// fixture is a project and the run over one criterion of it, small enough to
// change one input at a time.
type fixture struct {
	project *project
	run     *critRun
}

func newFixture() fixture {
	criterion := spec.Criterion{
		Ref:             spec.Ref{ID: "PAR-3", Revision: 1},
		StatementDigest: format.SHA256Hex([]byte("statement")),
		Examples:        []spec.Example{{Index: 0, Call: spec.Call{Operation: operation, Args: map[string]spec.Literal{"total": {Type: spec.TypeInteger, Value: "50"}}}, Expect: spec.Observation{Kind: spec.Returns, Type: spec.TypeInteger, Value: "0"}}},
		Tests:           []ecosystem.BoundTest{ecosystem.NewBoundTest("tests.test_rates", "test_shipping", testFile, 7)},
	}
	p := &project{
		settings:          Settings{Policy: policy.Default(), Operations: execution.Operations{operation: "parcel.rates:shipping_fee"}, Seed: 1},
		criteria:          []spec.Criterion{criterion},
		sources:           []mutation.SourceFile{{Path: sourceFile, Content: []byte("FREE = 50\n")}},
		testFileDigests:   map[string]string{testFile: format.SHA256Hex([]byte("tests"))},
		referenceOutcomes: map[ecosystem.TestID]ecosystem.TestOutcome{testName: ecosystem.TestPassed},
	}
	run := &critRun{p: p, c: criterion, st: &staticAnalysis{criterion: criterion, codeFiles: []string{sourceFile}, testFiles: []string{testFile}}, rules: p.settings.Policy, poolByID: map[string]*poolMutant{}}
	return fixture{project: p, run: run}
}

// failer is what the cache key helper needs of a test: both testing.T and
// rapid.T satisfy it.
type failer interface {
	Helper()
	Fatal(args ...any)
}

// cacheKey computes the cache key of the Statement the fixture's run would write.
func (f fixture) cacheKey(t failer) string {
	t.Helper()
	base, err := f.run.baseInputs()
	if err != nil {
		t.Fatal(err)
	}
	key, err := format.CacheKey(f.run.statementSubject(), f.run.finalInputs(base))
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func answerOf(model string, output string) hostbridge.Answer {
	id := hostproto.RequestID(format.SHA256Hex([]byte("request")))
	return hostbridge.Answer{
		ID: id,
		Votes: []hostbridge.VoteState{{
			Ticket: hostproto.TicketID{RequestID: id, Emission: hostproto.FirstEmission},
			State:  hostproto.StateValidated, Output: json.RawMessage(output),
			Executor: format.ExecutorProfile{Channel: format.ChannelHostInline, Harness: format.Harness{Name: "h"}, Isolation: format.IsolationInline, Model: format.ExecutorModel{Name: model, Source: format.ModelSourceDeclared}},
		}},
	}
}

func TestCacheKey(t *testing.T) {
	changes := []struct {
		name   string
		change func(f *fixture)
	}{
		{"the statement", func(f *fixture) { f.run.c.StatementDigest = format.SHA256Hex([]byte("another statement")) }},
		{"a test source", func(f *fixture) { f.project.testFileDigests[testFile] = format.SHA256Hex([]byte("other tests")) }},
		{"the code tree", func(f *fixture) { f.project.sources[0].Content = []byte("FREE = 51\n") }},
		{"a code tree file", func(f *fixture) {
			f.run.st.codeFiles = []string{sourceFile, "parcel/other.py"}
			f.project.sources = append(f.project.sources, mutation.SourceFile{Path: "parcel/other.py", Content: []byte("X = 1\n")})
		}},
		{"the reference results", func(f *fixture) { f.project.referenceOutcomes[testName] = ecosystem.TestFailed }},
		{"the examples", func(f *fixture) { f.project.criteria[0].Examples[0].Expect.Value = "1" }},
		{"the operation bindings", func(f *fixture) {
			f.project.settings.Operations = execution.Operations{operation: "parcel.rates:other"}
		}},
		{"the policy", func(f *fixture) { f.run.rules.TauJudge = 0.8 }},
		{"the mutant set", func(f *fixture) {
			f.run.add(&poolMutant{spec: execution.MutantSpec{ID: format.SHA256Hex([]byte("mutant"))}})
		}},
		{"the models", func(f *fixture) { f.run.answers = []hostbridge.Answer{answerOf("model-a", `{}`)} }},
		{"the host responses", func(f *fixture) { f.run.answers = []hostbridge.Answer{answerOf("", `{"validity":"VALID"}`)} }},
	}
	t.Run("a changed input gives another cache key [verifies SP-186~1]", func(t *testing.T) {
		unchanged := newFixture().cacheKey(t)
		if again := newFixture().cacheKey(t); again != unchanged {
			t.Fatalf("the cache key is not deterministic: %s then %s", unchanged, again)
		}
		for _, c := range changes {
			t.Run(c.name, func(t *testing.T) {
				f := newFixture()
				c.change(&f)
				if f.cacheKey(t) == unchanged {
					t.Fatalf("changing %s kept the cache key", c.name)
				}
			})
		}
	})

	t.Run("two different texts of an input never share a cache key [verifies SP-186~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			first := rapid.StringN(0, 40, -1).Draw(t, "first")
			second := rapid.StringN(0, 40, -1).Draw(t, "second")
			a, b := newFixture(), newFixture()
			a.project.sources[0].Content, b.project.sources[0].Content = []byte(first), []byte(second)
			if (first == second) != (a.cacheKey(t) == b.cacheKey(t)) {
				t.Fatalf("code %q and %q: equal texts %t, equal keys %t", first, second, first == second, a.cacheKey(t) == b.cacheKey(t))
			}
		})
	})

	t.Run("the order the answers were consumed in never changes the cache key [verifies SP-185~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			outputs := rapid.SliceOfNDistinct(rapid.StringMatching(`[a-z]{1,8}`), 2, 6, func(s string) string { return s }).Draw(t, "outputs")
			var answers []hostbridge.Answer
			for _, output := range outputs {
				answers = append(answers, answerOf("model", `{"v":"`+output+`"}`))
				answers[len(answers)-1].ID = hostproto.RequestID(format.SHA256Hex([]byte(output)))
			}
			shuffled := rapid.Permutation(answers).Draw(t, "order")
			a, b := newFixture(), newFixture()
			a.run.answers, b.run.answers = answers, shuffled
			if a.cacheKey(t) != b.cacheKey(t) {
				t.Fatal("the cache key depends on the consumption order")
			}
		})
	})
}

const (
	otherTestFile  = "tests/test_other.py"
	unrelatedFile  = "tests/test_unrelated.py"
	reachingName   = "tests.test_other::test_reaches"
	unrelatedName  = "tests.test_unrelated::test_elsewhere"
	footprintUnit  = "shipping_fee"
	elsewhereUnit  = "greeting"
	verifiesInputs = " [verifies SP-407~1]"
)

// withOtherTests adds to the fixture the tests of two more files, one that
// executes the criterion's footprint and one that does not.
func withOtherTests(f *fixture) footprint.Footprint {
	reaching := ecosystem.NewBoundTest("tests.test_other", "test_reaches", otherTestFile, 3)
	unrelated := ecosystem.NewBoundTest("tests.test_unrelated", "test_elsewhere", unrelatedFile, 3)
	f.project.tests = append(f.project.tests, f.run.c.Tests[0], reaching, unrelated)
	f.project.testFileDigests[otherTestFile] = format.SHA256Hex([]byte("other"))
	f.project.testFileDigests[unrelatedFile] = format.SHA256Hex([]byte("unrelated"))
	unit := footprint.Ref{File: sourceFile, Qualname: footprintUnit}
	f.project.runner.Coverage = coverage.Map{
		testName:      {unit},
		reachingName:  {unit},
		unrelatedName: {{File: sourceFile, Qualname: elsewhereUnit}},
	}
	return footprint.Footprint{Definitions: []footprint.Ref{unit}}
}

func TestTestFilesOf(t *testing.T) {
	t.Run("the files of the bound tests and of the tests that execute the footprint are listed, and no other"+verifiesInputs, func(t *testing.T) {
		f := newFixture()
		closed := withOtherTests(&f)
		got := f.project.testFilesOf(f.run.c, closed)
		want := []string{otherTestFile, testFile}
		if !slices.Equal(got, want) {
			t.Fatalf("test files %v, want %v", got, want)
		}
	})

	t.Run("a unit nested in a unit of the footprint counts as reached"+verifiesInputs, func(t *testing.T) {
		f := newFixture()
		closed := withOtherTests(&f)
		f.project.runner.Coverage[unrelatedName] = []footprint.Ref{{File: sourceFile, Qualname: footprintUnit + ".inner"}}
		if got := f.project.testFilesOf(f.run.c, closed); !slices.Contains(got, unrelatedFile) {
			t.Fatalf("test files %v", got)
		}
	})

	t.Run("without a footprint every file that holds a bound test of the project is listed"+verifiesInputs, func(t *testing.T) {
		f := newFixture()
		withOtherTests(&f)
		got := f.project.testFilesOf(f.run.c, footprint.Footprint{})
		want := []string{otherTestFile, testFile, unrelatedFile}
		if !slices.Equal(got, want) {
			t.Fatalf("test files %v, want %v", got, want)
		}
	})
}

func TestBaseInputsAreDescriptors(t *testing.T) {
	f := newFixture()
	closed := withOtherTests(&f)
	f.run.st.testFiles = f.project.testFilesOf(f.run.c, closed)
	inputs, err := f.run.baseInputs()
	if err != nil {
		t.Fatal(err)
	}
	byRole := map[format.InputRole][]format.Input{}
	for _, input := range inputs {
		byRole[input.Role] = append(byRole[input.Role], input)
	}

	t.Run("the spec input is named after the criterion and holds its statement digest"+verifiesInputs, func(t *testing.T) {
		want := format.SpecInput("PAR-3", 1, f.run.c.StatementDigest)
		if len(byRole[format.RoleSpec]) != 1 || byRole[format.RoleSpec][0] != want {
			t.Fatalf("spec inputs %+v, want %+v", byRole[format.RoleSpec], want)
		}
	})

	t.Run("each test file and code file is one descriptor with the digest of its exact bytes"+verifiesInputs, func(t *testing.T) {
		wantTests := []format.Input{
			format.FileInput(format.RoleTestSource, testFile, format.SHA256Hex([]byte("tests"))),
			format.FileInput(format.RoleTestSource, otherTestFile, format.SHA256Hex([]byte("other"))),
		}
		if len(byRole[format.RoleTestSource]) != len(wantTests) {
			t.Fatalf("test inputs %+v", byRole[format.RoleTestSource])
		}
		for _, want := range wantTests {
			if !slices.Contains(byRole[format.RoleTestSource], want) {
				t.Errorf("no descriptor %+v among %+v", want, byRole[format.RoleTestSource])
			}
		}
		wantCode := format.FileInput(format.RoleCodeTree, sourceFile, format.SHA256Hex([]byte("FREE = 50\n")))
		if len(byRole[format.RoleCodeTree]) != 1 || byRole[format.RoleCodeTree][0] != wantCode {
			t.Fatalf("code inputs %+v, want %+v", byRole[format.RoleCodeTree], wantCode)
		}
	})

	t.Run("a test file no listed test reaches is not a descriptor"+verifiesInputs, func(t *testing.T) {
		for _, input := range byRole[format.RoleTestSource] {
			if input.Name == unrelatedFile {
				t.Fatalf("the unrelated file is listed: %+v", input)
			}
		}
	})
}
