package hostproto_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"slices"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
)

const (
	verifiesOutstanding = " [verifies SP-2661~1]"
	verifiesWaveOpening = " [verifies SP-2663~1]"
	verifiesProgress    = " [verifies SP-2664~1]"
	verifiesRunLock     = " [verifies SP-2665~1]"
	verifiesStepBudget  = " [verifies SP-2666~1]"

	progressContractFile = "testdata/progress.contract.json"
	stopAgent            = "agent-7"
	drawnScanKey         = "drawn"
)

// drawnStates are the states a generated ticket is drawn in: those it waits for
// its answer in and those it does not.
var drawnStates = []hostproto.State{
	hostproto.StateEmitted, hostproto.StateFetched, hostproto.StateAccepted, hostproto.StateValidated,
	hostproto.StateConsumed, hostproto.StateRejected, hostproto.StateExpired, hostproto.StateSuperseded, hostproto.StateAbandoned,
}

// drawnStore is a generated host store: entries in random states held by random
// sessions, with random stop and fetch events in the journal.
type drawnStore struct {
	entries []hostproto.Entry
	items   []hostproto.QueueItem
	records []hostproto.Record
}

func drawStore(t *rapid.T) drawnStore {
	var store drawnStore
	sessions := []hostproto.Session{hostproto.SessionProve, hostproto.ScanSession(drawnScanKey)}
	stopKinds := []hostproto.EventKind{hostproto.EventAttested, hostproto.EventLeaked, hostproto.EventExposed, hostproto.EventRunReported, hostproto.EventFetched}
	count := rapid.IntRange(0, 12).Draw(t, "requests")
	var events []hostproto.Event
	for index := range count {
		request := hostprototest.Request(func(r *hostproto.Request) { r.Prompt.User = string(rune('a' + index)) })
		emission := rapid.IntRange(1, 4).Draw(t, "emission")
		ticket := hostprototest.Ticket(request, 0, emission)
		entry := hostproto.NewEntry(ticket, request.Role, hostprototest.Digest("schema"), hostproto.DefaultMaxEmissions)
		entry.State = rapid.SampledFrom(drawnStates).Draw(t, "state")
		store.entries = append(store.entries, entry)
		holders := []hostproto.Session{rapid.SampledFrom(sessions).Draw(t, "session")}
		store.items = append(store.items, hostproto.QueueItem{RequestID: ticket.RequestID, Sessions: holders, Wave: 1})
		for range rapid.IntRange(0, 2).Draw(t, "events") {
			named := ticket.TicketID
			if rapid.Bool().Draw(t, "earlier emission") && emission > 1 {
				named.Emission = emission - 1
			}
			agent := ""
			if rapid.Bool().Draw(t, "bound to an agent") {
				agent = stopAgent
			}
			events = append(events, hostproto.Event{Kind: rapid.SampledFrom(stopKinds).Draw(t, "kind"), Ticket: named.String(), AgentID: agent})
		}
	}
	store.records = chain(t, events)
	return store
}

func chain(t interface{ Fatal(...any) }, events []hostproto.Event) []hostproto.Record {
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

// expectedOrphan is the rule SP-2901 states, spelled out on its own.
func expectedOrphan(entry hostproto.Entry, records []hostproto.Record) bool {
	if entry.State != hostproto.StateFetched {
		return false
	}
	stopKinds := []hostproto.EventKind{
		hostproto.EventAttested, hostproto.EventLeaked, hostproto.EventExposed, hostproto.EventRestricted, hostproto.EventAllowlistObserved,
		hostproto.EventLaunchBare, hostproto.EventHistoryInherited, hostproto.EventBlindnessUnproven,
	}
	return slices.ContainsFunc(records, func(record hostproto.Record) bool {
		return record.Event.Ticket == entry.Ticket.TicketID.String() && record.Event.AgentID != "" && slices.Contains(stopKinds, record.Event.Kind)
	})
}

func TestOutstanding(t *testing.T) {
	t.Run("every waiting ticket of the selected sessions is sorted once, by its state and the stop events of its own emission"+verifiesOutstanding, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			store := drawStore(t)
			filter := rapid.SampledFrom([]hostproto.SessionFilter{hostproto.EverySession, hostproto.OnlySession(hostproto.SessionProve)}).Draw(t, "filter")
			outstanding := hostproto.ReadOutstanding(store.entries, store.items, store.records, filter)
			sessions := hostproto.IndexSessions(store.items)
			var launchable, inFlight, orphaned []hostproto.Entry
			for _, entry := range store.entries {
				if !sessions.Selects(filter, entry.Ticket.RequestID) {
					continue
				}
				switch {
				case entry.State == hostproto.StateEmitted:
					launchable = append(launchable, entry)
				case expectedOrphan(entry, store.records):
					orphaned = append(orphaned, entry)
				case entry.State == hostproto.StateFetched:
					inFlight = append(inFlight, entry)
				}
			}
			if !reflect.DeepEqual(outstanding.Launchable, launchable) || !reflect.DeepEqual(outstanding.InFlight, inFlight) || !reflect.DeepEqual(outstanding.Orphaned, orphaned) {
				t.Fatalf("outstanding %+v, want launchable %v, in flight %v, orphaned %v", outstanding, launchable, inFlight, orphaned)
			}
			if outstanding.Waiting() != len(launchable)+len(inFlight)+len(orphaned) || len(outstanding.Tickets()) != len(launchable) {
				t.Fatalf("waiting %d, tickets %d", outstanding.Waiting(), len(outstanding.Tickets()))
			}
		})
	})

	t.Run("the run-reported event of a launch, bound to no agent, never makes a ticket orphaned"+verifiesOutstanding, func(t *testing.T) {
		ticket := hostprototest.Ticket(hostprototest.Request(), 0, hostproto.FirstEmission).TicketID.String()
		if hostproto.RunEnded([]hostproto.Event{{Kind: hostproto.EventRunReported, Ticket: ticket}, {Kind: hostproto.EventLeaked, Ticket: ticket}}) {
			t.Fatal("events of the launch itself count as the end of the run")
		}
		if !hostproto.RunEnded([]hostproto.Event{{Kind: hostproto.EventLeaked, Ticket: ticket, AgentID: stopAgent}}) {
			t.Fatal("a stop event bound to the agent does not end the run")
		}
	})
}

func TestWaveOpening(t *testing.T) {
	t.Run("a pass opens a wave only for a session that has none open or that got an answer since its wave opened"+verifiesWaveOpening, func(t *testing.T) {
		cases := []struct {
			state    hostproto.SessionState
			answered int
			opens    bool
		}{
			{hostproto.SessionState{}, 0, true},
			{hostproto.SessionState{Wave: 2, Ended: true}, 0, true},
			{hostproto.SessionState{Wave: 2, Cursor: 5}, 0, false},
			{hostproto.SessionState{Wave: 2, Cursor: 5}, 1, true},
		}
		for _, c := range cases {
			if got := c.state.OpensWave(c.answered); got != c.opens {
				t.Errorf("%+v with %d answers: opens %v", c.state, c.answered, got)
			}
		}
	})

	t.Run("answers are counted from the cursor on, for the selected sessions only"+verifiesWaveOpening+verifiesProgress, func(t *testing.T) {
		prove := hostprototest.Ticket(hostprototest.Request(), 0, hostproto.FirstEmission)
		scanned := hostprototest.Ticket(hostprototest.Request(func(r *hostproto.Request) { r.Prompt.User = "scan" }), 0, hostproto.FirstEmission)
		items := []hostproto.QueueItem{
			{RequestID: prove.RequestID, Sessions: []hostproto.Session{hostproto.SessionProve}},
			{RequestID: scanned.RequestID, Sessions: []hostproto.Session{hostproto.ScanSession(drawnScanKey)}},
		}
		records := chain(t, []hostproto.Event{
			{Kind: hostproto.EventAccepted, Ticket: prove.TicketID.String()},
			{Kind: hostproto.EventFetched, Ticket: scanned.TicketID.String()},
			{Kind: hostproto.EventAccepted, Ticket: scanned.TicketID.String()},
		})
		sessions := hostproto.IndexSessions(items)
		only := hostproto.OnlySession(hostproto.SessionProve)
		for cursor, want := range map[int]int{0: 1, 1: 0, 3: 0} {
			if got := hostproto.AnsweredSince(records, cursor, sessions, only); got != want {
				t.Errorf("prove answers since %d: %d, want %d", cursor, got, want)
			}
		}
		if got := hostproto.AnsweredSince(records, 1, sessions, hostproto.EverySession); got != 1 {
			t.Errorf("every session's answers since 1: %d", got)
		}
		if hostproto.JournalCursor(records) != len(records) {
			t.Errorf("cursor %d", hostproto.JournalCursor(records))
		}
	})
}

func TestProgressContract(t *testing.T) {
	t.Run("the progress document has exactly the members of its contract file"+verifiesProgress, func(t *testing.T) {
		contract, err := os.ReadFile(progressContractFile)
		if err != nil {
			t.Fatal(err)
		}
		var want map[string]any
		if err := json.Unmarshal(contract, &want); err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(hostproto.Progress{Cursor: 12, Since: 7, Answered: 2, Launchable: 3, InFlight: 1, Orphaned: 1})
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(encoded, &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("progress %s, contract %s", encoded, contract)
		}
		if args := hostproto.ProgressArgs(hostproto.OnlySession(hostproto.SessionProve), 7); !slices.Equal(args, []string{"request", "progress", "--json", "--session", "prove", "--since", "7"}) {
			t.Fatalf("arguments %v", args)
		}
	})

	t.Run("a second read given the first one's cursor reports an answer exactly when one was accepted in between"+verifiesProgress, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			ticket := hostprototest.Ticket(hostprototest.Request(), 0, hostproto.FirstEmission)
			view := hostproto.StoreView{Items: []hostproto.QueueItem{{RequestID: ticket.RequestID, Sessions: []hostproto.Session{hostproto.SessionProve}}}}
			kinds := []hostproto.EventKind{hostproto.EventEmitted, hostproto.EventFetched, hostproto.EventAccepted, hostproto.EventValidated}
			var events []hostproto.Event
			for range rapid.IntRange(0, 6).Draw(t, "before") {
				events = append(events, hostproto.Event{Kind: rapid.SampledFrom(kinds).Draw(t, "kind"), Ticket: ticket.TicketID.String()})
			}
			view.Records = chain(t, events)
			first := view.Progress(hostproto.OnlySession(hostproto.SessionProve), 0)
			accepted := false
			for range rapid.IntRange(0, 6).Draw(t, "after") {
				kind := rapid.SampledFrom(kinds).Draw(t, "kind")
				accepted = accepted || kind == hostproto.EventAccepted
				events = append(events, hostproto.Event{Kind: kind, Ticket: ticket.TicketID.String()})
			}
			view.Records = chain(t, events)
			second := view.Progress(hostproto.OnlySession(hostproto.SessionProve), first.Cursor)
			if (second.Answered > 0) != accepted || second.Since != first.Cursor || second.Cursor != len(events) {
				t.Fatalf("first %+v, second %+v, accepted in between %v", first, second, accepted)
			}
		})
	})
}

func TestRunLock(t *testing.T) {
	t.Run("one run at a time holds a session of the reference store, and a busy session is refused at once"+verifiesRunLock, func(t *testing.T) {
		store := hostprototest.NewMemoryHostStore()
		release, err := store.LockRun(hostproto.SessionProve)
		if err != nil {
			t.Fatal(err)
		}
		before, _ := store.Sessions()
		if _, err := store.LockRun(hostproto.SessionProve); !errors.Is(err, hostproto.ErrSessionBusy) {
			t.Fatalf("a second run: %v", err)
		}
		if after, _ := store.Sessions(); !reflect.DeepEqual(before, after) {
			t.Fatal("a refused run changed the store")
		}
		other, err := store.LockRun(hostproto.ScanSession(drawnScanKey))
		if err != nil {
			t.Fatalf("another session: %v", err)
		}
		other()
		release()
		again, err := store.LockRun(hostproto.SessionProve)
		if err != nil {
			t.Fatalf("after the release: %v", err)
		}
		again()
	})
}

func TestStep(t *testing.T) {
	t.Run("a step is spent exactly when its own budget ran out, and its bounds leave it within the limit"+verifiesStepBudget, func(t *testing.T) {
		ctx, cancel := hostproto.WithStep(context.Background(), time.Millisecond)
		defer cancel()
		<-ctx.Done()
		if !hostproto.StepSpent(ctx) {
			t.Fatal("a step whose budget ran out is not spent")
		}
		parent, stop := context.WithCancel(context.Background())
		cut, release := hostproto.WithStep(parent, time.Hour)
		defer release()
		stop()
		if hostproto.StepSpent(cut) {
			t.Fatal("a step whose parent was cancelled counts as spent")
		}
		if hostproto.StepLimit != 90*time.Second || hostproto.MaxStepWork >= hostproto.StepLimit || hostproto.DefaultStepWork > hostproto.MaxStepWork || hostproto.MinStepWork != time.Second {
			t.Fatalf("limit %v, max %v, default %v", hostproto.StepLimit, hostproto.MaxStepWork, hostproto.DefaultStepWork)
		}
		if hostproto.StepBudgetSpent.Line() != "unfinished budget_spent" || hostproto.StepSessionBusy.Line() != "unfinished session_busy" || hostproto.StepFinished.Unfinished() {
			t.Fatal("the lines of an unfinished step")
		}
	})
}
