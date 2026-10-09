package hostprototest

import (
	"encoding/json"
	"testing"

	"github.com/RachidChabane/spec-proof/format/hostproto"
)

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
