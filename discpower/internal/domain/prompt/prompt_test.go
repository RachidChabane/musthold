package prompt_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/differential"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/prompt"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

const (
	verifiesSP173 = " [verifies SP-173~1]"
	verifiesSP174 = " [verifies SP-174~1]"

	digestHexLength = 64
	closedLimit     = 64
)

var identifierPattern = regexp.MustCompile(`[A-Z]{2,}-\d+`)

var (
	refFirst  = spec.Ref{ID: "SP-1", Revision: 1}
	refSecond = spec.Ref{ID: "SP-12", Revision: 3}
)

func statements(count int) []string {
	out := make([]string, count)
	for index := range out {
		out[index] = fmt.Sprintf("Requirement text number %d.", index)
	}
	return out
}

func sampleEdit() prompt.EditView {
	return prompt.EditView{File: "src/pricing.py", Qualname: "price", Before: "def price(x):\n    return x\n\n", After: "def price(x):\n    return x + 1\n"}
}

func sampleWitness() prompt.WitnessView {
	return prompt.WitnessView{
		Call:      spec.Call{Operation: "price", Args: map[string]spec.Literal{"b": {Type: spec.TypeString, Value: "x"}, "a": {Type: spec.TypeInteger, Value: "3"}}},
		Reference: spec.Observation{Kind: spec.Returns, Type: spec.TypeInteger, Value: "3"},
		Mutant:    spec.Observation{Kind: spec.Raises, Value: "ValueError"},
	}
}

func TestScrub(t *testing.T) {
	cases := []struct {
		name string
		text string
		refs []spec.Ref
		want string
	}{
		{"a full reference", "See SP-1~1 for details", []spec.Ref{refFirst}, "See the requirement for details"},
		{"a bare identifier", "As SP-1 states", []spec.Ref{refFirst}, "As the requirement states"},
		{"every occurrence", "SP-1~1 and SP-1~1 and SP-1", []spec.Ref{refFirst}, "the requirement and the requirement and the requirement"},
		{"a longer identifier that contains a shorter one", "SP-12~3 then SP-1~1", []spec.Ref{refFirst, refSecond}, "the requirement then the requirement"},
		{"the references in the opposite order", "SP-12~3 then SP-1~1", []spec.Ref{refSecond, refFirst}, "the requirement then the requirement"},
		{"a reference that the text does not state", "Plain text", []spec.Ref{refFirst}, "Plain text"},
		{"no reference", "SP-1~1 stays", nil, "SP-1~1 stays"},
		{"an identifier of another revision", "SP-1~9", []spec.Ref{refFirst}, "the requirement~9"},
		{"an empty text", "", []spec.Ref{refFirst}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name+verifiesSP173, func(t *testing.T) {
			if got := prompt.Scrub(tc.text, tc.refs); got != tc.want {
				t.Errorf("Scrub() = %q, want %q", got, tc.want)
			}
		})
	}
	t.Run("the references given are not reordered"+verifiesSP173, func(t *testing.T) {
		refs := []spec.Ref{refFirst, refSecond}
		prompt.Scrub("SP-12~3", refs)
		if refs[0] != refFirst || refs[1] != refSecond {
			t.Errorf("refs were reordered: %v", refs)
		}
	})
}

func TestNoPromptNamesACriterion(t *testing.T) {
	scrub := func(text string) string { return prompt.Scrub(text, []spec.Ref{refFirst, refSecond}) }
	statement := scrub("SP-1~1: WHEN a price is asked THE SYSTEM SHALL answer (see SP-12~3)")
	generation := prompt.BuildGeneration(prompt.GenerationInput{Statement: statement, Slice: []prompt.SliceEntry{{File: "a.py", Qualname: "f", Source: "def f(): pass"}}, Avoid: []string{"off by one"}})
	validity := prompt.BuildValidity(prompt.JudgeInput{Statement: statement, Edits: []prompt.EditView{sampleEdit()}, Witnesses: []prompt.WitnessView{sampleWitness()}})
	equivalence := prompt.BuildEquivalence(prompt.JudgeInput{Statement: statement, Edits: []prompt.EditView{sampleEdit()}})
	screen, err := prompt.BuildScreen(prompt.ScreenInput{Statements: []string{statement, statement}, Edits: []prompt.EditView{sampleEdit()}})
	if err != nil {
		t.Fatal(err)
	}
	built := map[string]prompt.Built{"generation": generation, "validity": validity, "equivalence": equivalence, "screen": screen}
	for name, one := range built {
		t.Run("the "+name+" request names no criterion"+verifiesSP173, func(t *testing.T) {
			text := one.Prompt.System + "\n" + one.Prompt.User + "\n" + string(one.Schema) + "\n" + string(one.Closed)
			if found := identifierPattern.FindString(text); found != "" {
				t.Errorf("the request contains %q", found)
			}
		})
	}
	t.Run("the screen questions carry opaque ids"+verifiesSP173, func(t *testing.T) {
		var closed struct {
			Questions []hostproto.ClosedQuestion `json:"questions"`
		}
		if err := json.Unmarshal(screen.Closed, &closed); err != nil {
			t.Fatal(err)
		}
		for index, question := range closed.Questions {
			if question.ID != prompt.QuestionID(index) || identifierPattern.MatchString(question.ID+question.Instructions) {
				t.Errorf("question %+v", question)
			}
		}
	})
}

func TestGenerationShowsNoTest(t *testing.T) {
	t.Run("a generation input has no field for tests"+verifiesSP173, func(t *testing.T) {
		for _, kind := range []reflect.Type{reflect.TypeOf(prompt.GenerationInput{}), reflect.TypeOf(prompt.SliceEntry{})} {
			for index := range kind.NumField() {
				if strings.Contains(strings.ToLower(kind.Field(index).Name), "test") {
					t.Errorf("%s has the field %s", kind.Name(), kind.Field(index).Name)
				}
			}
		}
	})
	t.Run("the generation prompt holds the statement and the slice and nothing else"+verifiesSP173, func(t *testing.T) {
		built := prompt.BuildGeneration(prompt.GenerationInput{
			Statement: "A price is never negative.",
			Slice: []prompt.SliceEntry{
				{File: "src/pricing.py", Qualname: "price", Source: "def price(x):\n    return x\n\n"},
				{File: "src/pricing.py", Qualname: "Cart.total", Source: "def total(self): ..."},
			},
		})
		for _, want := range []string{"Requirement:\nA price is never negative.", "--- src/pricing.py :: price\ndef price(x):\n    return x\n--- src/pricing.py :: Cart.total\ndef total(self): ..."} {
			if !strings.Contains(built.Prompt.User, want) {
				t.Errorf("the user prompt lacks %q:\n%s", want, built.Prompt.User)
			}
		}
		if strings.Contains(built.Prompt.User, "Mechanisms already tried") {
			t.Error("a first round lists mechanisms")
		}
		if built.Closed != nil {
			t.Error("a generation request is not closed")
		}
	})
	t.Run("the mechanisms of earlier rounds are listed"+verifiesSP173, func(t *testing.T) {
		built := prompt.BuildGeneration(prompt.GenerationInput{Statement: "S", Avoid: []string{"flip a comparison", "drop a branch"}})
		for _, want := range []string{"Mechanisms already tried", "- flip a comparison\n", "- drop a branch\n"} {
			if !strings.Contains(built.Prompt.User, want) {
				t.Errorf("the user prompt lacks %q", want)
			}
		}
	})
	t.Run("the generation prompt asks for three slots"+verifiesSP173, func(t *testing.T) {
		built := prompt.BuildGeneration(prompt.GenerationInput{})
		for _, slot := range power.Slots() {
			if !strings.Contains(built.Prompt.User, string(slot)) {
				t.Errorf("the slot %s is not described", slot)
			}
		}
	})
}

func TestScreenIsAClosedQuestionSet(t *testing.T) {
	const verifiesSP408 = " [verifies SP-408~1]"
	built, err := prompt.BuildScreen(prompt.ScreenInput{Statements: statements(3), Edits: []prompt.EditView{sampleEdit()}})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("the question set is valid and every question is a choice with the criteria text of its options"+verifiesSP408, func(t *testing.T) {
		set, err := hostproto.ParseClosedSet(built.Closed)
		if err != nil {
			t.Fatal(err)
		}
		for _, question := range set.Questions {
			if question.Type != hostproto.ClosedChoice || len(question.Options) != 2 || question.Options[0].Description == "" || question.Options[1].Description == "" {
				t.Errorf("question %+v", question)
			}
		}
	})
	t.Run("the response schema is the one the question set builds, whatever its size"+verifiesSP408, func(t *testing.T) {
		set, _ := hostproto.ParseClosedSet(built.Closed)
		want, err := set.ResponseSchema()
		if err != nil {
			t.Fatal(err)
		}
		if string(built.Schema) != string(want) {
			t.Errorf("schema %s, want %s", built.Schema, want)
		}
		one, _ := prompt.BuildScreen(prompt.ScreenInput{Statements: statements(1)})
		if one.Prompt.TemplateDigest != built.Prompt.TemplateDigest {
			t.Error("the template digest depends on the number of questions")
		}
	})
	t.Run("answers are read through the shared reader"+verifiesSP408, func(t *testing.T) {
		answers, err := prompt.ParseScreen([]byte(`{"answers":[{"id":"q1","choice":"VIOLATES","confidence":0.7}]}`))
		if err != nil || answers["q1"] != (power.ClosedAnswer{Choice: power.ChoiceViolates, Confidence: 0.7, HasConfidence: true}) {
			t.Fatalf("answers %+v (%v)", answers, err)
		}
	})
}

func TestBuildScreenLimits(t *testing.T) {
	for _, count := range []int{0, closedLimit + 1, closedLimit * 2} {
		t.Run(fmt.Sprintf("a request with %d statements is refused", count)+verifiesSP173, func(t *testing.T) {
			if _, err := prompt.BuildScreen(prompt.ScreenInput{Statements: statements(count)}); err == nil {
				t.Error("the request was accepted")
			}
		})
	}
	for _, count := range []int{1, 2, closedLimit} {
		t.Run(fmt.Sprintf("a request with %d statements is built", count)+verifiesSP173, func(t *testing.T) {
			built, err := prompt.BuildScreen(prompt.ScreenInput{Statements: statements(count), Edits: []prompt.EditView{sampleEdit()}, Witnesses: []prompt.WitnessView{sampleWitness()}})
			if err != nil {
				t.Fatal(err)
			}
			var closed struct {
				State     string                     `json:"state"`
				Questions []hostproto.ClosedQuestion `json:"questions"`
			}
			if err := json.Unmarshal(built.Closed, &closed); err != nil {
				t.Fatal(err)
			}
			if len(closed.Questions) != count {
				t.Fatalf("%d questions", len(closed.Questions))
			}
			last := closed.Questions[count-1]
			if last.ID != fmt.Sprintf("q%d", count) || last.Type != "choice" || len(last.Options) != 2 || last.Options[0].Name != "VIOLATES" || last.Options[1].Name != "CONSISTENT" {
				t.Errorf("question %+v", last)
			}
			if !strings.HasPrefix(built.Prompt.User, closed.State) || !strings.Contains(closed.State, fmt.Sprintf("Requirement %d:\n%s", count, statements(count)[count-1])) {
				t.Errorf("the state does not hold the requirements")
			}
			if !strings.Contains(closed.State, "before:\ndef price(x):\n    return x\nafter:") || !strings.Contains(closed.State, "Observed differences") {
				t.Errorf("the state does not show the change:\n%s", closed.State)
			}
		})
	}
	t.Run("the limit is published"+verifiesSP173, func(t *testing.T) {
		if prompt.MaxQuestions != closedLimit {
			t.Errorf("MaxQuestions = %d", prompt.MaxQuestions)
		}
	})
	t.Run("question ids are one-based and opaque"+verifiesSP173, func(t *testing.T) {
		if prompt.QuestionID(0) != "q1" || prompt.QuestionID(closedLimit-1) != "q64" {
			t.Errorf("ids %q %q", prompt.QuestionID(0), prompt.QuestionID(closedLimit-1))
		}
	})
}

func TestJudgeBuilders(t *testing.T) {
	input := prompt.JudgeInput{
		Statement:  "A price is never negative.",
		Operations: []prompt.OperationView{{Name: "price", Params: []string{"amount", "tier"}}, {Name: "reset"}},
		Edits:      []prompt.EditView{sampleEdit()},
		Witnesses:  []prompt.WitnessView{sampleWitness()},
	}
	t.Run("the validity prompt shows the operations, the change and the observed differences"+verifiesSP173, func(t *testing.T) {
		user := prompt.BuildValidity(input).Prompt.User
		for _, want := range []string{
			"Requirement:\nA price is never negative.",
			"- price(amount, tier)\n", "- reset()\n",
			"--- src/pricing.py :: price\nbefore:\ndef price(x):\n    return x\nafter:\ndef price(x):\n    return x + 1\n",
			`- price(a=3, b="x"): original 3, mutant raises ValueError`,
		} {
			if !strings.Contains(user, want) {
				t.Errorf("the user prompt lacks %q:\n%s", want, user)
			}
		}
	})
	t.Run("the equivalence prompt shows the change but no operation and no witness"+verifiesSP173, func(t *testing.T) {
		user := prompt.BuildEquivalence(input).Prompt.User
		if !strings.Contains(user, "before:") || strings.Contains(user, "Operations you may use") || strings.Contains(user, "Observed differences") {
			t.Errorf("unexpected user prompt:\n%s", user)
		}
	})
	t.Run("a validity prompt without operation or witness omits both sections"+verifiesSP173, func(t *testing.T) {
		user := prompt.BuildValidity(prompt.JudgeInput{Statement: "S", Edits: []prompt.EditView{sampleEdit()}}).Prompt.User
		if strings.Contains(user, "Operations you may use") || strings.Contains(user, "Observed differences") {
			t.Errorf("unexpected user prompt:\n%s", user)
		}
	})
	t.Run("the judge requests share a system prompt and differ in instructions and schema"+verifiesSP173, func(t *testing.T) {
		validity, equivalence := prompt.BuildValidity(input), prompt.BuildEquivalence(input)
		if validity.Prompt.System != equivalence.Prompt.System || validity.Prompt.System == "" {
			t.Error("the system prompts differ or are empty")
		}
		if string(validity.Schema) == string(equivalence.Schema) || validity.Prompt.TemplateDigest == equivalence.Prompt.TemplateDigest {
			t.Error("the schemas or digests coincide")
		}
	})
}

func TestEmbeddedSchemas(t *testing.T) {
	screen, err := prompt.BuildScreen(prompt.ScreenInput{Statements: statements(1)})
	if err != nil {
		t.Fatal(err)
	}
	builts := map[string]prompt.Built{
		"generate": prompt.BuildGeneration(prompt.GenerationInput{}),
		"validity": prompt.BuildValidity(prompt.JudgeInput{}),
		"equiv":    prompt.BuildEquivalence(prompt.JudgeInput{}),
		"closed":   screen,
	}
	for name, built := range builts {
		t.Run("the "+name+" schema is a JSON object schema"+verifiesSP173, func(t *testing.T) {
			var schema struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(built.Schema, &schema); err != nil || schema.Type != "object" {
				t.Errorf("schema %s: %v", built.Schema, err)
			}
			if built.Prompt.System == "" || built.Prompt.User == "" {
				t.Error("an empty prompt")
			}
		})
	}
	t.Run("the closed request carries a JSON question set"+verifiesSP173, func(t *testing.T) {
		if !json.Valid(screen.Closed) {
			t.Error("the closed payload is not JSON")
		}
	})
}

func TestTemplateDigests(t *testing.T) {
	kinds := []hostproto.Kind{hostproto.KindGenerateTargeted, hostproto.KindJudgeValidity, hostproto.KindJudgeEquivalent, hostproto.KindDecideClosed}
	t.Run("every kind of request has a digest"+verifiesSP173, func(t *testing.T) {
		digests := prompt.TemplateDigests()
		if len(digests) != len(kinds) {
			t.Errorf("%d digests", len(digests))
		}
		for _, kind := range kinds {
			if len(digests[kind]) != digestHexLength {
				t.Errorf("the digest of %s is %q", kind, digests[kind])
			}
		}
	})
	t.Run("the digests differ between kinds"+verifiesSP173, func(t *testing.T) {
		seen := map[string]hostproto.Kind{}
		for kind, digest := range prompt.TemplateDigests() {
			if other, clash := seen[digest]; clash {
				t.Errorf("%s and %s share a digest", kind, other)
			}
			seen[digest] = kind
		}
	})
	t.Run("the digests are stable across calls"+verifiesSP173, func(t *testing.T) {
		first, second := prompt.TemplateDigests(), prompt.TemplateDigests()
		for _, kind := range kinds {
			if first[kind] != second[kind] {
				t.Errorf("the digest of %s changed", kind)
			}
		}
	})
	t.Run("a built prompt carries the digest of its kind whatever it is about"+verifiesSP173, func(t *testing.T) {
		digests := prompt.TemplateDigests()
		screen, err := prompt.BuildScreen(prompt.ScreenInput{Statements: statements(3), Edits: []prompt.EditView{sampleEdit()}})
		if err != nil {
			t.Fatal(err)
		}
		input := prompt.JudgeInput{Statement: "Anything", Edits: []prompt.EditView{sampleEdit()}, Witnesses: []prompt.WitnessView{sampleWitness()}}
		got := map[hostproto.Kind]string{
			hostproto.KindGenerateTargeted: prompt.BuildGeneration(prompt.GenerationInput{Statement: "Anything", Avoid: []string{"x"}}).Prompt.TemplateDigest,
			hostproto.KindJudgeValidity:    prompt.BuildValidity(input).Prompt.TemplateDigest,
			hostproto.KindJudgeEquivalent:  prompt.BuildEquivalence(input).Prompt.TemplateDigest,
			hostproto.KindDecideClosed:     screen.Prompt.TemplateDigest,
		}
		for _, kind := range kinds {
			if got[kind] != digests[kind] {
				t.Errorf("the digest of a %s request differs from its template digest", kind)
			}
		}
	})
}

func TestFormatting(t *testing.T) {
	t.Run("a call renders with its arguments sorted and strings quoted"+verifiesSP173, func(t *testing.T) {
		got := prompt.FormatCall(sampleWitness().Call)
		if got != `price(a=3, b="x")` {
			t.Errorf("FormatCall() = %s", got)
		}
	})
	t.Run("a call without argument renders empty parentheses"+verifiesSP173, func(t *testing.T) {
		if got := prompt.FormatCall(spec.Call{Operation: "reset"}); got != "reset()" {
			t.Errorf("FormatCall() = %s", got)
		}
	})
	observations := []struct {
		name        string
		observation spec.Observation
		want        string
	}{
		{"an integer", spec.Observation{Kind: spec.Returns, Type: spec.TypeInteger, Value: "7"}, "7"},
		{"a string", spec.Observation{Kind: spec.Returns, Type: spec.TypeString, Value: "a b"}, `"a b"`},
		{"a datetime", spec.Observation{Kind: spec.Returns, Type: spec.TypeDatetime, Value: "2026-01-01T00:00:00Z"}, `"2026-01-01T00:00:00Z"`},
		{"a boolean", spec.Observation{Kind: spec.Returns, Type: spec.TypeBoolean, Value: "true"}, "true"},
		{"a raised exception", spec.Observation{Kind: spec.Raises, Value: "KeyError"}, "raises KeyError"},
	}
	for _, tc := range observations {
		t.Run("an observation of "+tc.name+verifiesSP173, func(t *testing.T) {
			if got := prompt.FormatObservation(tc.observation); got != tc.want {
				t.Errorf("FormatObservation() = %s, want %s", got, tc.want)
			}
		})
	}
	t.Run("operations are listed sorted by name"+verifiesSP173, func(t *testing.T) {
		views := prompt.OperationViews([]differential.Signature{
			{Operation: "zeta", Params: []differential.Param{{Name: "q"}}},
			{Operation: "alpha", Params: []differential.Param{{Name: "a"}, {Name: "b"}}},
		})
		if len(views) != 2 || views[0].Name != "alpha" || strings.Join(views[0].Params, ",") != "a,b" || views[1].Name != "zeta" {
			t.Errorf("views %+v", views)
		}
	})
	t.Run("witnesses convert field by field"+verifiesSP173, func(t *testing.T) {
		witness := power.Witness{Call: sampleWitness().Call, Reference: sampleWitness().Reference, Mutant: sampleWitness().Mutant}
		views := prompt.WitnessViews([]power.Witness{witness})
		if len(views) != 1 || !reflect.DeepEqual(views[0], sampleWitness()) {
			t.Errorf("views %+v", views)
		}
	})
}
