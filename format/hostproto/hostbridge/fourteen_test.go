package hostbridge

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
	"github.com/RachidChabane/musthold/format/hostproto/requests"
)

// The run the fake host drives: 14 criteria, each proved by 2 modules with a
// store of their own, each criterion asking a writer, then two judges.
const (
	verifiesFourteen     = " [verifies SP-2691~1]"
	fourteenCriteria     = 14
	unitsPerCriterion    = 3
	unitsPerStep         = 5
	judgeVotes           = 2
	maxDriverSteps       = 400
	maxStepsPerAnswer    = 30
	answeredOutput       = `{"validity":"VALID"}`
	orphanAgent          = "agent-orphan"
	purposeWriter        = "writer"
	purposeJudge         = "judge"
	moduleDiscriminating = "discpower"
	moduleAlignment      = "alignment"
)

// simulatedModule is a module of the run: its store, the units of deterministic
// work it memoized, the criteria it concluded and how.
type simulatedModule struct {
	name    string
	store   *hostprototest.MemoryHostStore
	memo    map[string]bool
	proved  map[int]bool
	fresh   map[int]bool
	pending format.Pending
	// spoiled is the criterion whose writer answers execution contradicts until
	// it is renewed, which makes it abandoned once.
	spoiled int
	renewed bool
}

// budgetContext is the context of one step: it is spent, as a step whose
// budget ran out is, once the step did its units of work.
type budgetContext struct {
	context.Context
	cancel context.CancelCauseFunc
	left   int
}

func newStepBudget() *budgetContext {
	ctx, cancel := context.WithCancelCause(context.Background())
	return &budgetContext{Context: ctx, cancel: cancel, left: unitsPerStep}
}

// work spends one unit, and spends the step once its units are gone.
func (b *budgetContext) work() error {
	if b.left == 0 {
		b.cancel(hostproto.ErrStepSpent)
	}
	if err := b.Err(); err != nil {
		return context.Cause(b)
	}
	b.left--
	return nil
}

// step is one `prove` call of the module: it opens its pass, does the
// deterministic work it can within the budget, asks for what it needs, and
// concludes every criterion whose answers are in. It reports whether the step
// was unfinished and what the session waits for.
func (m *simulatedModule) step(t *testing.T, clock func() time.Time, renew bool) (hostproto.StepEnd, hostproto.Outstanding) {
	t.Helper()
	release, err := m.store.LockRun(hostproto.SessionProve)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	budget := newStepBudget()
	bridge, err := Open(m.store, clock, &sequenceReader{next: uint64(len(m.memo)) * waveSeedStride}, hostproto.SessionProve)
	if err != nil {
		t.Fatal(err)
	}
	bridge.Renew = renew
	for criterion := range fourteenCriteria {
		if err := m.proveCriterion(t, bridge, budget, criterion); errors.Is(err, hostproto.ErrStepSpent) {
			return hostproto.StepBudgetSpent, m.outstanding(t, bridge)
		} else if err != nil {
			t.Fatal(err)
		}
	}
	outstanding, err := bridge.FinishWave()
	if err != nil {
		t.Fatal(err)
	}
	return hostproto.StepFinished, outstanding
}

func (m *simulatedModule) outstanding(t *testing.T, bridge *Bridge) hostproto.Outstanding {
	t.Helper()
	outstanding, err := bridge.Outstanding()
	if err != nil {
		t.Fatal(err)
	}
	return outstanding
}

func (m *simulatedModule) proveCriterion(t *testing.T, bridge *Bridge, budget *budgetContext, criterion int) error {
	if m.fresh[criterion] {
		return nil
	}
	for unit := range unitsPerCriterion {
		key := fmt.Sprintf("%d/%d", criterion, unit)
		if m.memo[key] {
			continue
		}
		if err := budget.work(); err != nil {
			return err
		}
		m.memo[key] = true
	}
	writer, err := bridge.Want(Ask{Request: m.request(criterion, purposeWriter), Votes: oneVote, Criteria: []string{m.ref(criterion)}, Purpose: purposeWriter})
	if err != nil {
		return err
	}
	if writer.Standing == Answered && criterion == m.spoiled && !m.renewed {
		return bridge.Contradict(writer.Votes[0], "the writer's answer is unusable")
	}
	if writer.Standing == Unresponsive {
		m.proved[criterion] = true
		return nil
	}
	if writer.Standing != Answered {
		return nil
	}
	judges, err := bridge.Want(Ask{Request: m.request(criterion, purposeJudge), Votes: judgeVotes, Criteria: []string{m.ref(criterion)}, Purpose: purposeJudge})
	if err != nil || judges.Standing != Answered {
		return err
	}
	for _, answer := range []Answer{writer, judges} {
		if err := bridge.Validate(answer); err != nil {
			return err
		}
		if err := bridge.Consume(answer); err != nil {
			return err
		}
	}
	m.proved[criterion], m.fresh[criterion] = true, true
	return nil
}

func (m *simulatedModule) ref(criterion int) string { return fmt.Sprintf("BAND-%d~1", criterion+1) }

func (m *simulatedModule) request(criterion int, purpose string) hostproto.Request {
	return hostprototest.Request(func(r *hostproto.Request) {
		r.Prompt.User = fmt.Sprintf("%s %s %s", m.name, purpose, m.ref(criterion))
	})
}

// abandoned lists the criteria concluded on an abandoned ticket: never fresh.
func (m *simulatedModule) abandoned() []int {
	var criteria []int
	for criterion := range fourteenCriteria {
		if m.proved[criterion] && !m.fresh[criterion] {
			criteria = append(criteria, criterion)
		}
	}
	return criteria
}

// fakeHost answers the launchable tickets of a module through its request
// service, the way the fulfil skill's executors do. The first ticket it meets is
// fetched by an executor that ends without answering, once.
type fakeHost struct {
	orphaned bool
}

func (h *fakeHost) answer(t *testing.T, m *simulatedModule, clock func() time.Time) {
	t.Helper()
	service := requests.Service{Store: m.store, Validator: hostprototest.MemberValidator{}, Clock: clock}
	listing, err := service.List(hostproto.OnlySession(hostproto.SessionProve))
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range listing.Items {
		if _, err := service.Show(item.Ticket.TicketID, ""); err != nil {
			t.Fatal(err)
		}
		if !h.orphaned {
			h.orphaned = true
			if _, err := m.store.Append(hostproto.Event{Kind: hostproto.EventLeaked, Ticket: item.Ticket.TicketID.String(), AgentID: orphanAgent}); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if _, err := service.Submit(requests.Submission{Ticket: item.Ticket.TicketID, Nonce: item.Ticket.Nonce, Output: []byte(answeredOutput), Origin: requests.OriginCLI}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFourteenCriteriaTwoProofs(t *testing.T) {
	t.Run("a fake host drives 14 criteria of 2 modules in bounded steps, through an orphaned fetch and an abandonment, to a publishable set"+verifiesFourteen, func(t *testing.T) {
		clock := (&fakeClock{now: hostprototest.Epoch}).Now
		host := &fakeHost{}
		modules := []*simulatedModule{
			{name: moduleDiscriminating, spoiled: 3},
			{name: moduleAlignment, spoiled: -1},
		}
		cuts, renewals := 0, 0
		for _, m := range modules {
			m.store, m.memo, m.proved, m.fresh = hostprototest.NewMemoryHostStore(), map[string]bool{}, map[int]bool{}, map[int]bool{}
			renew, idle := true, 0
			for step := 0; ; step++ {
				if step == maxDriverSteps || idle == maxStepsPerAnswer {
					t.Fatalf("%s did not finish: %d steps, proved %v, abandoned %v", m.name, step, m.proved, m.abandoned())
				}
				end, outstanding := m.step(t, clock, renew)
				renew = false
				switch {
				case end.Unfinished():
					cuts++
					idle++
					continue
				case len(outstanding.Launchable) > 0:
					host.answer(t, m, clock)
					idle = 0
				case outstanding.Waiting() > 0:
					idle++
				case len(m.abandoned()) > 0 && !m.renewed:
					m.renewed, renew = true, true
					for _, criterion := range m.abandoned() {
						delete(m.proved, criterion)
					}
					renewals++
				case len(m.proved) == fourteenCriteria:
					records, err := m.store.Records()
					if err != nil {
						t.Fatal(err)
					}
					published := format.Pending{Module: m.name, Pending: []format.PendingEntry{}, Answered: hostproto.KeptAnswers(records)}.Over(m.pending)
					if err := published.Validate(); err != nil || len(published.Pending) != 0 || published.Answered == 0 || !published.Kept() {
						t.Fatalf("%s publishes %+v, %v", m.name, published, err)
					}
					m.pending = published
				default:
					idle++
					continue
				}
				if len(m.proved) == fourteenCriteria && len(m.abandoned()) == 0 && m.pending.Answered > 0 {
					break
				}
			}
		}
		for _, m := range modules {
			outstanding, err := hostproto.QueryOutstanding(m.store, hostproto.OnlySession(hostproto.SessionProve))
			if err != nil || outstanding.Waiting() != 0 || len(m.fresh) != fourteenCriteria {
				t.Errorf("%s: outstanding %+v, %v, %d fresh", m.name, outstanding, err, len(m.fresh))
			}
		}
		if cuts == 0 || renewals != 1 || !host.orphaned {
			t.Fatalf("the run did not go through what it checks: %d cuts, %d renewals, orphan %v", cuts, renewals, host.orphaned)
		}
	})
}
