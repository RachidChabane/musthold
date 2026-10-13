package requests

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostsettings"
)

// ClassifyCommands are the classification verbs of `systemone` for one module:
// what the command line of the module hands them is the service over its host
// store, its decision kind, its settings and the admission table.
type ClassifyCommands struct {
	Service   Service
	Kind      format.Kind
	Settings  hostsettings.Settings
	Admission hostproto.ClassifyAdmission
	Stdin     io.Reader
	Stdout    io.Writer
}

// IsClassifyRefusal reports whether the error is the caller's: a report, a
// source, a ticket or a switch the protocol refuses, not a failure of the tool.
func IsClassifyRefusal(err error) bool {
	refused := []error{
		hostproto.ErrClassifyReport, hostproto.ErrClassifyPlan, hostproto.ErrClassifyVoters, hostproto.ErrClassifyNotFirstVote, hostproto.ErrClassifyReissued,
		hostproto.ErrNoClosedQuestions, hostproto.ErrClassifyMode, hostproto.ErrTicketMismatch, hostproto.ErrNonceMismatch,
		hostproto.ErrNotSubmittable, hostproto.ErrNotFound, hostproto.ErrSchemaViolation, hostproto.ErrResubmissionConflict,
		format.ErrClassifySource,
	}
	for _, known := range refused {
		if errors.Is(err, known) {
			return true
		}
	}
	return false
}

// Plan prints the plan of the closed requests that wait for their first vote, as
// a JSON array: empty unless the switch and the admission table ask for a
// classification under the source.
func (c ClassifyCommands) Plan(model, voters, sessionModel string) error {
	source, err := hostproto.ClassifySourceOf(model, voters)
	if err != nil {
		return err
	}
	if sessionModel == "" {
		sessionModel = model
	}
	listing, err := c.Service.List(hostproto.EverySession)
	if err != nil {
		return err
	}
	listing, err = c.unclassified(listing)
	if err != nil {
		return err
	}
	policy := hostproto.ClassifyPolicy{Kind: c.Kind, Mode: c.Settings.ClassifyModeFor(c.Kind), Admission: c.Admission, SessionModel: sessionModel}
	plans, err := hostproto.ClassifyPlans(listing, source, policy)
	if err != nil {
		return err
	}
	return json.NewEncoder(c.Stdout).Encode(plans)
}

// Import reads the report of the host mod from the input and prints what the
// import came to as one JSON line.
func (c ClassifyCommands) Import(ticketText, nonce, source string) error {
	ticket, err := hostproto.ParseTicketID(ticketText)
	if err != nil {
		return fmt.Errorf("%w: %v", hostproto.ErrTicketMismatch, err)
	}
	report, err := io.ReadAll(io.LimitReader(c.Stdin, hostproto.MaxClassifyReportBytes+1))
	if err != nil {
		return fmt.Errorf("read the classification: %w", err)
	}
	if len(report) > hostproto.MaxClassifyReportBytes {
		return fmt.Errorf("%w: more than %d bytes", hostproto.ErrClassifyReport, hostproto.MaxClassifyReportBytes)
	}
	outcome, err := c.Service.ImportClassification(ClassifyImport{
		Ticket: ticket, Nonce: nonce, Source: source, Report: report,
		ProofKind: c.Kind, Mode: c.Settings.ClassifyModeFor(c.Kind), Admission: c.Admission,
	})
	if err != nil {
		return err
	}
	return json.NewEncoder(c.Stdout).Encode(outcome)
}

// unclassified drops the requests a classification was already made for, so that a
// request is classified once: a second classification could only be a re-ask.
func (c ClassifyCommands) unclassified(listing hostproto.Listing) (hostproto.Listing, error) {
	records, err := c.Service.Store.Records()
	if err != nil {
		return listing, fmt.Errorf("read the journal: %w", err)
	}
	classified := map[string]bool{}
	for _, record := range records {
		switch record.Event.Kind {
		case hostproto.EventClassifyAnswered, hostproto.EventClassifyShadow, hostproto.EventClassifyFellThrough:
			classified[record.Event.Ticket] = true
		}
	}
	listing.Items = slices.DeleteFunc(slices.Clone(listing.Items), func(item hostproto.ListingItem) bool {
		return classified[item.Ticket.TicketID.String()]
	})
	return listing, nil
}
