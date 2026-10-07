package prove

import (
	"encoding/json"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/execution"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/hostbridge"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
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
	run := &critRun{p: p, c: criterion, st: &staticAnalysis{criterion: criterion, codeFiles: []string{sourceFile}}, rules: p.settings.Policy, poolByID: map[string]*poolMutant{}}
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
