package prove

import (
	"context"
	"fmt"
	"slices"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/execution"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostbridge"
)

// poolMutant is one mutant of a criterion's pool with what is known of it.
type poolMutant struct {
	spec   execution.MutantSpec
	origin power.Origin
	slot   power.Slot
	round  int
	edits  mutation.EditSet
	facts  power.Facts
	// flagged marks an operator survivor the screen let into the pool.
	flagged bool
	// violation is the screen's probability that the survivor violates the criterion.
	violation float64
	decision  *power.Decision
	// profiles are the executors of the votes the decision rests on.
	profiles []format.ExecutorProfile
}

// critRun is one pass over one criterion.
type critRun struct {
	p      *project
	bridge *hostbridge.Bridge
	c      spec.Criterion
	st     *staticAnalysis
	rules  policy.Policy

	subject  power.Subject
	pool     []*poolMutant
	poolByID map[string]*poolMutant

	// spent counts the host tickets of this pass that the budget pays for.
	spent            int
	waiting          bool
	budgetExhausted  bool
	hostUnresponsive bool
	mechanisms       []string
	// answers are the validated answers the Statement rests on.
	answers []hostbridge.Answer
	// screenedOut and flagged count the operator survivors the screen let go or
	// kept.
	screenedOut int
	flagged     int
}

func (p *project) newCritRun(bridge *hostbridge.Bridge, criterion spec.Criterion, st *staticAnalysis) *critRun {
	return &critRun{p: p, bridge: bridge, c: criterion, st: st, rules: p.settings.Policy, poolByID: map[string]*poolMutant{}}
}

func (r *critRun) add(m *poolMutant) {
	if _, present := r.poolByID[m.spec.ID]; present {
		return
	}
	r.pool = append(r.pool, m)
	r.poolByID[m.spec.ID] = m
}

// asState is what asking the host came to.
type askState int

const (
	askAnswered askState = iota
	askPending
	askUnresponsive
	askOverBudget
)

// ask puts a request to the host. A request the budget pays for is refused when
// the criterion has no tickets left; the budget is counted per pass, in the
// order the pass asks, so that it never depends on what earlier runs left in the
// store.
func (r *critRun) ask(ask hostbridge.Ask, billable bool) (hostbridge.Answer, askState, error) {
	if billable {
		if r.spent+ask.Votes > r.rules.MaxHostTickets {
			r.budgetExhausted = true
			return hostbridge.Answer{}, askOverBudget, nil
		}
		r.spent += ask.Votes
	}
	answer, err := r.bridge.Want(ask)
	if err != nil {
		return answer, askAnswered, err
	}
	switch answer.Standing {
	case hostbridge.Pending:
		r.waiting = true
		return answer, askPending, nil
	case hostbridge.Unresponsive:
		r.hostUnresponsive = true
		return answer, askUnresponsive, nil
	}
	return answer, askAnswered, nil
}

// buildSubject names the criterion's tests, every other test and the tests that
// killed a sentinel.
func (r *critRun) buildSubject(sentinels []power.Facts) {
	own := r.c.TestIDs()
	var others []ecosystem.TestID
	for _, test := range r.p.tests {
		if !slices.Contains(own, test.ID) {
			others = append(others, test.ID)
		}
	}
	r.subject = power.Subject{Ref: r.c.Ref, Tests: own, OtherTests: others, SentinelKillers: power.SentinelKillers(sentinels)}
}

// observeOperatorPool executes the sentinels and the operator mutants over the
// criterion's footprint and sorts them into the pool: sentinels, mutants that
// fail an example of the criterion, and survivors that still need the screen.
func (r *critRun) observeOperatorPool(ctx context.Context) (survivors []*poolMutant, err error) {
	var sentinels []*poolMutant
	for _, mutant := range r.st.generated.Sentinels {
		m, err := r.observe(ctx, mutant.ID, mutant.EditSet, power.OriginSentinel, "")
		if err != nil {
			return nil, err
		}
		if m != nil {
			sentinels = append(sentinels, m)
		}
	}
	facts := make([]power.Facts, len(sentinels))
	for index, m := range sentinels {
		facts[index] = m.facts
	}
	r.buildSubject(facts)
	for _, m := range sentinels {
		r.add(m)
	}
	for _, mutant := range r.st.generated.Operator {
		m, err := r.observe(ctx, mutant.ID, mutant.EditSet, power.OriginOperator, "")
		if err != nil {
			return nil, err
		}
		if m == nil {
			continue
		}
		switch {
		case slices.Contains(m.facts.FailedExamples, r.c.Ref):
			r.add(m)
		case len(m.facts.FailedExamples) > 0 || r.subject.Killed(m.facts):
		default:
			if m.facts.Differential, err = r.p.runner.Differential(ctx, m.spec, r.st.draws, r.st.skipReason); err != nil {
				return nil, err
			}
			if m.facts.Differential.Status != power.NoDiffFound {
				survivors = append(survivors, m)
			}
		}
	}
	return survivors, nil
}

// observe executes a mutant that built; it returns nil for one that did not.
func (r *critRun) observe(ctx context.Context, id string, edits mutation.EditSet, origin power.Origin, slot power.Slot) (*poolMutant, error) {
	mutantSpec := execution.MutantSpec{ID: id, Edits: edits, FullSuite: origin == power.OriginSentinel || !onlyDefinitions(edits), Sentinel: origin == power.OriginSentinel}
	observed, err := r.p.runner.Observe(ctx, mutantSpec)
	if err != nil {
		return nil, fmt.Errorf("execute mutant %s: %w", id, err)
	}
	if !observed.Built {
		return nil, nil
	}
	if err := r.p.deps.Vault.Put(id, edits); err != nil {
		return nil, fmt.Errorf("keep the mutant body: %w", err)
	}
	return &poolMutant{spec: mutantSpec, origin: origin, slot: slot, edits: edits, facts: observed.Facts(id, origin, slot, edits)}, nil
}

func onlyDefinitions(edits mutation.EditSet) bool {
	return !slices.ContainsFunc(edits.Edits, func(edit mutation.Edit) bool { return edit.Kind() != mutation.TargetDefinition })
}

// decide settles the decision of a mutant from the votes the host gave.
func (r *critRun) decide(m *poolMutant, votes []power.Vote, profiles []format.ExecutorProfile) {
	decision := power.Decide(r.subject, m.facts, votes, r.rules)
	m.decision, m.profiles = &decision, profiles
}

// standing assembles what the terminal outcome rests on. A mutant without a
// decision is undetermined: it only happens when the host could not answer.
func (r *critRun) standing(correctCodePasses, exampleFailed bool) power.Standing {
	entries := make([]power.Entry, 0, len(r.pool))
	for _, m := range r.pool {
		decision := power.Undetermined(power.ReasonNotAdjudicated)
		if m.decision != nil {
			decision = *m.decision
		}
		entries = append(entries, power.Entry{Facts: m.facts, Decision: decision})
	}
	return power.Standing{
		Subject: r.subject, CorrectCodePasses: correctCodePasses, ExampleFailed: exampleFailed,
		Entries: entries, BudgetExhausted: r.budgetExhausted, HostUnresponsive: r.hostUnresponsive,
	}
}
