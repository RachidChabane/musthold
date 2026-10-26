package app

import (
	"errors"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/verify/internal/domain/refusal"
)

// NoBundles is the BundleSource of a run without `--bundles`.
type NoBundles struct{}

// Bundles lists nothing.
func (NoBundles) Bundles() ([]BundleFile, error) { return nil, nil }

// NoLocal is the LocalSource of a run without `--accept-local`.
type NoLocal struct{}

// LocalStatements lists nothing.
func (NoLocal) LocalStatements() ([]LocalStatement, error) { return nil, nil }

// ErrNoSigstorePolicy is returned for a Sigstore bundle when the policy trusts no
// Sigstore signer.
var ErrNoSigstorePolicy = errors.New("the trust policy has no sigstore section")

// ErrUnknownBundle is returned for a file that is neither a DSSE envelope nor a
// Sigstore bundle.
var ErrUnknownBundle = errors.New("neither a DSSE envelope nor a Sigstore bundle")

// SchemeSwitch is the SignatureVerifier that sends a bundle to the verifier of its
// signature scheme. A scheme without verifier refuses its bundles.
type SchemeSwitch struct {
	// Envelope verifies bare DSSE envelopes signed with Ed25519 keys; nil when the
	// policy pins no key.
	Envelope SignatureVerifier
	// Sigstore verifies keyless bundles; nil when the policy has no sigstore section.
	Sigstore SignatureVerifier
}

// Verify verifies the bundle with the verifier of its scheme.
func (s SchemeSwitch) Verify(bundle []byte) ([]byte, error) {
	switch format.ShapeOfBundle(bundle) {
	case format.ShapeEnvelope:
		if s.Envelope == nil {
			return nil, refusal.Newf("the trust policy pins no Ed25519 key")
		}
		return s.Envelope.Verify(bundle)
	case format.ShapeSigstoreBundle:
		if s.Sigstore == nil {
			return nil, refusal.Wrap(ErrNoSigstorePolicy)
		}
		return s.Sigstore.Verify(bundle)
	default:
		return nil, refusal.Wrap(ErrUnknownBundle)
	}
}
