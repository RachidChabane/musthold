// Package failure classifies why a relayed request stays unanswered, so that the
// report names the cause and the drain knows when the endpoint itself is failing.
package failure

import (
	"errors"
	"fmt"
)

// Kind says why one request stays unanswered.
type Kind string

// The kinds of failure of a relayed request.
const (
	// KindTimeout: the endpoint did not answer within the time limit.
	KindTimeout Kind = "timeout"
	// KindTransport: no connection to the endpoint could be made or kept.
	KindTransport Kind = "transport"
	// KindRejected: the endpoint refused the request with a 4xx status, which
	// includes a state larger than the model accepts.
	KindRejected Kind = "rejected"
	// KindServer: the endpoint failed with a 5xx status.
	KindServer Kind = "server"
	// KindMalformed: the reply is not JSON, or does not answer every question with
	// one of its options.
	KindMalformed Kind = "malformed"
	// KindUnsupported: the request holds a question no endpoint can be asked.
	KindUnsupported Kind = "unsupported"
	// KindImport: the module refused the answers.
	KindImport Kind = "import"
	// KindSkipped: the request was not posted because the drain stopped early.
	KindSkipped Kind = "skipped"
)

// EndpointWide reports whether the kind says the endpoint itself is unhealthy,
// rather than that one request was unusable.
func (k Kind) EndpointWide() bool {
	return k == KindTimeout || k == KindTransport || k == KindServer
}

// Failure is an error that carries its kind.
type Failure struct {
	Kind   Kind
	Detail string
	Err    error
}

// New builds a Failure without an underlying error.
func New(kind Kind, format string, args ...any) *Failure {
	return &Failure{Kind: kind, Detail: fmt.Sprintf(format, args...)}
}

// Wrap builds a Failure around an underlying error.
func Wrap(kind Kind, err error, format string, args ...any) *Failure {
	return &Failure{Kind: kind, Detail: fmt.Sprintf(format, args...), Err: err}
}

func (f *Failure) Error() string {
	if f.Err == nil {
		return string(f.Kind) + ": " + f.Detail
	}
	return string(f.Kind) + ": " + f.Detail + ": " + f.Err.Error()
}

// Unwrap exposes the underlying error.
func (f *Failure) Unwrap() error { return f.Err }

// KindOf is the kind of an error: its own when it is a Failure, transport
// otherwise.
func KindOf(err error) Kind {
	var failure *Failure
	if errors.As(err, &failure) {
		return failure.Kind
	}
	return KindTransport
}

// DetailOf is the text that explains an error in a report.
func DetailOf(err error) string {
	var failure *Failure
	if errors.As(err, &failure) {
		if failure.Err == nil {
			return failure.Detail
		}
		return failure.Detail + ": " + failure.Err.Error()
	}
	return err.Error()
}
