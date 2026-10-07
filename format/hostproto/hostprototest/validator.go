package hostprototest

import (
	"encoding/json"
	"fmt"

	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// ValidityMember is the member the fixture request's response schema requires.
const ValidityMember = "validity"

// MemberValidator is a hostproto.ResponseValidator for tests: an output is valid
// when it is a JSON object that holds the member, whatever the schema says. It
// fails the way the modules' schema validators fail, so that the protocol's
// handling of a violation is exercised without a schema library.
type MemberValidator struct {
	// Member is the property the output must hold; it defaults to ValidityMember.
	Member string
}

var _ hostproto.ResponseValidator = MemberValidator{}

// Validate implements hostproto.ResponseValidator.
func (v MemberValidator) Validate(_ json.RawMessage, output []byte) error {
	member := v.Member
	if member == "" {
		member = ValidityMember
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(output, &document); err != nil {
		return fmt.Errorf("%w: the output is not JSON: %v", hostproto.ErrSchemaViolation, err)
	}
	if _, present := document[member]; !present {
		return fmt.Errorf("%w:%smissing property %q", hostproto.ErrSchemaViolation, hostproto.ViolationMarker, member)
	}
	return nil
}
