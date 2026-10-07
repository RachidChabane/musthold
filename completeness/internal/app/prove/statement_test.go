package prove_test

import (
	"bytes"
	"flag"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/completeness/internal/app/prove"
	"github.com/RachidChabane/spec-proof/completeness/internal/testkit"
	"github.com/RachidChabane/spec-proof/format"
)

// rapidChecks bounds the iterations of the properties that run the whole use case.
const rapidChecks = "15"

func TestMain(m *testing.M) {
	if err := flag.Set("rapid.checks", rapidChecks); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func inputsOf(statement format.Statement[format.Completeness], role format.InputRole) []format.Input {
	var found []format.Input
	for _, input := range statement.Predicate.Inputs {
		if input.Role == role {
			found = append(found, input)
		}
	}
	return found
}

func TestStatement(t *testing.T) {
	w := newWorld(t, testkit.PythonLedger)
	power, aligned := testkit.PythonEvidence()
	w.withEvidence(power, aligned)
	w.mustRun(prove.Options{})
	statement := w.statement()

	t.Run("the Statement is about the spec file and its hash, advisory, valid and canonical [verifies SP-526~1]", func(t *testing.T) {
		if len(statement.Subject) != 1 || statement.Subject[0].Name != "specs/invoicing.md" || statement.Subject[0].Digest.SHA256 != testkit.FileDigest(t, w.root, "specs/invoicing.md") {
			t.Fatalf("subject %+v", statement.Subject)
		}
		if !statement.Predicate.Advisory || statement.Predicate.ProducedBy != format.ProducedByLocal || statement.Predicate.Module.Name != "spec-proof-completeness" {
			t.Fatalf("predicate %+v", statement.Predicate.Core)
		}
		encoded, err := statement.Encode()
		if err != nil {
			t.Fatal(err)
		}
		for _, stored := range w.proofs.statements {
			if !bytes.Equal(stored, encoded) {
				t.Fatal("the stored bytes are not the canonical encoding")
			}
		}
	})

	t.Run("the inputs name the spec, its requirements, the policy, the templates, the changed files and the Statements read [verifies SP-526~1]", func(t *testing.T) {
		names := func(role format.InputRole) []string {
			var found []string
			for _, input := range inputsOf(statement, role) {
				found = append(found, input.Name)
			}
			return found
		}
		if got := names(format.RoleSpec); !slices.Equal(got, []string{"INV-1~1", "INV-2~1", "specs/invoicing.md"}) {
			t.Errorf("spec inputs %v", got)
		}
		if got := names(format.RolePromptTemplate); !slices.Equal(got, []string{"decide.dimensions", "decide.hunk", "decide.testability"}) {
			t.Errorf("template inputs %v", got)
		}
		if got := names(format.RoleCodeTree); !slices.Equal(got, []string{"src/invoicing/receipts.py", "src/invoicing/totals.py"}) {
			t.Errorf("code inputs %v", got)
		}
		for _, input := range inputsOf(statement, format.RoleCodeTree) {
			if input.Digest.SHA256 != testkit.FileDigest(t, w.root, input.Name) {
				t.Errorf("the digest of %s is not the digest of its bytes", input.Name)
			}
		}
		if got := names(format.RoleDecisionPolicy); !slices.Equal(got, []string{"decision-policy"}) {
			t.Errorf("policy inputs %v", got)
		}
		if got := names(format.RoleProof); len(got) != 0 {
			t.Errorf("proof inputs %v", got)
		}
		if got := names(format.RoleMutantSet); !slices.Equal(got, []string{"consumed-proofs"}) {
			t.Errorf("consumed proof inputs %v", got)
		}
		if got := names(format.RoleModelID); !slices.Equal(got, []string{"host-models"}) {
			t.Errorf("model inputs %v", got)
		}
		if got := names(format.RoleHostResponse); !slices.Equal(got, []string{"diff-hunks", "host-responses"}) {
			t.Errorf("host inputs %v", got)
		}
	})

	t.Run("a spec is never given an input of a file the change did not touch [verifies SP-526~1]", func(t *testing.T) {
		for _, input := range inputsOf(statement, format.RoleCodeTree) {
			if strings.HasPrefix(input.Name, "tests/") {
				t.Errorf("a test file %s is a code input", input.Name)
			}
		}
	})

	t.Run("the cache key changes when the spec, a changed file or a Statement read changes [verifies SP-526~1]", func(t *testing.T) {
		base := statement.Predicate.CacheKey
		keys := map[string]string{}
		edits := map[string]func(*world){
			"spec": func(other *world) {
				other.writeSpec("specs/invoicing.md", "# Invoicing\n\nanother text\n")
			},
			"code": func(other *world) {
				for index := range other.changes.set.Changes {
					if change := &other.changes.set.Changes[index]; change.Path == "src/invoicing/receipts.py" {
						change.After = append(slices.Clone(change.After), []byte("\n\ndef footer():\n    return ''\n")...)
					}
				}
			},
		}
		for name, edit := range edits {
			other := newWorld(t, testkit.PythonLedger)
			other.withEvidence(power, aligned)
			edit(other)
			other.mustRun(prove.Options{})
			keys[name] = other.statement().Predicate.CacheKey
			if keys[name] == base {
				t.Errorf("a change of the %s kept the cache key", name)
			}
		}
		without := newWorld(t, testkit.PythonLedger)
		without.mustRun(prove.Options{})
		if without.statement().Predicate.CacheKey == base {
			t.Error("the Statements read do not reach the cache key")
		}
	})

	t.Run("the questions are strictly sorted and the same from one run to the next [verifies SP-518~1]", func(t *testing.T) {
		other := newWorld(t, testkit.PythonLedger)
		other.withEvidence(power, aligned)
		other.mustRun(prove.Options{})
		if got, want := other.statement().Predicate.Questions, statement.Predicate.Questions; len(got) != len(want) {
			t.Fatalf("%d questions, want %d", len(got), len(want))
		}
		for index, question := range statement.Predicate.Questions {
			if index > 0 {
				previous := statement.Predicate.Questions[index-1]
				if previous.Dimension+previous.Question >= question.Dimension+question.Question {
					t.Fatalf("questions %d and %d are not strictly sorted", index-1, index)
				}
			}
		}
	})
}

func TestSelection(t *testing.T) {
	t.Run("prove --changed skips a spec whose inputs are unchanged and prove --all proves it again to the same bytes [verifies SP-530~1]", func(t *testing.T) {
		w := newWorld(t, testkit.GoLedger)
		first := w.mustRun(prove.Options{Selection: prove.SelectChanged})
		stored := cloneStatements(w)
		again := w.mustRun(prove.Options{Selection: prove.SelectChanged})
		if again.Specs[0].Status != prove.StatusFresh || again.Specs[0].CacheKey != first.Specs[0].CacheKey || again.Specs[0].Questions != first.Specs[0].Questions {
			t.Fatalf("second run %+v, first %+v", again.Specs[0], first.Specs[0])
		}
		if w.replay.served != 0 {
			t.Fatalf("a fresh spec asked the host again, %d tickets served", w.replay.served)
		}
		all := w.mustRun(prove.Options{Selection: prove.SelectAll})
		if all.Specs[0].Status != prove.StatusProved || all.Specs[0].CacheKey != first.Specs[0].CacheKey {
			t.Fatalf("--all %+v", all.Specs[0])
		}
		if len(w.proofs.statements) != len(stored) {
			t.Fatalf("%d Statements after --all, %d before", len(w.proofs.statements), len(stored))
		}
		for key, content := range stored {
			if !bytes.Equal(w.proofs.statements[key], content) {
				t.Fatalf("--all changed the Statement %q", key)
			}
		}
	})

	t.Run("a changed spec file is proved again by --changed [verifies SP-530~1]", func(t *testing.T) {
		w := newWorld(t, testkit.GoLedger)
		first := w.mustRun(prove.Options{Selection: prove.SelectChanged})
		w.writeSpec("specs/invoicing.md", "# Invoicing\n\nreworded\n")
		second := w.mustRun(prove.Options{Selection: prove.SelectChanged})
		if second.Specs[0].Status != prove.StatusProved || second.Specs[0].CacheKey == first.Specs[0].CacheKey {
			t.Fatalf("second run %+v", second.Specs[0])
		}
	})
}

func TestResume(t *testing.T) {
	t.Run("a resume continues with the base of the run it continues, and a run that is not a resume starts from its own [verifies SP-543~1]", func(t *testing.T) {
		w := newWorld(t, testkit.GoLedger)
		var bases []string
		w.changes.bases = &bases
		w.answers = nil
		w.mustRun(prove.Options{Base: "release-1"})
		w.mustRun(prove.Options{Resume: true})
		w.mustRun(prove.Options{Resume: true, Base: "release-2"})
		w.mustRun(prove.Options{Resume: true})
		w.mustRun(prove.Options{})
		w.mustRun(prove.Options{Resume: true})
		want := []string{"release-1", "release-1", "release-2", "release-1", "", ""}
		if !slices.Equal(bases, want) {
			t.Fatalf("bases %v, want %v", bases, want)
		}
	})
}

func cloneStatements(w *world) map[string][]byte {
	cloned := map[string][]byte{}
	for key, content := range w.proofs.statements {
		cloned[key] = slices.Clone(content)
	}
	return cloned
}

func TestPendingAndExitCodes(t *testing.T) {
	t.Run("while the host has not answered the run exits 3, writes no Statement and lists the criteria as awaiting_host [verifies SP-543~1]", func(t *testing.T) {
		w := newWorld(t, testkit.GoLedger)
		w.answers = nil
		result := w.mustRun(prove.Options{})
		if result.ExitCode() != prove.ExitAwaitingHost || result.PendingTickets == 0 {
			t.Fatalf("result %+v", result)
		}
		if result.Specs[0].Status != prove.StatusAwaitingHost || len(w.proofs.statements) != 0 {
			t.Fatalf("spec %+v with %d Statements", result.Specs[0], len(w.proofs.statements))
		}
		want := []format.PendingEntry{{Criterion: "INV-1~1", Reason: format.PendingAwaitingHost}, {Criterion: "INV-2~1", Reason: format.PendingAwaitingHost}}
		if got := w.pending.current; got.Module != "completeness" || !slices.Equal(got.Pending, want) {
			t.Fatalf("pending file %+v", got)
		}
	})

	t.Run("a run that finished leaves the pending file empty and exits 0 whatever questions it raised [verifies SP-543~1]", func(t *testing.T) {
		w := newWorld(t, testkit.GoLedger)
		result := w.mustRun(prove.Options{})
		if result.ExitCode() != prove.ExitClean || len(w.pending.current.Pending) != 0 {
			t.Fatalf("exit %d pending %+v", result.ExitCode(), w.pending.current)
		}
		if len(w.statement().Predicate.Questions) == 0 {
			t.Fatal("the run raised no question, so the exit code proves nothing")
		}
	})
}
