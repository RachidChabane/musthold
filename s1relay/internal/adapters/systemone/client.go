// Package systemone is the HTTP adapter to a System One endpoint, the only code of
// the relay that opens a connection. It posts one question set per request and
// maps every way the exchange can go wrong to a failure kind.
package systemone

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/RachidChabane/musthold/s1relay/internal/adapters/systemonewire"
	"github.com/RachidChabane/musthold/s1relay/internal/app/ports"
	"github.com/RachidChabane/musthold/s1relay/internal/domain/endpoint"
	"github.com/RachidChabane/musthold/s1relay/internal/domain/failure"
	"github.com/RachidChabane/musthold/s1relay/internal/domain/question"
)

// Limits and headers of the exchange.
const (
	// DefaultTimeout bounds one request when the caller names no limit.
	DefaultTimeout = 2 * time.Minute
	// MaxReplyBytes bounds the reply the relay reads.
	MaxReplyBytes = 4 << 20
	// maxExcerptBytes bounds the part of an error body quoted in a report.
	maxExcerptBytes = 300

	headerContentType = "Content-Type"
	headerAccept      = "Accept"
	mediaJSON         = "application/json"

	statusClientErrorFloor = 400
	statusServerErrorFloor = 500
)

var _ ports.SystemOne = (*Client)(nil)

// Client asks one endpoint, one request at a time.
type Client struct {
	endpoint endpoint.Endpoint
	model    string
	timeout  time.Duration
	http     *http.Client
}

// NewClient builds the client of an endpoint. The model is sent with each request
// when it is not empty; timeout bounds each request.
func NewClient(address endpoint.Endpoint, model string, timeout time.Duration) *Client {
	return &Client{endpoint: address, model: model, timeout: timeout, http: newHTTPClient()}
}

func newHTTPClient() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// Decide posts the question set and reads the answers.
func (c *Client) Decide(ctx context.Context, set question.Set) (ports.Decision, error) {
	body, err := systemonewire.EncodeRequest(c.model, set)
	if err != nil {
		return ports.Decision{}, failure.Wrap(failure.KindUnsupported, err, "the question set cannot be sent")
	}
	reply, err := c.post(ctx, c.endpoint.URL(), body)
	if err != nil {
		return ports.Decision{}, err
	}
	answers, replyModel, err := systemonewire.DecodeReply(reply, set)
	if err != nil {
		return ports.Decision{}, failure.Wrap(failure.KindMalformed, err, "the reply is unusable")
	}
	return ports.Decision{Model: c.modelOf(replyModel), Answers: answers, Reply: reply}, nil
}

func (c *Client) modelOf(replyModel string) string {
	if c.model != "" {
		return c.model
	}
	return replyModel
}

func (c *Client) post(ctx context.Context, target string, body []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, failure.Wrap(failure.KindTransport, err, "build the request")
	}
	request.Header.Set(headerContentType, mediaJSON)
	request.Header.Set(headerAccept, mediaJSON)
	response, err := c.http.Do(request)
	if err != nil {
		return nil, transportFailure(ctx, err)
	}
	defer response.Body.Close()
	reply, err := io.ReadAll(io.LimitReader(response.Body, MaxReplyBytes))
	if err != nil {
		return nil, transportFailure(ctx, err)
	}
	if response.StatusCode >= statusClientErrorFloor || response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, statusFailure(response.StatusCode, reply)
	}
	return reply, nil
}

func transportFailure(ctx context.Context, err error) error {
	var netErr net.Error
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return failure.Wrap(failure.KindTimeout, err, "no reply in time")
	}
	return failure.Wrap(failure.KindTransport, err, "the exchange failed")
}

func statusFailure(status int, body []byte) error {
	excerpt := excerptOf(body)
	switch {
	case status >= statusServerErrorFloor:
		return failure.New(failure.KindServer, "HTTP %d%s", status, excerpt)
	case status >= statusClientErrorFloor:
		return failure.New(failure.KindRejected, "HTTP %d%s", status, excerpt)
	default:
		return failure.New(failure.KindRejected, "unexpected HTTP %d%s", status, excerpt)
	}
}

// excerptOf quotes the start of an error body, for the report.
func excerptOf(body []byte) string {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return ""
	}
	if len(text) > maxExcerptBytes {
		text = text[:maxExcerptBytes] + "..."
	}
	return fmt.Sprintf(": %s", text)
}
