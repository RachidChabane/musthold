package testkit

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/RachidChabane/musthold/format"
)

// The build signer the Sigstore fixtures are signed by, and the ones the policy
// must refuse. The fixtures are generated with these values.
const (
	FixtureIssuer       = "https://token.actions.githubusercontent.com"
	FixtureSignerURI    = "https://github.com/musthold/ci/.github/workflows/prove.yml@refs/tags/v1.0.0"
	FixtureSignerDigest = "0123456789abcdef0123456789abcdef01234567"
	OtherSignerDigest   = "fedcba9876543210fedcba9876543210fedcba98"
	OtherSignerURI      = "https://github.com/musthold/ci/.github/workflows/evil.yml@refs/heads/main"
	OtherIssuer         = "https://issuer.example.test"
)

// Variants of the generated bundles: who signed them.
const (
	VariantTrusted     = "trusted"
	VariantOtherDigest = "other-digest"
	VariantOtherURI    = "other-uri"
	VariantOtherIssuer = "other-issuer"
	VariantOtherCA     = "other-ca"
)

const (
	sigstoreFixturesPath = "internal/adapters/signing/sigstoreverifier/testdata"
	tufRootName          = "tuf-root.json"
	snapshotName         = "snapshot"
	bundlesName          = "bundles"
)

// SigstoreFixtures are the committed Sigstore fixtures: a TUF repository over a
// trusted root and bundles of the golden statements signed by a virtual Sigstore.
type SigstoreFixtures struct {
	// TUFRoot is the root the snapshot chains to; it stands for the root embedded in
	// the binary.
	TUFRoot  []byte
	snapshot string
	bundles  string
}

// NewSigstoreFixtures locates the fixtures from the module of the running test.
func NewSigstoreFixtures(t testing.TB) SigstoreFixtures {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for range maxAscent {
		candidate := filepath.Join(directory, filepath.FromSlash(sigstoreFixturesPath))
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			root, err := os.ReadFile(filepath.Join(candidate, tufRootName))
			if err != nil {
				t.Fatal(err)
			}
			return SigstoreFixtures{TUFRoot: root, snapshot: filepath.Join(candidate, snapshotName), bundles: filepath.Join(candidate, bundlesName)}
		}
		directory = filepath.Dir(directory)
	}
	t.Fatal("sigstore fixtures not found")
	return SigstoreFixtures{}
}

// Export builds a directory shaped like the output of `ledger export`: the proofs
// of a variant and the TUF snapshot below `trust/`.
func (f SigstoreFixtures) Export(t testing.TB, variant string) string {
	t.Helper()
	destination := t.TempDir()
	copyTree(t, filepath.Join(f.bundles, variant), destination)
	copyTree(t, f.snapshot, filepath.Join(destination, format.TrustDirectory))
	return destination
}

func copyTree(t testing.TB, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		WriteFile(t, filepath.Join(to, relative), content)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// SigstorePolicyYAML renders a trust policy that pins one build signer.
func SigstorePolicyYAML(issuer, uri, digest string) []byte {
	return []byte("version: 1\nsigstore:\n  issuer: " + issuer + "\n  signers:\n    - uri: " + uri + "\n      digest: " + digest + "\n")
}
