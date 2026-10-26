package requests

import (
	"errors"
	"fmt"

	"github.com/RachidChabane/musthold/format"
)

// Recording is an answer kept from an earlier run: the output and, when it was
// recorded, the executor that gave it.
type Recording struct {
	Output   []byte
	Executor *format.ExecutorProfile
}

// Recordings serves recorded answers by recording name; an absent one is an error.
type Recordings interface {
	Load(name string) (Recording, error)
}

// Recorder keeps the answers a run consumed, each under its recording name.
type Recorder interface {
	Record(name string, output []byte, executor format.ExecutorProfile) error
}

// RecordingName names the recording of one vote of a request: the request id for
// the first vote, the request id and the vote number for the others.
func RecordingName(requestID string, vote int) string {
	if vote == 0 {
		return requestID
	}
	return fmt.Sprintf("%s.%d", requestID, vote)
}

// PendingTicket is a ticket waiting for an answer, with the nonce that binds the
// answer to it.
type PendingTicket struct {
	Ticket string
	Nonce  string
}

// ErrNothingServed is returned by a Replayer that answered none of the pending
// tickets: the run ends with them pending instead of asking again.
var ErrNothingServed = errors.New("the replay answered no pending ticket")

// Replayer serves the pending tickets from recorded answers, the way a script
// would, and fails hard on a ticket it holds no recording for.
type Replayer interface {
	Serve(tickets []PendingTicket) error
}
