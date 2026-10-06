package format_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/formattest/proofs"
)

func sha256Hex(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func TestFileSetDigest(t *testing.T) {
	t.Run("the digest hashes path NUL content NUL over files sorted by path [verifies SP-8~1]", func(t *testing.T) {
		files := []format.File{
			{Path: "b/two.txt", Content: []byte("world")},
			{Path: "a.txt", Content: []byte("hello")},
		}
		got, err := format.FileSetDigest(files)
		if err != nil {
			t.Fatal(err)
		}
		want := sha256Hex("a.txt\x00hello\x00b/two.txt\x00world\x00")
		if got != want {
			t.Fatalf("digest = %s, want %s", got, want)
		}
	})

	t.Run("an empty file set has the digest of the empty stream [verifies SP-8~1]", func(t *testing.T) {
		got, err := format.FileSetDigest(nil)
		if err != nil {
			t.Fatal(err)
		}
		if want := sha256Hex(""); got != want {
			t.Fatalf("digest = %s, want %s", got, want)
		}
	})

	t.Run("sorting is byte-wise, not numeric [verifies SP-8~1]", func(t *testing.T) {
		files := []format.File{{Path: "PARK-2", Content: []byte("2")}, {Path: "PARK-10", Content: []byte("10")}}
		got, err := format.FileSetDigest(files)
		if err != nil {
			t.Fatal(err)
		}
		if want := sha256Hex("PARK-10\x0010\x00PARK-2\x002\x00"); got != want {
			t.Fatalf("digest = %s, want %s", got, want)
		}
	})

	rejected := []struct {
		name  string
		files []format.File
	}{
		{"an absolute path", []format.File{{Path: "/etc/passwd"}}},
		{"a parent segment", []format.File{{Path: "a/../b"}}},
		{"a current-directory segment", []format.File{{Path: "./a"}}},
		{"an empty segment", []format.File{{Path: "a//b"}}},
		{"a backslash", []format.File{{Path: `a\b`}}},
		{"an empty path", []format.File{{Path: ""}}},
		{"a NUL byte", []format.File{{Path: "a\x00b"}}},
		{"a repeated path", []format.File{{Path: "a"}, {Path: "a"}}},
	}
	for _, c := range rejected {
		t.Run("the digest refuses "+c.name+" [verifies SP-8~1]", func(t *testing.T) {
			if _, err := format.FileSetDigest(c.files); err == nil {
				t.Fatal("expected an error")
			}
		})
	}

	t.Run("the digest does not depend on the order of the files [verifies SP-8~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			count := rapid.IntRange(0, 6).Draw(t, "count")
			files := make([]format.File, count)
			for index := range files {
				files[index] = format.File{
					Path:    "dir" + string(rune('a'+index)) + "/" + rapid.StringMatching(`[a-z]{1,5}`).Draw(t, "name"),
					Content: rapid.SliceOf(rapid.Byte()).Draw(t, "content"),
				}
			}
			shuffled := rapid.Permutation(files).Draw(t, "shuffled")
			first, err := format.FileSetDigest(files)
			if err != nil {
				t.Fatal(err)
			}
			second, err := format.FileSetDigest(shuffled)
			if err != nil {
				t.Fatal(err)
			}
			if first != second {
				t.Fatalf("digest depends on order: %s != %s", first, second)
			}
		})
	})

	t.Run("the digest changes when a path or a content changes [verifies SP-8~1]", func(t *testing.T) {
		base, _ := format.FileSetDigest([]format.File{{Path: "a", Content: []byte("x")}})
		renamed, _ := format.FileSetDigest([]format.File{{Path: "b", Content: []byte("x")}})
		edited, _ := format.FileSetDigest([]format.File{{Path: "a", Content: []byte("y")}})
		if base == renamed || base == edited {
			t.Fatal("digest did not change")
		}
	})
}

func TestCacheKey(t *testing.T) {
	subject := []format.Subject{format.RequirementSubject("PARK-2", 1, proofs.Digest("statement"))}
	inputs := []format.Input{proofs.Input(format.RoleSpec, "specs/a.md"), proofs.Input(format.RoleTestSource, "tests/a.py")}

	t.Run("the key is the digest of the canonical subject and inputs [verifies SP-9~1]", func(t *testing.T) {
		got, err := format.CacheKey(subject, inputs)
		if err != nil {
			t.Fatal(err)
		}
		canonical := `{"inputs":[` +
			`{"digest":{"sha256":"` + inputs[0].Digest.SHA256 + `"},"name":"specs/a.md","role":"spec"},` +
			`{"digest":{"sha256":"` + inputs[1].Digest.SHA256 + `"},"name":"tests/a.py","role":"test_source"}],` +
			`"subject":[{"digest":{"sha256":"` + subject[0].Digest.SHA256 + `"},"name":"PARK-2~1"}]}`
		if want := sha256Hex(canonical); got != want {
			t.Fatalf("key = %s, want %s", got, want)
		}
	})

	t.Run("the key does not depend on the order of the inputs [verifies SP-9~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			drawn := proofs.Inputs().Draw(t, "inputs")
			shuffled := rapid.Permutation(drawn).Draw(t, "shuffled")
			first, err := format.CacheKey(subject, drawn)
			if err != nil {
				t.Fatal(err)
			}
			second, err := format.CacheKey(subject, shuffled)
			if err != nil {
				t.Fatal(err)
			}
			if first != second {
				t.Fatalf("key depends on input order: %s != %s", first, second)
			}
		})
	})

	t.Run("the key changes when any input changes [verifies SP-9~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			drawn := proofs.Inputs().Draw(t, "inputs")
			victim := rapid.IntRange(0, len(drawn)-1).Draw(t, "victim")
			edited := append([]format.Input(nil), drawn...)
			edited[victim].Digest.SHA256 = proofs.Digest("edited " + drawn[victim].Digest.SHA256)
			before, err := format.CacheKey(subject, drawn)
			if err != nil {
				t.Fatal(err)
			}
			after, err := format.CacheKey(subject, edited)
			if err != nil {
				t.Fatal(err)
			}
			if before == after {
				t.Fatal("key did not change")
			}
		})
	})

	t.Run("the key changes when the subject changes [verifies SP-9~1]", func(t *testing.T) {
		other := []format.Subject{format.RequirementSubject("PARK-2", 2, subject[0].Digest.SHA256)}
		first, _ := format.CacheKey(subject, inputs)
		second, _ := format.CacheKey(other, inputs)
		if first == second {
			t.Fatal("key did not change")
		}
	})

	t.Run("computing the key does not reorder the caller's inputs [verifies SP-9~1]", func(t *testing.T) {
		reversed := []format.Input{inputs[1], inputs[0]}
		if _, err := format.CacheKey(subject, reversed); err != nil {
			t.Fatal(err)
		}
		if reversed[0].Role != format.RoleTestSource {
			t.Fatal("the caller's slice was reordered")
		}
	})
}

func TestResultDigest(t *testing.T) {
	t.Run("the digest covers the canonical map from test to outcome [verifies SP-21~1]", func(t *testing.T) {
		got, err := format.ResultDigest(map[string]string{"t2": "failed", "t1": "passed"})
		if err != nil {
			t.Fatal(err)
		}
		if want := sha256Hex(`{"t1":"passed","t2":"failed"}`); got != want {
			t.Fatalf("digest = %s, want %s", got, want)
		}
	})

	t.Run("an outcome change changes the digest and a test name change changes it [verifies SP-21~1]", func(t *testing.T) {
		base, _ := format.ResultDigest(map[string]string{"t1": "passed"})
		outcome, _ := format.ResultDigest(map[string]string{"t1": "failed"})
		name, _ := format.ResultDigest(map[string]string{"t2": "passed"})
		if base == outcome || base == name {
			t.Fatal("digest did not change")
		}
	})

	t.Run("the digest of a map is stable across calls [verifies SP-21~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			outcomes := rapid.MapOfN(rapid.StringMatching(`[a-z_]{1,8}`), rapid.SampledFrom([]string{"passed", "failed", "skipped", "error"}), 0, 6).Draw(t, "outcomes")
			first, err := format.ResultDigest(outcomes)
			if err != nil {
				t.Fatal(err)
			}
			second, err := format.ResultDigest(outcomes)
			if err != nil {
				t.Fatal(err)
			}
			if first != second {
				t.Fatal("digest is not stable")
			}
		})
	})
}
