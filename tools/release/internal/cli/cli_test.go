package cli_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/tools/release/internal/cli"
	"github.com/RachidChabane/spec-proof/tools/release/internal/domain"
)

const (
	verifiesPolicy = " [verifies SP-1817~1]"
	verifiesExits  = " [verifies SP-1819~1]"
	verifiesPlan   = " [verifies SP-1801~1]"
	headCommit     = "1111111111111111111111111111111111111111"
)

// fakes implements every port with fixed answers.
type fakes struct {
	tagCommit   string
	headErr     error
	goMod       domain.GoModFacts
	buildResult domain.Outcome
	distFacts   domain.DistFacts
	distErr     error
	written     map[string][]byte
}

func (f *fakes) Head() (string, error)                  { return headCommit, f.headErr }
func (f *fakes) TagCommit(string) (string, bool, error) { return f.tagCommit, f.tagCommit != "", nil }
func (f *fakes) DirtyPaths() ([]string, error)          { return nil, nil }
func (f *fakes) Tags() ([]string, error)                { return []string{"format/v0.1.0"}, nil }
func (f *fakes) CommitTime(string) (time.Time, error)   { return time.Unix(1_700_000_000, 0), nil }
func (f *fakes) PreviousTag(string, domain.Tag) (string, error) {
	return "", nil
}
func (f *fakes) CommitsSince(string, []string) ([]domain.Commit, error) { return nil, nil }
func (f *fakes) GoMod(string) (domain.GoModFacts, error)                { return f.goMod, nil }
func (f *fakes) Manifests([]string) ([]domain.ManifestFact, error)      { return nil, nil }
func (f *fakes) Marketplace() ([]domain.MarketplaceEntry, error)        { return nil, nil }
func (f *fakes) Notice() (domain.NoticeFacts, error) {
	return domain.NoticeFacts{Present: true, Text: "Copyright 2026 Example Holder\n"}, nil
}
func (f *fakes) Exists(string) (bool, error)           { return true, nil }
func (f *fakes) Build(string) domain.Outcome           { return f.buildResult }
func (f *fakes) Test(string) domain.Outcome            { return domain.Outcome{} }
func (f *fakes) PluginGenerated(string) domain.Outcome { return domain.Outcome{} }
func (f *fakes) Compile(domain.BuildSpec) error        { return errors.New("not used") }
func (f *fakes) Pack(domain.ArchiveFormat, []domain.ArchiveEntry, time.Time) ([]byte, error) {
	return nil, errors.New("not used")
}
func (f *fakes) Inspect([]byte) (domain.BuildFacts, error) {
	return domain.BuildFacts{}, errors.New("not used")
}
func (f *fakes) ReadFile(string) ([]byte, error) { return nil, errors.New("not used") }
func (f *fakes) WriteFile(path string, data []byte, _ int64) error {
	f.written[path] = data
	return nil
}
func (f *fakes) RemoveAll(string) error { return nil }
func (f *fakes) ReadDist(string, domain.Tag, []domain.Target) (domain.DistFacts, error) {
	return f.distFacts, f.distErr
}

func (f *fakes) wiring() cli.Wiring {
	return func(string) cli.Services {
		return cli.Services{
			Repository: f, History: f, Files: f, Checks: f, Compiler: f, Archiver: f, Inspector: f, Disk: f, Dist: f,
		}
	}
}

func newFakes() *fakes {
	return &fakes{tagCommit: headCommit, written: map[string][]byte{}}
}

func run(f *fakes, args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := cli.Run(f.wiring(), args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestExitCodes(t *testing.T) {
	t.Run("a command line that does not parse exits with the usage code"+verifiesExits, func(t *testing.T) {
		for name, args := range map[string][]string{
			"no argument":       nil,
			"an unknown verb":   {"publish", "discpower/v0.2.0"},
			"no tag":            {"plan"},
			"a bare version":    {"plan", "v0.2.0"},
			"a tool tag":        {"plan", "tools/release/v0.2.0"},
			"two tags":          {"plan", "discpower/v0.2.0", "format/v0.1.0"},
			"an unknown flag":   {"preflight", "--nonsense", "discpower/v0.2.0"},
			"a flag of no verb": {"plan", "--dry-run", "discpower/v0.2.0"},
			"a bad visibility":  {"preflight", "--visibility", "secret", "discpower/v0.2.0"},
			"a bad target":      {"build", "--targets", "plan9/386", "discpower/v0.2.0"},
			"a notes flag typo": {"notes", "--output", "x", "discpower/v0.2.0"},
		} {
			if code, _, stderr := run(newFakes(), args...); code != cli.ExitUsage || stderr == "" {
				t.Errorf("%s: code %d, stderr %q", name, code, stderr)
			}
		}
	})

	t.Run("help succeeds and lists the verbs"+verifiesExits, func(t *testing.T) {
		code, stdout, _ := run(newFakes(), "help")
		if code != cli.ExitClean || !strings.Contains(stdout, "plan|preflight|build|verify-dist|formula|notes") {
			t.Fatalf("code %d, stdout %q", code, stdout)
		}
	})

	t.Run("a failure that is neither a refusal nor a bad dist exits with 1"+verifiesExits, func(t *testing.T) {
		f := newFakes()
		f.headErr = errors.New("no repository")
		if code, _, stderr := run(f, "preflight", "discpower/v0.2.0"); code != cli.ExitFailure || !strings.Contains(stderr, "no repository") {
			t.Fatalf("code %d, stderr %q", code, stderr)
		}
	})
}

func TestPlanVerb(t *testing.T) {
	t.Run("the plan prints one key=value line per output"+verifiesPlan, func(t *testing.T) {
		code, stdout, _ := run(newFakes(), "plan", "alignment/v1.2.3-rc.1")
		if code != cli.ExitClean {
			t.Fatalf("code %d", code)
		}
		for _, want := range []string{
			"module=alignment\n", "kind=binary+plugin\n", "binary=spec-proof-alignment\n", "semver=1.2.3-rc.1\n",
			"ships_binary=true\n", "ships_plugin=true\n", "ci_modules=format alignment\n",
			"certificate_identity=https://github.com/RachidChabane/spec-proof/.github/workflows/release.yml@refs/tags/alignment/v1.2.3-rc.1\n",
		} {
			if !strings.Contains(stdout, want) {
				t.Errorf("plan lacks %q:\n%s", want, stdout)
			}
		}
	})
}

func TestPreflightPolicy(t *testing.T) {
	releasable := func() *fakes {
		f := newFakes()
		f.goMod = domain.GoModFacts{FormatRequirement: "v0.1.0"}
		return f
	}

	t.Run("a module whose facts are in order passes a real pre-flight"+verifiesPolicy, func(t *testing.T) {
		code, stdout, _ := run(releasable(), "preflight", "ledger/v0.2.0")
		if code != cli.ExitClean || !strings.Contains(stdout, "ok (0 finding(s)") {
			t.Fatalf("code %d, stdout %q", code, stdout)
		}
	})

	t.Run("a real release is refused on a replace rule with exit 3"+verifiesPolicy, func(t *testing.T) {
		f := releasable()
		f.goMod.Replaces = []string{"format => ../format"}
		code, stdout, _ := run(f, "preflight", "ledger/v0.2.0")
		if code != cli.ExitRefused || !strings.Contains(stdout, "refused: go-mod-releasable") {
			t.Fatalf("code %d, stdout %q", code, stdout)
		}
	})

	t.Run("a dry run reports the replace rule as what a real release would refuse and goes on"+verifiesPolicy, func(t *testing.T) {
		f := releasable()
		f.goMod.Replaces = []string{"format => ../format"}
		code, stdout, _ := run(f, "preflight", "--dry-run", "ledger/v0.2.0")
		if code != cli.ExitClean || !strings.Contains(stdout, "a real release would be refused: go-mod-releasable") || strings.Contains(stdout, "\nrefused:") || strings.HasPrefix(stdout, "refused:") {
			t.Fatalf("code %d, stdout %q", code, stdout)
		}
	})

	t.Run("a dry run is still refused on a build that fails"+verifiesPolicy, func(t *testing.T) {
		f := releasable()
		f.buildResult = domain.Outcome{Ran: true, Failure: "does not compile"}
		code, stdout, _ := run(f, "preflight", "--dry-run", "ledger/v0.2.0")
		if code != cli.ExitRefused || !strings.Contains(stdout, "does not compile") {
			t.Fatalf("code %d, stdout %q", code, stdout)
		}
	})

	t.Run("a tag on another commit is refused in a dry run"+verifiesPolicy, func(t *testing.T) {
		f := releasable()
		f.tagCommit = "2222222222222222222222222222222222222222"
		if code, _, _ := run(f, "preflight", "--dry-run", "ledger/v0.2.0"); code != cli.ExitRefused {
			t.Fatalf("code %d", code)
		}
	})

	t.Run("a tag that does not exist is refused for real and noted in a dry run"+verifiesPolicy, func(t *testing.T) {
		f := releasable()
		f.tagCommit = ""
		if code, _, _ := run(f, "preflight", "ledger/v0.2.0"); code != cli.ExitRefused {
			t.Fatalf("real: code %d", code)
		}
		if code, stdout, _ := run(f, "preflight", "--dry-run", "ledger/v0.2.0"); code != cli.ExitClean || !strings.Contains(stdout, "does not exist") {
			t.Fatalf("dry run: code %d, stdout %q", code, stdout)
		}
	})

	t.Run("requiring a public repository refuses a private one for real and not in a dry run"+verifiesPolicy, func(t *testing.T) {
		args := []string{"preflight", "--require-public", "--visibility", "private"}
		if code, _, _ := run(releasable(), append(args, "ledger/v0.2.0")...); code != cli.ExitRefused {
			t.Fatalf("real: code %d", code)
		}
		if code, _, _ := run(releasable(), append(args[:1:1], "--dry-run", "--require-public", "--visibility", "private", "ledger/v0.2.0")...); code != cli.ExitClean {
			t.Fatalf("dry run: code %d", code)
		}
		if code, _, _ := run(releasable(), "preflight", "--require-public", "--visibility", "public", "ledger/v0.2.0"); code != cli.ExitClean {
			t.Fatalf("public: code %d", code)
		}
	})
}

func TestVerifyDistExit(t *testing.T) {
	t.Run("a dist directory with a finding exits with 4"+verifiesExits, func(t *testing.T) {
		f := newFakes()
		code, stdout, _ := run(f, "verify-dist", "--targets", "linux/amd64", "ledger/v0.2.0")
		if code != cli.ExitDistInvalid || !strings.Contains(stdout, "invalid: dist:") {
			t.Fatalf("code %d, stdout %q", code, stdout)
		}
	})

	t.Run("a dist directory that cannot be read exits with 1"+verifiesExits, func(t *testing.T) {
		f := newFakes()
		f.distErr = errors.New("no such directory")
		if code, _, stderr := run(f, "verify-dist", "ledger/v0.2.0"); code != cli.ExitFailure || !strings.Contains(stderr, "no such directory") {
			t.Fatalf("code %d, stderr %q", code, stderr)
		}
	})

	t.Run("a module that ships no binary has nothing to verify"+verifiesExits, func(t *testing.T) {
		if code, stdout, _ := run(newFakes(), "verify-dist", "format/v0.2.0"); code != cli.ExitClean || !strings.Contains(stdout, "ok") {
			t.Fatalf("code %d, stdout %q", code, stdout)
		}
	})
}

func TestFormulaAndNotesVerbs(t *testing.T) {
	t.Run("the notes verb writes the notes to the file named"+verifiesExits, func(t *testing.T) {
		f := newFakes()
		if code, _, stderr := run(f, "notes", "--repo", "/repo", "--out", "/out/notes.md", "ledger/v0.2.0"); code != cli.ExitClean {
			t.Fatalf("code %d, stderr %q", code, stderr)
		}
		if !strings.Contains(string(f.written["/out/notes.md"]), "## ledger 0.2.0") {
			t.Fatalf("written = %v", f.written)
		}
	})

	t.Run("the formula verb fails when the checksums cannot be read"+verifiesExits, func(t *testing.T) {
		if code, _, stderr := run(newFakes(), "formula", "ledger/v0.2.0"); code != cli.ExitFailure || stderr == "" {
			t.Fatalf("code %d, stderr %q", code, stderr)
		}
	})
}
