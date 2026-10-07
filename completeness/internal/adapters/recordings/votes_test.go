package recordings

import (
	"errors"
	"testing"

	"github.com/RachidChabane/spec-proof/completeness/internal/app/ports"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

func TestRecordingNamesOfVotes(t *testing.T) {
	id := format.SHA256Hex([]byte("request"))

	t.Run("each vote of a request has a recording of its own [verifies SP-540~1]", func(t *testing.T) {
		directory := Directory{Dir: t.TempDir()}
		if ports.RecordingName(id, 0) != id || ports.RecordingName(id, 2) != id+".2" {
			t.Fatalf("names %q and %q", ports.RecordingName(id, 0), ports.RecordingName(id, 2))
		}
		for vote, output := range []string{`{"v":0}`, `{"v":1}`, `{"v":2}`} {
			if err := directory.Record(ports.RecordingName(id, vote), []byte(output), hostprototest.Profile()); err != nil {
				t.Fatal(err)
			}
		}
		for vote, want := range []string{`{"v":0}`, `{"v":1}`, `{"v":2}`} {
			recording, err := directory.Load(ports.RecordingName(id, vote))
			if err != nil || string(recording.Output) != want || recording.Executor == nil {
				t.Errorf("vote %d: %s, %v", vote, recording.Output, err)
			}
		}
	})

	t.Run("a name that is not a request id and a vote number is refused [verifies SP-540~1]", func(t *testing.T) {
		directory := Directory{Dir: t.TempDir()}
		for _, name := range []string{id + ".", id + ".x", id + ".-1", id + ".1.2", "../" + id, id + "/1"} {
			if _, err := directory.Load(name); !errors.Is(err, ErrInvalidRequestID) {
				t.Errorf("%q: %v", name, err)
			}
		}
	})
}
