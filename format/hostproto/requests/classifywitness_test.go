package requests

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/executorrun"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const witnessModel = "claude-sonnet-5-5-20260101"

// rawLog writes the classifier calls of a plan into a raw API log, answering each
// as the function says.
func rawLog(t testing.TB, item hostproto.ClassifyPlanItem, at time.Time, answer func(hostproto.ClassifiedCall) string, extra ...hostproto.ClassifiedCall) hostproto.WitnessLog {
	t.Helper()
	files := hostprototest.MemoryFiles{}
	var index []string
	calls := append(append([]hostproto.ClassifiedCall{}, item.Calls...), extra...)
	for number, call := range calls {
		id := fmt.Sprintf("req_%d", number)
		request, _ := json.Marshal(map[string]any{
			"model": witnessModel, "max_tokens": hostproto.ClassifyMaxTokens, "system": hostproto.ClassifierSystemPrompt(call.Labels),
			"messages": []map[string]any{{"role": "user", "content": hostproto.ClassifierUserPrompt(call.Text)}},
		})
		response, _ := json.Marshal(map[string]any{"content": []map[string]any{{"type": "text", "text": answer(call)}}})
		files[id+".req"], files[id+".res"] = request, response
		line, _ := json.Marshal(map[string]any{
			"timestamp": at.Format(time.RFC3339Nano), "session_id": "s", "query_source": hostproto.ClassifyQuerySource, "model": witnessModel,
			"request_id": id, "request_file": id + ".req", "response_file": id + ".res",
		})
		index = append(index, string(line))
	}
	files[hostproto.WitnessIndexFile] = []byte(strings.Join(index, "\n"))
	return hostproto.FileLog{Files: files}
}

// witnessFixture imports a classification that answered, with a pinned host
// version, and opens the window of its raw API log.
func witnessFixture(t testing.TB) (*classifyFixture, hostproto.ClassifyPlanItem) {
	t.Helper()
	f := newClassifyFixture(t)
	item := f.plan(hostproto.ClassifyShadow)
	if _, err := f.store.UpdateEntry(f.ticket.Key(), func(e hostproto.Entry) (hostproto.Entry, error) {
		e.EmittedAt = hostprototest.Epoch.Add(-time.Hour)
		return e, nil
	}); err != nil {
		t.Fatal(err)
	}
	report := hostproto.ClassifyReport{CLIVersion: hostproto.ClassifyPinnedCLIVersions()[0], SessionModel: hostprototest.ClassifySessionModel}
	for _, call := range item.Calls {
		label := classifyViolates
		report.Calls = append(report.Calls, hostproto.ReportedCall{Question: call.Question, Index: call.Index, Model: call.Model, Label: &label, Attempts: 1})
	}
	encoded, _ := json.Marshal(report)
	outcome, err := f.service.ImportClassification(ClassifyImport{
		Ticket: f.ticket.TicketID, Nonce: f.ticket.Nonce, Source: f.source.String(), Report: encoded,
		ProofKind: format.KindAlignment, Mode: hostproto.ClassifyAuto, Admission: f.admission,
	})
	if err != nil || outcome.Kind != ClassifyAnswered {
		t.Fatalf("outcome %+v (%v)", outcome, err)
	}
	return f, item
}

func (f *classifyFixture) effective() format.ExecutorProfile {
	f.t.Helper()
	records, err := f.store.Records()
	if err != nil {
		f.t.Fatal(err)
	}
	var events []hostproto.Event
	for _, record := range records {
		if record.Event.Ticket == f.ticket.TicketID.String() {
			events = append(events, record.Event)
		}
	}
	return executorrun.Effective(f.response().Executor, events)
}

func TestWitnessClassifications(t *testing.T) {
	inside := hostprototest.Epoch.Add(-time.Minute)
	honest := func(hostproto.ClassifiedCall) string { return classifyViolates }

	t.Run("a classification the raw API log holds is raised to restricted with the resolved model, and its entries are consumed [verifies SP-1909~1] [verifies SP-1913~1]", func(t *testing.T) {
		f, item := witnessFixture(t)
		if got := f.effective(); got.Isolation != format.IsolationRestrictedUnverified {
			t.Fatalf("before the check: %+v", got)
		}
		report, err := f.service.WitnessClassifications(rawLog(t, item, inside, honest))
		if err != nil || report.Raised != 1 {
			t.Fatalf("report %+v (%v)", report, err)
		}
		got := f.effective()
		if got.Isolation != format.IsolationRestricted || got.Model != (format.ExecutorModel{Name: witnessModel, Source: format.ModelSourceResolved}) {
			t.Fatalf("profile %+v", got)
		}
		if !f.journalHas(hostproto.EventWitnessConsumed) || !f.journalHas(hostproto.EventClassifyWitnessed) {
			t.Fatalf("journal %v", f.journalKinds())
		}
		again, err := f.service.WitnessClassifications(rawLog(t, item, inside, honest))
		if err != nil || again.Raised != 0 {
			t.Fatalf("a second check: %+v (%v)", again, err)
		}
	})

	t.Run("no entry witnesses two classifications [verifies SP-1913~1]", func(t *testing.T) {
		f, item := witnessFixture(t)
		other, otherItem := witnessFixture(t)
		_ = otherItem
		log := rawLog(t, item, inside, honest)
		if report, err := f.service.WitnessClassifications(log); err != nil || report.Raised != 1 {
			t.Fatalf("first: %+v (%v)", report, err)
		}
		consumed := f.lastDetail(hostproto.EventWitnessConsumed)
		other.service.Store.Append(hostproto.Event{Kind: hostproto.EventWitnessConsumed, Ticket: other.ticket.TicketID.String(), Detail: consumed})
		if report, err := other.service.WitnessClassifications(log); err != nil || report.Raised != 0 || other.effective().Isolation != format.IsolationRestrictedUnverified {
			t.Fatalf("second: %+v (%v), profile %+v", report, err, other.effective())
		}
	})

	t.Run("a log that contradicts the labels marks no exposure and raises nothing [verifies SP-1913~1]", func(t *testing.T) {
		f, item := witnessFixture(t)
		report, err := f.service.WitnessClassifications(rawLog(t, item, inside, func(call hostproto.ClassifiedCall) string {
			if call.Index == 0 {
				return classifyConsistent
			}
			return classifyViolates
		}))
		if err != nil || report.Contradicted != 1 || !f.journalHas(hostproto.EventClassifyLabelContradicted) || f.effective().Isolation != format.IsolationRestrictedUnverified {
			t.Fatalf("report %+v (%v) journal %v", report, err, f.journalKinds())
		}
	})

	t.Run("an extra logged call, a re-ask, settles nothing [verifies SP-1913~1]", func(t *testing.T) {
		f, item := witnessFixture(t)
		report, err := f.service.WitnessClassifications(rawLog(t, item, inside, honest, item.Calls[0]))
		if err != nil || report.Raised != 0 || f.effective().Isolation != format.IsolationRestrictedUnverified {
			t.Fatalf("report %+v (%v)", report, err)
		}
	})

	t.Run("a log outside the window of the ticket settles nothing [verifies SP-1913~1]", func(t *testing.T) {
		f, item := witnessFixture(t)
		report, err := f.service.WitnessClassifications(rawLog(t, item, hostprototest.Epoch.Add(time.Hour), honest))
		if err != nil || report.Raised != 0 {
			t.Fatalf("report %+v (%v)", report, err)
		}
	})

	t.Run("a classification made by a host version the check was not established for stays unraised [verifies SP-1913~1]", func(t *testing.T) {
		f := newClassifyFixture(t)
		item := f.plan(hostproto.ClassifyShadow)
		if _, err := f.importWith(hostproto.ClassifyAuto, hostprototest.Always(classifyViolates), f.admission); err != nil {
			t.Fatal(err)
		}
		if report, err := f.service.WitnessClassifications(rawLog(t, item, inside, honest)); err != nil || report.Raised != 0 || f.effective().Isolation != format.IsolationRestrictedUnverified {
			t.Fatalf("report %+v (%v)", report, err)
		}
	})

	t.Run("the check never raises a response of another channel [verifies SP-1909~1]", func(t *testing.T) {
		f := newFixture(t)
		f.show()
		if _, err := f.service.Submit(f.submission(validAnswer)); err != nil {
			t.Fatal(err)
		}
		if report, err := f.service.WitnessClassifications(hostproto.FileLog{Files: hostprototest.MemoryFiles{}}); err != nil || report != (WitnessReport{}) {
			t.Fatalf("report %+v (%v)", report, err)
		}
	})
}
