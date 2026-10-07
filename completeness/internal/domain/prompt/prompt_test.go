package prompt_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/completeness/internal/domain/blind"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/closedq"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/criterion"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/dimension"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/prompt"
)

func criteria(count int) []criterion.Criterion {
	list := make([]criterion.Criterion, count)
	for index := range list {
		id := fmt.Sprintf("INV-%d", index+1)
		list[index] = criterion.Criterion{
			Ref:       criterion.Ref{ID: id, Revision: 1},
			Statement: fmt.Sprintf("WHEN line %d of %s~1 is totalled THE SYSTEM SHALL add it.", index, id),
		}
	}
	return list
}

func refsOf(list []criterion.Criterion) []criterion.Ref {
	refs := make([]criterion.Ref, len(list))
	for index, c := range list {
		refs[index] = c.Ref
	}
	return refs
}

func questionsOf(t testing.TB, built prompt.Built) closedq.Set {
	t.Helper()
	var set closedq.Set
	if err := json.Unmarshal(built.Closed, &set); err != nil {
		t.Fatal(err)
	}
	return set
}

func TestBuildDimensions(t *testing.T) {
	list := criteria(3)
	tokenized := blind.Tokenize(list, "spec", refsOf(list))

	t.Run("one choice question per dimension asked, each COVERED, NOT_APPLICABLE or MISSING [verifies SP-502~1]", func(t *testing.T) {
		asked := dimension.Remaining([]string{"permissions"})
		built, err := prompt.BuildDimensions(prompt.DimensionsInput{Criteria: tokenized, Dimensions: asked})
		if err != nil {
			t.Fatal(err)
		}
		set := questionsOf(t, built)
		if len(set.Questions) != len(asked) {
			t.Fatalf("%d questions for %d dimensions", len(set.Questions), len(asked))
		}
		for index, question := range set.Questions {
			if question.ID != string(asked[index]) || question.Type != closedq.TypeChoice {
				t.Fatalf("question %+v", question)
			}
			if strings.Join(question.OptionNames(), ",") != "COVERED,NOT_APPLICABLE,MISSING" {
				t.Fatalf("options %v", question.OptionNames())
			}
		}
		if slices.ContainsFunc(set.Questions, func(q closedq.Question) bool { return q.ID == "permissions" }) {
			t.Fatal("a dimension that was not asked is in the request")
		}
	})

	t.Run("the request shows the requirements under tokens and the schema binds one answer per dimension [verifies SP-502~1]", func(t *testing.T) {
		built, err := prompt.BuildDimensions(prompt.DimensionsInput{Criteria: tokenized, Dimensions: dimension.Checklist()})
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range tokenized.Tokens {
			if !strings.Contains(built.Prompt.User, token+": ") {
				t.Fatalf("the request does not show %s", token)
			}
		}
		if !strings.Contains(string(built.Schema), `"minItems":8`) {
			t.Fatalf("schema %s", built.Schema)
		}
	})
}

func TestBuildHunk(t *testing.T) {
	list := criteria(5)
	tokenized := blind.Tokenize(list, "change", refsOf(list))

	t.Run("the question offers the tokens of the group and NONE [verifies SP-510~1]", func(t *testing.T) {
		group := tokenized.Tokens[:3]
		built, err := prompt.BuildHunk(prompt.HunkInput{Criteria: tokenized, Group: group, Change: "changed total in src/totals.py"})
		if err != nil {
			t.Fatal(err)
		}
		set := questionsOf(t, built)
		if len(set.Questions) != 1 || set.Questions[0].ID != prompt.QuestionCriterion {
			t.Fatalf("questions %+v", set.Questions)
		}
		want := append(slices.Clone(group), blind.NoneAnswer)
		if !slices.Equal(set.Questions[0].OptionNames(), want) {
			t.Fatalf("options %v, want %v", set.Questions[0].OptionNames(), want)
		}
		if !strings.Contains(built.Prompt.User, "changed total in src/totals.py") {
			t.Fatal("the request does not show the change")
		}
		if strings.Contains(built.Prompt.User, tokenized.Tokens[4]+": ") {
			t.Fatal("the request shows a requirement outside the group")
		}
	})
}

func TestBuildTestability(t *testing.T) {
	list := criteria(5)
	tokenized := blind.Tokenize(list, "spec", refsOf(list))

	t.Run("one question per requirement, answered TESTABLE, AMBIGUOUS or UNTESTABLE [verifies SP-512~1]", func(t *testing.T) {
		built, err := prompt.BuildTestability(prompt.TestabilityInput{Criteria: tokenized, Group: tokenized.Tokens})
		if err != nil {
			t.Fatal(err)
		}
		set := questionsOf(t, built)
		if len(set.Questions) != 5 {
			t.Fatalf("%d questions", len(set.Questions))
		}
		for index, question := range set.Questions {
			if question.ID != tokenized.Tokens[index] || strings.Join(question.OptionNames(), ",") != "TESTABLE,AMBIGUOUS,UNTESTABLE" {
				t.Fatalf("question %+v", question)
			}
		}
	})

	t.Run("requirements are asked in groups of at most 64 questions [verifies SP-512~1]", func(t *testing.T) {
		tokens := make([]string, 150)
		for index := range tokens {
			tokens[index] = fmt.Sprintf("K-%04d", index)
		}
		groups := prompt.TestabilityGroups(tokens)
		if len(groups) != 3 || len(groups[0]) != 64 || len(groups[2]) != 22 {
			t.Fatalf("group sizes %d %d %d", len(groups), len(groups[0]), len(groups[len(groups)-1]))
		}
		if flat := slices.Concat(groups...); !slices.Equal(flat, tokens) {
			t.Fatal("the groups are not a partition in order")
		}
		if len(prompt.TestabilityGroups(nil)) != 0 {
			t.Fatal("no token still gave a group")
		}
	})
}

func TestIdentity(t *testing.T) {
	list := criteria(4)
	tokenized := blind.Tokenize(list, "spec", refsOf(list))

	t.Run("the same work gives the same request and another statement gives another [verifies SP-517~1]", func(t *testing.T) {
		build := func(in blind.Tokenized) prompt.Built {
			built, err := prompt.BuildTestability(prompt.TestabilityInput{Criteria: in, Group: in.Tokens})
			if err != nil {
				t.Fatal(err)
			}
			return built
		}
		first, second := build(tokenized), build(tokenized)
		if first.Prompt != second.Prompt || string(first.Schema) != string(second.Schema) || string(first.Closed) != string(second.Closed) {
			t.Fatal("the same work gave another request")
		}
		changed := criteria(4)
		changed[0].Statement += " Always."
		other := build(blind.Tokenize(changed, "spec", refsOf(changed)))
		if other.Prompt.User == first.Prompt.User {
			t.Fatal("a changed statement gave the same request")
		}
	})

	t.Run("a template digest does not depend on the tokens of one request and tells the signals apart [verifies SP-517~1]", func(t *testing.T) {
		small := blind.Tokenize(criteria(2), "a", nil)
		large := blind.Tokenize(criteria(9), "b", nil)
		first, _ := prompt.BuildTestability(prompt.TestabilityInput{Criteria: small, Group: small.Tokens})
		second, _ := prompt.BuildTestability(prompt.TestabilityInput{Criteria: large, Group: large.Tokens})
		if first.Prompt.TemplateDigest != second.Prompt.TemplateDigest {
			t.Fatal("the template digest depends on the tokens")
		}
		digests := prompt.TemplateDigests()
		if len(digests) != len(prompt.Signals()) {
			t.Fatalf("digests %v", digests)
		}
		seen := map[string]bool{}
		for _, signal := range prompt.Signals() {
			if digests[signal] == "" || seen[digests[signal]] {
				t.Fatalf("signal %s digest %q", signal, digests[signal])
			}
			seen[digests[signal]] = true
		}
		if digests[prompt.SignalTestability] != first.Prompt.TemplateDigest {
			t.Fatal("the registry digest differs from the digest of a built request")
		}
	})
}

func TestBlindPayloads(t *testing.T) {
	t.Run("no request names a requirement of the project, whatever the requirements and the change [verifies SP-513~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			count := rapid.IntRange(1, 30).Draw(t, "count")
			all := criteria(count)
			refs := refsOf(all)
			seed := rapid.String().Draw(t, "seed")
			tokenized := blind.Tokenize(all, seed, refs)
			change := blind.Scrub(fmt.Sprintf("changed total for INV-%d~1 in src/INV-%d.py", rapid.IntRange(1, count).Draw(t, "cited"), 1), refs)
			dimensions, err := prompt.BuildDimensions(prompt.DimensionsInput{Criteria: tokenized, Dimensions: dimension.Checklist()})
			if err != nil {
				t.Fatal(err)
			}
			hunk, err := prompt.BuildHunk(prompt.HunkInput{Criteria: tokenized, Group: tokenized.Tokens[:min(count, 23)], Change: change})
			if err != nil {
				t.Fatal(err)
			}
			testability, err := prompt.BuildTestability(prompt.TestabilityInput{Criteria: tokenized, Group: tokenized.Tokens[:min(count, 64)]})
			if err != nil {
				t.Fatal(err)
			}
			for name, built := range map[string]prompt.Built{"dimensions": dimensions, "hunk": hunk, "testability": testability} {
				for _, payload := range []string{built.Prompt.System, built.Prompt.User, string(built.Schema), string(built.Closed)} {
					if names := blind.Names(payload, refs); len(names) != 0 {
						t.Fatalf("the %s request names %v", name, names)
					}
				}
			}
		})
	})
}
