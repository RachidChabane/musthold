package ports

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// QueueItem is the queue-only metadata of a request: which criteria it serves,
// in which wave it was first emitted and what for. An executor never sees it.
type QueueItem struct {
	RequestID hostproto.RequestID `json:"requestId"`
	Criteria  []string            `json:"criteria"`
	Wave      int                 `json:"wave"`
	Purpose   string              `json:"purpose"`
}

// Queue is the module-owned queue state next to the host store.
type Queue interface {
	// Record adds the item, or adds its criteria to the item already stored for
	// the request.
	Record(item QueueItem) error
	// Items lists every item ordered by request id.
	Items() ([]QueueItem, error)
	// Wave returns the number of the current wave, zero before the first.
	Wave() (int, error)
	// AdvanceWave starts the next wave and returns its number.
	AdvanceWave() (int, error)
}

// HostStore is the host directory of the module: the shared protocol store, the
// discard of a response a reissued ticket replaces, and the queue.
type HostStore interface {
	hostproto.Store
	hostproto.ResponseDiscarder
	Queue
}

// ErrSchemaViolation is returned by a ResponseValidator for an output that does
// not satisfy its schema; the message lists the violations.
var ErrSchemaViolation = errors.New("output violates the response schema")

// ResponseValidator checks an executor's output against a request's response
// schema.
type ResponseValidator interface {
	Validate(schema json.RawMessage, output []byte) error
}

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

// PendingTicket is a ticket waiting for an answer.
type PendingTicket struct {
	Ticket string
	Nonce  string
}

// Replayer serves the pending tickets from recorded answers, the way a script
// would, and fails hard on a ticket it holds no recording for.
type Replayer interface {
	Serve(tickets []PendingTicket) error
}
