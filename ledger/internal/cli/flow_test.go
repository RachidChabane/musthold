package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/ledger/internal/adapters/signing/ed25519signer"
	"github.com/RachidChabane/spec-proof/ledger/internal/cli"
	"github.com/RachidChabane/spec-proof/ledger/internal/testkit"
)

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Date(2026, time.April, 1, 8, 0, 0, 0, time.UTC) }

type runResult struct {
	code           int
	stdout, stderr string
}

type machine struct {
	t        *testing.T
	identity testkit.Ed25519Identity
	env      map[string]string
}

func newMachine(t *testing.T) *machine {
	t.Helper()
	identity := testkit.NewEd25519Identity(t, 5)
	return &machine{t: t, identity: identity, env: map[string]string{ed25519signer.KeyEnvironmentVariable: string(identity.PEM)}}
}

func (m *machine) run(args ...string) runResult {
	m.t.Helper()
	environment := cli.Environment{
		LookupEnv: func(name string) (string, bool) {
			value, found := m.env[name]
			return value, found
		},
		Clock:          fixedClock{},
		GitEnvironment: testkit.GitEnvironment(m.t),
	}
	var stdout, stderr bytes.Buffer
	code := cli.Run(environment, binary, args, &stdout, &stderr)
	return runResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func (m *machine) mustRun(args ...string) runResult {
	m.t.Helper()
	result := m.run(args...)
	if result.code != cli.ExitClean {
		m.t.Fatalf("%v: exit %d\n%s", args, result.code, result.stderr)
	}
	return result
}

// signedDirectory caches the valid statements, signs them and returns the output
// directory of `sign`.
func (m *machine) signedDirectory() string {
	m.t.Helper()
	cache, signed := m.t.TempDir(), m.t.TempDir()
	testkit.WriteStatements(m.t, cache, testkit.ValidStatements(m.t))
	m.mustRun("sign", "--identity", "ed25519", "--in", cache, "--out", signed)
	return signed
}

const (
	verifiesSignFlow  = " [verifies SP-152~1]"
	verifiesAppendCLI = " [verifies SP-154~1]"
	verifiesExported  = " [verifies SP-157~1]"
	verifiesAudited   = " [verifies SP-158~1]"
	verifiesSeedCLI   = " [verifies SP-159~1]"
	verifiesExitCodes = " [verifies SP-160~1]"
	verifiesSeedFiles = " [verifies SP-155~1]"
	verifiesRemoteCLI = " [verifies SP-156~1]"
	verifiesSignGate  = " [verifies SP-151~1]"
)

func TestSignThenAppendExportAudit(t *testing.T) {
	m := newMachine(t)
	repository := testkit.NewBareRepository(t)
	signed := m.signedDirectory()

	t.Run("signed proofs are appended to the ledger branch of a bare repository"+verifiesAppendCLI, func(t *testing.T) {
		result := m.mustRun("ledger", "append", "--repo", repository, "--signed", signed)
		if !strings.Contains(result.stdout, "3 files added, 0 already present") {
			t.Fatalf("stdout %q", result.stdout)
		}
		listing := testkit.Git(t, repository, "ls-tree", "-r", "--name-only", "refs/heads/"+format.LedgerBranch)
		lines := strings.Split(listing, "\n")
		if len(lines) != 3 {
			t.Fatalf("ledger files %v", lines)
		}
		for _, line := range lines {
			if _, ok := format.ParseBundlePath(line); !ok {
				t.Errorf("%s is not a proof path", line)
			}
		}
		if parents := testkit.Git(t, repository, "rev-list", "--parents", "-n", "1", "refs/heads/"+format.LedgerBranch); strings.Count(parents, " ") != 0 {
			t.Fatalf("the first commit has a parent: %s", parents)
		}
	})

	t.Run("appending the same proofs again changes nothing"+verifiesAppendCLI, func(t *testing.T) {
		before := testkit.Git(t, repository, "rev-parse", "refs/heads/"+format.LedgerBranch)
		result := m.mustRun("ledger", "append", "--repo", repository, "--signed", signed)
		if !strings.Contains(result.stdout, "0 files added, 3 already present") {
			t.Fatalf("stdout %q", result.stdout)
		}
		if after := testkit.Git(t, repository, "rev-parse", "refs/heads/"+format.LedgerBranch); after != before {
			t.Fatal("the ledger moved without an addition")
		}
	})

	t.Run("a path that would get other bytes is refused with exit 1 and the ledger does not move"+verifiesAppendCLI, func(t *testing.T) {
		before := testkit.Git(t, repository, "rev-parse", "refs/heads/"+format.LedgerBranch)
		other := newMachine(t)
		other.identity = testkit.NewEd25519Identity(t, 9)
		other.env[ed25519signer.KeyEnvironmentVariable] = string(other.identity.PEM)
		resigned := other.signedDirectory()
		result := m.run("ledger", "append", "--repo", repository, "--signed", resigned)
		if result.code != cli.ExitBlocked || !strings.Contains(result.stderr, "never rewrites") {
			t.Fatalf("exit %d stderr %q", result.code, result.stderr)
		}
		if after := testkit.Git(t, repository, "rev-parse", "refs/heads/"+format.LedgerBranch); after != before {
			t.Fatal("the ledger moved")
		}
	})

	t.Run("mutants, a seed and the trust snapshot are appended in the same commit"+verifiesSeedFiles, func(t *testing.T) {
		mutants, trust := t.TempDir(), t.TempDir()
		cacheKey := strings.Repeat("ab", 32)
		testkit.WriteFile(t, filepath.Join(mutants, cacheKey+".age"), []byte("age-encryption.org/v1\nciphertext"))
		testkit.WriteFile(t, filepath.Join(trust, "1.root.json"), []byte(`{"signed":{}}`))
		head := strings.Repeat("12", 20)
		result := m.mustRun("ledger", "append", "--repo", repository, "--mutants", mutants, "--trust", trust,
			"--head-sha", head, "--run-id", "100")
		if !strings.Contains(result.stdout, "3 files added") {
			t.Fatalf("stdout %q", result.stdout)
		}
		listing := testkit.Git(t, repository, "ls-tree", "-r", "--name-only", "refs/heads/"+format.LedgerBranch)
		for _, want := range []string{"mutants/" + cacheKey + ".age", "trust/1.root.json", "seeds/" + head + ".json"} {
			if !strings.Contains(listing, want) {
				t.Errorf("ledger lacks %s", want)
			}
		}
	})

	t.Run("the seed of a head commit stays the one its first run recorded"+verifiesSeedCLI, func(t *testing.T) {
		head := strings.Repeat("12", 20)
		first := m.mustRun("ledger", "seed", "--repo", repository, "--head-sha", head, "--run-id", "999").stdout
		again := m.mustRun("ledger", "seed", "--repo", repository, "--head-sha", head, "--run-id", "100").stdout
		fresh := m.mustRun("ledger", "seed", "--repo", repository, "--head-sha", strings.Repeat("34", 20), "--run-id", "999").stdout
		if first != again {
			t.Fatalf("recorded seed %q then %q: a re-run shopped for another seed", first, again)
		}
		if fresh == first || len(strings.TrimSpace(fresh)) != 64 {
			t.Fatalf("seed of an unseen head commit %q", fresh)
		}
		reappend := m.mustRun("ledger", "append", "--repo", repository, "--head-sha", head, "--run-id", "999")
		if !strings.Contains(reappend.stdout, "0 files added, 1 already present") {
			t.Fatalf("stdout %q", reappend.stdout)
		}
	})

	t.Run("the ledger is audited clean"+verifiesAudited, func(t *testing.T) {
		result := m.mustRun("ledger", "audit", "--repo", repository)
		if !strings.Contains(result.stdout, "append-only and content-addressed: 2 commits, 6 files") {
			t.Fatalf("stdout %q", result.stdout)
		}
	})

	t.Run("the proofs and the trust snapshot are exported and nothing else"+verifiesExported, func(t *testing.T) {
		destination := filepath.Join(t.TempDir(), "export")
		result := m.mustRun("ledger", "export", "--repo", repository, "--to", destination)
		if !strings.Contains(result.stdout, "exported 3 proofs and 1 trust files") {
			t.Fatalf("stdout %q", result.stdout)
		}
		for _, statement := range testkit.ValidStatements(t) {
			got, err := os.ReadFile(filepath.Join(destination, filepath.FromSlash(statement.Location.BundlePath())))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(got), `"payloadType":"application/vnd.in-toto+json"`) {
				t.Fatalf("exported file is not a DSSE envelope: %s", got)
			}
		}
		if _, err := os.Stat(filepath.Join(destination, "trust", "1.root.json")); err != nil {
			t.Fatal(err)
		}
		for _, absent := range []string{"mutants", "seeds"} {
			if _, err := os.Stat(filepath.Join(destination, absent)); err == nil {
				t.Errorf("%s was exported", absent)
			}
		}
		if again := m.run("ledger", "export", "--repo", repository, "--to", destination); again.code != cli.ExitBlocked {
			t.Fatalf("export into a directory that holds files: exit %d", again.code)
		}
	})

	t.Run("a ledger whose history was rewritten is reported by the audit with exit 1"+verifiesAudited, func(t *testing.T) {
		tampered := testkit.NewBareRepository(t)
		m.mustRun("ledger", "append", "--repo", tampered, "--signed", signed)
		tip := testkit.Git(t, tampered, "rev-parse", "refs/heads/"+format.LedgerBranch)
		empty := testkit.GitInput(t, tampered, "", "mktree")
		deleting := testkit.Git(t, tampered, "commit-tree", empty, "-p", tip, "-m", "delete everything")
		testkit.Git(t, tampered, "update-ref", "refs/heads/"+format.LedgerBranch, deleting)
		result := m.run("ledger", "audit", "--repo", tampered)
		if result.code != cli.ExitBlocked || !strings.Contains(result.stdout, "no_rewrite") {
			t.Fatalf("exit %d stdout %q stderr %q", result.code, result.stdout, result.stderr)
		}
	})

	t.Run("a ledger with a file that is not what its name says is reported"+verifiesAudited, func(t *testing.T) {
		forged := testkit.NewBareRepository(t)
		blob := testkit.GitInput(t, forged, "plain text mutant", "hash-object", "-w", "--stdin")
		mutants := testkit.GitInput(t, forged, "100644 blob "+blob+"\t"+strings.Repeat("cd", 32)+".age\n", "mktree")
		root := testkit.GitInput(t, forged, "040000 tree "+mutants+"\tmutants\n", "mktree")
		commit := testkit.Git(t, forged, "commit-tree", root, "-m", "plain mutant")
		testkit.Git(t, forged, "update-ref", "refs/heads/"+format.LedgerBranch, commit)
		result := m.run("ledger", "audit", "--repo", forged)
		if result.code != cli.ExitBlocked || !strings.Contains(result.stdout, "content_addressed") {
			t.Fatalf("exit %d stdout %q", result.code, result.stdout)
		}
	})
}

func TestSignCommand(t *testing.T) {
	t.Run("the signed bundles carry the exact statement bytes in a DSSE envelope"+verifiesSignFlow, func(t *testing.T) {
		m := newMachine(t)
		signed := m.signedDirectory()
		for _, statement := range testkit.ValidStatements(t) {
			bundle, err := os.ReadFile(filepath.Join(signed, filepath.FromSlash(statement.Location.BundlePath())))
			if err != nil {
				t.Fatal(err)
			}
			envelope, err := format.DecodeEnvelope(bundle)
			if err != nil {
				t.Fatal(err)
			}
			payload, err := envelope.StatementBytes()
			if err != nil || !bytes.Equal(payload, statement.Bytes) {
				t.Fatalf("payload differs: %v", err)
			}
		}
	})

	t.Run("the key can come from an owner-only file"+verifiesSignFlow, func(t *testing.T) {
		m := newMachine(t)
		keyFile := m.identity.WriteKeyFile(t, t.TempDir())
		delete(m.env, ed25519signer.KeyEnvironmentVariable)
		cache, signed := t.TempDir(), t.TempDir()
		testkit.WriteStatements(t, cache, testkit.ValidStatements(t))
		m.mustRun("sign", "--identity", "ed25519", "--key-file", keyFile, "--in", cache, "--out", signed)
	})

	t.Run("no key is a tool failure"+verifiesSignFlow, func(t *testing.T) {
		m := newMachine(t)
		delete(m.env, ed25519signer.KeyEnvironmentVariable)
		result := m.run("sign", "--identity", "ed25519", "--in", t.TempDir(), "--out", t.TempDir())
		if result.code != cli.ExitToolFailure || !strings.Contains(result.stderr, ed25519signer.KeyEnvironmentVariable) {
			t.Fatalf("exit %d stderr %q", result.code, result.stderr)
		}
	})

	t.Run("the sigstore identity needs an identity token"+verifiesSignFlow, func(t *testing.T) {
		m := newMachine(t)
		result := m.run("sign", "--identity", "sigstore", "--in", t.TempDir(), "--out", t.TempDir())
		if result.code != cli.ExitToolFailure || !strings.Contains(result.stderr, "OIDC identity token") {
			t.Fatalf("exit %d stderr %q", result.code, result.stderr)
		}
	})

	t.Run("a statement produced locally is refused with exit 1 and nothing is written"+verifiesSignGate, func(t *testing.T) {
		m := newMachine(t)
		cache, signed := t.TempDir(), t.TempDir()
		statements := testkit.ValidStatements(t)
		statements[0].Bytes = bytes.Replace(statements[0].Bytes, []byte(`"producedBy":"ci"`), []byte(`"producedBy":"local"`), 1)
		testkit.WriteStatements(t, cache, statements)
		result := m.run("sign", "--identity", "ed25519", "--in", cache, "--out", signed)
		if result.code != cli.ExitBlocked {
			t.Fatalf("exit %d stderr %q", result.code, result.stderr)
		}
		if entries, _ := os.ReadDir(signed); len(entries) != 0 {
			t.Fatalf("%d entries written", len(entries))
		}
	})

	t.Run("a digest manifest that disagrees refuses the run"+verifiesSignGate, func(t *testing.T) {
		m := newMachine(t)
		cache, signed := t.TempDir(), t.TempDir()
		statements := testkit.ValidStatements(t)
		testkit.WriteStatements(t, cache, statements)
		var lines []string
		for _, statement := range statements {
			lines = append(lines, format.SHA256Hex(statement.Bytes)+"  "+statement.Location.StatementPath())
		}
		manifest := filepath.Join(t.TempDir(), "digests")
		testkit.WriteFile(t, manifest, []byte(strings.Join(lines, "\n")+"\n"))
		m.mustRun("sign", "--identity", "ed25519", "--digests", manifest, "--in", cache, "--out", signed)

		testkit.WriteFile(t, filepath.Join(cache, filepath.FromSlash(statements[0].Location.StatementPath())), append(bytes.Clone(statements[0].Bytes), ' '))
		result := m.run("sign", "--identity", "ed25519", "--digests", manifest, "--in", cache, "--out", t.TempDir())
		if result.code != cli.ExitBlocked || !strings.Contains(result.stderr, "digest manifest") {
			t.Fatalf("exit %d stderr %q", result.code, result.stderr)
		}
	})
}

func TestRemoteFlow(t *testing.T) {
	m := newMachine(t)
	signed := m.signedDirectory()
	local, origin := testkit.NewBareRepository(t), testkit.NewBareRepository(t)
	testkit.Git(t, local, "remote", "add", "origin", origin)

	t.Run("an append with a remote publishes the ledger"+verifiesRemoteCLI, func(t *testing.T) {
		m.mustRun("ledger", "append", "--repo", local, "--remote", "origin", "--signed", signed)
		if got, want := testkit.Git(t, origin, "rev-parse", "refs/heads/"+format.LedgerBranch), testkit.Git(t, local, "rev-parse", "refs/heads/"+format.LedgerBranch); got != want {
			t.Fatalf("origin at %s, local at %s", got, want)
		}
	})

	t.Run("a second runner starts from the remote ledger and fast-forwards it"+verifiesRemoteCLI, func(t *testing.T) {
		second := testkit.NewBareRepository(t)
		testkit.Git(t, second, "remote", "add", "origin", origin)
		mutants := t.TempDir()
		testkit.WriteFile(t, filepath.Join(mutants, strings.Repeat("ef", 32)+".age"), []byte("age-encryption.org/v1\nx"))
		m.mustRun("ledger", "append", "--repo", second, "--remote", "origin", "--mutants", mutants)
		if count := testkit.Git(t, origin, "rev-list", "--count", "refs/heads/"+format.LedgerBranch); count != "2" {
			t.Fatalf("origin has %s commits, want 2", count)
		}
		m.mustRun("ledger", "audit", "--repo", origin)
	})
}

func TestExitCodes(t *testing.T) {
	m := newMachine(t)
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"an unknown command", []string{"frobnicate"}, cli.ExitToolFailure},
		{"ledger without subcommand", []string{"ledger"}, cli.ExitToolFailure},
		{"an unknown ledger subcommand", []string{"ledger", "rewrite"}, cli.ExitToolFailure},
		{"sign without identity", []string{"sign", "--in", "a", "--out", "b"}, cli.ExitToolFailure},
		{"sign with an unknown identity", []string{"sign", "--identity", "pgp", "--in", "a", "--out", "b"}, cli.ExitToolFailure},
		{"sign with an unknown flag", []string{"sign", "--bogus"}, cli.ExitToolFailure},
		{"sign with a stray argument", []string{"sign", "extra"}, cli.ExitToolFailure},
		{"append with nothing to append", []string{"ledger", "append"}, cli.ExitToolFailure},
		{"append with a head commit and no run", []string{"ledger", "append", "--head-sha", strings.Repeat("a", 40)}, cli.ExitToolFailure},
		{"export without destination", []string{"ledger", "export"}, cli.ExitToolFailure},
		{"seed without run", []string{"ledger", "seed", "--head-sha", strings.Repeat("a", 40)}, cli.ExitToolFailure},
	}
	for _, tt := range cases {
		t.Run(tt.name+" exits "+string(rune('0'+tt.want))+verifiesExitCodes, func(t *testing.T) {
			result := m.run(tt.args...)
			if result.code != tt.want || result.stderr == "" {
				t.Fatalf("exit %d stderr %q", result.code, result.stderr)
			}
		})
	}

	t.Run("a repository without a ledger branch is refused on export and audit with exit 1"+verifiesExitCodes, func(t *testing.T) {
		empty := testkit.NewBareRepository(t)
		for _, args := range [][]string{{"ledger", "export", "--repo", empty, "--to", filepath.Join(t.TempDir(), "out")}, {"ledger", "audit", "--repo", empty}} {
			if result := m.run(args...); result.code != cli.ExitBlocked {
				t.Errorf("%v: exit %d stderr %q", args, result.code, result.stderr)
			}
		}
	})

	t.Run("a directory that is not a repository is a tool failure with exit 2"+verifiesExitCodes, func(t *testing.T) {
		if result := m.run("ledger", "audit", "--repo", t.TempDir()); result.code != cli.ExitToolFailure {
			t.Fatalf("exit %d stderr %q", result.code, result.stderr)
		}
	})

	t.Run("a command line that does not parse prints the usage"+verifiesExitCodes, func(t *testing.T) {
		result := m.run("frobnicate")
		if !strings.Contains(result.stderr, "usage:") || !strings.Contains(result.stderr, "ledger append") {
			t.Fatalf("stderr %q", result.stderr)
		}
	})
}
