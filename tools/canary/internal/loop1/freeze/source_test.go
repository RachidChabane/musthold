package freeze_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/deviation"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/freeze"
)

const repositoryRoot = "../../../../../"

// memorySource serves files from a map, as a commit would.
type memorySource map[string]string

func (m memorySource) Show(_ context.Context, _, file string) ([]byte, error) {
	content, ok := m[file]
	if !ok {
		return nil, os.ErrNotExist
	}
	return []byte(content), nil
}

func (m memorySource) TreeFiles(_ context.Context, _ string, paths ...string) ([]string, error) {
	var files []string
	for file := range m {
		for _, path := range paths {
			if strings.HasPrefix(file, path+"/") {
				files = append(files, file)
			}
		}
	}
	return files, nil
}

func memoryPolicy(edits map[string]string) memorySource {
	source := memorySource{"format/executor.go": formatSource, "format/executor_test.go": "package format\nconst IsolationRestricted = \"wrong\""}
	for module := range freeze.ExpectedPolicy {
		source[module+"/internal/domain/policy/policy.go"] = policySource(module, edits)
	}
	return source
}

func TestReadPolicy(t *testing.T) {
	t.Run("constants, threshold wrappers and the isolation reference of the format package resolve, and 0.70 equals 0.7 [verifies SP-2621~1]", func(t *testing.T) {
		got, err := freeze.ReadPolicy(t.Context(), memoryPolicy(nil), "any")
		if err != nil {
			t.Fatal(err)
		}
		if failures := freeze.PolicyFailures("source", got, freeze.ExpectedPolicy); len(failures) != 0 {
			t.Errorf("failures %v", failures)
		}
		if got["discpower"]["tau_judge"] != "0.7" || got["alignment"]["min_isolation"] != "restricted" {
			t.Errorf("policy %v", got)
		}
	})

	t.Run("an edited default is read as edited [verifies SP-2621~1]", func(t *testing.T) {
		got, err := freeze.ReadPolicy(t.Context(), memoryPolicy(map[string]string{"completeness.max_hunks": "51"}), "any")
		if err != nil {
			t.Fatal(err)
		}
		if got["completeness"]["max_hunks"] != "51" {
			t.Errorf("policy %v", got["completeness"])
		}
	})

	t.Run("a module without a Default literal, or without a member of the table, is refused [verifies SP-2621~1]", func(t *testing.T) {
		source := memoryPolicy(nil)
		source["completeness/internal/domain/policy/policy.go"] = "package policy\nfunc Default() Policy { return load() }\n"
		if _, err := freeze.ReadPolicy(t.Context(), source, "any"); !errors.Is(err, freeze.ErrPolicy) {
			t.Errorf("no literal: %v", err)
		}
		source = memoryPolicy(nil)
		source["alignment/internal/domain/policy/policy.go"] = strings.Replace(source["alignment/internal/domain/policy/policy.go"], "ShadowRate: DefaultShadowRate,", "", 1)
		if _, err := freeze.ReadPolicy(t.Context(), source, "any"); !errors.Is(err, freeze.ErrPolicy) {
			t.Errorf("no member: %v", err)
		}
	})

	t.Run("the defaults of the repository's own modules equal the table of PREREG §2 [verifies SP-2621~1]", func(t *testing.T) {
		if _, err := exec.LookPath("git"); err != nil {
			t.Skip("git is not installed")
		}
		repo := freeze.Git{Dir: repositoryRoot, Exec: freeze.OSExec{}}
		got, err := freeze.ReadPolicy(t.Context(), repo, "HEAD")
		if err != nil {
			t.Fatal(err)
		}
		if failures := freeze.PolicyFailures("HEAD", got, freeze.ExpectedPolicy); len(failures) != 0 {
			t.Errorf("failures %v", failures)
		}
	})

	t.Run("a decimal is written in its shortest form and a non-decimal is refused [verifies SP-2621~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			whole := rapid.IntRange(0, 9999).Draw(t, "whole")
			tenths := rapid.IntRange(0, 9).Draw(t, "tenths")
			plain, padded := formatDecimal(whole, tenths, 1), formatDecimal(whole, tenths, 3)
			a, errA := freeze.NormalizeNumber(plain)
			b, errB := freeze.NormalizeNumber(padded)
			if errA != nil || errB != nil || a != b {
				t.Fatalf("%q -> %q (%v), %q -> %q (%v)", plain, a, errA, padded, b, errB)
			}
		})
		for _, bad := range []string{"", "1e3", "-1", "0x10", "1."} {
			if _, err := freeze.NormalizeNumber(bad); err == nil {
				t.Errorf("%q accepted", bad)
			}
		}
	})
}

func formatDecimal(whole, tenths, places int) string {
	return strings.TrimSpace(strings.Join([]string{itoa(whole), ".", itoa(tenths), strings.Repeat("0", places-1)}, ""))
}

func itoa(value int) string {
	digits := ""
	for {
		digits = string(rune('0'+value%10)) + digits
		if value /= 10; value == 0 {
			return digits
		}
	}
}

type fakeTags struct {
	tags     []string
	contains map[string]bool
	err      error
}

func (f fakeTags) Tags(context.Context) ([]string, error) { return f.tags, f.err }
func (f fakeTags) Contains(_ context.Context, tag, _ string) (bool, error) {
	return f.contains[tag], f.err
}

func TestFirstRelease(t *testing.T) {
	t.Run("the lowest version that holds the commit is the release, whatever order the tags come in [verifies SP-2618~1]", func(t *testing.T) {
		releases := fakeTags{tags: []string{"v0.10.0", "v0.5.1", "v0.9.0", "latest", "v0.5.0-rc1", "v0.5.2"}, contains: map[string]bool{"v0.5.1": true, "v0.5.2": true, "v0.9.0": true, "v0.10.0": true}}
		got, err := freeze.FirstRelease(t.Context(), releases, "pin")
		if err != nil || got != "v0.5.1" {
			t.Errorf("release %q, %v", got, err)
		}
	})
	t.Run("a pin no release holds is none, and a failed query is an error [verifies SP-2618~1]", func(t *testing.T) {
		got, err := freeze.FirstRelease(t.Context(), fakeTags{tags: []string{"v0.1.0"}}, "pin")
		if err != nil || got != freeze.NoRelease {
			t.Errorf("release %q, %v", got, err)
		}
		if _, err := freeze.FirstRelease(t.Context(), fakeTags{tags: []string{"v0.1.0"}, err: os.ErrPermission}, "pin"); err == nil {
			t.Error("a failed query passed")
		}
	})
}

func TestSums(t *testing.T) {
	t.Run("sums read back as they were written, sorted by path, and a malformed line is refused [verifies SP-2626~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			paths := rapid.SliceOfNDistinct(rapid.StringMatching(`[a-z][a-z0-9 ./-]{0,20}`), 1, 8, func(s string) string { return s }).Draw(t, "paths")
			var sums []freeze.Sum
			for _, path := range paths {
				sums = append(sums, freeze.Sum{Digest: deviation.Imprint([]byte(path)), Path: path})
			}
			parsed, err := freeze.ParseSums(freeze.FormatSums(sums))
			if err != nil || len(parsed) != len(sums) {
				t.Fatalf("parsed %v, %v", parsed, err)
			}
			if freeze.FormatSums(parsed) != freeze.FormatSums(sums) {
				t.Fatal("the format is not stable")
			}
		})
		for _, bad := range []string{"abc  file\n", strings.Repeat("a", 64) + " file\n", strings.Repeat("a", 64) + "  \n", ""} {
			if _, err := freeze.ParseSums(bad); !errors.Is(err, freeze.ErrSums) {
				t.Errorf("%q: %v", bad, err)
			}
		}
	})

	t.Run("the committed pre-registration sums parse and match the files at the tag [verifies SP-2626~1]", func(t *testing.T) {
		repo := freeze.Git{Dir: repositoryRoot, Exec: freeze.OSExec{}}
		if _, err := repo.PeelTag(t.Context(), freeze.PreregTag); err != nil {
			t.Skipf("the tag is not in this clone: %v", err)
		}
		checker := freeze.Checker{Root: repositoryRoot, Repo: repo, Tokens: deviation.OpenSSL{}, Runs: fakeRuns{}, Engine: fakeEngine{}}
		for _, failure := range checker.Check(t.Context()) {
			if failure.Condition == freeze.CondSums && strings.Contains(failure.Detail, "experiments/loop-1/freeze/SHA256SUMS") && !strings.Contains(failure.Detail, "inputs-SHA256SUMS") {
				t.Errorf("the pre-registration sums do not hold: %v", failure)
			}
		}
	})
}

func TestRealTags(t *testing.T) {
	repo := freeze.Git{Dir: repositoryRoot, Exec: freeze.OSExec{}}
	commit, err := repo.PeelTag(t.Context(), freeze.PreregTag)
	if err != nil {
		t.Skipf("the tag is not in this clone: %v", err)
	}
	t.Run("the peeled pre-registration tag is the commit its file holds, and its token stamps that file [verifies SP-2624~1]", func(t *testing.T) {
		file := filepath.Join(repositoryRoot, filepath.FromSlash(freeze.Prereg.CommitFile()))
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(content)) != commit {
			t.Errorf("file %q, tag %s", content, commit)
		}
		if _, err := exec.LookPath("openssl"); err != nil {
			t.Skip("openssl is not installed")
		}
		token, err := deviation.OpenSSL{}.Read(t.Context(), deviation.ReplyPath(file))
		if err != nil || token.Imprint != deviation.Imprint(content) {
			t.Errorf("token %+v, %v", token, err)
		}
	})
}
