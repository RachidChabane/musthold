package format

import "slices"

// SourceViewCap bounds the source text a host view or a statement shows: the
// whole functions of a host view, the code of a counterexample's witness.
const SourceViewCap = 48 << 10

// CounterexampleOrigin is how the mutant of a counterexample was drawn.
type CounterexampleOrigin string

// Origins of a counterexample's mutant.
const (
	// OriginTargeted: a generator wrote the mutant for one slot of the criterion.
	OriginTargeted CounterexampleOrigin = "targeted"
	// OriginOperator: a mutation operator of one family drew the mutant.
	OriginOperator CounterexampleOrigin = "operator"
)

// Generation slots of a targeted mutant (ADR 0003, S0 targeted).
const (
	SlotDirect   = "S1"
	SlotBoundary = "S2"
	SlotPartial  = "S3"
)

var (
	counterexampleOrigins = []CounterexampleOrigin{OriginTargeted, OriginOperator}
	generationSlots       = []string{SlotDirect, SlotBoundary, SlotPartial}
	slotRoles             = map[string]string{SlotDirect: "direct", SlotBoundary: "boundary", SlotPartial: "partial"}
)

// SlotRole names what a generation slot asks the generator for: a direct
// violation, a violation at a boundary, or one of part of the criterion.
func SlotRole(slot string) string { return slotRoles[slot] }

// Counterexample is what the local finding of a blocking survivor shows (ADR
// 0003, hiding): how its mutant was drawn and one confirmed witness of the
// criterion it fails, never its body, its diff or its output. It is written only
// in a statement produced locally.
type Counterexample struct {
	Origin CounterexampleOrigin `json:"origin"`
	// Slot is the generation slot of a targeted mutant, Family the operator
	// family of an operator one.
	Slot   string `json:"slot,omitempty"`
	Family string `json:"family,omitempty"`
	// Example is the index, among the criterion's examples, of one example the
	// mutant fails; Witness is a block-bearing witness of the criterion it fails.
	// Exactly one is set.
	Example *int            `json:"example,omitempty"`
	Witness *WitnessExample `json:"witness,omitempty"`
}

// WitnessExample is the witness a counterexample shows: a test written from the
// spec alone, with the clause of the criterion it checks.
type WitnessExample struct {
	Name   string `json:"name"`
	Clause string `json:"clause"`
	// Code is the witness's test, at most SourceViewCap bytes; Truncated says
	// that its end was cut there.
	Code      string `json:"code"`
	Truncated bool   `json:"truncated,omitempty"`
}

// checkCounterexample enforces the shape of a counterexample and that only a
// local statement carries one.
func checkCounterexample(sink *collector, path string, producedBy ProducedBy, counterexample *Counterexample) {
	if counterexample == nil {
		return
	}
	if producedBy != ProducedByLocal {
		sink.add(CodeCounterexampleNotLocal, path, "a statement produced by %s carries no counterexample", producedBy)
	}
	checkMember(sink, path+".origin", counterexample.Origin, counterexampleOrigins)
	switch counterexample.Origin {
	case OriginTargeted:
		if !slices.Contains(generationSlots, counterexample.Slot) || counterexample.Family != "" {
			sink.add(CodeUnknownValue, path+".slot", "a targeted mutant names one slot of %v and no family", generationSlots)
		}
	case OriginOperator:
		if counterexample.Family == "" || counterexample.Slot != "" {
			sink.add(CodeMissingField, path+".family", "an operator mutant names its family and no slot")
		}
	}
	switch {
	case (counterexample.Example == nil) == (counterexample.Witness == nil):
		sink.add(CodeMissingField, path, "a counterexample shows exactly one example or one witness")
	case counterexample.Witness != nil:
		checkNotEmpty(sink, path+".witness.name", counterexample.Witness.Name)
		checkNotEmpty(sink, path+".witness.code", counterexample.Witness.Code)
		if len(counterexample.Witness.Code) > SourceViewCap {
			sink.add(CodeCounterexampleSize, path+".witness.code", "the code of a witness is at most %d bytes", SourceViewCap)
		}
	}
}

// BoundedWitness returns the witness with its code cut, at a line boundary when
// there is one, to at most SourceViewCap bytes.
func BoundedWitness(name, clause, code string) WitnessExample {
	witness := WitnessExample{Name: name, Clause: clause, Code: code}
	if len(code) <= SourceViewCap {
		return witness
	}
	cut := code[:SourceViewCap]
	for end := len(cut); end > 0; end-- {
		if cut[end-1] == '\n' {
			cut = cut[:end]
			break
		}
	}
	witness.Code, witness.Truncated = cut, true
	return witness
}
