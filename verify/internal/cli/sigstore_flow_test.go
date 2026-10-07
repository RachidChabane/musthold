package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/verify/internal/cli"
	"github.com/RachidChabane/spec-proof/verify/internal/testkit"
)

const (
	verifiesSigstoreRun  = " [verifies SP-163~1]"
	verifiesTrustedRoot  = " [verifies SP-174~1]"
	verifiesMixedSchemes = " [verifies SP-164~1]"
)

func runWith(environment cli.Environment, args ...string) result {
	var stdout, stderr bytes.Buffer
	code := cli.Run(environment, binary, args, &stdout, &stderr)
	return result{code, stdout.String(), stderr.String()}
}

func sigstoreLayout(t *testing.T, policy []byte) testkit.Layout {
	t.Helper()
	layout := testkit.NewLayout(t)
	testkit.WriteFile(t, layout.Policy, policy)
	return layout
}

func TestSigstoreRun(t *testing.T) {
	fixtures := testkit.NewSigstoreFixtures(t)
	environment := cli.Environment{TUFRoot: fixtures.TUFRoot}
	pinned := testkit.SigstorePolicyYAML(testkit.FixtureIssuer, testkit.FixtureSignerURI, testkit.FixtureSignerDigest)

	t.Run("keyless bundles are verified offline against the trusted root of the exported snapshot"+verifiesSigstoreRun, func(t *testing.T) {
		layout := sigstoreLayout(t, pinned)
		exported := fixtures.Export(t, testkit.VariantTrusted)
		outcome := runWith(environment, "--bundles", exported, "--offline", "--out", layout.Verified)
		if outcome.code != cli.ExitClean {
			t.Fatalf("exit %d: %s", outcome.code, outcome.stderr)
		}
		manifest, _ := readManifest(t, layout.Verified)
		if manifest.Trust != "signed" || len(manifest.Statements) != 3 || manifest.PolicyDigest != format.SHA256Hex(pinned) {
			t.Fatalf("manifest %+v", manifest)
		}
		targets, err := filepath.Glob(filepath.Join(exported, "trust", "targets", "*"))
		if err != nil || len(targets) != 1 {
			t.Fatalf("targets %v: %v", targets, err)
		}
		raw, err := os.ReadFile(targets[0])
		if err != nil || manifest.TrustedRootDigest != format.SHA256Hex(raw) {
			t.Fatalf("trusted root digest %s, want %s: %v", manifest.TrustedRootDigest, format.SHA256Hex(raw), err)
		}
	})

	t.Run("the manifest is the same when the keyless verification is replayed"+verifiesSigstoreRun, func(t *testing.T) {
		exported := fixtures.Export(t, testkit.VariantTrusted)
		first, second := sigstoreLayout(t, pinned), sigstoreLayout(t, pinned)
		for _, layout := range []testkit.Layout{first, second} {
			if outcome := runWith(environment, "--bundles", exported, "--offline", "--out", layout.Verified); outcome.code != cli.ExitClean {
				t.Fatalf("exit %d: %s", outcome.code, outcome.stderr)
			}
		}
		_, a := readManifest(t, first.Verified)
		_, b := readManifest(t, second.Verified)
		if !bytes.Equal(a, b) {
			t.Fatalf("manifests differ:\n%s\n%s", a, b)
		}
	})

	t.Run("a build signer the policy does not pin is rejected with exit 1 and nothing is written"+verifiesSigstoreRun, func(t *testing.T) {
		for _, variant := range []string{testkit.VariantOtherDigest, testkit.VariantOtherURI, testkit.VariantOtherIssuer, testkit.VariantOtherCA} {
			layout := sigstoreLayout(t, pinned)
			outcome := runWith(environment, "--bundles", fixtures.Export(t, variant), "--offline", "--out", layout.Verified)
			if outcome.code != cli.ExitRejected {
				t.Errorf("%s: exit %d: %s", variant, outcome.code, outcome.stderr)
			}
			if _, err := os.Stat(layout.Verified); err == nil {
				t.Errorf("%s: a verified directory was written", variant)
			}
		}
	})

	t.Run("a policy that pins the other build signer accepts its bundles"+verifiesSigstoreRun, func(t *testing.T) {
		layout := sigstoreLayout(t, testkit.SigstorePolicyYAML(testkit.FixtureIssuer, testkit.FixtureSignerURI, testkit.OtherSignerDigest))
		outcome := runWith(environment, "--bundles", fixtures.Export(t, testkit.VariantOtherDigest), "--offline", "--out", layout.Verified)
		if outcome.code != cli.ExitClean {
			t.Fatalf("exit %d: %s", outcome.code, outcome.stderr)
		}
	})

	t.Run("envelopes of a pinned key and keyless bundles make one signed set"+verifiesMixedSchemes, func(t *testing.T) {
		identity := testkit.NewIdentity(4)
		var policy bytes.Buffer
		policy.Write(testkit.PolicyYAML(identity))
		policy.WriteString("sigstore:\n  issuer: " + testkit.FixtureIssuer + "\n  signers:\n    - uri: " + testkit.FixtureSignerURI + "\n      digest: " + testkit.FixtureSignerDigest + "\n")
		layout := sigstoreLayout(t, policy.Bytes())

		exported := fixtures.Export(t, testkit.VariantTrusted)
		statements := testkit.ValidStatements(t)
		replaced := statements[0]
		bundlePath := filepath.Join(exported, filepath.FromSlash(replaced.Location.BundlePath()))
		envelope, err := format.SignEnvelope(replaced.Bytes, identity.Private).Marshal()
		if err != nil {
			t.Fatal(err)
		}
		testkit.WriteFile(t, bundlePath, envelope)

		outcome := runWith(environment, "--bundles", exported, "--offline", "--out", layout.Verified)
		if outcome.code != cli.ExitClean {
			t.Fatalf("exit %d: %s", outcome.code, outcome.stderr)
		}
		manifest, _ := readManifest(t, layout.Verified)
		if manifest.Trust != "signed" || len(manifest.Statements) != 3 {
			t.Fatalf("manifest %+v", manifest)
		}
	})

	t.Run("a Ed25519 key policy refuses a keyless bundle"+verifiesMixedSchemes, func(t *testing.T) {
		layout := testkit.NewLayout(t)
		testkit.WriteFile(t, layout.Policy, testkit.PolicyYAML(testkit.NewIdentity(4)))
		outcome := runWith(environment, "--bundles", fixtures.Export(t, testkit.VariantTrusted), "--offline", "--out", layout.Verified)
		if outcome.code != cli.ExitRejected || !strings.Contains(outcome.stderr, "sigstore section") {
			t.Fatalf("exit %d: %s", outcome.code, outcome.stderr)
		}
	})
}

func TestTrustedRootThroughTUF(t *testing.T) {
	fixtures := testkit.NewSigstoreFixtures(t)
	pinned := testkit.SigstorePolicyYAML(testkit.FixtureIssuer, testkit.FixtureSignerURI, testkit.FixtureSignerDigest)

	t.Run("a snapshot that does not chain to the root of the binary is a tool failure"+verifiesTrustedRoot, func(t *testing.T) {
		layout := sigstoreLayout(t, pinned)
		outcome := run("--bundles", fixtures.Export(t, testkit.VariantTrusted), "--offline", "--out", layout.Verified)
		if outcome.code != cli.ExitToolFailure || !strings.Contains(outcome.stderr, "tuf") {
			t.Fatalf("exit %d: %s", outcome.code, outcome.stderr)
		}
		if _, err := os.Stat(layout.Verified); err == nil {
			t.Fatal("a verified directory was written")
		}
	})

	t.Run("an exported ledger without a trust snapshot is a tool failure"+verifiesTrustedRoot, func(t *testing.T) {
		layout := sigstoreLayout(t, pinned)
		exported := fixtures.Export(t, testkit.VariantTrusted)
		if err := os.RemoveAll(filepath.Join(exported, "trust")); err != nil {
			t.Fatal(err)
		}
		outcome := runWith(cli.Environment{TUFRoot: fixtures.TUFRoot}, "--bundles", exported, "--offline", "--out", layout.Verified)
		if outcome.code != cli.ExitToolFailure {
			t.Fatalf("exit %d: %s", outcome.code, outcome.stderr)
		}
	})

	t.Run("a tampered trust snapshot is a tool failure"+verifiesTrustedRoot, func(t *testing.T) {
		layout := sigstoreLayout(t, pinned)
		exported := fixtures.Export(t, testkit.VariantTrusted)
		targets, _ := filepath.Glob(filepath.Join(exported, "trust", "targets", "*"))
		content, err := os.ReadFile(targets[0])
		if err != nil {
			t.Fatal(err)
		}
		content[len(content)/2] ^= 0x01
		testkit.WriteFile(t, targets[0], content)
		outcome := runWith(cli.Environment{TUFRoot: fixtures.TUFRoot}, "--bundles", exported, "--offline", "--out", layout.Verified)
		if outcome.code != cli.ExitToolFailure {
			t.Fatalf("exit %d: %s", outcome.code, outcome.stderr)
		}
	})

	t.Run("a policy without a Sigstore section never reads the trust snapshot"+verifiesTrustedRoot, func(t *testing.T) {
		identity := testkit.NewIdentity(6)
		layout := testkit.NewLayout(t)
		testkit.WriteFile(t, layout.Policy, testkit.PolicyYAML(identity))
		exported := testkit.ExportBundles(t, identity, testkit.ValidStatements(t))
		testkit.WriteFile(t, filepath.Join(exported, "trust", "garbage.json"), []byte("not a TUF repository"))
		if outcome := run("--bundles", exported, "--offline", "--out", layout.Verified); outcome.code != cli.ExitClean {
			t.Fatalf("exit %d: %s", outcome.code, outcome.stderr)
		}
		manifest, _ := readManifest(t, layout.Verified)
		if manifest.TrustedRootDigest != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
			t.Fatalf("trusted root digest %s", manifest.TrustedRootDigest)
		}
	})
}
