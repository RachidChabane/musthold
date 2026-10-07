package cli_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/verify/internal/cli"
	"github.com/RachidChabane/spec-proof/verify/internal/testkit"
	"github.com/RachidChabane/spec-proof/verify/internal/version"
)

const (
	verifiesSignedRun = " [verifies SP-164~1]"
	verifiesLocalRun  = " [verifies SP-165~1]"
	verifiesManifest  = " [verifies SP-166~1]"
	verifiesPending   = " [verifies SP-167~1]"
	verifiesReplay    = " [verifies SP-168~1]"
	verifiesCommand   = " [verifies SP-169~1]"
)

type result struct {
	code           int
	stdout, stderr string
}

func run(args ...string) result {
	var stdout, stderr bytes.Buffer
	code := cli.Main(binary, args, &stdout, &stderr)
	return result{code, stdout.String(), stderr.String()}
}

// manifestView is the manifest as Shallnot's gate reads it.
type manifestView struct {
	VerifierVersion   string `json:"verifierVersion"`
	Trust             string `json:"trust"`
	TrustedRootDigest string `json:"trustedRootDigest"`
	PolicyDigest      string `json:"policyDigest"`
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

func readManifest(t *testing.T, verified string) (manifestView, []byte) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(verified, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var view manifestView
	if err := json.Unmarshal(raw, &view); err != nil {
		t.Fatal(err)
	}
	return view, raw
}

func signedSetup(t *testing.T) (layout testkit.Layout, bundles string, policy []byte, statements []testkit.Statement) {
	t.Helper()
	identity := testkit.NewIdentity(1)
	statements = testkit.ValidStatements(t)
	layout = testkit.NewLayout(t)
	policy = testkit.PolicyYAML(identity)
	testkit.WriteFile(t, layout.Policy, policy)
	return layout, testkit.ExportBundles(t, identity, statements), policy, statements
}

func TestSignedRun(t *testing.T) {
	layout, bundles, policy, statements := signedSetup(t)

	t.Run("signed bundles are verified into bare statements and a manifest of the contract"+verifiesManifest, func(t *testing.T) {
		outcome := run("--bundles", bundles, "--offline", "--out", layout.Verified)
		if outcome.code != cli.ExitClean {
			t.Fatalf("exit %d: %s", outcome.code, outcome.stderr)
		}
		manifest, raw := readManifest(t, layout.Verified)
		if manifest.VerifierVersion != version.Version || manifest.Trust != "signed" {
			t.Fatalf("manifest %+v", manifest)
		}
		if manifest.PolicyDigest != format.SHA256Hex(policy) {
			t.Fatalf("policy digest %s", manifest.PolicyDigest)
		}
		if !format.IsSHA256Hex(manifest.TrustedRootDigest) {
			t.Fatalf("trusted root digest %q", manifest.TrustedRootDigest)
		}
		if len(manifest.Statements) != 3 {
			t.Fatalf("statements %+v", manifest.Statements)
		}
		for index, entry := range manifest.Statements {
			content, err := os.ReadFile(filepath.Join(layout.Verified, filepath.FromSlash(entry.Path)))
			if err != nil || entry.Digest != format.SHA256Hex(content) {
				t.Fatalf("%s: digest %s, file digest %s (%v)", entry.Path, entry.Digest, format.SHA256Hex(content), err)
			}
			if index > 0 && manifest.Statements[index-1].Path >= entry.Path {
				t.Fatalf("statements are not sorted by path: %v", manifest.Statements)
			}
			if _, ok := format.ParseStatementPath(entry.Path); !ok {
				t.Fatalf("%s is not a statement path", entry.Path)
			}
		}
		if canonical, err := format.CanonicalizeJSON(raw); err != nil || !bytes.Equal(canonical, raw) {
			t.Fatalf("manifest is not canonical JSON: %v", err)
		}
		for _, statement := range statements {
			stored, err := os.ReadFile(filepath.Join(layout.Verified, filepath.FromSlash(statement.Location.StatementPath())))
			if err != nil || !bytes.Equal(stored, statement.Bytes) {
				t.Fatalf("bare statement %s differs: %v", statement.Location.StatementPath(), err)
			}
		}
	})

	t.Run("replaying the verification gives a byte-identical manifest"+verifiesReplay, func(t *testing.T) {
		_, first := readManifest(t, layout.Verified)
		again := filepath.Join(t.TempDir(), ".spec-proof")
		testkit.WriteFile(t, filepath.Join(again, "trust-policy.yaml"), policy)
		if outcome := run("--bundles", bundles, "--offline", "--out", filepath.Join(again, "verified")); outcome.code != cli.ExitClean {
			t.Fatalf("exit %d: %s", outcome.code, outcome.stderr)
		}
		_, second := readManifest(t, filepath.Join(again, "verified"))
		if !bytes.Equal(first, second) {
			t.Fatalf("manifests differ:\n%s\n%s", first, second)
		}
		if outcome := run("--bundles", bundles, "--offline", "--out", layout.Verified); outcome.code != cli.ExitClean {
			t.Fatalf("second run in place: exit %d: %s", outcome.code, outcome.stderr)
		}
		if _, inPlace := readManifest(t, layout.Verified); !bytes.Equal(first, inPlace) {
			t.Fatal("a second run in place changed the manifest")
		}
	})

	t.Run("a byte flipped in any bundle rejects the run and keeps the verified directory"+verifiesSignedRun, func(t *testing.T) {
		_, before := readManifest(t, layout.Verified)
		for _, statement := range statements {
			path := filepath.Join(bundles, filepath.FromSlash(statement.Location.BundlePath()))
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, position := range []int{len(original) / 4, len(original) / 2, len(original) - 5} {
				tampered := bytes.Clone(original)
				tampered[position] ^= 0x01
				testkit.WriteFile(t, path, tampered)
				outcome := run("--bundles", bundles, "--offline", "--out", layout.Verified)
				if outcome.code != cli.ExitRejected && outcome.code != cli.ExitToolFailure {
					t.Errorf("%s byte %d: exit %d", statement.Location.BundlePath(), position, outcome.code)
				}
				if outcome.code == cli.ExitClean {
					t.Errorf("a tampered bundle verified")
				}
			}
			testkit.WriteFile(t, path, original)
		}
		if _, after := readManifest(t, layout.Verified); !bytes.Equal(before, after) {
			t.Fatal("the verified directory changed after rejected runs")
		}
	})

	t.Run("a statement byte changed inside a validly framed envelope is rejected with exit 1"+verifiesSignedRun, func(t *testing.T) {
		statement := statements[0]
		path := filepath.Join(bundles, filepath.FromSlash(statement.Location.BundlePath()))
		original, _ := os.ReadFile(path)
		var envelope format.Envelope
		if err := json.Unmarshal(original, &envelope); err != nil {
			t.Fatal(err)
		}
		payload, _ := envelope.StatementBytes()
		payload[10] ^= 0x01
		envelope.Payload = base64.StdEncoding.EncodeToString(payload)
		tampered, _ := envelope.Marshal()
		testkit.WriteFile(t, path, tampered)
		defer testkit.WriteFile(t, path, original)
		outcome := run("--bundles", bundles, "--offline", "--out", layout.Verified)
		if outcome.code != cli.ExitRejected || !strings.Contains(outcome.stderr, "rejected") {
			t.Fatalf("exit %d: %s", outcome.code, outcome.stderr)
		}
	})

	t.Run("a bundle signed by a key the policy does not pin is rejected"+verifiesSignedRun, func(t *testing.T) {
		stranger := testkit.ExportBundles(t, testkit.NewIdentity(9), statements[:1])
		other := testkit.NewLayout(t)
		testkit.WriteFile(t, other.Policy, policy)
		if outcome := run("--bundles", stranger, "--offline", "--out", other.Verified); outcome.code != cli.ExitRejected {
			t.Fatalf("exit %d: %s", outcome.code, outcome.stderr)
		}
		if _, err := os.Stat(other.Verified); err == nil {
			t.Fatal("a verified directory was written")
		}
	})

	t.Run("a missing trust policy is a tool failure"+verifiesSignedRun, func(t *testing.T) {
		other := testkit.NewLayout(t)
		outcome := run("--bundles", bundles, "--offline", "--out", other.Verified)
		if outcome.code != cli.ExitToolFailure || !strings.Contains(outcome.stderr, "trust policy") {
			t.Fatalf("exit %d: %s", outcome.code, outcome.stderr)
		}
	})

	t.Run("a Sigstore bundle is rejected when the policy pins no Sigstore signer"+verifiesSignedRun, func(t *testing.T) {
		withSigstore := filepath.Join(t.TempDir(), "bundles")
		testkit.WriteFile(t, filepath.Join(withSigstore, filepath.FromSlash(statements[0].Location.BundlePath())), []byte(`{"mediaType":"application/vnd.dev.sigstore.bundle.v0.3+json"}`))
		other := testkit.NewLayout(t)
		testkit.WriteFile(t, other.Policy, policy)
		if outcome := run("--bundles", withSigstore, "--offline", "--out", other.Verified); outcome.code != cli.ExitRejected {
			t.Fatalf("exit %d: %s", outcome.code, outcome.stderr)
		}
	})
}

func TestLocalRun(t *testing.T) {
	statements := testkit.ValidStatements(t)
	local := []testkit.Statement{statements[0].AsLocal(t), statements[1].AsLocal(t)}

	t.Run("local statements are admitted without a policy and the trust is local"+verifiesLocalRun, func(t *testing.T) {
		layout := testkit.NewLayout(t)
		cache := testkit.WriteCache(t, local)
		if outcome := run("--accept-local", cache, "--out", layout.Verified); outcome.code != cli.ExitClean {
			t.Fatalf("exit %d: %s", outcome.code, outcome.stderr)
		}
		manifest, raw := readManifest(t, layout.Verified)
		if manifest.Trust != "local" || len(manifest.Statements) != 2 || manifest.PolicyDigest != "" || manifest.TrustedRootDigest != "" {
			t.Fatalf("manifest %s", raw)
		}
	})

	t.Run("a statement produced by CI in the local cache is refused"+verifiesLocalRun, func(t *testing.T) {
		layout := testkit.NewLayout(t)
		cache := testkit.WriteCache(t, statements[:1])
		if outcome := run("--accept-local", cache, "--out", layout.Verified); outcome.code != cli.ExitRejected {
			t.Fatalf("exit %d: %s", outcome.code, outcome.stderr)
		}
		if _, err := os.Stat(layout.Verified); err == nil {
			t.Fatal("a verified directory was written")
		}
	})

	t.Run("signed bundles and local statements make one set whose trust is local"+verifiesLocalRun, func(t *testing.T) {
		layout := testkit.NewLayout(t)
		identity := testkit.NewIdentity(1)
		testkit.WriteFile(t, layout.Policy, testkit.PolicyYAML(identity))
		bundles := testkit.ExportBundles(t, identity, statements[:2])
		cache := testkit.WriteCache(t, []testkit.Statement{statements[2].AsLocal(t)})
		if outcome := run("--bundles", bundles, "--accept-local", cache, "--offline", "--out", layout.Verified); outcome.code != cli.ExitClean {
			t.Fatalf("exit %d: %s", outcome.code, outcome.stderr)
		}
		manifest, _ := readManifest(t, layout.Verified)
		if manifest.Trust != "local" || len(manifest.Statements) != 3 || manifest.PolicyDigest == "" {
			t.Fatalf("manifest %+v", manifest)
		}
	})

	t.Run("a local statement that duplicates a signed one does not downgrade the trust"+verifiesLocalRun, func(t *testing.T) {
		layout, bundles, _, _ := signedSetup(t)
		cache := testkit.WriteCache(t, local[:1])
		if outcome := run("--bundles", bundles, "--accept-local", cache, "--offline", "--out", layout.Verified); outcome.code != cli.ExitClean {
			t.Fatalf("exit %d: %s", outcome.code, outcome.stderr)
		}
		manifest, _ := readManifest(t, layout.Verified)
		if manifest.Trust != "signed" || len(manifest.Statements) != 3 {
			t.Fatalf("manifest %+v", manifest)
		}
	})
}

func TestPendingFold(t *testing.T) {
	t.Run("the pending files of the modules are folded into the manifest, sorted"+verifiesPending, func(t *testing.T) {
		layout, bundles, _, _ := signedSetup(t)
		testkit.WriteFile(t, filepath.Join(layout.Pending, "discpower.json"),
			[]byte(`{"module":"discpower","pending":[{"criterion":"PARK-2~1","reason":"awaiting_host"},{"criterion":"PARK-9~1","reason":"host_unresponsive"}]}`))
		testkit.WriteFile(t, filepath.Join(layout.Pending, "alignment.json"),
			[]byte(`{"module":"alignment","pending":[{"criterion":"PARK-1~1","reason":"awaiting_proof"}]}`))
		if outcome := run("--bundles", bundles, "--offline", "--out", layout.Verified); outcome.code != cli.ExitClean {
			t.Fatalf("exit %d: %s", outcome.code, outcome.stderr)
		}
		manifest, _ := readManifest(t, layout.Verified)
		want := []string{"alignment PARK-1~1 awaiting_proof", "discpower PARK-2~1 awaiting_host", "discpower PARK-9~1 host_unresponsive"}
		var got []string
		for _, item := range manifest.Pending {
			got = append(got, item.Module+" "+item.Criterion+" "+item.Reason)
		}
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Fatalf("pending %v", got)
		}
	})

	t.Run("a malformed pending file rejects the run"+verifiesPending, func(t *testing.T) {
		layout, bundles, _, _ := signedSetup(t)
		testkit.WriteFile(t, filepath.Join(layout.Pending, "discpower.json"), []byte(`{"module":"discpower","pending":[{"criterion":"nope","reason":"awaiting_host"}]}`))
		if outcome := run("--bundles", bundles, "--offline", "--out", layout.Verified); outcome.code != cli.ExitRejected {
			t.Fatalf("exit %d: %s", outcome.code, outcome.stderr)
		}
	})
}

func TestCommandLine(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"no argument", nil},
		{"an unknown argument", []string{"frobnicate"}},
		{"an unknown flag", []string{"--verbose"}},
		{"no output directory", []string{"--bundles", "b"}},
		{"neither bundles nor local", []string{"--out", "o"}},
		{"offline without bundles", []string{"--accept-local", "l", "--offline", "--out", "o"}},
		{"a stray argument", []string{"--bundles", "b", "--out", "o", "extra"}},
	}
	for _, tt := range cases {
		t.Run(tt.name+" is a usage error with exit 2"+verifiesCommand, func(t *testing.T) {
			outcome := run(tt.args...)
			if outcome.code != cli.ExitToolFailure || !strings.Contains(outcome.stderr, "usage:") {
				t.Fatalf("exit %d: %s", outcome.code, outcome.stderr)
			}
		})
	}

	t.Run("a missing bundles directory is a tool failure"+verifiesCommand, func(t *testing.T) {
		layout := testkit.NewLayout(t)
		testkit.WriteFile(t, layout.Policy, testkit.PolicyYAML(testkit.NewIdentity(1)))
		outcome := run("--bundles", filepath.Join(t.TempDir(), "absent"), "--offline", "--out", layout.Verified)
		if outcome.code != cli.ExitToolFailure {
			t.Fatalf("exit %d: %s", outcome.code, outcome.stderr)
		}
	})
}
