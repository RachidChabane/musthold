package cli

import (
	"fmt"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/app/prove"
	"github.com/RachidChabane/musthold/format"
)

// Words of a survivor's lines.
const (
	survivorIDLength = 12
	survivorIndent   = "    "
)

// survivorLines say what the local finding of a blocking survivor shows: the
// mutant's reference, how it was drawn and the input of the criterion that every
// test passes on while the mutant fails it. They never show the mutant's code.
func survivorLines(survivor prove.Survivor) []string {
	counterexample := survivor.Counterexample
	head := fmt.Sprintf("  survivor %s: %s, which every test of the criterion passes, ", abbreviated(survivor.Mutant), drawnAs(counterexample))
	switch {
	case counterexample.Example != nil && survivor.Example != nil:
		return []string{head + fmt.Sprintf("fails its example %d: %s", *counterexample.Example, survivor.Example)}
	case counterexample.Example != nil:
		return []string{head + fmt.Sprintf("fails its example %d", *counterexample.Example)}
	case counterexample.Witness != nil:
		witness := counterexample.Witness
		head += fmt.Sprintf("fails the witness %s, which checks %q:", witness.Name, witness.Clause)
		return append([]string{head}, witnessCodeLines(witness.Code, witness.Truncated)...)
	}
	return []string{strings.TrimSuffix(head, ", ")}
}

// failedWitnessLines say what a witness the code under proof fails shows: its
// name, the clause it checks and, in a local run, its code, which holds the
// input and the output the spec fixes for it.
func failedWitnessLines(failed format.FailedWitness) []string {
	head := fmt.Sprintf("  spec witness %s, which checks %q, fails in every run, and every scope judge holds its output fixed by the spec", failed.Name, failed.Clause)
	if failed.Code == "" {
		return []string{head}
	}
	return append([]string{head + ":"}, witnessCodeLines(failed.Code, failed.Truncated)...)
}

// witnessCodeLines indent a witness's code and say when it was cut.
func witnessCodeLines(code string, truncated bool) []string {
	var lines []string
	for _, line := range strings.Split(strings.TrimRight(code, "\n"), "\n") {
		lines = append(lines, survivorIndent+line)
	}
	if truncated {
		lines = append(lines, fmt.Sprintf("%s(the witness is cut at %d bytes)", survivorIndent, format.SourceViewCap))
	}
	return lines
}

// drawnAs says how the mutant was drawn.
func drawnAs(counterexample format.Counterexample) string {
	if counterexample.Origin == format.OriginTargeted {
		return fmt.Sprintf("a targeted mutant of slot %s (%s)", counterexample.Slot, format.SlotRole(counterexample.Slot))
	}
	return fmt.Sprintf("an operator mutant of family %s", counterexample.Family)
}

func abbreviated(id string) string {
	if len(id) <= survivorIDLength {
		return id
	}
	return id[:survivorIDLength]
}
