package prove

import (
	"encoding/json"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/domain/power"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto/hostbridge"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
)

func TestScreenAnswersOfAClassification(t *testing.T) {
	const question = "q1"
	source, err := format.SingleVoterClassifySource(5, format.ClassifySchemeRot1, hostprototest.ClassifySessionModel)
	if err != nil {
		t.Fatal(err)
	}
	record := &format.ClassifyRecord{Source: source.Source()}
	classified := func(isolation format.Isolation, choice string, violates float64) hostbridge.VoteState {
		return hostbridge.VoteState{
			Output: json.RawMessage(`{"answers":[{"id":"q1","choice":"` + choice + `","confidence":0.96,"probabilities":{"VIOLATES":` + jsonNumber(violates) + `}}]}`),
			Executor: format.ExecutorProfile{
				Channel: format.ChannelHostClassify, Isolation: isolation, Model: format.ExecutorModel{Name: hostprototest.ClassifySessionModel, Source: format.ModelSourceDeclared},
			},
			Classify: record,
		}
	}
	exact := format.Threshold{Default: 0.2, BySource: map[format.ConfidenceSource]float64{source.Source(): 0.5}}

	t.Run("a classification replaces the votes, is held to the threshold of its exact source and keeps its stated probability [verifies SP-1916~1]", func(t *testing.T) {
		answer := hostbridge.Answer{Votes: []hostbridge.VoteState{
			classified(format.IsolationRestricted, "VIOLATES", 0.96),
			{Output: json.RawMessage(`{"answers":[{"id":"q1","choice":"CONSISTENT"}]}`), Executor: format.ExecutorProfile{Channel: format.ChannelHostInline}},
		}}
		answers, got := closedAnswers(answer, question, exact)
		if len(answers) != 1 || got != source.Source() || answers[0].NoExactThreshold || !answers[0].HasViolates || answers[0].Violates != 0.96 {
			t.Fatalf("answers %+v, source %q", answers, got)
		}
		if !answers[0].AtFloor(format.IsolationRestricted) || answers[0].AtFloor(format.IsolationRestrictedAttested) {
			t.Fatalf("floor of %+v", answers[0])
		}
	})

	t.Run("a classification without a threshold of its exact source never reaches the floor [verifies SP-1916~1]", func(t *testing.T) {
		answer := hostbridge.Answer{Votes: []hostbridge.VoteState{classified(format.IsolationRestricted, "CONSISTENT", 0.04)}}
		for name, threshold := range map[string]format.Threshold{"the default only": format.UniformThreshold(0.2), "its kind only": {Default: 0.2, BySource: map[format.ConfidenceSource]float64{format.SourceHostClassify: 0.3}}} {
			answers, _ := closedAnswers(answer, question, threshold)
			if len(answers) != 1 || !answers[0].NoExactThreshold || answers[0].AtFloor(format.IsolationRestrictedUnverified) {
				t.Fatalf("%s: answers %+v", name, answers)
			}
			verdict := power.Screen(answers, format.IsolationRestricted, threshold.For(source.Source()))
			if !verdict.Flagged || !verdict.FloorEntry {
				t.Fatalf("%s: a survivor the classification let go was not kept for the deciders: %+v", name, verdict)
			}
		}
	})

	t.Run("an unraised classification that flags a survivor keeps it in the pool without a floor entry [verifies SP-1916~1]", func(t *testing.T) {
		answer := hostbridge.Answer{Votes: []hostbridge.VoteState{classified(format.IsolationRestrictedUnverified, "VIOLATES", 0.96)}}
		answers, _ := closedAnswers(answer, question, exact)
		verdict := power.Screen(answers, format.IsolationRestricted, exact.For(source.Source()))
		if !verdict.Flagged || verdict.FloorEntry || verdict.Violation != 1 {
			t.Fatalf("verdict %+v", verdict)
		}
	})

	t.Run("a classification at the floor with its own threshold may screen a survivor out [verifies SP-1916~1]", func(t *testing.T) {
		answer := hostbridge.Answer{Votes: []hostbridge.VoteState{classified(format.IsolationRestricted, "CONSISTENT", 0.04)}}
		answers, _ := closedAnswers(answer, question, exact)
		verdict := power.Screen(answers, format.IsolationRestricted, exact.For(source.Source()))
		if verdict.Flagged || verdict.FloorEntry {
			t.Fatalf("verdict %+v", verdict)
		}
	})
}

func jsonNumber(value float64) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
