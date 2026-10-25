package promise

import (
	"slices"

	"github.com/RachidChabane/spec-proof/format"
)

// Labels map a promise to the functions of the generation view it may break.
type Labels map[string][]string

// NormalizeLabels keeps of the labels a host returned only the promises the
// scan keeps and, for each, only the functions of the generation view. A label
// can name nothing else.
func NormalizeLabels(raw map[string][]string, kept, view []string) Labels {
	labels := Labels{}
	for _, promise := range kept {
		functions := slices.DeleteFunc(slices.Clone(raw[promise]), func(function string) bool {
			return !slices.Contains(view, function)
		})
		slices.Sort(functions)
		if functions = slices.Compact(functions); len(functions) > 0 {
			labels[promise] = functions
		}
	}
	return labels
}

// Of lists, sorted, the promises labeling at least one of the functions.
func (l Labels) Of(functions []string) []string {
	var promises []string
	for promise, labeled := range l {
		if slices.ContainsFunc(functions, func(function string) bool { return slices.Contains(labeled, function) }) {
			promises = append(promises, promise)
		}
	}
	slices.Sort(promises)
	return promises
}

// Label returns the test with the promises that label it: a promise labels a
// test when one of its settled changes sits in a function the promise is
// labeled with. functionOf gives the function of each mutant, by mutant id. A
// label sets the Promises of the test and nothing else: it never creates,
// removes or changes a verdict.
func Label(test format.ScanTest, labels Labels, functionOf map[string]string) format.ScanTest {
	var functions []string
	for _, change := range test.Changes {
		if function, known := functionOf[change.Mutant]; known && settled(change.Outcome) {
			functions = append(functions, function)
		}
	}
	test.Promises = labels.Of(functions)
	return test
}

func settled(outcome format.ScanOutcome) bool {
	return outcome == format.ScanKilled || outcome == format.ScanSurvived
}
