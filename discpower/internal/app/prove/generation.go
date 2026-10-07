package prove

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/prompt"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostbridge"
)

const (
	seedSeparator = "|"
	// contradictionNoUsableMutant is the ground on which a generation response
	// is rejected: none of its mutants splices, builds and differs.
	contradictionNoUsableMutant = "no_usable_mutant"
)

// generationSeed draws the seed of a round from the run seed, the criterion's
// statement and the round, so that rounds differ and a rerun repeats them.
func (r *critRun) generationSeed(round int) string {
	return digestText(strings.Join([]string{fmt.Sprint(r.p.settings.Seed), r.c.StatementDigest, fmt.Sprint(round)}, seedSeparator))
}

// generate puts the targeted generation request of a round to the host and, once
// it is answered, turns the response into the mutants that survive F0 and F1 and
// execute. A response none of whose mutants is usable is contradicted by
// execution, which rejects its ticket.
func (r *critRun) generate(ctx context.Context, round int) ([]*poolMutant, askState, error) {
	built := prompt.BuildGeneration(prompt.GenerationInput{Statement: r.p.blindStatement(r.c), Slice: r.st.slice, Avoid: r.mechanisms})
	inputs := []hostproto.Input{
		{Role: format.RoleSpec, Digest: r.c.StatementDigest},
		{Role: format.RoleCodeTree, Digest: r.sliceDigest()},
	}
	if len(r.mechanisms) > 0 {
		inputs = append(inputs, hostproto.Input{Role: format.RoleHostResponse, Digest: digestText(strings.Join(r.mechanisms, "\n"))})
	}
	request, err := r.p.assemble(requestDraft{kind: hostproto.KindGenerateTargeted, built: built, inputs: inputs, round: round, seed: r.generationSeed(round)})
	if err != nil {
		return nil, askAnswered, err
	}
	ask := hostbridge.Ask{Request: request, Votes: 1, Criteria: []string{r.c.Ref.String()}, Purpose: string(hostproto.KindGenerateTargeted)}
	answer, state, err := r.ask(ask, true)
	if err != nil || state != askAnswered {
		return nil, state, err
	}
	mutants, usable, err := r.intake(ctx, answer.Votes[0].Output, round)
	if err != nil {
		return nil, askAnswered, err
	}
	if usable > 0 {
		r.answers = append(r.answers, answer)
		return mutants, askAnswered, r.bridge.Validate(answer)
	}
	if err := r.bridge.Contradict(answer.Votes[0], contradictionNoUsableMutant); err != nil {
		return nil, askAnswered, err
	}
	_, state, err = r.ask(ask, false)
	return nil, state, err
}

func (r *critRun) sliceDigest() string {
	var text strings.Builder
	for _, entry := range r.st.slice {
		text.WriteString(entry.File + "\x00" + entry.Qualname + "\x00" + entry.Source + "\x00")
	}
	return digestText(text.String())
}

// intake reads a generation response into pool mutants and counts the proposals
// that were usable. A mutant that does not build (F0), equals its original after
// normalization, or fails an example of another criterion only, is dropped; one
// the pool already holds is usable but adds nothing.
func (r *critRun) intake(ctx context.Context, output []byte, round int) (mutants []*poolMutant, usable int, err error) {
	generated, err := prompt.ParseGeneration(output)
	if err != nil {
		return nil, 0, nil
	}
	for _, proposal := range generated {
		r.mechanisms = append(r.mechanisms, proposal.Mechanism)
		m, known, err := r.intakeOne(ctx, proposal, round)
		if err != nil {
			return nil, 0, err
		}
		if known {
			usable++
		}
		if m != nil {
			usable++
			r.add(m)
			mutants = append(mutants, m)
		}
	}
	return mutants, usable, nil
}

// intakeOne turns one proposal into a pool mutant. It returns known when the
// pool already holds the mutant.
func (r *critRun) intakeOne(ctx context.Context, proposal prompt.GeneratedMutant, round int) (m *poolMutant, known bool, err error) {
	if !proposal.Slot.Valid() || !r.admissible(proposal.Edits) {
		return nil, false, nil
	}
	id, err := proposal.Edits.ID()
	if err != nil || r.trivial(proposal.Edits) {
		return nil, false, nil
	}
	if _, repeated := r.poolByID[id]; repeated {
		return nil, true, nil
	}
	m, err = r.observe(ctx, id, proposal.Edits, power.OriginTargeted, proposal.Slot)
	if err != nil || m == nil {
		return nil, false, err
	}
	m.round = round
	if len(m.facts.FailedExamples) > 0 && !slices.Contains(m.facts.FailedExamples, r.c.Ref) {
		return nil, false, nil
	}
	return m, false, nil
}

// admissible checks the edit set a host proposed: known target kinds, no unit
// edited twice and paths the policy allows.
func (r *critRun) admissible(edits mutation.EditSet) bool {
	if edits.Validate() != nil {
		return false
	}
	for _, edit := range edits.Edits {
		if edit.Kind() == mutation.TargetModuleAddition || r.p.executor.Policy.Check(edit.File) != nil {
			return false
		}
	}
	return true
}

// trivial reports whether the edit set changes nothing: every edit leaves its
// unit equal to the original after normalization. A set that names a unit that
// does not exist, or whose new source does not parse, is unusable and counts as
// trivial too.
func (r *critRun) trivial(edits mutation.EditSet) bool {
	for _, edit := range edits.Edits {
		changed, err := r.changesUnit(edit)
		if err != nil {
			return true
		}
		if changed {
			return false
		}
	}
	return true
}

func (r *critRun) changesUnit(edit mutation.Edit) (bool, error) {
	original, err := r.p.unitSource(edit.File, edit.Qualname, edit.Kind())
	if err != nil {
		return false, err
	}
	engine, err := r.p.deps.Engines.For(edit.File)
	if err != nil {
		return false, err
	}
	before, err := engine.Normalize(edit.File, edit.Kind(), original)
	if err != nil {
		return false, err
	}
	after, err := engine.Normalize(edit.File, edit.Kind(), edit.NewSource)
	if err != nil {
		return false, err
	}
	return before != after, nil
}
