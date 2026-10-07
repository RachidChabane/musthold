package executorhost_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/executorhost"
	"github.com/RachidChabane/spec-proof/format/hostproto/executorrun"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
	"github.com/RachidChabane/spec-proof/format/hostproto/mcpstdio"
	"github.com/RachidChabane/spec-proof/format/hostproto/requests"
)

const (
	moduleAlias   = "dp"
	judgeRole     = "spec-proof-dp:judge"
	agentID       = "agent-0001"
	otherAgent    = "agent-0002"
	validAnswer   = `{"validity":"VALID"}`
	otherAnswer   = `{"validity":"EQUIVALENT"}`
	brokenAnswer  = `{"verdict":"VALID"}`
	harnessName   = "claude-code"
	harnessNumber = "2.1.292"
)

type fixture struct {
	t        *testing.T
	store    *hostprototest.MemoryStore
	service  executorhost.Service
	requests requests.Service
	request  hostproto.Request
	ticket   hostproto.Ticket
}

func newFixture(t *testing.T, requireAttestation bool) *fixture {
	t.Helper()
	store := hostprototest.NewMemoryStore()
	service := requests.Service{Plugin: hostprototest.PluginName,
		Store: store, Validator: hostprototest.MemberValidator{}, Clock: func() time.Time { return hostprototest.Epoch },
		RequireAttestation: requireAttestation,
		Executor: requests.ExecutorContext{
			Harness: format.Harness{Name: harnessName, Version: harnessNumber}, Isolation: format.IsolationRestricted,
		},
	}
	f := &fixture{t: t, store: store, requests: service, service: executorhost.Service{Requests: service, ModuleNames: []string{moduleAlias, hostprototest.ModuleName}}, request: hostprototest.Request()}
	f.ticket = f.emit(f.request, 0)
	return f
}

func (f *fixture) emit(request hostproto.Request, vote int) hostproto.Ticket {
	f.t.Helper()
	if _, err := f.store.PutRequest(request); err != nil {
		f.t.Fatal(err)
	}
	digest, err := request.SchemaDigest()
	if err != nil {
		f.t.Fatal(err)
	}
	ticket := hostprototest.Ticket(request, vote, hostproto.FirstEmission)
	if err := f.store.PutEntry(hostproto.NewEntry(ticket, request.Role, digest, hostproto.DefaultMaxEmissions)); err != nil {
		f.t.Fatal(err)
	}
	return ticket
}

// startAgent journals the start of an executor and returns its capability.
func (f *fixture) startAgent(id string) string {
	f.t.Helper()
	if _, err := f.store.Append(hostproto.Event{Kind: hostproto.EventAgentStarted, AgentID: id, Role: judgeRole}); err != nil {
		f.t.Fatal(err)
	}
	secret, err := f.store.Secret()
	if err != nil {
		f.t.Fatal(err)
	}
	capability, err := hostproto.FormatCap(secret, id)
	if err != nil {
		f.t.Fatal(err)
	}
	return capability
}

func (f *fixture) fetch(ticket hostproto.Ticket, capability string) (mcpstdio.FetchResult, error) {
	return f.service.FetchRequest(context.Background(), mcpstdio.FetchArgs{Ticket: ticket.TicketID.String(), Cap: capability})
}

func (f *fixture) submit(ticket hostproto.Ticket, output, capability string) (mcpstdio.SubmitResult, error) {
	return f.service.SubmitResponse(context.Background(), mcpstdio.SubmitArgs{
		Ticket: ticket.TicketID.String(), Nonce: ticket.Nonce, Output: json.RawMessage(output), Cap: capability,
	})
}

func (f *fixture) kinds() []hostproto.EventKind {
	records, err := f.store.Records()
	if err != nil {
		f.t.Fatal(err)
	}
	var kinds []hostproto.EventKind
	for _, record := range records {
		kinds = append(kinds, record.Event.Kind)
	}
	return kinds
}

func (f *fixture) response() hostproto.Response {
	f.t.Helper()
	response, err := f.store.GetResponse(f.ticket.Key())
	if err != nil {
		f.t.Fatal(err)
	}
	return response
}

func TestAnswerWithACapability(t *testing.T) {
	t.Run("an executor with a capability of a started agent fetches and is recorded as a restricted subagent [verifies SP-702~1]", func(t *testing.T) {
		f := newFixture(t, true)
		capability := f.startAgent(agentID)
		result, err := f.fetch(f.ticket, capability)
		if err != nil || result.View.Nonce != f.ticket.Nonce || result.View.Ticket != f.ticket.TicketID.String() {
			t.Fatalf("fetch = %+v, %v", result, err)
		}
		submitted, err := f.submit(f.ticket, validAnswer, capability)
		if err != nil || submitted.Receipt != executorrun.Receipt(f.ticket.TicketID) {
			t.Fatalf("submit = %+v, %v", submitted, err)
		}
		executor := f.response().Executor
		if executor.Channel != format.ChannelHostSubagent || executor.Isolation != format.IsolationRestricted ||
			executor.Harness != (format.Harness{Name: harnessName, Version: harnessNumber}) ||
			executor.Attestation.AgentID != agentID || executor.Attestation.CapDigest != hostproto.CapDigest(capability) {
			t.Fatalf("executor = %+v", executor)
		}
		if slices.Contains(f.kinds(), hostproto.EventRestricted) || slices.Contains(f.kinds(), hostproto.EventExposed) {
			t.Fatalf("events = %v", f.kinds())
		}
	})

	t.Run("without a capability a fetch and an answer are refused while attestation is required [verifies SP-702~1]", func(t *testing.T) {
		f := newFixture(t, true)
		if _, err := f.fetch(f.ticket, ""); !errors.Is(err, hostproto.ErrCapMissing) {
			t.Fatalf("fetch error = %v", err)
		}
		if _, err := f.submit(f.ticket, validAnswer, ""); err == nil {
			t.Fatal("an answer without a capability was accepted")
		}
		entry, _ := f.store.GetEntry(f.ticket.Key())
		if entry.State != hostproto.StateEmitted || !slices.Contains(f.kinds(), hostproto.EventExposureAttempt) {
			t.Fatalf("state %s, events %v", entry.State, f.kinds())
		}
	})

	t.Run("a capability of an agent that never started, or of a forged secret, is refused [verifies SP-702~1]", func(t *testing.T) {
		f := newFixture(t, true)
		secret, _ := f.store.Secret()
		unstarted, _ := hostproto.FormatCap(secret, otherAgent)
		forged, _ := hostproto.FormatCap([]byte("not-the-secret"), agentID)
		f.startAgent(agentID)
		if _, err := f.fetch(f.ticket, unstarted); !errors.Is(err, hostproto.ErrAgentNotStarted) {
			t.Errorf("unstarted agent: %v", err)
		}
		if _, err := f.fetch(f.ticket, forged); !errors.Is(err, hostproto.ErrBadCap) {
			t.Errorf("forged secret: %v", err)
		}
	})

	t.Run("one capability serves one ticket only [verifies SP-702~1]", func(t *testing.T) {
		f := newFixture(t, true)
		second := f.emit(hostprototest.Request(func(r *hostproto.Request) { r.Prompt.User = "another mutant" }), 0)
		capability := f.startAgent(agentID)
		if _, err := f.fetch(f.ticket, capability); err != nil {
			t.Fatal(err)
		}
		if _, err := f.fetch(second, capability); !errors.Is(err, hostproto.ErrCapBoundElsewhere) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("a waived attestation accepts an answer without a capability and journals it as restricted [verifies SP-702~1]", func(t *testing.T) {
		f := newFixture(t, false)
		if _, err := f.fetch(f.ticket, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := f.submit(f.ticket, validAnswer, ""); err != nil {
			t.Fatal(err)
		}
		executor := f.response().Executor
		if executor.Attestation.AgentID != "" || executor.Isolation != format.IsolationRestricted || executor.Channel != format.ChannelHostSubagent {
			t.Fatalf("executor = %+v", executor)
		}
		if kinds := f.kinds(); !slices.Contains(kinds, hostproto.EventRestricted) || slices.Contains(kinds, hostproto.EventExposed) {
			t.Fatalf("events = %v", kinds)
		}
	})
}

func TestToolBehaviour(t *testing.T) {
	t.Run("a schema violation is returned as a result and allows one correction before the ticket is rejected [verifies SP-703~1]", func(t *testing.T) {
		f := newFixture(t, false)
		if _, err := f.fetch(f.ticket, ""); err != nil {
			t.Fatal(err)
		}
		first, err := f.submit(f.ticket, brokenAnswer, "")
		if err != nil || len(first.SchemaErrors) == 0 || first.Receipt != "" {
			t.Fatalf("first = %+v, %v", first, err)
		}
		if entry, _ := f.store.GetEntry(f.ticket.Key()); entry.State != hostproto.StateFetched {
			t.Fatalf("state = %s after the first violation", entry.State)
		}
		if _, err := f.submit(f.ticket, brokenAnswer, ""); err != nil {
			t.Fatal(err)
		}
		if entry, _ := f.store.GetEntry(f.ticket.Key()); entry.State != hostproto.StateRejected {
			t.Fatalf("state = %s after the second violation", entry.State)
		}
	})

	t.Run("the corrected answer is accepted after one violation [verifies SP-703~1]", func(t *testing.T) {
		f := newFixture(t, false)
		if _, err := f.fetch(f.ticket, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := f.submit(f.ticket, brokenAnswer, ""); err != nil {
			t.Fatal(err)
		}
		if result, err := f.submit(f.ticket, validAnswer, ""); err != nil || result.Receipt == "" {
			t.Fatalf("result = %+v, %v", result, err)
		}
	})

	t.Run("a second, different answer is a resubmission conflict and the same answer is a replay [verifies SP-703~1]", func(t *testing.T) {
		f := newFixture(t, false)
		if _, err := f.fetch(f.ticket, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := f.submit(f.ticket, validAnswer, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := f.submit(f.ticket, `{ "validity" : "VALID" }`, ""); err != nil {
			t.Fatalf("replay: %v", err)
		}
		if _, err := f.submit(f.ticket, otherAnswer, ""); !errors.Is(err, hostproto.ErrResubmissionConflict) {
			t.Fatalf("conflict: %v", err)
		}
		if string(f.response().Output) != validAnswer {
			t.Fatalf("the first answer was replaced by %s", f.response().Output)
		}
		if !slices.Contains(f.kinds(), hostproto.EventResubmissionConflict) {
			t.Fatalf("events = %v", f.kinds())
		}
	})

	t.Run("a ticket that is not an identifier is refused before the store is asked [verifies SP-703~1]", func(t *testing.T) {
		f := newFixture(t, false)
		if _, err := f.service.FetchRequest(context.Background(), mcpstdio.FetchArgs{Ticket: "latest"}); !errors.Is(err, hostproto.ErrBadTicket) {
			t.Errorf("fetch: %v", err)
		}
		if _, err := f.service.SubmitResponse(context.Background(), mcpstdio.SubmitArgs{Ticket: "latest", Output: json.RawMessage(`{}`)}); !errors.Is(err, hostproto.ErrBadTicket) {
			t.Errorf("submit: %v", err)
		}
	})

	t.Run("an answer with another nonce than the issue's is refused [verifies SP-703~1]", func(t *testing.T) {
		f := newFixture(t, false)
		if _, err := f.fetch(f.ticket, ""); err != nil {
			t.Fatal(err)
		}
		stale := f.ticket
		stale.Nonce = "00000000000000000000000000000000"
		if _, err := f.submit(stale, validAnswer, ""); !errors.Is(err, hostproto.ErrNonceMismatch) {
			t.Fatalf("error = %v", err)
		}
	})
}

// scriptedSampler answers every sampling request from a script.
type scriptedSampler struct {
	answers  []string
	failWith error
	asked    []mcpstdio.SamplingRequest
}

func (s *scriptedSampler) Sample(_ context.Context, request mcpstdio.SamplingRequest) (string, error) {
	s.asked = append(s.asked, request)
	if s.failWith != nil {
		return "", s.failWith
	}
	answer := s.answers[0]
	s.answers = s.answers[1:]
	return answer, nil
}

func TestDrain(t *testing.T) {
	drain := func(f *fixture, sampler mcpstdio.Sampler, module string) (mcpstdio.DrainResult, error) {
		return f.service.Drain(context.Background(), mcpstdio.DrainArgs{Module: module}, sampler)
	}

	t.Run("every pending ticket is put to the sampled model blind and recorded as a sampled response [verifies SP-703~1]", func(t *testing.T) {
		f := newFixture(t, true)
		second := f.emit(hostprototest.Request(func(r *hostproto.Request) { r.Prompt.User = "another mutant" }), 0)
		sampler := &scriptedSampler{answers: []string{"```json\n" + validAnswer + "\n```", "I think " + otherAnswer + " fits."}}
		result, err := drain(f, sampler, moduleAlias)
		if err != nil || result.Served != 2 {
			t.Fatalf("result = %+v, %v", result, err)
		}
		if len(sampler.asked) != 2 || sampler.asked[0].System != f.request.Prompt.System {
			t.Fatalf("asked = %+v", sampler.asked)
		}
		response := f.response()
		if string(response.Output) != validAnswer || response.Executor.Channel != format.ChannelHostSampled ||
			response.Executor.Isolation != format.IsolationRestricted || response.Executor.Attestation.AgentID != "" {
			t.Fatalf("response = %+v", response)
		}
		if got, err := f.store.GetResponse(second.Key()); err != nil || string(got.Output) != otherAnswer {
			t.Fatalf("second = %+v, %v", got, err)
		}
		if err := response.Validate(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("the module is named by its alias or its binary and no other [verifies SP-703~1]", func(t *testing.T) {
		f := newFixture(t, true)
		if _, err := drain(f, &scriptedSampler{}, "alignment"); !errors.Is(err, executorhost.ErrUnknownModule) {
			t.Fatalf("error = %v", err)
		}
		sampler := &scriptedSampler{answers: []string{validAnswer}}
		if result, err := drain(f, sampler, hostprototest.ModuleName); err != nil || result.Served != 1 {
			t.Fatalf("result = %+v, %v", result, err)
		}
	})

	t.Run("a refused sample or an answer without a JSON object stops the drain and leaves nothing accepted [verifies SP-703~1]", func(t *testing.T) {
		refused := errors.New("user declined")
		f := newFixture(t, true)
		if result, err := drain(f, &scriptedSampler{failWith: refused}, moduleAlias); !errors.Is(err, refused) || result.Served != 0 {
			t.Fatalf("refused: %+v, %v", result, err)
		}
		g := newFixture(t, true)
		if _, err := drain(g, &scriptedSampler{answers: []string{"no object here"}}, moduleAlias); !errors.Is(err, executorhost.ErrNoJSONObject) {
			t.Fatalf("prose: %v", err)
		}
		h := newFixture(t, true)
		if _, err := drain(h, &scriptedSampler{answers: []string{brokenAnswer}}, moduleAlias); !errors.Is(err, hostproto.ErrSchemaViolation) {
			t.Fatalf("schema: %v", err)
		}
		for _, fixture := range []*fixture{f, g, h} {
			if slices.Contains(fixture.kinds(), hostproto.EventAccepted) {
				t.Fatalf("events = %v", fixture.kinds())
			}
		}
	})

	t.Run("a drain with nothing pending serves nothing [verifies SP-703~1]", func(t *testing.T) {
		f := newFixture(t, true)
		if _, err := drain(f, &scriptedSampler{answers: []string{validAnswer}}, moduleAlias); err != nil {
			t.Fatal(err)
		}
		sampler := &scriptedSampler{}
		if result, err := drain(f, sampler, moduleAlias); err != nil || result.Served != 0 || len(sampler.asked) != 0 {
			t.Fatalf("result = %+v, %v", result, err)
		}
	})
}
