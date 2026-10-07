package requests

import (
	"testing"

	"github.com/RachidChabane/spec-proof/completeness/internal/app/ports"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

func TestRecordingOfAVote(t *testing.T) {
	id := hostproto.RequestID(format.SHA256Hex([]byte("request")))
	first, second := []byte(`{"validity":"VALID"}`), []byte(`{"validity":"EQUIVALENT"}`)

	t.Run("a vote is answered from its own recording before the first vote's [verifies SP-540~1]", func(t *testing.T) {
		recordings := fakeRecordings{
			ports.RecordingName(string(id), 0): {Output: first},
			ports.RecordingName(string(id), 1): {Output: second},
		}
		for vote, want := range map[int][]byte{0: first, 1: second} {
			got, err := (Replayer{Recordings: recordings}).recordingOf(hostproto.TicketID{RequestID: id, Vote: vote, Emission: hostproto.FirstEmission})
			if err != nil || string(got.Output) != string(want) {
				t.Errorf("vote %d: %s, %v", vote, got.Output, err)
			}
		}
	})

	t.Run("a vote without a recording of its own falls back on the first vote's [verifies SP-540~1]", func(t *testing.T) {
		recordings := fakeRecordings{string(id): {Output: first}}
		got, err := (Replayer{Recordings: recordings}).recordingOf(hostproto.TicketID{RequestID: id, Vote: 2, Emission: hostproto.FirstEmission})
		if err != nil || string(got.Output) != string(first) {
			t.Fatalf("%s, %v", got.Output, err)
		}
	})

	t.Run("a vote with no recording at all is a miss [verifies SP-540~1]", func(t *testing.T) {
		if _, err := (Replayer{Recordings: fakeRecordings{}}).recordingOf(hostproto.TicketID{RequestID: id, Vote: 1, Emission: hostproto.FirstEmission}); err == nil {
			t.Fatal("expected a miss")
		}
	})
}
