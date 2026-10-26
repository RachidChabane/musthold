package vacuity

import (
	"slices"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/format"
)

// Line is one line of a source file.
type Line = format.ScanLine

// Difference is what the full-resolution differential search found on one input
// of a mutant, with the two facts that make it evidence.
type Difference struct {
	// Operation is the operation the search called with the input.
	Operation string
	Input     string
	Channel   format.ScanChannel
	// Reference and Mutant are the two results, as the probe wrote them.
	Reference string
	Mutant    string
	// Reproduced is set when a second run on the mutant gave the same result.
	Reproduced bool
	// ReferenceAgrees is set when the reference gave the same result on every
	// run of the input.
	ReferenceAgrees bool
}

// proves reports whether the difference is executed evidence of a behaviour
// change: an input of a named operation, reproduced, on a reference that agrees
// with itself, on a compared channel, with two different results.
func (d Difference) proves() bool {
	return d.Reproduced && d.ReferenceAgrees && d.Operation != "" && d.Input != "" && d.Reference != d.Mutant &&
		slices.Contains(comparedChannels, d.Channel)
}

// comparedChannels are the observations a difference may rest on; logs and the
// standard output are never compared.
var comparedChannels = []format.ScanChannel{
	format.ScanChannelReturnValue, format.ScanChannelException, format.ScanChannelArgumentMutation,
}

// WitnessFailure is a witness that failed on a mutant, with the facts that give
// the failure its power.
type WitnessFailure struct {
	Witness string
	// Admitted is set when the witness passed the reference every run.
	Admitted bool
	// Blind is set when its writer saw no mutant, no diff and no body.
	Blind bool
	// Reproduced is set when the failure repeated on a rerun.
	Reproduced bool
}

func (w WitnessFailure) proves() bool {
	return w.Witness != "" && w.Admitted && w.Blind && w.Reproduced
}

// Mutant is one mutant of a scan and the evidence gathered on it besides the
// test runs.
type Mutant struct {
	ID string
	// Lines are the lines the mutant mutates.
	Lines []Line
	// Sentinel is set for a mutant that keeps behaviour by construction.
	Sentinel bool
	// CompiledIdentical is set when every edited site compiles to the
	// reference's form (E0).
	CompiledIdentical bool
	Differences       []Difference
	WitnessFailures   []WitnessFailure
}

// Site is where a mutant is reported: its first mutated line, by file then
// number.
func (m Mutant) Site() Line {
	return slices.MinFunc(m.Lines, format.CompareScanLines)
}

// eligible reports whether the mutant can be proven at all.
func (m Mutant) eligible() bool {
	return !m.Sentinel && !m.CompiledIdentical && len(m.Lines) > 0
}

// prover answers whether a test's kill proves a behaviour change: the test is
// green and failed no run on any sentinel.
type prover func(test ecosystem.TestID) bool

// routesOf lists, in the format's order and without repeats, the executed routes
// that prove the mutant changes behaviour. A sentinel, a mutant identical in
// compiled form and a mutant that mutates no line have none.
func routesOf(m Mutant, outcomes map[ecosystem.TestID][]Run, canProve prover) []format.ScanRoute {
	if !m.eligible() {
		return nil
	}
	var routes []format.ScanRoute
	for test, runs := range outcomes {
		if Settle(runs) == SettledKilled && canProve(test) {
			routes = append(routes, format.ScanRoute{Kind: format.ScanRouteKilledByTest, Test: string(test)})
		}
	}
	for _, difference := range m.Differences {
		if difference.proves() {
			routes = append(routes, format.ScanRoute{
				Kind: format.ScanRouteDifferential, Operation: difference.Operation, Input: difference.Input, Channel: difference.Channel,
				Reference: difference.Reference, Mutant: difference.Mutant,
			})
		}
	}
	for _, failure := range m.WitnessFailures {
		if failure.proves() {
			routes = append(routes, format.ScanRoute{Kind: format.ScanRouteWitness, Witness: failure.Witness})
		}
	}
	slices.SortFunc(routes, format.CompareScanRoutes)
	return slices.CompactFunc(routes, func(a, b format.ScanRoute) bool { return format.CompareScanRoutes(a, b) == 0 })
}
