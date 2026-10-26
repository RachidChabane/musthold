package prompt_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/discpower/internal/domain/prompt"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
	"github.com/RachidChabane/musthold/discpower/internal/domain/witness"
)

const (
	python      = "Python with pytest"
	conventions = "One module, plain functions named test_*."

	interfaceEntry = "def price(amount: int) -> int: ..."
	helperEntry    = "def make_order(total: int) -> Order: ..."
	hiddenBody     = "return amount * 3 + 41"

	minWitnessExamples = 3
	maxWitnessExamples = 6
	maxWitnessProps    = 3
)

var (
	refPark  = spec.Ref{ID: "PARK-1", Revision: 1}
	refParks = spec.Ref{ID: "PARK-10", Revision: 2}
)

func parkLabels() prompt.Labels { return prompt.NewLabels([]spec.Ref{refParks, refPark}) }

func witnessDocument(imports string, examples, properties int) string {
	var items []string
	for index := range examples {
		items = append(items, witnessItem(fmt.Sprintf("test_example_%d", index), "example"))
	}
	for index := range properties {
		items = append(items, witnessItem(fmt.Sprintf("test_property_%d", index), "property"))
	}
	return fmt.Sprintf(`{"imports":%q,"witnesses":[%s]}`, imports, strings.Join(items, ","))
}

func witnessItem(name, kind string) string {
	return fmt.Sprintf(`{"name":%q,"clause":"the amount is rounded","kind":%q,"code":"def %s():\n    assert price(1) == 1\n"}`, name, kind, name)
}

func TestLabels(t *testing.T) {
	t.Run("a criterion is labelled by its position among the sorted references"+verifiesSP173, func(t *testing.T) {
		labels := parkLabels()
		if got := labels.Of(refPark); got != "R1" {
			t.Errorf("Of(%s) = %q", refPark, got)
		}
		if got := labels.Of(refParks); got != "R2" {
			t.Errorf("Of(%s) = %q", refParks, got)
		}
	})
	t.Run("an unknown reference has no label"+verifiesSP173, func(t *testing.T) {
		if got := parkLabels().Of(spec.Ref{ID: "PARK-2", Revision: 1}); got != "" {
			t.Errorf("got %q", got)
		}
		if got := parkLabels().Of(spec.Ref{ID: refPark.ID, Revision: refPark.Revision + 1}); got != "" {
			t.Errorf("another revision got %q", got)
		}
	})
	t.Run("a duplicated reference is labelled once"+verifiesSP173, func(t *testing.T) {
		labels := prompt.NewLabels([]spec.Ref{refPark, refPark, refParks})
		if got := labels.Of(refParks); got != "R2" {
			t.Errorf("got %q", got)
		}
	})
	for _, c := range []struct {
		name string
		text string
		want string
	}{
		{"a full reference is replaced by its label", "see PARK-1~1", "see R1"},
		{"a bare identifier is replaced by its label", "see PARK-1 here", "see R1 here"},
		{"the longest identifier is replaced first", "PARK-10~2 then PARK-1~1 then PARK-10 then PARK-1", "R2 then R1 then R2 then R1"},
		{"a replaced text is never replaced again", "PARK-10 R1", "R2 R1"},
		{"an identifier that is not labelled stays", "PARK-3~1 and PARK-1", "PARK-3~1 and R1"},
		{"a text naming nothing stays", "plain text", "plain text"},
	} {
		t.Run(c.name+verifiesSP173, func(t *testing.T) {
			if got := parkLabels().Relabel(c.text); got != c.want {
				t.Errorf("Relabel(%q) = %q, want %q", c.text, got, c.want)
			}
		})
	}
	t.Run("no labelled identifier survives relabelling"+verifiesSP173, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			number := rapid.IntRange(1, 99).Draw(t, "number")
			revision := rapid.IntRange(1, 9).Draw(t, "revision")
			ref := spec.Ref{ID: fmt.Sprintf("PARK-%d", number), Revision: revision}
			labels := prompt.NewLabels([]spec.Ref{ref, refPark, refParks})
			text := fmt.Sprintf("a %s b %s c %s", ref, ref.ID, refPark)
			if got := labels.Relabel(text); strings.Contains(got, "PARK") {
				t.Fatalf("%q still names a criterion", got)
			}
		})
	})
}

func TestBuildWitness(t *testing.T) {
	labels := parkLabels()
	input := prompt.WitnessInput{
		Spec:        labels.Relabel("PARK-1~1 sets the fee. PARK-10 overrides PARK-1."),
		Target:      labels.Of(refPark),
		Statement:   labels.Relabel("WHEN a car leaves THE SYSTEM SHALL charge as PARK-10~2 says"),
		Language:    python,
		Conventions: conventions,
		Interface:   []string{interfaceEntry},
		Helpers:     []string{helperEntry + "\n"},
	}
	built := prompt.BuildWitness(input)
	t.Run("the prompt names no criterion by its identifier when given relabelled text"+verifiesSP173, func(t *testing.T) {
		for _, text := range []string{built.Prompt.System, built.Prompt.User} {
			if found := identifierPattern.FindString(text); found != "" {
				t.Errorf("the prompt names %s", found)
			}
		}
		if !strings.Contains(built.Prompt.User, "R1") || !strings.Contains(built.Prompt.User, "R2") {
			t.Errorf("the labels are missing from %q", built.Prompt.User)
		}
	})
	t.Run("the prompt shows the interface, the helpers, the language and the conventions"+verifiesSP173, func(t *testing.T) {
		for _, want := range []string{interfaceEntry, helperEntry, python, conventions} {
			if !strings.Contains(built.Prompt.User, want) {
				t.Errorf("%q is missing", want)
			}
		}
	})
	t.Run("the prompt holds no body of the code"+verifiesSP173, func(t *testing.T) {
		if strings.Contains(built.Prompt.User, hiddenBody) || strings.Contains(built.Prompt.System, hiddenBody) {
			t.Error("a body is shown")
		}
	})
	t.Run("an empty interface and no helper are shown as none"+verifiesSP173, func(t *testing.T) {
		empty := prompt.BuildWitness(prompt.WitnessInput{})
		if got := strings.Count(empty.Prompt.User, "(none)"); got != 2 {
			t.Errorf("%d absent lists marked", got)
		}
	})
	t.Run("the response schema is the witness schema and the digest is stable"+verifiesSP173, func(t *testing.T) {
		if !strings.Contains(string(built.Schema), `"witnesses"`) {
			t.Errorf("schema %s", built.Schema)
		}
		if again := prompt.BuildWitness(prompt.WitnessInput{}); again.Prompt.TemplateDigest != built.Prompt.TemplateDigest {
			t.Error("the digest depends on the input")
		}
	})
}

func TestBuildScope(t *testing.T) {
	labels := parkLabels()
	input := prompt.ScopeInput{
		Spec:      labels.Relabel("PARK-1~1 sets the fee."),
		Target:    labels.Of(refPark),
		Statement: labels.Relabel("WHEN a car leaves THE SYSTEM SHALL charge per PARK-10"),
		Witnesses: []prompt.ScopeWitness{
			{Clause: "the fee is charged", Code: "def test_fee():\n    assert fee(1) == 2\n"},
			{Clause: "the fee is rounded", Code: "def test_round():\n    assert fee(2) == 3\n"},
		},
	}
	built := prompt.BuildScope(input)
	t.Run("the prompt names no criterion by its identifier when given relabelled text"+verifiesSP173, func(t *testing.T) {
		for _, text := range []string{built.Prompt.System, built.Prompt.User} {
			if found := identifierPattern.FindString(text); found != "" {
				t.Errorf("the prompt names %s", found)
			}
		}
	})
	t.Run("every witness is shown under its opaque label with its clause and code"+verifiesSP173, func(t *testing.T) {
		for index, item := range input.Witnesses {
			label := prompt.ScopeLabel(index)
			for _, want := range []string{"Witness " + label, item.Clause, strings.TrimSpace(item.Code)} {
				if !strings.Contains(built.Prompt.User, want) {
					t.Errorf("%q is missing", want)
				}
			}
		}
	})
	t.Run("the scope labels are w1, w2 and so on"+verifiesSP173, func(t *testing.T) {
		if prompt.ScopeLabel(0) != "w1" || prompt.ScopeLabel(1) != "w2" {
			t.Errorf("got %s and %s", prompt.ScopeLabel(0), prompt.ScopeLabel(1))
		}
	})
	t.Run("the response schema is the scope schema"+verifiesSP173, func(t *testing.T) {
		if !strings.Contains(string(built.Schema), `"verdicts"`) {
			t.Errorf("schema %s", built.Schema)
		}
	})
}

func TestParseWitnesses(t *testing.T) {
	for examples := range maxWitnessExamples + 2 {
		for properties := range maxWitnessProps + 2 {
			complete := examples >= minWitnessExamples && examples <= maxWitnessExamples && properties >= 1 && properties <= maxWitnessProps
			t.Run(fmt.Sprintf("%d examples and %d properties", examples, properties)+verifiesSP174, func(t *testing.T) {
				set, err := prompt.ParseWitnesses([]byte(witnessDocument("import math", examples, properties)), refPark)
				if complete {
					if err != nil {
						t.Fatalf("refused: %v", err)
					}
					if len(set.Witnesses) != examples+properties || set.Criterion != refPark || set.Imports != "import math" {
						t.Errorf("got %+v", set)
					}
					return
				}
				if !errors.Is(err, prompt.ErrIncompleteWitnesses) {
					t.Errorf("err = %v", err)
				}
			})
		}
	}
	t.Run("the witnesses carry their content digest, clause and kind"+verifiesSP174, func(t *testing.T) {
		set, err := prompt.ParseWitnesses([]byte(witnessDocument("", minWitnessExamples, 1)), refPark)
		if err != nil {
			t.Fatal(err)
		}
		first := set.Witnesses[0]
		want := witness.New(refPark, first.Name, first.Clause, first.Kind, first.Code)
		if first.ID == "" || first != want || first.Kind != witness.KindExample {
			t.Errorf("got %+v", first)
		}
	})
	t.Run("two witnesses with one name are refused"+verifiesSP174, func(t *testing.T) {
		items := []string{witnessItem("same", "example"), witnessItem("same", "example"), witnessItem("b", "example"), witnessItem("c", "example"), witnessItem("d", "property")}
		_, err := prompt.ParseWitnesses([]byte(`{"imports":"","witnesses":[`+strings.Join(items, ",")+`]}`), refPark)
		if !errors.Is(err, prompt.ErrDuplicateWitness) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("an unknown kind is refused"+verifiesSP174, func(t *testing.T) {
		items := []string{witnessItem("a", "example"), witnessItem("b", "example"), witnessItem("c", "example"), witnessItem("d", "fuzz")}
		if _, err := prompt.ParseWitnesses([]byte(`{"imports":"","witnesses":[`+strings.Join(items, ",")+`]}`), refPark); err == nil {
			t.Error("accepted")
		}
	})
	t.Run("a response that is not JSON is refused"+verifiesSP174, func(t *testing.T) {
		if _, err := prompt.ParseWitnesses([]byte(brokenJSON), refPark); err == nil {
			t.Error("accepted")
		}
	})
	t.Run("a complete set is accepted whatever the counts inside the bounds"+verifiesSP174, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			examples := rapid.IntRange(minWitnessExamples, maxWitnessExamples).Draw(t, "examples")
			properties := rapid.IntRange(1, maxWitnessProps).Draw(t, "properties")
			set, err := prompt.ParseWitnesses([]byte(witnessDocument("", examples, properties)), refPark)
			if err != nil || !set.Complete() {
				t.Fatalf("refused %d examples and %d properties: %v", examples, properties, err)
			}
		})
	})
}

func TestParseScope(t *testing.T) {
	labels := []string{prompt.ScopeLabel(0), prompt.ScopeLabel(1)}
	t.Run("a vote is returned per label with its confidence"+verifiesSP174, func(t *testing.T) {
		votes, err := prompt.ParseScope([]byte(`{"verdicts":{"w1":{"answer":"FIXED","confidence":0.8},"w2":{"answer":"NOT_FIXED","confidence":0.3}}}`), labels)
		if err != nil {
			t.Fatal(err)
		}
		if votes["w1"].Answer != witness.ScopeFixed || votes["w1"].Confidence != 0.8 {
			t.Errorf("w1 = %+v", votes["w1"])
		}
		if votes["w2"].Answer != witness.ScopeNotFixed || votes["w2"].Confidence != 0.3 {
			t.Errorf("w2 = %+v", votes["w2"])
		}
	})
	t.Run("a vote that states no confidence counts as full confidence"+verifiesSP174, func(t *testing.T) {
		votes, err := prompt.ParseScope([]byte(`{"verdicts":{"w1":{"answer":"FIXED"},"w2":{"answer":"UNDETERMINED"}}}`), labels)
		if err != nil {
			t.Fatal(err)
		}
		for label, vote := range votes {
			if vote.Confidence != prompt.FullConfidence {
				t.Errorf("%s has confidence %v", label, vote.Confidence)
			}
		}
	})
	t.Run("a stated confidence of zero is kept"+verifiesSP174, func(t *testing.T) {
		votes, err := prompt.ParseScope([]byte(`{"verdicts":{"w1":{"answer":"FIXED","confidence":0}}}`), labels[:1])
		if err != nil || votes["w1"].Confidence != 0 {
			t.Errorf("got %+v, %v", votes, err)
		}
	})
	t.Run("a label left unanswered is refused"+verifiesSP174, func(t *testing.T) {
		_, err := prompt.ParseScope([]byte(`{"verdicts":{"w1":{"answer":"FIXED"}}}`), labels)
		if !errors.Is(err, prompt.ErrUnansweredWitness) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("an answer to a label that was not shown is ignored"+verifiesSP174, func(t *testing.T) {
		votes, err := prompt.ParseScope([]byte(`{"verdicts":{"w1":{"answer":"FIXED"},"w9":{"answer":"FIXED"}}}`), labels[:1])
		if err != nil || len(votes) != 1 {
			t.Errorf("got %+v, %v", votes, err)
		}
	})
	t.Run("a response that is not JSON is refused"+verifiesSP174, func(t *testing.T) {
		if _, err := prompt.ParseScope([]byte(brokenJSON), labels); err == nil {
			t.Error("accepted")
		}
	})
}
