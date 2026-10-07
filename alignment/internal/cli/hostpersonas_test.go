package cli_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/alignment/internal/cli"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/hostharness"
	"github.com/RachidChabane/spec-proof/alignment/internal/testkit"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	launchLabel     = "judge a test"
	launchPadding   = "Mind the boundary at 50. "
	launchSwapModel = "claude-haiku-4-5"
	resolvedModel   = "claude-sonnet-5-5"
	maxHostWaves    = 12
)

// hostOver returns a simulated host that serves the tickets of the harness's
// project through the hooks and the executor server of the binary. With hooks off
// it plays a harness whose hooks never fire.
func hostOver(h *harness, hooks bool) *commandHost {
	h.t.Helper()
	h.write(".spec-proof/config.yaml", moduleConfigYAML)
	return &commandHost{
		t: h.t, root: h.root, state: h.state, harness: hostharness.Claude(), hooks: hooks, now: h.now,
		entropy: &counterEntropy{}, roles: map[string]string{}, store: h.store(), scratch: h.t.TempDir(), askVotes: 1,
	}
}

// serve runs one executor for the ticket the way Claude Code does: the launch goes
// through the rewrite hook, the agent is started, fetches and submits with the
// capability the hook adds, and stops.
func (c *commandHost) serve(item hostproto.ListingItem, answers hostprototest.AnswerSource) {
	c.t.Helper()
	ticket := item.Ticket.TicketID
	c.current = item.Ticket
	launched, err := c.Launch(hostproto.AgentInput{
		SubagentType: item.Role, Description: launchLabel, Model: launchSwapModel, RunInBackground: true,
		Prompt: launchPadding + hostproto.ExecutorPrompt(ticket),
	})
	if err != nil || launched.Model != "" || launched.RunInBackground || launched.Prompt != hostproto.ExecutorPrompt(ticket) {
		c.t.Fatalf("the launch of %s was not rewritten to the bare ticket: %+v (%v)", ticket, launched, err)
	}
	agent := c.StartExecutor(item.Role)
	capability := c.Cap(agent)
	if capability == "" {
		c.t.Fatalf("the hook issued no capability to %s for %s", agent, ticket)
	}
	view, err := c.Fetch(ticket, capability)
	if err != nil {
		c.t.Fatalf("fetch %s: %v", ticket, err)
	}
	answer, err := answers.Answer(hostprototest.PersonaHonest, view)
	if err != nil {
		c.t.Fatal(err)
	}
	receipt, err := c.Submit(hostprototest.Submit{Ticket: ticket, Nonce: view.Nonce, Cap: capability, Output: json.RawMessage(answer)})
	if err != nil {
		c.t.Fatalf("submit %s: %v", ticket, err)
	}
	input := json.RawMessage(ticketInput(ticket))
	if err := c.AfterRun(hostprototest.Run{
		Ticket: ticket, AgentID: agent, Reply: receipt.Text, ResolvedModel: resolvedModel,
		Transcript: []hostproto.ToolCall{
			{Name: c.harness.ToolName(hostproto.ToolFetchRequest), Input: input},
			{Name: c.harness.ToolName(hostproto.ToolSubmitResponse), Input: input},
		},
	}); err != nil {
		c.t.Fatalf("the run of %s deviated: %v", ticket, err)
	}
}

// ticketInput is the input of an executor tool call about the ticket.
func ticketInput(ticket hostproto.TicketID) string {
	encoded, _ := json.Marshal(map[string]string{"ticket": ticket.String()})
	return string(encoded)
}

// driveThroughHooks runs `prove` and lets the host serve the pending tickets
// between waves until `prove` stops waiting or the waves run out.
func driveThroughHooks(h *harness, host *commandHost, answers hostprototest.AnswerSource) (code int, output string) {
	h.t.Helper()
	args := []string{proveCommand, allFlag}
	code, output, stderr := h.run(nil, args...)
	for wave := 0; code == cli.ExitAwaitingHost && wave < maxHostWaves; wave++ {
		for _, item := range h.requestsJSON().Items {
			host.serve(item, answers)
		}
		code, output, stderr = h.run(nil, append(args, resumeFlag)...)
	}
	if code == cli.ExitToolFailure {
		h.t.Fatalf("prove failed: %s", stderr)
	}
	return code, output
}

func journalKinds(host *commandHost) []hostproto.EventKind {
	kinds := []hostproto.EventKind{}
	for _, event := range host.Journal() {
		kinds = append(kinds, event.Kind)
	}
	return kinds
}

func TestHonestHostThroughTheHooks(t *testing.T) {
	h := newHarness(t, testkit.PythonParcel)
	h.withPowers(testkit.PythonPowers())
	host := hostOver(h, true)
	code, output := driveThroughHooks(h, host, testkit.Oracle{})

	t.Run("the hooks and the server settle the proof with the verdicts of the attested host [verifies SP-751~1]", func(t *testing.T) {
		if code != cli.ExitBlocked {
			t.Fatalf("exit code %d, want %d because two tests are MISALIGNED:\n%s", code, cli.ExitBlocked, output)
		}
		got := verdictsOf(h, output)
		for key, want := range parcelVerdicts {
			if got[key] != want {
				t.Errorf("%s: %q, want %q\n%s", key, got[key], want, output)
			}
		}
		if len(h.statements()) != len(parcelVerdicts) {
			t.Errorf("%d Statements for %d tests", len(h.statements()), len(parcelVerdicts))
		}
	})

	t.Run("every answer is attested and credits the model the host resolved [verifies SP-751~1]", func(t *testing.T) {
		kinds := journalKinds(host)
		accepted, attested := 0, 0
		for _, kind := range kinds {
			switch kind {
			case hostproto.EventAccepted:
				accepted++
			case hostproto.EventAttested:
				attested++
			}
		}
		if accepted == 0 || accepted != attested {
			t.Errorf("%d accepted, %d attested", accepted, attested)
		}
		for _, bad := range []hostproto.EventKind{hostproto.EventLeaked, hostproto.EventExposed, hostproto.EventRestricted, hostproto.EventExposureAttempt, hostproto.EventResubmissionConflict} {
			if slices.Contains(kinds, bad) {
				t.Errorf("the honest host journaled %s", bad)
			}
		}
		for key, statement := range h.statements() {
			executor := statement.Predicate.Executor
			if executor.Isolation != format.IsolationRestrictedAttested || executor.Model.Name != resolvedModel || executor.Model.Source != format.ModelSourceResolved {
				t.Errorf("%s: executor %+v", key, executor.ExecutorProfile)
			}
		}
	})
}

func TestMainSessionCallOfAnExecutorTool(t *testing.T) {
	t.Run("a main-session call of an executor tool is denied and left in the journal during a proof [verifies SP-751~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		h.withPowers(testkit.PythonPowers())
		host := hostOver(h, true)
		if code, _, _ := h.run(nil, proveCommand, allFlag); code != cli.ExitAwaitingHost {
			t.Fatalf("the first prove exited %d", code)
		}
		item := h.requestsJSON().Items[0]
		code, stdout, stderr := host.run(executorCallPayload(t, host, item.Ticket.TicketID.String()), "hook", "pre-tool")
		if code != cli.ExitClean || !strings.Contains(stdout, `"permissionDecision":"deny"`) {
			t.Fatalf("exit %d, stdout %s, stderr %s", code, stdout, stderr)
		}
		if !slices.Contains(journalKinds(host), hostproto.EventExposureAttempt) {
			t.Fatal("the attempt was not journaled")
		}
		if entry, err := host.Entry(item.Ticket.Key()); err != nil || entry.State != hostproto.StateEmitted {
			t.Fatalf("entry %+v, %v", entry, err)
		}
	})
}

// outcomeOf describes what a persona left behind, for the verdict check.
func (c *commandHost) outcomeOf(item hostproto.ListingItem, errs []error) hostprototest.Outcome {
	c.t.Helper()
	entry, err := c.Entry(item.Ticket.Key())
	if err != nil {
		c.t.Fatal(err)
	}
	return hostprototest.Outcome{Errors: errs, Ticket: entry.Ticket, Entry: entry, Events: journalKinds(c)}
}

func TestForgerAndHooklessHosts(t *testing.T) {
	t.Run("a forger without a capability earns no accepted answer and the tickets stay pending [verifies SP-752~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		h.withPowers(testkit.PythonPowers())
		host := hostOver(h, true)
		if code, _, _ := h.run(nil, proveCommand, allFlag); code != cli.ExitAwaitingHost {
			t.Fatalf("the first prove exited %d", code)
		}
		persona, err := hostprototest.NewPersona(hostprototest.PersonaForger)
		if err != nil {
			t.Fatal(err)
		}
		items := h.requestsJSON().Items
		for _, item := range items {
			host.current = item.Ticket
			request, err := host.store.GetRequest(item.Ticket.RequestID)
			if err != nil {
				t.Fatal(err)
			}
			answer, err := testkit.Oracle{}.Answer(hostprototest.PersonaHonest, item.View)
			if err != nil {
				t.Fatal(err)
			}
			errs := persona.Play(hostprototest.Env{Host: host, Request: request, Ticket: item.Ticket, Answer: json.RawMessage(answer), Out: &hostprototest.Outcome{}})
			if err := hostprototest.Check(persona, host.outcomeOf(item, errs)); err != nil {
				t.Fatalf("%s: %v", item.Ticket.TicketID, err)
			}
		}
		if code, output, _ := h.run(nil, proveCommand, allFlag, resumeFlag); code != cli.ExitAwaitingHost || len(h.statements()) != 0 {
			t.Fatalf("exit %d with %d Statements:\n%s", code, len(h.statements()), output)
		}
	})

	t.Run("a host that forges the content of its answers is attested but earns no ALIGNED the matrix refutes [verifies SP-752~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		h.withPowers(testkit.PythonPowers())
		host := hostOver(h, true)
		_, output := driveThroughHooks(h, host, testkit.Forger{})
		got := verdictsOf(h, output)
		if got["PAR-1~1 "+testkit.PyRejectedLineLabel] == "ALIGNED" {
			t.Errorf("the INERT test was aligned:\n%s", output)
		}
		for _, wrong := range []string{"PAR-2~1 " + testkit.PyRemoteLabel, "PAR-2~1 " + testkit.PyNumberLabel, "PAR-3~1 " + testkit.PyZeroRejectedLabel} {
			if got[wrong] == "ALIGNED" {
				t.Errorf("%s was aligned by a forger", wrong)
			}
		}
		if len(got) == 0 {
			t.Fatalf("no verdict:\n%s", output)
		}
		if !slices.Contains(journalKinds(host), hostproto.EventAttested) {
			t.Error("the forging host was served through the hooks without being attested, so the test checks the wrong path")
		}
	})

	t.Run("a host whose hooks never fire cannot get an answer accepted [verifies SP-752~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		h.withPowers(testkit.PythonPowers())
		host := hostOver(h, false)
		if code, _, _ := h.run(nil, proveCommand, allFlag); code != cli.ExitAwaitingHost {
			t.Fatalf("the first prove exited %d", code)
		}
		persona := hostprototest.Hookless{}
		for _, item := range h.requestsJSON().Items {
			host.current = item.Ticket
			request, err := host.store.GetRequest(item.Ticket.RequestID)
			if err != nil {
				t.Fatal(err)
			}
			answer, err := testkit.Oracle{}.Answer(hostprototest.PersonaHonest, item.View)
			if err != nil {
				t.Fatal(err)
			}
			errs := persona.Play(hostprototest.Env{Host: host, Request: request, Ticket: item.Ticket, Answer: json.RawMessage(answer), Out: &hostprototest.Outcome{}})
			if err := hostprototest.Check(persona, host.outcomeOf(item, errs)); err != nil {
				t.Fatalf("%s: %v", item.Ticket.TicketID, err)
			}
		}
		if code, output, _ := h.run(nil, proveCommand, allFlag, resumeFlag); code != cli.ExitAwaitingHost || len(h.statements()) != 0 {
			t.Fatalf("exit %d with %d Statements:\n%s", code, len(h.statements()), output)
		}
	})

	t.Run("a hookless host is accepted as restricted when the project waives attestation [verifies SP-752~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		h.withPowers(testkit.PythonPowers())
		host := hostOver(h, false)
		h.write(".spec-proof/config.yaml", requireOptOutYAML)
		code, output, stderr := h.run(nil, proveCommand, allFlag)
		for wave := 0; code == cli.ExitAwaitingHost && wave < maxHostWaves; wave++ {
			for _, item := range h.requestsJSON().Items {
				view, err := host.Fetch(item.Ticket.TicketID, "")
				if err != nil {
					t.Fatalf("fetch %s: %v", item.Ticket.TicketID, err)
				}
				answer, err := testkit.Oracle{}.Answer(hostprototest.PersonaHonest, view)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := host.Submit(hostprototest.Submit{Ticket: item.Ticket.TicketID, Nonce: view.Nonce, Output: json.RawMessage(answer)}); err != nil {
					t.Fatalf("submit %s: %v", item.Ticket.TicketID, err)
				}
			}
			code, output, stderr = h.run(nil, proveCommand, allFlag, resumeFlag)
		}
		if code == cli.ExitAwaitingHost || code == cli.ExitToolFailure {
			t.Fatalf("exit %d: %s\n%s", code, stderr, output)
		}
		kinds := journalKinds(host)
		if !slices.Contains(kinds, hostproto.EventRestricted) || slices.Contains(kinds, hostproto.EventAttested) {
			t.Fatalf("journal %v", kinds)
		}
		for key, statement := range h.statements() {
			if statement.Predicate.Executor.Isolation == format.IsolationRestrictedAttested {
				t.Errorf("%s: a hookless executor was credited as attested", key)
			}
		}
	})
}

func TestPersonasAgainstTheCommandLine(t *testing.T) {
	factory := func(t *testing.T) func(hostproto.Observed, hostproto.Policy) hostprototest.Host {
		return func(observed hostproto.Observed, policy hostproto.Policy) hostprototest.Host {
			return newCommandHost(t, observed, policy)
		}
	}
	for _, name := range hostprototest.PersonaNames() {
		t.Run(name+" receives the asserted verdict from the module as it ships [verifies SP-753~1]", func(t *testing.T) {
			persona, err := hostprototest.NewPersona(name)
			if err != nil {
				t.Fatal(err)
			}
			outcome, err := hostprototest.Drive(persona, factory(t))
			if err != nil {
				t.Fatal(err)
			}
			if err := hostprototest.Check(persona, outcome); err != nil {
				t.Fatalf("%v\noutcome: %+v", err, outcome)
			}
		})
	}

	t.Run("a hookless executor is recorded as restricted when attestation is waived [verifies SP-753~1]", func(t *testing.T) {
		persona := hostprototest.Hookless{OptOut: true}
		outcome, err := hostprototest.Drive(persona, factory(t))
		if err != nil {
			t.Fatal(err)
		}
		if err := hostprototest.Check(persona, outcome); err != nil {
			t.Fatalf("%v\noutcome: %+v", err, outcome)
		}
	})
}
