package cli

import (
	"fmt"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/prove"
	"github.com/RachidChabane/spec-proof/format"
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
		lines := []string{head + fmt.Sprintf("fails the witness %s, which checks %q:", witness.Name, witness.Clause)}
		for _, line := range strings.Split(strings.TrimRight(witness.Code, "\n"), "\n") {
			lines = append(lines, survivorIndent+line)
		}
		if witness.Truncated {
			lines = append(lines, fmt.Sprintf("%s(the witness is cut at %d bytes)", survivorIndent, format.SourceViewCap))
		}
		return lines
	}
	return []string{strings.TrimSuffix(head, ", ")}
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
