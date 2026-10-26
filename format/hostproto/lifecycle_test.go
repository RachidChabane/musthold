package hostproto_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
)

var epoch = hostprototest.Epoch

// legalTransitions is the table of ADR 0005 as the test reads it: the only moves
// the lifecycle allows.
var legalTransitions = map[hostproto.State][]hostproto.State{
	hostproto.StateEmitted:    {hostproto.StateFetched, hostproto.StateSuperseded},
	hostproto.StateFetched:    {hostproto.StateAccepted, hostproto.StateExpired, hostproto.StateRejected},
	hostproto.StateAccepted:   {hostproto.StateValidated, hostproto.StateRejected},
	hostproto.StateRejected:   {hostproto.StateEmitted, hostproto.StateAbandoned},
	hostproto.StateExpired:    {hostproto.StateEmitted, hostproto.StateAbandoned},
	hostproto.StateValidated:  {hostproto.StateConsumed},
	hostproto.StateSuperseded: {hostproto.StateEmitted},
}

func freshEntry() hostproto.Entry {
	request := hostprototest.Request()
	return hostproto.NewEntry(hostprototest.Ticket(request, 0, hostproto.FirstEmission), request.Role, hostprototest.Digest("schema"), hostproto.DefaultMaxEmissions)
}

// entryIn drives a fresh entry to the state through legal calls.
func entryIn(t fataler, state hostproto.State) hostproto.Entry {
	t.Helper()
	entry := freshEntry()
	steps := map[hostproto.State][]func(hostproto.Entry) (hostproto.Entry, error){
		hostproto.StateEmitted:    {},
		hostproto.StateSuperseded: {hostproto.Entry.Supersede},
		hostproto.StateFetched:    {fetchStep},
		hostproto.StateExpired:    {fetchStep, expireStep},
		hostproto.StateAccepted:   {fetchStep, acceptStep},
		hostproto.StateRejected:   {fetchStep, acceptStep, hostproto.Entry.Contradict},
		hostproto.StateValidated:  {fetchStep, acceptStep, hostproto.Entry.MarkValidated},
		hostproto.StateConsumed:   {fetchStep, acceptStep, hostproto.Entry.MarkValidated, hostproto.Entry.Consume},
		hostproto.StateAbandoned:  {fetchStep, acceptStep, hostproto.Entry.Contradict, abandonStep},
	}
	for _, step := range steps[state] {
		var err error
		if entry, err = step(entry); err != nil {
			t.Fatal(err)
		}
	}
	if entry.State != state {
		t.Fatalf("entry is %s, want %s", entry.State, state)
	}
	return entry
}

func fetchStep(e hostproto.Entry) (hostproto.Entry, error) {
	return e.Fetch(hostprototest.Digest("cap"), epoch)
}
func acceptStep(e hostproto.Entry) (hostproto.Entry, error) {
	return e.Accept(hostprototest.Digest("answer"))
}
func expireStep(e hostproto.Entry) (hostproto.Entry, error) {
	return e.Expire(epoch.Add(hostproto.FetchTTL))
}
func abandonStep(e hostproto.Entry) (hostproto.Entry, error) {
	e.Ticket.Emission = e.MaxEmissions
	return e.Reissue(strings.Repeat("a", hostproto.NonceHexLength))
}

func TestTransitionTable(t *testing.T) {
	t.Run("the table allows exactly the legal transitions and no other [verifies SP-120~2]", func(t *testing.T) {
		for _, from := range hostproto.States() {
			for _, to := range hostproto.States() {
				want := slices.Contains(legalTransitions[from], to)
				if got := hostproto.Legal(from, to); got != want {
					t.Errorf("Legal(%s, %s) = %v, want %v", from, to, got, want)
				}
			}
		}
	})

	t.Run("the states are the nine of the lifecycle and the terminal ones have no way out [verifies SP-120~2]", func(t *testing.T) {
		if len(hostproto.States()) != 9 {
			t.Fatalf("states = %v", hostproto.States())
		}
		for _, state := range hostproto.States() {
			if hostproto.Terminal(state) != (len(legalTransitions[state]) == 0) {
				t.Errorf("Terminal(%s) = %v", state, hostproto.Terminal(state))
			}
		}
		for _, terminal := range []hostproto.State{hostproto.StateConsumed, hostproto.StateAbandoned} {
			if !hostproto.Terminal(terminal) {
				t.Errorf("%s is not terminal", terminal)
			}
		}
		if hostproto.Terminal(hostproto.StateSuperseded) {
			t.Error("a superseded ticket must be able to come back as a new emission")
		}
	})

	methods := map[string]struct {
		to    hostproto.State
		apply func(hostproto.Entry) (hostproto.Entry, error)
	}{
		"Fetch":         {hostproto.StateFetched, fetchStep},
		"Accept":        {hostproto.StateAccepted, acceptStep},
		"Contradict":    {hostproto.StateRejected, hostproto.Entry.Contradict},
		"MarkValidated": {hostproto.StateValidated, hostproto.Entry.MarkValidated},
		"Consume":       {hostproto.StateConsumed, hostproto.Entry.Consume},
		"Supersede":     {hostproto.StateSuperseded, hostproto.Entry.Supersede},
		"Expire":        {hostproto.StateExpired, expireStep},
	}
	for name, method := range methods {
		t.Run(name+" succeeds exactly from the states the table allows and leaves the entry alone otherwise [verifies SP-120~2]", func(t *testing.T) {
			for _, from := range hostproto.States() {
				entry := entryIn(t, from)
				next, err := method.apply(entry)
				legal := hostproto.Legal(from, method.to)
				// Fetch from fetched and Accept of the accepted answer are idempotent replays.
				replay := (name == "Fetch" && from == hostproto.StateFetched) || (name == "Accept" && slices.Contains([]hostproto.State{hostproto.StateAccepted, hostproto.StateValidated, hostproto.StateConsumed}, from))
				switch {
				case legal && err != nil:
					t.Errorf("%s from %s failed: %v", name, from, err)
				case !legal && !replay && !errors.Is(err, hostproto.ErrIllegalTransition):
					t.Errorf("%s from %s: error = %v, want an illegal transition", name, from, err)
				case err != nil && next != entry:
					t.Errorf("%s from %s changed the entry although it failed", name, from)
				}
			}
		})
	}
}

func TestLifecycleGuards(t *testing.T) {
	t.Run("fetching again with the same capability changes nothing, with another is refused [verifies SP-121~2]", func(t *testing.T) {
		fetched := entryIn(t, hostproto.StateFetched)
		again, err := fetched.Fetch(hostprototest.Digest("cap"), epoch.Add(time.Hour))
		if err != nil || again != fetched {
			t.Fatalf("again = %+v, %v", again, err)
		}
		if _, err := fetched.Fetch(hostprototest.Digest("other cap"), epoch); !errors.Is(err, hostproto.ErrCapBoundElsewhere) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("a schema failure allows one correction and then rejects [verifies SP-121~2]", func(t *testing.T) {
		entry := entryIn(t, hostproto.StateFetched)
		first, allowed, err := entry.FailSchema()
		if err != nil || !allowed || first.State != hostproto.StateFetched || first.Corrections != 1 {
			t.Fatalf("first failure: %+v %v %v", first, allowed, err)
		}
		second, allowed, err := first.FailSchema()
		if err != nil || allowed || second.State != hostproto.StateRejected {
			t.Fatalf("second failure: %+v %v %v", second, allowed, err)
		}
		if _, _, err := entryIn(t, hostproto.StateEmitted).FailSchema(); !errors.Is(err, hostproto.ErrIllegalTransition) {
			t.Fatalf("a failure before the fetch: %v", err)
		}
	})

	t.Run("a fetched ticket expires only after the time to live [verifies SP-121~2]", func(t *testing.T) {
		fetched := entryIn(t, hostproto.StateFetched)
		if _, err := fetched.Expire(epoch.Add(hostproto.FetchTTL - time.Nanosecond)); !errors.Is(err, hostproto.ErrNotExpired) {
			t.Fatalf("error = %v", err)
		}
		expired, err := fetched.Expire(epoch.Add(hostproto.FetchTTL))
		if err != nil || expired.State != hostproto.StateExpired {
			t.Fatalf("expired = %+v, %v", expired, err)
		}
		if hostproto.FetchTTL != 20*time.Minute {
			t.Fatalf("FetchTTL = %v", hostproto.FetchTTL)
		}
	})

	t.Run("a rejected or expired ticket is reissued with a new nonce while emissions remain [verifies SP-121~2]", func(t *testing.T) {
		nonce := strings.Repeat("b", hostproto.NonceHexLength)
		for _, state := range []hostproto.State{hostproto.StateRejected, hostproto.StateExpired} {
			entry := entryIn(t, state)
			next, err := entry.Reissue(nonce)
			if err != nil {
				t.Fatal(err)
			}
			if next.State != hostproto.StateEmitted || next.Ticket.Emission != entry.Ticket.Emission+1 || next.Ticket.Nonce != nonce ||
				next.Corrections != 0 || next.CapDigest != "" || next.AcceptedOutput != "" || !next.FetchedAt.IsZero() {
				t.Errorf("%s reissued as %+v", state, next)
			}
			if next.Ticket.RequestID != entry.Ticket.RequestID || next.Ticket.Vote != entry.Ticket.Vote {
				t.Errorf("the reissue changed the request or the vote")
			}
		}
	})

	t.Run("a ticket is abandoned when its emissions are exhausted [verifies SP-121~2]", func(t *testing.T) {
		abandoned := entryIn(t, hostproto.StateAbandoned)
		if abandoned.Reason != hostproto.ReasonHostUnresponsive || abandoned.Ticket.Emission != hostproto.DefaultMaxEmissions {
			t.Fatalf("abandoned = %+v", abandoned)
		}
		if hostproto.DefaultMaxEmissions != 3 {
			t.Fatalf("DefaultMaxEmissions = %d", hostproto.DefaultMaxEmissions)
		}
		for _, state := range []hostproto.State{hostproto.StateEmitted, hostproto.StateFetched, hostproto.StateAccepted, hostproto.StateValidated} {
			if _, err := entryIn(t, state).Reissue("n"); !errors.Is(err, hostproto.ErrIllegalTransition) {
				t.Errorf("reissue from %s: %v", state, err)
			}
		}
	})

	t.Run("a superseded ticket is re-emitted with a new nonce and the next emission number, and only a superseded one [verifies SP-2149~1]", func(t *testing.T) {
		nonce := strings.Repeat("e", hostproto.NonceHexLength)
		superseded := entryIn(t, hostproto.StateSuperseded)
		next, err := superseded.Reemit(nonce)
		if err != nil {
			t.Fatal(err)
		}
		if next.State != hostproto.StateEmitted || next.Ticket.Emission != superseded.Ticket.Emission+1 || next.Ticket.Nonce != nonce || next.Reemissions != 1 {
			t.Fatalf("re-emitted as %+v", next)
		}
		for _, state := range hostproto.States() {
			if state == hostproto.StateSuperseded {
				continue
			}
			entry := entryIn(t, state)
			if again, err := entry.Reemit(nonce); !errors.Is(err, hostproto.ErrIllegalTransition) || again != entry {
				t.Errorf("re-emission from %s: %+v, %v", state, again, err)
			}
		}
	})

	t.Run("re-emissions after supersessions never count toward abandonment, host failures still do [verifies SP-2149~1] [verifies SP-121~2]", func(t *testing.T) {
		nonce := strings.Repeat("f", hostproto.NonceHexLength)
		entry := freshEntry()
		for range hostproto.DefaultMaxEmissions * 2 {
			superseded, err := entry.Supersede()
			if err != nil {
				t.Fatal(err)
			}
			if entry, err = superseded.Reemit(nonce); err != nil {
				t.Fatal(err)
			}
		}
		for failure := range hostproto.DefaultMaxEmissions {
			rejected, err := fetchStep(entry)
			if err == nil {
				rejected, err = acceptStep(rejected)
			}
			if err == nil {
				rejected, err = rejected.Contradict()
			}
			if err != nil {
				t.Fatal(err)
			}
			if entry, err = rejected.Reissue(nonce); err != nil {
				t.Fatal(err)
			}
			wantAbandoned := failure == hostproto.DefaultMaxEmissions-1
			if (entry.State == hostproto.StateAbandoned) != wantAbandoned {
				t.Fatalf("after host failure %d the ticket is %s", failure+1, entry.State)
			}
		}
		if entry.Reason != hostproto.ReasonHostUnresponsive {
			t.Fatalf("reason %q", entry.Reason)
		}
	})

	t.Run("only a validated answer is consumable [verifies SP-121~2]", func(t *testing.T) {
		for _, state := range hostproto.States() {
			if got := entryIn(t, state).Consumable(); got != (state == hostproto.StateValidated) {
				t.Errorf("Consumable in %s = %v", state, got)
			}
		}
	})

	t.Run("a stateful queue never consumes an unvalidated answer and brings a superseded one back only as a new emission [verifies SP-121~2] [verifies SP-2149~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			entry := freshEntry()
			var wasValidated bool
			actions := map[string]func(hostproto.Entry) (hostproto.Entry, error){
				"fetch": fetchStep, "accept": acceptStep, "contradict": hostproto.Entry.Contradict,
				"validate": hostproto.Entry.MarkValidated, "consume": hostproto.Entry.Consume,
				"supersede": hostproto.Entry.Supersede,
				"expire":    func(e hostproto.Entry) (hostproto.Entry, error) { return e.Expire(epoch.Add(2 * hostproto.FetchTTL)) },
				"reissue": func(e hostproto.Entry) (hostproto.Entry, error) {
					return e.Reissue(strings.Repeat("c", hostproto.NonceHexLength))
				},
				"reemit": func(e hostproto.Entry) (hostproto.Entry, error) {
					return e.Reemit(strings.Repeat("d", hostproto.NonceHexLength))
				},
				"schema": func(e hostproto.Entry) (hostproto.Entry, error) {
					next, _, err := e.FailSchema()
					return next, err
				},
			}
			names := make([]string, 0, len(actions))
			for name := range actions {
				names = append(names, name)
			}
			slices.Sort(names)
			t.Repeat(map[string]func(*rapid.T){
				"": func(t *rapid.T) {
					name := rapid.SampledFrom(names).Draw(t, "action")
					before := entry
					next, err := actions[name](entry)
					if err != nil && next != before {
						t.Fatalf("%s failed but changed the entry", name)
					}
					if err == nil && next.State != before.State && !hostproto.Legal(before.State, next.State) {
						t.Fatalf("%s moved %s to %s", name, before.State, next.State)
					}
					if name == "consume" && err == nil && !wasValidated {
						t.Fatal("an unvalidated answer was consumed")
					}
					wasValidated = wasValidated || next.State == hostproto.StateValidated
					if next.State == hostproto.StateRejected || next.State == hostproto.StateEmitted && next.Ticket.Emission > before.Ticket.Emission {
						wasValidated = false
					}
					if before.State == hostproto.StateSuperseded && next.State != hostproto.StateSuperseded &&
						(name != "reemit" || next.Ticket.Emission != before.Ticket.Emission+1 || next.Reemissions != before.Reemissions+1) {
						t.Fatalf("%s brought a superseded entry back as %+v", name, next)
					}
					if next.State == hostproto.StateAbandoned && next.Ticket.Emission-next.Reemissions < next.MaxEmissions {
						t.Fatalf("abandoned after %d emissions for the host", next.Ticket.Emission-next.Reemissions)
					}
					entry = next
				},
			})
		})
	})
}

func TestResubmission(t *testing.T) {
	a, b := hostprototest.Digest("a"), hostprototest.Digest("b")

	t.Run("a second, different output for an accepted ticket is a conflict [verifies SP-124~1]", func(t *testing.T) {
		cases := []struct {
			accepted, incoming string
			want               hostproto.ResubmissionVerdict
		}{
			{"", a, hostproto.ResubmissionFirst},
			{a, a, hostproto.ResubmissionIdentical},
			{a, b, hostproto.ResubmissionConflict},
		}
		for _, c := range cases {
			if got := hostproto.ClassifyResubmission(c.accepted, c.incoming); got != c.want {
				t.Errorf("Classify(%q, %q) = %s, want %s", c.accepted, c.incoming, got, c.want)
			}
		}
	})

	t.Run("the entry replays the same answer and refuses another [verifies SP-124~1]", func(t *testing.T) {
		accepted := entryIn(t, hostproto.StateAccepted)
		if same, err := accepted.Accept(hostprototest.Digest("answer")); !errors.Is(err, hostproto.ErrAlreadyAccepted) || same != accepted {
			t.Fatalf("replay: %+v, %v", same, err)
		}
		if other, err := accepted.Accept(b); !errors.Is(err, hostproto.ErrResubmissionConflict) || other != accepted {
			t.Fatalf("conflict: %+v, %v", other, err)
		}
	})

	t.Run("a conflicting resubmission is journaled by the host and the first answer stands [verifies SP-124~1]", func(t *testing.T) {
		host := hostprototest.NewReferenceHost(hostprototest.ClaudeCodeObserved(), hostprototest.RequireAttestation())
		request := hostprototest.Request()
		ticket, err := host.Emit(request)
		if err != nil {
			t.Fatal(err)
		}
		agent := host.StartExecutor(request.Role)
		capability := host.Cap(agent)
		if _, err := host.Fetch(ticket.TicketID, capability); err != nil {
			t.Fatal(err)
		}
		submit := func(output string) error {
			_, err := host.Submit(hostprototest.Submit{Ticket: ticket.TicketID, Nonce: ticket.Nonce, Cap: capability, Output: []byte(output)})
			return err
		}
		if err := submit(`{"validity":"VALID"}`); err != nil {
			t.Fatal(err)
		}
		if err := submit(`{ "validity" : "VALID" }`); err != nil {
			t.Fatalf("an identical answer in other spacing: %v", err)
		}
		if err := submit(`{"validity":"EQUIVALENT"}`); !errors.Is(err, hostproto.ErrResubmissionConflict) {
			t.Fatalf("error = %v", err)
		}
		journaled := slices.ContainsFunc(host.Journal(), func(e hostproto.Event) bool { return e.Kind == hostproto.EventResubmissionConflict })
		if !journaled {
			t.Fatal("the conflict was not journaled")
		}
	})
}
