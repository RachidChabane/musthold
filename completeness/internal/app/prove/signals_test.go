package prove_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/completeness/internal/app/ports"
	"github.com/RachidChabane/musthold/completeness/internal/app/prove"
	"github.com/RachidChabane/musthold/completeness/internal/domain/closedq"
	"github.com/RachidChabane/musthold/completeness/internal/domain/criterion"
	"github.com/RachidChabane/musthold/completeness/internal/testkit"
	"github.com/RachidChabane/musthold/format"
)

// everythingMissing answers MISSING to every dimension, so that a question is
// raised for every dimension that was asked.
func everythingMissing() testkit.Answerer {
	honest := testkit.Honest()
	return testkit.Answerer{Choose: func(question closedq.Question, state string, vote int) (string, float64) {
		if slices.Contains(question.OptionNames(), "MISSING") {
			return "MISSING", 1
		}
		return honest.Choose(question, state, vote)
	}}
}

func TestOutOfScope(t *testing.T) {
	t.Run("a dimension the spec declares out of scope is never asked and never yields a question, and the declaration is recorded [verifies SP-501~1]", func(t *testing.T) {
		w := newWorld(t, testkit.PythonLedger)
		w.answers = everythingMissing()
		w.mustRun(prove.Options{})
		statement := w.statement()
		if got := questionsOf(statement, "permissions"); len(got) != 0 {
			t.Fatalf("permissions is declared out of scope and yields %+v", got)
		}
		for _, dimension := range []string{"error_handling", "boundaries", "concurrency", "compatibility", "input_validation", "time_and_locale", "state_and_persistence"} {
			if got := questionsOf(statement, dimension); len(got) != 1 {
				t.Errorf("%s: %+v", dimension, got)
			}
		}
		declared := statement.Predicate.OutOfScopeDeclared
		if len(declared) != 1 || declared[0].Dimension != "permissions" || declared[0].Reason != "single-user tool" {
			t.Fatalf("declarations %+v", declared)
		}
		entries, err := w.store.ListEntries()
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			request, err := w.store.GetRequest(entry.Ticket.RequestID)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(request.Closed), `"id":"permissions"`) {
				t.Fatal("the permissions question was put to the host")
			}
		}
	})

	t.Run("a spec without declarations is asked about every dimension [verifies SP-501~1]", func(t *testing.T) {
		w := newWorld(t, testkit.GoLedger)
		w.answers = everythingMissing()
		w.mustRun(prove.Options{})
		if got := questionsOf(w.statement(), "permissions"); len(got) != 1 {
			t.Fatalf("permissions: %+v", got)
		}
		if declared := w.statement().Predicate.OutOfScopeDeclared; len(declared) != 0 {
			t.Fatalf("declarations %+v", declared)
		}
	})

	t.Run("a spec that declares every dimension out of scope asks no dimension question [verifies SP-501~1]", func(t *testing.T) {
		w := newWorld(t, testkit.GoLedger)
		for index, name := range []string{"error_handling", "boundaries", "permissions", "concurrency", "compatibility", "input_validation", "time_and_locale", "state_and_persistence"} {
			w.report.Project.Specs[0].OutOfScope = append(w.report.Project.Specs[0].OutOfScope, criterion.Declaration{Dimension: name, Reason: "not here", Line: index})
		}
		w.mustRun(prove.Options{})
		if got := w.requestKinds()["dimensions"]; got != 0 {
			t.Fatalf("%d dimension requests", got)
		}
		if got := len(w.statement().Predicate.OutOfScopeDeclared); got != 8 {
			t.Fatalf("%d declarations", got)
		}
	})
}

func TestChangedFiles(t *testing.T) {
	t.Run("only source files that hold no tests, lie outside the test roots and are not specs are read for hunks [verifies SP-506~1]", func(t *testing.T) {
		w := newWorld(t, testkit.PythonLedger)
		changed := []ports.Change{
			{Path: "tests/test_totals.py", OldPath: "tests/test_totals.py", After: []byte("def test_new():\n    assert True\n")},
			{Path: "src/invoicing/test_helpers.py", OldPath: "src/invoicing/test_helpers.py", After: []byte("def helper():\n    return 1\n")},
			{Path: "checks/audit.py", OldPath: "checks/audit.py", After: []byte("def audit():\n    return 1\n")},
			{Path: "specs/invoicing.md", OldPath: "specs/invoicing.md", After: []byte("# changed\n")},
			{Path: "README.md", OldPath: "README.md", After: []byte("def not_python():\n")},
			{Path: "src/invoicing/gone.py", OldPath: "src/invoicing/gone.py", Before: []byte("def gone():\n    return 1\n")},
			{Path: "src/invoicing/extra.py", OldPath: "src/invoicing/extra.py", After: []byte("def extra_fee(total):\n    return total\n")},
		}
		w.changes = changeSource{set: ports.ChangeSet{Base: "abc", Changes: changed}}
		w.report.Project.TestRoots = []string{"tests", "checks"}
		w.mustRun(prove.Options{})
		if got := w.requestKinds()["hunk"]; got != 1 {
			t.Fatalf("%d hunk requests, want only the one for extra_fee", got)
		}
	})

	t.Run("a changed file that does not parse is reported and leaves the others [verifies SP-506~1]", func(t *testing.T) {
		w := newWorld(t, testkit.GoLedger)
		set := testkit.FixtureChanges(t, testkit.GoLedger)
		set.Changes = append(set.Changes, ports.Change{Path: "broken.go", OldPath: "broken.go", After: []byte("package x\nfunc (")})
		w.changes = changeSource{set: set}
		result := w.mustRun(prove.Options{})
		if !slices.ContainsFunc(result.Findings, func(finding string) bool { return strings.HasPrefix(finding, "unreadable_source broken.go") }) {
			t.Fatalf("findings %v", result.Findings)
		}
		if got := w.requestKinds()["hunk"]; got != 4 {
			t.Fatalf("%d hunk requests", got)
		}
	})
}

func TestNoBase(t *testing.T) {
	t.Run("a project without a version-controlled base is reported and gets the signals that need no diff [verifies SP-507~1]", func(t *testing.T) {
		w := newWorld(t, testkit.GoLedger)
		w.changes = changeSource{err: ports.ErrNoRepository}
		result := w.mustRun(prove.Options{})
		if !slices.ContainsFunc(result.Findings, func(finding string) bool { return strings.HasPrefix(finding, "no_base:") }) {
			t.Fatalf("findings %v", result.Findings)
		}
		statement := w.statement()
		if got := w.requestKinds()["hunk"]; got != 0 {
			t.Fatalf("%d hunk requests without a base", got)
		}
		if len(questionsOf(statement, "error_handling")) != 1 || len(questionsOf(statement, "clarity")) != 1 {
			t.Fatalf("questions %v", questionLines(statement))
		}
	})
}

func TestBudget(t *testing.T) {
	t.Run("changed units beyond max_hunks are reported and not asked, the first in file and line order being kept [verifies SP-510~1]", func(t *testing.T) {
		w := newWorld(t, testkit.PythonLedger)
		w.policy.MaxHunks = 2
		result := w.mustRun(prove.Options{})
		if got := w.requestKinds()["hunk"]; got != 2 {
			t.Fatalf("%d hunk requests", got)
		}
		if !slices.ContainsFunc(result.Findings, func(finding string) bool {
			return strings.HasPrefix(finding, "hunks_unreviewed") && strings.Contains(finding, "3 changed units")
		}) {
			t.Fatalf("findings %v", result.Findings)
		}
	})
}

func TestTournament(t *testing.T) {
	t.Run("above 23 criteria a changed unit is put through groups of at most 23 and a final round [verifies SP-510~1]", func(t *testing.T) {
		w := newWorld(t, testkit.GoLedger)
		spec := &w.report.Project.Specs[0]
		for index := range 30 {
			spec.Criteria = append(spec.Criteria, criterion.Criterion{
				Ref:             criterion.Ref{ID: fmt.Sprintf("INV-%d", index+10), Revision: 1},
				Statement:       fmt.Sprintf("WHEN case %d happens THE SYSTEM SHALL add the amount.", index),
				StatementDigest: format.SHA256Hex([]byte(fmt.Sprint(index))),
			})
		}
		set := testkit.FixtureChanges(t, testkit.GoLedger)
		set.Changes = []ports.Change{{Path: "extra.go", OldPath: "extra.go", After: []byte("package ledger\n\nfunc Surcharge(total float64) float64 { return total }\n")}}
		w.changes = changeSource{set: set}
		honest := testkit.Honest()
		w.answers = testkit.Answerer{Choose: func(question closedq.Question, state string, vote int) (string, float64) {
			if question.ID == "criterion" && len(question.Options) <= 3 {
				return "NONE", 1
			}
			return honest.Choose(question, state, vote)
		}}
		w.mustRun(prove.Options{})
		var sizes []int
		entries, err := w.store.ListEntries()
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, entry := range entries {
			if seen[string(entry.Ticket.RequestID)] {
				continue
			}
			seen[string(entry.Ticket.RequestID)] = true
			request, err := w.store.GetRequest(entry.Ticket.RequestID)
			if err != nil {
				t.Fatal(err)
			}
			var set closedq.Set
			if err := json.Unmarshal(request.Closed, &set); err != nil {
				t.Fatal(err)
			}
			if set.Questions[0].ID == "criterion" {
				sizes = append(sizes, len(set.Questions[0].Options))
			}
		}
		slices.Sort(sizes)
		if len(sizes) != 3 || sizes[0] != 3 || sizes[2] > 24 {
			t.Fatalf("hunk request sizes %v: want two groups and a final round, each with at most 23 criteria and NONE", sizes)
		}
		if got := questionsOf(w.statement(), "unjustified_change"); len(got) != 1 {
			t.Fatalf("the final round ended on NONE and raised %+v", got)
		}
	})
}

func TestAttribution(t *testing.T) {
	t.Run("the question of an unjustified change goes to the spec whose requirements execute the file, else to the first spec [verifies SP-556~1]", func(t *testing.T) {
		w := newWorld(t, testkit.PythonLedger)
		w.report.Project.Specs = append(w.report.Project.Specs, criterion.Spec{
			Path:     "specs/shipping.md",
			Criteria: []criterion.Criterion{{Ref: criterion.Ref{ID: "SHP-1", Revision: 1}, Statement: "WHEN a parcel is dispatched THE SYSTEM SHALL record the carrier.", StatementDigest: format.SHA256Hex([]byte("shp"))}},
		})
		w.writeSpec("specs/shipping.md", "# Shipping\n")
		power, _ := testkit.PythonEvidence()
		power.Ref, power.Digest = criterion.Ref{ID: "SHP-1", Revision: 1}, format.SHA256Hex([]byte("shp"))
		power.Examples, power.Unattributed = nil, 0
		power.CodeFiles = []string{"src/invoicing/totals.py"}
		power.Write(t, w.root)
		w.mustRun(prove.Options{})
		statements := w.proofs.bySpec(t)
		if got := questionsOf(statements["specs/shipping.md"], "unjustified_change"); len(got) != 1 || !strings.Contains(got[0].Question, "apply_discount") {
			t.Errorf("the shipping spec, whose requirement executes the totals file, holds %+v, want apply_discount", got)
		}
		if got := questionsOf(statements["specs/invoicing.md"], "unjustified_change"); len(got) != 1 || !strings.Contains(got[0].Question, "receipt_line") {
			t.Fatalf("the first spec holds %+v, want the unclaimed receipt_line", got)
		}
	})
}

func TestAdvisory(t *testing.T) {
	t.Run("whatever the host answers, the Statement is advisory, carries no verdict and is valid [verifies SP-519~1]", func(outer *testing.T) {
		w := newWorld(outer, testkit.GoLedger)
		power, aligned := testkit.GoEvidence()
		w.withEvidence(power, aligned)
		rapid.Check(outer, func(t *rapid.T) {
			w.proofs.statements = map[string][]byte{}
			w.memo.entries = map[string][]byte{}
			w.answers = testkit.Answerer{Choose: func(question closedq.Question, _ string, _ int) (string, float64) {
				options := question.OptionNames()
				return rapid.SampledFrom(options).Draw(t, "choice"), rapid.Float64Range(0, 1).Draw(t, "confidence")
			}}
			if rapid.Bool().Draw(t, "relayed") {
				w.answers = relay{honest: w.answers, confidence: rapid.Float64Range(0, 1).Draw(t, "stated")}
			}
			w.policy.TauAdv = format.UniformThreshold(rapid.Float64Range(0, 1).Draw(t, "tau"))
			w.store = freshStore(outer)
			w.mustRun(prove.Options{})
			for _, content := range w.proofs.statements {
				var document map[string]any
				if err := json.Unmarshal(content, &document); err != nil {
					t.Fatal(err)
				}
				predicate := document["predicate"].(map[string]any)
				if predicate["advisory"] != true {
					t.Fatalf("advisory is %v", predicate["advisory"])
				}
				if hasKey(document, "verdict") {
					t.Fatalf("the Statement carries a verdict: %s", content)
				}
				if statement, err := format.DecodeAs[format.Completeness](content); err != nil || statement.Validate() != nil {
					t.Fatalf("the Statement is invalid: %v %v", err, statement.Validate())
				}
			}
		})
	})
}

func hasKey(value any, key string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for name, member := range typed {
			if name == key || hasKey(member, key) {
				return true
			}
		}
	case []any:
		for _, member := range typed {
			if hasKey(member, key) {
				return true
			}
		}
	}
	return false
}
