package ledger_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/ledger/internal/domain/ledger"
	"github.com/RachidChabane/spec-proof/ledger/internal/domain/refusal"
	"github.com/RachidChabane/spec-proof/ledger/internal/testkit"
)

// plainPayloads reads a bundle that is its own statement.
type plainPayloads struct{}

func (plainPayloads) Payload(bundle []byte) ([]byte, error) { return bundle, nil }

const (
	verifiesProofCheck = " [verifies SP-203~1]"
	verifiesAudit      = " [verifies SP-207~1]"
)

func TestCheckFileProofs(t *testing.T) {
	statements := testkit.ValidStatements(t)
	valid := statements[0]
	path := valid.Location.BundlePath()

	t.Run("a bundle of an admitted CI statement at its own path is accepted"+verifiesProofCheck, func(t *testing.T) {
		for _, statement := range statements {
			if err := ledger.CheckFile(statement.Location.BundlePath(), statement.Bytes, plainPayloads{}); err != nil {
				t.Errorf("%s: %v", statement.Location.BundlePath(), err)
			}
		}
	})

	t.Run("a statement of another cache key than its file name is refused"+verifiesProofCheck, func(t *testing.T) {
		other := format.ProofLocation{Module: valid.Module, SubjectKey: valid.Location.SubjectKey, CacheKey: strings.Repeat("0", 64)}
		if err := ledger.CheckFile(other.BundlePath(), valid.Bytes, plainPayloads{}); !errors.Is(err, refusal.ErrRefused) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a statement under another subject key is refused"+verifiesProofCheck, func(t *testing.T) {
		other := format.ProofLocation{Module: valid.Module, SubjectKey: "PARK-9~1", CacheKey: valid.Location.CacheKey}
		if err := ledger.CheckFile(other.BundlePath(), valid.Bytes, plainPayloads{}); !errors.Is(err, refusal.ErrRefused) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a statement that is not canonical is refused"+verifiesProofCheck, func(t *testing.T) {
		if err := ledger.CheckFile(path, append(bytes.Clone(valid.Bytes), '\n'), plainPayloads{}); !errors.Is(err, refusal.ErrRefused) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a statement produced locally is refused"+verifiesProofCheck, func(t *testing.T) {
		local := bytes.Replace(valid.Bytes, []byte(`"producedBy":"ci"`), []byte(`"producedBy":"local"`), 1)
		if bytes.Equal(local, valid.Bytes) {
			t.Fatal("fixture has no producedBy member")
		}
		if err := ledger.CheckFile(path, local, plainPayloads{}); !errors.Is(err, refusal.ErrRefused) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a path outside the proof layout is refused"+verifiesProofCheck, func(t *testing.T) {
		for _, bad := range []string{"proofs/m/" + valid.Location.CacheKey + ".dsse.json", "proofs/m/s/x.dsse.json", "elsewhere/file", "README.md"} {
			if err := ledger.CheckFile(bad, valid.Bytes, plainPayloads{}); !errors.Is(err, refusal.ErrRefused) {
				t.Errorf("%s: error %v", bad, err)
			}
		}
	})

	t.Run("a payload that cannot be read is refused"+verifiesProofCheck, func(t *testing.T) {
		if err := ledger.CheckFile(path, valid.Bytes, failingPayloads{}); !errors.Is(err, refusal.ErrRefused) {
			t.Fatalf("error %v", err)
		}
	})
}

type failingPayloads struct{}

func (failingPayloads) Payload([]byte) ([]byte, error) { return nil, errors.New("unreadable") }

func TestCheckFileOthers(t *testing.T) {
	ciphertext := []byte("age-encryption.org/v1\n-> X25519 abc\n\nbody")

	t.Run("a mutant file is age ciphertext named by its cache key"+verifiesMutantsSeedsTrust, func(t *testing.T) {
		if err := ledger.CheckFile(ledger.MutantPath(cacheKey), ciphertext, plainPayloads{}); err != nil {
			t.Fatal(err)
		}
		if err := ledger.CheckFile(ledger.MutantPath(cacheKey), []byte("plain mutant"), plainPayloads{}); !errors.Is(err, refusal.ErrRefused) {
			t.Fatalf("plain mutant: error %v", err)
		}
		if err := ledger.CheckFile("mutants/not-a-key.age", ciphertext, plainPayloads{}); !errors.Is(err, refusal.ErrRefused) {
			t.Fatalf("badly named mutant: error %v", err)
		}
	})

	t.Run("a seed file records the seed of the head commit it is named by"+verifiesMutantsSeedsTrust, func(t *testing.T) {
		record, _ := ledger.NewSeedRecord(headSHA, "7")
		content, _ := record.Bytes()
		if err := ledger.CheckFile(ledger.SeedPath(headSHA), content, plainPayloads{}); err != nil {
			t.Fatal(err)
		}
		other := strings.Repeat("cd", 20)
		if err := ledger.CheckFile(ledger.SeedPath(other), content, plainPayloads{}); !errors.Is(err, refusal.ErrRefused) {
			t.Fatalf("seed under another head commit: error %v", err)
		}
	})

	t.Run("a trust file sits below the id of its snapshot and a target is named by its digest"+verifiesMutantsSeedsTrust, func(t *testing.T) {
		timestamp := []byte(`{"signed":{"version":3}}`)
		id := ledger.SnapshotID(timestamp)
		content := []byte(`{"trusted":"root"}`)
		named := ledger.TrustPath(id, "targets/"+format.SHA256Hex(content)+".trusted_root.json")
		if err := ledger.CheckFile(named, content, plainPayloads{}); err != nil {
			t.Fatal(err)
		}
		if err := ledger.CheckFile(named, []byte("tampered"), plainPayloads{}); !errors.Is(err, refusal.ErrRefused) {
			t.Fatalf("tampered target: error %v", err)
		}
		if err := ledger.CheckFile(ledger.TrustPath(id, "2.root.json"), content, plainPayloads{}); err != nil {
			t.Fatalf("metadata file: %v", err)
		}
		if err := ledger.CheckFile(ledger.TrustPath(id, "timestamp.json"), timestamp, plainPayloads{}); err != nil {
			t.Fatalf("timestamp file: %v", err)
		}
		if err := ledger.CheckFile(ledger.TrustPath(id, "timestamp.json"), []byte(`{"signed":{"version":4}}`), plainPayloads{}); !errors.Is(err, refusal.ErrRefused) {
			t.Fatalf("a timestamp file in the snapshot of another: error %v", err)
		}
		for _, bad := range []string{"trust/root.json", "trust/short/root.json", "trust/" + id, "trust/" + id + "/../escape", "trust/../escape"} {
			if err := ledger.CheckFile(bad, content, plainPayloads{}); !errors.Is(err, refusal.ErrRefused) {
				t.Errorf("%s: error %v", bad, err)
			}
		}
	})
}

func TestAuditRules(t *testing.T) {
	commit := func(id string, parents ...string) ledger.CommitRecord {
		record := ledger.CommitRecord{ID: ledger.ObjectID(id)}
		for _, parent := range parents {
			record.Parents = append(record.Parents, ledger.ObjectID(parent))
		}
		return record
	}

	t.Run("a chain with one root is linear"+verifiesAudit, func(t *testing.T) {
		if findings := ledger.CheckLinearHistory([]ledger.CommitRecord{commit("c", "b"), commit("b", "a"), commit("a")}); len(findings) != 0 {
			t.Fatalf("findings %v", findings)
		}
	})

	t.Run("a merge commit is a finding"+verifiesAudit, func(t *testing.T) {
		findings := ledger.CheckLinearHistory([]ledger.CommitRecord{commit("m", "a", "b"), commit("a", "r"), commit("b", "r"), commit("r")})
		if len(findings) != 1 || findings[0].Rule != ledger.RuleLinearHistory || findings[0].Subject != "m" {
			t.Fatalf("findings %v", findings)
		}
	})

	t.Run("two roots are a finding"+verifiesAudit, func(t *testing.T) {
		findings := ledger.CheckLinearHistory([]ledger.CommitRecord{commit("b"), commit("a")})
		if len(findings) != 1 || findings[0].Rule != ledger.RuleLinearHistory {
			t.Fatalf("findings %v", findings)
		}
	})

	t.Run("every rewritten path is a finding"+verifiesAudit, func(t *testing.T) {
		findings := ledger.CheckNoRewrite([]string{"proofs/a", "seeds/b"})
		if len(findings) != 2 || findings[0].Rule != ledger.RuleNoRewrite || findings[1].Subject != "seeds/b" {
			t.Fatalf("findings %v", findings)
		}
		if len(ledger.CheckNoRewrite(nil)) != 0 {
			t.Fatal("no rewrite must give no finding")
		}
	})

	t.Run("a file that is not what its name says is a finding and a good one is not"+verifiesAudit, func(t *testing.T) {
		if findings := ledger.CheckFileName("mutants/"+cacheKey+".age", []byte("age-encryption.org/v1\nx"), plainPayloads{}); len(findings) != 0 {
			t.Fatalf("findings %v", findings)
		}
		findings := ledger.CheckFileName("mutants/"+cacheKey+".age", []byte("plain"), plainPayloads{})
		if len(findings) != 1 || findings[0].Rule != ledger.RuleContentAddressed || !strings.Contains(findings[0].String(), "content_addressed") {
			t.Fatalf("findings %v", findings)
		}
	})
}

func FuzzCheckFile(f *testing.F) {
	for _, statement := range testkit.ValidStatements(f) {
		f.Add(statement.Location.BundlePath(), statement.Bytes)
	}
	f.Add("mutants/"+cacheKey+".age", []byte("age-encryption.org/v1\n"))
	f.Add("trust/targets/x.json", []byte("{}"))
	f.Fuzz(func(t *testing.T, path string, content []byte) {
		_ = ledger.CheckFile(path, content, plainPayloads{})
	})
}
