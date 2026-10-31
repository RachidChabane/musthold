package hostbridge

import (
	"fmt"

	"github.com/RachidChabane/musthold/format/hostproto"
)

// Lines a step prints about what it leaves for later.
const (
	awaitingLine = "awaiting_host %d tickets"
	inFlightLine = "in_flight %d tickets"
)

// Waiting is what one step of a session's run leaves for a later invocation: the
// launchable tickets, the count of tickets executors fetched and still hold,
// and why the step ended before its run finished, if it did.
type Waiting struct {
	Step       hostproto.StepEnd
	Launchable []hostproto.Ticket
	InFlight   int
}

// WaitingOf is what a step that closed its wave leaves: what the session still
// waits for.
func WaitingOf(outstanding hostproto.Outstanding) Waiting {
	return Waiting{Launchable: outstanding.Tickets(), InFlight: len(outstanding.InFlight) + len(outstanding.Orphaned)}
}

// Unfinished is what a step that ended before its run finished leaves, with the
// session's wave: the tickets the session waits for as the store holds them,
// read without closing the wave, so that nothing the step did not reach is
// superseded or retired. A step that spent its budget before it opened a wave
// opens one, so that the run it started can be resumed; a step refused because
// another run holds the session changes nothing.
func Unfinished(store hostproto.HostStore, session hostproto.Session, end hostproto.StepEnd) (Waiting, int, error) {
	wave, err := unfinishedWave(store, session, end)
	if err != nil {
		return Waiting{}, 0, err
	}
	outstanding, err := hostproto.QueryOutstanding(store, hostproto.OnlySession(session))
	if err != nil {
		return Waiting{}, 0, err
	}
	waiting := WaitingOf(outstanding)
	waiting.Step = end
	return waiting, wave, nil
}

// unfinishedWave is the wave an unfinished step leaves the session in.
func unfinishedWave(store hostproto.HostStore, session hostproto.Session, end hostproto.StepEnd) (int, error) {
	states, err := store.Sessions()
	if err != nil {
		return 0, fmt.Errorf("read the sessions: %w", err)
	}
	state := hostproto.SessionState{Session: session}
	for _, known := range states {
		if known.Session == session {
			state = known
		}
	}
	if end != hostproto.StepBudgetSpent || !state.OpensWave(0) {
		return state.Wave, nil
	}
	records, err := store.Records()
	if err != nil {
		return 0, fmt.Errorf("read the journal: %w", err)
	}
	return store.AdvanceWave(session, hostproto.JournalCursor(records))
}

// Pending reports whether the step leaves anything for later: a ticket to
// answer, a ticket an executor holds, or work its budget did not reach.
func (w Waiting) Pending() bool {
	return w.Step.Unfinished() || len(w.Launchable) > 0 || w.InFlight > 0
}

// Lines are what a command prints about what the step leaves: the launchable
// count, which is the number of tickets `request list` shows, the tickets in
// flight, and the unfinished step.
func (w Waiting) Lines() []string {
	var lines []string
	if len(w.Launchable) > 0 {
		lines = append(lines, fmt.Sprintf(awaitingLine, len(w.Launchable)))
	}
	if w.InFlight > 0 {
		lines = append(lines, fmt.Sprintf(inFlightLine, w.InFlight))
	}
	if w.Step.Unfinished() {
		lines = append(lines, w.Step.Line())
	}
	return lines
}
