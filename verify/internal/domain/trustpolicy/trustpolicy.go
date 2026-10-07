// Package trustpolicy is the domain model of `.spec-proof/trust-policy.yaml`: the
// Ed25519 public keys and the Sigstore build signers the verifier trusts. It
// holds no file or YAML knowledge; an adapter decodes the file into a Spec.
package trustpolicy

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"

	"github.com/RachidChabane/spec-proof/format"
)

// SupportedVersion is the only policy version this verifier reads.
const SupportedVersion = 1

// ErrInvalid wraps every reason a policy is refused.
var ErrInvalid = errors.New("invalid trust policy")

// buildSignerDigestPattern matches the commit digest a build signer is pinned to.
var buildSignerDigestPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// BuildSignerSpec is one pinned build signer as the file states it.
type BuildSignerSpec struct {
	URI    string
	Digest string
}

// Spec is the content of a trust policy file, decoded and not yet checked.
type Spec struct {
	Version int
	// Ed25519PublicKeys are base64 raw Ed25519 public keys.
	Ed25519PublicKeys []string
	// SigstoreIssuer and SigstoreSigners pin the keyless identities; both are empty
	// for a policy that trusts no Sigstore signer.
	SigstoreIssuer  string
	SigstoreSigners []BuildSignerSpec
}

// Ed25519Key is a trusted Ed25519 public key.
type Ed25519Key struct {
	ID     string
	Public ed25519.PublicKey
}

// BuildSigner pins the reusable workflow a keyless signature must come from: its
// URI and the commit digest it was run at.
type BuildSigner struct {
	URI    string
	Digest string
}

// Sigstore is the keyless part of a policy.
type Sigstore struct {
	Issuer  string
	Signers []BuildSigner
}

// Policy is a checked trust policy.
type Policy struct {
	// Digest is the SHA-256 of the exact bytes of the policy file.
	Digest   string
	Ed25519  []Ed25519Key
	Sigstore *Sigstore
}

// Build checks a spec and returns the policy whose file digest is digest. A policy
// without a trust anchor, with a malformed key or pin, or of another version is
// refused.
func Build(spec Spec, digest string) (Policy, error) {
	if spec.Version != SupportedVersion {
		return Policy{}, fmt.Errorf("%w: version %d, want %d", ErrInvalid, spec.Version, SupportedVersion)
	}
	policy := Policy{Digest: digest}
	seen := map[string]bool{}
	for index, encoded := range spec.Ed25519PublicKeys {
		raw, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(raw) != ed25519.PublicKeySize {
			return Policy{}, fmt.Errorf("%w: ed25519 key %d is not a base64 raw Ed25519 public key", ErrInvalid, index)
		}
		key := Ed25519Key{ID: format.Ed25519KeyID(raw), Public: raw}
		if seen[key.ID] {
			return Policy{}, fmt.Errorf("%w: ed25519 key %d is listed twice", ErrInvalid, index)
		}
		seen[key.ID] = true
		policy.Ed25519 = append(policy.Ed25519, key)
	}
	sigstore, err := buildSigstore(spec)
	if err != nil {
		return Policy{}, err
	}
	policy.Sigstore = sigstore
	if len(policy.Ed25519) == 0 && policy.Sigstore == nil {
		return Policy{}, fmt.Errorf("%w: no trust anchor", ErrInvalid)
	}
	return policy, nil
}

func buildSigstore(spec Spec) (*Sigstore, error) {
	if spec.SigstoreIssuer == "" && len(spec.SigstoreSigners) == 0 {
		return nil, nil
	}
	if spec.SigstoreIssuer == "" || len(spec.SigstoreSigners) == 0 {
		return nil, fmt.Errorf("%w: sigstore needs an issuer and at least one signer", ErrInvalid)
	}
	sigstore := &Sigstore{Issuer: spec.SigstoreIssuer}
	seen := map[BuildSigner]bool{}
	for index, signer := range spec.SigstoreSigners {
		if signer.URI == "" || !buildSignerDigestPattern.MatchString(signer.Digest) {
			return nil, fmt.Errorf("%w: sigstore signer %d needs a URI and a lowercase hex commit digest", ErrInvalid, index)
		}
		pinned := BuildSigner(signer)
		if seen[pinned] {
			return nil, fmt.Errorf("%w: sigstore signer %d is listed twice", ErrInvalid, index)
		}
		seen[pinned] = true
		sigstore.Signers = append(sigstore.Signers, pinned)
	}
	return sigstore, nil
}

// Ed25519Key returns the trusted key with the given identifier.
func (p Policy) Ed25519Key(id string) (Ed25519Key, bool) {
	for _, key := range p.Ed25519 {
		if key.ID == id {
			return key, true
		}
	}
	return Ed25519Key{}, false
}
