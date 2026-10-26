package promise_test

import (
	"reflect"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/discpower/internal/domain/promise"
	"github.com/RachidChabane/musthold/format"
)

const (
	verifiesLabelJoin = " [verifies SP-2193~1]"
	verifiesNoVerdict = " [verifies SP-2194~1]"
)

var (
	labelPromises = []string{"p1", "p2", "p3"}
	labelView     = []string{"f1", "f2", "f3", "f4"}
	labelMutants  = []string{"m1", "m2", "m3", "m4"}
	outcomes      = []format.ScanOutcome{format.ScanKilled, format.ScanSurvived, format.ScanInconclusive}
)

func TestNormalizeLabels(t *testing.T) {
	t.Run("a label names only a promise that is kept and a function of the generation view"+verifiesLabelJoin, func(t *testing.T) {
		raw := map[string][]string{
			"p1":      {"f2", "f1", "f2", "elsewhere"},
			"p2":      {"elsewhere"},
			"unkept":  {"f1"},
			"p3":      nil,
			"p3 also": {"f3"},
		}
		got := promise.NormalizeLabels(raw, labelPromises, labelView)
		if want := (promise.Labels{"p1": {"f1", "f2"}}); !reflect.DeepEqual(got, want) {
			t.Fatalf("labels %v, want %v", got, want)
		}
	})
}

func testWith(changes ...format.ScanChange) format.ScanTest {
	return format.ScanTest{ID: "t1", Class: format.ScanCaught, Changes: changes}
}

func TestLabel(t *testing.T) {
	labels := promise.Labels{"p1": {"f1"}, "p2": {"f2", "f3"}}
	functionOf := map[string]string{"m1": "f1", "m2": "f2", "m3": "f9"}
	t.Run("a test receives each promise labeling the function of one of its settled changes"+verifiesLabelJoin, func(t *testing.T) {
		test := testWith(
			format.ScanChange{Mutant: "m1", Outcome: format.ScanSurvived},
			format.ScanChange{Mutant: "m2", Outcome: format.ScanKilled},
		)
		if got := promise.Label(test, labels, functionOf).Promises; !slices.Equal(got, []string{"p1", "p2"}) {
			t.Fatalf("promises %v", got)
		}
	})
	t.Run("an inconclusive change, a change outside every label and an unknown mutant label nothing"+verifiesLabelJoin, func(t *testing.T) {
		test := testWith(
			format.ScanChange{Mutant: "m1", Outcome: format.ScanInconclusive},
			format.ScanChange{Mutant: "m3", Outcome: format.ScanKilled},
			format.ScanChange{Mutant: "m4", Outcome: format.ScanKilled},
		)
		if got := promise.Label(test, labels, functionOf).Promises; len(got) != 0 {
			t.Fatalf("promises %v", got)
		}
	})
	t.Run("two changes in functions of one promise label the test with it once"+verifiesLabelJoin, func(t *testing.T) {
		test := testWith(format.ScanChange{Mutant: "m2", Outcome: format.ScanKilled}, format.ScanChange{Mutant: "m1", Outcome: format.ScanKilled})
		wide := promise.Labels{"p1": {"f1", "f2"}}
		if got := promise.Label(test, wide, functionOf).Promises; !slices.Equal(got, []string{"p1"}) {
			t.Fatalf("promises %v", got)
		}
	})
}

func drawTest(t *rapid.T) format.ScanTest {
	change := rapid.Custom(func(t *rapid.T) format.ScanChange {
		return format.ScanChange{Mutant: rapid.SampledFrom(labelMutants).Draw(t, "mutant"), Outcome: rapid.SampledFrom(outcomes).Draw(t, "outcome")}
	})
	return format.ScanTest{
		ID:         "t1",
		Class:      rapid.SampledFrom(format.ScanClasses()).Draw(t, "class"),
		Reason:     rapid.SampledFrom([]format.ScanUnprovenReason{"", format.ScanTooFewChanges, format.ScanKillsSentinel}).Draw(t, "reason"),
		Changes:    rapid.SliceOfN(change, 0, 6).Draw(t, "changes"),
		NotStarted: rapid.SliceOfN(rapid.SampledFrom(labelMutants), 0, 2).Draw(t, "not started"),
		Promises:   rapid.SliceOfN(rapid.SampledFrom(labelPromises), 0, 2).Draw(t, "earlier promises"),
	}
}

func drawLabels(t *rapid.T) (promise.Labels, map[string]string) {
	raw := map[string][]string{}
	for _, id := range labelPromises {
		raw[id] = rapid.SliceOfN(rapid.SampledFrom(labelView), 0, 3).Draw(t, "functions of "+id)
	}
	functionOf := map[string]string{}
	for _, mutant := range labelMutants {
		functionOf[mutant] = rapid.SampledFrom(labelView).Draw(t, "function of "+mutant)
	}
	return promise.NormalizeLabels(raw, labelPromises, labelView), functionOf
}

func TestLabelProperties(t *testing.T) {
	t.Run("a label never creates, removes or changes a verdict"+verifiesNoVerdict, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			test := drawTest(t)
			labels, functionOf := drawLabels(t)
			labeled := promise.Label(test, labels, functionOf)
			test.Promises, labeled.Promises = nil, nil
			if !reflect.DeepEqual(test, labeled) {
				t.Fatalf("labeling changed %+v into %+v", test, labeled)
			}
		})
	})
	t.Run("a label is the same whatever the order of the changes and of the labels"+verifiesLabelJoin, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			test := drawTest(t)
			labels, functionOf := drawLabels(t)
			shuffled := test
			shuffled.Changes = rapid.Permutation(test.Changes).Draw(t, "order")
			if a, b := promise.Label(test, labels, functionOf).Promises, promise.Label(shuffled, labels, functionOf).Promises; !slices.Equal(a, b) {
				t.Fatalf("%v differs from %v", a, b)
			}
		})
	})
	t.Run("a promise labels a test only through a settled change in one of its functions"+verifiesLabelJoin, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			test := drawTest(t)
			labels, functionOf := drawLabels(t)
			for _, id := range promise.Label(test, labels, functionOf).Promises {
				found := slices.ContainsFunc(test.Changes, func(c format.ScanChange) bool {
					return (c.Outcome == format.ScanKilled || c.Outcome == format.ScanSurvived) && slices.Contains(labels[id], functionOf[c.Mutant])
				})
				if !found {
					t.Fatalf("promise %s labels %+v with no change in %v", id, test, labels[id])
				}
			}
		})
	})
}
