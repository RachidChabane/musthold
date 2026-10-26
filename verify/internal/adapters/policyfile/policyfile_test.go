package policyfile_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/verify/internal/adapters/policyfile"
	"github.com/RachidChabane/musthold/verify/internal/domain/refusal"
	"github.com/RachidChabane/musthold/verify/internal/testkit"
)

const verifiesPolicyFile = " [verifies SP-210~1]"

func TestLoad(t *testing.T) {
	t.Run("a policy file gives its keys and the digest of its exact bytes"+verifiesPolicyFile, func(t *testing.T) {
		raw := testkit.PolicyYAML(testkit.NewIdentity(1), testkit.NewIdentity(2))
		path := filepath.Join(t.TempDir(), "trust-policy.yaml")
		testkit.WriteFile(t, path, raw)
		policy, err := policyfile.File{Path: path}.Load()
		if err != nil || len(policy.Ed25519) != 2 || policy.Digest != format.SHA256Hex(raw) {
			t.Fatalf("policy %+v: %v", policy, err)
		}
	})

	t.Run("a keyless section is read with its signers"+verifiesPolicyFile, func(t *testing.T) {
		raw := "version: 1\nsigstore:\n  issuer: https://token.actions.githubusercontent.com\n  signers:\n" +
			"    - uri: https://github.com/o/r/.github/workflows/prove.yml@refs/tags/v1\n      digest: " + strings.Repeat("cd", 20) + "\n"
		policy, err := policyfile.Parse([]byte(raw))
		if err != nil || policy.Sigstore == nil || policy.Sigstore.Issuer != "https://token.actions.githubusercontent.com" || len(policy.Sigstore.Signers) != 1 {
			t.Fatalf("policy %+v: %v", policy, err)
		}
	})

	t.Run("a missing file is reported as not found"+verifiesPolicyFile, func(t *testing.T) {
		_, err := policyfile.File{Path: filepath.Join(t.TempDir(), "absent.yaml")}.Load()
		if !errors.Is(err, policyfile.ErrNotFound) || refusal.Is(err) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a directory in place of the file is a failure"+verifiesPolicyFile, func(t *testing.T) {
		if _, err := (policyfile.File{Path: t.TempDir()}).Load(); err == nil {
			t.Fatal("loaded a directory")
		}
	})

	refused := map[string]string{
		"an unknown member":      "version: 1\nunknown: true\ned25519: []\n",
		"a misspelled key":       "version: 1\ned25519:\n  - publicKeys: abc\n",
		"not YAML":               "version: [",
		"an empty file":          "",
		"no trust anchor":        "version: 1\n",
		"an empty sigstore part": "version: 1\nsigstore: {}\n",
		"a malformed key":        "version: 1\ned25519:\n  - publicKey: AAAA\n",
	}
	for name, raw := range refused {
		t.Run(name+" is refused"+verifiesPolicyFile, func(t *testing.T) {
			if _, err := policyfile.Parse([]byte(raw)); !refusal.Is(err) {
				t.Fatalf("error %v", err)
			}
		})
	}
}

func FuzzParse(f *testing.F) {
	f.Add(testkit.PolicyYAML(testkit.NewIdentity(1)))
	f.Add([]byte("version: 1\nsigstore:\n  issuer: i\n  signers: []\n"))
	f.Fuzz(func(t *testing.T, raw []byte) {
		policy, err := policyfile.Parse(raw)
		if err == nil && policy.Digest != format.SHA256Hex(raw) {
			t.Fatal("digest is not the digest of the bytes")
		}
	})
}
