package prove

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
)

// search runs the staged search of a criterion's pool: the deterministic pool,
// the screen, then rounds of targeted generation, each followed by the
// adjudication of what is in the pool, until the stop rule says the pool is
// enough or the rounds are spent. Where no probe can run, no judgment of the
// host could be corroborated, so the host is not asked.
func (r *critRun) search(ctx context.Context) error {
	survivors, err := r.observeOperatorPool(ctx)
	if err != nil {
		return err
	}
	if !r.p.referencePasses(r.c) || r.p.exampleFailsOnReference(r.c.Ref) || r.st.draws == nil {
		r.attribute()
		return nil
	}
	if err := r.screen(ctx, survivors); err != nil {
		return err
	}
	for round := 1; round <= r.rules.MaxRounds; round++ {
		if round > 1 && (r.waiting || r.stop()) {
			break
		}
		_, state, err := r.generate(ctx, round)
		if err != nil {
			return err
		}
		if err := r.adjudicate(ctx); err != nil {
			return err
		}
		if state != askAnswered {
			break
		}
	}
	return nil
}

// stop applies the stop rule to the decisions taken so far.
func (r *critRun) stop() bool {
	return power.StopSearching(r.decidedStanding(), r.rules)
}

// decidedStanding is the standing of the mutants that have a decision.
func (r *critRun) decidedStanding() power.Standing {
	standing := r.standing(true, false)
	standing.Entries = slices.DeleteFunc(standing.Entries, func(entry power.Entry) bool {
		m := r.poolByID[entry.Facts.ID]
		return m == nil || m.decision == nil
	})
	return standing
}

// attribute settles what needs no host: sentinels are equivalent by construction
// and a mutant that fails an example is attributed to the criterion that states
// it.
func (r *critRun) attribute() {
	for _, m := range r.pool {
		if m.decision == nil && (m.origin == power.OriginSentinel || len(m.facts.FailedExamples) > 0) {
			r.decide(m, nil, nil)
		}
	}
}

// adjudicate puts the pool to the host in priority order: survivors of the
// targeted rounds first, then the flagged operator survivors by descending
// probability of violating, then, only while the pool is not yet enough, killed
// targeted mutants in seeded order for pass credit.
func (r *critRun) adjudicate(ctx context.Context) error {
	r.attribute()
	for _, m := range r.survivorsInOrder() {
		if err := r.judge(ctx, m); err != nil {
			return err
		}
	}
	if r.waiting {
		return nil
	}
	for _, m := range r.creditCandidatesInOrder() {
		if r.stop() {
			break
		}
		if err := r.judge(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

func (r *critRun) survivorsInOrder() []*poolMutant {
	var survivors []*poolMutant
	for _, m := range r.pool {
		if m.decision == nil && (m.origin == power.OriginTargeted || m.flagged) && !r.subject.Killed(m.facts) {
			survivors = append(survivors, m)
		}
	}
	slices.SortStableFunc(survivors, func(a, b *poolMutant) int {
		return cmp.Or(
			cmp.Compare(originRank(a), originRank(b)),
			cmp.Compare(a.round, b.round),
			strings.Compare(string(a.slot), string(b.slot)),
			-cmp.Compare(a.violation, b.violation),
			strings.Compare(a.spec.ID, b.spec.ID),
		)
	})
	return survivors
}

func originRank(m *poolMutant) int {
	if m.origin == power.OriginTargeted {
		return 0
	}
	return 1
}

func (r *critRun) creditCandidatesInOrder() []*poolMutant {
	var candidates []*poolMutant
	for _, m := range r.pool {
		if m.decision == nil && m.origin == power.OriginTargeted && r.subject.Killed(m.facts) {
			candidates = append(candidates, m)
		}
	}
	seed := r.generationSeed(0)
	slices.SortStableFunc(candidates, func(a, b *poolMutant) int {
		return strings.Compare(digestText(seed+a.spec.ID), digestText(seed+b.spec.ID))
	})
	return candidates
}

// groupByBehaviour groups survivors that behave identically on the witnesses the
// differential search found, so that one question serves the group; a survivor
// without witnesses is a group of its own. Groups and members come sorted by id.
func groupByBehaviour(survivors []*poolMutant) [][]*poolMutant {
	ordered := slices.Clone(survivors)
	slices.SortFunc(ordered, func(a, b *poolMutant) int { return strings.Compare(a.spec.ID, b.spec.ID) })
	var keys []string
	groups := map[string][]*poolMutant{}
	for _, m := range ordered {
		key := behaviourKey(m)
		if _, seen := groups[key]; !seen {
			keys = append(keys, key)
		}
		groups[key] = append(groups[key], m)
	}
	result := make([][]*poolMutant, len(keys))
	for index, key := range keys {
		result[index] = groups[key]
	}
	return result
}

func behaviourKey(m *poolMutant) string {
	if m.facts.Differential.Status != power.DiffersAt || len(m.facts.Differential.Witnesses) == 0 {
		return m.spec.ID
	}
	canonical, err := canonicalWitnesses(m.facts.Differential.Witnesses)
	if err != nil {
		return m.spec.ID
	}
	return canonical
}

// scopeOf lists the criteria whose operator mutants include the mutant, sorted.
func (p *project) scopeOf(ctx context.Context, id string) ([]spec.Ref, error) {
	if p.scopes == nil {
		p.scopes = map[string][]spec.Ref{}
		for _, criterion := range p.criteria {
			analysis, err := p.staticOf(ctx, criterion)
			if err != nil {
				return nil, err
			}
			for _, mutant := range analysis.generated.Operator {
				p.scopes[mutant.ID] = append(p.scopes[mutant.ID], criterion.Ref)
			}
		}
	}
	return p.scopes[id], nil
}
