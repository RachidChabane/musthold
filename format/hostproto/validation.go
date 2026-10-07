package hostproto

import (
	"encoding/json"
	"errors"
	"strings"
)

// ErrSchemaViolation is returned by a ResponseValidator for an output that does
// not satisfy its schema; the message lists the violations, one per line, each
// behind ViolationMarker.
var ErrSchemaViolation = errors.New("output violates the response schema")

// ViolationMarker starts every violation a schema error lists after its first
// line.
const ViolationMarker = "\n- "

// ResponseValidator checks an executor's output against a request's response
// schema. Each module wires the implementation its dependencies allow; the
// protocol only fixes how a failure reads.
type ResponseValidator interface {
	Validate(schema json.RawMessage, output []byte) error
}

// Violations lists the violated keywords a schema error carries, or the error's
// own text when it lists none.
func Violations(err error) []string {
	_, listed, found := strings.Cut(err.Error(), ViolationMarker)
	if !found {
		return []string{err.Error()}
	}
	return strings.Split(listed, ViolationMarker)
}
