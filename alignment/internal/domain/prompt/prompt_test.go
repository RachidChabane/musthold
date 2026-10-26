package prompt_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/alignment/internal/domain/criterion"
	"github.com/RachidChabane/musthold/alignment/internal/domain/masking"
	"github.com/RachidChabane/musthold/alignment/internal/domain/prompt"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
)

const (
	unit       = "def test_case():\n    assert helper_1(4, 3) == 12\n"
	signatures = "def line_total(unit_price, quantity):\n    ...\n"
)

func criteria(count int) []criterion.Criterion {
	list := make([]criterion.Criterion, count)
	for index := range list {
		list[index] = criterion.Criterion{
			Ref:       criterion.Ref{ID: fmt.Sprintf("PAR-%d", index+1), Revision: 1},
			Statement: fmt.Sprintf("WHEN case %d occurs THE SYSTEM SHALL answer %d.\n\nExamples:\n  line_total(unit_price=%d, quantity=1) => %d", index+1, index+1, index+1, index+1),
		}
	}
	return list
}

func input(count int, seed string) prompt.TestInput {
	return prompt.TestInput{Criteria: masking.Tokenize(criteria(count), seed), Unit: unit, Signatures: signatures}
}

type schema struct {
	Required   []string `json:"required"`
	Properties map[string]struct {
		Enum []string `json:"enum"`
	} `json:"properties"`
}

func TestJudgeRequest(t *testing.T) {
	built, err := prompt.BuildJudge(input(4, "seed"))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("the prompt shows the criteria under tokens, the test and the signatures, and no criterion identifier [verifies SP-318~1]", func(t *testing.T) {
		user := built.Prompt.User
		for _, want := range []string{"K-", "WHEN case 1 occurs", "def test_case", "line_total(unit_price, quantity)", "Examples:"} {
			if !strings.Contains(user, want) {
				t.Errorf("the prompt lacks %q", want)
			}
		}
		if leaks := masking.CheckText(built.Prompt.System+built.Prompt.User, criterion.Project{Criteria: criteria(4)}.Refs()); len(leaks) != 0 {
			t.Fatalf("leaks %v", leaks)
		}
	})

	t.Run("the response schema enumerates the tokens and NONE and ties NONE to the coverage NONE [verifies SP-318~1]", func(t *testing.T) {
		var parsed schema
		if err := json.Unmarshal(built.Schema, &parsed); err != nil {
			t.Fatal(err)
		}
		tokens := input(4, "seed").Criteria.Tokens
		if got := parsed.Properties[prompt.MemberRequirement].Enum; len(got) != len(tokens)+1 || got[len(got)-1] != masking.NoneAnswer {
			t.Fatalf("requirement enum %v", got)
		}
		if !strings.Contains(string(built.Schema), `"oneOf"`) {
			t.Fatal("the schema does not tie NONE to the coverage NONE")
		}
	})

	t.Run("a judge answer is read with its confidence as a fraction [verifies SP-318~1]", func(t *testing.T) {
		answer, err := prompt.ParseJudge([]byte(`{"requirement":"K-1a2b","coverage":"PARTIAL","confidence":85}`))
		if err != nil || answer.Token != "K-1a2b" || answer.Coverage != "PARTIAL" || answer.Confidence != 0.85 {
			t.Fatalf("answer %+v: %v", answer, err)
		}
	})
}

func TestClosedRequest(t *testing.T) {
	tokens := input(30, "seed").Criteria.Tokens

	t.Run("a closed request asks the criterion question over its group and NONE [verifies SP-319~1]", func(t *testing.T) {
		built, err := prompt.BuildClosed(prompt.ClosedInput{Test: input(30, "seed"), Group: tokens[:23]})
		if err != nil {
			t.Fatal(err)
		}
		var closed struct {
			Questions []hostproto.ClosedQuestion `json:"questions"`
		}
		if err := json.Unmarshal(built.Closed, &closed); err != nil || len(closed.Questions) != 1 {
			t.Fatalf("questions %+v: %v", closed, err)
		}
		question := closed.Questions[0]
		if question.ID != prompt.QuestionCriterion || question.Type != hostproto.ClosedChoice || len(question.Options) != 24 || question.Options[23].Name != masking.NoneAnswer {
			t.Fatalf("question %+v", question)
		}
		if question.Options[0].Description == "" {
			t.Fatal("an option carries no statement")
		}
	})

	t.Run("the final round also asks the coverage over the four levels [verifies SP-319~1]", func(t *testing.T) {
		built, err := prompt.BuildClosed(prompt.ClosedInput{Test: input(5, "seed"), Group: input(5, "seed").Criteria.Tokens, Final: true})
		if err != nil {
			t.Fatal(err)
		}
		var closed struct {
			Questions []hostproto.ClosedQuestion `json:"questions"`
		}
		if err := json.Unmarshal(built.Closed, &closed); err != nil || len(closed.Questions) != 2 {
			t.Fatalf("questions %+v: %v", closed, err)
		}
		coverage := closed.Questions[1]
		if coverage.ID != prompt.QuestionCoverage || coverage.Type != hostproto.ClosedScore || len(coverage.Options) != 4 || coverage.Options[0].Name != "NONE" || coverage.Options[3].Name != "FULL" {
			t.Fatalf("coverage %+v", coverage)
		}
		if !strings.Contains(string(built.Schema), `"minContains"`) {
			t.Fatal("the schema does not require each question to be answered once")
		}
	})

	t.Run("the state of a round shows only the requirements of its group [verifies SP-319~1]", func(t *testing.T) {
		built, err := prompt.BuildClosed(prompt.ClosedInput{Test: input(30, "seed"), Group: tokens[:2]})
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Count(built.Prompt.User, "WHEN case"); got != 2 {
			t.Fatalf("%d statements in the state", got)
		}
	})

	t.Run("a closed answer is read by question id [verifies SP-319~1]", func(t *testing.T) {
		answers, err := hostproto.ParseClosedAnswers([]byte(`{"answers":[{"id":"criterion","choice":"K-1","confidence":0.9},{"id":"coverage","choice":"FULL"}]}`))
		if err != nil || !answers["criterion"].HasConfidence || answers["criterion"].Confidence != 0.9 || answers["coverage"].HasConfidence || answers["coverage"].Choice != "FULL" {
			t.Fatalf("answers %+v: %v", answers, err)
		}
	})
}

func TestClosedRequestIsAClosedQuestionSet(t *testing.T) {
	const verifiesSP411 = " [verifies SP-411~1]"
	in := input(5, "seed")
	built, err := prompt.BuildClosed(prompt.ClosedInput{Test: in, Group: in.Criteria.Tokens, Final: true})
	if err != nil {
		t.Fatal(err)
	}
	set, err := hostproto.ParseClosedSet(built.Closed)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("the set is valid, its score levels say what each coverage means and its options say what each requirement is"+verifiesSP411, func(t *testing.T) {
		criterion, coverage := set.Questions[0], set.Questions[1]
		for _, option := range append(slices.Clone(criterion.Options), coverage.Options...) {
			if option.Description == "" {
				t.Errorf("option %+v carries no criteria text", option)
			}
		}
		seen := map[string]bool{}
		for _, level := range coverage.Options {
			if seen[level.Description] {
				t.Errorf("two levels say %q", level.Description)
			}
			seen[level.Description] = true
		}
	})

	t.Run("the response schema is the one the set builds and the digest does not depend on the group"+verifiesSP411, func(t *testing.T) {
		want, err := set.ResponseSchema()
		if err != nil || string(built.Schema) != string(want) {
			t.Fatalf("schema %s, want %s (%v)", built.Schema, want, err)
		}
		other := input(9, "another seed")
		narrower, err := prompt.BuildClosed(prompt.ClosedInput{Test: other, Group: other.Criteria.Tokens[:3], Final: true})
		if err != nil {
			t.Fatal(err)
		}
		if narrower.Prompt.TemplateDigest != built.Prompt.TemplateDigest {
			t.Error("the template digest depends on the tokens of the test")
		}
	})

	t.Run("a System One response to the set is read back through the shared translation"+verifiesSP411, func(t *testing.T) {
		response := `{"answers":{"criterion":{"type":"choice","choice":"` + in.Criteria.Tokens[1] + `","confidence":0.8},"coverage":{"type":"score","score":3,"confidence":0.7}}}`
		document, err := set.AnswersFromSystemOne([]byte(response))
		if err != nil {
			t.Fatal(err)
		}
		answers, err := hostproto.ParseClosedAnswers(document)
		if err != nil || answers[prompt.QuestionCriterion].Choice != in.Criteria.Tokens[1] || answers[prompt.QuestionCoverage].Choice != "FULL" {
			t.Fatalf("answers %+v (%v)", answers, err)
		}
	})
}

func TestRequestIdentity(t *testing.T) {
	requestID := func(t *testing.T, built prompt.Built) hostproto.RequestID {
		t.Helper()
		request := hostproto.Request{
			Type: hostproto.RequestType, Module: format.Module{Name: "musthold-alignment", Version: "1"}, Kind: hostproto.KindJudgeAlignment,
			Role: hostproto.RoleFor("musthold-alignment", hostproto.AgentJudge), Shape: hostproto.ShapeOpen,
			Prompt: built.Prompt, ResponseSchema: built.Schema, Limits: hostproto.Limits{MaxOutputBytes: hostproto.MaxOutputBytes},
			Inputs: nil, CacheKey: strings.Repeat("a", 64),
		}
		id, err := request.ID()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}

	t.Run("the same work builds the same request and any change of the test or a statement builds another [verifies SP-320~1]", func(t *testing.T) {
		first, _ := prompt.BuildJudge(input(6, "seed"))
		second, _ := prompt.BuildJudge(input(6, "seed"))
		if requestID(t, first) != requestID(t, second) {
			t.Fatal("identical work gave different request ids")
		}
		changedTest := input(6, "seed")
		changedTest.Unit += "    assert helper_1(1, 1) == 1\n"
		changedStatements := criteria(6)
		changedStatements[2].Statement += " Also fast."
		for name, in := range map[string]prompt.TestInput{
			"test":       changedTest,
			"statement":  {Criteria: masking.Tokenize(changedStatements, "seed"), Unit: unit, Signatures: signatures},
			"signatures": {Criteria: input(6, "seed").Criteria, Unit: unit, Signatures: signatures + "def other(): ..."},
			"seed":       input(6, "another"),
		} {
			built, _ := prompt.BuildJudge(in)
			if requestID(t, built) == requestID(t, first) {
				t.Errorf("a change of the %s kept the request id", name)
			}
		}
	})

	t.Run("the template digest does not depend on the tokens of one test [verifies SP-320~1]", func(t *testing.T) {
		a, _ := prompt.BuildJudge(input(6, "one"))
		b, _ := prompt.BuildJudge(input(9, "two"))
		if a.Prompt.TemplateDigest != b.Prompt.TemplateDigest || a.Prompt.TemplateDigest != prompt.TemplateDigests()[hostproto.KindJudgeAlignment] {
			t.Fatal("the template digest varies with the test")
		}
		if prompt.TemplateDigests()[hostproto.KindDecideClosed] == prompt.TemplateDigests()[hostproto.KindJudgeAlignment] {
			t.Fatal("two kinds share a template digest")
		}
	})
}

func TestBlindness(t *testing.T) {
	t.Run("no payload of any request names a declared criterion [verifies SP-318~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			count := rapid.IntRange(1, 40).Draw(t, "count")
			seed := rapid.String().Draw(t, "seed")
			in := input(count, seed)
			refs := criterion.Project{Criteria: criteria(count)}.Refs()
			judge, err := prompt.BuildJudge(in)
			if err != nil {
				t.Fatal(err)
			}
			closed, err := prompt.BuildClosed(prompt.ClosedInput{Test: in, Group: in.Criteria.Tokens[:min(count, 23)], Final: true})
			if err != nil {
				t.Fatal(err)
			}
			for _, text := range []string{judge.Prompt.System, judge.Prompt.User, string(judge.Schema), closed.Prompt.User, string(closed.Closed), string(closed.Schema)} {
				if leaks := masking.CheckText(text, refs); len(leaks) != 0 {
					t.Fatalf("leaks %v in %q", leaks, text)
				}
			}
		})
	})
}
