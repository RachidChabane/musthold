package construct

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
)

// Caller makes the model calls of one task's construction and records each: it
// answers a call the records of an earlier invocation already hold from them,
// counts the attempts and the repairs of each role, and stops at a call that
// failed on the harness side.
type Caller struct {
	Model   Model
	Records records.Writer
	// Replay holds the calls an earlier invocation recorded for the task; nil
	// when there are none.
	Replay   *Replay
	attempts map[records.Role]int
	repairs  map[records.Role]int
}

// NewCaller builds the caller of one task.
func NewCaller(model Model, writer records.Writer, replay *Replay) *Caller {
	return &Caller{Model: model, Records: writer, Replay: replay, attempts: map[records.Role]int{}, repairs: map[records.Role]int{}}
}

// Attempts is how many calls of a role were made, harness failures excluded.
func (c *Caller) Attempts(role records.Role) int { return c.attempts[role] }

// Repairs is how many of those calls repaired.
func (c *Caller) Repairs(role records.Role) int { return c.repairs[role] }

// Next is the request of the role's next call, numbered after its attempts.
func (c *Caller) Next(role records.Role, purpose records.Purpose, domain, prompt string, schema Schema) Request {
	return Request{Role: role, Purpose: purpose, Domain: domain, Attempt: c.attempts[role] + 1, Prompt: prompt, Schema: schema}
}

// Call makes one model call and records it. A call the model could not answer is
// returned as a failure on the role and counts as an attempt; a call that failed
// on the harness side is recorded, counts as nothing, and is returned as an
// error so that construction stops where it is. Only a failure to record is
// otherwise an error.
func (c *Caller) Call(ctx context.Context, request Request) ([]File, []records.Finding, error) {
	recorded, err := c.Replay.nextCall(request)
	if err != nil {
		return nil, nil, err
	}
	reply, callErr := c.answer(ctx, request, recorded)
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	var harness *HarnessFailure
	if errors.As(callErr, &harness) {
		return nil, nil, c.stopOnHarness(request, reply, callErr, harness)
	}
	c.attempts[request.Role]++
	if request.Purpose == records.PurposeRepair {
		c.repairs[request.Role]++
	}
	if recorded == nil {
		if err := c.Records.Append(records.Entry{Call: callRecord(request, reply, callErr)}); err != nil {
			return nil, nil, err
		}
	}
	if callErr != nil {
		return nil, []records.Finding{{Role: request.Role, Target: records.TargetOutput, Text: "the call failed: " + callErr.Error()}}, nil
	}
	return reply.Files, nil, nil
}

// answer returns the recorded answer of a call that an earlier invocation made,
// or asks the model.
func (c *Caller) answer(ctx context.Context, request Request, recorded *answer) (Reply, error) {
	if recorded != nil {
		return recorded.reply, recorded.callErr
	}
	return c.Model.Complete(ctx, request)
}

// stopOnHarness records a call that failed on the harness side and returns the
// error that stops construction, naming where it stopped.
func (c *Caller) stopOnHarness(request Request, reply Reply, callErr error, harness *HarnessFailure) error {
	entry := callRecord(request, reply, callErr)
	entry.HarnessFailure = string(harness.Kind)
	if err := c.Records.Append(records.Entry{Call: entry}); err != nil {
		return err
	}
	return fmt.Errorf("%s %s attempt %d (%s): %w", request.Domain, request.Role, request.Attempt, request.Purpose, callErr)
}

func callRecord(request Request, reply Reply, callErr error) *records.Call {
	call := &records.Call{
		Role: request.Role, Purpose: request.Purpose, Domain: request.Domain, Attempt: request.Attempt,
		PromptSHA256: Digest(request.Prompt), Schema: string(request.Schema.JSON),
		Output: string(reply.Output), Stderr: string(reply.Stderr), CodexVersion: reply.CodexVersion, CLIVersion: reply.CLIVersion,
		Start: reply.Start.UTC(), End: reply.End.UTC(), Exit: reply.Exit,
	}
	if callErr != nil {
		call.Error = callErr.Error()
	}
	return call
}

// Digest is the SHA-256 of a text, in hexadecimal.
func Digest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
