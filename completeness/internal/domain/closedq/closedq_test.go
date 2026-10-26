package closedq_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/completeness/internal/domain/closedq"
)

func options(names ...string) []closedq.Option {
	list := make([]closedq.Option, len(names))
	for index, name := range names {
		list[index] = closedq.Option{Name: name, Description: "the option " + name}
	}
	return list
}

func twoQuestions() closedq.Set {
	return closedq.Set{
		State: "the state",
		Questions: []closedq.Question{
			closedq.Choice("first", "Which?", options("A", "B")),
			closedq.Choice("second", "Which one?", options("X", "Y", "Z")),
		},
	}
}

func TestMarshalAndSchema(t *testing.T) {
	t.Run("the question set carries its state and every question with its options [verifies SP-514~1]", func(t *testing.T) {
		raw, err := twoQuestions().Marshal()
		if err != nil {
			t.Fatal(err)
		}
		var decoded closedq.Set
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.State != "the state" || len(decoded.Questions) != 2 || decoded.Questions[1].Type != closedq.TypeChoice {
			t.Fatalf("decoded %+v", decoded)
		}
		if names := decoded.Questions[1].OptionNames(); strings.Join(names, ",") != "X,Y,Z" {
			t.Fatalf("option names %v", names)
		}
	})

	t.Run("the schema holds exactly one answer per question, each with one of its options [verifies SP-514~1]", func(t *testing.T) {
		raw, err := twoQuestions().ResponseSchema()
		if err != nil {
			t.Fatal(err)
		}
		var schema struct {
			Properties struct {
				Answers struct {
					MinItems int `json:"minItems"`
					MaxItems int `json:"maxItems"`
				} `json:"answers"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		if schema.Properties.Answers.MinItems != 2 || schema.Properties.Answers.MaxItems != 2 {
			t.Fatalf("schema %s", raw)
		}
		if !strings.Contains(string(raw), `"const":"first"`) || !strings.Contains(string(raw), `"enum":["X","Y","Z"]`) {
			t.Fatalf("schema %s", raw)
		}
	})
}

func TestParse(t *testing.T) {
	t.Run("answers are read by question id, a missing confidence staying missing [verifies SP-514~1]", func(t *testing.T) {
		answers, err := closedq.Parse([]byte(`{"answers":[{"id":"first","choice":"A","confidence":0.75},{"id":"second","choice":"Z"}]}`))
		if err != nil {
			t.Fatal(err)
		}
		if got := answers["first"]; got.Choice != "A" || got.Confidence != 0.75 || !got.HasConfidence {
			t.Fatalf("first %+v", got)
		}
		if got := answers["second"]; got.Choice != "Z" || got.HasConfidence {
			t.Fatalf("second %+v", got)
		}
	})

	t.Run("a document that is not JSON is an error [verifies SP-514~1]", func(t *testing.T) {
		if _, err := closedq.Parse([]byte(`{"answers":`)); err == nil {
			t.Fatal("parsed a broken document")
		}
	})
}

func answer(choices ...string) []byte {
	parts := make([]string, len(choices))
	for index, choice := range choices {
		parts[index] = fmt.Sprintf(`{"id":"q%d","choice":%q}`, index, choice)
	}
	return []byte(`{"answers":[` + strings.Join(parts, ",") + `]}`)
}

func TestRead(t *testing.T) {
	t.Run("a relayed first vote stands for the host votes and gives the endpoint's own confidence [verifies SP-515~1]", func(t *testing.T) {
		reading := closedq.Read([]closedq.Vote{{
			Output:  []byte(`{"answers":[{"id":"q0","choice":"A","confidence":0.7},{"id":"q1","choice":"B"}]}`),
			Relayed: true, Source: "system-one:nimble:9b@abc",
		}})
		if reading.Source != "system-one:nimble:9b@abc" {
			t.Fatalf("source %q", reading.Source)
		}
		if got := reading.Questions["q0"]; got.Choice != "A" || got.Confidence != 0.7 {
			t.Fatalf("q0 %+v", got)
		}
		if got := reading.Questions["q1"]; got.Choice != "B" || got.Confidence != 0 {
			t.Fatalf("q1 %+v (an answer without confidence counts as zero)", got)
		}
	})

	t.Run("host votes give the modal choice of each question and its share as confidence [verifies SP-515~1]", func(t *testing.T) {
		reading := closedq.Read([]closedq.Vote{{Output: answer("A", "X")}, {Output: answer("A", "Y")}, {Output: answer("B", "Y")}})
		if reading.Source != "host-votes(3)" {
			t.Fatalf("source %q", reading.Source)
		}
		if got := reading.Questions["q0"]; got.Choice != "A" || got.Confidence < 0.66 || got.Confidence > 0.67 {
			t.Fatalf("q0 %+v", got)
		}
		if got := reading.Questions["q1"]; got.Choice != "Y" || got.Confidence < 0.66 || got.Confidence > 0.67 {
			t.Fatalf("q1 %+v", got)
		}
	})

	t.Run("a tie goes to the smaller choice and a vote that does not parse counts for nothing [verifies SP-515~1]", func(t *testing.T) {
		reading := closedq.Read([]closedq.Vote{{Output: answer("B")}, {Output: answer("A")}, {Output: []byte("garbage")}})
		if got := reading.Questions["q0"]; got.Choice != "A" || got.Confidence < 0.33 || got.Confidence > 0.34 {
			t.Fatalf("q0 %+v", got)
		}
	})

	t.Run("no vote reads as no answer [verifies SP-515~1]", func(t *testing.T) {
		if reading := closedq.Read(nil); len(reading.Questions) != 0 {
			t.Fatalf("reading %+v", reading)
		}
	})

	t.Run("the confidence of a host reading is a share in [0,1] and the choice one that was voted, whatever the votes [verifies SP-515~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			voted := rapid.SliceOfN(rapid.SampledFrom([]string{"A", "B", "C"}), 1, 9).Draw(t, "votes")
			votes := make([]closedq.Vote, len(voted))
			for index, choice := range voted {
				votes[index] = closedq.Vote{Output: answer(choice)}
			}
			decided := closedq.Read(votes).Questions["q0"]
			if decided.Confidence <= 0 || decided.Confidence > 1 {
				t.Fatalf("confidence %v", decided.Confidence)
			}
			found := false
			for _, choice := range voted {
				found = found || choice == decided.Choice
			}
			if !found {
				t.Fatalf("choice %q was never voted in %v", decided.Choice, voted)
			}
		})
	})
}

func TestSource(t *testing.T) {
	t.Run("a relayed answer has the system-one source and the endpoint's mapping, host votes the modal share [verifies SP-515~1]", func(t *testing.T) {
		relayed := closedq.Read([]closedq.Vote{{Output: answer("A"), Relayed: true, Source: "system-one:nimble:9b@abc"}})
		if relayed.Source != "system-one:nimble:9b@abc" || relayed.Mapping != "system-one.choice.confidence" {
			t.Fatalf("relayed %q %q", relayed.Source, relayed.Mapping)
		}
		voted := closedq.Read([]closedq.Vote{{Output: answer("A")}})
		if voted.Source != "host-votes(1)" || voted.Mapping != "host-votes.modal-share" {
			t.Fatalf("voted %q %q", voted.Source, voted.Mapping)
		}
	})

	t.Run("the source of k host votes is named host-votes(k) [verifies SP-515~1]", func(t *testing.T) {
		if got := closedq.HostVotesSource(5); got != "host-votes(5)" {
			t.Fatalf("source %q", got)
		}
	})
}
