package drain_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/s1relay/internal/app/drain"
	"github.com/RachidChabane/musthold/s1relay/internal/app/ports"
	"github.com/RachidChabane/musthold/s1relay/internal/domain/endpoint"
	"github.com/RachidChabane/musthold/s1relay/internal/domain/failure"
	"github.com/RachidChabane/musthold/s1relay/internal/domain/question"
)

const modelName = "tev1:0.8b"

type imported struct {
	ticket, nonce, source string
	reply                 []byte
}

// fakeModule serves a fixed export and records the imports and the order of events.
type fakeModule struct {
	exported  []ports.Exported
	exportErr error
	importErr map[string]error
	imports   []imported
	events    *[]string
}

func (m *fakeModule) Export(context.Context) ([]ports.Exported, error) {
	return m.exported, m.exportErr
}

func (m *fakeModule) Import(_ context.Context, ticket, nonce, source string, reply []byte) error {
	*m.events = append(*m.events, "import "+ticket)
	if err := m.importErr[ticket]; err != nil {
		return err
	}
	m.imports = append(m.imports, imported{ticket, nonce, source, reply})
	return nil
}

// fakeSystemOne answers by ticket-independent script: errs[i] is the error of the
// i-th call, else the first option of every question.
type fakeSystemOne struct {
	errs    map[int]error
	answers func(set question.Set) question.Answers
	events  *[]string
	calls   int
	active  int
	maximum int
}

func (s *fakeSystemOne) Decide(_ context.Context, set question.Set) (ports.Decision, error) {
	index := s.calls
	s.calls++
	s.active++
	s.maximum = max(s.maximum, s.active)
	defer func() { s.active-- }()
	*s.events = append(*s.events, fmt.Sprintf("decide %d", index))
	if err := s.errs[index]; err != nil {
		return ports.Decision{}, err
	}
	if s.answers != nil {
		return ports.Decision{Model: modelName, Answers: s.answers(set), Reply: replyOf(index)}, nil
	}
	return ports.Decision{Model: modelName, Answers: firstOptions(set), Reply: replyOf(index)}, nil
}

// replyOf is the body the fake endpoint answered to the i-th call.
func replyOf(index int) []byte { return []byte(fmt.Sprintf(`{"reply":%d}`, index)) }

func firstOptions(set question.Set) question.Answers {
	answers := make(question.Answers, len(set.Questions))
	for index, q := range set.Questions {
		answers[index] = question.Answer{QuestionID: q.ID, Choice: q.Options[0].Name}
	}
	return answers
}

type fakeGuard struct {
	verdicts []ports.MemoryVerdict
	err      error
	calls    int
}

func (g *fakeGuard) Check(context.Context) (ports.MemoryVerdict, error) {
	index := min(g.calls, len(g.verdicts)-1)
	g.calls++
	return g.verdicts[index], g.err
}

type fakeUnloader struct {
	result ports.UnloadResult
	err    error
	models []string
}

func (u *fakeUnloader) Unload(_ context.Context, model string) (ports.UnloadResult, error) {
	u.models = append(u.models, model)
	return u.result, u.err
}

type recordingLog struct{ lines []string }

func (l *recordingLog) Logf(format string, args ...any) {
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

type rig struct {
	module   *fakeModule
	systemOn *fakeSystemOne
	guard    *fakeGuard
	unloader *fakeUnloader
	log      *recordingLog
	events   []string
	service  drain.Service
}

func newRig(t *testing.T, tickets int) *rig {
	t.Helper()
	address, err := endpoint.Parse("http://localhost:11434")
	if err != nil {
		t.Fatal(err)
	}
	r := &rig{log: &recordingLog{}, guard: &fakeGuard{verdicts: []ports.MemoryVerdict{{Allowed: true}}}, unloader: &fakeUnloader{result: ports.UnloadResult{Applicable: true}}}
	r.module = &fakeModule{events: &r.events, importErr: map[string]error{}}
	r.systemOn = &fakeSystemOne{events: &r.events, errs: map[int]error{}}
	for index := range tickets {
		r.module.exported = append(r.module.exported, exportedRequest(fmt.Sprintf("t%d", index)))
	}
	r.service = drain.Service{Module: r.module, Endpoint: address, SystemOne: r.systemOn, Guard: r.guard, Unloader: r.unloader, Log: r.log}
	return r
}

func exportedRequest(ticket string) ports.Exported {
	return ports.Exported{Ticket: ticket, Nonce: "n-" + ticket, Set: question.Set{
		State:     "state of " + ticket,
		Questions: []question.Question{{ID: "q1", Type: question.TypeChoice, Text: "Which?", Options: []question.Option{{Name: "A"}, {Name: "B"}}}},
	}}
}

func (r *rig) run(t *testing.T, options drain.Options) drain.Report {
	t.Helper()
	report, err := r.service.Run(context.Background(), options)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return report
}

func TestRunImportsEachAnswer(t *testing.T) {
	t.Run("each request is posted and its answers imported under the model and the endpoint digest [verifies SP-600~2]", func(t *testing.T) {
		r := newRig(t, 2)
		report := r.run(t, drain.Options{})
		if report.Answered() != 2 || report.Unanswered() != 0 || report.Remaining != 0 {
			t.Fatalf("report %+v", report)
		}
		wantSource := r.service.Endpoint.Source(modelName)
		for index, got := range r.module.imports {
			ticket := fmt.Sprintf("t%d", index)
			if got.ticket != ticket || got.nonce != "n-"+ticket || got.source != wantSource || string(got.reply) != fmt.Sprintf(`{"reply":%d}`, index) {
				t.Errorf("import %d = %+v", index, got)
			}
		}
		if !strings.HasPrefix(wantSource, "system-one:"+modelName+"@") || report.Items[0].Source != wantSource {
			t.Errorf("source %q, item %+v", wantSource, report.Items[0])
		}
	})

	t.Run("nothing exported means nothing posted and nothing unloaded [verifies SP-600~2]", func(t *testing.T) {
		r := newRig(t, 0)
		report := r.run(t, drain.Options{Unload: true})
		if len(report.Items) != 0 || r.systemOn.calls != 0 || len(r.unloader.models) != 0 || report.Unload.Attempted {
			t.Fatalf("report %+v, %d posts, unloads %v", report, r.systemOn.calls, r.unloader.models)
		}
	})

	t.Run("a module that cannot export is an error and posts nothing [verifies SP-602~1]", func(t *testing.T) {
		r := newRig(t, 1)
		r.module.exportErr = errors.New("musthold-dp: no project")
		if _, err := r.service.Run(context.Background(), drain.Options{}); err == nil || r.systemOn.calls != 0 {
			t.Fatalf("error %v, %d posts", err, r.systemOn.calls)
		}
	})
}

func TestRunOneRequestAtATime(t *testing.T) {
	t.Run("the next request is posted only after the previous one is imported [verifies SP-609~1]", func(t *testing.T) {
		r := newRig(t, 3)
		r.run(t, drain.Options{})
		want := "decide 0,import t0,decide 1,import t1,decide 2,import t2"
		if got := strings.Join(r.events, ","); got != want {
			t.Fatalf("events %s, want %s", got, want)
		}
		if r.systemOn.maximum != 1 {
			t.Fatalf("%d requests were open at once", r.systemOn.maximum)
		}
	})
}

func TestRunLeavesUnusableRequestsUnanswered(t *testing.T) {
	t.Run("a failed request is reported with its kind and the next one goes on [verifies SP-607~1]", func(t *testing.T) {
		r := newRig(t, 4)
		r.systemOn.errs[0] = failure.New(failure.KindTimeout, "no reply in time")
		r.systemOn.errs[1] = failure.New(failure.KindRejected, "HTTP 400: state too large")
		r.systemOn.errs[2] = failure.New(failure.KindServer, "HTTP 503")
		report := r.run(t, drain.Options{})
		kinds := []failure.Kind{failure.KindTimeout, failure.KindRejected, failure.KindServer, ""}
		for index, item := range report.Items {
			if item.Kind != kinds[index] {
				t.Errorf("item %d kind %q, want %q", index, item.Kind, kinds[index])
			}
		}
		if report.Answered() != 1 || report.Unanswered() != 3 || len(r.module.imports) != 1 || r.module.imports[0].ticket != "t3" {
			t.Fatalf("report %+v, imports %+v", report, r.module.imports)
		}
		if !strings.Contains(report.Items[1].Detail, "state too large") {
			t.Errorf("detail %q", report.Items[1].Detail)
		}
	})

	t.Run("an answer outside the options is never imported [verifies SP-606~1]", func(t *testing.T) {
		r := newRig(t, 2)
		r.systemOn.answers = func(set question.Set) question.Answers {
			return question.Answers{{QuestionID: "q1", Choice: "ELSE"}}
		}
		report := r.run(t, drain.Options{})
		if report.Answered() != 0 || len(r.module.imports) != 0 {
			t.Fatalf("report %+v", report)
		}
		if report.Items[0].Kind != failure.KindMalformed || !strings.Contains(report.Items[0].Detail, "ELSE") {
			t.Errorf("item %+v", report.Items[0])
		}
	})

	t.Run("a partial answer is never imported [verifies SP-606~1]", func(t *testing.T) {
		r := newRig(t, 1)
		r.module.exported[0].Set.Questions = append(r.module.exported[0].Set.Questions, question.Question{ID: "q2", Type: question.TypeChoice, Options: []question.Option{{Name: "X"}}})
		r.systemOn.answers = func(question.Set) question.Answers { return question.Answers{{QuestionID: "q1", Choice: "A"}} }
		if report := r.run(t, drain.Options{}); report.Items[0].Kind != failure.KindMalformed || len(r.module.imports) != 0 {
			t.Fatalf("report %+v", report)
		}
	})

	t.Run("a request the relay cannot read is not posted and is reported [verifies SP-616~1]", func(t *testing.T) {
		r := newRig(t, 2)
		r.module.exported[0].Problem = question.ErrUnsupportedType
		report := r.run(t, drain.Options{})
		if r.systemOn.calls != 1 || report.Items[0].Kind != failure.KindUnsupported || report.Items[1].Status != drain.StatusAnswered {
			t.Fatalf("report %+v, %d posts", report, r.systemOn.calls)
		}
	})

	t.Run("a module that refuses the answers is reported and the drain goes on [verifies SP-614~1]", func(t *testing.T) {
		r := newRig(t, 2)
		r.module.importErr["t0"] = failure.New(failure.KindImport, "nonce mismatch")
		report := r.run(t, drain.Options{})
		if report.Items[0].Kind != failure.KindImport || report.Items[0].Detail != "nonce mismatch" || report.Items[1].Status != drain.StatusAnswered {
			t.Fatalf("report %+v", report)
		}
	})
}

func TestRunStopsOnAFailingEndpoint(t *testing.T) {
	timeout := failure.New(failure.KindTimeout, "no reply in time")

	t.Run("after three failures of the endpoint in a row no further request is posted [verifies SP-608~1]", func(t *testing.T) {
		r := newRig(t, 6)
		for index := range 6 {
			r.systemOn.errs[index] = timeout
		}
		report := r.run(t, drain.Options{})
		if r.systemOn.calls != drain.ConsecutiveFailureLimit {
			t.Fatalf("%d requests were posted", r.systemOn.calls)
		}
		for _, item := range report.Items[drain.ConsecutiveFailureLimit:] {
			if item.Kind != failure.KindSkipped || item.Status != drain.StatusUnanswered || !strings.Contains(item.Detail, "in a row") {
				t.Errorf("item %+v", item)
			}
		}
	})

	t.Run("a success or a refusal of one request resets the count [verifies SP-608~1]", func(t *testing.T) {
		r := newRig(t, 6)
		r.systemOn.errs[0], r.systemOn.errs[1] = timeout, timeout
		r.systemOn.errs[2] = failure.New(failure.KindRejected, "HTTP 400")
		r.systemOn.errs[3], r.systemOn.errs[4] = timeout, timeout
		r.run(t, drain.Options{})
		if r.systemOn.calls != 6 {
			t.Fatalf("%d requests were posted", r.systemOn.calls)
		}
	})
}

func TestRunMaxRequests(t *testing.T) {
	t.Run("at most N requests are posted and the rest is counted [verifies SP-610~1]", func(t *testing.T) {
		r := newRig(t, 5)
		report := r.run(t, drain.Options{MaxRequests: 2})
		if r.systemOn.calls != 2 || len(report.Items) != 2 || report.Remaining != 3 {
			t.Fatalf("%d posts, report %+v", r.systemOn.calls, report)
		}
	})

	t.Run("a request the relay cannot read does not use the limit [verifies SP-610~1]", func(t *testing.T) {
		r := newRig(t, 3)
		r.module.exported[0].Problem = question.ErrUnsupportedType
		report := r.run(t, drain.Options{MaxRequests: 1})
		if r.systemOn.calls != 1 || report.Remaining != 1 {
			t.Fatalf("%d posts, report %+v", r.systemOn.calls, report)
		}
	})

	t.Run("a limit of zero drains everything [verifies SP-610~1]", func(t *testing.T) {
		r := newRig(t, 4)
		if report := r.run(t, drain.Options{MaxRequests: drain.Unlimited}); len(report.Items) != 4 || report.Remaining != 0 {
			t.Fatalf("report %+v", report)
		}
	})
}

func TestRunUnload(t *testing.T) {
	t.Run("after a batch that posted something the model is unloaded by the name the endpoint answered with [verifies SP-611~1]", func(t *testing.T) {
		r := newRig(t, 2)
		report := r.run(t, drain.Options{Unload: true})
		if len(r.unloader.models) != 1 || r.unloader.models[0] != modelName || !report.Unload.Attempted || !report.Unload.Done {
			t.Fatalf("unloads %v, report %+v", r.unloader.models, report.Unload)
		}
	})

	t.Run("the unload is still attempted after a batch whose requests all failed [verifies SP-611~1]", func(t *testing.T) {
		r := newRig(t, 1)
		r.systemOn.errs[0] = failure.New(failure.KindRejected, "HTTP 400")
		report := r.run(t, drain.Options{Unload: true})
		if !report.Unload.Attempted || len(r.unloader.models) != 1 {
			t.Fatalf("report %+v", report.Unload)
		}
	})

	t.Run("no unload without --unload, and none for a server that is not Ollama, and a failure is only reported [verifies SP-611~1]", func(t *testing.T) {
		r := newRig(t, 1)
		r.run(t, drain.Options{Unload: false})
		if len(r.unloader.models) != 0 {
			t.Fatal("the model was unloaded against the option")
		}
		r = newRig(t, 1)
		r.unloader.result = ports.UnloadResult{Detail: "the endpoint is not an Ollama server"}
		report := r.run(t, drain.Options{Unload: true})
		if report.Unload.Done || report.Unload.Detail == "" {
			t.Fatalf("report %+v", report.Unload)
		}
		r = newRig(t, 1)
		r.unloader.err = errors.New("connection reset")
		report = r.run(t, drain.Options{Unload: true})
		if report.Unload.Done || !strings.Contains(report.Unload.Detail, "connection reset") || report.Answered() != 1 {
			t.Fatalf("report %+v, answered %d", report.Unload, report.Answered())
		}
	})
}

func TestRunMemoryRule(t *testing.T) {
	allowed := ports.MemoryVerdict{Allowed: true}
	refused := ports.MemoryVerdict{Reason: "12% of memory is free, the rule needs 40%"}

	t.Run("a refused batch exports nothing and posts nothing [verifies SP-612~1]", func(t *testing.T) {
		r := newRig(t, 2)
		r.guard.verdicts = []ports.MemoryVerdict{refused}
		_, err := r.service.Run(context.Background(), drain.Options{MemGuard: true})
		if !errors.Is(err, drain.ErrMemoryRefused) || !strings.Contains(err.Error(), "12%") || r.systemOn.calls != 0 || len(r.events) != 0 {
			t.Fatalf("error %v, %d posts", err, r.systemOn.calls)
		}
	})

	t.Run("a rule that cannot be read refuses the batch [verifies SP-612~1]", func(t *testing.T) {
		r := newRig(t, 1)
		r.guard.err = errors.New("memory_pressure not found")
		if _, err := r.service.Run(context.Background(), drain.Options{MemGuard: true}); !errors.Is(err, drain.ErrMemoryRefused) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a rule that does not apply is skipped, logged and not asked again [verifies SP-612~1]", func(t *testing.T) {
		r := newRig(t, 3)
		r.guard.verdicts = []ports.MemoryVerdict{{Skipped: true, Reason: "memory_pressure is not available on linux"}}
		report := r.run(t, drain.Options{MemGuard: true})
		if report.Answered() != 3 || r.guard.calls != 1 || !strings.Contains(strings.Join(r.log.lines, "\n"), "not available on linux") {
			t.Fatalf("report %+v, %d checks, log %q", report, r.guard.calls, r.log.lines)
		}
	})

	t.Run("the rule is asked before each request and stops the batch when it refuses between two [verifies SP-612~1]", func(t *testing.T) {
		r := newRig(t, 4)
		r.guard.verdicts = []ports.MemoryVerdict{allowed, allowed, refused}
		report := r.run(t, drain.Options{MemGuard: true})
		if r.systemOn.calls != 1 || report.Answered() != 1 {
			t.Fatalf("%d posts, report %+v", r.systemOn.calls, report)
		}
		for _, item := range report.Items[1:] {
			if item.Kind != failure.KindSkipped || !strings.Contains(item.Detail, "memory rule") {
				t.Errorf("item %+v", item)
			}
		}
	})

	t.Run("without the option the rule is never asked [verifies SP-612~1]", func(t *testing.T) {
		r := newRig(t, 2)
		r.guard.verdicts = []ports.MemoryVerdict{refused}
		r.run(t, drain.Options{})
		if r.guard.calls != 0 {
			t.Fatalf("the rule was asked %d times", r.guard.calls)
		}
	})
}
