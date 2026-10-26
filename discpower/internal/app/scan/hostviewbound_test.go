package scan_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/RachidChabane/musthold/discpower/internal/app/scan"
	"github.com/RachidChabane/musthold/format/hostproto"
)

const (
	verifiesViewBoundScan = " [verifies SP-2272~1]"
	// catalog is a module of the fake project whose functions together pass
	// the size bound of a view.
	catalog = "invoice/catalog.py"
	// catalogFunctions and catalogFunctionSize make the module twice the
	// bound.
	catalogFunctions    = 64
	catalogFunctionSize = 2 * scan.ViewSourceCap / catalogFunctions
	omittedNote         = "more units of this code are not shown: the code above is cut to a size bound."
	// promptOverhead bounds what a generation prompt holds besides its code.
	promptOverhead = 8 << 10
)

// catalogSource is a module of catalogFunctions functions of about
// catalogFunctionSize bytes each.
func catalogSource() string {
	var source strings.Builder
	for index := range catalogFunctions {
		fmt.Fprintf(&source, "def price_%d(total: int) -> int:\n    \"\"\"%s\"\"\"\n    return total + %d\n\n\n", index, strings.Repeat("x", catalogFunctionSize), index)
	}
	return source.String()
}

// newLargeFixture is the host fixture whose project holds the catalog
// module, and whose witnesses record no coverage, so that every view starts
// from every unit of the code.
func newLargeFixture() *hostFixture {
	f := newHostFixture()
	f.workspaces.project.(fstest.MapFS)[catalog] = &fstest.MapFile{Data: []byte(catalogSource())}
	f.source.tracked = []string{catalog}
	return f
}

// requestsOfKind lists the prompts of the session's requests of a kind.
func requestsOfKind(t *testing.T, store hostproto.HostStore, session hostproto.Session, kind hostproto.Kind) []string {
	t.Helper()
	var prompts []string
	for _, id := range sessionRequests(t, store, session) {
		request, err := store.GetRequest(id)
		if err != nil {
			t.Fatal(err)
		}
		if request.Kind == kind {
			prompts = append(prompts, request.Prompt.User)
		}
	}
	return prompts
}

func TestHostViewBound(t *testing.T) {
	f := newLargeFixture()
	results := f.loop(t, hostRequest(), 6)
	final := results[len(results)-1]
	t.Run("a view past the size bound is cut, and the report names the generation and labels views it cut"+verifiesViewBoundScan, func(t *testing.T) {
		truncated := final.Report.Promises.Truncated
		if !slices.ContainsFunc(truncated, scan.TruncatedView.OfLabels) || !slices.ContainsFunc(truncated, func(view scan.TruncatedView) bool { return !view.OfLabels() }) {
			t.Fatalf("truncated %+v", truncated)
		}
		for _, view := range truncated {
			if view.Shown == 0 || view.Shown >= view.Total {
				t.Errorf("view %+v", view)
			}
		}
	})
	t.Run("the generation and labels requests say how many units the bound left out and hold no more code than it allows"+verifiesViewBoundScan, func(t *testing.T) {
		generation := requestsOfKind(t, f.store, final.Session, hostproto.KindGenerateTargeted)
		labels := requestsOfKind(t, f.store, final.Session, hostproto.KindJudgeLabels)
		if len(generation) == 0 || len(labels) == 0 {
			t.Fatalf("%d generation and %d labels requests", len(generation), len(labels))
		}
		for _, user := range append(generation, labels...) {
			if !strings.Contains(user, omittedNote) || len(user) > scan.ViewSourceCap+promptOverhead {
				t.Errorf("a prompt of %d bytes, note %t", len(user), strings.Contains(user, omittedNote))
			}
		}
	})
	t.Run("the cut views are the same in every scan of the same project, request for request"+verifiesViewBoundScan, func(t *testing.T) {
		again := newLargeFixture()
		replayed := again.loop(t, hostRequest(), 6)
		if before, after := sessionRequests(t, f.store, final.Session), sessionRequests(t, again.store, replayed[len(replayed)-1].Session); !slices.Equal(before, after) {
			t.Fatalf("requests %v, then %v", before, after)
		}
	})
}
