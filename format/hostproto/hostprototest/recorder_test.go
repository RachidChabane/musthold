package hostprototest_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
)

const (
	testPersona = "honest"
	firstChoice = "COVERED"
	otherChoice = "MISSING"
)

func closedView(t *testing.T, questions ...hostproto.ClosedQuestion) hostproto.ExecutorView {
	t.Helper()
	request := hostprototest.Request()
	view := request.ViewFor(hostprototest.Ticket(request, 0, 1))
	closed, err := json.Marshal(hostproto.ClosedSet{State: "state", Questions: questions})
	if err != nil {
		t.Fatal(err)
	}
	view.Closed = closed
	return view
}

func choiceQuestion(id string, names ...string) hostproto.ClosedQuestion {
	options := make([]hostproto.ClosedOption, len(names))
	for index, name := range names {
		options[index] = hostproto.ClosedOption{Name: name, Description: "the " + name + " option"}
	}
	return hostproto.ClosedQuestion{ID: id, Type: hostproto.ClosedChoice, Instructions: "pick one", Options: options}
}

type fixedSource string

func (f fixedSource) Answer(string, hostproto.ExecutorView) (string, error) { return string(f), nil }

func TestFirstOption(t *testing.T) {
	t.Run("every question is answered with its first option at the stated confidence", func(t *testing.T) {
		view := closedView(t, choiceQuestion("boundaries", firstChoice, otherChoice), choiceQuestion("permissions", otherChoice, firstChoice))
		got, err := hostprototest.FirstOption{}.Answer(testPersona, view)
		if err != nil {
			t.Fatal(err)
		}
		want := `{"answers": [{"choice": "COVERED", "confidence": 0.95, "id": "boundaries"}, {"choice": "MISSING", "confidence": 0.95, "id": "permissions"}]}`
		if got != want {
			t.Fatalf("answer %s, want %s", got, want)
		}
	})

	t.Run("the answer is a document the closed members accept", func(t *testing.T) {
		view := closedView(t, choiceQuestion("boundaries", firstChoice, otherChoice))
		got, err := hostprototest.FirstOption{}.Answer(testPersona, view)
		if err != nil || !json.Valid([]byte(got)) {
			t.Fatalf("answer %q, error %v", got, err)
		}
	})

	t.Run("a request without closed questions is refused", func(t *testing.T) {
		request := hostprototest.Request()
		view := request.ViewFor(hostprototest.Ticket(request, 0, 1))
		view.Closed = nil
		if _, err := (hostprototest.FirstOption{}).Answer(testPersona, view); !errors.Is(err, hostprototest.ErrNoClosedQuestions) {
			t.Fatalf("error %v", err)
		}
	})
}

func TestRecorder(t *testing.T) {
	request := hostprototest.Request()
	view := request.ViewFor(hostprototest.Ticket(request, 0, 1))

	t.Run("an answer is kept under the request of its ticket, trimmed", func(t *testing.T) {
		recorder := hostprototest.NewRecorder(fixedSource(" {\"a\":1}\n"))
		if _, err := recorder.Answer(testPersona, view); err != nil {
			t.Fatal(err)
		}
		if got := recorder.Answers[hostprototest.MustID(request)]; got != `{"a":1}` {
			t.Fatalf("recorded %q", got)
		}
	})

	t.Run("the same answer twice is kept once", func(t *testing.T) {
		recorder := hostprototest.NewRecorder(fixedSource("x"))
		for range 2 {
			if _, err := recorder.Answer(testPersona, view); err != nil {
				t.Fatal(err)
			}
		}
		if len(recorder.Answers) != 1 {
			t.Fatalf("recorded %d answers", len(recorder.Answers))
		}
	})

	t.Run("two different answers to one request are refused", func(t *testing.T) {
		recorder := hostprototest.NewRecorder(fixedSource("x"))
		if _, err := recorder.Answer(testPersona, view); err != nil {
			t.Fatal(err)
		}
		recorder.Source = fixedSource("y")
		if _, err := recorder.Answer(testPersona, view); !errors.Is(err, hostprototest.ErrConflictingRecording) {
			t.Fatalf("error %v", err)
		}
	})
}
