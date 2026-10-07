package hostproto_test

import (
	"errors"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const testAgent = "agent-0001"

type roster map[string]string

func (r roster) Started(agentID, role string) bool { return r[agentID] == role }

func TestCapabilities(t *testing.T) {
	secret := hostprototest.Secret()

	t.Run("a capability verifies under its secret and names its agent [verifies SP-123~1]", func(t *testing.T) {
		capability, err := hostproto.FormatCap(secret, testAgent)
		if err != nil {
			t.Fatal(err)
		}
		agent, err := hostproto.VerifyCap(secret, capability)
		if err != nil || agent != testAgent {
			t.Fatalf("agent = %q, %v", agent, err)
		}
	})

	t.Run("a capability is the HMAC-SHA256 of the agent identifier [verifies SP-123~1]", func(t *testing.T) {
		capability, _ := hostproto.FormatCap([]byte("key"), "a")
		const want = "a.780c3db4ce3de5b9e55816fba98f590631d96c075271b26976238d5f4444219b"
		if capability != want {
			t.Fatalf("capability = %q", capability)
		}
	})

	t.Run("another secret, agent or MAC is refused [verifies SP-123~1]", func(t *testing.T) {
		capability, _ := hostproto.FormatCap(secret, testAgent)
		other, _ := hostproto.FormatCap(secret, "agent-0002")
		macOfOther := other[strings.LastIndex(other, ".")+1:]
		cases := map[string]string{
			"another secret":    mustCap(t, []byte("other"), testAgent),
			"another agent":     testAgent + "x" + capability[len(testAgent):],
			"a swapped MAC":     testAgent + "." + macOfOther,
			"an upper-case MAC": testAgent + "." + strings.ToUpper(capability[len(testAgent)+1:]),
			"no separator":      "agent0001",
			"no agent":          "." + capability[len(testAgent)+1:],
			"no MAC":            testAgent + ".",
			"a MAC not hex":     testAgent + ".zz",
			"empty":             "",
		}
		for name, presented := range cases {
			if _, err := hostproto.VerifyCap(secret, presented); !errors.Is(err, hostproto.ErrBadCap) {
				t.Errorf("%s: error = %v", name, err)
			}
		}
	})

	t.Run("an agent identifier that cannot be told from its MAC is refused [verifies SP-123~1]", func(t *testing.T) {
		for _, agent := range []string{"", "a.b"} {
			if _, err := hostproto.FormatCap(secret, agent); !errors.Is(err, hostproto.ErrBadCap) {
				t.Errorf("%q: error = %v", agent, err)
			}
		}
	})

	t.Run("flipping any character of a capability is always rejected [verifies SP-123~1]", func(t *testing.T) {
		capability := mustCap(t, secret, testAgent)
		rapid.Check(t, func(t *rapid.T) {
			index := rapid.IntRange(0, len(capability)-1).Draw(t, "index")
			replacement := rapid.SampledFrom([]byte("0123456789abcdefg.-")).Draw(t, "replacement")
			if capability[index] == replacement {
				t.Skip("no change")
			}
			tampered := capability[:index] + string(replacement) + capability[index+1:]
			if agent, err := hostproto.VerifyCap(secret, tampered); err == nil && agent == testAgent {
				t.Fatalf("%q verified as %q", tampered, agent)
			}
		})
	})

	t.Run("the cap digest is the SHA-256 of the capability [verifies SP-123~1]", func(t *testing.T) {
		digest := hostproto.CapDigest("cap")
		if len(digest) != 64 || digest != hostproto.CapDigest("cap") || digest == hostproto.CapDigest("cap2") {
			t.Fatalf("digest = %q", digest)
		}
	})
}

func mustCap(t fataler, secret []byte, agent string) string {
	t.Helper()
	capability, err := hostproto.FormatCap(secret, agent)
	if err != nil {
		t.Fatal(err)
	}
	return capability
}

// bound is a fetched entry with everything a submission needs.
type bound struct {
	entry      hostproto.Entry
	schema     string
	submission hostproto.Submission
	verifier   hostproto.CapVerifier
}

func newBound(t fataler) bound {
	t.Helper()
	request := hostprototest.Request()
	ticket := hostprototest.Ticket(request, 0, hostproto.FirstEmission)
	schema, err := request.SchemaDigest()
	if err != nil {
		t.Fatal(err)
	}
	capability := mustCap(t, hostprototest.Secret(), testAgent)
	entry, err := hostproto.NewEntry(ticket, request.Role, schema, hostproto.DefaultMaxEmissions).Fetch(hostproto.CapDigest(capability), epoch)
	if err != nil {
		t.Fatal(err)
	}
	return bound{
		entry: entry, schema: schema,
		submission: hostproto.Submission{Ticket: ticket.TicketID, Nonce: ticket.Nonce, Cap: capability},
		verifier:   hostproto.CapVerifier{Secret: hostprototest.Secret(), Roster: roster{testAgent: request.Role}, RequireAttestation: true},
	}
}

func (b bound) check() (hostproto.Binding, error) {
	return hostproto.CheckBinding(b.entry, b.schema, b.submission, b.verifier)
}

func TestBinding(t *testing.T) {
	t.Run("a submission bound to its ticket, nonce, schema, state and capability is accepted [verifies SP-122~1]", func(t *testing.T) {
		binding, err := newBound(t).check()
		if err != nil || !binding.Attested || binding.AgentID != testAgent || binding.CapDigest != hostproto.CapDigest(newBound(t).submission.Cap) {
			t.Fatalf("binding = %+v, %v", binding, err)
		}
	})

	cases := []struct {
		name   string
		tamper func(*bound)
		want   error
	}{
		{"another request", func(b *bound) { b.submission.Ticket.RequestID = hostproto.RequestID(strings.Repeat("0", 64)) }, hostproto.ErrTicketMismatch},
		{"another vote", func(b *bound) { b.submission.Ticket.Vote++ }, hostproto.ErrTicketMismatch},
		{"another emission", func(b *bound) { b.submission.Ticket.Emission++ }, hostproto.ErrTicketMismatch},
		{"another nonce", func(b *bound) { b.submission.Nonce = strings.Repeat("0", hostproto.NonceHexLength) }, hostproto.ErrNonceMismatch},
		{"no nonce", func(b *bound) { b.submission.Nonce = "" }, hostproto.ErrNonceMismatch},
		{"a changed schema digest", func(b *bound) { b.schema = hostprototest.Digest("another schema") }, hostproto.ErrSchemaDigestMismatch},
		{"a forged capability", func(b *bound) { b.submission.Cap = mustCap(t, []byte("forged"), testAgent) }, hostproto.ErrBadCap},
		{"no capability", func(b *bound) { b.submission.Cap = "" }, hostproto.ErrCapMissing},
		{"an agent that was never started", func(b *bound) { b.verifier.Roster = roster{} }, hostproto.ErrAgentNotStarted},
		{"an agent started for another role", func(b *bound) { b.verifier.Roster = roster{testAgent: "spec-proof-dp:generator"} }, hostproto.ErrAgentNotStarted},
		{"no roster", func(b *bound) { b.verifier.Roster = nil }, hostproto.ErrAgentNotStarted},
		{"a capability bound to another ticket", func(b *bound) { b.entry.CapDigest = hostprototest.Digest("another cap") }, hostproto.ErrCapBoundElsewhere},
	}
	for _, c := range cases {
		t.Run("a submission with "+c.name+" is rejected [verifies SP-122~1]", func(t *testing.T) {
			b := newBound(t)
			c.tamper(&b)
			if _, err := b.check(); !errors.Is(err, c.want) {
				t.Fatalf("error = %v, want %v", err, c.want)
			}
		})
	}

	t.Run("a ticket in a state that takes no answer is rejected [verifies SP-122~1]", func(t *testing.T) {
		for _, state := range hostproto.States() {
			b := newBound(t)
			b.entry.State = state
			_, err := b.check()
			takes := state == hostproto.StateFetched || state == hostproto.StateAccepted
			if takes != (err == nil) || (!takes && !errors.Is(err, hostproto.ErrNotSubmittable)) {
				t.Errorf("state %s: error = %v", state, err)
			}
		}
	})

	t.Run("an absent capability is accepted only when attestation is not required [verifies SP-122~1]", func(t *testing.T) {
		b := newBound(t)
		b.submission.Cap = ""
		b.verifier.RequireAttestation = false
		binding, err := b.check()
		if err != nil || binding.Attested || binding.AgentID != "" {
			t.Fatalf("binding = %+v, %v", binding, err)
		}
		b.submission.Cap = mustCap(t, []byte("forged"), testAgent)
		if _, err := b.check(); !errors.Is(err, hostproto.ErrBadCap) {
			t.Fatalf("a forged capability was waived: %v", err)
		}
	})

	t.Run("a tampered ticket, nonce, capability or digest is always rejected [verifies SP-122~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			b := newBound(t)
			field := rapid.SampledFrom([]string{"request", "vote", "emission", "nonce", "cap", "schema"}).Draw(t, "field")
			switch field {
			case "request":
				b.submission.Ticket.RequestID = flip(t, b.submission.Ticket.RequestID)
			case "vote":
				b.submission.Ticket.Vote += rapid.IntRange(1, 5).Draw(t, "delta")
			case "emission":
				b.submission.Ticket.Emission += rapid.IntRange(1, 5).Draw(t, "delta")
			case "nonce":
				b.submission.Nonce = flip(t, b.submission.Nonce)
			case "cap":
				b.submission.Cap = flip(t, b.submission.Cap)
			case "schema":
				b.schema = flip(t, b.schema)
			}
			if _, err := b.check(); err == nil {
				t.Fatalf("a submission with a tampered %s was accepted", field)
			}
		})
	})

	t.Run("a fetch is bound to its ticket, state and capability [verifies SP-122~1]", func(t *testing.T) {
		b := newBound(t)
		emitted := hostproto.NewEntry(b.entry.Ticket, b.entry.Role, b.schema, hostproto.DefaultMaxEmissions)
		if _, err := hostproto.CheckFetch(emitted, b.submission.Ticket, b.submission.Cap, b.verifier); err != nil {
			t.Fatal(err)
		}
		other := b.submission.Ticket
		other.Emission++
		if _, err := hostproto.CheckFetch(emitted, other, b.submission.Cap, b.verifier); !errors.Is(err, hostproto.ErrTicketMismatch) {
			t.Errorf("another ticket: %v", err)
		}
		if _, err := hostproto.CheckFetch(emitted, b.submission.Ticket, "", b.verifier); !errors.Is(err, hostproto.ErrCapMissing) {
			t.Errorf("no capability: %v", err)
		}
		consumed := entryIn(t, hostproto.StateConsumed)
		if _, err := hostproto.CheckFetch(consumed, consumed.Ticket.TicketID, b.submission.Cap, b.verifier); !errors.Is(err, hostproto.ErrNotSubmittable) {
			t.Errorf("a consumed ticket: %v", err)
		}
	})
}

// flip changes one character of a string to another character of the same kind.
func flip[S ~string](t *rapid.T, value S) S {
	index := rapid.IntRange(0, len(value)-1).Draw(t, "index")
	replacement := byte('0')
	if value[index] == replacement {
		replacement = '1'
	}
	return value[:index] + S(string(replacement)) + value[index+1:]
}

func TestJournal(t *testing.T) {
	events := []hostproto.Event{
		{Kind: hostproto.EventEmitted, Ticket: "t"},
		{Kind: hostproto.EventAgentStarted, AgentID: testAgent, Role: "spec-proof-dp:judge"},
		{Kind: hostproto.EventAccepted, Ticket: "t"},
	}
	build := func() []hostproto.Record {
		var records []hostproto.Record
		for _, event := range events {
			var head *hostproto.Record
			if len(records) > 0 {
				head = &records[len(records)-1]
			}
			record, err := hostproto.Chain(head, event)
			if err != nil {
				t.Fatal(err)
			}
			records = append(records, record)
		}
		return records
	}

	t.Run("a chain verifies and starts at the genesis digest [verifies SP-126~1]", func(t *testing.T) {
		records := build()
		if err := hostproto.VerifyChain(records); err != nil {
			t.Fatal(err)
		}
		if records[0].Prev != hostproto.GenesisDigest || records[1].Prev != records[0].Digest {
			t.Fatalf("records = %+v", records)
		}
		if err := hostproto.VerifyChain(nil); err != nil {
			t.Fatalf("the empty chain: %v", err)
		}
	})

	t.Run("any edit, removal, reordering or truncation from the front breaks the chain [verifies SP-126~1]", func(t *testing.T) {
		edited := build()
		edited[1].Event.AgentID = "agent-9999"
		reordered := build()
		reordered[0], reordered[1] = reordered[1], reordered[0]
		cases := map[string][]hostproto.Record{
			"an edited event":     edited,
			"a removed record":    append(build()[:1], build()[2:]...),
			"a reordered pair":    reordered,
			"a dropped head":      build()[1:],
			"a forged digest":     func() []hostproto.Record { r := build(); r[2].Digest = hostprototest.Digest("x"); return r }(),
			"a renumbered record": func() []hostproto.Record { r := build(); r[1].Sequence = 5; return r }(),
		}
		for name, records := range cases {
			if err := hostproto.VerifyChain(records); !errors.Is(err, hostproto.ErrChainBroken) {
				t.Errorf("%s: error = %v", name, err)
			}
		}
	})

	t.Run("the roster is read from agent_started events [verifies SP-126~1]", func(t *testing.T) {
		roster := hostproto.JournalRoster(build())
		if !roster.Started(testAgent, "spec-proof-dp:judge") || roster.Started(testAgent, "spec-proof-dp:generator") || roster.Started("agent-0002", "spec-proof-dp:judge") {
			t.Fatal("the roster does not follow the journal")
		}
	})
}

func TestSelectChannel(t *testing.T) {
	enforced := hostproto.Observed{PluginPresent: true, ServerPresent: true, HooksPresent: true, Allowlist: hostproto.AllowlistEnforced}
	with := func(o hostproto.Observed, edit func(*hostproto.Observed)) hostproto.Observed { edit(&o); return o }
	cases := []struct {
		name     string
		observed hostproto.Observed
		policy   hostproto.Policy
		channel  string
		isolate  string
		ready    hostproto.Availability
	}{
		{"a plugin subagent with hooks", enforced, hostproto.Policy{RequireAttestation: true}, "host-subagent", "restricted-attested", hostproto.AvailabilityReady},
		{"hooks missing and attestation required", with(enforced, func(o *hostproto.Observed) { o.HooksPresent = false }), hostproto.Policy{RequireAttestation: true}, "host-subagent", "restricted", hostproto.AvailabilityPending},
		{"hooks missing and attestation waived", with(enforced, func(o *hostproto.Observed) { o.HooksPresent = false }), hostproto.Policy{}, "host-subagent", "restricted", hostproto.AvailabilityReady},
		{"an unverified allowlist", with(enforced, func(o *hostproto.Observed) { o.Allowlist = hostproto.AllowlistUnverified }), hostproto.Policy{}, "host-subagent", "restricted-unverified", hostproto.AvailabilityReady},
		{"no allowlist", with(enforced, func(o *hostproto.Observed) { o.Allowlist = hostproto.AllowlistAbsent }), hostproto.Policy{}, "host-subagent", "unrestricted", hostproto.AvailabilityReady},
		{"an unobserved allowlist", with(enforced, func(o *hostproto.Observed) { o.Allowlist = "" }), hostproto.Policy{}, "host-subagent", "unrestricted", hostproto.AvailabilityReady},
		{"the plugin without its server and sampling on", hostproto.Observed{PluginPresent: true, SamplingDeclared: true}, hostproto.Policy{SamplingEnabled: true}, "host-sampled", "restricted", hostproto.AvailabilityReady},
		{"sampling declared but the policy is off", hostproto.Observed{SamplingDeclared: true}, hostproto.Policy{}, "host-inline", "inline", hostproto.AvailabilityReady},
		{"sampling enabled but not declared", hostproto.Observed{}, hostproto.Policy{SamplingEnabled: true}, "host-inline", "inline", hostproto.AvailabilityReady},
		{"nothing observed", hostproto.Observed{}, hostproto.Policy{RequireAttestation: true}, "host-inline", "inline", hostproto.AvailabilityReady},
		{"the subagent wins over sampling", with(enforced, func(o *hostproto.Observed) { o.SamplingDeclared = true }), hostproto.Policy{SamplingEnabled: true}, "host-subagent", "restricted-attested", hostproto.AvailabilityReady},
	}
	for _, c := range cases {
		t.Run(c.name+" [verifies SP-131~1]", func(t *testing.T) {
			got := hostproto.SelectChannel(c.observed, c.policy)
			if string(got.Channel) != c.channel || string(got.Isolation) != c.isolate || got.Availability != c.ready {
				t.Fatalf("selection = %+v", got)
			}
			if c.ready == hostproto.AvailabilityPending && got.Reason != hostproto.ReasonHooksMissing {
				t.Fatalf("reason = %q", got.Reason)
			}
		})
	}

	t.Run("the selection reads observed state only and is a pure function [verifies SP-131~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			observed := hostproto.Observed{
				PluginPresent: rapid.Bool().Draw(t, "plugin"), ServerPresent: rapid.Bool().Draw(t, "server"),
				HooksPresent: rapid.Bool().Draw(t, "hooks"), SamplingDeclared: rapid.Bool().Draw(t, "sampling"),
				Allowlist: rapid.SampledFrom([]hostproto.ToolAllowlist{hostproto.AllowlistEnforced, hostproto.AllowlistUnverified, hostproto.AllowlistAbsent}).Draw(t, "allowlist"),
			}
			policy := hostproto.Policy{SamplingEnabled: rapid.Bool().Draw(t, "policySampling"), RequireAttestation: rapid.Bool().Draw(t, "require")}
			first, second := hostproto.SelectChannel(observed, policy), hostproto.SelectChannel(observed, policy)
			if first != second {
				t.Fatal("two calls disagree")
			}
			if first.Isolation == "restricted-attested" && !(observed.HooksPresent && observed.Allowlist == hostproto.AllowlistEnforced) {
				t.Fatalf("attested without hooks and an enforced allowlist: %+v", first)
			}
		})
	})
}
