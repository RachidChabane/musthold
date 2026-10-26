package ledger_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/ledger/internal/domain/ledger"
	"github.com/RachidChabane/musthold/ledger/internal/domain/refusal"
)

const (
	verifiesSeed    = " [verifies SP-204~1]"
	verifiesSeedSHA = " [verifies SP-208~1]"
)

var headSHA = strings.Repeat("ab", 20)

func TestSeedRecord(t *testing.T) {
	t.Run("the seed is the SHA-256 of the head commit and the run joined by a newline"+verifiesSeedSHA, func(t *testing.T) {
		record, err := ledger.NewSeedRecord(headSHA, "12345")
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256([]byte(headSHA + "\n12345"))
		if record.Seed != hex.EncodeToString(sum[:]) || record.HeadSHA != headSHA || record.RunID != "12345" {
			t.Fatalf("record %+v", record)
		}
	})

	t.Run("a record round-trips through its canonical bytes"+verifiesSeed, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			run := rapid.StringMatching(`[0-9a-zA-Z_-]{1,12}`).Draw(t, "run")
			record, err := ledger.NewSeedRecord(headSHA, run)
			if err != nil {
				t.Fatal(err)
			}
			content, err := record.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := ledger.ParseSeedRecord(content)
			if err != nil || parsed != record {
				t.Fatalf("parsed %+v: %v", parsed, err)
			}
		})
	})

	t.Run("a different run gives a different seed"+verifiesSeed, func(t *testing.T) {
		first, _ := ledger.NewSeedRecord(headSHA, "1")
		second, _ := ledger.NewSeedRecord(headSHA, "2")
		if first.Seed == second.Seed {
			t.Fatal("seeds are equal")
		}
	})

	t.Run("the addition records the seed at its path and keeps an existing one"+verifiesSeed, func(t *testing.T) {
		record, _ := ledger.NewSeedRecord(headSHA, "1")
		addition, err := record.Addition()
		if err != nil || addition.Path != "seeds/"+headSHA+".json" || addition.Policy != ledger.PolicyKeepExisting {
			t.Fatalf("addition %+v: %v", addition, err)
		}
	})

	invalid := []struct{ name, head, run string }{
		{"a head commit that is not a digest", "main", "1"},
		{"a short head commit", "abcd", "1"},
		{"an upper-case head commit", strings.ToUpper(headSHA), "1"},
		{"an empty run", headSHA, ""},
		{"a run with a control character", headSHA, "1\n2"},
	}
	for _, tt := range invalid {
		t.Run(tt.name+" is refused"+verifiesSeed, func(t *testing.T) {
			if _, err := ledger.NewSeedRecord(tt.head, tt.run); !errors.Is(err, refusal.ErrRefused) {
				t.Fatalf("error %v", err)
			}
		})
	}

	t.Run("a record whose seed is not the digest of its head commit and run is refused"+verifiesSeed, func(t *testing.T) {
		forged := `{"headSha":"` + headSHA + `","runId":"1","seed":"` + strings.Repeat("0", 64) + `"}`
		if _, err := ledger.ParseSeedRecord([]byte(forged)); !errors.Is(err, refusal.ErrRefused) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a record that is not canonical is refused"+verifiesSeed, func(t *testing.T) {
		record, _ := ledger.NewSeedRecord(headSHA, "1")
		content, _ := record.Bytes()
		if _, err := ledger.ParseSeedRecord(append(content, '\n')); !errors.Is(err, refusal.ErrRefused) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a document that is not JSON is refused"+verifiesSeed, func(t *testing.T) {
		if _, err := ledger.ParseSeedRecord([]byte("{")); !errors.Is(err, refusal.ErrRefused) {
			t.Fatalf("error %v", err)
		}
	})
}

func FuzzParseSeedRecord(f *testing.F) {
	record, _ := ledger.NewSeedRecord(headSHA, "1")
	content, _ := record.Bytes()
	f.Add(content)
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		parsed, err := ledger.ParseSeedRecord(data)
		if err != nil {
			return
		}
		again, err := parsed.Bytes()
		if err != nil || string(again) != string(data) {
			t.Fatalf("accepted bytes %q are not the canonical record %q", data, again)
		}
	})
}
