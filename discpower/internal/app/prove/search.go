package prove

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/domain/power"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
	"github.com/RachidChabane/musthold/discpower/internal/domain/witness"
)

// search runs the staged search of a criterion's pool: the deterministic pool,
// the screen, then rounds of targeted generation until the stop rule says the
// pool holds enough witnessed mutants or the rounds are spent, and one further
// witness draw when it holds fewer than two. Neither the rounds nor the further
// draw read a test, so the requests of a criterion are the same whatever its
// tests. The validity votes of the survivors no witness settles are asked as the
// rounds go, beside them and never in their way. A criterion whose reference
// fails a test or an example blocks on execution alone and asks for no mutant.
func (r *critRun) search(ctx context.Context) error {
	survivors, err := r.observeOperatorPool(ctx)
	if err != nil {
		return err
	}
	if !r.p.referencePasses(r.c) || r.p.exampleFailsOnReference(r.c.Ref) {
		return nil
	}
	if err := r.screen(ctx, survivors); err != nil {
		return err
	}
	if err := r.judgeSurvivors(ctx, r.p.searchSets, r.p.searchRecords); err != nil {
		return err
	}
	for round := 1; round <= r.rules.MaxRounds; round++ {
		if round > 1 && r.stop() {
			break
		}
		_, state, err := r.generate(ctx, round)
		if err != nil {
			return err
		}
		if state != askAnswered {
			r.generationPending = state == askPending
			break
		}
		if err := r.judgeSurvivors(ctx, r.p.searchSets, r.p.searchRecords); err != nil {
			return err
		}
	}
	return r.furtherDraw(ctx)
}

// stop applies the stop rule to the pool.
func (r *critRun) stop() bool {
	return power.StopSearching(r.ownUniverse(), r.c.Ref, r.poolFacts(), r.rules)
}

// furtherDraw asks one further writer for a criterion whose finished search left
// fewer than two witnessed mutants in the pool.
func (r *critRun) furtherDraw(ctx context.Context) error {
	if r.generationPending || !r.p.runner.Witnesses.Available() || !power.NeedsFurtherWitnesses(r.ownUniverse(), r.c.Ref, r.poolFacts()) {
		return nil
	}
	draw, err := r.drawWitnesses(ctx, furtherDraw, furtherDrawVote)
	if err != nil {
		return err
	}
	r.draws = append(r.draws, draw)
	if !draw.settled {
		r.waiting = true
	}
	return nil
}

// settle runs every pool mutant against the witnesses of every criterion and
// asks the validity votes of each survivor that execution still leaves unsettled.
func (r *critRun) settle(ctx context.Context, sets []witness.Set, records []witness.Record) error {
	r.records = records
	return r.judgeSurvivors(ctx, sets, records)
}

// judgeSurvivors runs the pool against the witness sets, compares each mutant's
// compiled form with the reference's, and asks the validity votes of each
// survivor that fails no block-bearing witness of the criterion and that
// execution does not settle: survivors of the targeted rounds first, then the
// flagged operator survivors by descending probability of violating. A survivor
// is asked about once per pass.
func (r *critRun) judgeSurvivors(ctx context.Context, sets []witness.Set, records []witness.Record) error {
	for _, m := range r.pool {
		if m.origin == power.OriginSentinel {
			continue
		}
		if len(sets) > 0 {
			outcomes, err := r.p.runner.EditWitnesses(ctx, m.spec.ID, m.edits, sets)
			if err != nil {
				return err
			}
			m.facts.WitnessOutcomes = outcomes
		}
		identity, err := r.p.runner.Identity(ctx, m.spec)
		if err != nil {
			return err
		}
		m.facts.Identity = identity
	}
	universe := power.NewUniverse(records, r.g3w())
	for _, m := range r.survivorsInOrder() {
		if m.asked {
			continue
		}
		decision := power.Decide(r.subject, universe, m.facts, nil, r.g3w())
		if decision.Class != power.ClassUnsettled || len(universe.FailingBlockBearing(m.facts, r.c.Ref)) > 0 {
			continue
		}
		m.asked = true
		if err := r.judge(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

func (r *critRun) survivorsInOrder() []*poolMutant {
	var survivors []*poolMutant
	for _, m := range r.pool {
		if m.origin != power.OriginSentinel && r.subject.AllSurvived(m.facts) {
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

// scopeOf lists the criteria whose operator mutants include the mutant, sorted,
// from the analyses made in this pass.
func (p *project) scopeOf(id string) []spec.Ref {
	if p.scopes == nil {
		p.scopes = map[string][]spec.Ref{}
		for _, criterion := range p.criteria {
			analysis, done := p.statics[criterion.Ref.String()+seedSeparator+footprintKey(p.footprints[criterion.Ref])]
			if !done {
				continue
			}
			for _, mutant := range analysis.generated.Operator {
				p.scopes[mutant.ID] = append(p.scopes[mutant.ID], criterion.Ref)
			}
		}
	}
	return p.scopes[id]
}
