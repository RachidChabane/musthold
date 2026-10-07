package prove_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/completeness/internal/app/prove"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/closedq"
	"github.com/RachidChabane/spec-proof/completeness/internal/testkit"
	"github.com/RachidChabane/spec-proof/format"
)

// ledger describes a fixture project for the table of tests below: what the
// honest host finds unjustified with and without the evidence of the other
// modules, and the changed units the discriminating-power Statement is asked
// about.
type ledger struct {
	fixture string
	// withoutEvidence and withEvidence are the units asked about in a hunk request.
	hunksWithoutEvidence int
	hunksWithEvidence    int
	unjustified          []string
	evidence             func() (testkit.Power, testkit.Aligned)
	changedFile          string
	changedFunctions     string
}

var ledgers = []ledger{
	{
		fixture: testkit.PythonLedger, hunksWithoutEvidence: 5, hunksWithEvidence: 2,
		unjustified: []string{"apply_discount", "receipt_line"}, evidence: testkit.PythonEvidence,
		changedFile: "src/invoicing/totals.py", changedFunctions: "TAX_RATE, apply_discount, invoice_total, line_amount",
	},
	{
		fixture: testkit.GoLedger, hunksWithoutEvidence: 4, hunksWithEvidence: 1,
		unjustified: []string{"ApplyDiscount"}, evidence: testkit.GoEvidence,
		changedFile: "totals.go", changedFunctions: "ApplyDiscount, InvoiceTotal, LineAmount, taxRate",
	},
	{
		fixture: testkit.TypeScriptLedger, hunksWithoutEvidence: 5, hunksWithEvidence: 2,
		unjustified: []string{"applyDiscount", "receiptLine"}, evidence: testkit.TypeScriptEvidence,
		changedFile: "src/totals.ts", changedFunctions: "TAX_RATE, applyDiscount, invoiceTotal, lineAmount",
	},
	{
		fixture: testkit.JavaLedger, hunksWithoutEvidence: 5, hunksWithEvidence: 2,
		unjustified: []string{"Totals.applyDiscount", "Receipts.receiptLine"}, evidence: testkit.JavaEvidence,
		changedFile: "src/main/java/ledger/Totals.java", changedFunctions: "Totals.TAX_RATE, Totals.applyDiscount, Totals.invoiceTotal, Totals.lineAmount",
	},
}

func TestWithoutEvidence(t *testing.T) {
	for _, l := range ledgers {
		w := newWorld(t, l.fixture)
		result := w.mustRun(prove.Options{})
		statement := w.statement()
		lines := questionLines(statement)

		t.Run(l.fixture+": every changed unit is asked about when no example probe and no ALIGNED test is known, and the host's NONE raises the question [verifies SP-510~1]", func(t *testing.T) {
			if got := w.requestKinds()["hunk"]; got != l.hunksWithoutEvidence {
				t.Fatalf("%d hunk requests, want %d", got, l.hunksWithoutEvidence)
			}
			unjustified := questionsOf(statement, "unjustified_change")
			if len(unjustified) != len(l.unjustified) {
				t.Fatalf("unjustified changes %+v, want %v", unjustified, l.unjustified)
			}
			for _, name := range l.unjustified {
				if !anyContains(lines, "unjustified_change: The change to "+name+" in ") {
					t.Errorf("no question about %s:\n%s", name, strings.Join(lines, "\n"))
				}
			}
			if len(unjustified[0].RequirementIDs) != 0 {
				t.Errorf("an unjustified change names requirements %v", unjustified[0].RequirementIDs)
			}
		})

		t.Run(l.fixture+": a dimension MISSING raises a question, one the owner declared out of scope is never asked [verifies SP-503~1]", func(t *testing.T) {
			for _, dimension := range []string{"error_handling", "boundaries"} {
				if got := questionsOf(statement, dimension); len(got) != 1 || strings.HasPrefix(got[0].Question, "(low confidence)") {
					t.Errorf("%s: %+v", dimension, got)
				}
			}
			if got := questionsOf(statement, "concurrency"); len(got) != 0 {
				t.Errorf("a COVERED dimension raised %+v", got)
			}
		})

		t.Run(l.fixture+": an AMBIGUOUS requirement raises a clarity question that names it and no other [verifies SP-512~1]", func(t *testing.T) {
			clarity := questionsOf(statement, "clarity")
			if len(clarity) != 1 || !slices.Equal(clarity[0].RequirementIDs, []string{"INV-2"}) {
				t.Fatalf("clarity questions %+v", clarity)
			}
			if got := w.requestKinds()["testability"]; got != 1 {
				t.Fatalf("%d testability requests", got)
			}
		})

		t.Run(l.fixture+": without evidence of the other modules no behaviour is called unconstrained [verifies SP-511~1]", func(t *testing.T) {
			if got := questionsOf(statement, "unconstrained_behaviour"); len(got) != 0 {
				t.Fatalf("questions %+v", got)
			}
			for _, input := range statement.Predicate.Inputs {
				if input.Role == format.RoleProof {
					t.Fatalf("a proof input %q without a proof", input.Name)
				}
			}
		})

		t.Run(l.fixture+": the run asks the host once per wave and writes the Statement when everything is answered [verifies SP-543~1]", func(t *testing.T) {
			if result.PendingTickets != 0 || result.ExitCode() != prove.ExitClean {
				t.Fatalf("result %+v", result)
			}
			if len(result.Specs) != 1 || result.Specs[0].Status != prove.StatusProved || result.Specs[0].Questions != len(statement.Predicate.Questions) {
				t.Fatalf("specs %+v", result.Specs)
			}
		})
	}
}

func TestWithEvidence(t *testing.T) {
	for _, l := range ledgers {
		w := newWorld(t, l.fixture)
		power, aligned := l.evidence()
		w.withEvidence(power, aligned)
		w.mustRun(prove.Options{})
		statement := w.statement()

		t.Run(l.fixture+": the units an example probe or an ALIGNED test may execute are justified and no longer asked about [verifies SP-509~1]", func(t *testing.T) {
			if got := w.requestKinds()["hunk"]; got != l.hunksWithEvidence {
				t.Fatalf("%d hunk requests, want %d", got, l.hunksWithEvidence)
			}
			if got := questionsOf(statement, "unjustified_change"); len(got) != len(l.unjustified) {
				t.Fatalf("unjustified changes %+v, want %v", got, l.unjustified)
			}
		})

		t.Run(l.fixture+": mutants no requirement owns raise one question per requirement and changed file, with the changed functions and no witness [verifies SP-511~1]", func(t *testing.T) {
			got := questionsOf(statement, "unconstrained_behaviour")
			if len(got) != 1 {
				t.Fatalf("questions %+v", got)
			}
			if !slices.Equal(got[0].RequirementIDs, []string{"INV-1"}) {
				t.Errorf("requirements %v", got[0].RequirementIDs)
			}
			for _, want := range []string{l.changedFile, l.changedFunctions, "2 ways"} {
				if !strings.Contains(got[0].Question, want) {
					t.Errorf("the question lacks %q: %s", want, got[0].Question)
				}
			}
		})

		t.Run(l.fixture+": the Statements read are one consumed-proofs input, and a Statement of another text of the criterion is not read [verifies SP-509~1]", func(t *testing.T) {
			var consumed []format.Input
			for _, input := range statement.Predicate.Inputs {
				if input.Name == "consumed-proofs" {
					consumed = append(consumed, input)
				}
				if input.Role == format.RoleProof {
					t.Fatalf("a spec of many criteria reads many proofs of a kind, which no proof input can name: %+v", input)
				}
			}
			if len(consumed) != 1 || consumed[0].Role != format.RoleMutantSet || !format.IsSHA256Hex(consumed[0].Digest.SHA256) {
				t.Fatalf("consumed-proofs inputs %+v", consumed)
			}
			stale := newWorld(t, l.fixture)
			stalePower, staleAligned := l.evidence()
			stalePower.StatementDigest = testkit.DisplayDigest
			staleAligned.Digest = testkit.DisplayDigest
			stale.withEvidence(stalePower, staleAligned)
			stale.mustRun(prove.Options{})
			if got := stale.requestKinds()["hunk"]; got != l.hunksWithoutEvidence {
				t.Fatalf("%d hunk requests with stale evidence, want %d", got, l.hunksWithoutEvidence)
			}
			if got := questionsOf(stale.statement(), "unconstrained_behaviour"); len(got) != 0 {
				t.Fatalf("stale evidence raised %+v", got)
			}
		})
	}
}

func TestEvidenceGoesStaleWithTheCode(t *testing.T) {
	t.Run("a Statement whose code file changed since it was written is not read, and a MISALIGNED test justifies nothing [verifies SP-509~1]", func(t *testing.T) {
		w := newWorld(t, testkit.PythonLedger)
		power, aligned := testkit.PythonEvidence()
		power.Write(t, w.root)
		aligned.Verdict = format.VerdictMisaligned
		aligned.Write(t, w.root)
		// The probe still justifies invoice_total and what it mentions; the MISALIGNED
		// test adds nothing, so only the two units no probe reaches stay unjustified.
		w.mustRun(prove.Options{})
		if got := w.requestKinds()["hunk"]; got != 2 {
			t.Fatalf("%d hunk requests", got)
		}
		stale := newWorld(t, testkit.PythonLedger)
		stalePower, staleAligned := testkit.PythonEvidence()
		stalePower.Write(t, stale.root)
		staleAligned.Write(t, stale.root)
		appendTo(t, stale.root, "src/invoicing/totals.py", "\n\ndef another(total):\n    return total\n")
		stale.mustRun(prove.Options{})
		if got := stale.requestKinds()["hunk"]; got != 5 {
			t.Fatalf("%d hunk requests after the code moved on, want 5", got)
		}
	})
}

func TestRelay(t *testing.T) {
	t.Run("a relayed answer is used alone, with the confidence the endpoint gave, and the questions below tau_adv are flagged [verifies SP-515~1]", func(t *testing.T) {
		w := newWorld(t, testkit.GoLedger)
		w.answers = relay{honest: testkit.Honest(), confidence: 0.3}
		w.mustRun(prove.Options{})
		statement := w.statement()
		boundaries := questionsOf(statement, "boundaries")
		if len(boundaries) != 1 || !strings.HasPrefix(boundaries[0].Question, "(low confidence) ") {
			t.Fatalf("boundaries %+v", boundaries)
		}
		decision := statement.Predicate.Extensions.Decision
		if decision == nil || decision.Basis != format.BasisSystemOne || decision.ConfidenceSource != format.SystemOneSource("nimble:9b@endpoint") || !slices.Equal(decision.Deciders, []string{"system_one"}) || decision.RawConfidence != 0.3 || decision.Threshold != 0.5 {
			t.Fatalf("decision %+v", decision)
		}
		entries, err := w.store.ListEntries()
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.Ticket.Vote != 0 {
				t.Fatalf("a host vote %d was asked besides the relay", entry.Ticket.Vote)
			}
		}
	})

	t.Run("a relayed answer without a confidence counts as zero and no unjustified change is raised from it [verifies SP-510~1]", func(t *testing.T) {
		w := newWorld(t, testkit.GoLedger)
		w.answers = relay{honest: testkit.Honest(), confidence: -1}
		w.mustRun(prove.Options{})
		if got := questionsOf(w.statement(), "unjustified_change"); len(got) != 0 {
			t.Fatalf("questions %+v", got)
		}
	})
}

func TestHostVotes(t *testing.T) {
	t.Run("host decider votes give the modal choice and its share as confidence, recorded as host-votes(k) [verifies SP-515~1]", func(t *testing.T) {
		w := newWorld(t, testkit.GoLedger)
		w.mustRun(prove.Options{})
		decision := w.statement().Predicate.Extensions.Decision
		if decision == nil || decision.ConfidenceSource != format.HostVotesSource(3) || !slices.Equal(decision.Deciders, []string{"system_one"}) || decision.RawConfidence != 1 {
			t.Fatalf("decision %+v", decision)
		}
	})

	splitVote := func(t *testing.T, tau format.Threshold) (*world, format.Statement[format.Completeness]) {
		t.Helper()
		w := newWorld(t, testkit.GoLedger)
		w.policy.TauAdv = tau
		honest := testkit.Honest()
		w.answers = testkit.Answerer{Choose: func(question closedq.Question, state string, vote int) (string, float64) {
			if question.ID == "boundaries" && vote == 0 {
				return "COVERED", 1
			}
			return honest.Choose(question, state, vote)
		}}
		w.mustRun(prove.Options{})
		return w, w.statement()
	}
	lowConfidence := func(statement format.Statement[format.Completeness], dimension string) bool {
		questions := questionsOf(statement, dimension)
		return len(questions) == 1 && strings.HasPrefix(questions[0].Question, "(low confidence)")
	}

	t.Run("a split vote below tau_adv flags the question as low confidence [verifies SP-503~1]", func(t *testing.T) {
		_, statement := splitVote(t, format.UniformThreshold(0.9))
		if !lowConfidence(statement, "boundaries") || lowConfidence(statement, "error_handling") {
			t.Fatalf("boundaries %+v, error handling %+v", questionsOf(statement, "boundaries"), questionsOf(statement, "error_handling"))
		}
	})

	t.Run("the threshold of host votes flags the split vote although the default would not [verifies SP-904~1]", func(t *testing.T) {
		_, statement := splitVote(t, format.Threshold{Default: 0.1, BySource: map[format.ConfidenceSource]float64{format.SourceHostVotes: 0.9}})
		if !lowConfidence(statement, "boundaries") || lowConfidence(statement, "error_handling") {
			t.Fatalf("boundaries %+v", questionsOf(statement, "boundaries"))
		}
	})

	t.Run("a threshold of System One leaves the host votes on the default [verifies SP-904~1]", func(t *testing.T) {
		_, statement := splitVote(t, format.Threshold{Default: 0.1, BySource: map[format.ConfidenceSource]float64{format.SourceSystemOne: 0.99}})
		if lowConfidence(statement, "boundaries") {
			t.Fatalf("boundaries %+v", questionsOf(statement, "boundaries"))
		}
	})

	t.Run("the decision records the threshold of the source it was held to [verifies SP-904~1]", func(t *testing.T) {
		_, statement := splitVote(t, format.Threshold{Default: 0.1, BySource: map[format.ConfidenceSource]float64{format.SourceHostVotes: 0.9}})
		decision := statement.Predicate.Extensions.Decision
		if decision == nil || decision.Threshold != 0.9 {
			t.Fatalf("decision %+v", decision)
		}
	})
}
