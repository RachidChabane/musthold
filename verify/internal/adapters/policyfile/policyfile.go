// Package policyfile reads `.musthold/trust-policy.yaml`. Unknown members are an
// error: a policy is security configuration, and a misspelled key must not be
// silently ignored.
package policyfile

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/verify/internal/domain/refusal"
	"github.com/RachidChabane/musthold/verify/internal/domain/trustpolicy"
)

// ErrNotFound is returned when the policy file does not exist.
var ErrNotFound = errors.New("trust policy not found")

type document struct {
	Version int `yaml:"version"`
	Ed25519 []struct {
		PublicKey string `yaml:"publicKey"`
	} `yaml:"ed25519"`
	Sigstore *struct {
		Issuer  string `yaml:"issuer"`
		Signers []struct {
			URI    string `yaml:"uri"`
			Digest string `yaml:"digest"`
		} `yaml:"signers"`
	} `yaml:"sigstore"`
}

// File is the trust policy at a path.
type File struct {
	Path string
}

// Load reads and checks the policy. Its digest is the SHA-256 of the exact bytes of
// the file.
func (f File) Load() (trustpolicy.Policy, error) {
	raw, err := os.ReadFile(f.Path)
	if errors.Is(err, os.ErrNotExist) {
		return trustpolicy.Policy{}, fmt.Errorf("%w: %s", ErrNotFound, f.Path)
	}
	if err != nil {
		return trustpolicy.Policy{}, fmt.Errorf("trust policy: %w", err)
	}
	return Parse(raw)
}

// Parse checks policy bytes and returns the policy whose digest is theirs.
func Parse(raw []byte) (trustpolicy.Policy, error) {
	var parsed document
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	if err := decoder.Decode(&parsed); err != nil {
		return trustpolicy.Policy{}, refusal.Newf("trust policy: %v", err)
	}
	spec := trustpolicy.Spec{Version: parsed.Version}
	for _, key := range parsed.Ed25519 {
		spec.Ed25519PublicKeys = append(spec.Ed25519PublicKeys, key.PublicKey)
	}
	if parsed.Sigstore != nil {
		spec.SigstoreIssuer = parsed.Sigstore.Issuer
		for _, signer := range parsed.Sigstore.Signers {
			spec.SigstoreSigners = append(spec.SigstoreSigners, trustpolicy.BuildSignerSpec{URI: signer.URI, Digest: signer.Digest})
		}
	}
	policy, err := trustpolicy.Build(spec, format.SHA256Hex(raw))
	if err != nil {
		return trustpolicy.Policy{}, refusal.Wrap(err)
	}
	return policy, nil
}
