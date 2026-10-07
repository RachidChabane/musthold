package trustpolicy_test

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/verify/internal/domain/trustpolicy"
)

const verifiesPolicy = " [verifies SP-210~1]"

func publicKey(seed byte) string {
	private := ed25519.NewKeyFromSeed([]byte(strings.Repeat(string([]byte{seed}), ed25519.SeedSize)))
	public, _ := private.Public().(ed25519.PublicKey)
	return base64.StdEncoding.EncodeToString(public)
}

var commit = strings.Repeat("ab", 20)

func TestBuild(t *testing.T) {
	t.Run("the keys of a policy are identified by the digest of their raw bytes"+verifiesPolicy, func(t *testing.T) {
		policy, err := trustpolicy.Build(trustpolicy.Spec{Version: 1, Ed25519PublicKeys: []string{publicKey(1), publicKey(2)}}, "digest")
		if err != nil {
			t.Fatal(err)
		}
		if len(policy.Ed25519) != 2 || policy.Digest != "digest" || policy.Sigstore != nil {
			t.Fatalf("policy %+v", policy)
		}
		raw, _ := base64.StdEncoding.DecodeString(publicKey(1))
		key, found := policy.Ed25519Key(format.Ed25519KeyID(raw))
		if !found || string(key.Public) != string(raw) {
			t.Fatalf("key %+v %v", key, found)
		}
		if _, found := policy.Ed25519Key("unknown"); found {
			t.Fatal("an unknown key identifier was found")
		}
	})

	t.Run("a keyless policy pins an issuer and build signers"+verifiesPolicy, func(t *testing.T) {
		policy, err := trustpolicy.Build(trustpolicy.Spec{
			Version:        1,
			SigstoreIssuer: "https://token.actions.githubusercontent.com",
			SigstoreSigners: []trustpolicy.BuildSignerSpec{
				{URI: "https://github.com/o/r/.github/workflows/prove.yml@refs/tags/v1", Digest: commit},
			},
		}, "d")
		if err != nil || policy.Sigstore == nil || len(policy.Sigstore.Signers) != 1 || policy.Sigstore.Signers[0].Digest != commit {
			t.Fatalf("policy %+v: %v", policy, err)
		}
	})

	invalid := map[string]trustpolicy.Spec{
		"another version":                    {Version: 2, Ed25519PublicKeys: []string{publicKey(1)}},
		"no version":                         {Ed25519PublicKeys: []string{publicKey(1)}},
		"no trust anchor":                    {Version: 1},
		"a key that is not base64":           {Version: 1, Ed25519PublicKeys: []string{"***"}},
		"a key of the wrong size":            {Version: 1, Ed25519PublicKeys: []string{base64.StdEncoding.EncodeToString([]byte("short"))}},
		"a key listed twice":                 {Version: 1, Ed25519PublicKeys: []string{publicKey(1), publicKey(1)}},
		"an issuer without signer":           {Version: 1, SigstoreIssuer: "https://issuer"},
		"a signer without issuer":            {Version: 1, SigstoreSigners: []trustpolicy.BuildSignerSpec{{URI: "u", Digest: commit}}},
		"a signer without URI":               {Version: 1, SigstoreIssuer: "i", SigstoreSigners: []trustpolicy.BuildSignerSpec{{Digest: commit}}},
		"a signer without digest":            {Version: 1, SigstoreIssuer: "i", SigstoreSigners: []trustpolicy.BuildSignerSpec{{URI: "u"}}},
		"a signer with a short digest":       {Version: 1, SigstoreIssuer: "i", SigstoreSigners: []trustpolicy.BuildSignerSpec{{URI: "u", Digest: "abc"}}},
		"a signer with an upper-case digest": {Version: 1, SigstoreIssuer: "i", SigstoreSigners: []trustpolicy.BuildSignerSpec{{URI: "u", Digest: strings.ToUpper(commit)}}},
		"a signer listed twice":              {Version: 1, SigstoreIssuer: "i", SigstoreSigners: []trustpolicy.BuildSignerSpec{{URI: "u", Digest: commit}, {URI: "u", Digest: commit}}},
	}
	for name, spec := range invalid {
		t.Run(name+" is refused"+verifiesPolicy, func(t *testing.T) {
			if _, err := trustpolicy.Build(spec, "d"); !errors.Is(err, trustpolicy.ErrInvalid) {
				t.Fatalf("error %v", err)
			}
		})
	}
}
