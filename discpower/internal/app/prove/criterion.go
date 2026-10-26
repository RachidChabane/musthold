package prove

import (
	"context"
	"fmt"
	"slices"

	"github.com/RachidChabane/musthold/discpower/internal/app/execution"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/policy"
	"github.com/RachidChabane/musthold/discpower/internal/domain/power"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
	"github.com/RachidChabane/musthold/discpower/internal/domain/witness"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto/hostbridge"
)

// poolMutant is one mutant of a criterion's pool with what is known of it.
type poolMutant struct {
	spec   execution.MutantSpec
	origin power.Origin
	slot   power.Slot
	// family is the operator family of an operator mutant.
	family string
	round  int
	edits  mutation.EditSet
	facts  power.Facts
	// flagged marks an operator survivor the screen let into the pool.
	flagged bool
	// violation is the screen's probability that the survivor violates the criterion.
	violation float64
	// votes are the validity votes cast on an unwitnessed survivor, and profiles
	// the executors that cast them.
	votes    []power.Vote
	profiles []format.ExecutorProfile
	// unjudged says why a survivor that needed votes has none.
	unjudged power.Reason
	// asked is set once this pass asked the votes of the survivor.
	asked bool
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
	// draws are the witness draws of the criterion the search reads, and
	// creditDraws the draw a CI run asks for credit alone.
	draws       []*witnessDraw
	creditDraws []*witnessDraw
	// records are the witnesses of every criterion the pool was settled against.
	records []witness.Record

	// spentSchedule and spentJudgment count the host tickets of this pass the
	// budget pays for.
	spentSchedule int
	spentJudgment int
	// generationPending is set while a generation request waits for its answer:
	// the search has not ended.
	generationPending bool
	waiting           bool
	budgetExhausted   bool
	hostUnresponsive  bool
	mechanisms        []string
	// answers are the validated answers the Statement rests on.
	answers []hostbridge.Answer
	// screenedOut and flagged count the operator survivors the screen let go or
	// kept.
	screenedOut int
	flagged     int
	// screenFloorEntries counts the flagged survivors that only the isolation floor
	// kept in the pool.
	screenFloorEntries int
}

func (p *project) newCritRun(bridge *hostbridge.Bridge, criterion spec.Criterion) *critRun {
	return &critRun{p: p, bridge: bridge, c: criterion, rules: p.settings.Policy, poolByID: map[string]*poolMutant{}}
}

// firstDraw asks the first writers of the criterion, when the ecosystem can run
// witnesses.
func (r *critRun) firstDraw(ctx context.Context) error {
	if !r.p.runner.Witnesses.Available() {
		return nil
	}
	draw, err := r.drawWitnesses(ctx, firstDraw, firstDrawVotes)
	if err != nil {
		return err
	}
	r.draws = append(r.draws, draw)
	if !r.p.redrawsForCredit() {
		return nil
	}
	// The credit draw holds back the Statement alone, never the search.
	searchWaiting := r.waiting
	credit, err := r.drawWitnesses(ctx, creditDraw, creditDrawVotes)
	if err != nil {
		return err
	}
	r.waiting = searchWaiting
	r.creditDraws = append(r.creditDraws, credit)
	return nil
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

// billing says which part of a criterion's ticket budget a request is paid from.
type billing int

const (
	// billNone: the request is not counted, such as a reissue or a screen.
	billNone billing = iota
	// billSchedule: the writers, the scope judges and the generation, which the
	// validity votes never crowd out, so that these requests never depend on
	// the tests.
	billSchedule
	// billJudgment: the validity votes, paid from what the schedule left.
	billJudgment
)

// ask puts a request to the host. A request the budget pays for is refused when
// the criterion has no tickets left; the budget is counted per pass, in the
// order the pass asks, so that it never depends on what earlier runs left in the
// store.
func (r *critRun) ask(ask hostbridge.Ask, bill billing) (hostbridge.Answer, askState, error) {
	switch bill {
	case billSchedule:
		if r.spentSchedule+ask.Votes > r.rules.MaxHostTickets {
			r.budgetExhausted = true
			return hostbridge.Answer{}, askOverBudget, nil
		}
		r.spentSchedule += ask.Votes
	case billJudgment:
		if r.spentSchedule+r.spentJudgment+ask.Votes > r.rules.MaxHostTickets {
			r.budgetExhausted = true
			return hostbridge.Answer{}, askOverBudget, nil
		}
		r.spentJudgment += ask.Votes
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

// bindings names, for every bound test of the suite, the criteria it is bound
// to.
func (p *project) bindings() map[ecosystem.TestID][]spec.Ref {
	bindings := map[ecosystem.TestID][]spec.Ref{}
	for _, criterion := range p.criteria {
		for _, test := range criterion.TestIDs() {
			if !slices.Contains(bindings[test], criterion.Ref) {
				bindings[test] = append(bindings[test], criterion.Ref)
			}
		}
	}
	return bindings
}

// buildSubject names the criterion's tests, the bindings of every test and the
// tests that killed a sentinel.
func (r *critRun) buildSubject(sentinels []power.Facts) {
	r.subject = power.NewSubject(r.c.Ref, r.p.bindings(), power.SentinelKillers(sentinels))
}

// observeOperatorPool executes the sentinels and the operator mutants over the
// criterion's footprint and sorts them into the pool: sentinels, mutants that
// fail an example of the criterion, survivors that fail a block-bearing witness
// of the criterion, and survivors that still need the screen. A survivor that
// fails such a witness enters the pool whatever the differential search found.
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
	own := r.ownUniverse()
	for _, mutant := range r.st.generated.Operator {
		m, err := r.observe(ctx, mutant.ID, mutant.EditSet, power.OriginOperator, "")
		if err != nil {
			return nil, err
		}
		if m == nil {
			continue
		}
		m.family = string(mutant.Family)
		switch {
		case slices.Contains(m.facts.FailedExamples, r.c.Ref):
			r.add(m)
		case len(m.facts.FailedExamples) > 0 || r.subject.Killed(m.facts):
		default:
			if err := r.runOwnWitnesses(ctx, m); err != nil {
				return nil, err
			}
			if own.Witnessed(m.facts, r.c.Ref) {
				r.add(m)
				continue
			}
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

// runOwnWitnesses runs the criterion's own witnesses on a mutant.
func (r *critRun) runOwnWitnesses(ctx context.Context, m *poolMutant) error {
	sets := r.searchSets()
	if len(sets) == 0 {
		return nil
	}
	outcomes, err := r.p.runner.EditWitnesses(ctx, m.spec.ID, m.edits, sets)
	if err != nil {
		return err
	}
	m.facts.WitnessOutcomes = outcomes
	return nil
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

// rules are the G3-W rules of the run.
func (r *critRun) g3w() power.Rules {
	return power.Rules{Policy: r.rules, Admission: r.p.settings.Admission()}
}

// standing assembles what the terminal outcome rests on.
func (r *critRun) standing(correctCodePasses, exampleFailed bool) power.Standing {
	entries := make([]power.Entry, 0, len(r.pool))
	for _, m := range r.pool {
		entries = append(entries, power.Entry{Facts: m.facts, Votes: m.votes, Unjudged: m.unjudged})
	}
	return power.Standing{
		Subject: r.subject, CorrectCodePasses: correctCodePasses, ExampleFailed: exampleFailed,
		Witnesses: r.records, Entries: entries, BudgetExhausted: r.budgetExhausted, HostUnresponsive: r.hostUnresponsive,
	}
}

// poolFacts lists the facts of the pool, in pool order.
func (r *critRun) poolFacts() []power.Facts {
	facts := make([]power.Facts, len(r.pool))
	for index, m := range r.pool {
		facts[index] = m.facts
	}
	return facts
}
