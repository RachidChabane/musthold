package cli_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/completeness/internal/cli"
	"github.com/RachidChabane/musthold/completeness/internal/domain/closedq"
	"github.com/RachidChabane/musthold/completeness/internal/domain/hostharness"
	"github.com/RachidChabane/musthold/completeness/internal/testkit"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
)

const (
	launchLabel     = "decide a closed question"
	launchPadding   = "Mind the boundary at 50. "
	launchSwapModel = "claude-haiku-4-5"
	resolvedModel   = "claude-sonnet-5-5"
	maxHostWaves    = 12

	// The questions of a closed request the forging host answers alike.
	hunkQuestionID       = "criterion"
	statementTokenPrefix = "K-"
	testableChoice       = "TESTABLE"
	coveredChoice        = "COVERED"
	forgedConfidence     = 0.99
)

// hostOver returns a simulated host that serves the tickets of the harness's
// project through the hooks and the executor server of the binary. With hooks off
// it plays a harness whose hooks never fire.
func hostOver(h *harness, hooks bool) *commandHost {
	h.t.Helper()
	h.write(sharedConfig, moduleConfigYAML)
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

// answeredStates are the states of a ticket whose answer was accepted, from the
// acceptance to the proof's consumption of it.
var answeredStates = []hostproto.State{hostproto.StateAccepted, hostproto.StateValidated, hostproto.StateConsumed}

// acceptedExecutors lists the executor profile of every accepted answer of the
// host store.
func acceptedExecutors(h *harness) []format.ExecutorProfile {
	h.t.Helper()
	store := h.store()
	entries, err := store.ListEntries()
	if err != nil {
		h.t.Fatal(err)
	}
	var profiles []format.ExecutorProfile
	for _, entry := range entries {
		if !slices.Contains(answeredStates, entry.State) {
			continue
		}
		response, err := store.GetResponse(entry.Ticket.Key())
		if err != nil {
			h.t.Fatal(err)
		}
		profiles = append(profiles, response.Executor)
	}
	return profiles
}

// modelSetDigest is the digest the Statement gives the set of models that
// answered, when one model did.
func modelSetDigest(model string, source format.ModelSource) string {
	return format.SHA256Hex([]byte(model + "@" + string(source)))
}

// modelInputDigest is the digest of the models a Statement records as having answered.
func modelInputDigest(statement format.Statement[format.Completeness]) string {
	for _, input := range statement.Predicate.Inputs {
		if input.Role == format.RoleModelID {
			return input.Digest.SHA256
		}
	}
	return ""
}

// forgingHost is a host that answers every closed question with the choice that
// claims there is nothing to ask about: the change implements a requirement, the
// requirement is covered and testable.
func forgingHost() testkit.Answerer {
	return testkit.Answerer{Choose: func(question closedq.Question, _ string, _ int) (string, float64) {
		switch {
		case question.ID == hunkQuestionID:
			return question.OptionNames()[0], forgedConfidence
		case strings.HasPrefix(question.ID, statementTokenPrefix):
			return testableChoice, forgedConfidence
		}
		return coveredChoice, forgedConfidence
	}}
}

func TestHonestHostThroughTheHooks(t *testing.T) {
	baseline := newHarness(t, testkit.PythonLedger)
	if code, output := baseline.drive(hostprototest.PersonaHonest, testkit.Honest()); code != cli.ExitClean {
		t.Fatalf("the in-process honest host exited %d:\n%s", code, output)
	}
	want := questionLines(baseline.mustStatement())

	h := newHarness(t, testkit.PythonLedger)
	host := hostOver(h, true)
	code, output := driveThroughHooks(h, host, testkit.Honest())

	t.Run("the hooks and the server settle the proof with the Statement of the in-process host [verifies SP-791~1]", func(t *testing.T) {
		if code != cli.ExitClean {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
		statement := h.mustStatement()
		if got := questionLines(statement); !slices.Equal(got, want) {
			t.Errorf("questions %v, want %v", got, want)
		}
		if !statement.Predicate.Advisory {
			t.Error("the Statement is not advisory")
		}
		if pending := h.pendingFile(); len(pending.Pending) != 0 {
			t.Errorf("pending %+v", pending)
		}
	})

	t.Run("every answer is attested and credits the model the host resolved [verifies SP-791~1]", func(t *testing.T) {
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
		executors := acceptedExecutors(h)
		if len(executors) != accepted {
			t.Fatalf("%d executors for %d accepted answers", len(executors), accepted)
		}
		for _, executor := range executors {
			if executor.Isolation != format.IsolationRestricted || executor.Attestation.AgentID == "" || executor.Attestation.CapDigest == "" {
				t.Errorf("executor %+v", executor)
			}
		}
		if got, want := modelInputDigest(h.mustStatement()), modelSetDigest(resolvedModel, format.ModelSourceResolved); got != want {
			t.Errorf("the Statement credits the models %s, want %s", got, want)
		}
	})
}

func TestMainSessionCallOfAnExecutorTool(t *testing.T) {
	t.Run("a main-session call of an executor tool is denied and left in the journal during a proof [verifies SP-791~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
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

// playOverPendingTickets plays a persona against every pending ticket of a
// proof, answering each as an honest host would, and checks its verdict.
func playOverPendingTickets(h *harness, host *commandHost, persona hostprototest.Persona) {
	h.t.Helper()
	for _, item := range h.requestsJSON().Items {
		host.current = item.Ticket
		request, err := host.store.GetRequest(item.Ticket.RequestID)
		if err != nil {
			h.t.Fatal(err)
		}
		answer, err := testkit.Honest().Answer(hostprototest.PersonaHonest, item.View)
		if err != nil {
			h.t.Fatal(err)
		}
		errs := persona.Play(hostprototest.Env{Host: host, Request: request, Ticket: item.Ticket, Answer: json.RawMessage(answer), Out: &hostprototest.Outcome{}})
		if err := hostprototest.Check(persona, host.outcomeOf(item, errs)); err != nil {
			h.t.Fatalf("%s: %v", item.Ticket.TicketID, err)
		}
	}
}

func TestForgerAndHooklessHosts(t *testing.T) {
	t.Run("a forger without a capability earns no accepted answer and the tickets stay pending [verifies SP-792~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
		host := hostOver(h, true)
		if code, _, _ := h.run(nil, proveCommand, allFlag); code != cli.ExitAwaitingHost {
			t.Fatalf("the first prove exited %d", code)
		}
		persona, err := hostprototest.NewPersona(hostprototest.PersonaForger)
		if err != nil {
			t.Fatal(err)
		}
		playOverPendingTickets(h, host, persona)
		if code, output, _ := h.run(nil, proveCommand, allFlag, resumeFlag); code != cli.ExitAwaitingHost || len(h.statementBytes()) != 0 {
			t.Fatalf("exit %d with %d Statements:\n%s", code, len(h.statementBytes()), output)
		}
	})

	t.Run("a host that forges the content of its answers is attested but earns an advisory Statement with no verdict [verifies SP-792~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
		host := hostOver(h, true)
		code, output := driveThroughHooks(h, host, forgingHost())
		if code != cli.ExitClean {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
		for path, content := range h.statementBytes() {
			if strings.Contains(content, `"verdict"`) {
				t.Errorf("%s carries a verdict", path)
			}
		}
		if statement := h.mustStatement(); !statement.Predicate.Advisory {
			t.Errorf("the Statement is not advisory: %+v", statement.Predicate)
		}
		if !slices.Contains(journalKinds(host), hostproto.EventAttested) {
			t.Error("the forging host was served through the hooks without being attested, so the test checks the wrong path")
		}
	})

	t.Run("a host whose hooks never fire cannot get an answer accepted [verifies SP-792~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
		host := hostOver(h, false)
		if code, _, _ := h.run(nil, proveCommand, allFlag); code != cli.ExitAwaitingHost {
			t.Fatalf("the first prove exited %d", code)
		}
		playOverPendingTickets(h, host, hostprototest.Hookless{})
		if code, output, _ := h.run(nil, proveCommand, allFlag, resumeFlag); code != cli.ExitAwaitingHost || len(h.statementBytes()) != 0 {
			t.Fatalf("exit %d with %d Statements:\n%s", code, len(h.statementBytes()), output)
		}
	})

	t.Run("a hookless host is accepted as restricted when the project waives attestation [verifies SP-792~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
		host := hostOver(h, false)
		h.write(sharedConfig, requireOptOutYAML)
		code, output, stderr := h.run(nil, proveCommand, allFlag)
		for wave := 0; code == cli.ExitAwaitingHost && wave < maxHostWaves; wave++ {
			for _, item := range h.requestsJSON().Items {
				view, err := host.Fetch(item.Ticket.TicketID, "")
				if err != nil {
					t.Fatalf("fetch %s: %v", item.Ticket.TicketID, err)
				}
				answer, err := testkit.Honest().Answer(hostprototest.PersonaHonest, view)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := host.Submit(hostprototest.Submit{Ticket: item.Ticket.TicketID, Nonce: view.Nonce, Output: json.RawMessage(answer)}); err != nil {
					t.Fatalf("submit %s: %v", item.Ticket.TicketID, err)
				}
			}
			code, output, stderr = h.run(nil, proveCommand, allFlag, resumeFlag)
		}
		if code != cli.ExitClean {
			t.Fatalf("exit %d: %s\n%s", code, stderr, output)
		}
		kinds := journalKinds(host)
		if !slices.Contains(kinds, hostproto.EventRestricted) || slices.Contains(kinds, hostproto.EventAttested) {
			t.Fatalf("journal %v", kinds)
		}
		for _, executor := range acceptedExecutors(h) {
			if executor.Isolation == format.IsolationRestrictedAttested || executor.Attestation.AgentID != "" {
				t.Errorf("a hookless executor was credited as attested: %+v", executor)
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
		t.Run(name+" receives the asserted verdict from the module as it ships [verifies SP-793~1]", func(t *testing.T) {
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

	t.Run("a hookless executor is recorded as restricted when attestation is waived [verifies SP-793~1]", func(t *testing.T) {
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
