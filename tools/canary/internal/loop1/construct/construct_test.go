package construct_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

const (
	passingDomain  = "library-late-fees"
	repairedDomain = "train-fare"
	droppedDomain  = "bicycle-rental"
)

// threeDomainHandler answers the roles for three domains: the first is right at
// once, the second needs one repair of the author, the third is never right.
func threeDomainHandler(w func() *world) func(construct.Request) ([]construct.File, error) {
	return func(request construct.Request) ([]construct.File, error) {
		sample := w().sample
		if request.Role == records.RoleOracle {
			return oracleFiles(sample), nil
		}
		switch {
		case request.Domain == repairedDomain && request.Purpose == records.PurposeGenerate:
			return authorFiles(sample, func(files map[string]string) {
				files["violations/FARE-1/1/zones.py"] = strings.Replace(files["violations/FARE-1/1/zones.py"], "# BREAKS: FARE-1", "# harmless", 1)
			}), nil
		case request.Domain == droppedDomain:
			return authorFiles(sample, func(files map[string]string) {
				files["fixture/fare/zones.py"] = strings.Replace(files["fixture/fare/zones.py"], "  # BREAKS: FARE-4", "", 1)
			}), nil
		}
		return authorFiles(sample, nil), nil
	}
}

func newThreeDomainWorld(t *testing.T) *world {
	var w *world
	w = newWorld(t, domains(passingDomain, repairedDomain, droppedDomain), threeDomainHandler(func() *world { return w }))
	w.markBoxofficeBuilt()
	return w
}

func TestConstructAndAccept(t *testing.T) {
	t.Run("one domain passes, one is repaired, one is dropped after two failed repairs of each role [verifies SP-2540~2]", func(t *testing.T) {
		w := newThreeDomainWorld(t)
		summary, err := w.constructor.Run(t.Context())
		if !errors.Is(err, construct.ErrTooFewTasks) {
			t.Fatalf("error %v", err)
		}
		if got := strings.Join(summary.Accepted, ","); got != passingDomain+","+repairedDomain {
			t.Fatalf("accepted %q", got)
		}
		if len(summary.Dropped) != 1 || summary.Dropped[0].Domain != droppedDomain || !strings.HasPrefix(summary.Dropped[0].Check, "A4") {
			t.Fatalf("dropped %+v", summary.Dropped)
		}
		listed, err := accept.ListedTasks(w.constructor.Config)
		if err != nil || strings.Join(listed, ",") != "boxoffice,"+passingDomain+","+repairedDomain {
			t.Fatalf("listed %v %v", listed, err)
		}
		if w.constructor.Store.Exists(droppedDomain) {
			t.Error("the dropped domain left files in the repository")
		}
	})

	t.Run("every call, every check and the drop are recorded with their provenance [verifies SP-2549~2]", func(t *testing.T) {
		w := newThreeDomainWorld(t)
		_, _ = w.constructor.Run(t.Context())
		log := w.log()
		count := map[string]int{}
		for _, call := range log.Calls {
			count[call.Domain+"/"+string(call.Role)+"/"+string(call.Purpose)]++
			if call.PromptSHA256 == "" || call.Schema == "" || call.Output == "" || call.CodexVersion != fakeVersion || call.Start.IsZero() || call.End.Before(call.Start) || call.Attempt < 1 {
				t.Errorf("call %+v lacks a member", call)
			}
		}
		want := map[string]int{
			passingDomain + "/author/generate": 1, passingDomain + "/oracle/generate": 1,
			repairedDomain + "/author/generate": 1, repairedDomain + "/author/repair": 1, repairedDomain + "/oracle/generate": 1, repairedDomain + "/oracle/repair": 1,
			droppedDomain + "/author/generate": 1, droppedDomain + "/author/repair": 2, droppedDomain + "/oracle/generate": 1, droppedDomain + "/oracle/repair": 2,
		}
		for key, n := range want {
			if count[key] != n {
				t.Errorf("%s: %d calls, want %d (%v)", key, count[key], n, count)
			}
		}
		if len(log.Drops) != 1 || log.Drops[0].Domain != droppedDomain || log.Drops[0].Reason == "" {
			t.Errorf("drops %+v", log.Drops)
		}
		failed := 0
		for _, check := range log.Checks {
			if !check.Passed && check.Domain == droppedDomain && check.ID == accept.CheckBase {
				failed++
			}
		}
		if failed != 3 {
			t.Errorf("%d failed A4 records for the dropped domain, want 3 (one per round)", failed)
		}
	})

	t.Run("repair calls go to the role a check concerns, at most two per role and domain [verifies SP-2547~2]", func(t *testing.T) {
		w := newThreeDomainWorld(t)
		_, _ = w.constructor.Run(t.Context())
		repairs := map[string]int{}
		for _, request := range w.model.requests {
			if request.Purpose == records.PurposeRepair {
				repairs[request.Domain+"/"+string(request.Role)]++
			}
		}
		for key, n := range repairs {
			if n > construct.MaxRepairs {
				t.Errorf("%s: %d repairs", key, n)
			}
		}
		if repairs[repairedDomain+"/oracle"] != 1 || repairs[repairedDomain+"/author"] != 1 {
			t.Errorf("repairs %v", repairs)
		}
	})

	t.Run("a dropped domain is logged with its failing check and never repaired by hand [verifies SP-2548~1]", func(t *testing.T) {
		w := newThreeDomainWorld(t)
		summary, _ := w.constructor.Run(t.Context())
		drop := summary.Dropped[0]
		if !strings.Contains(drop.Check, "base") || drop.Reason == "" {
			t.Fatalf("%+v", drop)
		}
		if _, err := os.Stat(filepath.Join(w.repo, study.TaskDir(droppedDomain))); !os.IsNotExist(err) {
			t.Fatalf("the dropped task remains: %v", err)
		}
	})

	t.Run("the test writer never receives the project's source, tests, reference, violations or refactors, in a first call, a repair or a rewrite [verifies SP-2544~1]", func(t *testing.T) {
		w := newThreeDomainWorld(t)
		_, _ = w.constructor.Run(t.Context())
		forbidden := []string{"SHORT_LIMIT", "MEDIUM_LIMIT", "return 9", "return 14", "BREAKS", "TESTS-MISS", "verifies(", "test_short_trip_costs_two", "zones.py", "fare/__init__", "violations/", "refactors/", "reference/"}
		oracleCalls := w.model.callsOf(records.RoleOracle)
		if len(oracleCalls) < 5 {
			t.Fatalf("%d test-writer calls", len(oracleCalls))
		}
		for _, request := range oracleCalls {
			for _, word := range forbidden {
				if strings.Contains(request.Prompt, word) {
					t.Errorf("the prompt of %s %s attempt %d holds %q", request.Domain, request.Purpose, request.Attempt, word)
				}
			}
			if !strings.Contains(request.Prompt, "WHEN the distance is 0 to 10 THE SYSTEM SHALL charge 2.") || !strings.Contains(request.Prompt, "fare_for(distance)") {
				t.Errorf("the prompt of %s lacks the spec or the interface", request.Domain)
			}
		}
	})

	t.Run("what a repair tells the test writer names tests and criteria, and the author is told the details [verifies SP-2547~2]", func(t *testing.T) {
		w := newThreeDomainWorld(t)
		_, _ = w.constructor.Run(t.Context())
		var oracleRepair, authorRepair string
		for _, request := range w.model.requests {
			if request.Domain == droppedDomain && request.Purpose == records.PurposeRepair {
				if request.Role == records.RoleOracle {
					oracleRepair = request.Prompt
				} else {
					authorRepair = request.Prompt
				}
			}
		}
		if !strings.Contains(oracleRepair, "no test of FARE-4 fails against an implementation that lacks the behaviour it states") {
			t.Errorf("the test writer's repair lacks its finding:\n%s", oracleRepair)
		}
		if !strings.Contains(authorRepair, "its source already behaves as the criterion states") || !strings.Contains(authorRepair, "# Previous answer") {
			t.Errorf("the author's repair lacks its finding or its previous answer")
		}
		if strings.Contains(oracleRepair, "already behaves") {
			t.Error("the test writer was told what the author's check found")
		}
	})
}

func TestConstructionStopsAtFiveDomains(t *testing.T) {
	t.Run("domains are processed in order and construction stops once five have passed [verifies SP-2540~2]", func(t *testing.T) {
		names := []string{"d1", "d2", "d3", "d4", "d5", "d6", "d7"}
		var w *world
		w = newWorld(t, domains(names...), func(request construct.Request) ([]construct.File, error) {
			if request.Role == records.RoleOracle {
				return oracleFiles(w.sample), nil
			}
			return authorFiles(w.sample, nil), nil
		})
		w.markBoxofficeBuilt()
		summary, err := w.constructor.Run(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(summary.Accepted, ",") != "d1,d2,d3,d4,d5" {
			t.Fatalf("accepted %v", summary.Accepted)
		}
		for _, request := range w.model.requests {
			if request.Domain == "d6" || request.Domain == "d7" {
				t.Fatalf("a call concerned %s after five domains passed", request.Domain)
			}
		}
		listed, _ := accept.ListedTasks(w.constructor.Config)
		if len(listed) != 6 || listed[1] != "d1" || listed[5] != "d5" {
			t.Fatalf("listed %v", listed)
		}
	})

	t.Run("a domain already built and listed counts as passed and is not built again [verifies SP-2540~2]", func(t *testing.T) {
		var w *world
		w = newWorld(t, domains("d1", "d2"), func(request construct.Request) ([]construct.File, error) {
			if request.Role == records.RoleOracle {
				return oracleFiles(w.sample), nil
			}
			return authorFiles(w.sample, nil), nil
		})
		w.markBoxofficeBuilt()
		w.constructor.Target = 2
		if _, err := w.constructor.Run(t.Context()); err != nil {
			t.Fatal(err)
		}
		calls := len(w.model.requests)
		again, err := w.constructor.Run(t.Context())
		if err != nil || len(again.Accepted) != 0 || len(w.model.requests) != calls {
			t.Fatalf("%v %+v, %d calls more", err, again, len(w.model.requests)-calls)
		}
	})
}

func TestACallThatFailsIsRepairedAsTheRoleItConcerns(t *testing.T) {
	t.Run("a call that returns no usable files consumes a repair call, then the domain passes [verifies SP-2547~2]", func(t *testing.T) {
		var w *world
		w = newWorld(t, domains("d1"), func(request construct.Request) ([]construct.File, error) {
			switch {
			case request.Role == records.RoleOracle:
				return oracleFiles(w.sample), nil
			case request.Purpose == records.PurposeGenerate:
				return nil, construct.ErrBadReply
			}
			return authorFiles(w.sample, nil), nil
		})
		w.markBoxofficeBuilt()
		w.constructor.Target = 1
		summary, err := w.constructor.Run(t.Context())
		if err != nil || len(summary.Accepted) != 1 {
			t.Fatalf("%v %+v", err, summary)
		}
		log := w.log()
		if len(log.Calls) != 3 || log.Calls[0].Error == "" || log.Calls[1].Purpose != records.PurposeRepair {
			t.Fatalf("calls %+v", log.Calls)
		}
	})

	t.Run("files outside the task layout are refused and reported to the author [verifies SP-2543~1]", func(t *testing.T) {
		var w *world
		w = newWorld(t, domains("d1"), func(request construct.Request) ([]construct.File, error) {
			if request.Role == records.RoleOracle {
				return oracleFiles(w.sample), nil
			}
			files := authorFiles(w.sample, nil)
			if request.Purpose == records.PurposeGenerate {
				files = append(files, construct.File{Path: "../escape.py", Content: "x"})
			}
			return files, nil
		})
		w.markBoxofficeBuilt()
		w.constructor.Target = 1
		if _, err := w.constructor.Run(t.Context()); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(w.repo, "experiments/loop-1/escape.py")); err == nil {
			t.Fatal("a file was written outside the task")
		}
		repair := w.model.callsOf(records.RoleAuthor)[1].Prompt
		if !strings.Contains(repair, "not a clean relative path") {
			t.Fatalf("the repair does not name the problem:\n%s", repair)
		}
	})
}
