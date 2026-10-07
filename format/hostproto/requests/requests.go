// Package requests is the command-line side of the host protocol: it lists the
// tickets that wait for an answer, shows one to an executor, and accepts its
// answer after the binding and the response schema check out. Scripts, relays,
// fake hosts and the replay of recordings all drive it; none of them is trusted
// beyond what the checks verify.
package requests

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/executorrun"
)

// Identity of the executor a command-line submission is recorded under: the
// module's plugin name and this suffix give its harness name.
const (
	cliHarnessSuffix  = "-cli"
	cliHarnessVersion = "1"
	relayHarnessName  = "systemone-relay"
)

// Receipt is what a submission returns: the bare text an executor replies with,
// and whether a schema failure may still be corrected.
type Receipt struct {
	Text              string
	CorrectionAllowed bool
}

// Origin tells where a submission comes from, which fixes the executor profile
// and the marks the response carries.
type Origin string

// Origins of a submission.
const (
	// OriginCLI: someone ran `request submit`, outside any executor.
	OriginCLI Origin = "cli"
	// OriginReplay: a recorded answer served back, with the executor it had.
	OriginReplay Origin = "replay"
	// OriginRelay: an answer from a System One endpoint, relayed.
	OriginRelay Origin = "relay"
	// OriginExecutor: an executor answered through the module's MCP server.
	OriginExecutor Origin = "executor"
	// OriginSampled: the client sampled a model for the module's MCP server.
	OriginSampled Origin = "sampled"
)

// Submission is one answer presented for a ticket.
type Submission struct {
	Ticket hostproto.TicketID
	Nonce  string
	Cap    string
	Output []byte
	Origin Origin
	// Executor is the profile a replayed answer had when it was recorded, or the
	// description a relay gives of its model; it is ignored for a CLI submission.
	Executor *format.ExecutorProfile
}

// ExecutorContext describes the host an MCP server answers for: the harness it
// runs under and the isolation its executors reach before a stop hook attests
// them.
type ExecutorContext struct {
	Harness   format.Harness
	Isolation format.Isolation
}

// Service implements the verbs of `request` over a host store.
type Service struct {
	// Plugin is the name of the module's plugin, which names the harness of a
	// submission made from the command line.
	Plugin    string
	Store     hostproto.Store
	Validator hostproto.ResponseValidator
	Clock     func() time.Time
	// RequireAttestation refuses a fetch or an answer that carries no capability.
	RequireAttestation bool
	// Executor describes the host the executor origins answer for.
	Executor ExecutorContext
}

// List returns the tickets that wait for an answer: those in state `emitted`,
// ordered by request id then vote.
func (s Service) List() (hostproto.Listing, error) {
	entries, err := s.Store.ListEntries()
	if err != nil {
		return hostproto.Listing{}, fmt.Errorf("list the tickets: %w", err)
	}
	listing := hostproto.Listing{Items: []hostproto.ListingItem{}}
	for _, entry := range entries {
		if entry.State != hostproto.StateEmitted {
			continue
		}
		request, err := s.Store.GetRequest(entry.Ticket.RequestID)
		if err != nil {
			return hostproto.Listing{}, fmt.Errorf("read the request of %s: %w", entry.Ticket.TicketID, err)
		}
		listing.Items = append(listing.Items, hostproto.ListingItem{Ticket: entry.Ticket, Role: entry.Role, View: request.ViewFor(entry.Ticket)})
	}
	return listing, nil
}

func (s Service) verifier() (hostproto.CapVerifier, error) {
	secret, err := s.Store.Secret()
	if err != nil {
		return hostproto.CapVerifier{}, err
	}
	records, err := s.Store.Records()
	if err != nil {
		return hostproto.CapVerifier{}, err
	}
	entries, err := s.Store.ListEntries()
	if err != nil {
		return hostproto.CapVerifier{}, err
	}
	return hostproto.CapVerifier{
		Secret: secret, Roster: hostproto.JournalRoster(records), Bindings: hostproto.EntryCapBindings(entries),
		RequireAttestation: s.RequireAttestation,
	}, nil
}

func (s Service) journal(kind hostproto.EventKind, ticket hostproto.TicketID, detail string) {
	_, _ = s.Store.Append(hostproto.Event{Kind: kind, Ticket: ticket.String(), Detail: detail})
}

// Show fetches the ticket for an executor: it moves the ticket to `fetched` and
// returns what an executor sees, the prompt, the schema and the nonce.
func (s Service) Show(ticket hostproto.TicketID, capability string) (hostproto.ExecutorView, error) {
	entry, err := s.Store.GetEntry(ticket.Key())
	if err != nil {
		return hostproto.ExecutorView{}, err
	}
	verifier, err := s.verifier()
	if err != nil {
		return hostproto.ExecutorView{}, err
	}
	binding, err := hostproto.CheckFetch(entry, ticket, capability, verifier)
	if err != nil {
		s.journal(hostproto.EventExposureAttempt, ticket, err.Error())
		return hostproto.ExecutorView{}, err
	}
	fetched, err := s.Store.UpdateEntry(ticket.Key(), func(e hostproto.Entry) (hostproto.Entry, error) {
		return e.Fetch(binding.CapDigest, s.Clock())
	})
	if err != nil {
		return hostproto.ExecutorView{}, err
	}
	request, err := s.Store.GetRequest(ticket.RequestID)
	if err != nil {
		return hostproto.ExecutorView{}, err
	}
	s.journal(hostproto.EventFetched, ticket, "")
	return request.ViewFor(fetched.Ticket), nil
}

// Submit presents an answer: the binding, the response schema and the lifecycle
// decide whether it is accepted. A submission without a capability is journaled
// as exposed, because it did not come through an executor; a replayed or relayed
// answer is not.
func (s Service) Submit(submission Submission) (Receipt, error) {
	entry, err := s.Store.GetEntry(submission.Ticket.Key())
	if err != nil {
		return Receipt{}, err
	}
	request, err := s.Store.GetRequest(submission.Ticket.RequestID)
	if err != nil {
		return Receipt{}, err
	}
	schemaDigest, err := request.SchemaDigest()
	if err != nil {
		return Receipt{}, err
	}
	verifier, err := s.verifier()
	if err != nil {
		return Receipt{}, err
	}
	binding, err := hostproto.CheckBinding(entry, schemaDigest, hostproto.Submission{Ticket: submission.Ticket, Nonce: submission.Nonce, Cap: submission.Cap}, verifier)
	if err != nil {
		s.journalRefusal(submission.Ticket, err)
		return Receipt{}, err
	}
	if err := s.Validator.Validate(request.ResponseSchema, submission.Output); err != nil {
		return s.failSchema(submission.Ticket, err)
	}
	return s.accept(submission, binding)
}

func (s Service) journalRefusal(ticket hostproto.TicketID, err error) {
	kind := hostproto.EventExposureAttempt
	if errors.Is(err, hostproto.ErrNonceMismatch) || errors.Is(err, hostproto.ErrTicketMismatch) {
		kind = hostproto.EventRejected
	}
	s.journal(kind, ticket, err.Error())
}

func (s Service) failSchema(ticket hostproto.TicketID, cause error) (Receipt, error) {
	var allowed bool
	_, err := s.Store.UpdateEntry(ticket.Key(), func(e hostproto.Entry) (hostproto.Entry, error) {
		next, correctionAllowed, failure := e.FailSchema()
		allowed = correctionAllowed
		return next, failure
	})
	if err != nil {
		return Receipt{}, err
	}
	s.journal(hostproto.EventSchemaFailed, ticket, "")
	if !allowed {
		s.journal(hostproto.EventRejected, ticket, "schema")
	}
	return Receipt{CorrectionAllowed: allowed}, cause
}

func (s Service) accept(submission Submission, binding hostproto.Binding) (Receipt, error) {
	digest, err := hostproto.OutputDigest(submission.Output)
	if err != nil {
		return Receipt{}, err
	}
	_, err = s.Store.UpdateEntry(submission.Ticket.Key(), func(e hostproto.Entry) (hostproto.Entry, error) { return e.Accept(digest) })
	receipt := Receipt{Text: executorrun.Receipt(submission.Ticket)}
	switch {
	case errors.Is(err, hostproto.ErrAlreadyAccepted):
		return receipt, nil
	case errors.Is(err, hostproto.ErrResubmissionConflict):
		s.journal(hostproto.EventResubmissionConflict, submission.Ticket, "")
		return Receipt{}, err
	case err != nil:
		return Receipt{}, err
	}
	response := hostproto.Response{
		RequestID: submission.Ticket.RequestID, Vote: submission.Ticket.Vote, Nonce: submission.Nonce,
		Output: json.RawMessage(submission.Output), Executor: s.profile(submission, binding),
	}
	if err := response.Validate(); err != nil {
		return Receipt{}, err
	}
	if err := s.Store.PutResponse(response); err != nil {
		return Receipt{}, err
	}
	s.journal(hostproto.EventAccepted, submission.Ticket, "")
	s.journalUnattested(submission, binding)
	return receipt, nil
}

// journalUnattested records an answer that no capability backs: outside an
// executor it is exposed, and inside one whose attestation was waived it is
// restricted.
func (s Service) journalUnattested(submission Submission, binding hostproto.Binding) {
	if binding.Attested {
		return
	}
	switch submission.Origin {
	case OriginCLI:
		s.journal(hostproto.EventExposed, submission.Ticket, "submitted outside an executor")
	case OriginExecutor:
		s.journal(hostproto.EventRestricted, submission.Ticket, "attestation waived")
	}
}

// profile is the executor profile a response is recorded with.
func (s Service) profile(submission Submission, binding hostproto.Binding) format.ExecutorProfile {
	if submission.Executor != nil && submission.Origin != OriginCLI {
		return *submission.Executor
	}
	switch submission.Origin {
	case OriginExecutor:
		return s.executorProfile(binding)
	case OriginSampled:
		return format.ExecutorProfile{
			Channel: format.ChannelHostSampled, Harness: s.Executor.Harness, Isolation: format.IsolationRestricted,
			Model: format.ExecutorModel{Source: format.ModelSourceUnknown},
		}
	}
	if binding.Attested {
		return format.ExecutorProfile{
			Channel:     format.ChannelHostSubagent,
			Harness:     format.Harness{Name: s.Plugin + cliHarnessSuffix, Version: cliHarnessVersion},
			Isolation:   format.IsolationRestricted,
			Attestation: format.Attestation{AgentID: binding.AgentID, CapDigest: binding.CapDigest},
			Model:       format.ExecutorModel{Source: format.ModelSourceUnknown},
		}
	}
	return format.ExecutorProfile{
		Channel:   format.ChannelHostInline,
		Harness:   format.Harness{Name: s.Plugin + cliHarnessSuffix, Version: cliHarnessVersion},
		Isolation: format.IsolationInline,
		Model:     format.ExecutorModel{Source: format.ModelSourceUnknown},
	}
}

// executorProfile is the profile of an answer an executor gave through the MCP
// server: the isolation its harness enforces, with the capability's evidence when
// one was presented. The stop hook raises it to attested.
func (s Service) executorProfile(binding hostproto.Binding) format.ExecutorProfile {
	profile := format.ExecutorProfile{
		Channel: format.ChannelHostSubagent, Harness: s.Executor.Harness, Isolation: s.Executor.Isolation,
		Model: format.ExecutorModel{Source: format.ModelSourceUnknown},
	}
	if binding.Attested {
		profile.Attestation = format.Attestation{AgentID: binding.AgentID, CapDigest: binding.CapDigest}
	}
	return profile
}

// RelayProfile describes the executor of an answer a System One endpoint gave.
func RelayProfile(source string) format.ExecutorProfile {
	return format.ExecutorProfile{
		Channel:   format.ChannelSystemOneRelay,
		Harness:   format.Harness{Name: relayHarnessName, Version: cliHarnessVersion},
		Isolation: format.IsolationRestricted,
		Model:     format.ExecutorModel{Name: source, Source: format.ModelSourceDeclared},
	}
}

// ForSampling returns the service for answers served through sampling: no agent
// holds a capability there, so none is required.
func (s Service) ForSampling() Service {
	s.RequireAttestation = false
	return s
}
