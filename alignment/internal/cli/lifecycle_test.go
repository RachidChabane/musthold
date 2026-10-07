package cli_test

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/alignment/internal/cli"
	"github.com/RachidChabane/spec-proof/alignment/internal/testkit"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

func TestGoProject(t *testing.T) {
	h := newHarness(t, testkit.GoParcel)
	h.withPowers(testkit.GoPowers())
	_, output := h.driveAttested(testkit.Oracle{})

	t.Run("the tests of a Go project are masked and judged like a Python one [verifies SP-339~1]", func(t *testing.T) {
		want := map[string]string{
			"PAR-1~1 " + testkit.GoMultipliesLabel: "ALIGNED",
			"PAR-3~1 " + testkit.GoRejectsLabel:    "ALIGNED",
			"PAR-2~1 " + testkit.GoSurchargeLabel:  "ALIGNED",
			"PAR-2~1 " + testkit.GoPositiveLabel:   "MISALIGNED",
		}
		got := verdictsOf(h, output)
		for key, verdict := range want {
			if got[key] != verdict {
				t.Errorf("%s: %q, want %q\n%s", key, got[key], verdict, output)
			}
		}
	})

	t.Run("no request shows a criterion reference, the citation marker or the test name [verifies SP-318~1]", func(t *testing.T) {
		store := h.store()
		entries, err := store.ListEntries()
		if err != nil || len(entries) == 0 {
			t.Fatalf("entries %d: %v", len(entries), err)
		}
		for _, entry := range entries {
			request, err := store.GetRequest(entry.Ticket.RequestID)
			if err != nil {
				t.Fatal(err)
			}
			text := request.Prompt.User + string(request.Closed) + string(request.ResponseSchema)
			for _, forbidden := range []string{"PAR-1", "PAR-2", "PAR-3", "verifies", "multiplies_the_unit_price", "TestLineTotal", "rejects_a_zero"} {
				if strings.Contains(text, forbidden) {
					t.Errorf("a request of %s shows %q", entry.Role, forbidden)
				}
			}
		}
	})
}

func TestTournament(t *testing.T) {
	h := newHarness(t, testkit.PythonParcel)
	h.reportWith(func(requirements []any) []any {
		for index := range 27 {
			requirements = append(requirements, map[string]any{
				"id": fmt.Sprintf("FIL-%d", index+1), "revision": 1, "ref": fmt.Sprintf("FIL-%d~1", index+1), "status": "active",
				"statement":        fmt.Sprintf("WHEN filler %d occurs THE SYSTEM SHALL record the occurrence %d.", index+1, index+1),
				"statement_digest": strings.Repeat(fmt.Sprintf("%x", index%16), 64), "tests": []any{},
			})
		}
		return requirements
	})
	h.withPowers(testkit.PythonPowers())
	_, output := h.driveAttested(testkit.Oracle{})

	t.Run("more than 23 criteria are played as groups and a final round, and System One still aligns the test [verifies SP-309~1]", func(t *testing.T) {
		got := verdictsOf(h, output)
		for key, want := range parcelVerdicts {
			if got[key] != want {
				t.Errorf("%s: %q, want %q", key, got[key], want)
			}
		}
		decision := h.statements()["PAR-3~1 "+testkit.PyZeroRejectedLabel].Predicate.Extensions.Decision
		if decision == nil || decision.Basis != format.BasisSystemOne {
			t.Fatalf("decision %+v", decision)
		}
		closed := h.requestKinds()[hostproto.KindDecideClosed]
		const eligibleTests, roundsPerTest = 4, 3
		if closed != eligibleTests*roundsPerTest {
			t.Fatalf("%d closed requests, want %d (two groups and a final round per eligible test)", closed, eligibleTests*roundsPerTest)
		}
	})
}

func TestRecordAndReplay(t *testing.T) {
	recordings := t.TempDir()
	recorder := newHarness(t, testkit.PythonParcel)
	recorder.withPowers(testkit.PythonPowers())
	_, output := recorder.driveAttested(testkit.Oracle{}, "--record", recordings)
	want := recorder.statementBytes()

	t.Run("replaying the recorded answers gives byte-identical Statements, in one command [verifies SP-339~1]", func(t *testing.T) {
		replayer := newHarness(t, testkit.PythonParcel)
		replayer.withPowers(testkit.PythonPowers())
		code, replayed, stderr := replayer.run(nil, proveCommand, allFlag, "--replay", recordings)
		if code != cli.ExitBlocked {
			t.Fatalf("exit code %d: %s\n%s", code, stderr, replayed)
		}
		got := replayer.statementBytes()
		if len(got) != len(want) || len(want) == 0 {
			t.Fatalf("%d Statements, want %d", len(got), len(want))
		}
		for path, content := range want {
			if got[path] != content {
				t.Errorf("%s differs after the replay", path)
			}
		}
		if listing := replayer.requestsJSON(); len(listing.Items) != 0 {
			t.Errorf("%d tickets are still pending after the replay", len(listing.Items))
		}
		if !strings.Contains(output, "PAR-3~1") || verdictsOf(replayer, replayed)["PAR-3~1 "+testkit.PyZeroRejectedLabel] != "ALIGNED" {
			t.Errorf("replayed output\n%s", replayed)
		}
	})

	t.Run("a replay with no recording of a request is a hard error [verifies SP-334~1]", func(t *testing.T) {
		replayer := newHarness(t, testkit.PythonParcel)
		replayer.withPowers(testkit.PythonPowers())
		code, _, stderr := replayer.run(nil, proveCommand, allFlag, "--replay", t.TempDir())
		if code != cli.ExitToolFailure || !strings.Contains(stderr, "no recorded answer") {
			t.Fatalf("exit code %d: %s", code, stderr)
		}
	})
}

func TestFreshness(t *testing.T) {
	h := newHarness(t, testkit.PythonParcel)
	h.withPowers(testkit.PythonPowers())
	h.driveAttested(testkit.Oracle{})
	before := h.statementBytes()

	t.Run("prove --changed skips a test whose inputs are unchanged and asks the host for nothing [verifies SP-331~1]", func(t *testing.T) {
		code, output, stderr := h.run(nil, proveCommand, changedFlag)
		if code != cli.ExitBlocked {
			t.Fatalf("exit code %d: %s", code, stderr)
		}
		for key, entry := range results(output) {
			if entry.Status != "fresh" {
				t.Errorf("%s: status %s", key, entry.Status)
			}
		}
		if !maps.Equal(h.statementBytes(), before) {
			t.Error("a fresh run rewrote a Statement")
		}
	})

	t.Run("prove --all proves it again with the same Statement [verifies SP-331~1]", func(t *testing.T) {
		code, output, stderr := h.run(nil, proveCommand, allFlag)
		if code != cli.ExitBlocked {
			t.Fatalf("exit code %d: %s", code, stderr)
		}
		for key, entry := range results(output) {
			if entry.Status != "proved" {
				t.Errorf("%s: status %s", key, entry.Status)
			}
		}
		if !maps.Equal(h.statementBytes(), before) {
			t.Error("proving again changed a Statement")
		}
	})
}

func TestStaleness(t *testing.T) {
	t.Run("a change of the discriminating-power Statement gives another cache key, written beside the old one [verifies SP-329~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		h.withPowers(testkit.PythonPowers())
		h.driveAttested(testkit.Oracle{})
		before := h.statementBytes()
		powers := testkit.PythonPowers()
		powers[0].Mutants = append(powers[0].Mutants, testkit.Mutant{Digest: "m4", Kind: testkit.ValidJudged, Killers: []string{testkit.PyMultiplies}})
		path := powers[0].Write(t, h.root)
		future := time.Now().Add(time.Minute)
		if err := os.Chtimes(path, future, future); err != nil {
			t.Fatal(err)
		}
		h.driveAttested(testkit.Oracle{}, "--changed")
		after := h.statementBytes()
		if len(after) <= len(before) {
			t.Fatalf("%d Statements after the change, %d before", len(after), len(before))
		}
		for path, content := range before {
			if strings.HasPrefix(path, "PAR-2") || strings.HasPrefix(path, "PAR-3") {
				if after[path] != content {
					t.Errorf("%s changed although its proof did not", path)
				}
			}
		}
		fresh := 0
		for key, entry := range results(func() string { _, out, _ := h.run(nil, proveCommand, changedFlag); return out }()) {
			if entry.Status == "fresh" && strings.HasPrefix(key, "PAR-1~1") {
				fresh++
			}
		}
		if fresh != 3 {
			t.Errorf("%d fresh PAR-1 tests after the proof was reproved", fresh)
		}
	})

	t.Run("a Statement whose test or code file changed is not read and the matrix is absent [verifies SP-329~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		h.withPowers(testkit.PythonPowers())
		path := filepath.Join(h.root, "src", "parcel", "pricing.py")
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(content, []byte("\nEXTRA = 1\n")...), fileMode); err != nil {
			t.Fatal(err)
		}
		h.driveAttested(testkit.Oracle{})
		for key, statement := range h.statements() {
			if !strings.Contains(string(statement.Predicate.Extensions.Matrix), `"ABSENT"`) {
				t.Errorf("%s read a stale Statement: %s", key, statement.Predicate.Extensions.Matrix)
			}
		}
	})

	t.Run("a Statement of another statement digest is not read [verifies SP-329~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		powers := testkit.PythonPowers()
		powers[2].StatementDigest = strings.Repeat("ab", 32)
		h.withPowers(powers)
		h.driveAttested(testkit.Oracle{})
		matrix := string(h.statements()["PAR-3~1 "+testkit.PyZeroRejectedLabel].Predicate.Extensions.Matrix)
		if !strings.Contains(matrix, `"ABSENT"`) {
			t.Fatalf("matrix %s", matrix)
		}
	})

	t.Run("a test that the Statement does not list gets no matrix [verifies SP-329~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		powers := testkit.PythonPowers()
		powers[1].Tests = []string{testkit.PyRemote}
		h.withPowers(powers)
		h.driveAttested(testkit.Oracle{})
		matrix := string(h.statements()["PAR-2~1 "+testkit.PyNumberLabel].Predicate.Extensions.Matrix)
		if !strings.Contains(matrix, `"ABSENT"`) {
			t.Fatalf("matrix %s", matrix)
		}
	})
}

func TestMatrixVerdicts(t *testing.T) {
	t.Run("a test that kills a sentinel is UNDETERMINED with mutation_sensitive_test and no request is made for it [verifies SP-310~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		powers := testkit.PythonPowers()
		powers[2].Mutants = append(powers[2].Mutants, testkit.Mutant{Digest: "s2", Kind: testkit.Sentinel, Killers: []string{testkit.PyZeroRejected}})
		h.withPowers(powers)
		_, output := h.driveAttested(testkit.Oracle{})
		entry := results(output)["PAR-3~1 "+testkit.PyZeroRejectedLabel]
		if entry.Verdict != "UNDETERMINED" || entry.Reason != "mutation_sensitive_test" {
			t.Fatalf("%+v", entry)
		}
		statement := h.statements()["PAR-3~1 "+testkit.PyZeroRejectedLabel]
		if len(statement.Predicate.Executor.Attestation.AgentID) != 0 {
			t.Errorf("a contaminated test rests on an answer: %+v", statement.Predicate.Executor)
		}
	})

	t.Run("a test that pins behaviour no criterion states does not get the two-signal verdict [verifies SP-305~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		powers := testkit.PythonPowers()
		powers[2].Mutants = append(powers[2].Mutants, testkit.Mutant{Digest: "u1", Kind: testkit.Unattributed, Killers: []string{testkit.PyZeroRejected}})
		h.withPowers(powers)
		_, output := h.driveAttested(testkit.Oracle{})
		entry := results(output)["PAR-3~1 "+testkit.PyZeroRejectedLabel]
		if entry.Verdict != "UNDETERMINED" || entry.Reason != "judge_matrix_disagreement" {
			t.Fatalf("%+v", entry)
		}
	})

	t.Run("a collateral VALID mutant stays out of the matrix [verifies SP-303~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		powers := testkit.PythonPowers()
		powers[2].Mutants = []testkit.Mutant{{Digest: "c1", Kind: testkit.ValidCollateral, Killers: []string{testkit.PyZeroRejected}}}
		h.withPowers(powers)
		_, output := h.driveAttested(testkit.Oracle{})
		matrix := string(h.statements()["PAR-3~1 "+testkit.PyZeroRejectedLabel].Predicate.Extensions.Matrix)
		if !strings.Contains(matrix, `"UNDEFINED_OWN"`) || results(output)["PAR-3~1 "+testkit.PyZeroRejectedLabel].Verdict != "ALIGNED" {
			t.Fatalf("matrix %s\n%s", matrix, output)
		}
	})
}

func TestStatementShape(t *testing.T) {
	h := newHarness(t, testkit.PythonParcel)
	h.withPowers(testkit.PythonPowers())
	h.driveAttested(testkit.Oracle{})
	statements := h.statements()

	t.Run("one Statement is written per test and declared criterion, at its cache key [verifies SP-330~1]", func(t *testing.T) {
		if len(statements) != len(parcelVerdicts) {
			t.Fatalf("%d Statements", len(statements))
		}
		paths := h.statementBytes()
		for path, content := range paths {
			decoded, err := format.DecodeAs[format.Alignment](([]byte)(content))
			if err != nil {
				t.Fatal(err)
			}
			wantPath := filepath.Join(decoded.Subject[0].Name, decoded.Predicate.CacheKey+".json")
			if path != wantPath {
				t.Errorf("%s is not at %s", path, wantPath)
			}
			canonical, err := format.CanonicalizeJSON([]byte(content))
			if err != nil || string(canonical) != content {
				t.Errorf("%s is not in canonical form", path)
			}
		}
	})

	t.Run("the executor, the decision and the inputs are those of the answers it rests on [verifies SP-330~1]", func(t *testing.T) {
		for key, statement := range statements {
			predicate := statement.Predicate
			if !predicate.Executor.Blind || predicate.Executor.Isolation != format.IsolationRestricted || predicate.Executor.Channel != format.ChannelHostSubagent {
				t.Errorf("%s: executor %+v", key, predicate.Executor)
			}
			roles := []format.InputRole{}
			for _, input := range predicate.Inputs {
				roles = append(roles, input.Role)
			}
			for _, role := range []format.InputRole{format.RoleSpec, format.RoleTestSource, format.RoleCodeTree, format.RoleDecisionPolicy, format.RolePromptTemplate, format.RoleHostResponse, format.RoleModelID, format.RoleProof} {
				if !slices.Contains(roles, role) {
					t.Errorf("%s: no input of role %s", key, role)
				}
			}
			if predicate.Extensions.Decision.Threshold == 0 || len(predicate.Extensions.Decision.Deciders) == 0 {
				t.Errorf("%s: decision %+v", key, predicate.Extensions.Decision)
			}
		}
	})

	t.Run("a test bound to several criteria gets one Statement per criterion from one masked input [verifies SP-330~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		h.reportWith(func(requirements []any) []any {
			return bind(requirements, "PAR-3", pythonTest("test_line_total_multiplies"))
		})
		h.withPowers(testkit.PythonPowers())
		h.driveAttested(testkit.Oracle{})
		found := h.statements()
		first, second := found["PAR-1~1 "+testkit.PyMultipliesLabel], found["PAR-3~1 "+testkit.PyMultipliesLabel]
		if first.Predicate.Verdict.Value != format.VerdictAligned || second.Predicate.Verdict.Value != format.VerdictMisaligned {
			t.Fatalf("verdicts %s and %s", first.Predicate.Verdict.Value, second.Predicate.Verdict.Value)
		}
		if first.Predicate.CacheKey == second.Predicate.CacheKey {
			t.Error("two criteria share a cache key")
		}
	})
}
