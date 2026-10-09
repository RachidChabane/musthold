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
	// Classify holds the classification a host-classify response was read from:
	// every call, the seed input and the version of the host. It is set exactly
	// when the channel is host-classify.
	Classify *format.ClassifyRecord `json:"classify,omitempty"`
}

// Key returns the key of the response.
func (r Response) Key() Key { return Key{RequestID: r.RequestID, Vote: r.Vote} }

// isJSONObject reports whether the document is a JSON object.
func isJSONObject(document json.RawMessage) bool {
	var members map[string]json.RawMessage
	return json.Unmarshal(document, &members) == nil && members != nil
}

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
	if !isJSONObject(r.Output) {
		sink.add("output", "is not a JSON object")
	}
	if len(r.Output) > MaxOutputBytes {
		sink.add("output", "%d bytes exceed %d", len(r.Output), MaxOutputBytes)
	}
	if err := r.Executor.Validate(); err != nil {
		sink.add("executor", "%v", err)
	}
	if classified := r.Executor.Channel == format.ChannelHostClassify; classified != (r.Classify != nil) {
		sink.add("classify", "set exactly when the channel is %s", format.ChannelHostClassify)
	}
	return sink.err()
}
