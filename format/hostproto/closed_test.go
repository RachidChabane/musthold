package hostproto_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

func option(name string) hostproto.ClosedOption {
	return hostproto.ClosedOption{Name: name, Description: "when the answer is " + name}
}

func options(names ...string) []hostproto.ClosedOption {
	listed := make([]hostproto.ClosedOption, len(names))
	for index, name := range names {
		listed[index] = option(name)
	}
	return listed
}

func mixedSet() hostproto.ClosedSet {
	return hostproto.ClosedSet{State: "the state", Questions: []hostproto.ClosedQuestion{
		hostproto.ChoiceQuestion("pick", "Which one?", options("A", "B", "NONE")),
		hostproto.NoulQuestion("claims", "Does it claim it?", "It claims it.", "It does not."),
		hostproto.ScoreQuestion("coverage", "How much?", options("LOW", "MID", "HIGH")),
	}}
}

func problemPathsOf(set hostproto.ClosedSet) []string {
	var paths []string
	for _, problem := range set.Problems() {
		paths = append(paths, problem.Path)
	}
	slices.Sort(paths)
	return paths
}

func TestClosedSetValidation(t *testing.T) {
	t.Run("a set of the three question types is valid and survives a round trip [verifies SP-402~1]", func(t *testing.T) {
		set := mixedSet()
		if problems := set.Problems(); len(problems) > 0 {
			t.Fatalf("problems: %v", problems)
		}
		encoded, err := set.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := hostproto.ParseClosedSet(encoded)
		if err != nil {
			t.Fatal(err)
		}
		again, _ := parsed.Marshal()
		if string(encoded) != string(again) {
			t.Fatalf("the set changed:\n%s\n%s", encoded, again)
		}
	})

	cases := []struct {
		name string
		edit func(*hostproto.ClosedSet)
		want []string
	}{
		{"no state", func(s *hostproto.ClosedSet) { s.State = "" }, []string{"closed.state"}},
		{"no question", func(s *hostproto.ClosedSet) { s.Questions = nil }, []string{"closed.questions"}},
		{"too many questions", func(s *hostproto.ClosedSet) {
			s.Questions = nil
			for index := range hostproto.MaxClosedQuestions + 1 {
				s.Questions = append(s.Questions, hostproto.NoulQuestion(fmt.Sprintf("q%d", index), "q", "y", "n"))
			}
		}, []string{"closed.questions"}},
		{"a question id used twice", func(s *hostproto.ClosedSet) { s.Questions[1].ID = "pick" }, []string{"closed.questions[1].id"}},
		{"an empty question id", func(s *hostproto.ClosedSet) { s.Questions[0].ID = "" }, []string{"closed.questions[0].id"}},
		{"no instructions", func(s *hostproto.ClosedSet) { s.Questions[0].Instructions = "" }, []string{"closed.questions[0].instructions"}},
		{"an unknown type", func(s *hostproto.ClosedSet) { s.Questions[0].Type = "free" }, []string{"closed.questions[0].type"}},
		{"a choice with one option", func(s *hostproto.ClosedSet) { s.Questions[0].Options = options("A") }, []string{"closed.questions[0].options"}},
		{"a choice with too many options", func(s *hostproto.ClosedSet) {
			s.Questions[0].Options = nil
			for index := range hostproto.MaxChoiceOptions + 1 {
				s.Questions[0].Options = append(s.Questions[0].Options, option(string(rune('a'+index))))
			}
		}, []string{"closed.questions[0].options"}},
		{"an option named twice", func(s *hostproto.ClosedSet) { s.Questions[0].Options = options("A", "A") }, []string{"closed.questions[0].options[1].name"}},
		{"an option without criteria text", func(s *hostproto.ClosedSet) { s.Questions[0].Options[0].Description = "" }, []string{"closed.questions[0].options[0].description"}},
		{"a score with one level", func(s *hostproto.ClosedSet) { s.Questions[2].Options = options("LOW") }, []string{"closed.questions[2].options"}},
		{"a noul with other options", func(s *hostproto.ClosedSet) { s.Questions[1].Options = options("yes", "no") }, []string{"closed.questions[1].options"}},
		{"a noul with its options reversed", func(s *hostproto.ClosedSet) { slices.Reverse(s.Questions[1].Options) }, []string{"closed.questions[1].options"}},
	}
	for _, c := range cases {
		t.Run(c.name+" is refused [verifies SP-402~1]", func(t *testing.T) {
			set := mixedSet()
			c.edit(&set)
			got := problemPathsOf(set)
			want := slices.Clone(c.want)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Fatalf("problems at %v, want %v", got, want)
			}
		})
	}

	t.Run("a noul question may leave its criteria out [verifies SP-402~1]", func(t *testing.T) {
		set := mixedSet()
		set.Questions[1].Options = nil
		if problems := set.Problems(); len(problems) > 0 {
			t.Fatalf("problems: %v", problems)
		}
	})

	t.Run("a member the format does not define is refused [verifies SP-402~1]", func(t *testing.T) {
		_, err := hostproto.ParseClosedSet(json.RawMessage(`{"state":"s","questions":[],"extra":1}`))
		if err == nil {
			t.Fatal("an unknown member was accepted")
		}
		_, err = hostproto.ParseClosedSet(json.RawMessage(`[1]`))
		if err == nil {
			t.Fatal("a non-object was accepted")
		}
	})

	t.Run("a request whose set breaks a rule names it under closed [verifies SP-402~1]", func(t *testing.T) {
		request := hostprototest.ClosedRequest(func(r *hostproto.Request) { r.Closed = json.RawMessage(`{"state":"","questions":[]}`) })
		var invalid *hostproto.InvalidError
		if err := request.Validate(); !errors.As(err, &invalid) || len(invalid.Problems) != 2 {
			t.Fatalf("validate: %v", err)
		}
		notJSON := hostprototest.ClosedRequest(func(r *hostproto.Request) { r.Closed = json.RawMessage(`[`) })
		if err := notJSON.Validate(); err == nil {
			t.Fatal("a closed member that is not JSON was accepted")
		}
	})
}

func TestClosedResponseSchema(t *testing.T) {
	set := mixedSet()
	schema, err := set.ResponseSchema()
	if err != nil {
		t.Fatal(err)
	}

	t.Run("the schema asks for one answer per question whose choice is one of its options [verifies SP-403~2]", func(t *testing.T) {
		var document struct {
			Properties struct {
				Answers struct {
					MinItems int `json:"minItems"`
					MaxItems int `json:"maxItems"`
					Items    struct {
						OneOf []struct {
							Properties struct {
								ID     struct{ Const string }  `json:"id"`
								Choice struct{ Enum []string } `json:"choice"`
							} `json:"properties"`
						} `json:"oneOf"`
					} `json:"items"`
					AllOf []json.RawMessage `json:"allOf"`
				} `json:"answers"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(schema, &document); err != nil {
			t.Fatal(err)
		}
		answers := document.Properties.Answers
		if answers.MinItems != 3 || answers.MaxItems != 3 || len(answers.AllOf) != 3 || len(answers.Items.OneOf) != 3 {
			t.Fatalf("answers = %+v", answers)
		}
		wantChoices := [][]string{{"A", "B", "NONE"}, {"true", "false"}, {"LOW", "MID", "HIGH"}}
		for index, form := range answers.Items.OneOf {
			if form.Properties.ID.Const != set.Questions[index].ID || !slices.Equal(form.Properties.Choice.Enum, wantChoices[index]) {
				t.Errorf("form %d = %+v", index, form.Properties)
			}
		}
	})

	t.Run("answers are read by id and written in the order of the questions [verifies SP-403~2]", func(t *testing.T) {
		answers, err := hostproto.ParseClosedAnswers([]byte(`{"answers":[{"id":"coverage","choice":"HIGH"},{"id":"pick","choice":"A","confidence":0.8}]}`))
		if err != nil {
			t.Fatal(err)
		}
		if pick := answers["pick"]; pick.Choice != "A" || pick.Confidence != 0.8 || !pick.HasConfidence || pick.Probabilities != nil || answers["coverage"].HasConfidence {
			t.Fatalf("answers = %+v", answers)
		}
		answers["claims"] = hostproto.ClosedAnswer{Choice: "true"}
		document, err := set.AnswerDocument(answers)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(document), `{"answers":[{"choice":"A","confidence":0.8,"id":"pick"}`) {
			t.Fatalf("document = %s", document)
		}
		delete(answers, "claims")
		if _, err := set.AnswerDocument(answers); err == nil {
			t.Fatal("an unanswered question was written")
		}
		if _, err := hostproto.ParseClosedAnswers([]byte(`{`)); err == nil {
			t.Fatal("a truncated document was read")
		}
	})
}

func TestSystemOneRendering(t *testing.T) {
	set := mixedSet()

	t.Run("the questions are named, typed and carry their criteria, in order [verifies SP-404~2]", func(t *testing.T) {
		body, err := set.SystemOneRequest("tev1:0.8b")
		if err != nil {
			t.Fatal(err)
		}
		want := `{"model":"tev1:0.8b","state":"the state","questions":{` +
			`"pick":{"type":"choice","instructions":"Which one?","criteria":{"A":"when the answer is A","B":"when the answer is B","NONE":"when the answer is NONE"}},` +
			`"claims":{"type":"noul","instructions":"Does it claim it?","criteria":{"true":"It claims it.","false":"It does not."}},` +
			`"coverage":{"type":"score","instructions":"How much?","criteria":["when the answer is LOW","when the answer is MID","when the answer is HIGH"]}}}`
		if string(body) != want {
			t.Fatalf("body:\n%s\nwant:\n%s", body, want)
		}
		if !json.Valid(body) {
			t.Fatal("the body is not JSON")
		}
	})

	t.Run("a noul question without criteria sends none [verifies SP-404~2]", func(t *testing.T) {
		bare := hostproto.ClosedSet{State: "s", Questions: []hostproto.ClosedQuestion{{ID: "q", Type: hostproto.ClosedNoul, Instructions: "Is it?"}}}
		body, err := bare.SystemOneRequest("m")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "criteria") {
			t.Fatalf("body = %s", body)
		}
	})

	response := `{"model":"m","answers":{` +
		`"pick":{"type":"choice","choice":"B","probabilities":{"A":0.1,"B":0.8,"NONE":0.1},"confidence":0.8},` +
		`"claims":{"type":"noul","noul":0.2},` +
		`"coverage":{"type":"score","score":1.6,"legend":["LOW","MID","HIGH"],"confidence":0.7}},` +
		`"usage":{"input_tokens":10,"output_tokens":1}}`

	t.Run("the response of the endpoint becomes the answers of the set [verifies SP-404~2]", func(t *testing.T) {
		document, err := set.AnswersFromSystemOne([]byte(response))
		if err != nil {
			t.Fatal(err)
		}
		answers, err := hostproto.ParseClosedAnswers(document)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]hostproto.ClosedAnswer{
			"pick":     {Choice: "B", Confidence: 0.8, HasConfidence: true},
			"claims":   {Choice: "false", Confidence: 0.8, HasConfidence: true},
			"coverage": {Choice: "HIGH", Confidence: 0.7, HasConfidence: true},
		}
		for id, expected := range want {
			got := answers[id]
			if got.Choice != expected.Choice || !got.HasConfidence || got.Confidence-expected.Confidence > 1e-9 || expected.Confidence-got.Confidence > 1e-9 {
				t.Errorf("%s: %+v, want %+v", id, got, expected)
			}
		}
	})

	t.Run("a noul probability of one half is true and a score is clamped to its levels [verifies SP-404~2]", func(t *testing.T) {
		document, err := set.AnswersFromSystemOne([]byte(`{"answers":{"pick":{"type":"choice","choice":"A"},"claims":{"type":"noul","noul":0.5},"coverage":{"type":"score","score":9}}}`))
		if err != nil {
			t.Fatal(err)
		}
		answers, _ := hostproto.ParseClosedAnswers(document)
		if answers["claims"].Choice != hostproto.NoulTrue || answers["coverage"].Choice != "HIGH" || answers["pick"].HasConfidence {
			t.Fatalf("answers = %+v", answers)
		}
		low, _ := set.AnswersFromSystemOne([]byte(`{"answers":{"pick":{"type":"choice","choice":"A"},"claims":{"type":"noul","noul":0.5},"coverage":{"type":"score","score":-3}}}`))
		answers, _ = hostproto.ParseClosedAnswers(low)
		if answers["coverage"].Choice != "LOW" {
			t.Fatalf("answers = %+v", answers)
		}
	})

	t.Run("the probabilities of the endpoint travel with the answer, apart from its confidence [verifies SP-404~2]", func(t *testing.T) {
		document, err := set.AnswersFromSystemOne([]byte(response))
		if err != nil {
			t.Fatal(err)
		}
		answers, err := hostproto.ParseClosedAnswers(document)
		if err != nil {
			t.Fatal(err)
		}
		if probability, given := answers["pick"].ProbabilityOf("A"); !given || probability != 0.1 {
			t.Errorf("P(A) = %v, %v", probability, given)
		}
		if probability, given := answers["pick"].ProbabilityOf("B"); !given || probability != 0.8 {
			t.Errorf("P(B) = %v, %v", probability, given)
		}
		if probability, given := answers["claims"].ProbabilityOf(hostproto.NoulTrue); !given || probability != 0.2 {
			t.Errorf("P(true) = %v, %v", probability, given)
		}
		if _, given := answers["coverage"].ProbabilityOf("HIGH"); given {
			t.Error("a score answer carries option probabilities")
		}
	})

	t.Run("a probability that is not on an option or lies outside [0,1] is dropped, and an endpoint that gave none gives none [verifies SP-404~2]", func(t *testing.T) {
		document, err := set.AnswersFromSystemOne([]byte(`{"answers":{"pick":{"type":"choice","choice":"A","probabilities":{"A":1.5,"B":0.3,"Z":0.7}},"claims":{"type":"noul","noul":0.5},"coverage":{"type":"score","score":1}}}`))
		if err != nil {
			t.Fatal(err)
		}
		answers, _ := hostproto.ParseClosedAnswers(document)
		if len(answers["pick"].Probabilities) != 1 {
			t.Fatalf("probabilities = %v", answers["pick"].Probabilities)
		}
		if _, given := answers["pick"].ProbabilityOf("B"); !given {
			t.Fatal("a probability on an option was dropped")
		}
		bare, _ := set.AnswersFromSystemOne([]byte(`{"answers":{"pick":{"type":"choice","choice":"A"},"claims":{"type":"noul","noul":0.5},"coverage":{"type":"score","score":1}}}`))
		answers, _ = hostproto.ParseClosedAnswers(bare)
		if answers["pick"].Probabilities != nil {
			t.Fatalf("probabilities = %v", answers["pick"].Probabilities)
		}
	})

	t.Run("the response schema allows probabilities on the options of each question and nowhere else [verifies SP-403~2]", func(t *testing.T) {
		schema, err := set.ResponseSchema()
		if err != nil {
			t.Fatal(err)
		}
		var document struct {
			Properties struct {
				Answers struct {
					Items struct {
						OneOf []struct {
							Properties struct {
								Probabilities struct {
									AdditionalProperties bool                `json:"additionalProperties"`
									Properties           map[string]struct{} `json:"properties"`
								} `json:"probabilities"`
							} `json:"properties"`
						} `json:"oneOf"`
					} `json:"items"`
				} `json:"answers"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(schema, &document); err != nil {
			t.Fatal(err)
		}
		forms := document.Properties.Answers.Items.OneOf
		wantOptions := [][]string{{"A", "B", "NONE"}, {"false", "true"}, {"HIGH", "LOW", "MID"}}
		for index, form := range forms {
			probabilities := form.Properties.Probabilities
			names := make([]string, 0, len(probabilities.Properties))
			for name := range probabilities.Properties {
				names = append(names, name)
			}
			slices.Sort(names)
			if probabilities.AdditionalProperties || !slices.Equal(names, wantOptions[index]) {
				t.Errorf("form %d allows probabilities %+v", index, probabilities)
			}
		}
	})

	t.Run("a response that leaves a question unanswered or answers it with another type is refused [verifies SP-404~2]", func(t *testing.T) {
		for name, document := range map[string]string{
			"missing":     `{"answers":{"pick":{"type":"choice","choice":"A"}}}`,
			"wrong type":  `{"answers":{"pick":{"type":"noul","noul":0.9},"claims":{"type":"noul","noul":0.1},"coverage":{"type":"score","score":1}}}`,
			"not option":  `{"answers":{"pick":{"type":"choice","choice":"Z"},"claims":{"type":"noul","noul":0.1},"coverage":{"type":"score","score":1}}}`,
			"not a doc":   `[`,
			"empty field": `{"answers":{"pick":{},"claims":{},"coverage":{}}}`,
		} {
			if _, err := set.AnswersFromSystemOne([]byte(document)); !errors.Is(err, hostproto.ErrSystemOneResponse) {
				t.Errorf("%s: error %v", name, err)
			}
		}
	})

	t.Run("each question type has its confidence mapping [verifies SP-404~2]", func(t *testing.T) {
		want := map[hostproto.ClosedType]string{
			hostproto.ClosedChoice: format.MappingSystemOneChoice, hostproto.ClosedNoul: format.MappingSystemOneNoul, hostproto.ClosedScore: format.MappingSystemOneScore,
		}
		for kind, mapping := range want {
			if kind.ConfidenceMapping() != mapping {
				t.Errorf("%s maps to %q", kind, kind.ConfidenceMapping())
			}
		}
		if got := hostproto.ClosedTypes(); len(got) != len(want) {
			t.Errorf("types = %v", got)
		}
	})
}

func TestAnswersForAView(t *testing.T) {
	t.Run("the response of an endpoint is translated with the question set the view shows [verifies SP-404~2]", func(t *testing.T) {
		request := hostprototest.ClosedRequest()
		view := request.ViewFor(hostprototest.Ticket(request, 0, 1))
		document, err := hostproto.AnswersFromSystemOneFor(view, []byte(hostprototest.ClosedSystemOneResponse))
		if err != nil {
			t.Fatal(err)
		}
		answers, err := hostproto.ParseClosedAnswers(document)
		if err != nil || answers[hostprototest.ClosedQuestionID].Choice != "VIOLATES" {
			t.Fatalf("answers %+v (%v)", answers, err)
		}
	})

	t.Run("a view without a question set has no answers to give [verifies SP-404~2]", func(t *testing.T) {
		open := hostprototest.Request()
		view := open.ViewFor(hostprototest.Ticket(open, 0, 1))
		if _, err := hostproto.AnswersFromSystemOneFor(view, []byte(hostprototest.ClosedSystemOneResponse)); !errors.Is(err, hostproto.ErrNoClosedQuestions) {
			t.Fatalf("error %v", err)
		}
	})
}

func TestClosedExport(t *testing.T) {
	t.Run("the first vote of each closed request is exported with its ticket, nonce and set [verifies SP-405~1]", func(t *testing.T) {
		closed := hostprototest.ClosedRequest()
		open := hostprototest.Request()
		first := hostprototest.Ticket(closed, 0, 1)
		second := hostprototest.Ticket(closed, 1, 1)
		view := func(r hostproto.Request, ticket hostproto.Ticket) hostproto.ListingItem {
			return hostproto.ListingItem{Ticket: ticket, Role: r.Role, View: r.ViewFor(ticket)}
		}
		listing := hostproto.Listing{Items: []hostproto.ListingItem{
			view(open, hostprototest.Ticket(open, 0, 1)), view(closed, first), view(closed, second),
		}}
		exported := hostproto.ExportClosed(listing)
		if len(exported) != 1 || exported[0].Ticket != first.TicketID.String() || exported[0].Nonce != first.Nonce {
			t.Fatalf("exported = %+v", exported)
		}
		if _, err := hostproto.ParseClosedSet(exported[0].Closed); err != nil {
			t.Fatalf("the exported set: %v", err)
		}
		if empty := hostproto.ExportClosed(hostproto.Listing{}); empty == nil || len(empty) != 0 {
			t.Fatalf("an empty listing exports %v, want an empty list", empty)
		}
	})
}
