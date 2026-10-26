package systemonewire_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/s1relay/internal/adapters/systemonewire"
	"github.com/RachidChabane/musthold/s1relay/internal/domain/question"
	"github.com/RachidChabane/musthold/s1relay/internal/testkit/stubendpoint"
)

// skySet is the question the recorded exchanges asked.
func skySet() question.Set {
	return question.Set{
		State: "the sky is green",
		Questions: []question.Question{{
			ID: "color", Type: question.TypeChoice, Text: "Which color does the text say the sky is?",
			Options: []question.Option{{Name: "blue"}, {Name: "green"}, {Name: "grey"}},
		}},
	}
}

func coverageSet() question.Set {
	return question.Set{
		State: "a test",
		Questions: []question.Question{
			{ID: "criterion", Type: question.TypeChoice, Text: "Which?", Options: []question.Option{{Name: "K-2", Description: "second"}, {Name: "K-1", Description: "first"}, {Name: "NONE", Description: "none"}}},
			{ID: "coverage", Type: question.TypeScore, Text: "How much?", Options: []question.Option{{Name: "NONE"}, {Name: "WEAK"}, {Name: "PARTIAL"}, {Name: "FULL"}}},
		},
	}
}

func TestEncodeRequest(t *testing.T) {
	t.Run("a choice is posted exactly as the recorded Ollama request [verifies SP-604~1]", func(t *testing.T) {
		body, err := systemonewire.EncodeRequest("tev1:0.8b", skySet())
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != stubendpoint.RecordedOllamaChoiceRequest {
			t.Fatalf("body\n%s\nwant\n%s", body, stubendpoint.RecordedOllamaChoiceRequest)
		}
	})

	t.Run("the model is left out when none is named [verifies SP-604~1]", func(t *testing.T) {
		body, err := systemonewire.EncodeRequest("", skySet())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), `"model"`) {
			t.Fatalf("body %s names a model", body)
		}
	})

	t.Run("questions and options keep the order of the set, not the order of their names [verifies SP-604~1]", func(t *testing.T) {
		body, err := systemonewire.EncodeRequest("m", coverageSet())
		if err != nil {
			t.Fatal(err)
		}
		want := `{"model":"m","state":"a test","questions":{"criterion":{"type":"choice","instructions":"Which?","criteria":{"K-2":"second","K-1":"first","NONE":"none"}},"coverage":{"type":"score","instructions":"How much?","criteria":["NONE","WEAK","PARTIAL","FULL"]}}}`
		if string(body) != want {
			t.Fatalf("body\n%s\nwant\n%s", body, want)
		}
		if !json.Valid(body) {
			t.Fatal("the body is not JSON")
		}
	})

	t.Run("a set the route cannot take is refused before any request [verifies SP-616~1]", func(t *testing.T) {
		set := question.Set{State: "s"}
		for index := 0; index <= systemonewire.MaxQuestions; index++ {
			set.Questions = append(set.Questions, question.Question{ID: fmt.Sprintf("q%d", index), Type: question.TypeChoice, Options: []question.Option{{Name: "A"}}})
		}
		if _, err := systemonewire.EncodeRequest("m", set); !errors.Is(err, systemonewire.ErrTooManyQuestions) {
			t.Fatalf("error %v", err)
		}
		if _, err := systemonewire.EncodeRequest("m", question.Set{}); !errors.Is(err, question.ErrNoQuestion) {
			t.Fatalf("an empty set: error %v", err)
		}
	})
}

func TestDecodeClosed(t *testing.T) {
	t.Run("options given as objects with a description, as alignment and completeness emit them [verifies SP-604~1]", func(t *testing.T) {
		raw := `{"state":"S","questions":[{"id":"criterion","type":"choice","instructions":"Which?","options":[{"name":"K-1","description":"first"},{"name":"NONE","description":"No listed requirement is checked."}]},{"id":"coverage","type":"score","instructions":"How much?","options":[{"name":"NONE","description":"NONE"},{"name":"FULL","description":"FULL"}]}]}`
		set, err := systemonewire.DecodeClosed(json.RawMessage(raw))
		if err != nil {
			t.Fatal(err)
		}
		if set.State != "S" || len(set.Questions) != 2 || set.Questions[1].Type != question.TypeScore || set.Questions[0].Options[0].Description != "first" {
			t.Fatalf("set %+v", set)
		}
	})

	t.Run("a noul question is one the relay does not carry [verifies SP-604~1]", func(t *testing.T) {
		raw := `{"state":"S","questions":[{"id":"q","type":"noul","instructions":"Is it?"}]}`
		if _, err := systemonewire.DecodeClosed(json.RawMessage(raw)); !errors.Is(err, question.ErrUnsupportedType) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a closed request that is not readable or not askable is an error [verifies SP-616~1]", func(t *testing.T) {
		for name, raw := range map[string]string{
			"not JSON":         `{"state":`,
			"a bare option":    `{"state":"S","questions":[{"id":"q","type":"choice","instructions":"Which?","options":["a","b"]}]}`,
			"an unknown type":  `{"state":"S","questions":[{"id":"q","type":"free","instructions":"Which?","options":[{"name":"a","description":"a"}]}]}`,
			"no question":      `{"state":"S","questions":[]}`,
			"a duplicate id":   `{"state":"S","questions":[{"id":"q","type":"choice","instructions":"W","options":[{"name":"a","description":"a"},{"name":"b","description":"b"}]},{"id":"q","type":"choice","instructions":"W","options":[{"name":"a","description":"a"},{"name":"b","description":"b"}]}]}`,
			"a missing option": `{"state":"S","questions":[{"id":"q","type":"choice","instructions":"W","options":[]}]}`,
		} {
			if _, err := systemonewire.DecodeClosed(json.RawMessage(raw)); err == nil {
				t.Errorf("%s: decoded", name)
			}
		}
	})
}

func TestExportAndAnswers(t *testing.T) {
	t.Run("the export of a module is read [verifies SP-600~2]", func(t *testing.T) {
		exported, err := systemonewire.DecodeExport([]byte(`[{"ticket":"abc.0.1","nonce":"n0","closed":{"state":"S"}}]`))
		if err != nil || len(exported) != 1 || exported[0].Ticket != "abc.0.1" || exported[0].Nonce != "n0" {
			t.Fatalf("export %+v (%v)", exported, err)
		}
		if _, err := systemonewire.DecodeExport([]byte(`{`)); err == nil {
			t.Fatal("a broken export was read")
		}
	})
}

func TestDecodeReply(t *testing.T) {
	t.Run("the recorded Ollama reply is a choice with the confidence the server stated [verifies SP-605~1]", func(t *testing.T) {
		answers, model, err := systemonewire.DecodeReply([]byte(stubendpoint.RecordedOllamaChoiceReply), skySet())
		if err != nil {
			t.Fatal(err)
		}
		if model != "tev1:0.8b" || len(answers) != 1 || answers[0].Choice != "green" || answers[0].QuestionID != "color" {
			t.Fatalf("model %q answers %+v", model, answers)
		}
		if answers[0].Confidence == nil || *answers[0].Confidence != 0.9674686077174837 {
			t.Fatalf("confidence %v", answers[0].Confidence)
		}
	})

	t.Run("the recorded Kev reply is read the same way and its latency is ignored [verifies SP-605~1]", func(t *testing.T) {
		answers, model, err := systemonewire.DecodeReply([]byte(stubendpoint.RecordedKevChoiceReply), skySet())
		if err != nil {
			t.Fatal(err)
		}
		if model != "kev-latest" || answers[0].Choice != "green" || *answers[0].Confidence != 0.8802 {
			t.Fatalf("model %q answers %+v", model, answers)
		}
	})

	t.Run("a score maps to the nearest level and a missing confidence stays absent [verifies SP-605~1]", func(t *testing.T) {
		reply := `{"model":"m","answers":{"criterion":{"type":"choice","choice":"K-1"},"coverage":{"type":"score","score":2.4,"legend":["NONE","WEAK","PARTIAL","FULL"],"confidence":0.7}}}`
		answers, _, err := systemonewire.DecodeReply([]byte(reply), coverageSet())
		if err != nil {
			t.Fatal(err)
		}
		if answers[0].Choice != "K-1" || answers[0].Confidence != nil {
			t.Fatalf("criterion %+v", answers[0])
		}
		if answers[1].Choice != "PARTIAL" || *answers[1].Confidence != 0.7 {
			t.Fatalf("coverage %+v", answers[1])
		}
		reply = strings.Replace(reply, "2.4", "2.6", 1)
		if answers, _, _ = systemonewire.DecodeReply([]byte(reply), coverageSet()); answers[1].Choice != "FULL" {
			t.Fatalf("2.6 maps to %q", answers[1].Choice)
		}
	})

	t.Run("a reply that does not answer the question in the type asked is malformed [verifies SP-606~1]", func(t *testing.T) {
		replies := map[string]string{
			"not JSON":                    `{"answers":`,
			"a missing question":          `{"answers":{}}`,
			"a noul to a choice":          stubendpoint.RecordedOllamaNoulReply,
			"a choice with no choice":     `{"answers":{"color":{"type":"choice"}}}`,
			"a choice for a score":        `{"answers":{"criterion":{"type":"choice","choice":"K-1"},"coverage":{"type":"choice","choice":"FULL"}}}`,
			"a score above the levels":    `{"answers":{"criterion":{"type":"choice","choice":"K-1"},"coverage":{"type":"score","score":4}}}`,
			"a negative score":            `{"answers":{"criterion":{"type":"choice","choice":"K-1"},"coverage":{"type":"score","score":-1}}}`,
			"an answer that is no object": `{"answers":{"color":"green"}}`,
		}
		for name, reply := range replies {
			set := skySet()
			if strings.Contains(name, "score") || strings.Contains(name, "criterion") {
				set = coverageSet()
			}
			if _, _, err := systemonewire.DecodeReply([]byte(reply), set); err == nil {
				t.Errorf("%s: decoded", name)
			}
		}
	})
}
