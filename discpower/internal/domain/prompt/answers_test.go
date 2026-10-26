package prompt_test

import (
	"math"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/power"
	"github.com/RachidChabane/musthold/discpower/internal/domain/prompt"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
	"github.com/RachidChabane/musthold/format/hostproto"
)

const brokenJSON = `{"mutants": [`

func TestParseGeneration(t *testing.T) {
	t.Run("the mutants of a response are read in order"+verifiesSP174, func(t *testing.T) {
		got, err := prompt.ParseGeneration([]byte(`{"mutants":[
			{"slot":"S1","mechanism":"flip the sign","edits":[{"file":"src/a.py","qualname":"f","new_source":"def f(): return -1"}]},
			{"slot":"S3","mechanism":"skip a branch","edits":[
				{"file":"src/a.py","qualname":"g","target":"module_assignment","new_source":"LIMIT = 3"},
				{"file":"src/b.py","qualname":"C.h","target":"definition","new_source":"def h(self): pass"}]}]}`))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Fatalf("%d mutants", len(got))
		}
		if got[0].Slot != power.SlotDirect || got[0].Mechanism != "flip the sign" || len(got[0].Edits.Edits) != 1 {
			t.Errorf("first mutant %+v", got[0])
		}
		firstEdit := got[0].Edits.Edits[0]
		if firstEdit.File != "src/a.py" || firstEdit.Qualname != "f" || firstEdit.NewSource != "def f(): return -1" || firstEdit.Kind() != mutation.TargetDefinition {
			t.Errorf("first edit %+v", firstEdit)
		}
		if got[1].Slot != power.SlotPartial || len(got[1].Edits.Edits) != 2 || got[1].Edits.Edits[0].Kind() != mutation.TargetModuleAssignment || got[1].Edits.Edits[1].Qualname != "C.h" {
			t.Errorf("second mutant %+v", got[1])
		}
	})
	t.Run("a response without mutant gives none"+verifiesSP174, func(t *testing.T) {
		got, err := prompt.ParseGeneration([]byte(`{"mutants":[]}`))
		if err != nil || len(got) != 0 {
			t.Errorf("got %v, %v", got, err)
		}
	})
	t.Run("a response that is not JSON is refused"+verifiesSP174, func(t *testing.T) {
		if _, err := prompt.ParseGeneration([]byte(brokenJSON)); err == nil {
			t.Error("no error")
		}
	})
}

func TestParseValidity(t *testing.T) {
	cases := []struct {
		name       string
		output     string
		answer     power.Answer
		confidence float64
		witness    *prompt.WitnessClaim
	}{
		{"an answer with confidence", `{"validity":"VALID","confidence":0.8}`, power.AnswerValid, 0.8, nil},
		{"an answer without confidence counts as full confidence", `{"validity":"NOT_VIOLATING"}`, power.AnswerNotViolating, prompt.FullConfidence, nil},
		{"an explicit zero confidence is kept", `{"validity":"EQUIVALENT","confidence":0}`, power.AnswerEquivalent, 0, nil},
		{"a rationale is ignored", `{"validity":"UNDETERMINED","rationale":"unsure"}`, power.AnswerUndetermined, prompt.FullConfidence, nil},
		{"a witness with its call and expectation", `{"validity":"VALID","confidence":0.95,"witness":{"operation":"price","args":{"amount":{"type":"integer","value":"5"},"tier":{"type":"string","value":"gold"}},"expected":{"kind":"returns","type":"decimal","value":"4.5"}}}`,
			power.AnswerValid, 0.95, &prompt.WitnessClaim{
				Call:   spec.Call{Operation: "price", Args: map[string]spec.Literal{"amount": {Type: spec.TypeInteger, Value: "5"}, "tier": {Type: spec.TypeString, Value: "gold"}}},
				Expect: spec.Observation{Kind: spec.Returns, Type: spec.TypeDecimal, Value: "4.5"},
			}},
		{"a witness expecting an exception", `{"validity":"VALID","witness":{"operation":"price","args":{},"expected":{"kind":"raises","value":"ValueError"}}}`,
			power.AnswerValid, prompt.FullConfidence, &prompt.WitnessClaim{
				Call:   spec.Call{Operation: "price", Args: map[string]spec.Literal{}},
				Expect: spec.Observation{Kind: spec.Raises, Value: "ValueError"},
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name+verifiesSP174, func(t *testing.T) {
			got, err := prompt.ParseValidity([]byte(tc.output))
			if err != nil {
				t.Fatal(err)
			}
			if got.Answer != tc.answer || got.Confidence != tc.confidence {
				t.Errorf("answer %q confidence %v", got.Answer, got.Confidence)
			}
			if (got.Witness == nil) != (tc.witness == nil) {
				t.Fatalf("witness %+v, want %+v", got.Witness, tc.witness)
			}
			if tc.witness == nil {
				return
			}
			if got.Witness.Call.Operation != tc.witness.Call.Operation || len(got.Witness.Call.Args) != len(tc.witness.Call.Args) || got.Witness.Expect != tc.witness.Expect {
				t.Errorf("witness %+v, want %+v", got.Witness, tc.witness)
			}
			for name, literal := range tc.witness.Call.Args {
				if got.Witness.Call.Args[name] != literal {
					t.Errorf("argument %s is %v", name, got.Witness.Call.Args[name])
				}
			}
		})
	}
	t.Run("a response that is not JSON is refused"+verifiesSP174, func(t *testing.T) {
		if _, err := prompt.ParseValidity([]byte(brokenJSON)); err == nil {
			t.Error("no error")
		}
	})
}

func TestParseEquivalence(t *testing.T) {
	cases := []struct {
		name       string
		output     string
		answer     power.Answer
		confidence float64
	}{
		{"EQUIVALENT is an equivalence answer", `{"equivalent":"EQUIVALENT","confidence":0.9}`, power.AnswerEquivalent, 0.9},
		{"DIFFERENT is undetermined", `{"equivalent":"DIFFERENT","confidence":0.9}`, power.AnswerUndetermined, 0.9},
		{"UNDETERMINED is undetermined", `{"equivalent":"UNDETERMINED"}`, power.AnswerUndetermined, prompt.FullConfidence},
		{"a missing answer is undetermined", `{}`, power.AnswerUndetermined, prompt.FullConfidence},
		{"another spelling is undetermined", `{"equivalent":"equivalent"}`, power.AnswerUndetermined, prompt.FullConfidence},
		{"an equivalence answer without confidence counts as full confidence", `{"equivalent":"EQUIVALENT"}`, power.AnswerEquivalent, prompt.FullConfidence},
		{"an explicit zero confidence is kept", `{"equivalent":"EQUIVALENT","confidence":0}`, power.AnswerEquivalent, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name+verifiesSP174, func(t *testing.T) {
			got, err := prompt.ParseEquivalence([]byte(tc.output))
			if err != nil {
				t.Fatal(err)
			}
			if got.Answer != tc.answer || got.Confidence != tc.confidence || got.Witness != nil {
				t.Errorf("got %+v", got)
			}
		})
	}
	t.Run("a response that is not JSON is refused"+verifiesSP174, func(t *testing.T) {
		if _, err := prompt.ParseEquivalence([]byte(brokenJSON)); err == nil {
			t.Error("no error")
		}
	})
}

func TestParseScreen(t *testing.T) {
	t.Run("the answers are given by question id"+verifiesSP174, func(t *testing.T) {
		got, err := prompt.ParseScreen([]byte(`{"answers":[
			{"id":"q1","choice":"VIOLATES","confidence":0.6,"probabilities":{"VIOLATES":0.7,"CONSISTENT":0.3}},
			{"id":"q2","choice":"CONSISTENT"},
			{"id":"q3","choice":"VIOLATES","confidence":0,"probabilities":{"VIOLATES":0.55,"CONSISTENT":0.45}},
			{"id":"q4","choice":"CONSISTENT","confidence":0.9}]}`))
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]power.ClosedAnswer{
			"q1": {Choice: power.ChoiceViolates, Violates: 0.7, HasViolates: true},
			"q2": {Choice: power.ChoiceConsistent},
			"q3": {Choice: power.ChoiceViolates, Violates: 0.55, HasViolates: true},
			"q4": {Choice: power.ChoiceConsistent},
		}
		if len(got) != len(want) {
			t.Fatalf("answers %v", got)
		}
		for id, answer := range want {
			if got[id] != answer {
				t.Errorf("answer %s is %+v, want %+v", id, got[id], answer)
			}
		}
	})
	t.Run("a response without answers gives an empty map"+verifiesSP174, func(t *testing.T) {
		got, err := prompt.ParseScreen([]byte(`{"answers":[]}`))
		if err != nil || len(got) != 0 {
			t.Errorf("got %v, %v", got, err)
		}
	})
	t.Run("a response that is not JSON is refused"+verifiesSP174, func(t *testing.T) {
		if _, err := prompt.ParseScreen([]byte(brokenJSON)); err == nil {
			t.Error("no error")
		}
	})
	t.Run("a screen answer without confidence feeds the share of VIOLATES"+verifiesSP174, func(t *testing.T) {
		got, err := prompt.ParseScreen([]byte(`{"answers":[{"id":"q1","choice":"VIOLATES"},{"id":"q1","choice":"CONSISTENT"}]}`))
		if err != nil {
			t.Fatal(err)
		}
		if power.ViolationProbability([]power.ClosedAnswer{got["q1"]}) != 0 {
			t.Error("the last answer for an id does not win")
		}
	})
}

func TestFullConfidence(t *testing.T) {
	t.Run("a vote that states no confidence is a full confidence"+verifiesSP174, func(t *testing.T) {
		if prompt.FullConfidence != 1 {
			t.Errorf("FullConfidence = %v", prompt.FullConfidence)
		}
	})
}

// TestScreenReadsTheProbabilityOfViolates covers the path an endpoint's reply
// takes to the pool: translated with the question set of the screen, parsed, and
// turned into P(VIOLATES).
func TestScreenReadsTheProbabilityOfViolates(t *testing.T) {
	const (
		verifiesSP169 = " [verifies SP-169~2]"
		screenTau     = 0.20
	)
	built, err := prompt.BuildScreen(prompt.ScreenInput{Statements: []string{"Requirement text."}})
	if err != nil {
		t.Fatal(err)
	}
	set, err := hostproto.ParseClosedSet(built.Closed)
	if err != nil {
		t.Fatal(err)
	}
	question := prompt.QuestionID(0)

	violationOf := func(t *testing.T, reply string) float64 {
		t.Helper()
		output, err := set.AnswersFromSystemOne([]byte(reply))
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := prompt.ParseScreen(output)
		if err != nil {
			t.Fatal(err)
		}
		return power.ViolationProbability([]power.ClosedAnswer{parsed[question]})
	}

	t.Run("an endpoint reply with P(VIOLATES) of 0.55 and a concentration near zero flags the survivor"+verifiesSP169, func(t *testing.T) {
		reply := `{"answers":{"` + question + `":{"type":"choice","choice":"VIOLATES","probabilities":{"VIOLATES":0.55,"CONSISTENT":0.45},"confidence":0.0001}}}`
		probability := violationOf(t, reply)
		if math.Abs(probability-0.55) > 1e-9 || !power.FlagsSurvivor(probability, screenTau) {
			t.Fatalf("P(VIOLATES) = %v", probability)
		}
	})

	t.Run("an endpoint that picks CONSISTENT gives P(VIOLATES) of its own probability, whatever its confidence"+verifiesSP169, func(t *testing.T) {
		reply := `{"answers":{"` + question + `":{"type":"choice","choice":"CONSISTENT","probabilities":{"VIOLATES":0.3,"CONSISTENT":0.7},"confidence":0.9}}}`
		if probability := violationOf(t, reply); math.Abs(probability-0.3) > 1e-9 {
			t.Fatalf("P(VIOLATES) = %v", probability)
		}
	})

	t.Run("an endpoint that gives no probabilities counts as one vote, never as its confidence"+verifiesSP169, func(t *testing.T) {
		reply := `{"answers":{"` + question + `":{"type":"choice","choice":"CONSISTENT","confidence":0.1}}}`
		if probability := violationOf(t, reply); probability != 0 {
			t.Fatalf("P(VIOLATES) = %v", probability)
		}
	})

	t.Run("host votes give the share of VIOLATES votes, whatever confidence they state"+verifiesSP169, func(t *testing.T) {
		votes := []string{
			`{"answers":[{"id":"` + question + `","choice":"VIOLATES","confidence":0.1}]}`,
			`{"answers":[{"id":"` + question + `","choice":"CONSISTENT","confidence":0.1}]}`,
			`{"answers":[{"id":"` + question + `","choice":"CONSISTENT"}]}`,
		}
		var answers []power.ClosedAnswer
		for _, vote := range votes {
			parsed, err := prompt.ParseScreen([]byte(vote))
			if err != nil {
				t.Fatal(err)
			}
			answers = append(answers, parsed[question])
		}
		if probability := power.ViolationProbability(answers); math.Abs(probability-1.0/3.0) > 1e-9 {
			t.Fatalf("P(VIOLATES) = %v", probability)
		}
	})
}
