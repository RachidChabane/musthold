// Package drain is the use case of the relay: take the closed requests a proof
// module exported, ask the System One endpoint about each one, one at a time, and
// import the answers it can use. A request it cannot answer stays with the module,
// for the host votes of the next step of the chain.
package drain

import (
	"context"
	"errors"
	"fmt"

	"github.com/RachidChabane/musthold/s1relay/internal/app/ports"
	"github.com/RachidChabane/musthold/s1relay/internal/domain/endpoint"
	"github.com/RachidChabane/musthold/s1relay/internal/domain/failure"
)

// ConsecutiveFailureLimit is how many requests in a row may fail because of the
// endpoint itself (timeout, no connection, 5xx) before the drain gives up on it.
const ConsecutiveFailureLimit = 3

// Unlimited is the value of Options.MaxRequests that drains every request.
const Unlimited = 0

// ErrMemoryRefused is returned when the memory rule refuses the batch.
var ErrMemoryRefused = errors.New("the memory rule refused the batch")

// Status is the outcome of one request.
type Status string

// The outcomes of a request.
const (
	StatusAnswered   Status = "answered"
	StatusUnanswered Status = "unanswered"
)

// Options tunes one drain.
type Options struct {
	// MaxRequests bounds how many requests are posted; Unlimited drains them all.
	MaxRequests int
	// MemGuard asks the memory rule before the batch and before each request.
	MemGuard bool
	// Unload frees the model of a local server after the batch.
	Unload bool
}

// Item is the outcome of one request.
type Item struct {
	Ticket string
	Status Status
	// Source is the name the answers were imported under, for an answered request.
	Source string
	// Kind and Detail say why a request stayed unanswered.
	Kind   failure.Kind
	Detail string
}

// UnloadReport says what the unload after the batch did.
type UnloadReport struct {
	Attempted bool
	Done      bool
	Detail    string
}

// Report is what one drain did.
type Report struct {
	Items []Item
	// Remaining counts the exported requests the batch limit left untouched.
	Remaining int
	Unload    UnloadReport
}

// Answered counts the requests that were answered and imported.
func (r Report) Answered() int { return r.count(StatusAnswered) }

// Unanswered counts the requests left to the host votes.
func (r Report) Unanswered() int { return r.count(StatusUnanswered) }

func (r Report) count(status Status) int {
	total := 0
	for _, item := range r.Items {
		if item.Status == status {
			total++
		}
	}
	return total
}

// Service drains the closed requests of one module into one endpoint.
type Service struct {
	Module    ports.ModuleGateway
	Endpoint  endpoint.Endpoint
	SystemOne ports.SystemOne
	Guard     ports.MemoryGuard
	Unloader  ports.Unloader
	Log       ports.Logger
}

// run holds the state of one drain.
type run struct {
	Service
	options     Options
	report      Report
	guardActive bool
	consecutive int
	lastModel   string
	posted      int
}

// Run drains the module. It returns an error only when nothing could be tried: the
// memory rule refused, or the module could not export.
func (s Service) Run(ctx context.Context, options Options) (Report, error) {
	r := &run{Service: s, options: options}
	if err := r.checkMemoryBeforeBatch(ctx); err != nil {
		return Report{}, err
	}
	exported, err := s.Module.Export(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("export the closed requests: %w", err)
	}
	for index, request := range exported {
		if options.MaxRequests != Unlimited && r.posted >= options.MaxRequests {
			r.report.Remaining = len(exported) - index
			break
		}
		r.handle(ctx, request)
	}
	r.unloadAfterBatch(ctx)
	return r.report, nil
}

func (r *run) checkMemoryBeforeBatch(ctx context.Context) error {
	if !r.options.MemGuard {
		return nil
	}
	verdict, err := r.Guard.Check(ctx)
	switch {
	case err != nil:
		return fmt.Errorf("%w: %v", ErrMemoryRefused, err)
	case verdict.Skipped:
		r.Log.Logf("memory rule skipped: %s", verdict.Reason)
	case !verdict.Allowed:
		return fmt.Errorf("%w: %s", ErrMemoryRefused, verdict.Reason)
	default:
		r.guardActive = true
	}
	return nil
}

func (r *run) handle(ctx context.Context, request ports.Exported) {
	if reason := r.reasonToSkip(ctx); reason != "" {
		r.record(Item{Ticket: request.Ticket, Status: StatusUnanswered, Kind: failure.KindSkipped, Detail: reason})
		return
	}
	if request.Problem != nil {
		r.record(Item{Ticket: request.Ticket, Status: StatusUnanswered, Kind: failure.KindUnsupported, Detail: request.Problem.Error()})
		return
	}
	r.posted++
	decision, err := r.SystemOne.Decide(ctx, request.Set)
	if err == nil {
		err = r.checkAnswers(request, decision)
	}
	if err != nil {
		r.fail(request.Ticket, err)
		return
	}
	r.consecutive = 0
	r.lastModel = decision.Model
	source := r.Endpoint.Source(decision.Model)
	if err := r.Module.Import(ctx, request.Ticket, request.Nonce, source, decision.Reply); err != nil {
		r.record(Item{Ticket: request.Ticket, Status: StatusUnanswered, Kind: failure.KindImport, Detail: failure.DetailOf(err)})
		return
	}
	r.record(Item{Ticket: request.Ticket, Status: StatusAnswered, Source: source})
}

func (r *run) checkAnswers(request ports.Exported, decision ports.Decision) error {
	if err := request.Set.Check(decision.Answers); err != nil {
		return failure.Wrap(failure.KindMalformed, err, "the endpoint's answers cannot be used")
	}
	return nil
}

// reasonToSkip says why the next request must not be posted, or "" when it may.
func (r *run) reasonToSkip(ctx context.Context) string {
	if r.consecutive >= ConsecutiveFailureLimit {
		return fmt.Sprintf("the endpoint failed %d requests in a row", r.consecutive)
	}
	if !r.guardActive {
		return ""
	}
	verdict, err := r.Guard.Check(ctx)
	switch {
	case err != nil:
		return fmt.Sprintf("memory rule: %v", err)
	case !verdict.Allowed:
		return "memory rule: " + verdict.Reason
	}
	return ""
}

func (r *run) fail(ticket string, err error) {
	kind := failure.KindOf(err)
	if kind.EndpointWide() {
		r.consecutive++
	} else {
		r.consecutive = 0
	}
	r.record(Item{Ticket: ticket, Status: StatusUnanswered, Kind: kind, Detail: failure.DetailOf(err)})
}

func (r *run) record(item Item) {
	r.report.Items = append(r.report.Items, item)
	if item.Status == StatusAnswered {
		r.Log.Logf("answered %s as %s", item.Ticket, item.Source)
		return
	}
	r.Log.Logf("left %s to the host votes (%s): %s", item.Ticket, item.Kind, item.Detail)
}

func (r *run) unloadAfterBatch(ctx context.Context) {
	if !r.options.Unload || r.posted == 0 {
		return
	}
	r.report.Unload.Attempted = true
	result, err := r.Unloader.Unload(ctx, r.lastModel)
	switch {
	case err != nil:
		r.report.Unload.Detail = err.Error()
		r.Log.Logf("unload failed: %v", err)
	case !result.Applicable:
		r.report.Unload.Detail = result.Detail
	default:
		r.report.Unload.Done = true
	}
}
