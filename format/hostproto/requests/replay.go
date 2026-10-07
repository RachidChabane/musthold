package requests

import (
	"errors"
	"fmt"

	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// ErrReplayMiss is returned when a ticket has no recorded answer: a replay never
// guesses.
var ErrReplayMiss = errors.New("no recorded answer for the request")

// RecordingReplayer answers pending tickets from recordings, through the same checks a
// script's submission goes through.
type RecordingReplayer struct {
	Service    Service
	Recordings Recordings
}

var _ Replayer = RecordingReplayer{}

// Serve fetches and answers every ticket. A ticket without a recording is a hard
// error.
func (r RecordingReplayer) Serve(tickets []PendingTicket) error {
	for _, pending := range tickets {
		ticket, err := hostproto.ParseTicketID(pending.Ticket)
		if err != nil {
			return err
		}
		recording, err := r.recordingOf(ticket)
		if err != nil {
			return fmt.Errorf("%w %s: %v", ErrReplayMiss, ticket.RequestID, err)
		}
		if _, err := r.Service.Show(ticket, ""); err != nil {
			return fmt.Errorf("fetch %s for the replay: %w", ticket, err)
		}
		_, err = r.Service.Submit(Submission{Ticket: ticket, Nonce: pending.Nonce, Output: recording.Output, Origin: OriginReplay, Executor: recording.Executor})
		if err != nil {
			return fmt.Errorf("replay %s: %w", ticket, err)
		}
	}
	return nil
}

// recordingOf finds the recording of a ticket's vote, which falls back on the
// recording of the request's first vote.
func (r RecordingReplayer) recordingOf(ticket hostproto.TicketID) (Recording, error) {
	recording, err := r.Recordings.Load(RecordingName(string(ticket.RequestID), ticket.Vote))
	if err != nil && ticket.Vote != 0 {
		return r.Recordings.Load(string(ticket.RequestID))
	}
	return recording, err
}
