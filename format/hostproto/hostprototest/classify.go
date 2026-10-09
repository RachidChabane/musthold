package hostprototest

import (
	"encoding/json"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// Values the classification tests share.
const (
	// ClassifyCLIVersion is the host version the fake mod reports.
	ClassifyCLIVersion = "2.1.293"
	// ClassifySessionModel is the model of the fake session.
	ClassifySessionModel = "claude-sonnet-5-5"
)

// ClassifyTable is the table the tests admit with: a modal count of 3 gives 0.90,
// of 4 gives 0.96 and of 5 gives 0.99, so that only 4 and 5 open the gate.
func ClassifyTable() hostproto.ShareTable {
	return hostproto.ShareTable{K: hostproto.ClassifyCalls, Scheme: hostproto.ClassifyScheme, Points: []hostproto.SharePoint{
		{Count: 3, Confidence: 0.90}, {Count: 4, Confidence: 0.96}, {Count: 5, Confidence: 0.99},
	}}
}

// ClassifyAdmissionOf admits the classification under the source for the kind and
// the session model, with ClassifyTable.
func ClassifyAdmissionOf(t testing.TB, kind format.Kind, source format.ClassifySource, sessionModel string) hostproto.ClassifyAdmission {
	t.Helper()
	table := ClassifyTable()
	digest, err := table.Digest()
	if err != nil {
		t.Fatal(err)
	}
	admission := hostproto.ClassifyAdmission{Entries: []hostproto.ClassifyAdmissionEntry{{
		Kind: kind, Source: source.Source(), Model: sessionModel, Outcome: hostproto.ClassifyOutcomeMet,
		CLIVersion: ClassifyCLIVersion, Table: table, TableDigest: digest,
	}}}
	encoded, err := json.Marshal(admission)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hostproto.ParseClassifyAdmission(encoded); err != nil {
		t.Fatal(err)
	}
	return admission
}

// ClassifyReportOf renders the report a mod hands `systemone import` for a plan:
// the label each call returns is the one the answer function picks, and an empty
// label is a call that returned none, after two attempts.
func ClassifyReportOf(t testing.TB, item hostproto.ClassifyPlanItem, answer func(hostproto.ClassifiedCall) string) []byte {
	t.Helper()
	report := hostproto.ClassifyReport{CLIVersion: ClassifyCLIVersion, SessionModel: ClassifySessionModel}
	for _, call := range item.Calls {
		reported := hostproto.ReportedCall{Question: call.Question, Index: call.Index, Model: call.Model, Attempts: 1}
		if label := answer(call); label != "" {
			reported.Label = &label
		} else {
			reported.Attempts = hostproto.MaxClassifyAttempts
		}
		report.Calls = append(report.Calls, reported)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

// Always answers every call with the label.
func Always(label string) func(hostproto.ClassifiedCall) string {
	return func(hostproto.ClassifiedCall) string { return label }
}

// DecodeClassifyPlans decodes the output of `systemone classify-plan`.
func DecodeClassifyPlans(t testing.TB, output string) []hostproto.ClassifyPlanItem {
	t.Helper()
	var items []hostproto.ClassifyPlanItem
	if err := json.Unmarshal([]byte(output), &items); err != nil {
		t.Fatalf("classify-plan printed %q: %v", output, err)
	}
	return items
}

// CheckClassifyPlan checks one item of `systemone classify-plan` against the plan
// of the question set it covers: the ticket, the source, the digest, and for
// every question five calls whose model, labels, text and seed input are those
// of the plan under the source.
func CheckClassifyPlan(t testing.TB, item hostproto.ClassifyPlanItem, ticket hostproto.Ticket, closed json.RawMessage, source string) {
	t.Helper()
	set, err := hostproto.ParseClosedSet(closed)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := set.ClassifyPlan()
	if err != nil {
		t.Fatal(err)
	}
	if item.Ticket != ticket.TicketID.String() || item.Nonce != ticket.Nonce || item.Source != source ||
		item.Scheme != plan.Scheme || item.K != plan.K || item.ClosedSetDigest != plan.ClosedSetDigest {
		t.Fatalf("item %+v", item)
	}
	if len(item.Calls) != len(set.Questions)*plan.K {
		t.Fatalf("%d calls for %d questions", len(item.Calls), len(set.Questions))
	}
	for _, call := range item.Calls {
		question, known := plan.Question(call.Question)
		if !known || call.Index < 0 || call.Index >= plan.K {
			t.Fatalf("call %+v", call)
		}
		planned := question.Calls[call.Index]
		if call.Text != planned.Text || call.SeedInput != question.SeedInput || len(call.Labels) != len(planned.Labels) || call.Model == "" {
			t.Fatalf("call %+v differs from the plan %+v", call, planned)
		}
		for position := range call.Labels {
			if call.Labels[position] != planned.Labels[position] {
				t.Fatalf("call %+v differs from the plan %+v", call, planned)
			}
		}
	}
}
