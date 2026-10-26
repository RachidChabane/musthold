package format_test

import (
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/format"
)

const verifiesLayout = " [verifies SP-197~1]"

var layoutCacheKey = strings.Repeat("ab", 32)

func TestProofLocationPaths(t *testing.T) {
	location := format.ProofLocation{Module: "discpower", SubjectKey: "PARK-2~1", CacheKey: layoutCacheKey}

	t.Run("a bundle path round-trips"+verifiesLayout, func(t *testing.T) {
		path := location.BundlePath()
		if want := "proofs/discpower/PARK-2~1/" + layoutCacheKey + ".dsse.json"; path != want {
			t.Fatalf("path %q, want %q", path, want)
		}
		parsed, ok := format.ParseBundlePath(path)
		if !ok || parsed != location {
			t.Fatalf("parsed %+v %v", parsed, ok)
		}
	})

	t.Run("a statement path round-trips"+verifiesLayout, func(t *testing.T) {
		path := location.StatementPath()
		if want := "discpower/PARK-2~1/" + layoutCacheKey + ".json"; path != want {
			t.Fatalf("path %q, want %q", path, want)
		}
		parsed, ok := format.ParseStatementPath(path)
		if !ok || parsed != location {
			t.Fatalf("parsed %+v %v", parsed, ok)
		}
	})

	rejected := []string{
		"", "proofs", "proofs/m/s", "proofs/m/s/" + layoutCacheKey + ".json",
		"proofs/m/s/short.dsse.json", "proofs/m/s/x/" + layoutCacheKey + ".dsse.json",
		"mutants/m/s/" + layoutCacheKey + ".dsse.json", "proofs/../s/" + layoutCacheKey + ".dsse.json",
		"proofs/m//" + layoutCacheKey + ".dsse.json", "proofs/m/s/" + strings.ToUpper(layoutCacheKey) + ".dsse.json",
	}
	for _, path := range rejected {
		t.Run("the bundle path "+path+" is refused"+verifiesLayout, func(t *testing.T) {
			if _, ok := format.ParseBundlePath(path); ok {
				t.Fatal("accepted")
			}
		})
	}

	t.Run("a statement path with the wrong suffix or depth is refused"+verifiesLayout, func(t *testing.T) {
		for _, path := range []string{"m/s/" + layoutCacheKey + ".dsse.json", "m/" + layoutCacheKey + ".json", "m/s/x/" + layoutCacheKey + ".json"} {
			if _, ok := format.ParseStatementPath(path); ok {
				t.Errorf("accepted %q", path)
			}
		}
	})
}

func TestIsPathSegment(t *testing.T) {
	cases := map[string]bool{
		"module": true, "PARK-2~1": true, "a%2Fb": true, "": false, ".": false, "..": false,
		"a/b": false, `a\b`: false, "a\nb": false, "a\x00b": false, "a\x7fb": false,
	}
	for name, want := range cases {
		t.Run(name+verifiesLayout, func(t *testing.T) {
			if got := format.IsPathSegment(name); got != want {
				t.Fatalf("IsPathSegment(%q) = %v", name, got)
			}
		})
	}
}

func TestSubjectKey(t *testing.T) {
	t.Run("a criterion is its own key"+verifiesLayout, func(t *testing.T) {
		if key := format.SubjectKey("PARK-2~1"); key != "PARK-2~1" {
			t.Fatalf("key %q", key)
		}
	})

	t.Run("a spec path becomes one segment"+verifiesLayout, func(t *testing.T) {
		if key := format.SubjectKey("specs/parkfee.md"); key != "specs%2Fparkfee.md" {
			t.Fatalf("key %q", key)
		}
	})

	t.Run("the key of a name without control characters is one segment and distinct names keep distinct keys"+verifiesLayout, func(t *testing.T) {
		name := rapid.StringMatching(`[^\x00-\x1f\x7f]{1,12}`)
		rapid.Check(t, func(t *rapid.T) {
			a, b := name.Draw(t, "a"), name.Draw(t, "b")
			keyA, keyB := format.SubjectKey(a), format.SubjectKey(b)
			if a == "." || a == ".." {
				return
			}
			if !format.IsPathSegment(keyA) {
				t.Fatalf("key %q of %q is not a path segment", keyA, a)
			}
			if (a == b) != (keyA == keyB) {
				t.Fatalf("keys %q %q of %q %q", keyA, keyB, a, b)
			}
		})
	})
}
