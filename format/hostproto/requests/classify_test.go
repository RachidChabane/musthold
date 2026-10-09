package requests

import (
	"errors"
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	classifyViolates   = "VIOLATES"
	classifyConsistent = "CONSISTENT"
)

// classifyFixture is a store holding the first vote of a closed request.
type classifyFixture struct {
	*fixture
	source    format.ClassifySource
	admission hostproto.ClassifyAdmission
}

func newClassifyFixture(t testing.TB) *classifyFixture {
	t.Helper()
	f := &classifyFixture{fixture: newFixture(t)}
	f.service.Validator = hostprototest.MemberValidator{Member: "answers"}
	f.request, f.ticket = f.emit(hostprototest.ClosedRequest(), 0)
	var err error
	if f.source, err = format.SingleVoterClassifySource(hostproto.ClassifyCalls, hostproto.ClassifyScheme, hostprototest.ClassifySessionModel); err != nil {
		t.Fatal(err)
	}
	f.admission = hostprototest.ClassifyAdmissionOf(t, format.KindAlignment, f.source, hostprototest.ClassifySessionModel)
	return f
}

// plan lists the calls of the fixture's ticket.
func (f *classifyFixture) plan(mode hostproto.ClassifyMode) hostproto.ClassifyPlanItem {
	f.t.Helper()
	listing, err := f.service.List()
	if err != nil {
		f.t.Fatal(err)
	}
	policy := hostproto.ClassifyPolicy{Kind: format.KindAlignment, Mode: mode, Admission: f.admission, SessionModel: hostprototest.ClassifySessionModel}
	items, err := hostproto.ClassifyPlans(listing, f.source, policy)
	if err != nil || len(items) != 1 {
		f.t.Fatalf("items %v (%v)", items, err)
	}
	return items[0]
}

func (f *classifyFixture) importWith(mode hostproto.ClassifyMode, answer func(hostproto.ClassifiedCall) string, admission hostproto.ClassifyAdmission) (ClassifyOutcome, error) {
	f.t.Helper()
	item := f.plan(hostproto.ClassifyShadow)
	return f.service.ImportClassification(ClassifyImport{
		Ticket: f.ticket.TicketID, Nonce: f.ticket.Nonce, Source: f.source.String(), Report: hostprototest.ClassifyReportOf(f.t, item, answer),
		ProofKind: format.KindAlignment, Mode: mode, Admission: admission,
	})
}

func (f *classifyFixture) journalHas(kind hostproto.EventKind) bool {
	return slices.Contains(f.journalKinds(), kind)
}

func TestImportClassification(t *testing.T) {
	t.Run("an admitted classification with an open gate answers the ticket on the host-classify channel at the unverified isolation [verifies SP-1908~1]", func(t *testing.T) {
		f := newClassifyFixture(t)
		outcome, err := f.importWith(hostproto.ClassifyAuto, hostprototest.Always(classifyViolates), f.admission)
		if err != nil || outcome.Kind != ClassifyAnswered {
			t.Fatalf("outcome %+v (%v)", outcome, err)
		}
		response := f.response()
		if f.state() != hostproto.StateAccepted || response.Classify == nil {
			t.Fatalf("state %s response %+v", f.state(), response)
		}
		if executor := response.Executor; executor.Channel != format.ChannelHostClassify || executor.Isolation != format.IsolationRestrictedUnverified ||
			executor.Model != (format.ExecutorModel{Name: hostprototest.ClassifySessionModel, Source: format.ModelSourceDeclared}) ||
			executor.Harness.Name != "claude-code" || executor.Harness.Version != hostprototest.ClassifyCLIVersion {
			t.Fatalf("executor %+v", executor)
		}
		if err := response.Validate(); err != nil {
			t.Fatal(err)
		}
		record := response.Classify
		if len(record.Calls) != hostproto.ClassifyCalls || record.Questions[0].Modal != classifyViolates || record.Questions[0].Confidence != 0.99 {
			t.Fatalf("record %+v", record)
		}
		answers, err := hostproto.ParseClosedAnswers(response.Output)
		if err != nil || answers[hostprototest.ClosedQuestionID].Choice != classifyViolates {
			t.Fatalf("output %s (%v)", response.Output, err)
		}
		if !f.journalHas(hostproto.EventClassifyAnswered) || f.journalHas(hostproto.EventExposed) {
			t.Fatalf("journal %v", f.journalKinds())
		}
	})

	t.Run("a closed gate leaves the ticket untouched for the host votes and journals why [verifies SP-1908~1]", func(t *testing.T) {
		f := newClassifyFixture(t)
		mixed := func(call hostproto.ClassifiedCall) string {
			if call.Index < 3 {
				return classifyViolates
			}
			return classifyConsistent
		}
		outcome, err := f.importWith(hostproto.ClassifyAuto, mixed, f.admission)
		if err != nil || outcome.Kind != ClassifyFellThrough || outcome.Reason != string(hostproto.GateBelowTau) {
			t.Fatalf("outcome %+v (%v)", outcome, err)
		}
		if f.state() != hostproto.StateEmitted || !f.journalHas(hostproto.EventClassifyFellThrough) {
			t.Fatalf("state %s journal %v", f.state(), f.journalKinds())
		}
		if _, err := f.service.Show(f.ticket.TicketID, ""); err != nil {
			t.Fatalf("a host executor cannot fetch the ticket: %v", err)
		}
	})

	t.Run("a classification no admission entry covers leaves the ticket to the host votes [verifies SP-1908~1]", func(t *testing.T) {
		f := newClassifyFixture(t)
		outcome, err := f.importWith(hostproto.ClassifyAuto, hostprototest.Always(classifyViolates), hostproto.ClassifyAdmission{})
		if err != nil || outcome.Kind != ClassifyNotAdmitted || outcome.Reason != reasonNotAdmitted {
			t.Fatalf("outcome %+v (%v)", outcome, err)
		}
		if f.state() != hostproto.StateEmitted {
			t.Fatalf("state %s", f.state())
		}
	})

	t.Run("an admission entry of another decision kind does not cover the classification [verifies SP-1908~1]", func(t *testing.T) {
		f := newClassifyFixture(t)
		other := hostprototest.ClassifyAdmissionOf(t, format.KindCompleteness, f.source, hostprototest.ClassifySessionModel)
		outcome, err := f.importWith(hostproto.ClassifyAuto, hostprototest.Always(classifyViolates), other)
		if err != nil || outcome.Kind != ClassifyNotAdmitted {
			t.Fatalf("outcome %+v (%v)", outcome, err)
		}
	})

	t.Run("shadow journals the classification, whatever the admission table says, and answers nothing [verifies SP-1908~1]", func(t *testing.T) {
		f := newClassifyFixture(t)
		outcome, err := f.importWith(hostproto.ClassifyShadow, hostprototest.Always(classifyViolates), f.admission)
		if err != nil || outcome.Kind != ClassifyShadowed {
			t.Fatalf("outcome %+v (%v)", outcome, err)
		}
		if f.state() != hostproto.StateEmitted || !f.journalHas(hostproto.EventClassifyShadow) {
			t.Fatalf("state %s journal %v", f.state(), f.journalKinds())
		}
		detail := f.lastDetail(hostproto.EventClassifyShadow)
		parsed, ok := hostproto.ParseClassifyDetail(detail)
		if !ok || parsed.CLIVersion != hostprototest.ClassifyCLIVersion || parsed.Record == nil || len(parsed.Record.Calls) != hostproto.ClassifyCalls {
			t.Fatalf("detail %q", detail)
		}
		unadmitted, err := newClassifyFixture(t).importWith(hostproto.ClassifyShadow, hostprototest.Always(classifyViolates), hostproto.ClassifyAdmission{})
		if err != nil || unadmitted.Kind != ClassifyShadowed {
			t.Fatalf("shadow without an admission entry: %+v (%v)", unadmitted, err)
		}
	})

	t.Run("off does nothing at all [verifies SP-1908~1]", func(t *testing.T) {
		f := newClassifyFixture(t)
		before := len(f.journalKinds())
		outcome, err := f.service.ImportClassification(ClassifyImport{Ticket: f.ticket.TicketID, Nonce: f.ticket.Nonce, Source: "not a source", Mode: hostproto.ClassifyOff})
		if err != nil || outcome.Kind != ClassifyOff || len(f.journalKinds()) != before || f.state() != hostproto.StateEmitted {
			t.Fatalf("outcome %+v (%v) journal %v", outcome, err, f.journalKinds())
		}
	})

	refusals := map[string]struct {
		prep func(f *classifyFixture)
		edit func(in *ClassifyImport)
		want error
	}{
		"another nonce": {edit: func(in *ClassifyImport) { in.Nonce = wrongNonce }, want: hostproto.ErrNonceMismatch},
		"a vote after the first": {prep: func(f *classifyFixture) {
			f.ticket = hostprototest.Ticket(f.request, 1, hostproto.FirstEmission)
			f.emit(f.request, 1)
		}, want: hostproto.ErrClassifyNotFirstVote},
		"a ticket a host executor fetched": {prep: func(f *classifyFixture) { f.show() }, want: hostproto.ErrNotSubmittable},
		"a source off the grammar":         {edit: func(in *ClassifyImport) { in.Source = "host-classify(5,rot1):a*4" }, want: format.ErrClassifySource},
		"a report that is not JSON":        {edit: func(in *ClassifyImport) { in.Report = []byte("{") }, want: hostproto.ErrClassifyReport},
		"an unknown ticket": {edit: func(in *ClassifyImport) {
			in.Ticket = hostprototest.Ticket(hostprototest.Request(func(r *hostproto.Request) { r.Prompt.User = "other" }), 0, hostproto.FirstEmission).TicketID
		}, want: hostproto.ErrNotFound},
	}
	for name, tt := range refusals {
		t.Run("an import for "+name+" is refused and leaves the ticket as it was [verifies SP-1908~1]", func(t *testing.T) {
			f := newClassifyFixture(t)
			item := f.plan(hostproto.ClassifyShadow)
			if tt.prep != nil {
				tt.prep(f)
			}
			in := ClassifyImport{
				Ticket: f.ticket.TicketID, Nonce: f.ticket.Nonce, Source: f.source.String(), ProofKind: format.KindAlignment,
				Mode: hostproto.ClassifyAuto, Admission: f.admission, Report: hostprototest.ClassifyReportOf(t, item, hostprototest.Always(classifyViolates)),
			}
			if tt.edit != nil {
				tt.edit(&in)
			}
			stateBefore := f.state()
			if _, err := f.service.ImportClassification(in); !errors.Is(err, tt.want) {
				t.Fatalf("error %v, want %v", err, tt.want)
			}
			if f.state() != stateBefore {
				t.Fatalf("the state moved from %s to %s", stateBefore, f.state())
			}
			if _, err := f.store.GetResponse(f.ticket.Key()); err == nil {
				t.Fatal("a refused import left a response")
			}
		})
	}

	t.Run("a report that does not match the plan is a report error and changes nothing [verifies SP-1908~1]", func(t *testing.T) {
		f := newClassifyFixture(t)
		in := ClassifyImport{
			Ticket: f.ticket.TicketID, Nonce: f.ticket.Nonce, Source: f.source.String(), ProofKind: format.KindAlignment,
			Mode: hostproto.ClassifyAuto, Admission: f.admission,
			Report: []byte(`{"cliVersion":"2.1.293","sessionModel":"claude-sonnet-5-5","calls":[]}`),
		}
		if _, err := f.service.ImportClassification(in); !errors.Is(err, hostproto.ErrClassifyReport) {
			t.Fatalf("error %v", err)
		}
		if f.state() != hostproto.StateEmitted {
			t.Fatalf("state %s", f.state())
		}
	})

	t.Run("the refusals of an import are the caller's, not the tool's [verifies SP-1908~1]", func(t *testing.T) {
		for _, err := range []error{hostproto.ErrClassifyReport, hostproto.ErrNonceMismatch, format.ErrClassifySource, hostproto.ErrClassifyNotFirstVote, hostproto.ErrClassifyVoters, hostproto.ErrClassifyMode} {
			if !IsClassifyRefusal(err) {
				t.Errorf("%v is not a refusal", err)
			}
		}
		if IsClassifyRefusal(errors.New("disk full")) {
			t.Error("a tool failure is a refusal")
		}
	})
}

func (f *classifyFixture) lastDetail(kind hostproto.EventKind) string {
	f.t.Helper()
	records, err := f.store.Records()
	if err != nil {
		f.t.Fatal(err)
	}
	for _, record := range slices.Backward(records) {
		if record.Event.Kind == kind {
			return record.Event.Detail
		}
	}
	return ""
}
