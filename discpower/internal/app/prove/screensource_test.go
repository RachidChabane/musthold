package prove

import (
	"encoding/json"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostbridge"
)

func TestScreenAnswersSource(t *testing.T) {
	const question = "q1"
	endpoint := "nimble:9b@" + format.SHA256Hex([]byte("endpoint"))
	vote := func(channel format.Channel, model, choice string) hostbridge.VoteState {
		return hostbridge.VoteState{
			Output:   json.RawMessage(`{"answers":[{"id":"` + question + `","choice":"` + choice + `","probabilities":{"VIOLATES":0.55,"CONSISTENT":0.45}}]}`),
			Executor: format.ExecutorProfile{Channel: channel, Model: format.ExecutorModel{Name: model}},
		}
	}

	t.Run("host votes are held to the threshold of host votes by their count [verifies SP-902~1]", func(t *testing.T) {
		answer := hostbridge.Answer{Votes: []hostbridge.VoteState{
			vote(format.ChannelHostInline, "m", "VIOLATES"), vote(format.ChannelHostInline, "m", "CONSISTENT"), vote(format.ChannelHostInline, "m", "CONSISTENT"),
		}}
		answers, source := closedAnswers(answer, question)
		if len(answers) != 3 || source != format.HostVotesSource(3) {
			t.Fatalf("answers %d, source %q", len(answers), source)
		}
	})

	t.Run("a relayed answer replaces the votes and is held to the threshold of its model [verifies SP-902~1]", func(t *testing.T) {
		answer := hostbridge.Answer{Votes: []hostbridge.VoteState{
			vote(format.ChannelSystemOneRelay, endpoint, "VIOLATES"), vote(format.ChannelHostInline, "m", "CONSISTENT"),
		}}
		answers, source := closedAnswers(answer, question)
		if len(answers) != 1 || source != format.SystemOneSource(endpoint) {
			t.Fatalf("answers %d, source %q", len(answers), source)
		}
		if probability := power.ViolationProbability(answers); probability != 0.55 {
			t.Fatalf("P(VIOLATES) = %v", probability)
		}
	})
}
