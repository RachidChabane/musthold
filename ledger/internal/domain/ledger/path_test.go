package ledger_test

import (
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/ledger/internal/domain/ledger"
)

const verifiesMutantsSeedsTrust = " [verifies SP-204~1]"

var cacheKey = strings.Repeat("c", 64)

func TestClassify(t *testing.T) {
	cases := map[string]ledger.Category{
		"proofs/m/s/" + cacheKey + ".dsse.json":      ledger.CategoryProof,
		"mutants/" + cacheKey + ".age":               ledger.CategoryMutant,
		"seeds/" + strings.Repeat("a", 40) + ".json": ledger.CategorySeed,
		"trust/" + cacheKey + "/root.json":           ledger.CategoryTrust,
		"README.md":                                  ledger.CategoryUnknown,
		"other/file":                                 ledger.CategoryUnknown,
		"proofs":                                     ledger.CategoryUnknown,
		"":                                           ledger.CategoryUnknown,
	}
	for path, want := range cases {
		t.Run(path+verifiesMutantsSeedsTrust, func(t *testing.T) {
			if got := ledger.Classify(path); got != want {
				t.Fatalf("Classify(%q) = %d, want %d", path, got, want)
			}
		})
	}
}

func TestMutantPath(t *testing.T) {
	t.Run("a mutant path round-trips"+verifiesMutantsSeedsTrust, func(t *testing.T) {
		got, ok := ledger.ParseMutantPath(ledger.MutantPath(cacheKey))
		if !ok || got != cacheKey {
			t.Fatalf("parsed %q %v", got, ok)
		}
	})
	for _, path := range []string{"mutants/short.age", "mutants/" + cacheKey, "mutants/" + cacheKey + ".txt", "seeds/" + cacheKey + ".age", "mutants/a/" + cacheKey + ".age"} {
		t.Run(path+" is refused"+verifiesMutantsSeedsTrust, func(t *testing.T) {
			if _, ok := ledger.ParseMutantPath(path); ok {
				t.Fatal("accepted")
			}
		})
	}
}

func TestSeedPath(t *testing.T) {
	sha1 := strings.Repeat("ab", 20)
	sha256 := strings.Repeat("cd", 32)
	for _, head := range []string{sha1, sha256} {
		t.Run("a head commit digest of "+string(rune('0'+len(head)/10))+" hex digits round-trips"+verifiesMutantsSeedsTrust, func(t *testing.T) {
			got, ok := ledger.ParseSeedPath(ledger.SeedPath(head))
			if !ok || got != head {
				t.Fatalf("parsed %q %v", got, ok)
			}
		})
	}
	for _, path := range []string{"seeds/abc.json", "seeds/" + strings.ToUpper(sha1) + ".json", "seeds/" + sha1, "proofs/" + sha1 + ".json"} {
		t.Run(path+" is refused"+verifiesMutantsSeedsTrust, func(t *testing.T) {
			if _, ok := ledger.ParseSeedPath(path); ok {
				t.Fatal("accepted")
			}
		})
	}
}

func TestIsAgeCiphertext(t *testing.T) {
	t.Run("only the age header marks ciphertext"+verifiesMutantsSeedsTrust, func(t *testing.T) {
		if !ledger.IsAgeCiphertext([]byte("age-encryption.org/v1\n-> X25519 abc\n")) {
			t.Fatal("age file refused")
		}
		for _, plain := range []string{"", "def mutant(): pass", "age-encryption.org/v2\n", "age-encryption.org/v1"} {
			if ledger.IsAgeCiphertext([]byte(plain)) {
				t.Fatalf("%q accepted", plain)
			}
		}
	})
}

func TestIsRelativeFilePath(t *testing.T) {
	cases := map[string]bool{
		"root.json": true, "targets/abc.root.json": true, "": false, "/abs": false, "a//b": false,
		"a/../b": false, "./a": false, "a/": false, `a\b`: false,
	}
	for path, want := range cases {
		t.Run(path+verifiesMutantsSeedsTrust, func(t *testing.T) {
			if got := ledger.IsRelativeFilePath(path); got != want {
				t.Fatalf("IsRelativeFilePath(%q) = %v", path, got)
			}
		})
	}
	t.Run("a path of safe segments is relative and never leaves its directory"+verifiesMutantsSeedsTrust, func(t *testing.T) {
		segments := rapid.SliceOfN(rapid.StringMatching(`[a-z0-9._-]{1,6}`), 1, 4)
		rapid.Check(t, func(t *rapid.T) {
			parts := segments.Draw(t, "segments")
			path := strings.Join(parts, "/")
			want := true
			for _, part := range parts {
				want = want && part != "." && part != ".."
			}
			if ledger.IsRelativeFilePath(path) != want {
				t.Fatalf("IsRelativeFilePath(%q) != %v", path, want)
			}
		})
	})
}
