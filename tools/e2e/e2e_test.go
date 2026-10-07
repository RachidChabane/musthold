// Package e2e_test drives the built binaries of the ledger and verify modules as
// black boxes through the whole proof chain: golden statements, sign, ledger
// append, export, offline verification and the manifest Shallnot's gate reads.
package e2e_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
)

const (
	repositoryDir  = "../.."
	goldenDir      = "format/testdata/golden"
	ledgerBinary   = "spec-proof-ledger"
	verifyBinary   = "spec-proof-verify"
	keyVariable    = "SPEC_PROOF_ED25519_KEY"
	exitRejected   = 1
	directoryMode  = 0o755
	fileMode       = 0o644
	flippedBit     = 0x01
	verifiesChain  = " [verifies SP-219~1]"
	verifiesTamper = " [verifies SP-220~1]"
	verifiesReplay = " [verifies SP-221~1]"
	verifiesRunner = " [verifies SP-222~1]"
)

var goldenStatements = []struct{ file, module string }{
	{"alignment-valid.json", "alignment"},
	{"discriminating-power-valid.json", "discpower"},
	{"completeness-valid.json", "completeness"},
}

var (
	buildOnce sync.Once
	binaries  string
	buildErr  error
)

// build compiles both binaries once, throttled: one compiler job at a time.
func build(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		binaries, buildErr = os.MkdirTemp("", "spec-proof-e2e-bin-")
		if buildErr != nil {
			return
		}
		for module, binary := range map[string]string{"ledger": ledgerBinary, "verify": verifyBinary} {
			command := exec.Command("go", "build", "-C", filepath.Join(repositoryDir, module), "-p", "1", "-o", filepath.Join(binaries, binary), "./cmd/"+binary)
			command.Env = append(os.Environ(), "GOMAXPROCS=2")
			if output, err := command.CombinedOutput(); err != nil {
				buildErr = &buildError{module: module, output: string(output), cause: err}
				return
			}
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return binaries
}

type buildError struct {
	module, output string
	cause          error
}

func (e *buildError) Error() string {
	return "build " + e.module + ": " + e.cause.Error() + "\n" + e.output
}

func TestMain(m *testing.M) {
	code := m.Run()
	if binaries != "" {
		os.RemoveAll(binaries)
	}
	os.Exit(code)
}

// chain is one run of the proof chain in a temporary workspace.
type chain struct {
	t        *testing.T
	bin      string
	dir      string
	key      ed25519.PrivateKey
	keyPEM   string
	cache    string
	signed   string
	exported string
	spec     string // the .spec-proof directory
}

func newChain(t *testing.T) *chain {
	t.Helper()
	_, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	c := &chain{
		t: t, bin: build(t), dir: dir, key: private,
		keyPEM: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		cache:  filepath.Join(dir, "cache"), signed: filepath.Join(dir, "signed"),
		exported: filepath.Join(dir, "exported"), spec: filepath.Join(dir, ".spec-proof"),
	}
	c.writeGoldenCache()
	c.writePolicy(private.Public().(ed25519.PublicKey))
	return c
}

// writeGoldenCache places the valid golden statements in a module cache. A golden
// file ends with a newline that the statement does not have.
func (c *chain) writeGoldenCache() {
	c.t.Helper()
	for _, golden := range goldenStatements {
		raw, err := os.ReadFile(filepath.Join(repositoryDir, goldenDir, golden.file))
		if err != nil {
			c.t.Fatal(err)
		}
		statement := bytes.TrimSuffix(raw, []byte("\n"))
		admitted, err := format.Admit(statement)
		if err != nil {
			c.t.Fatalf("%s: %v", golden.file, err)
		}
		c.write(filepath.Join(c.cache, filepath.FromSlash(admitted.Location(golden.module).StatementPath())), statement)
	}
}

func (c *chain) writePolicy(public ed25519.PublicKey) {
	c.t.Helper()
	policy := "version: 1\ned25519:\n  - publicKey: " + base64.StdEncoding.EncodeToString(public) + "\n"
	c.write(filepath.Join(c.spec, "trust-policy.yaml"), []byte(policy))
}

func (c *chain) write(path string, content []byte) {
	c.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), directoryMode); err != nil {
		c.t.Fatal(err)
	}
	if err := os.WriteFile(path, content, fileMode); err != nil {
		c.t.Fatal(err)
	}
}

type outcome struct {
	code           int
	stdout, stderr string
}

// execute runs a binary with git isolated from the machine's configuration.
func (c *chain) execute(binary string, args ...string) outcome {
	c.t.Helper()
	command := exec.Command(filepath.Join(c.bin, binary), args...)
	command.Env = append(os.Environ(), keyVariable+"="+c.keyPEM, "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", "GOMAXPROCS=2")
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	code := 0
	if err != nil {
		exit, isExit := err.(*exec.ExitError)
		if !isExit {
			c.t.Fatalf("%s: %v", binary, err)
		}
		code = exit.ExitCode()
	}
	return outcome{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func (c *chain) mustExecute(binary string, args ...string) outcome {
	c.t.Helper()
	result := c.execute(binary, args...)
	if result.code != 0 {
		c.t.Fatalf("%s %v: exit %d\n%s", binary, args, result.code, result.stderr)
	}
	return result
}

func (c *chain) git(dir string, args ...string) string {
	c.t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	command.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	output, err := command.CombinedOutput()
	if err != nil {
		c.t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func (c *chain) newBareRepository() string {
	c.t.Helper()
	repository := c.t.TempDir()
	c.git(repository, "init", "--bare", "--quiet")
	return repository
}

func (c *chain) sign(out string) {
	c.t.Helper()
	c.mustExecute(ledgerBinary, "sign", "--identity", "ed25519", "--in", c.cache, "--out", out)
}

func (c *chain) verified() string { return filepath.Join(c.spec, "verified") }

func (c *chain) verify(exported string) outcome {
	c.t.Helper()
	return c.execute(verifyBinary, "--bundles", exported, "--offline", "--out", c.verified())
}

// manifestContract is the manifest exactly as Shallnot's gate reads it.
type manifestContract struct {
	VerifierVersion   *string `json:"verifierVersion"`
	Trust             *string `json:"trust"`
	TrustedRootDigest *string `json:"trustedRootDigest"`
	PolicyDigest      *string `json:"policyDigest"`
	Statements        []struct {
		Path   string `json:"path"`
		Digest string `json:"digest"`
	} `json:"statements"`
	Pending []struct {
		Module    string `json:"module"`
		Criterion string `json:"criterion"`
		Reason    string `json:"reason"`
	} `json:"pending"`
}

func (c *chain) readManifest() (manifestContract, []byte) {
	c.t.Helper()
	raw, err := os.ReadFile(filepath.Join(c.verified(), "manifest.json"))
	if err != nil {
		c.t.Fatal(err)
	}
	var manifest manifestContract
	if err := json.Unmarshal(raw, &manifest); err != nil {
		c.t.Fatal(err)
	}
	return manifest, raw
}

func TestProofChain(t *testing.T) {
	c := newChain(t)
	repository := c.newBareRepository()

	c.sign(c.signed)
	c.mustExecute(ledgerBinary, "ledger", "append", "--repo", repository, "--signed", c.signed)

	t.Run("the ledger of the appended proofs is append-only and content-addressed"+verifiesChain, func(t *testing.T) {
		result := c.mustExecute(ledgerBinary, "ledger", "audit", "--repo", repository)
		if !strings.Contains(result.stdout, "append-only and content-addressed") {
			t.Fatalf("audit said %q", result.stdout)
		}
		if branch := c.git(repository, "for-each-ref", "--format=%(refname)"); branch != "refs/heads/spec-proof/ledger" {
			t.Fatalf("refs %q", branch)
		}
	})

	c.mustExecute(ledgerBinary, "ledger", "export", "--repo", repository, "--to", c.exported)
	c.mustExecute(verifyBinary, "--bundles", c.exported, "--offline", "--out", c.verified())

	t.Run("the bare statements are the golden ones and the manifest is the contract of the gate"+verifiesChain, func(t *testing.T) {
		manifest, raw := c.readManifest()
		if manifest.VerifierVersion == nil || *manifest.VerifierVersion == "" || manifest.Trust == nil || *manifest.Trust != "signed" {
			t.Fatalf("manifest %s", raw)
		}
		if manifest.PolicyDigest == nil || !format.IsSHA256Hex(*manifest.PolicyDigest) || manifest.TrustedRootDigest == nil || !format.IsSHA256Hex(*manifest.TrustedRootDigest) {
			t.Fatalf("digests of a signed manifest: %s", raw)
		}
		policy, err := os.ReadFile(filepath.Join(c.spec, "trust-policy.yaml"))
		if err != nil || *manifest.PolicyDigest != format.SHA256Hex(policy) {
			t.Fatalf("policy digest %s is not the digest of the policy file", *manifest.PolicyDigest)
		}
		if len(manifest.Statements) != len(goldenStatements) || manifest.Pending == nil || len(manifest.Pending) != 0 {
			t.Fatalf("manifest %s", raw)
		}
		paths := make([]string, 0, len(manifest.Statements))
		for _, entry := range manifest.Statements {
			paths = append(paths, entry.Path)
			content, err := os.ReadFile(filepath.Join(c.verified(), filepath.FromSlash(entry.Path)))
			if err != nil || entry.Digest != format.SHA256Hex(content) {
				t.Fatalf("%s: the digest is not the digest of the exact file bytes (%v)", entry.Path, err)
			}
			cached, err := os.ReadFile(filepath.Join(c.cache, filepath.FromSlash(entry.Path)))
			if err != nil || !bytes.Equal(cached, content) {
				t.Fatalf("%s: the verified statement is not the golden one (%v)", entry.Path, err)
			}
		}
		if !slices.IsSorted(paths) {
			t.Fatalf("statements are not sorted by path: %v", paths)
		}
		if canonical, err := format.CanonicalizeJSON(raw); err != nil || !bytes.Equal(canonical, raw) {
			t.Fatalf("manifest is not RFC 8785 canonical: %v", err)
		}
	})

	t.Run("a byte changed in any exported bundle rejects the run and keeps the earlier verified directory"+verifiesTamper, func(t *testing.T) {
		_, before := c.readManifest()
		var bundles []string
		err := filepath.WalkDir(c.exported, func(path string, entry os.DirEntry, err error) error {
			if err == nil && !entry.IsDir() && strings.HasSuffix(path, format.BundleSuffix) {
				bundles = append(bundles, path)
			}
			return err
		})
		if err != nil || len(bundles) != len(goldenStatements) {
			t.Fatalf("bundles %v: %v", bundles, err)
		}
		for _, path := range bundles {
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, position := range []int{len(original) / 3, 2 * len(original) / 3} {
				tampered := bytes.Clone(original)
				tampered[position] ^= flippedBit
				c.write(path, tampered)
				if result := c.verify(c.exported); result.code == 0 {
					t.Errorf("%s byte %d: a tampered bundle verified", filepath.Base(path), position)
				}
			}
			c.write(path, original)
		}
		if _, after := c.readManifest(); !bytes.Equal(before, after) {
			t.Fatal("the verified directory changed after rejected runs")
		}
	})

	t.Run("a payload changed under a valid frame is rejected with the exit code of a rejection"+verifiesTamper, func(t *testing.T) {
		path := filepath.Join(c.exported, "proofs")
		var bundle string
		_ = filepath.WalkDir(path, func(p string, entry os.DirEntry, err error) error {
			if err == nil && !entry.IsDir() && bundle == "" {
				bundle = p
			}
			return err
		})
		original, err := os.ReadFile(bundle)
		if err != nil {
			t.Fatal(err)
		}
		var envelope format.Envelope
		if err := json.Unmarshal(original, &envelope); err != nil {
			t.Fatal(err)
		}
		payload, _ := envelope.StatementBytes()
		payload[len(payload)/2] ^= flippedBit
		envelope.Payload = base64.StdEncoding.EncodeToString(payload)
		tampered, _ := envelope.Marshal()
		c.write(bundle, tampered)
		defer c.write(bundle, original)
		if result := c.verify(c.exported); result.code != exitRejected {
			t.Fatalf("exit %d: %s", result.code, result.stderr)
		}
	})

	t.Run("verifying the same exported ledger again gives byte-identical output"+verifiesReplay, func(t *testing.T) {
		_, first := c.readManifest()
		firstFiles := snapshot(t, c.verified())
		c.mustExecute(verifyBinary, "--bundles", c.exported, "--offline", "--out", c.verified())
		_, second := c.readManifest()
		if !bytes.Equal(first, second) {
			t.Fatalf("manifests differ:\n%s\n%s", first, second)
		}
		if secondFiles := snapshot(t, c.verified()); !maps.Equal(firstFiles, secondFiles) {
			t.Fatal("the verified statements differ")
		}
	})

	t.Run("signing the same statements twice gives byte-identical bundles"+verifiesReplay, func(t *testing.T) {
		again := filepath.Join(t.TempDir(), "signed")
		c.sign(again)
		if !maps.Equal(snapshot(t, c.signed), snapshot(t, again)) {
			t.Fatal("two signatures of the same statements differ")
		}
	})
}

// snapshot reads every file below root into a map from relative path to content.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := os.ReadFile(path)
		relative, _ := filepath.Rel(root, path)
		files[filepath.ToSlash(relative)] = string(content)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestTwoRunnersOneLedger(t *testing.T) {
	c := newChain(t)
	origin := c.newBareRepository()
	first, second := c.newBareRepository(), c.newBareRepository()
	c.git(first, "remote", "add", "origin", origin)
	c.git(second, "remote", "add", "origin", origin)

	t.Run("two runners append to one remote ledger and every proof verifies from the export"+verifiesRunner, func(t *testing.T) {
		firstCache := filepath.Join(t.TempDir(), "cache")
		secondCache := filepath.Join(t.TempDir(), "cache")
		c.splitCache(firstCache, secondCache)

		for _, runner := range []struct{ repository, cache string }{{first, firstCache}, {second, secondCache}} {
			signed := filepath.Join(t.TempDir(), "signed")
			c.mustExecute(ledgerBinary, "sign", "--identity", "ed25519", "--in", runner.cache, "--out", signed)
			c.mustExecute(ledgerBinary, "ledger", "append", "--repo", runner.repository, "--remote", "origin", "--signed", signed)
		}
		if count := c.git(origin, "rev-list", "--count", "refs/heads/spec-proof/ledger"); count != "2" {
			t.Fatalf("the remote ledger has %s commits, want 2", count)
		}
		c.mustExecute(ledgerBinary, "ledger", "audit", "--repo", origin)
		c.mustExecute(ledgerBinary, "ledger", "export", "--repo", origin, "--to", c.exported)
		c.mustExecute(verifyBinary, "--bundles", c.exported, "--offline", "--out", c.verified())
		manifest, raw := c.readManifest()
		if len(manifest.Statements) != len(goldenStatements) {
			t.Fatalf("manifest %s", raw)
		}
	})
}

// splitCache moves the statements of the golden cache into two caches, the first
// holding the first statement and the second the others.
func (c *chain) splitCache(first, second string) {
	c.t.Helper()
	statements := snapshot(c.t, c.cache)
	paths := make([]string, 0, len(statements))
	for path := range statements {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	for index, path := range paths {
		target := second
		if index == 0 {
			target = first
		}
		c.write(filepath.Join(target, filepath.FromSlash(path)), []byte(statements[path]))
	}
}
