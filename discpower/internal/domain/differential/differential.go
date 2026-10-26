// Package differential is the pure half of F3: it draws the inputs a differential
// search runs on both the reference and a mutant, biased to both sides of every
// constant of the code and seeded with the inputs of the spec examples, and it
// compares what the two did into witnesses. Running the inputs is the work of an
// ecosystem's prober.
package differential

import (
	"cmp"
	"slices"

	"github.com/RachidChabane/musthold/discpower/internal/domain/power"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
)

// MaxWitnesses is the number of shrunk witnesses a search keeps.
const MaxWitnesses = 3

// Hint tells what a prober learned about the type of a parameter.
type Hint string

// Parameter type hints.
const (
	HintInteger  Hint = "integer"
	HintDecimal  Hint = "decimal"
	HintString   Hint = "string"
	HintBoolean  Hint = "boolean"
	HintDatetime Hint = "datetime"
	HintUnknown  Hint = "unknown"
)

// Param is one named parameter of a public operation.
type Param struct {
	Name string
	Hint Hint
}

// Signature is the shape of one bound operation.
type Signature struct {
	// Operation is the name the spec calls the operation by.
	Operation string
	Params    []Param
}

// Compare turns the observations of one input set on the reference and on a
// mutant into the outcome of the search: the simplest inputs on which the
// reference returns and the mutant differs or raises, at most MaxWitnesses of
// them; no difference found when the two always agree.
func Compare(calls []spec.Call, reference, mutant []spec.Observation) power.Differential {
	var differing []power.Witness
	anyDifference := false
	for index, call := range calls {
		if index >= len(reference) || index >= len(mutant) || reference[index].Same(mutant[index]) {
			continue
		}
		anyDifference = true
		if reference[index].Kind == spec.Returns {
			differing = append(differing, power.Witness{Call: call, Reference: reference[index], Mutant: mutant[index]})
		}
	}
	if !anyDifference {
		return power.Differential{Status: power.NoDiffFound}
	}
	slices.SortStableFunc(differing, func(a, b power.Witness) int {
		return cmp.Or(cmp.Compare(callSize(a.Call), callSize(b.Call)), cmp.Compare(callKey(a.Call), callKey(b.Call)))
	})
	if len(differing) > MaxWitnesses {
		differing = differing[:MaxWitnesses]
	}
	return power.Differential{Status: power.DiffersAt, Witnesses: differing}
}

func callSize(call spec.Call) int {
	size := 0
	for _, arg := range call.Args {
		size += len(arg.Value)
	}
	return size
}

// SkippedBecause builds the outcome of a search that could not run.
func SkippedBecause(reason string) power.Differential {
	return power.Differential{Status: power.Skipped, SkipReason: reason}
}
