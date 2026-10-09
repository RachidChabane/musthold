package hostproto_test

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	planItemContractPath = "testdata/classify-plan-item.contract.json"
	reportContractPath   = "testdata/classify-report.contract.json"
)

// contractItem is the item `systemone classify-plan` prints for the closed
// fixture, from one voter, with a fixed ticket.
func contractItem(t *testing.T) hostproto.ClassifyPlanItem {
	t.Helper()
	request := hostprototest.ClosedRequest()
	ticket := hostprototest.Ticket(request, 0, hostproto.FirstEmission)
	source, err := format.SingleVoterClassifySource(hostproto.ClassifyCalls, hostproto.ClassifyScheme, hostprototest.ClassifySessionModel)
	if err != nil {
		t.Fatal(err)
	}
	listing := hostproto.Listing{Items: []hostproto.ListingItem{{Ticket: ticket, Role: request.Role, View: request.ViewFor(ticket)}}}
	policy := hostproto.ClassifyPolicy{Kind: format.KindAlignment, Mode: hostproto.ClassifyShadow, SessionModel: hostprototest.ClassifySessionModel}
	items, err := hostproto.ClassifyPlans(listing, source, policy)
	if err != nil || len(items) != 1 {
		t.Fatalf("items %v (%v)", items, err)
	}
	return items[0]
}

func matchesContract(t *testing.T, path string, value any) {
	t.Helper()
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, '\n')
	if *updateGolden {
		if err := os.WriteFile(path, encoded, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, want) {
		t.Fatalf("the document differs from %s; run the test with -update to see the new bytes in the diff", path)
	}
}

func TestClassifyContracts(t *testing.T) {
	t.Run("the item classify-plan prints has the members the host mod reads [verifies SP-1906~1]", func(t *testing.T) {
		item := contractItem(t)
		matchesContract(t, planItemContractPath, item)

		var members map[string]json.RawMessage
		encoded, _ := json.Marshal(item)
		if err := json.Unmarshal(encoded, &members); err != nil {
			t.Fatal(err)
		}
		for _, member := range []string{"ticket", "nonce", "source", "mode", "scheme", "k", "closedSetDigest", "calls"} {
			if _, present := members[member]; !present {
				t.Errorf("the item has no %q", member)
			}
		}
		var calls []map[string]json.RawMessage
		if err := json.Unmarshal(members["calls"], &calls); err != nil || len(calls) != hostproto.ClassifyCalls {
			t.Fatalf("calls %s (%v)", members["calls"], err)
		}
		for _, member := range []string{"question", "i", "model", "text", "labels", "seedInput"} {
			if _, present := calls[0][member]; !present {
				t.Errorf("a call has no %q", member)
			}
		}
	})

	t.Run("the report the host mod pipes into import has the members the module reads, and the module reads it back [verifies SP-1906~1]", func(t *testing.T) {
		item := contractItem(t)
		report := hostprototest.ClassifyReportOf(t, item, hostprototest.Always("VIOLATES"))
		var document any
		if err := json.Unmarshal(report, &document); err != nil {
			t.Fatal(err)
		}
		matchesContract(t, reportContractPath, document)

		parsed, err := hostproto.ParseClassifyReport(report)
		if err != nil || len(parsed.Calls) != hostproto.ClassifyCalls || parsed.CLIVersion == "" || parsed.SessionModel == "" {
			t.Fatalf("report %+v (%v)", parsed, err)
		}
		reading, err := hostproto.ReadClassification(classifyRequestID, hostprototest.ClosedSet(), classifySource(t), parsed, hostprototest.ClassifyTable())
		if err != nil || !reading.Answers() {
			t.Fatalf("reading %+v (%v)", reading, err)
		}
		var members map[string]json.RawMessage
		_ = json.Unmarshal(report, &members)
		keys := make([]string, 0, len(members))
		for key := range members {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		if !slices.Equal(keys, []string{"calls", "cliVersion", "sessionModel"}) {
			t.Errorf("report members %v", keys)
		}
	})
}
