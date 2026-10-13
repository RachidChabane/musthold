package hostproto

import (
	"errors"
	"fmt"
	"strings"

	"github.com/RachidChabane/spec-proof/format"
)

// GenesisDigest is the `prev` of the first journal record.
var GenesisDigest = strings.Repeat("0", format.Sha256HexLength)

// EventKind names one journaled event.
type EventKind string

// Journal events.
const (
	EventEmitted              EventKind = "emitted"
	EventFetched              EventKind = "fetched"
	EventAccepted             EventKind = "accepted"
	EventRejected             EventKind = "rejected"
	EventExpired              EventKind = "expired"
	EventValidated            EventKind = "validated"
	EventConsumed             EventKind = "consumed"
	EventSuperseded           EventKind = "superseded"
	EventAbandoned            EventKind = "abandoned"
	EventAgentStarted         EventKind = "agent_started"
	EventAttested             EventKind = "attested"
	EventRestricted           EventKind = "restricted"
	EventLeaked               EventKind = "leaked"
	EventExposed              EventKind = "exposed"
	EventExposureAttempt      EventKind = "exposure_attempt"
	EventResubmissionConflict EventKind = "resubmission_conflict"
	EventSchemaFailed         EventKind = "schema_failed"
	EventUncappedSubmission   EventKind = "uncapped_submission"
	// EventRunReported records what the harness reported about a finished
	// executor run, the resolved model and the usage, as the event's detail.
	EventRunReported EventKind = "run_reported"
	// EventHistoryInherited records an executor run whose transcript shows that it
	// started with history of the agent that spawned it; the detail says where.
	// Its answer is credited at the isolation `inline`.
	EventHistoryInherited EventKind = "history_inherited"
	// EventBlindnessUnproven records an executor run whose transcript cannot show
	// that it started with none of the spawning agent's history; the detail says
	// why. Its answer is credited at the isolation `inline`.
	EventBlindnessUnproven EventKind = "blindness_unproven"

	// EventClassifyAnswered records a classification that answered its request:
	// the detail is a ClassifyJournalDetail.
	EventClassifyAnswered EventKind = "classify_answered"
	// EventClassifyShadow records a classification made in shadow mode, which
	// answers nothing.
	EventClassifyShadow EventKind = "classify_shadow"
	// EventClassifyFellThrough records a classification that left its request to
	// the host votes, with the reason.
	EventClassifyFellThrough EventKind = "classify_fell_through"
	// EventClassifyWitnessed records that the raw API log matched the calls of a
	// classification: its votes keep the isolation `restricted`.
	EventClassifyWitnessed EventKind = "classify_witnessed"
	// EventClassifyLabelContradicted records that the raw API log holds the dictated
	// calls with other labels than the ones submitted.
	EventClassifyLabelContradicted EventKind = "classify_label_contradicted"
	// EventClassifyShadowWitnessed records what the raw API log shows of a
	// classification made in shadow, which answers nothing: its detail is a
	// ShadowWitness.
	EventClassifyShadowWitnessed EventKind = "classify_shadow_witnessed"
	// EventWitnessConsumed records the request ids of the raw API log a witness
	// matched, so that no entry witnesses twice.
	EventWitnessConsumed EventKind = "witness_consumed"
	// EventWitnessSetCommitted records, before the witnesses of a criterion run on
	// any mutant and before the first generation ticket of that criterion, the
	// digest of a witness set a writer returned: the detail is
	// `<criterion> <draw> <set digest>`.
	EventWitnessSetCommitted EventKind = "witness_set_committed"
)

// Event is one journaled fact. Only the fields that apply are set.
type Event struct {
	Kind    EventKind `json:"kind"`
	Ticket  string    `json:"ticket,omitempty"`
	AgentID string    `json:"agentId,omitempty"`
	Role    string    `json:"role,omitempty"`
	Detail  string    `json:"detail,omitempty"`
	// Session is the session whose bridge moved the ticket; an emission journaled
	// with it counts toward that session's emission cap.
	Session Session `json:"session,omitempty"`
}

// Emissions counts the emissions the journal records for the session: first
// issues, reissues and re-emissions alike.
func Emissions(records []Record, session Session) int {
	count := 0
	for _, record := range records {
		if record.Event.Kind == EventEmitted && record.Event.Session == session {
			count++
		}
	}
	return count
}

// Record is one link of the digest chain: the event, the digest of the previous
// record and its own digest.
type Record struct {
	Sequence int    `json:"seq"`
	Prev     string `json:"prev"`
	Event    Event  `json:"event"`
	Digest   string `json:"digest"`
}

// ErrChainBroken is returned when a journal does not verify.
var ErrChainBroken = errors.New("journal chain broken")

type recordBody struct {
	Sequence int    `json:"seq"`
	Prev     string `json:"prev"`
	Event    Event  `json:"event"`
}

func digestOf(body recordBody) (string, error) {
	canonical, err := format.Canonicalize(body)
	if err != nil {
		return "", err
	}
	return format.SHA256Hex(canonical), nil
}

// Chain appends an event to a journal: it builds the record that follows head,
// which is nil for an empty journal.
func Chain(head *Record, event Event) (Record, error) {
	body := recordBody{Sequence: 0, Prev: GenesisDigest, Event: event}
	if head != nil {
		body = recordBody{Sequence: head.Sequence + 1, Prev: head.Digest, Event: event}
	}
	digest, err := digestOf(body)
	if err != nil {
		return Record{}, err
	}
	return Record{Sequence: body.Sequence, Prev: body.Prev, Event: event, Digest: digest}, nil
}

// VerifyChain checks that every record follows its predecessor, starts at the
// genesis digest and carries the digest of its own content.
func VerifyChain(records []Record) error {
	prev := GenesisDigest
	for index, record := range records {
		expected, err := digestOf(recordBody{Sequence: record.Sequence, Prev: record.Prev, Event: record.Event})
		if err != nil {
			return err
		}
		if record.Sequence != index || record.Prev != prev || record.Digest != expected {
			return fmt.Errorf("%w at record %d", ErrChainBroken, index)
		}
		prev = record.Digest
	}
	return nil
}

// JournalRoster answers the roster question from journaled agent_started events.
type JournalRoster []Record

// Started reports whether the journal records the agent as started for the role.
func (j JournalRoster) Started(agentID, role string) bool {
	for _, record := range j {
		event := record.Event
		if event.Kind == EventAgentStarted && event.AgentID == agentID && event.Role == role {
			return true
		}
	}
	return false
}
