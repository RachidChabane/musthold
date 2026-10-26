package hostprototest

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// Names of the fake-host personas of ADR 0005.
const (
	PersonaHonest        = "honest"
	PersonaForger        = "forger"
	PersonaOmitter       = "omitter"
	PersonaStale         = "stale"
	PersonaBreaker       = "breaker"
	PersonaLeaker        = "leaker"
	PersonaPadder        = "padder"
	PersonaModelSwapper  = "model-swapper"
	PersonaHookless      = "hookless"
	PersonaUnrestricted  = "unrestricted"
	PersonaSelector      = "selector"
	PersonaChurner       = "churner"
	PersonaSlow          = "slow"
	PersonaInheritor     = "inheritor"
	invalidAnswer        = `{"validity":"MAYBE"}`
	swappedModel         = "claude-haiku-4-5"
	resolvedModel        = "claude-sonnet-5-5"
	forgerAgentID        = "agent-9999"
	forgerSecret         = "not-the-install-secret"
	leakedReplySuffix    = " The mutant changes the boundary at 15 minutes."
	paddingPrefix        = "The tests live in tests/test_parkfee.py. "
	selectorTrivialValue = `{"validity":"VALID"}`
)

// Env is what a persona is given: the host under test, the request to answer, its
// ticket and the answer an honest executor gives.
type Env struct {
	Host    Host
	Request hostproto.Request
	Ticket  hostproto.Ticket
	Answer  json.RawMessage
	// Out receives what a persona wants to report besides its errors.
	Out *Outcome
}

// Outcome is what a persona's run left behind.
type Outcome struct {
	// Errors are the errors the host returned to the persona, in order.
	Errors []error
	// Launched is the Agent input the harness would have used, when the persona launched one.
	Launched *hostproto.AgentInput
	// Ticket is the current ticket of the request after the run.
	Ticket hostproto.Ticket
	// Entry is the stored entry after the run.
	Entry hostproto.Entry
	// Profile is the executor profile recorded for the answer, when one was accepted.
	Profile *format.ExecutorProfile
	// Events are the journaled event kinds, in order.
	Events []hostproto.EventKind
}

// Verdict is what the protocol must make of a persona.
type Verdict struct {
	// Final is the lifecycle state the ticket ends in.
	Final hostproto.State
	// FirstError, when set, is the error the host returns first.
	FirstError error
	// Present events must be journaled; Absent ones must not be.
	Present []hostproto.EventKind
	Absent  []hostproto.EventKind
	// Extra holds the checks that are specific to the persona.
	Extra func(Outcome) error
}

// Persona is a fake host with a behaviour and the verdict the protocol gives it.
type Persona interface {
	// Name is the persona's name.
	Name() string
	// Configure returns the host the persona runs against.
	Configure() (hostproto.Observed, hostproto.Policy)
	// Play drives the host.
	Play(env Env) []error
	// Verdict is the asserted outcome.
	Verdict() Verdict
}

// PersonaNames lists the personas of ADR 0005.
func PersonaNames() []string {
	return []string{
		PersonaHonest, PersonaForger, PersonaOmitter, PersonaStale, PersonaBreaker, PersonaLeaker, PersonaPadder,
		PersonaModelSwapper, PersonaHookless, PersonaUnrestricted, PersonaSelector, PersonaChurner, PersonaSlow,
		PersonaInheritor,
	}
}

// ErrUnknownPersona is returned for a name that is not a persona.
var ErrUnknownPersona = errors.New("unknown persona")

// NewPersona builds a persona by name.
func NewPersona(name string) (Persona, error) {
	constructors := map[string]func() Persona{
		PersonaHonest:       func() Persona { return basicPersona{name: name, verdict: honestVerdict()} },
		PersonaForger:       func() Persona { return forger{} },
		PersonaOmitter:      func() Persona { return omitter{} },
		PersonaStale:        func() Persona { return stale{} },
		PersonaBreaker:      func() Persona { return breaker{} },
		PersonaLeaker:       func() Persona { return leaker{} },
		PersonaPadder:       func() Persona { return padder{} },
		PersonaModelSwapper: func() Persona { return modelSwapper{} },
		PersonaHookless:     func() Persona { return Hookless{} },
		PersonaUnrestricted: func() Persona { return unrestricted{} },
		PersonaSelector:     func() Persona { return selector{} },
		PersonaChurner:      func() Persona { return churner{} },
		PersonaSlow:         func() Persona { return slow{} },
		PersonaInheritor:    func() Persona { return inheritor{} },
	}
	construct, known := constructors[name]
	if !known {
		if persona, copilot := newCopilotPersona(name); copilot {
			return persona, nil
		}
		return nil, fmt.Errorf("%w: %q", ErrUnknownPersona, name)
	}
	return construct(), nil
}

// Drive plays the persona against a host built by newHost and returns what it left.
func Drive(persona Persona, newHost func(hostproto.Observed, hostproto.Policy) Host) (Outcome, error) {
	observed, policy := persona.Configure()
	host := newHost(observed, policy)
	request := Request()
	ticket, err := host.Emit(request)
	if err != nil {
		return Outcome{}, err
	}
	outcome := Outcome{}
	outcome.Errors = persona.Play(Env{Host: host, Request: request, Ticket: ticket, Answer: json.RawMessage(HonestAnswer), Out: &outcome})
	key := ticket.Key()
	entry, err := host.Entry(key)
	if err != nil {
		return Outcome{}, err
	}
	outcome.Ticket, outcome.Entry = entry.Ticket, entry
	if profile, err := host.Profile(key); err == nil {
		outcome.Profile = &profile
	}
	for _, event := range host.Journal() {
		outcome.Events = append(outcome.Events, event.Kind)
	}
	return outcome, nil
}

// Check compares an outcome with the persona's verdict and returns every
// disagreement.
func Check(persona Persona, outcome Outcome) error {
	verdict := persona.Verdict()
	var problems []error
	if outcome.Entry.State != verdict.Final {
		problems = append(problems, fmt.Errorf("final state %s, want %s", outcome.Entry.State, verdict.Final))
	}
	if verdict.FirstError != nil && (len(outcome.Errors) == 0 || !errors.Is(outcome.Errors[0], verdict.FirstError)) {
		problems = append(problems, fmt.Errorf("first error %v, want %v", firstOf(outcome.Errors), verdict.FirstError))
	}
	if verdict.FirstError == nil && len(outcome.Errors) > 0 {
		problems = append(problems, fmt.Errorf("unexpected error %v", outcome.Errors[0]))
	}
	for _, kind := range verdict.Present {
		if !slices.Contains(outcome.Events, kind) {
			problems = append(problems, fmt.Errorf("event %s was not journaled", kind))
		}
	}
	for _, kind := range verdict.Absent {
		if slices.Contains(outcome.Events, kind) {
			problems = append(problems, fmt.Errorf("event %s was journaled", kind))
		}
	}
	if verdict.Extra != nil {
		if err := verdict.Extra(outcome); err != nil {
			problems = append(problems, err)
		}
	}
	return errors.Join(problems...)
}

func firstOf(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	return errs[0]
}

// executor runs one honest executor round trip and returns the errors met.
type executor struct {
	host    Host
	request hostproto.Request
	ticket  hostproto.Ticket
	agentID string
	cap     string
}

func startExecutor(env Env) *executor {
	agent := env.Host.StartExecutor(env.Request.Role)
	return &executor{host: env.Host, request: env.Request, ticket: env.Ticket, agentID: agent, cap: env.Host.Cap(agent)}
}

func (e *executor) fetch() (hostproto.ExecutorView, error) {
	return e.host.Fetch(e.ticket.TicketID, e.cap)
}

func (e *executor) submit(nonce string, output string) (Receipt, error) {
	return e.host.Submit(Submit{Ticket: e.ticket.TicketID, Nonce: nonce, Cap: e.cap, Output: json.RawMessage(output)})
}

func (e *executor) finish(receipt Receipt, submits int, reply string, model string) error {
	return e.host.AfterRun(e.run(submits, reply, model))
}

func (e *executor) run(submits int, reply string, model string) Run {
	return Run{Ticket: e.ticket.TicketID, AgentID: e.agentID, Transcript: transcript(e.ticket.TicketID, submits), Reply: reply, ResolvedModel: model}
}

func transcript(ticket hostproto.TicketID, submits int) []hostproto.ToolCall {
	input := json.RawMessage(fmt.Sprintf(`{"ticket":%q}`, ticket.String()))
	calls := []hostproto.ToolCall{{Name: ExecutorToolName(hostproto.ToolFetchRequest), Input: input}}
	for range submits {
		calls = append(calls, hostproto.ToolCall{Name: ExecutorToolName(hostproto.ToolSubmitResponse), Input: input})
	}
	return calls
}

// honestRun fetches, submits the answer and finishes the run with the bare receipt.
func honestRun(env Env, answer string, reply func(Receipt) string, model string) []error {
	run := startExecutor(env)
	view, err := run.fetch()
	if err != nil {
		return []error{err}
	}
	receipt, err := run.submit(view.Nonce, answer)
	if err != nil {
		return []error{err}
	}
	if err := run.finish(receipt, 1, reply(receipt), model); err != nil {
		return []error{err}
	}
	return nil
}

func bareReceipt(receipt Receipt) string { return receipt.Text }

func honestVerdict() Verdict {
	return Verdict{
		Final:   hostproto.StateAccepted,
		Present: []hostproto.EventKind{hostproto.EventAgentStarted, hostproto.EventFetched, hostproto.EventAccepted, hostproto.EventAttested},
		Absent:  []hostproto.EventKind{hostproto.EventLeaked, hostproto.EventExposed, hostproto.EventRestricted, hostproto.EventExposureAttempt, hostproto.EventResubmissionConflict},
		Extra: func(o Outcome) error {
			if o.Profile == nil || o.Profile.Isolation != format.IsolationRestrictedAttested {
				return fmt.Errorf("profile %+v is not attested", o.Profile)
			}
			return nil
		},
	}
}

// basicPersona is a persona whose behaviour is an honest run.
type basicPersona struct {
	name    string
	verdict Verdict
	answer  string
}

func (p basicPersona) Name() string { return p.name }
func (basicPersona) Configure() (hostproto.Observed, hostproto.Policy) {
	return ClaudeCodeObserved(), RequireAttestation()
}
func (p basicPersona) Play(env Env) []error {
	answer := p.answer
	if answer == "" {
		answer = string(env.Answer)
	}
	return honestRun(env, answer, bareReceipt, resolvedModel)
}
func (p basicPersona) Verdict() Verdict { return p.verdict }

// selector answers with trivially satisfiable content: the protocol accepts it and
// leaves the measurement of its worth to the modules.
type selector struct{}

func (selector) Name() string { return PersonaSelector }
func (selector) Configure() (hostproto.Observed, hostproto.Policy) {
	return ClaudeCodeObserved(), RequireAttestation()
}
func (selector) Play(env Env) []error {
	return honestRun(env, selectorTrivialValue, bareReceipt, resolvedModel)
}
func (selector) Verdict() Verdict { return honestVerdict() }

// forger holds no capability: it fetches uncapped, then with a capability made
// under a secret of its own, then submits without fetching.
type forger struct{}

func (forger) Name() string { return PersonaForger }
func (forger) Configure() (hostproto.Observed, hostproto.Policy) {
	return ClaudeCodeObserved(), RequireAttestation()
}
func (forger) Play(env Env) []error {
	var errs []error
	_, err := env.Host.Fetch(env.Ticket.TicketID, "")
	errs = append(errs, err)
	forged, _ := hostproto.FormatCap([]byte(forgerSecret), forgerAgentID)
	_, err = env.Host.Fetch(env.Ticket.TicketID, forged)
	errs = append(errs, err)
	_, err = env.Host.Submit(Submit{Ticket: env.Ticket.TicketID, Nonce: env.Ticket.Nonce, Cap: forged, Output: env.Answer})
	return append(errs, err)
}
func (forger) Verdict() Verdict {
	return Verdict{
		Final: hostproto.StateEmitted, FirstError: hostproto.ErrCapMissing,
		Present: []hostproto.EventKind{hostproto.EventExposureAttempt},
		Absent:  []hostproto.EventKind{hostproto.EventAccepted, hostproto.EventFetched},
	}
}

// omitter never answers.
type omitter struct{}

func (omitter) Name() string { return PersonaOmitter }
func (omitter) Configure() (hostproto.Observed, hostproto.Policy) {
	return ClaudeCodeObserved(), RequireAttestation()
}
func (omitter) Play(Env) []error { return nil }
func (omitter) Verdict() Verdict {
	return Verdict{Final: hostproto.StateEmitted, Absent: []hostproto.EventKind{hostproto.EventFetched, hostproto.EventAccepted}}
}

// stale answers a reissued ticket with the nonce of its first issue.
type stale struct{}

func (stale) Name() string { return PersonaStale }
func (stale) Configure() (hostproto.Observed, hostproto.Policy) {
	return ClaudeCodeObserved(), RequireAttestation()
}
func (stale) Play(env Env) []error {
	first := startExecutor(env)
	if _, err := first.fetch(); err != nil {
		return []error{err}
	}
	env.Host.Advance(hostproto.FetchTTL)
	current, err := env.Host.Sweep(env.Ticket.RequestID)
	if err != nil {
		return []error{err}
	}
	second := startExecutor(Env{Host: env.Host, Request: env.Request, Ticket: current})
	if _, err := second.fetch(); err != nil {
		return []error{err}
	}
	_, err = second.submit(env.Ticket.Nonce, string(env.Answer))
	return []error{err}
}
func (stale) Verdict() Verdict {
	return Verdict{
		Final: hostproto.StateFetched, FirstError: hostproto.ErrNonceMismatch,
		Present: []hostproto.EventKind{hostproto.EventExpired, hostproto.EventEmitted},
		Absent:  []hostproto.EventKind{hostproto.EventAccepted},
	}
}

// breaker answers twice with content that violates the response schema.
type breaker struct{}

func (breaker) Name() string { return PersonaBreaker }
func (breaker) Configure() (hostproto.Observed, hostproto.Policy) {
	return ClaudeCodeObserved(), RequireAttestation()
}
func (breaker) Play(env Env) []error {
	run := startExecutor(env)
	view, err := run.fetch()
	if err != nil {
		return []error{err}
	}
	first, firstErr := run.submit(view.Nonce, invalidAnswer)
	_, secondErr := run.submit(view.Nonce, invalidAnswer)
	if !first.CorrectionAllowed {
		return []error{errors.New("the first schema failure did not allow a correction")}
	}
	return []error{firstErr, secondErr}
}
func (breaker) Verdict() Verdict {
	return Verdict{
		Final: hostproto.StateRejected, FirstError: ErrSchemaViolation,
		Present: []hostproto.EventKind{hostproto.EventSchemaFailed, hostproto.EventRejected},
		Absent:  []hostproto.EventKind{hostproto.EventAccepted},
	}
}

// leaker answers correctly and then replies with more than the bare receipt.
type leaker struct{}

func (leaker) Name() string { return PersonaLeaker }
func (leaker) Configure() (hostproto.Observed, hostproto.Policy) {
	return ClaudeCodeObserved(), RequireAttestation()
}
func (leaker) Play(env Env) []error {
	return honestRun(env, string(env.Answer), func(r Receipt) string { return r.Text + leakedReplySuffix }, resolvedModel)
}
func (leaker) Verdict() Verdict {
	return Verdict{
		Final: hostproto.StateAccepted, Present: []hostproto.EventKind{hostproto.EventLeaked},
		Absent: []hostproto.EventKind{hostproto.EventAttested},
		Extra: func(o Outcome) error {
			if o.Profile == nil || o.Profile.Isolation == format.IsolationRestrictedAttested {
				return fmt.Errorf("a leaking executor was attested: %+v", o.Profile)
			}
			return nil
		},
	}
}

// padder pads the prompt of its Agent call and names a model.
type padder struct{}

func (padder) Name() string { return PersonaPadder }
func (padder) Configure() (hostproto.Observed, hostproto.Policy) {
	return ClaudeCodeObserved(), RequireAttestation()
}
func (padder) Play(env Env) []error {
	input := hostproto.AgentInput{
		SubagentType: env.Request.Role, Description: "judge", Model: swappedModel, RunInBackground: true,
		Prompt: paddingPrefix + hostproto.ExecutorPrompt(env.Ticket.TicketID),
	}
	launched, err := env.Host.Launch(input)
	if err != nil {
		return []error{err}
	}
	extra := input
	extra.Prompt = input.Prompt + " " + hostproto.ExecutorPrompt(hostproto.TicketID{RequestID: env.Ticket.RequestID, Vote: 1, Emission: 1})
	if _, err := env.Host.Launch(extra); !errors.Is(err, hostproto.ErrNoSingleTicket) {
		return []error{fmt.Errorf("a prompt with two tickets was not refused: %v", err)}
	}
	env.Out.Launched = &launched
	return honestRun(env, string(env.Answer), bareReceipt, resolvedModel)
}

func (padder) Verdict() Verdict {
	return Verdict{
		Final: hostproto.StateAccepted,
		Extra: func(o Outcome) error {
			if o.Launched == nil {
				return errors.New("no launch was recorded")
			}
			return checkSanitized(*o.Launched)
		},
	}
}

func checkSanitized(launched hostproto.AgentInput) error {
	if launched.Model != "" || launched.RunInBackground || launched.Prompt == "" || launched.Prompt != hostproto.ExecutorPrompt(mustFindTicket(launched.Prompt)) {
		return fmt.Errorf("the launch was not reduced to the bare ticket: %+v", launched)
	}
	return nil
}

func mustFindTicket(prompt string) hostproto.TicketID {
	id, err := hostproto.FindTicketID(prompt)
	if err != nil {
		return hostproto.TicketID{}
	}
	return id
}

// modelSwapper asks for another model than the one it is run with.
type modelSwapper struct{}

func (modelSwapper) Name() string { return PersonaModelSwapper }
func (modelSwapper) Configure() (hostproto.Observed, hostproto.Policy) {
	return ClaudeCodeObserved(), RequireAttestation()
}
func (modelSwapper) Play(env Env) []error {
	launched, err := env.Host.Launch(hostproto.AgentInput{
		SubagentType: env.Request.Role, Description: "judge", Model: swappedModel,
		Prompt: hostproto.ExecutorPrompt(env.Ticket.TicketID),
	})
	if err != nil {
		return []error{err}
	}
	if launched.Model != "" {
		return []error{fmt.Errorf("the model %q survived the rewrite", launched.Model)}
	}
	return honestRun(env, string(env.Answer), bareReceipt, resolvedModel)
}
func (modelSwapper) Verdict() Verdict {
	return Verdict{
		Final: hostproto.StateAccepted,
		Extra: func(o Outcome) error {
			if o.Profile == nil || o.Profile.Model.Name != resolvedModel || o.Profile.Model.Source != format.ModelSourceResolved {
				return fmt.Errorf("the recorded model is %+v, want the resolved %q", o.Profile, resolvedModel)
			}
			return nil
		},
	}
}

// Hookless runs against a host whose hooks never fire. With OptOut, attestation
// is not required and the answer is recorded as restricted; without it, the
// tickets stay pending.
type Hookless struct{ OptOut bool }

// Name implements Persona.
func (Hookless) Name() string { return PersonaHookless }

// Configure implements Persona.
func (h Hookless) Configure() (hostproto.Observed, hostproto.Policy) {
	observed := ClaudeCodeObserved()
	observed.HooksPresent = false
	return observed, hostproto.Policy{RequireAttestation: !h.OptOut}
}

// Play implements Persona.
func (Hookless) Play(env Env) []error {
	run := startExecutor(env)
	view, err := run.fetch()
	if err != nil {
		return []error{err}
	}
	if _, err := run.submit(view.Nonce, string(env.Answer)); err != nil {
		return []error{err}
	}
	return nil
}

// Verdict implements Persona.
func (h Hookless) Verdict() Verdict {
	if h.OptOut {
		return Verdict{Final: hostproto.StateAccepted, Present: []hostproto.EventKind{hostproto.EventRestricted}, Absent: []hostproto.EventKind{hostproto.EventAttested}}
	}
	return Verdict{
		Final: hostproto.StateEmitted, FirstError: hostproto.ErrCapMissing,
		Present: []hostproto.EventKind{hostproto.EventExposureAttempt}, Absent: []hostproto.EventKind{hostproto.EventAccepted},
	}
}

// unrestricted runs on a harness with no allowlist of built-in tools.
type unrestricted struct{}

func (unrestricted) Name() string { return PersonaUnrestricted }
func (unrestricted) Configure() (hostproto.Observed, hostproto.Policy) {
	observed := ClaudeCodeObserved()
	observed.Allowlist = hostproto.AllowlistAbsent
	return observed, hostproto.Policy{}
}
func (unrestricted) Play(env Env) []error {
	return honestRun(env, string(env.Answer), bareReceipt, resolvedModel)
}
func (unrestricted) Verdict() Verdict {
	return Verdict{
		Final: hostproto.StateAccepted,
		Extra: func(o Outcome) error {
			if o.Profile == nil || o.Profile.Isolation != format.IsolationUnrestricted {
				return fmt.Errorf("profile %+v is not unrestricted", o.Profile)
			}
			return nil
		},
	}
}

// inheritor is an executor spawned on a harness with no allowlist of built-in
// tools that started with the history of the agent that spawned it, and
// otherwise answers honestly.
type inheritor struct{}

func (inheritor) Name() string { return PersonaInheritor }
func (inheritor) Configure() (hostproto.Observed, hostproto.Policy) {
	return unrestricted{}.Configure()
}
func (inheritor) Play(env Env) []error {
	run := startExecutor(env)
	view, err := run.fetch()
	if err != nil {
		return []error{err}
	}
	receipt, err := run.submit(view.Nonce, string(env.Answer))
	if err != nil {
		return []error{err}
	}
	finished := run.run(1, bareReceipt(receipt), resolvedModel)
	finished.InheritedHistory = true
	if err := env.Host.AfterRun(finished); err != nil {
		return []error{err}
	}
	return nil
}
func (inheritor) Verdict() Verdict {
	return Verdict{
		Final:   hostproto.StateAccepted,
		Present: []hostproto.EventKind{hostproto.EventAccepted, hostproto.EventHistoryInherited},
		Extra: func(o Outcome) error {
			if o.Profile == nil || o.Profile.Isolation != format.IsolationInline {
				return fmt.Errorf("profile %+v is not inline", o.Profile)
			}
			return nil
		},
	}
}

// churner answers every issue of its ticket with content that breaks the schema,
// until the emissions run out.
type churner struct{}

func (churner) Name() string { return PersonaChurner }
func (churner) Configure() (hostproto.Observed, hostproto.Policy) {
	return ClaudeCodeObserved(), RequireAttestation()
}
func (churner) Play(env Env) []error {
	current := env.Ticket
	for range hostproto.DefaultMaxEmissions {
		run := startExecutor(Env{Host: env.Host, Request: env.Request, Ticket: current})
		view, err := run.fetch()
		if err != nil {
			return []error{err}
		}
		for range hostproto.MaxCorrections + 1 {
			_, _ = run.submit(view.Nonce, invalidAnswer)
		}
		if current, err = env.Host.Sweep(env.Ticket.RequestID); err != nil {
			return []error{err}
		}
	}
	return nil
}
func (churner) Verdict() Verdict {
	return Verdict{
		Final: hostproto.StateAbandoned, Present: []hostproto.EventKind{hostproto.EventRejected, hostproto.EventAbandoned},
		Extra: func(o Outcome) error {
			if o.Entry.Reason != hostproto.ReasonHostUnresponsive || o.Entry.Ticket.Emission != hostproto.DefaultMaxEmissions {
				return fmt.Errorf("entry %+v: want emission %d abandoned as %s", o.Entry, hostproto.DefaultMaxEmissions, hostproto.ReasonHostUnresponsive)
			}
			return nil
		},
	}
}

// slow fetches its ticket and answers after the ticket expired.
type slow struct{}

func (slow) Name() string { return PersonaSlow }
func (slow) Configure() (hostproto.Observed, hostproto.Policy) {
	return ClaudeCodeObserved(), RequireAttestation()
}
func (slow) Play(env Env) []error {
	run := startExecutor(env)
	view, err := run.fetch()
	if err != nil {
		return []error{err}
	}
	env.Host.Advance(hostproto.FetchTTL + time.Second)
	if _, err := env.Host.Sweep(env.Ticket.RequestID); err != nil {
		return []error{err}
	}
	_, err = run.submit(view.Nonce, string(env.Answer))
	return []error{err}
}
func (slow) Verdict() Verdict {
	return Verdict{
		Final: hostproto.StateEmitted, FirstError: hostproto.ErrTicketMismatch,
		Present: []hostproto.EventKind{hostproto.EventExpired}, Absent: []hostproto.EventKind{hostproto.EventAccepted},
	}
}
