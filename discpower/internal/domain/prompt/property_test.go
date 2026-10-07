package prompt_test

import (
	"strconv"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/prompt"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

const (
	maxRefs         = 5
	maxFragments    = 8
	maxSliceEntries = 3
	testMarker      = "TESTSOURCE"
)

func genRef() *rapid.Generator[spec.Ref] {
	return rapid.Custom(func(t *rapid.T) spec.Ref {
		return spec.Ref{ID: "SP-" + strconv.Itoa(rapid.IntRange(1, 999).Draw(t, "number")), Revision: rapid.IntRange(1, 9).Draw(t, "revision")}
	})
}

func genStatementAround(refs []spec.Ref) *rapid.Generator[string] {
	fragment := rapid.Custom(func(t *rapid.T) string {
		if len(refs) > 0 && rapid.Bool().Draw(t, "mention") {
			ref := rapid.SampledFrom(refs).Draw(t, "ref")
			if rapid.Bool().Draw(t, "full") {
				return ref.String()
			}
			return ref.ID
		}
		return rapid.StringMatching(`[a-zA-Z0-9 .,~()-]{0,20}`).Draw(t, "words")
	})
	return rapid.Custom(func(t *rapid.T) string {
		return strings.Join(rapid.SliceOfN(fragment, 0, maxFragments).Draw(t, "fragments"), " ")
	})
}

func TestPromptProperties(t *testing.T) {
	t.Run("Scrub leaves none of the references in a statement"+verifiesSP173, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			refs := rapid.SliceOfN(genRef(), 1, maxRefs).Draw(t, "refs")
			statement := genStatementAround(refs).Draw(t, "statement")
			scrubbed := prompt.Scrub(statement, refs)
			for _, ref := range refs {
				if strings.Contains(scrubbed, ref.String()) || strings.Contains(scrubbed, ref.ID) {
					t.Fatalf("%q still names %s after scrubbing %q", scrubbed, ref, statement)
				}
			}
		})
	})

	t.Run("a scrubbed statement leaves every request without a reference"+verifiesSP173, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			refs := rapid.SliceOfN(genRef(), 1, maxRefs).Draw(t, "refs")
			statement := prompt.Scrub(genStatementAround(refs).Draw(t, "statement"), refs)
			edits := []prompt.EditView{sampleEdit()}
			generation := prompt.BuildGeneration(prompt.GenerationInput{Statement: statement})
			validity := prompt.BuildValidity(prompt.JudgeInput{Statement: statement, Edits: edits})
			equivalence := prompt.BuildEquivalence(prompt.JudgeInput{Statement: statement, Edits: edits})
			screen, err := prompt.BuildScreen(prompt.ScreenInput{Statements: []string{statement}, Edits: edits})
			if err != nil {
				t.Fatal(err)
			}
			for _, built := range []prompt.Built{generation, validity, equivalence, screen} {
				text := built.Prompt.System + built.Prompt.User + string(built.Closed)
				for _, ref := range refs {
					if strings.Contains(text, ref.String()) || strings.Contains(text, ref.ID) {
						t.Fatalf("a request names %s", ref)
					}
				}
			}
		})
	})

	t.Run("a generation prompt never contains test text passed elsewhere"+verifiesSP173, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			testSource := testMarker + rapid.StringMatching(`[a-f0-9]{16}`).Draw(t, "testSource")
			statement := rapid.StringMatching(`[a-z .]{0,40}`).Draw(t, "statement")
			count := rapid.IntRange(0, maxSliceEntries).Draw(t, "entries")
			entries := make([]prompt.SliceEntry, count)
			for index := range entries {
				entries[index] = prompt.SliceEntry{
					File: rapid.StringMatching(`[a-z/]{1,10}\.py`).Draw(t, "file"), Qualname: rapid.StringMatching(`[A-Za-z_.]{1,12}`).Draw(t, "qualname"),
					Source: rapid.StringMatching(`def [a-z]{1,6}\(\): return [0-9]`).Draw(t, "source"),
				}
			}
			avoid := rapid.SliceOfN(rapid.StringMatching(`[a-z ]{1,20}`), 0, maxSliceEntries).Draw(t, "avoid")
			generation := prompt.BuildGeneration(prompt.GenerationInput{Statement: statement, Slice: entries, Avoid: avoid})
			judge := prompt.JudgeInput{Statement: statement, Edits: []prompt.EditView{{File: "a.py", Qualname: "f", Before: "x", After: "y"}}}
			validity := prompt.BuildValidity(judge)
			if strings.Contains(generation.Prompt.System+generation.Prompt.User+string(generation.Schema), testSource) {
				t.Fatalf("the generation prompt contains %s", testSource)
			}
			if strings.Contains(validity.Prompt.System+validity.Prompt.User, testSource) {
				t.Fatalf("the judge prompt contains %s", testSource)
			}
		})
	})

	t.Run("the template digest of a kind ignores the content and differs between kinds"+verifiesSP173, func(t *testing.T) {
		digests := prompt.TemplateDigests()
		rapid.Check(t, func(t *rapid.T) {
			statement := rapid.String().Draw(t, "statement")
			edits := []prompt.EditView{{File: rapid.String().Draw(t, "file"), Qualname: "f", Before: rapid.String().Draw(t, "before"), After: rapid.String().Draw(t, "after")}}
			count := rapid.IntRange(1, closedLimit).Draw(t, "questions")
			screen, err := prompt.BuildScreen(prompt.ScreenInput{Statements: repeatedStatement(statement, count), Edits: edits})
			if err != nil {
				t.Fatal(err)
			}
			got := map[hostproto.Kind]string{
				hostproto.KindGenerateTargeted: prompt.BuildGeneration(prompt.GenerationInput{Statement: statement, Avoid: []string{statement}}).Prompt.TemplateDigest,
				hostproto.KindJudgeValidity:    prompt.BuildValidity(prompt.JudgeInput{Statement: statement, Edits: edits}).Prompt.TemplateDigest,
				hostproto.KindJudgeEquivalent:  prompt.BuildEquivalence(prompt.JudgeInput{Statement: statement, Edits: edits}).Prompt.TemplateDigest,
				hostproto.KindDecideClosed:     screen.Prompt.TemplateDigest,
			}
			seen := map[string]hostproto.Kind{}
			for kind, digest := range got {
				if digest != digests[kind] {
					t.Fatalf("the digest of %s depends on the content", kind)
				}
				if other, clash := seen[digest]; clash {
					t.Fatalf("%s and %s share a digest", kind, other)
				}
				seen[digest] = kind
			}
		})
	})

	t.Run("a closed request carries one question per statement up to the limit and refuses more"+verifiesSP173, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			count := rapid.IntRange(0, closedLimit*2).Draw(t, "count")
			_, err := prompt.BuildScreen(prompt.ScreenInput{Statements: statements(count)})
			if accepted := count >= 1 && count <= closedLimit; (err == nil) != accepted {
				t.Fatalf("%d statements: error %v", count, err)
			}
		})
	})
}

func repeatedStatement(statement string, count int) []string {
	out := make([]string, count)
	for index := range out {
		out[index] = statement
	}
	return out
}
