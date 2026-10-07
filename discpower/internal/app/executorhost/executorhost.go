// Package executorhost is the server side of the executor tools: it answers
// `fetch_request`, `submit_response` and `drain` over the request service, so
// that the binding, the response schema and the lifecycle decide what is
// accepted. It speaks no wire protocol; the MCP server does.
package executorhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/requests"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/mcpstdio"
)

// Names the module answers to in `drain`.
const (
	// ModuleAlias is the short module name the `drain` tool is given.
	ModuleAlias = "dp"
	// ModuleName is the module's binary name, which `drain` accepts as well.
	ModuleName = "spec-proof-dp"

	violationMarker = "\n- "
)

// Errors the server returns to the executor.
var (
	// ErrUnknownModule is returned when `drain` names another module.
	ErrUnknownModule = errors.New("this server serves the module dp only")
	// ErrNoJSONObject is returned when a sampled answer holds no JSON object.
	ErrNoJSONObject = errors.New("the sampled answer holds no JSON object")
)

var _ mcpstdio.ExecutorService = Service{}

// Service implements mcpstdio.ExecutorService.
type Service struct {
	Requests requests.Service
}

// FetchRequest implements mcpstdio.ExecutorService: the ticket moves to fetched
// when the capability is valid for its role, and the view is what the executor
// sees of the request.
func (s Service) FetchRequest(_ context.Context, args mcpstdio.FetchArgs) (mcpstdio.FetchResult, error) {
	ticket, err := hostproto.ParseTicketID(args.Ticket)
	if err != nil {
		return mcpstdio.FetchResult{}, err
	}
	view, err := s.Requests.Show(ticket, args.Cap)
	if err != nil {
		return mcpstdio.FetchResult{}, err
	}
	return mcpstdio.FetchResult{View: view}, nil
}

// SubmitResponse implements mcpstdio.ExecutorService. A schema violation is a
// result, not a failure: the executor gets the violations and may correct once.
func (s Service) SubmitResponse(_ context.Context, args mcpstdio.SubmitArgs) (mcpstdio.SubmitResult, error) {
	ticket, err := hostproto.ParseTicketID(args.Ticket)
	if err != nil {
		return mcpstdio.SubmitResult{}, err
	}
	receipt, err := s.Requests.Submit(requests.Submission{
		Ticket: ticket, Nonce: args.Nonce, Cap: args.Cap, Output: args.Output, Origin: requests.OriginExecutor,
	})
	if errors.Is(err, ports.ErrSchemaViolation) {
		return mcpstdio.SubmitResult{SchemaErrors: violations(err)}, nil
	}
	if err != nil {
		return mcpstdio.SubmitResult{}, err
	}
	return mcpstdio.SubmitResult{Receipt: receipt.Text}, nil
}

// violations lists the violated keywords a schema error carries.
func violations(err error) []string {
	_, listed, found := strings.Cut(err.Error(), violationMarker)
	if !found {
		return []string{err.Error()}
	}
	return strings.Split(listed, violationMarker)
}

// Drain implements mcpstdio.ExecutorService: every pending ticket is put to the
// client's model through sampling, with no context and no tools, and the answer
// is submitted as a sampled response. The first failure stops the drain.
func (s Service) Drain(ctx context.Context, args mcpstdio.DrainArgs, sampler mcpstdio.Sampler) (mcpstdio.DrainResult, error) {
	if args.Module != ModuleAlias && args.Module != ModuleName {
		return mcpstdio.DrainResult{}, ErrUnknownModule
	}
	sampling := s.Requests.ForSampling()
	listing, err := sampling.List()
	if err != nil {
		return mcpstdio.DrainResult{}, err
	}
	served := 0
	for _, item := range listing.Items {
		if err := s.serve(ctx, sampling, sampler, item); err != nil {
			return mcpstdio.DrainResult{Served: served}, fmt.Errorf("serve %s: %w", item.Ticket.TicketID, err)
		}
		served++
	}
	return mcpstdio.DrainResult{Served: served}, nil
}

func (s Service) serve(ctx context.Context, sampling requests.Service, sampler mcpstdio.Sampler, item hostproto.ListingItem) error {
	view, err := sampling.Show(item.Ticket.TicketID, "")
	if err != nil {
		return err
	}
	text, err := sampler.Sample(ctx, mcpstdio.SamplingRequest{System: view.System, User: view.User})
	if err != nil {
		return err
	}
	output, err := jsonObjectIn(text)
	if err != nil {
		return err
	}
	_, err = sampling.Submit(requests.Submission{Ticket: item.Ticket.TicketID, Nonce: view.Nonce, Output: output, Origin: requests.OriginSampled})
	return err
}

// jsonObjectIn extracts the JSON object a sampled answer holds, which a model may
// wrap in prose or a code fence.
func jsonObjectIn(text string) ([]byte, error) {
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return nil, ErrNoJSONObject
	}
	candidate := []byte(text[start : end+1])
	if !json.Valid(candidate) {
		return nil, ErrNoJSONObject
	}
	return candidate, nil
}
