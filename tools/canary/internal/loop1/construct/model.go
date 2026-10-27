// Package construct builds the tasks of the loop-1 study (PREREG §3.3). For each
// domain it asks a model to write a small Python project with the material that
// goes with it, asks a second, separate call to write the tests of the project's
// spec from the spec and the interface alone, and runs the acceptance checks. A
// failing check returns its findings to the role they concern for at most two
// repair calls; a domain that still fails is dropped and logged.
package construct

import (
	"context"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
)

// File is one file a model writes: its path and its whole text.
type File struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Schema is the JSON Schema a model's final message must match.
type Schema struct {
	Name string
	JSON []byte
}

// Request is one call to the model.
type Request struct {
	Role    records.Role
	Purpose records.Purpose
	Domain  string
	// Attempt counts the calls of this role for this domain from 1.
	Attempt int
	Prompt  string
	Schema  Schema
}

// Reply is what a call returned. Every member that the call produced is set even
// when Complete returns an error, so that the call can be recorded.
type Reply struct {
	Files        []File
	Output       []byte
	Stderr       []byte
	CodexVersion string
	Start        time.Time
	End          time.Time
	Exit         int
}

// Model answers a prompt with a set of files.
type Model interface {
	Complete(ctx context.Context, request Request) (Reply, error)
}
