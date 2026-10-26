package power

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/policy"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
	"github.com/RachidChabane/musthold/discpower/internal/domain/witness"
	"github.com/RachidChabane/musthold/format"
)

// RecordVersion is the version of the facts record; a reader refuses another.
const RecordVersion = 1

// ErrRecordVersion is returned for a facts record of another version.
var ErrRecordVersion = errors.New("unsupported facts record version")

// Record is what the module keeps of one criterion's proof: every executed fact
// and every answer Evaluate reads, so that the outcome of the criterion can be
// recomputed for any test set and any binding of the tests.
type Record struct {
	Version   int               `json:"version"`
	Criterion spec.Ref          `json:"criterion"`
	Policy    policy.Policy     `json:"policy"`
	Admission witness.Admission `json:"admission"`
	// Reference holds what every bound test of the suite did on the reference.
	Reference map[ecosystem.TestID]ecosystem.TestOutcome `json:"reference"`
	// ExampleFailed is set when an example of the criterion fails on the
	// reference.
	ExampleFailed bool `json:"exampleFailed,omitempty"`
	// Bindings names the criteria every bound test of the suite is bound to.
	Bindings  map[ecosystem.TestID][]spec.Ref `json:"bindings"`
	Witnesses []witness.Record                `json:"witnesses"`
	Entries   []EntryRecord                   `json:"entries"`
	// BudgetExhausted and HostUnresponsive are set when a ticket could not be
	// obtained.
	BudgetExhausted  bool `json:"budgetExhausted,omitempty"`
	HostUnresponsive bool `json:"hostUnresponsive,omitempty"`
}

// EntryRecord is one pool mutant of a Record.
type EntryRecord struct {
	Facts    Facts  `json:"facts"`
	Votes    []Vote `json:"votes,omitempty"`
	Unjudged Reason `json:"unjudged,omitempty"`
}

// NewRecord builds the record of a standing.
func NewRecord(standing Standing, rules Rules, reference map[ecosystem.TestID]ecosystem.TestOutcome) Record {
	record := Record{
		Version: RecordVersion, Criterion: standing.Subject.Ref, Policy: rules.Policy, Admission: rules.Admission,
		Reference: reference, ExampleFailed: standing.ExampleFailed, Bindings: standing.Subject.Bindings,
		Witnesses: slices.Clone(standing.Witnesses), BudgetExhausted: standing.BudgetExhausted, HostUnresponsive: standing.HostUnresponsive,
	}
	for _, entry := range standing.Entries {
		record.Entries = append(record.Entries, EntryRecord{Facts: entry.Facts, Votes: entry.Votes, Unjudged: entry.Unjudged})
	}
	return record
}

// Rules returns the rules the record was decided under, with another admission
// when one is given.
func (r Record) Rules(admission witness.Admission) Rules {
	if admission == "" {
		admission = r.Admission
	}
	return Rules{Policy: r.Policy, Admission: admission}
}

// StandingFor rebuilds the standing of the criterion for a test set given by its
// bindings: the tests bound to the criterion form the set, the others decide
// isolation, and the reference results of the set say whether the correct code
// passes it.
func (r Record) StandingFor(bindings map[ecosystem.TestID][]spec.Ref) Standing {
	var sentinels []Facts
	entries := make([]Entry, len(r.Entries))
	for index, entry := range r.Entries {
		entries[index] = Entry{Facts: entry.Facts, Votes: entry.Votes, Unjudged: entry.Unjudged}
		if entry.Facts.Origin == OriginSentinel {
			sentinels = append(sentinels, entry.Facts)
		}
	}
	subject := NewSubject(r.Criterion, bindings, SentinelKillers(sentinels))
	correct := true
	for _, test := range subject.Tests {
		correct = correct && r.Reference[test] == ecosystem.TestPassed
	}
	return Standing{
		Subject: subject, CorrectCodePasses: correct, ExampleFailed: r.ExampleFailed,
		Witnesses: r.Witnesses, Entries: entries, BudgetExhausted: r.BudgetExhausted, HostUnresponsive: r.HostUnresponsive,
	}
}

// Encode renders the record as canonical JSON.
func (r Record) Encode() ([]byte, error) { return format.Canonicalize(r) }

// DecodeRecord reads a record and refuses another version.
func DecodeRecord(data []byte) (Record, error) {
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, fmt.Errorf("decode a facts record: %w", err)
	}
	if record.Version != RecordVersion {
		return Record{}, fmt.Errorf("%w: %d", ErrRecordVersion, record.Version)
	}
	return record, nil
}
