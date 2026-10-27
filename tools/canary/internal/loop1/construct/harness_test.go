package construct_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
)

const (
	validMessage  = `{"files":[{"path":"a.py","content":"x = 1\n"}]}`
	shortTimeout  = 20 * time.Millisecond
	harnessDomain = "d1"
)

// classificationCases lists what a failed call prints and the kind it shows.
var classificationCases = []struct {
	name   string
	result construct.ProcessResult
	want   construct.HarnessKind
}{
	{"not logged in", construct.ProcessResult{Stderr: []byte("Error: Not logged in"), ExitCode: 1}, construct.HarnessAuthentication},
	{"401 on standard output", construct.ProcessResult{Stdout: []byte("request failed: 401"), ExitCode: 1}, construct.HarnessAuthentication},
	{"unauthorized", construct.ProcessResult{Stderr: []byte("Unauthorized"), ExitCode: 1}, construct.HarnessAuthentication},
	{"usage limit", construct.ProcessResult{Stderr: []byte("You've hit your usage limit."), ExitCode: 1}, construct.HarnessUsageLimit},
	{"rate limit", construct.ProcessResult{Stderr: []byte("rate limit reached"), ExitCode: 1}, construct.HarnessUsageLimit},
	{"429", construct.ProcessResult{Stderr: []byte("status 429"), ExitCode: 1}, construct.HarnessUsageLimit},
	{"try again", construct.ProcessResult{Stderr: []byte("Please try again in 3 hours"), ExitCode: 1}, construct.HarnessUsageLimit},
	{"network", construct.ProcessResult{Stderr: []byte("dial tcp 10.0.0.1:443: connection refused"), ExitCode: 1}, construct.HarnessNetwork},
	{"no such host", construct.ProcessResult{Stderr: []byte("lookup api: no such host"), ExitCode: 1}, construct.HarnessNetwork},
	{"no final message", construct.ProcessResult{Stderr: []byte("something broke"), ExitCode: 2}, construct.HarnessNoMessage},
}

func TestClassifyOutcome(t *testing.T) {
	t.Run("a call that failed shows its kind of harness failure from its output text or its missing final message [verifies SP-2650~1]", func(t *testing.T) {
		for _, tc := range classificationCases {
			failure := construct.ClassifyOutcome(tc.result, nil)
			if failure == nil || failure.Kind != tc.want {
				t.Errorf("%s: %+v, want %s", tc.name, failure, tc.want)
			}
			if failure != nil && !errors.Is(failure, construct.ErrHarnessFailure) {
				t.Errorf("%s: the failure does not match ErrHarnessFailure", tc.name)
			}
		}
	})

	t.Run("an empty final message is a harness failure when the output shows a limit, and a model failure otherwise [verifies SP-2650~1]", func(t *testing.T) {
		limited := construct.ProcessResult{Stderr: []byte("usage limit"), ExitCode: 0}
		if failure := construct.ClassifyOutcome(limited, []byte("  \n")); failure == nil || failure.Kind != construct.HarnessUsageLimit {
			t.Errorf("%+v", failure)
		}
		if failure := construct.ClassifyOutcome(construct.ProcessResult{}, nil); failure != nil {
			t.Errorf("an empty message after a clean exit is the model's: %+v", failure)
		}
	})

	t.Run("a call that exited cleanly with a final message is the model's answer, whatever its output says [verifies SP-2650~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			text := rapid.SampledFrom([]string{"401", "429", "rate limit", "try again", "Not logged in", "network", "usage limit"}).Draw(t, "text")
			noise := rapid.String().Draw(t, "noise")
			result := construct.ProcessResult{Stderr: []byte(noise + text), Stdout: []byte(text + noise)}
			if failure := construct.ClassifyOutcome(result, []byte(validMessage)); failure != nil {
				t.Fatalf("%+v", failure)
			}
		})
	})

	t.Run("a non-zero exit that left a final message and shows no harness text is the model's failure [verifies SP-2650~1]", func(t *testing.T) {
		result := construct.ProcessResult{Stderr: []byte("stderr text"), ExitCode: 3}
		if failure := construct.ClassifyOutcome(result, []byte(validMessage)); failure != nil {
			t.Errorf("%+v", failure)
		}
	})
}

func TestCodexHarnessFailures(t *testing.T) {
	t.Run("every kind of harness failure comes out of the adapter as an ErrHarnessFailure that still carries the call's output [verifies SP-2650~1]", func(t *testing.T) {
		for _, tc := range classificationCases {
			fake, model := newCodex(t)
			fake.message = ""
			fake.exit = tc.result.ExitCode
			model.Process = processFunc(func(argv []string) (construct.ProcessResult, error) {
				result, err := fake.Run(t.Context(), argv)
				if len(argv) > 2 {
					result.Stderr, result.Stdout = tc.result.Stderr, tc.result.Stdout
				}
				return result, err
			})
			reply, err := model.Complete(t.Context(), construct.Request{Prompt: "p", Schema: schemaFor(t)})
			var failure *construct.HarnessFailure
			if !errors.Is(err, construct.ErrHarnessFailure) || !errors.As(err, &failure) || failure.Kind != tc.want {
				t.Errorf("%s: %v", tc.name, err)
			}
			if reply.CodexVersion == "" || reply.Exit != tc.result.ExitCode {
				t.Errorf("%s: %+v", tc.name, reply)
			}
		}
	})

	t.Run("a call that outlasts its time limit is killed and is a harness failure; the limit is thirty minutes [verifies SP-2650~1]", func(t *testing.T) {
		if construct.CallTimeout != 30*time.Minute {
			t.Fatalf("the limit is %s", construct.CallTimeout)
		}
		_, model := newCodex(t)
		model.Timeout = shortTimeout
		var deadline time.Time
		model.Process = blockingProcess{onExec: func(ctx context.Context) { deadline, _ = ctx.Deadline() }}
		started := time.Now()
		_, err := model.Complete(t.Context(), construct.Request{Prompt: "p", Schema: schemaFor(t)})
		var failure *construct.HarnessFailure
		if !errors.As(err, &failure) || failure.Kind != construct.HarnessTimeout || !errors.Is(err, construct.ErrHarnessFailure) {
			t.Fatalf("%v", err)
		}
		if deadline.IsZero() || time.Since(started) > 10*time.Second {
			t.Errorf("the call had no deadline or ran on: %v", deadline)
		}
	})

	t.Run("a cancellation by the caller is not a harness failure [verifies SP-2650~1]", func(t *testing.T) {
		_, model := newCodex(t)
		model.Process = blockingProcess{}
		ctx, cancel := context.WithCancel(t.Context())
		time.AfterFunc(shortTimeout, cancel)
		_, err := model.Complete(ctx, construct.Request{Prompt: "p", Schema: schemaFor(t)})
		if !errors.Is(err, context.Canceled) || errors.Is(err, construct.ErrHarnessFailure) {
			t.Fatalf("%v", err)
		}
	})

	t.Run("a codex that is missing or fails its version is a harness failure [verifies SP-2650~1]", func(t *testing.T) {
		_, model := newCodex(t)
		model.Process = processFunc(func([]string) (construct.ProcessResult, error) { return construct.ProcessResult{ExitCode: 127}, nil })
		_, err := model.Complete(t.Context(), construct.Request{Prompt: "p", Schema: schemaFor(t)})
		if !errors.Is(err, construct.ErrHarnessFailure) || !errors.Is(err, construct.ErrCodexVersion) {
			t.Fatalf("%v", err)
		}
		model.Process = processFunc(func([]string) (construct.ProcessResult, error) {
			return construct.ProcessResult{}, errors.New("not found")
		})
		if _, err := model.Complete(t.Context(), construct.Request{Prompt: "p", Schema: schemaFor(t)}); !errors.Is(err, construct.ErrHarnessFailure) {
			t.Fatalf("%v", err)
		}
	})

	t.Run("a final message that is not the expected document stays a model failure [verifies SP-2650~1]", func(t *testing.T) {
		fake, model := newCodex(t)
		fake.message = "not json"
		_, err := model.Complete(t.Context(), construct.Request{Prompt: "p", Schema: schemaFor(t)})
		if !errors.Is(err, construct.ErrBadReply) || errors.Is(err, construct.ErrHarnessFailure) {
			t.Fatalf("%v", err)
		}
	})
}

// blockingProcess answers `codex --version` and holds `codex exec` until its
// context ends.
type blockingProcess struct{ onExec func(ctx context.Context) }

func (p blockingProcess) Run(ctx context.Context, argv []string) (construct.ProcessResult, error) {
	if len(argv) == 2 {
		return construct.ProcessResult{Stdout: []byte(fakeVersion)}, nil
	}
	if p.onExec != nil {
		p.onExec(ctx)
	}
	<-ctx.Done()
	return construct.ProcessResult{ExitCode: -1}, nil
}

// flaky is a Model that fails on the harness side at chosen calls: it fails the
// request for which stopAt returns true, once, and otherwise delegates.
type flaky struct {
	mu       sync.Mutex
	inner    construct.Model
	stopAt   func(construct.Request) bool
	failures int
	requests []construct.Request
}

func (f *flaky) Complete(ctx context.Context, request construct.Request) (construct.Reply, error) {
	f.mu.Lock()
	f.requests = append(f.requests, request)
	stop := f.stopAt != nil && f.stopAt(request)
	if stop {
		f.failures++
	}
	f.mu.Unlock()
	if stop {
		return construct.Reply{Stderr: []byte("usage limit"), CodexVersion: fakeVersion, Exit: 1}, &construct.HarnessFailure{Kind: construct.HarnessUsageLimit, Detail: "exit 1"}
	}
	return f.inner.Complete(ctx, request)
}

// passingHandler answers every role correctly.
func passingHandler(w func() *world) func(construct.Request) ([]construct.File, error) {
	return func(request construct.Request) ([]construct.File, error) {
		if request.Role == records.RoleOracle {
			return oracleFiles(w().sample), nil
		}
		return authorFiles(w().sample, nil), nil
	}
}

func newPassingWorld(t *testing.T, names ...string) *world {
	var w *world
	w = newWorld(t, domains(names...), passingHandler(func() *world { return w }))
	w.markBoxofficeBuilt()
	w.constructor.Target = len(names)
	return w
}

// callKey identifies a call that the model answered, whenever it was made.
func callKey(call records.Call) string {
	return fmt.Sprintf("%s/%s/%s#%d:%s", call.Domain, call.Role, call.Purpose, call.Attempt, call.PromptSHA256)
}

func answeredCalls(log records.Log) []string {
	var keys []string
	for _, call := range log.Calls {
		if call.HarnessFailure == "" {
			keys = append(keys, callKey(call))
		}
	}
	return keys
}

func TestAHarnessFailureStopsConstruction(t *testing.T) {
	t.Run("a harness failure stops the run at once, uses no repair, drops nothing and is recorded as a harness failure [verifies SP-2651~1]", func(t *testing.T) {
		w := newPassingWorld(t, harnessDomain)
		failing := &flaky{inner: w.model, stopAt: func(request construct.Request) bool { return request.Role == records.RoleAuthor }}
		w.constructor.Model = failing
		summary, err := w.constructor.Run(t.Context())
		if !errors.Is(err, construct.ErrHarnessFailure) || errors.Is(err, construct.ErrTooFewTasks) {
			t.Fatalf("%v", err)
		}
		if len(summary.Dropped) != 0 || len(summary.Accepted) != 0 || len(failing.requests) != 1 {
			t.Fatalf("%+v after %d calls", summary, len(failing.requests))
		}
		log := w.log()
		if len(log.Drops) != 0 || len(log.Calls) != 1 || len(log.Checks) != 0 {
			t.Fatalf("%+v", log)
		}
		call := log.Calls[0]
		if call.HarnessFailure != string(construct.HarnessUsageLimit) || call.Error == "" || call.Purpose != records.PurposeGenerate || call.Attempt != 1 || call.Domain != harnessDomain {
			t.Fatalf("%+v", call)
		}
		if w.constructor.Store.Exists(harnessDomain) && w.constructor.Store.Complete(harnessDomain) {
			t.Error("a task was left complete")
		}
	})

	t.Run("a harness failure in a repair call does not count as the repair, in either role [verifies SP-2547~2]", func(t *testing.T) {
		for _, role := range []records.Role{records.RoleAuthor, records.RoleOracle} {
			var w *world
			w = newWorld(t, domains(repairedDomain), threeDomainHandler(func() *world { return w }))
			w.markBoxofficeBuilt()
			w.constructor.Target = 1
			failing := &flaky{inner: w.model, stopAt: func(request construct.Request) bool {
				return request.Role == role && request.Purpose == records.PurposeRepair
			}}
			w.constructor.Model = failing
			if _, err := w.constructor.Run(t.Context()); !errors.Is(err, construct.ErrHarnessFailure) {
				t.Fatalf("%s: %v", role, err)
			}
			for _, call := range w.log().Calls {
				if call.Role == role && call.Purpose == records.PurposeRepair && call.HarnessFailure == "" {
					t.Errorf("%s: a repair call was answered: %+v", role, call)
				}
			}
			failing.stopAt = nil
			summary, err := w.constructor.Run(t.Context())
			if err != nil || len(summary.Dropped) != 0 || len(summary.Accepted) != 1 {
				t.Fatalf("%s: %v %+v", role, err, summary)
			}
			for _, call := range w.log().Calls {
				if call.Purpose == records.PurposeRepair && call.HarnessFailure == "" && call.Attempt > 1+construct.MaxRepairs {
					t.Errorf("%s: attempt %d", role, call.Attempt)
				}
			}
		}
	})

	t.Run("a model failure still counts as a repair call [verifies SP-2547~2]", func(t *testing.T) {
		var w *world
		w = newWorld(t, domains("d1"), func(request construct.Request) ([]construct.File, error) {
			if request.Role == records.RoleOracle {
				return oracleFiles(w.sample), nil
			}
			if request.Purpose == records.PurposeGenerate {
				return nil, construct.ErrBadReply
			}
			return authorFiles(w.sample, nil), nil
		})
		w.markBoxofficeBuilt()
		w.constructor.Target = 1
		if _, err := w.constructor.Run(t.Context()); err != nil {
			t.Fatal(err)
		}
		if log := w.log(); len(log.Calls) != 3 || log.Calls[0].HarnessFailure != "" || log.Calls[1].Attempt != 2 {
			t.Fatalf("%+v", log.Calls)
		}
	})
}

func TestConstructionResumesWhereItStopped(t *testing.T) {
	t.Run("the next invocation repeats the same domain, role and attempt, and makes no call that was already answered [verifies SP-2652~1]", func(t *testing.T) {
		w := newPassingWorld(t, harnessDomain)
		failing := &flaky{inner: w.model, stopAt: func(request construct.Request) bool { return request.Role == records.RoleOracle }}
		w.constructor.Model = failing
		if _, err := w.constructor.Run(t.Context()); !errors.Is(err, construct.ErrHarnessFailure) {
			t.Fatalf("%v", err)
		}
		before := len(w.model.requests)
		failing.stopAt = nil
		failing.requests = nil
		summary, err := w.constructor.Run(t.Context())
		if err != nil || len(summary.Accepted) != 1 {
			t.Fatalf("%v %+v", err, summary)
		}
		if len(failing.requests) != 1 {
			t.Fatalf("%d calls on resume: %+v", len(failing.requests), failing.requests)
		}
		resumed := failing.requests[0]
		if resumed.Domain != harnessDomain || resumed.Role != records.RoleOracle || resumed.Attempt != 1 || resumed.Purpose != records.PurposeGenerate {
			t.Fatalf("%+v", resumed)
		}
		if len(w.model.requests) != before+1 {
			t.Errorf("the model answered %d calls on resume", len(w.model.requests)-before)
		}
	})

	t.Run("a run that is interrupted and resumed leaves the same answered calls and the same outcome as one that is not [verifies SP-2652~1]", func(t *testing.T) {
		clean := newThreeDomainWorld(t)
		cleanSummary, _ := clean.constructor.Run(t.Context())
		want := answeredCalls(clean.log())

		for _, stopRole := range []records.Role{records.RoleAuthor, records.RoleOracle} {
			for _, stopPurpose := range []records.Purpose{records.PurposeGenerate, records.PurposeRepair} {
				w := newThreeDomainWorld(t)
				failing := &flaky{inner: w.model, stopAt: func(request construct.Request) bool {
					return request.Domain == droppedDomain && request.Role == stopRole && request.Purpose == stopPurpose && request.Attempt >= 1
				}}
				w.constructor.Model = failing
				if _, err := w.constructor.Run(t.Context()); !errors.Is(err, construct.ErrHarnessFailure) {
					t.Fatalf("%s %s: %v", stopRole, stopPurpose, err)
				}
				failing.stopAt = nil
				summary, err := w.constructor.Run(t.Context())
				if !errors.Is(err, construct.ErrTooFewTasks) {
					t.Fatalf("%s %s: %v", stopRole, stopPurpose, err)
				}
				if len(summary.Accepted) != 0 || len(summary.Dropped) != len(cleanSummary.Dropped) {
					t.Errorf("%s %s: %+v", stopRole, stopPurpose, summary)
				}
				if got := answeredCalls(w.log()); strings.Join(got, "\n") != strings.Join(want, "\n") {
					t.Errorf("%s %s: answered calls differ\n got %v\nwant %v", stopRole, stopPurpose, got, want)
				}
				if len(w.log().Drops) != 1 || w.log().Drops[0].Domain != droppedDomain {
					t.Errorf("%s %s: drops %+v", stopRole, stopPurpose, w.log().Drops)
				}
			}
		}
	})

	t.Run("resuming twice after two harness failures in a row still repeats the same attempt [verifies SP-2652~1]", func(t *testing.T) {
		w := newPassingWorld(t, harnessDomain)
		failing := &flaky{inner: w.model, stopAt: func(construct.Request) bool { return true }}
		w.constructor.Model = failing
		for range 2 {
			if _, err := w.constructor.Run(t.Context()); !errors.Is(err, construct.ErrHarnessFailure) {
				t.Fatalf("%v", err)
			}
		}
		for _, request := range failing.requests {
			if request.Role != records.RoleAuthor || request.Attempt != 1 || request.Purpose != records.PurposeGenerate {
				t.Errorf("%+v", request)
			}
		}
		failing.stopAt = nil
		if summary, err := w.constructor.Run(t.Context()); err != nil || len(summary.Accepted) != 1 {
			t.Fatalf("%v %+v", err, summary)
		}
	})

	t.Run("a domain the records show as dropped stays dropped and is not built again [verifies SP-2540~2]", func(t *testing.T) {
		w := newThreeDomainWorld(t)
		if _, err := w.constructor.Run(t.Context()); !errors.Is(err, construct.ErrTooFewTasks) {
			t.Fatalf("%v", err)
		}
		calls := len(w.model.requests)
		again, err := w.constructor.Run(t.Context())
		if !errors.Is(err, construct.ErrTooFewTasks) || len(w.model.requests) != calls {
			t.Fatalf("%v, %d calls more", err, len(w.model.requests)-calls)
		}
		if len(again.Dropped) != 1 || again.Dropped[0].Domain != droppedDomain {
			t.Fatalf("%+v", again.Dropped)
		}
	})

	t.Run("records that no longer match the calls refuse the resume [verifies SP-2652~1]", func(t *testing.T) {
		w := newPassingWorld(t, harnessDomain)
		failing := &flaky{inner: w.model, stopAt: func(request construct.Request) bool { return request.Role == records.RoleOracle }}
		w.constructor.Model = failing
		if _, err := w.constructor.Run(t.Context()); !errors.Is(err, construct.ErrHarnessFailure) {
			t.Fatalf("%v", err)
		}
		w.constructor.Domains = []construct.Domain{{Name: harnessDomain, Line: "another subject entirely"}}
		failing.stopAt = nil
		if _, err := w.constructor.Run(t.Context()); !errors.Is(err, construct.ErrResumeDiverged) {
			t.Fatalf("%v", err)
		}
	})
}
