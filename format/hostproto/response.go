package hostproto

import (
	"encoding/json"

	"github.com/RachidChabane/spec-proof/format"
)

// Response is what a module records for an accepted submission.
type Response struct {
	RequestID RequestID              `json:"requestId"`
	Vote      int                    `json:"vote"`
	Nonce     string                 `json:"nonce"`
	Output    json.RawMessage        `json:"output"`
	Executor  format.ExecutorProfile `json:"executor"`
}

// Key returns the key of the response.
func (r Response) Key() Key { return Key{RequestID: r.RequestID, Vote: r.Vote} }

// Validate checks the shape of the response and its executor.
func (r Response) Validate() error {
	var sink problemSink
	checkDigestField(&sink, "requestId", string(r.RequestID))
	if r.Vote < 0 {
		sink.add("vote", "%d is negative", r.Vote)
	}
	if !ValidNonce(r.Nonce) {
		sink.add("nonce", "%q is not a nonce", r.Nonce)
	}
	if len(r.Output) == 0 || !json.Valid(r.Output) {
		sink.add("output", "is not a JSON document")
	}
	if len(r.Output) > MaxOutputBytes {
		sink.add("output", "%d bytes exceed %d", len(r.Output), MaxOutputBytes)
	}
	if err := r.Executor.Validate(); err != nil {
		sink.add("executor", "%v", err)
	}
	return sink.err()
}
