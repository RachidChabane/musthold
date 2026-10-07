// Package systemonewire is the one place that knows the wire forms of the relay:
// the closed request a proof module exports and the answer it imports, and the
// request and reply of a System One endpoint. Everything else of the relay works
// on the question types of the domain.
package systemonewire

import (
	"encoding/json"
	"fmt"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/s1relay/internal/domain/question"
)

// ModuleRequest is one entry of the output of `systemone export`.
type ModuleRequest struct {
	Ticket string          `json:"ticket"`
	Nonce  string          `json:"nonce"`
	Closed json.RawMessage `json:"closed"`
}

// DecodeExport reads the output of `systemone export`.
func DecodeExport(output []byte) ([]ModuleRequest, error) {
	var requests []ModuleRequest
	if err := json.Unmarshal(output, &requests); err != nil {
		return nil, fmt.Errorf("read the export: %w", err)
	}
	return requests, nil
}

// DecodeClosed reads the question set of a closed request as the modules emit it,
// through the host protocol's closed question type. A noul question is one the
// relay does not carry.
func DecodeClosed(closed json.RawMessage) (question.Set, error) {
	decoded, err := hostproto.ParseClosedSet(closed)
	if err != nil {
		return question.Set{}, fmt.Errorf("read the closed request: %w", err)
	}
	set := question.Set{State: decoded.State}
	for _, q := range decoded.Questions {
		if q.Type == hostproto.ClosedNoul {
			return question.Set{}, fmt.Errorf("%w: %q", question.ErrUnsupportedType, q.Type)
		}
		options := make([]question.Option, len(q.Options))
		for index, option := range q.Options {
			options[index] = question.Option{Name: option.Name, Description: option.Description}
		}
		set.Questions = append(set.Questions, question.Question{ID: q.ID, Type: question.Type(q.Type), Text: q.Instructions, Options: options})
	}
	if err := set.Validate(); err != nil {
		return question.Set{}, err
	}
	return set, nil
}

// ClosedSetOf renders a question set of the relay as the host protocol's.
func ClosedSetOf(set question.Set) hostproto.ClosedSet {
	closed := hostproto.ClosedSet{State: set.State}
	for _, q := range set.Questions {
		options := make([]hostproto.ClosedOption, len(q.Options))
		for index, option := range q.Options {
			options[index] = hostproto.ClosedOption{Name: option.Name, Description: option.Meaning()}
		}
		closed.Questions = append(closed.Questions, hostproto.ClosedQuestion{ID: q.ID, Type: hostproto.ClosedType(q.Type), Instructions: q.Text, Options: options})
	}
	return closed
}
