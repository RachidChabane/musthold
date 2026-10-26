package scan_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/app/scan"
	"github.com/RachidChabane/musthold/discpower/internal/domain/promise"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
)

const (
	verifiesSources     = " [verifies SP-2221~1]"
	verifiesExtraction  = " [verifies SP-2222~1]"
	verifiesNoSource    = " [verifies SP-2223~1]"
	verifiesWriters     = " [verifies SP-2224~1]"
	verifiesAdmission   = " [verifies SP-2225~1]"
	verifiesStageThree  = " [verifies SP-2226~1]"
	verifiesContinues   = " [verifies SP-2227~1]"
	verifiesAwaiting    = " [verifies SP-2228~1]"
	verifiesContradict  = " [verifies SP-2229~1]"
	verifiesOneProcess  = " [verifies SP-2230~1]"
	verifiesModels      = " [verifies SP-2231~1]"
	verifiesWitnessed   = " [verifies SP-2232~1]"
	verifiesSixRuns     = " [verifies SP-2233~1]"
	verifiesBroken      = " [verifies SP-2235~1]"
	verifiesLabels      = " [verifies SP-2236~1]"
	verifiesSessionEnd  = " [verifies SP-2237~1]"
	verifiesSessionKey  = " [verifies SP-2238~1]"
	verifiesResume      = " [verifies SP-2239~1]"
	verifiesStableIDs   = " [verifies SP-2240~1]"
	verifiesBudget      = " [verifies SP-2243~1]"
	verifiesBinding     = " [verifies SP-2259~1]"
	verifiesPullRequest = " [verifies SP-2261~1]"
)

func TestHostStages(t *testing.T) {
	f := newHostFixture()
	f.host.promises = []promiseAnswer{{Quote: quoteZero, Source: format.ScanSourceReadme}, {Quote: quoteAbsent, Source: format.ScanSourceReadme}}
	req := hostRequest()

	first := f.invoke(t, req)
	t.Run("a fresh scan saves its sources and asks the one extraction, then waits"+verifiesExtraction, func(t *testing.T) {
		if first.Finished || first.ExitCode() != scan.ExitAwaitingHost || !slices.Equal(pendingKinds(t, f.store, first), []hostproto.Kind{hostproto.KindGeneratePromises}) {
			t.Fatalf("result %+v", first)
		}
		if saved, err := f.sources.Load(headCommit, format.ScanSourceReadme); err != nil || string(saved) != readmeText {
			t.Fatalf("saved %q, %v", saved, err)
		}
	})
	t.Run("the extraction shows the README under its name, and no diff in repository mode"+verifiesSources, func(t *testing.T) {
		request, err := f.store.GetRequest(first.Pending[0].RequestID)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(request.Prompt.User, "Source README.md:") || strings.Contains(request.Prompt.User, "Files the change touches") {
			t.Fatalf("prompt %s", request.Prompt.User)
		}
	})
	t.Run("the session is a scan session, live, its binding journaled once"+verifiesBinding, func(t *testing.T) {
		if !first.Session.IsScan() || first.Wave != 1 {
			t.Fatalf("session %s wave %d", first.Session, first.Wave)
		}
		records, err := f.store.Records()
		if err != nil {
			t.Fatal(err)
		}
		bound := 0
		for _, record := range records {
			if record.Event.Kind == hostproto.EventSessionBound && record.Event.Session == first.Session {
				bound++
			}
		}
		if bound != 1 {
			t.Fatalf("bound %d times", bound)
		}
	})
	if calls := f.suite.took(); calls != 0 {
		t.Fatalf("the suite ran %d times before the final stage", calls)
	}

	f.host.answer(t, first.Session)
	req.Resume = true
	second := f.invoke(t, req)
	t.Run("the quotes are checked on the saved bytes and the kept promise gets two writers"+verifiesWriters, func(t *testing.T) {
		if second.Finished || !slices.Equal(pendingKinds(t, f.store, second), []hostproto.Kind{hostproto.KindGenerateWitness, hostproto.KindGenerateWitness}) {
			t.Fatalf("pending %v", pendingKinds(t, f.store, second))
		}
		if second.Session != first.Session || second.Wave != 2 {
			t.Fatalf("session %s wave %d", second.Session, second.Wave)
		}
	})

	f.host.answer(t, second.Session)
	third := f.invoke(t, req)
	t.Run("the admission asks two scope votes, four generation tickets and the labels ticket"+verifiesStageThree, func(t *testing.T) {
		want := []hostproto.Kind{hostproto.KindGenerateTargeted, hostproto.KindGenerateTargeted, hostproto.KindGenerateTargeted, hostproto.KindGenerateTargeted,
			hostproto.KindJudgeLabels, hostproto.KindJudgeScope, hostproto.KindJudgeScope}
		if got := pendingKinds(t, f.store, third); !slices.Equal(got, want) {
			t.Fatalf("pending %v", got)
		}
	})
	t.Run("each witness set ran on the reference three times and its digest was journaled first"+verifiesAdmission, func(t *testing.T) {
		if f.runs.onSource["test_va_zero"] != 3 || f.runs.onSource["test_vb_zero"] != 3 {
			t.Fatalf("runs on the reference %v", f.runs.onSource)
		}
		records, err := f.store.Records()
		if err != nil {
			t.Fatal(err)
		}
		committed := slices.IndexFunc(records, func(record hostproto.Record) bool { return record.Event.Kind == hostproto.EventWitnessSetCommitted })
		if committed < 0 {
			t.Fatal("no witness set was journaled")
		}
	})
	if calls := f.suite.took(); calls != 0 {
		t.Fatalf("the suite ran %d times before the final stage", calls)
	}

	f.host.answer(t, third.Session)
	fourth := f.invoke(t, req)
	t.Run("the final stage runs the suite in its one invocation and finishes"+verifiesOneProcess, func(t *testing.T) {
		if !fourth.Finished || fourth.ExitCode() != scan.ExitFinished || len(fourth.Pending) != 0 {
			t.Fatalf("result %+v", fourth)
		}
		if calls := f.suite.took(); calls == 0 {
			t.Fatal("the suite never ran")
		}
	})
	report := fourth.Report
	statement := report.Statement.Predicate
	t.Run("the statement validates and records the checked promise, its witnesses and the broken promise"+verifiesBroken, func(t *testing.T) {
		if _, err := format.AdmitScan(report.Encoded); err != nil {
			t.Fatal(err)
		}
		if !statement.PromisesChecked || len(statement.Promises) != 1 || statement.Promises[0].Quote != quoteZero {
			t.Fatalf("promises %+v", statement.Promises)
		}
		if len(statement.BrokenPromises) == 0 {
			t.Fatal("no broken promise")
		}
		if report.Promises == nil || len(report.Promises.Broken) == 0 || report.Promises.Broken[0].Promise.Quote != quoteZero {
			t.Fatalf("report %+v", report.Promises)
		}
	})
	t.Run("the model mutant joins the draw with origin model"+verifiesModels, func(t *testing.T) {
		if !slices.ContainsFunc(statement.Mutants, func(mutant format.ScanMutant) bool { return mutant.Origin == format.ScanOriginModel }) {
			t.Fatal("no model mutant")
		}
		if report.Promises.ModelMutants != 1 {
			t.Fatalf("model mutants %d", report.Promises.ModelMutants)
		}
	})
	t.Run("a stable witness that fails a mutant no test killed proves it by a witness_failure route"+verifiesWitnessed, func(t *testing.T) {
		routed := slices.ContainsFunc(statement.Mutants, func(mutant format.ScanMutant) bool {
			return slices.ContainsFunc(mutant.Routes, func(route format.ScanRoute) bool { return route.Kind == format.ScanRouteWitness })
		})
		if !routed {
			t.Fatal("no witness_failure route")
		}
	})
	t.Run("the dropped quote is reported, and the model calls are counted"+verifiesBudget, func(t *testing.T) {
		if len(report.Promises.Dropped) != 1 || report.Promises.Dropped[0].Reason != promise.DropQuoteAbsent {
			t.Fatalf("dropped %+v", report.Promises.Dropped)
		}
		if report.Promises.Calls != emissions(t, f.store, fourth.Session) || report.Promises.Calls != 1+2+2+4+1 {
			t.Fatalf("calls %d", report.Promises.Calls)
		}
	})
	t.Run("the labels name tests whose changes sit in a labelled function, and change no verdict"+verifiesLabels, func(t *testing.T) {
		labelled := slices.ContainsFunc(statement.Tests, func(test format.ScanTest) bool { return len(test.Promises) > 0 })
		if !labelled {
			t.Fatal("no test is labelled")
		}
		plain := run(t, newFixture(), request()).Statement.Predicate
		for _, test := range statement.Tests {
			if classOfTest(plain.Tests, test.ID) == "" {
				continue
			}
			if got := classOfTest(plain.Tests, test.ID); got != test.Class && !touchedByModel(statement, test) {
				t.Fatalf("%s is %s with labels and %s without", test.ID, test.Class, got)
			}
		}
	})
	t.Run("writing the statement ends the session"+verifiesSessionEnd, func(t *testing.T) {
		states, err := f.store.Sessions()
		if err != nil {
			t.Fatal(err)
		}
		index := slices.IndexFunc(states, func(state hostproto.SessionState) bool { return state.Session == fourth.Session })
		if index < 0 || !states[index].Ended {
			t.Fatalf("states %+v", states)
		}
		entries, err := f.store.ListEntries()
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.State != hostproto.StateConsumed {
				t.Fatalf("ticket %s is %s once the statement is written", entry.Ticket.TicketID, entry.State)
			}
		}
	})
}

func classOfTest(tests []format.ScanTest, id string) format.ScanClass {
	for _, test := range tests {
		if test.ID == id {
			return test.Class
		}
	}
	return ""
}

// touchedByModel reports whether a model mutant is among the test's changes:
// its verdict may differ from a scan without one.
func touchedByModel(statement format.Scan, test format.ScanTest) bool {
	for _, change := range test.Changes {
		for _, mutant := range statement.Mutants {
			if mutant.ID == change.Mutant && mutant.Origin == format.ScanOriginModel {
				return true
			}
		}
	}
	return false
}

func TestHostStageWithNothingToAsk(t *testing.T) {
	t.Run("a scan with no quotable source asks nothing and finishes in its first invocation"+verifiesNoSource, func(t *testing.T) {
		f := newHostFixture()
		f.head = fakeHead{}
		result := f.invoke(t, hostRequest())
		if !result.Finished || emissions(t, f.store, result.Session) != 0 {
			t.Fatalf("result %+v", result)
		}
		promises := result.Report.Promises
		if promises == nil || !promises.Checked || !promises.NoSource || !promises.NoPromise() || !result.Report.Statement.Predicate.PromisesChecked {
			t.Fatalf("promises %+v", promises)
		}
	})
	t.Run("a scan whose extracted quotes are all absent continues to the final stage in the invocation that read them"+verifiesContinues, func(t *testing.T) {
		f := newHostFixture()
		f.host.promises = []promiseAnswer{{Quote: quoteAbsent, Source: format.ScanSourceReadme}}
		results := f.loop(t, hostRequest(), 3)
		if len(results) != 2 || !results[1].Finished || !results[1].Report.Promises.NoPromise() {
			t.Fatalf("%d invocations, last %+v", len(results), results[len(results)-1])
		}
	})
}

func TestHostAwaiting(t *testing.T) {
	t.Run("an invocation that waits lists the tickets of its own session only"+verifiesAwaiting, func(t *testing.T) {
		f := newHostFixture()
		other := hostprototestRequest(t)
		bridge := proveBridge(t, f)
		if _, err := bridge.Want(other); err != nil {
			t.Fatal(err)
		}
		result := f.invoke(t, hostRequest())
		if len(result.Pending) != 1 || result.Pending[0].RequestID == mustID(t, other.Request) {
			t.Fatalf("pending %+v", result.Pending)
		}
	})
}

func TestHostContradiction(t *testing.T) {
	t.Run("a generation answer with no usable mutant is rejected and asked again before any suite run"+verifiesContradict, func(t *testing.T) {
		f := newHostFixture()
		unusable := honestGeneration("        return total +")
		f.host.answers[hostproto.KindGenerateTargeted] = func(_ hostproto.Request, _, times int) string {
			if times == 0 {
				return unusable
			}
			return honestGeneration(breaksTotal)
		}
		req := hostRequest()
		for range 3 {
			result := f.invoke(t, req)
			f.host.answer(t, result.Session)
			req.Resume = true
		}
		f.suite.took()
		contradicted := f.invoke(t, req)
		if contradicted.Finished || f.suite.took() != 0 || len(contradicted.Pending) == 0 {
			t.Fatalf("result %+v", contradicted)
		}
		f.host.answer(t, contradicted.Session)
		if final := f.invoke(t, req); !final.Finished {
			t.Fatalf("result %+v", final)
		}
	})
}

func TestHostSixRuns(t *testing.T) {
	t.Run("a witness that fails one run of the final stage becomes a question and proves nothing"+verifiesSixRuns, func(t *testing.T) {
		f := newHostFixture()
		f.runs.failRuns["test_va_zero"] = []int{5}
		results := f.loop(t, hostRequest(), 5)
		statement := results[len(results)-1].Report.Statement.Predicate
		var unstable string
		for _, one := range statement.Witnesses {
			if one.Name == "test_va_zero" {
				unstable = one.ID
				if one.Admitted || one.BlockBearing {
					t.Fatalf("witness %+v", one)
				}
			}
		}
		if !slices.ContainsFunc(statement.Questions, func(question format.ScanQuestion) bool { return question.Witness == unstable }) {
			t.Fatalf("questions %+v", statement.Questions)
		}
		for _, mutant := range statement.Mutants {
			for _, route := range mutant.Routes {
				if route.Witness == unstable {
					t.Fatalf("mutant %s has a route of the unstable witness", mutant.ID)
				}
			}
		}
		questions := results[len(results)-1].Report.Promises.Questions
		if !slices.ContainsFunc(questions, func(question scan.PromiseQuestion) bool { return question.Reason == promise.QuestionUnstable }) {
			t.Fatalf("questions %+v", questions)
		}
	})
}

func TestHostStableIDs(t *testing.T) {
	t.Run("a reference flaky across invocations leaves the requests of the first three stages byte-identical"+verifiesStableIDs, func(t *testing.T) {
		f := newHostFixture()
		req := hostRequest()
		var before []hostproto.RequestID
		for range 3 {
			result := f.invoke(t, req)
			before = sessionRequests(t, f.store, result.Session)
			f.host.answer(t, result.Session)
			req.Resume = true
		}
		f.runs.failRuns["test_va_free"] = []int{4, 5, 6, 7, 8, 9}
		final := f.invoke(t, req)
		if !final.Finished {
			t.Fatalf("result %+v", final)
		}
		if after := sessionRequests(t, f.store, final.Session); !slices.Equal(before, after) {
			t.Fatalf("requests before %v, after %v", before, after)
		}
	})
}

func TestHostResume(t *testing.T) {
	t.Run("a resume whose binding differs from the open session exits with a tool failure that names the difference"+verifiesResume, func(t *testing.T) {
		f := newHostFixture()
		first := f.invoke(t, hostRequest())
		changed := hostRequest()
		changed.Seed, changed.Resume = 8, true
		_, err := f.hostService(t).RunHost(context.Background(), changed)
		if !errors.Is(err, scan.ErrSessionMismatch) || !strings.Contains(err.Error(), "seed 7 → 8") {
			t.Fatalf("error %v", err)
		}
		same := hostRequest()
		same.Resume = true
		if resumed := f.invoke(t, same); resumed.Session != first.Session {
			t.Fatalf("resumed %s, opened %s", resumed.Session, first.Session)
		}
	})
	t.Run("a resume with no open scan session has nothing to resume"+verifiesResume, func(t *testing.T) {
		f := newHostFixture()
		req := hostRequest()
		req.Resume = true
		if _, err := f.hostService(t).RunHost(context.Background(), req); !errors.Is(err, scan.ErrNothingToResume) {
			t.Fatalf("error %v", err)
		}
	})
	t.Run("the session key follows the binding: another seed is another session"+verifiesSessionKey, func(t *testing.T) {
		f := newHostFixture()
		first := f.invoke(t, hostRequest())
		other := hostRequest()
		other.Seed = 8
		second := f.invoke(t, other)
		if first.Session == second.Session {
			t.Fatalf("one session %s", first.Session)
		}
		states, err := f.store.Sessions()
		if err != nil {
			t.Fatal(err)
		}
		for _, state := range states {
			if state.Session == first.Session && !state.Ended {
				t.Fatal("a new scan left the older scan session open")
			}
		}
	})
}

func TestHostBudget(t *testing.T) {
	t.Run("a reissued writer cuts the labels ticket and every promise keeps its two scope votes"+verifiesBudget, func(t *testing.T) {
		f := newHostFixture()
		f.host.promises = []promiseAnswer{
			{Quote: quoteZero, Source: format.ScanSourceReadme}, {Quote: quoteShipping, Source: format.ScanSourceReadme}, {Quote: quoteRounded, Source: format.ScanSourceReadme},
		}
		rejected := false
		f.host.answers[hostproto.KindGenerateWitness] = func(_ hostproto.Request, vote, times int) string {
			if !rejected && vote == 1 && times == 0 {
				rejected = true
				return `{"imports":"","witnesses":[]}`
			}
			return honestWitnesses(vote)
		}
		results := f.loop(t, hostRequest(), 6)
		report := results[len(results)-1].Report.Promises
		if !slices.Contains(report.Cut, promise.Cut{Ask: promise.LabelTicket(), Reason: promise.CutBudget}) {
			t.Fatalf("cut %+v", report.Cut)
		}
		if report.Calls > promise.Budget || len(report.Unchecked) != 0 {
			t.Fatalf("calls %d, unchecked %+v", report.Calls, report.Unchecked)
		}
		scopes := 0
		for _, result := range results {
			scopes += countKind(t, f.store, result, hostproto.KindJudgeScope)
		}
		if scopes != promise.MaxPromises*promise.ScopeVotesPerPromise {
			t.Fatalf("%d scope votes", scopes)
		}
	})
}

func countKind(t *testing.T, store hostproto.HostStore, result scan.HostResult, kind hostproto.Kind) int {
	t.Helper()
	count := 0
	for _, pending := range pendingKinds(t, store, result) {
		if pending == kind {
			count++
		}
	}
	return count
}

func TestHostPullRequest(t *testing.T) {
	t.Run("scan <PR> measures the change from the merge base and quotes the description and the closing issues"+verifiesPullRequest, func(t *testing.T) {
		f := newHostFixture()
		f.source.changed = map[string][]int{totals: {3}}
		f.pulls = &fakePullRequests{pull: scan.PullRequest{Title: "Keep the total", Body: "Zero percent keeps the total.", ClosingIssues: []scan.Issue{{Number: 4, Title: "Zero discount", Body: "It must keep the total."}}}}
		req := hostRequest()
		req.PullRequest = 12
		first := f.invoke(t, req)
		request, err := f.store.GetRequest(first.Pending[0].RequestID)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"Source description:", "Source issue #4:", "Files the change touches:", totals, "Public signatures"} {
			if !strings.Contains(request.Prompt.User, want) {
				t.Fatalf("prompt lacks %q:\n%s", want, request.Prompt.User)
			}
		}
		if strings.Contains(request.Prompt.User, keepsTotal) {
			t.Fatalf("the prompt shows a body:\n%s", request.Prompt.User)
		}
	})
	t.Run("scan <PR> without a host is a change scan against the pull request's merge base"+verifiesPullRequest, func(t *testing.T) {
		f := newHostFixture()
		f.source.changed = map[string][]int{totals: {4}}
		f.pulls = &fakePullRequests{}
		req := request()
		req.PullRequest = 12
		report, err := f.replayService(t).Run(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		if report.Mode != format.ScanModeChange || report.Base != baseCommit || report.Promises != nil {
			t.Fatalf("mode %s base %s promises %+v", report.Mode, report.Base, report.Promises)
		}
	})
}
