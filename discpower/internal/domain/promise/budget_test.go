package promise_test

import (
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/discpower/internal/domain/promise"
)

const (
	verifiesBudget      = " [verifies SP-2183~1]"
	verifiesReserve     = " [verifies SP-2184~1]"
	verifiesLabelsFirst = " [verifies SP-2185~1]"
	verifiesReverse     = " [verifies SP-2186~1]"
	verifiesRoundRobin  = " [verifies SP-2187~1]"
	verifiesOrder       = " [verifies SP-2188~1]"
)

var keptIDs = []string{"a", "b", "c"}

func writersOf(ids ...string) []promise.Ask {
	var asks []promise.Ask
	for _, id := range ids {
		for slot := range promise.WritersPerPromise {
			asks = append(asks, promise.Writer(id, slot))
		}
	}
	return asks
}

func scopesOf(ids ...string) []promise.Ask {
	var asks []promise.Ask
	for _, id := range ids {
		for slot := range promise.ScopeVotesPerPromise {
			asks = append(asks, promise.Scope(id, slot))
		}
	}
	return asks
}

// scopeStageWanted is what the stage after the writers asks for.
func scopeStageWanted() []promise.Ask {
	return slices.Concat(scopesOf(keptIDs...), promise.AssignGeneration(keptIDs), []promise.Ask{promise.LabelTicket()})
}

// scopeStageUsed is the journal at the scope stage: the extraction, the writers
// of the three promises, and the given number of reissued writers.
func scopeStageUsed(reissued int) []promise.Ask {
	used := slices.Concat([]promise.Ask{promise.Extraction()}, writersOf(keptIDs...))
	return slices.Concat(used, writersOf(keptIDs...)[:reissued])
}

func TestBudgetConstants(t *testing.T) {
	t.Run("the budget is one extraction, four calls for each of three promises, four generation tickets and one labels ticket"+verifiesBudget, func(t *testing.T) {
		if promise.Budget != 18 || promise.CallsPerPromise != 4 || promise.MaxPromises != 3 {
			t.Fatalf("budget %d, per promise %d, promises %d", promise.Budget, promise.CallsPerPromise, promise.MaxPromises)
		}
	})
}

func TestScheduleStages(t *testing.T) {
	t.Run("a fresh scan issues its extraction"+verifiesBudget, func(t *testing.T) {
		got := promise.Schedule(promise.Request{Wanted: []promise.Ask{promise.Extraction()}})
		if !slices.Equal(got.Issue, []promise.Ask{promise.Extraction()}) || len(got.Cut) != 0 {
			t.Fatalf("plan %+v", got)
		}
	})
	t.Run("the writers of three promises are all issued when the budget covers their four calls each"+verifiesReserve, func(t *testing.T) {
		got := promise.Schedule(promise.Request{Kept: keptIDs, Used: []promise.Ask{promise.Extraction()}, Wanted: writersOf(keptIDs...)})
		if len(got.Issue) != promise.MaxPromises*promise.WritersPerPromise || len(got.Cut) != 0 || len(got.Unchecked) != 0 {
			t.Fatalf("plan %+v", got)
		}
	})
	t.Run("a promise the remaining budget cannot cover is dropped and reported unchecked, the later ones first"+verifiesReserve, func(t *testing.T) {
		used := slices.Repeat([]promise.Ask{promise.Extraction()}, 11)
		got := promise.Schedule(promise.Request{Kept: keptIDs, Used: used, Wanted: writersOf(keptIDs...)})
		wantUnchecked := []promise.Unchecked{
			{Promise: "b", Reason: promise.UncheckedBudget},
			{Promise: "c", Reason: promise.UncheckedBudget},
		}
		if !slices.Equal(got.Issue, writersOf("a")) || !slices.Equal(got.Unchecked, wantUnchecked) {
			t.Fatalf("plan %+v", got)
		}
		for _, cut := range got.Cut {
			if cut.Reason != promise.CutBudget || cut.Ask.Promise == "a" {
				t.Fatalf("cut %+v", cut)
			}
		}
	})
	t.Run("the scope votes of a promise are reserved while its writers are emitted"+verifiesReserve, func(t *testing.T) {
		used := slices.Repeat([]promise.Ask{promise.Extraction()}, 15)
		got := promise.Schedule(promise.Request{Kept: keptIDs[:1], Used: used, Wanted: writersOf("a")})
		if len(got.Issue) != 0 || len(got.Unchecked) != 1 {
			t.Fatalf("a writer left no room for its scope votes: %+v", got)
		}
	})
	t.Run("a scan with no reissue issues every scope vote, every generation ticket and the labels ticket"+verifiesLabelsFirst, func(t *testing.T) {
		got := promise.Schedule(promise.Request{Kept: keptIDs, Used: scopeStageUsed(0), Wanted: scopeStageWanted()})
		if len(got.Issue) != len(scopeStageWanted()) || len(got.Cut) != 0 {
			t.Fatalf("plan %+v", got)
		}
	})
	t.Run("one reissued writer cuts the labels ticket and every promise keeps its two scope votes"+verifiesLabelsFirst, func(t *testing.T) {
		got := promise.Schedule(promise.Request{Kept: keptIDs, Used: scopeStageUsed(1), Wanted: scopeStageWanted()})
		if len(got.Cut) != 1 || got.Cut[0] != (promise.Cut{Ask: promise.LabelTicket(), Reason: promise.CutBudget}) {
			t.Fatalf("cut %+v", got.Cut)
		}
		for _, scope := range scopesOf(keptIDs...) {
			if !slices.Contains(got.Issue, scope) {
				t.Fatalf("scope vote %+v not issued", scope)
			}
		}
		if len(got.Unchecked) != 0 {
			t.Fatalf("unchecked %v", got.Unchecked)
		}
	})
	t.Run("generation tickets are cut in the reverse of their round-robin order after the labels ticket"+verifiesReverse, func(t *testing.T) {
		tickets := promise.AssignGeneration(keptIDs)
		for reissued, wantIssued := range map[int]int{2: 3, 3: 2, 4: 1, 5: 0} {
			got := promise.Schedule(promise.Request{Kept: keptIDs, Used: scopeStageUsed(reissued), Wanted: scopeStageWanted()})
			if issued := generationOf(got.Issue); !slices.Equal(issued, tickets[:wantIssued]) {
				t.Errorf("%d reissues issued %v, want the first %d tickets", reissued, issued, wantIssued)
			}
			wantCut := append(slices.Clone(tickets[wantIssued:]), promise.LabelTicket())
			if cut := cutAsks(got.Cut); !slices.Equal(cut, wantCut) {
				t.Errorf("%d reissues cut %v, want %v", reissued, cut, wantCut)
			}
		}
	})
	t.Run("a promise that is not kept is cut with its own reason"+verifiesReverse, func(t *testing.T) {
		stranger := promise.Scope("z", 0)
		got := promise.Schedule(promise.Request{Kept: keptIDs, Wanted: []promise.Ask{stranger}})
		if len(got.Issue) != 0 || !slices.Equal(got.Cut, []promise.Cut{{Ask: stranger, Reason: promise.CutPromiseNotKept}}) {
			t.Fatalf("plan %+v", got)
		}
	})
	t.Run("an exhausted budget issues nothing, not even the extraction"+verifiesBudget, func(t *testing.T) {
		used := slices.Repeat([]promise.Ask{promise.Extraction()}, promise.Budget+2)
		got := promise.Schedule(promise.Request{Used: used, Wanted: []promise.Ask{promise.Extraction()}})
		if len(got.Issue) != 0 || len(got.Cut) != 1 {
			t.Fatalf("plan %+v", got)
		}
	})
}

func TestAssignGeneration(t *testing.T) {
	t.Run("tickets go round-robin over the kept promises in their kept order"+verifiesRoundRobin, func(t *testing.T) {
		got := promise.AssignGeneration(keptIDs)
		want := []promise.Ask{promise.Generation("a", 0), promise.Generation("b", 1), promise.Generation("c", 2), promise.Generation("a", 3)}
		if !slices.Equal(got, want) {
			t.Fatalf("assigned %v", got)
		}
	})
	t.Run("a single promise takes all four tickets and no promise takes none"+verifiesRoundRobin, func(t *testing.T) {
		if got := promise.AssignGeneration([]string{"a"}); len(got) != promise.GenerationTickets || got[3].Promise != "a" {
			t.Fatalf("assigned %v", got)
		}
		if got := promise.AssignGeneration(nil); len(got) != 0 {
			t.Fatalf("assigned %v", got)
		}
	})
	t.Run("tickets differ by at most one between promises"+verifiesRoundRobin, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			count := rapid.IntRange(1, promise.MaxPromises).Draw(t, "promises")
			ids := keptIDs[:count]
			share := map[string]int{}
			for _, ask := range promise.AssignGeneration(ids) {
				share[ask.Promise]++
			}
			for _, id := range ids {
				if share[id] < promise.GenerationTickets/count || share[id] > promise.GenerationTickets/count+1 {
					t.Fatalf("promise %s has %d of %d tickets", id, share[id], promise.GenerationTickets)
				}
			}
		})
	})
}

func generationOf(asks []promise.Ask) []promise.Ask {
	return slices.DeleteFunc(slices.Clone(asks), func(ask promise.Ask) bool { return ask.Category != promise.CategoryGeneration })
}

func cutAsks(cuts []promise.Cut) []promise.Ask {
	asks := make([]promise.Ask, len(cuts))
	for index, cut := range cuts {
		asks[index] = cut.Ask
	}
	return asks
}

var askPool = slices.Concat(
	[]promise.Ask{promise.Extraction(), promise.LabelTicket(), promise.Writer("z", 0)},
	writersOf(keptIDs...),
	scopesOf(keptIDs...),
	promise.AssignGeneration(keptIDs),
)

func drawAsks(t *rapid.T, label string, most int) []promise.Ask {
	return rapid.SliceOfN(rapid.SampledFrom(askPool), 0, most).Draw(t, label)
}

func drawRequest(t *rapid.T) promise.Request {
	return promise.Request{
		Kept:   keptIDs[:rapid.IntRange(0, len(keptIDs)).Draw(t, "kept")],
		Used:   drawAsks(t, "used", 2*promise.Budget),
		Wanted: drawAsks(t, "wanted", len(askPool)),
	}
}

func TestScheduleProperties(t *testing.T) {
	t.Run("the planner never issues more calls than the budget has left"+verifiesBudget, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			request := drawRequest(t)
			got := promise.Schedule(request)
			if len(request.Used)+len(got.Issue) > max(promise.Budget, len(request.Used)) {
				t.Fatalf("%d used and %d issued", len(request.Used), len(got.Issue))
			}
		})
	})
	t.Run("every wanted ask is issued or cut, once"+verifiesBudget, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			request := drawRequest(t)
			got := promise.Schedule(request)
			decided := slices.Concat(got.Issue, cutAsks(got.Cut))
			slices.SortFunc(decided, compareForTest)
			wanted := slices.Clone(request.Wanted)
			slices.SortFunc(wanted, compareForTest)
			if !slices.Equal(decided, slices.Compact(wanted)) {
				t.Fatalf("wanted %v, decided %v", wanted, decided)
			}
		})
	})
	t.Run("the plan does not depend on the order of the emissions or of the asks"+verifiesOrder, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			request := drawRequest(t)
			shuffled := promise.Request{
				Kept:   request.Kept,
				Used:   rapid.Permutation(request.Used).Draw(t, "used order"),
				Wanted: rapid.Permutation(request.Wanted).Draw(t, "wanted order"),
			}
			if a, b := promise.Schedule(request), promise.Schedule(shuffled); !equalPlans(a, b) {
				t.Fatalf("%+v differs from %+v", a, b)
			}
		})
	})
	t.Run("the labels ticket is cut before any generation ticket, and every promise left keeps its scope votes"+verifiesLabelsFirst, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			reissued := rapid.IntRange(0, len(writersOf(keptIDs...))).Draw(t, "reissued")
			extra := rapid.IntRange(0, promise.Budget).Draw(t, "extra emissions")
			used := slices.Concat(scopeStageUsed(reissued), slices.Repeat([]promise.Ask{promise.Extraction()}, extra))
			got := promise.Schedule(promise.Request{Kept: keptIDs, Used: used, Wanted: scopeStageWanted()})
			if len(generationOf(cutAsks(got.Cut))) > 0 && slices.Contains(got.Issue, promise.LabelTicket()) {
				t.Fatalf("labels issued while a generation ticket is cut: %+v", got)
			}
			unchecked := map[string]bool{}
			for _, u := range got.Unchecked {
				unchecked[u.Promise] = true
			}
			for _, id := range keptIDs {
				for _, scope := range scopesOf(id) {
					if !unchecked[id] && !slices.Contains(got.Issue, scope) {
						t.Fatalf("promise %s is kept and lost %+v: %+v", id, scope, got)
					}
				}
			}
		})
	})
}

func compareForTest(a, b promise.Ask) int {
	return slices.Compare(
		[]string{string(a.Category), a.Promise, string(rune('0' + a.Slot))},
		[]string{string(b.Category), b.Promise, string(rune('0' + b.Slot))},
	)
}

func equalPlans(a, b promise.Plan) bool {
	return slices.Equal(a.Issue, b.Issue) && slices.Equal(a.Cut, b.Cut) && slices.Equal(a.Unchecked, b.Unchecked)
}
