package prove

import (
	"encoding/json"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/domain/power"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto/hostbridge"
)

func plain(answers []power.ScreenAnswer) []power.ClosedAnswer {
	closed := make([]power.ClosedAnswer, len(answers))
	for index, answer := range answers {
		closed[index] = answer.ClosedAnswer
	}
	return closed
}

func TestScreenAnswersProvenance(t *testing.T) {
	const question = "q1"
	vote := func(channel format.Channel, isolation format.Isolation, marked bool, choice string) hostbridge.VoteState {
		return hostbridge.VoteState{
			Output:   json.RawMessage(`{"answers":[{"id":"` + question + `","choice":"` + choice + `"}]}`),
			Executor: format.ExecutorProfile{Channel: channel, Isolation: isolation},
			Marked:   marked,
		}
	}

	t.Run("a host answer carries the isolation of its executor and its mark [verifies SP-252~1]", func(t *testing.T) {
		answer := hostbridge.Answer{Votes: []hostbridge.VoteState{
			vote(format.ChannelHostInline, format.IsolationInline, false, "CONSISTENT"),
			vote(format.ChannelHostSubagent, format.IsolationRestricted, true, "CONSISTENT"),
		}}
		answers, _ := closedAnswers(answer, question, format.UniformThreshold(0.2))
		if len(answers) != 2 || answers[0].Isolation != format.IsolationInline || answers[0].Marked || answers[1].Isolation != format.IsolationRestricted || !answers[1].Marked {
			t.Fatalf("answers %+v", answers)
		}
	})

	t.Run("a relayed answer carries the isolation the relay profile records [verifies SP-252~1]", func(t *testing.T) {
		answer := hostbridge.Answer{Votes: []hostbridge.VoteState{vote(format.ChannelSystemOneRelay, format.IsolationRestricted, false, "CONSISTENT")}}
		answers, _ := closedAnswers(answer, question, format.UniformThreshold(0.2))
		if len(answers) != 1 || !answers[0].AtFloor(format.IsolationRestricted) || answers[0].AtFloor(format.IsolationRestrictedAttested) {
			t.Fatalf("answers %+v", answers)
		}
	})
}
