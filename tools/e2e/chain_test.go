package e2e_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/formattest/parcel"
)

const (
	verifiesHonest    = " [verifies SP-412~5]"
	verifiesCareless  = " [verifies SP-413~1]"
	verifiesStaleness = " [verifies SP-414~1]"
	verifiesForger    = " [verifies SP-415~1]"

	manifestFileName   = "manifest.json"
	dpDirectory        = "discriminating-power"
	alignmentDirectory = "alignment"
	boundTestFile      = "import pytest\n\nfrom parcel.rates import shipping_fee\n\n\n@pytest.mark.verifies(\"PAR-3~1\")\ndef test_exact_threshold_is_free():\n    assert shipping_fee(50) == 0\n"
)

// parcelTable is what the gate says of the parcel project when an honest host made
// both proofs: the line total is adequately tested and aligned; the discount's
// rounding survivors are real changes that no witness fails and no execution
// excludes, so the discount escalates; and the shipping tests leave a surviving
// mutant that violates the criterion.
var parcelTable = map[string]string{"PAR-1": verdictPass, "PAR-2": verdictEscalate, "PAR-3": verdictBlock}

// manifestEntry is one statement of the verified set as the manifest lists it.
type manifestEntry struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

func (p *parcelProject) manifest() []manifestEntry {
	p.t.Helper()
	var manifest struct {
		Statements []manifestEntry `json:"statements"`
	}
	if err := json.Unmarshal([]byte(p.read(verifiedPath+"/"+manifestFileName)), &manifest); err != nil {
		p.t.Fatal(err)
	}
	return manifest.Statements
}

// provedWithHonestDiscriminatingPower builds a project that both modules are
// installed in, whose code was edited and whose discriminating-power proof an
// honest host made through the real executor server and the verifier accepted.
func provedWithHonestDiscriminatingPower(t *testing.T, requirements []string) *parcelProject {
	t.Helper()
	p := newParcelProject(t, requirements)
	p.equip()
	p.edit(rangeCodeFile)
	last := p.proveDiscriminatingPower(claudeHostOf(p), parcel.Oracle{})
	if last.code == exitAwaiting || last.code == exitToolFailed {
		t.Fatalf("the discriminating-power proof ended with exit %d:\n%s", last.code, last.output())
	}
	p.verifyLocal()
	return p
}

func TestHonestChain(t *testing.T) {
	if testing.Short() {
		t.Skip("builds three binaries and runs the parcel project's tests against many mutants")
	}
	p := provedWithHonestDiscriminatingPower(t, []string{"SP-412~5", "SP-413~1", "SP-414~1", "SP-415~1"})
	careless, forgedAlignment := p.copyAs("careless"), p.copyAs("forged alignment")

	aligned := p.proveAlignment(parcel.HonestAlignment())
	p.verifyLocal()
	report := p.check()

	t.Run("an honest host gets the verdict each criterion earned and none is stale"+verifiesHonest, func(t *testing.T) {
		got := report.table()
		t.Logf("honest hosts: %v", got)
		if !equalTables(got, parcelTable) {
			t.Fatalf("verdicts %v, want %v\n%s", got, parcelTable, report.output)
		}
		for _, f := range report.findings {
			if f.Category == "stale_proof" || f.Category == "malformed_proof" {
				t.Errorf("finding %+v", f)
			}
		}
		if report.code != exitBlocked {
			t.Errorf("shallnot check exited %d", report.code)
		}
	})

	t.Run("the alignment host aligned the tests it could judge blind and the gate consumed every statement as valid"+verifiesHonest, func(t *testing.T) {
		verdicts := alignmentVerdicts(aligned.stdout)
		if verdicts["PAR-1~1 tests.test_rates.test_line_total_multiplies_price_by_quantity"] != "ALIGNED" {
			t.Fatalf("alignment verdicts %v\n%s", verdicts, aligned.output())
		}
		for _, proof := range report.consumed {
			if proof.State != "valid" {
				t.Errorf("%s is %s: %s", proof.Path, proof.State, proof.Reason)
			}
		}
		for _, criterion := range []string{"PAR-1~1", "PAR-2~1", "PAR-3~1"} {
			for _, kind := range []string{dpDirectory, alignmentDirectory} {
				if state := report.proofState(kind, criterion); state != "valid" {
					t.Errorf("the %s proof of %s is %s", kind, criterion, state)
				}
			}
		}
	})

	t.Run("an alignment statement records the digest of the exact bytes of the statement it consumed, which the manifest lists"+verifiesHonest, func(t *testing.T) {
		listed := map[string]string{}
		for _, entry := range p.manifest() {
			listed[entry.Path] = entry.Digest
		}
		checked := 0
		for _, entry := range p.manifest() {
			if !strings.HasPrefix(entry.Path, alignmentDirectory+"/") {
				continue
			}
			statement, err := format.DecodeAs[format.Alignment](p.readBytes(verifiedPath + "/" + entry.Path))
			if err != nil {
				t.Fatal(err)
			}
			criterion := format.SubjectName(statement.Predicate.Requirement.ID, statement.Predicate.Requirement.Revision)
			var consumed []format.Input
			for _, input := range statement.Predicate.Inputs {
				if input.Role == format.RoleProof {
					consumed = append(consumed, input)
				}
			}
			if len(consumed) != 1 || consumed[0].Name != string(format.KindDiscriminatingPower) || consumed[0].Annotations.Criterion != criterion {
				t.Errorf("%s: proof inputs %+v", entry.Path, consumed)
				continue
			}
			if !manifestHolds(listed, dpDirectory+"/"+criterion+"/", consumed[0].Digest.SHA256) {
				t.Errorf("%s: the digest %s of the consumed statement is not one the manifest lists", entry.Path, consumed[0].Digest.SHA256)
			}
			checked++
		}
		if checked == 0 {
			t.Fatal("no alignment statement was checked")
		}
	})

	t.Run("a file outside every footprint changes no verdict"+verifiesStaleness, func(t *testing.T) {
		original := p.read(outsideFile)
		p.edit(outsideFile)
		defer p.write(outsideFile, original)
		got := p.check()
		table := got.table()
		t.Logf("file outside every footprint edited: %v", table)
		if !equalTables(table, parcelTable) {
			t.Fatalf("verdicts %v, want %v\n%s", table, parcelTable, got.output)
		}
	})

	t.Run("a footprint file that changed makes the proofs that depend on it stale"+verifiesStaleness, func(t *testing.T) {
		original := p.read(rangeCodeFile)
		p.edit(rangeCodeFile)
		defer p.write(rangeCodeFile, original)
		got := p.check()
		t.Logf("footprint file edited: %v", got.table())
		for _, criterion := range []string{"PAR-1", "PAR-2", "PAR-3"} {
			if got.verdict(criterion) != verdictStale {
				t.Errorf("%s reads as %s\n%s", criterion, got.verdict(criterion), got.output)
			}
		}
	})

	t.Run("a bound test file that is added makes the proof of that criterion stale and the alignment proofs that consume it, and no other"+verifiesStaleness, func(t *testing.T) {
		p.write(extraTestFile, boundTestFile)
		defer os.Remove(filepath.Join(p.root, extraTestFile))
		got := p.check()
		t.Logf("bound test file added: %v", got.table())
		if state := got.proofState(dpDirectory, "PAR-3~1"); state != "stale" {
			t.Errorf("the discriminating-power proof of PAR-3~1 is %s\n%s", state, got.output)
		}
		if state := got.proofState(alignmentDirectory, "PAR-3~1"); state != "stale" {
			t.Errorf("the alignment proofs of PAR-3~1 are %s", state)
		}
		for _, criterion := range []string{"PAR-1~1", "PAR-2~1"} {
			for _, kind := range []string{dpDirectory, alignmentDirectory} {
				if state := got.proofState(kind, criterion); state != "valid" {
					t.Errorf("the %s proof of %s is %s", kind, criterion, state)
				}
			}
		}
		if got.verdict("PAR-1") != verdictPass || got.verdict("PAR-3") != verdictStale {
			t.Errorf("verdicts %v", got.table())
		}
	})

	t.Run("an alignment host that finds every assertion too weak escalates the criteria it judged"+verifiesCareless, func(t *testing.T) {
		careless.proveAlignment(parcel.CarelessAlignment())
		careless.verifyLocal()
		got := careless.check()
		want := map[string]string{"PAR-1": verdictEscalate, "PAR-2": verdictEscalate, "PAR-3": verdictBlock}
		t.Logf("careless alignment host: %v", got.table())
		if table := got.table(); !equalTables(table, want) {
			t.Fatalf("verdicts %v, want %v\n%s", table, want, got.output)
		}
	})

	t.Run("answers of a forger given to the alignment module outside any executor never pass"+verifiesForger, func(t *testing.T) {
		last := forgedAlignment.alignment("", "prove", "--all")
		for wave := 0; last.code == exitAwaiting && wave < maxWaves; wave++ {
			playForger(forgedAlignment, forgedAlignment.alignment)
			last = forgedAlignment.alignment("", "prove", "--all", "--resume")
		}
		forgedAlignment.verifyLocal()
		got := forgedAlignment.check()
		t.Logf("forged alignment answers: %v", got.table())
		assertNoPass(t, got)
	})
}

func TestForgedChain(t *testing.T) {
	if testing.Short() {
		t.Skip("builds three binaries and runs the parcel project's tests against many mutants")
	}
	p := newParcelProject(t, []string{"SP-415~1"})
	p.equip()
	p.edit(rangeCodeFile)

	t.Run("a forger that answers every ticket of both modules from the command line never gets a pass"+verifiesForger, func(t *testing.T) {
		last := p.dp("", "prove", "--changed")
		for wave := 0; last.code == exitAwaiting && wave < maxWaves; wave++ {
			playForger(p, p.dp)
			last = p.dp("", "prove", "--changed", "--resume")
		}
		if last.code == exitClean {
			t.Fatalf("the discriminating-power module passed under the forger:\n%s", last.output())
		}
		p.verifyLocal()
		alignment := p.alignment("", "prove", "--all")
		for wave := 0; alignment.code == exitAwaiting && wave < maxWaves; wave++ {
			playForger(p, p.alignment)
			alignment = p.alignment("", "prove", "--all", "--resume")
		}
		p.verifyLocal()
		got := p.check()
		t.Logf("forged answers of both modules: %v", got.table())
		assertNoPass(t, got)
	})
}

func assertNoPass(t *testing.T, report gateReport) {
	t.Helper()
	if report.code == exitClean {
		t.Fatalf("shallnot check passed forged proofs:\n%s", report.output)
	}
	for criterion, verdict := range report.table() {
		if verdict == verdictPass {
			t.Errorf("%s passed", criterion)
		}
	}
}

func equalTables(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}

// manifestHolds reports whether the manifest lists a statement below the prefix with
// the digest.
func manifestHolds(listed map[string]string, prefix, digest string) bool {
	for path, listedDigest := range listed {
		if strings.HasPrefix(path, prefix) && listedDigest == digest {
			return true
		}
	}
	return false
}
