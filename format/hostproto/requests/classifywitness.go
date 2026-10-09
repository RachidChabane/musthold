package requests

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/executorrun"
)

// WitnessReport counts what the check of the raw API log did.
type WitnessReport struct {
	Raised, Exposed, Contradicted, Unsettled int
}

// WitnessClassifications checks the classifications that answered a request and
// no check settled yet against the raw API log. A classification the log holds as
// dictated, blind and with the submitted labels is witnessed: the journal records
// it, and its votes keep the isolation `restricted` with the model the log
// resolved. One the log contradicts is journaled as such, and one the log shows
// was not blind is journaled as exposed. Anything else stays as it was.
func (s Service) WitnessClassifications(log hostproto.WitnessLog) (WitnessReport, error) {
	records, err := s.Store.Records()
	if err != nil {
		return WitnessReport{}, fmt.Errorf("read the journal: %w", err)
	}
	entries, err := s.Store.ListEntries()
	if err != nil {
		return WitnessReport{}, fmt.Errorf("list the tickets: %w", err)
	}
	journal := witnessJournal{byTicket: map[string][]hostproto.Event{}, consumed: map[string]bool{}}
	for _, record := range records {
		journal.add(record.Event)
	}
	var report WitnessReport
	if err := s.witnessShadows(log, records, journal); err != nil {
		return report, err
	}
	for _, entry := range entries {
		if entry.State != hostproto.StateAccepted && entry.State != hostproto.StateValidated {
			continue
		}
		verdict, settled, err := s.witnessEntry(log, entry, journal)
		if err != nil {
			return report, err
		}
		if !settled {
			continue
		}
		switch verdict.Verdict {
		case hostproto.WitnessRaised:
			report.Raised++
			for _, id := range verdict.RequestIDs {
				journal.consumed[id] = true
			}
		case hostproto.WitnessExposed:
			report.Exposed++
		case hostproto.WitnessLabelsContradicted:
			report.Contradicted++
		default:
			report.Unsettled++
		}
	}
	return report, nil
}

// witnessJournal is the journal grouped for the check.
type witnessJournal struct {
	byTicket map[string][]hostproto.Event
	consumed map[string]bool
}

func (j witnessJournal) add(event hostproto.Event) {
	j.byTicket[event.Ticket] = append(j.byTicket[event.Ticket], event)
	if event.Kind != hostproto.EventWitnessConsumed {
		return
	}
	var ids []string
	if json.Unmarshal([]byte(event.Detail), &ids) == nil {
		for _, id := range ids {
			j.consumed[id] = true
		}
	}
}

// decided reports whether the check already came to a verdict for the ticket.
func (j witnessJournal) decided(ticket string) bool {
	return slices.ContainsFunc(j.byTicket[ticket], func(event hostproto.Event) bool {
		return event.Kind == hostproto.EventClassifyWitnessed || event.Kind == hostproto.EventClassifyLabelContradicted ||
			event.Kind == hostproto.EventExposed
	})
}

// importedAt is when the classification of the ticket was imported.
func (j witnessJournal) importedAt(ticket string) (hostproto.ClassifyJournalDetail, bool) {
	for _, event := range slices.Backward(j.byTicket[ticket]) {
		if event.Kind != hostproto.EventClassifyAnswered {
			continue
		}
		if detail, ok := hostproto.ParseClassifyDetail(event.Detail); ok && !detail.ImportedAt.IsZero() {
			return detail, true
		}
	}
	return hostproto.ClassifyJournalDetail{}, false
}

// witnessEntry checks the classification the ticket holds. settled is false for
// a ticket the check has nothing to say about.
func (s Service) witnessEntry(log hostproto.WitnessLog, entry hostproto.Entry, journal witnessJournal) (hostproto.WitnessResult, bool, error) {
	ticket := entry.Ticket.TicketID.String()
	if journal.decided(ticket) {
		return hostproto.WitnessResult{}, false, nil
	}
	response, err := s.Store.GetResponse(entry.Ticket.Key())
	if err != nil || response.Executor.Channel != format.ChannelHostClassify || response.Classify == nil {
		return hostproto.WitnessResult{}, false, nil
	}
	imported, found := journal.importedAt(ticket)
	if !found {
		return hostproto.WitnessResult{}, false, nil
	}
	texts, planned := s.plannedTexts(entry.Ticket.RequestID)
	if !planned {
		return hostproto.WitnessResult{}, false, nil
	}
	result := hostproto.CheckClassifyWitness(log, hostproto.WitnessInput{
		Record: *response.Classify, Texts: texts, From: entry.EmittedAt, To: imported.ImportedAt, Consumed: journal.consumed,
	})
	return result, true, s.journalWitness(entry.Ticket.TicketID, result)
}

func (s Service) journalWitness(ticket hostproto.TicketID, result hostproto.WitnessResult) error {
	switch result.Verdict {
	case hostproto.WitnessRaised:
		consumed, err := json.Marshal(result.RequestIDs)
		if err != nil {
			return err
		}
		s.journal(hostproto.EventWitnessConsumed, ticket, string(consumed))
		s.journal(hostproto.EventClassifyWitnessed, ticket, executorrun.WitnessDetail(executorrun.Witness{Model: result.Model, Models: result.Models}))
	case hostproto.WitnessExposed:
		s.journal(hostproto.EventExposed, ticket, "the raw API log shows the classifier calls were not blind")
	case hostproto.WitnessLabelsContradicted:
		s.journal(hostproto.EventClassifyLabelContradicted, ticket, result.String())
	}
	return nil
}

// witnessShadows records, for each classification made in shadow, what the raw API
// log shows of it. A classification that answers nothing is only observed: it
// raises nothing, marks nothing and consumes no entry.
func (s Service) witnessShadows(log hostproto.WitnessLog, records []hostproto.Record, journal witnessJournal) error {
	for _, record := range records {
		if record.Event.Kind != hostproto.EventClassifyShadow || slices.ContainsFunc(journal.byTicket[record.Event.Ticket], func(event hostproto.Event) bool {
			return event.Kind == hostproto.EventClassifyShadowWitnessed
		}) {
			continue
		}
		ticket, err := hostproto.ParseTicketID(record.Event.Ticket)
		detail, parsed := hostproto.ParseClassifyDetail(record.Event.Detail)
		if err != nil || !parsed || detail.Record == nil {
			continue
		}
		entry, err := s.Store.GetEntry(ticket.Key())
		if err != nil {
			continue
		}
		texts, ok := s.plannedTexts(ticket.RequestID)
		if !ok {
			continue
		}
		result := hostproto.CheckClassifyWitness(log, hostproto.WitnessInput{
			Record: *detail.Record, Texts: texts, From: entry.EmittedAt, To: detail.ImportedAt, Consumed: map[string]bool{}, AnyQuerySource: true,
		})
		s.journal(hostproto.EventClassifyShadowWitnessed, ticket, hostproto.ShadowWitnessDetail(result))
	}
	return nil
}

// plannedTexts lists the text of each call of each question of a request.
func (s Service) plannedTexts(id hostproto.RequestID) (map[string][]string, bool) {
	request, err := s.Store.GetRequest(id)
	if err != nil {
		return nil, false
	}
	set, err := hostproto.ParseClosedSet(request.Closed)
	if err != nil {
		return nil, false
	}
	plan, err := set.ClassifyPlan()
	if err != nil {
		return nil, false
	}
	texts := map[string][]string{}
	for _, question := range plan.Questions {
		for _, call := range question.Calls {
			texts[question.ID] = append(texts[question.ID], call.Text)
		}
	}
	return texts, true
}
