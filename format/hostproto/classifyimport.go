package hostproto

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/RachidChabane/musthold/format"
)

// Identity of the executor a classification is recorded under.
const (
	// ClassifyHarnessName is the harness whose classifier calls make the answer.
	ClassifyHarnessName = "claude-code"
)

// ErrClassifyNotFirstVote is returned for a classification imported for a ticket
// that is not the first vote of its request: a classification replaces the host
// votes as a whole.
var ErrClassifyNotFirstVote = errors.New("a classification answers the first vote of a request")

// ErrClassifyReissued is returned for a classification imported for a ticket that
// was reissued: the host's deciders answer it.
var ErrClassifyReissued = errors.New("a reissued ticket is answered by the host's deciders")

// CheckClassifiable ties an import to its entry: the ticket, the nonce, the first
// vote and the state `emitted`. A ticket a host executor already fetched takes no
// classification, so that the host votes keep their ticket.
func CheckClassifiable(entry Entry, ticket TicketID, nonce string) error {
	switch {
	case ticket != entry.Ticket.TicketID:
		return ErrTicketMismatch
	case !sameSecret(nonce, entry.Ticket.Nonce):
		return ErrNonceMismatch
	case entry.Ticket.Vote != 0:
		return ErrClassifyNotFirstVote
	case entry.Ticket.Emission != FirstEmission:
		return ErrClassifyReissued
	case entry.State != StateEmitted:
		return fmt.Errorf("%w: %s", ErrNotSubmittable, entry.State)
	}
	return nil
}

// ClassifyProfile is the executor profile of an answer made of classifier calls:
// the channel `host-classify` at the isolation `restricted-unverified`, since
// nothing the harness writes ties a label to the module's call, with the model
// the session requested as declared.
func ClassifyProfile(source format.ClassifySource, cliVersion string) format.ExecutorProfile {
	return format.ExecutorProfile{
		Channel:   format.ChannelHostClassify,
		Harness:   format.Harness{Name: ClassifyHarnessName, Version: cliVersion},
		Isolation: format.IsolationRestrictedUnverified,
		Model:     format.ExecutorModel{Name: source.PrimaryModel(), Source: format.ModelSourceDeclared},
	}
}

// ClassifyJournalDetail is the detail of the journal events of a classification.
type ClassifyJournalDetail struct {
	Source       string                 `json:"source"`
	CLIVersion   string                 `json:"cliVersion"`
	SessionModel string                 `json:"sessionModel"`
	Gate         ClassifyGate           `json:"gate,omitempty"`
	Record       *format.ClassifyRecord `json:"record,omitempty"`
	// ImportedAt is when the import was made: the raw API log is read up to this
	// time.
	ImportedAt time.Time `json:"importedAt,omitzero"`
}

// ClassifyDetail encodes the detail of a classification event.
func ClassifyDetail(reading ClassifyReading, importedAt time.Time) string {
	detail := ClassifyJournalDetail{
		Source: string(reading.Record.Source), CLIVersion: reading.Record.CLIVersion,
		SessionModel: reading.Record.SessionModel, Gate: reading.Gate, Record: &reading.Record, ImportedAt: importedAt,
	}
	encoded, err := json.Marshal(detail)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// ParseClassifyDetail reads the detail of a classification event.
func ParseClassifyDetail(detail string) (ClassifyJournalDetail, bool) {
	var parsed ClassifyJournalDetail
	if json.Unmarshal([]byte(detail), &parsed) != nil || parsed.Source == "" {
		return ClassifyJournalDetail{}, false
	}
	return parsed, true
}

// ShadowWitness is the detail of a `classify_shadow_witnessed` event: what the raw
// API log shows of a classification that answered nothing.
type ShadowWitness struct {
	Verdict      WitnessVerdict `json:"verdict"`
	Reason       string         `json:"reason,omitempty"`
	Model        string         `json:"model,omitempty"`
	QuerySources []string       `json:"querySources,omitempty"`
	// LoggedCalls is the number of lines the index of the log held.
	LoggedCalls int `json:"loggedCalls"`
}

// ShadowWitnessDetail encodes the detail of a `classify_shadow_witnessed` event.
func ShadowWitnessDetail(result WitnessResult) string {
	encoded, err := json.Marshal(ShadowWitness{
		Verdict: result.Verdict, Reason: result.Reason, Model: result.Model, QuerySources: result.QuerySources, LoggedCalls: result.LoggedCalls,
	})
	if err != nil {
		return ""
	}
	return string(encoded)
}

// ParseShadowWitness reads the detail of a `classify_shadow_witnessed` event.
func ParseShadowWitness(detail string) (ShadowWitness, bool) {
	var parsed ShadowWitness
	if json.Unmarshal([]byte(detail), &parsed) != nil || parsed.Verdict == "" {
		return ShadowWitness{}, false
	}
	return parsed, true
}
