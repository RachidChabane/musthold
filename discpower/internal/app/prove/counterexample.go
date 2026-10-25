package prove

import (
	"slices"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/witness"
	"github.com/RachidChabane/spec-proof/format"
)

// Survivor is a mutant that blocks its criterion: a valid mutant every test of
// the criterion passes on, with what its local finding shows (ADR 0003,
// hiding): how the mutant was drawn and one confirmed witness, never its body.
type Survivor struct {
	Mutant         string                `json:"mutant"`
	Counterexample format.Counterexample `json:"counterexample"`
	// Example is the criterion's example the counterexample names, when it
	// names one.
	Example *spec.Example `json:"example,omitempty"`
}

// generationSlots are the slots a targeted counterexample may name.
var generationSlots = []string{format.SlotDirect, format.SlotBoundary, format.SlotPartial}

// counterexample builds what the local finding of a blocking survivor shows: a
// survivor whose decision gives it the power to block, in a statement produced
// locally, with the lowest example of the criterion it fails, or else a
// block-bearing witness of the criterion it fails, bounded. A mutant that is not
// such a survivor has none.
func (r *critRun) counterexample(m *poolMutant, decision power.Decision) *format.Counterexample {
	if r.p.producedBy() != format.ProducedByLocal || !decision.Carries() || !r.subject.AllSurvived(m.facts) {
		return nil
	}
	var counterexample format.Counterexample
	switch {
	case m.origin == power.OriginTargeted && slices.Contains(generationSlots, string(m.slot)):
		counterexample.Origin, counterexample.Slot = format.OriginTargeted, string(m.slot)
	case m.origin == power.OriginOperator && m.family != "":
		counterexample.Origin, counterexample.Family = format.OriginOperator, m.family
	default:
		return nil
	}
	if index, fails := m.facts.FirstFailedExample(r.c.Ref); fails {
		counterexample.Example = &index
		return &counterexample
	}
	for _, id := range witnessesOf(decision) {
		if record, found := r.witnessRecord(id); found {
			witness := format.BoundedWitness(record.Witness.Name, record.Witness.Clause, record.Witness.Code)
			counterexample.Witness = &witness
			return &counterexample
		}
	}
	return nil
}

// witnessesOf lists the witnesses a decision rests on: the one its claim names
// first, then every block-bearing witness of the criterion the mutant fails.
func witnessesOf(decision power.Decision) []string {
	if decision.Witness == "" {
		return decision.Failing
	}
	return append([]string{decision.Witness}, decision.Failing...)
}

// survivors lists the blocking survivors of a predicate, by mutant id, with the
// criterion's example each one names.
func (r *critRun) survivors(predicate format.DiscriminatingPower) []Survivor {
	var found []Survivor
	for _, mutant := range predicate.Mutants {
		if mutant.Extensions == nil || mutant.Extensions.Counterexample == nil {
			continue
		}
		survivor := Survivor{Mutant: mutant.Digest, Counterexample: *mutant.Extensions.Counterexample}
		if index := survivor.Counterexample.Example; index != nil {
			if example, found := r.c.Example(*index); found {
				survivor.Example = &example
			}
		}
		found = append(found, survivor)
	}
	return found
}

// witnessRecord finds the record of a witness the pool was run against.
func (r *critRun) witnessRecord(id string) (witness.Record, bool) {
	index := slices.IndexFunc(r.records, func(record witness.Record) bool { return record.Witness.ID == id })
	if index < 0 {
		return witness.Record{}, false
	}
	return r.records[index], true
}
