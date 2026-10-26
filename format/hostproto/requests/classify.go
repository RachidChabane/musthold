package requests

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
)

// ClassifyOutcomeKind tells what an import of a classification came to.
type ClassifyOutcomeKind string

// Outcomes of an import of a classification.
const (
	// ClassifyAnswered: the classification answered the request; its ticket holds
	// the answer.
	ClassifyAnswered ClassifyOutcomeKind = "answered"
	// ClassifyFellThrough: the gate stayed closed; the ticket is left to the host votes.
	ClassifyFellThrough ClassifyOutcomeKind = "fell-through"
	// ClassifyShadowed: the classification was journaled and answers nothing.
	ClassifyShadowed ClassifyOutcomeKind = "shadow"
	// ClassifyNotAdmitted: no admission entry covers the classification; the ticket
	// is left to the host votes.
	ClassifyNotAdmitted ClassifyOutcomeKind = "not-admitted"
	// ClassifyOff: the switch is off; nothing happened.
	ClassifyOff ClassifyOutcomeKind = "off"
)

// Reasons an import leaves the ticket to the host votes, besides the gate's own.
const reasonNotAdmitted = "not_admitted"

// ClassifyOutcome is what an import of a classification came to.
type ClassifyOutcome struct {
	Kind   ClassifyOutcomeKind `json:"outcome"`
	Reason string              `json:"reason,omitempty"`
	Ticket string              `json:"ticket"`
}

// ClassifyImport is one import of a classification.
type ClassifyImport struct {
	Ticket hostproto.TicketID
	Nonce  string
	// Source is the text of the confidence source the mod names.
	Source string
	// Report is the document the mod piped in.
	Report []byte
	// ProofKind is the decision kind of the module that imports.
	ProofKind format.Kind
	Mode      hostproto.ClassifyMode
	Admission hostproto.ClassifyAdmission
}

// ImportClassification reads the classifier calls a host mod made for a closed
// request. In `auto` it answers the ticket when an admission entry covers the
// (decision kind, exact source, session model) and every question reaches
// TauClassify, and leaves the ticket untouched otherwise, so that the host votes
// can answer it; in `shadow` it journals the classification and answers nothing;
// in `off` it does nothing.
func (s Service) ImportClassification(in ClassifyImport) (ClassifyOutcome, error) {
	outcome := ClassifyOutcome{Ticket: in.Ticket.String()}
	if in.Mode == hostproto.ClassifyOff {
		outcome.Kind = ClassifyOff
		return outcome, nil
	}
	source, err := format.ParseClassifySource(in.Source)
	if err != nil {
		return outcome, err
	}
	report, err := hostproto.ParseClassifyReport(in.Report)
	if err != nil {
		return outcome, err
	}
	entry, err := s.Store.GetEntry(in.Ticket.Key())
	if err != nil {
		return outcome, err
	}
	if err := hostproto.CheckClassifiable(entry, in.Ticket, in.Nonce); err != nil {
		s.journal(hostproto.EventRejected, in.Ticket, err.Error())
		return outcome, err
	}
	request, err := s.Store.GetRequest(in.Ticket.RequestID)
	if err != nil {
		return outcome, err
	}
	set, err := hostproto.ParseClosedSet(request.Closed)
	if err != nil {
		return outcome, fmt.Errorf("%w: %v", hostproto.ErrNoClosedQuestions, err)
	}
	admitted, covered := in.Admission.Lookup(hostproto.ClassifyAdmissionKey{Kind: in.ProofKind, Source: source.Source(), Model: report.SessionModel})
	table := admitted.Table
	if !covered {
		table = hostproto.ShareTable{K: source.K, Scheme: source.Scheme}
	}
	reading, err := hostproto.ReadClassification(in.Ticket.RequestID, set, source, report, table)
	if err != nil {
		return outcome, err
	}
	return s.settleClassification(in, outcome, set, source, reading, covered)
}

// now is the time of the service's clock, the zero time without one.
func (s Service) now() time.Time {
	if s.Clock == nil {
		return time.Time{}
	}
	return s.Clock()
}

func (s Service) settleClassification(in ClassifyImport, outcome ClassifyOutcome, set hostproto.ClosedSet, source format.ClassifySource, reading hostproto.ClassifyReading, covered bool) (ClassifyOutcome, error) {
	switch {
	case in.Mode == hostproto.ClassifyShadow:
		s.journal(hostproto.EventClassifyShadow, in.Ticket, hostproto.ClassifyDetail(reading, s.now()))
		outcome.Kind = ClassifyShadowed
		return outcome, nil
	case !covered:
		s.journal(hostproto.EventClassifyFellThrough, in.Ticket, hostproto.ClassifyDetail(reading, s.now()))
		outcome.Kind, outcome.Reason = ClassifyNotAdmitted, reasonNotAdmitted
		return outcome, nil
	case !reading.Answers():
		s.journal(hostproto.EventClassifyFellThrough, in.Ticket, hostproto.ClassifyDetail(reading, s.now()))
		outcome.Kind, outcome.Reason = ClassifyFellThrough, string(reading.Gate)
		return outcome, nil
	}
	output, err := reading.Output(set)
	if err != nil {
		return outcome, err
	}
	if _, err := s.Show(in.Ticket, ""); err != nil {
		return outcome, err
	}
	profile := hostproto.ClassifyProfile(source, reading.Record.CLIVersion)
	_, err = s.Submit(Submission{
		Ticket: in.Ticket, Nonce: in.Nonce, Output: output, Origin: OriginClassify, Executor: &profile, Classify: &reading.Record,
	})
	if err != nil {
		return outcome, err
	}
	s.journal(hostproto.EventClassifyAnswered, in.Ticket, hostproto.ClassifyDetail(reading, s.now()))
	outcome.Kind = ClassifyAnswered
	return outcome, nil
}

// EncodeClassifyOutcome renders the outcome as the single line the import prints.
func EncodeClassifyOutcome(outcome ClassifyOutcome) ([]byte, error) { return json.Marshal(outcome) }
