// Package sigstoreverifier verifies keyless Sigstore bundles. It uses the verifier
// of sigstore-go under a policy that pins the build signer: the issuer, the URI of
// the reusable workflow and the commit digest it ran at. The trusted material is a
// trusted root that was verified through TUF back to a root embedded in the binary.
package sigstoreverifier

import (
	"errors"
	"fmt"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/fulcio/certificate"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/verify"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/verify/internal/domain/refusal"
	"github.com/RachidChabane/musthold/verify/internal/domain/trustpolicy"
)

// What a bundle must prove besides its certificate chain: one Rekor entry whose
// integrated time is the time the certificate is checked at. The signing side
// produces exactly this, so a bundle that lacks either is refused.
const (
	requiredLogEntries           = 1
	requiredIntegratedTimestamps = 1
)

// ErrNoDSSEEnvelope is returned for a bundle that carries no DSSE envelope.
var ErrNoDSSEEnvelope = errors.New("the bundle carries no DSSE envelope")

// Verifier accepts a bundle signed by a pinned build signer.
type Verifier struct {
	verifier *verify.Verifier
	policy   verify.PolicyBuilder
}

// New returns the verifier of a policy over trusted material.
func New(material root.TrustedMaterial, pinned trustpolicy.Sigstore) (*Verifier, error) {
	verifier, err := verify.NewVerifier(material,
		verify.WithTransparencyLog(requiredLogEntries),
		verify.WithIntegratedTimestamps(requiredIntegratedTimestamps))
	if err != nil {
		return nil, fmt.Errorf("sigstore verifier: %w", err)
	}
	options := make([]verify.PolicyOption, 0, len(pinned.Signers))
	for _, signer := range pinned.Signers {
		identity, err := identityOf(pinned.Issuer, signer)
		if err != nil {
			return nil, err
		}
		options = append(options, verify.WithCertificateIdentity(identity))
	}
	return &Verifier{verifier: verifier, policy: verify.NewPolicy(verify.WithoutArtifactUnsafe(), options...)}, nil
}

// identityOf is the certificate identity of a pinned build signer: the workflow
// URI is both the subject alternative name and the build signer URI of the
// certificate, the issuer is the pinned one, and the build signer digest is the
// pinned commit.
func identityOf(issuer string, signer trustpolicy.BuildSigner) (verify.CertificateIdentity, error) {
	san, err := verify.NewSANMatcher(signer.URI, "")
	if err != nil {
		return verify.CertificateIdentity{}, fmt.Errorf("build signer %s: %w", signer.URI, err)
	}
	issuerMatcher, err := verify.NewIssuerMatcher(issuer, "")
	if err != nil {
		return verify.CertificateIdentity{}, fmt.Errorf("issuer %s: %w", issuer, err)
	}
	identity, err := verify.NewCertificateIdentity(san, issuerMatcher, certificate.Extensions{
		BuildSignerURI:    signer.URI,
		BuildSignerDigest: signer.Digest,
	})
	if err != nil {
		return verify.CertificateIdentity{}, fmt.Errorf("build signer %s: %w", signer.URI, err)
	}
	return identity, nil
}

// Verify checks the bundle against the trusted root and the pinned build signers
// and returns the exact statement bytes of its DSSE envelope. A bundle that does
// not verify is a refusal.
func (v *Verifier) Verify(bundleJSON []byte) ([]byte, error) {
	parsed := &bundle.Bundle{}
	if err := parsed.UnmarshalJSON(bundleJSON); err != nil {
		return nil, refusal.Newf("sigstore bundle: %v", err)
	}
	if _, err := v.verifier.Verify(parsed, v.policy); err != nil {
		return nil, refusal.Newf("sigstore verification: %v", err)
	}
	envelope := parsed.GetDsseEnvelope()
	if envelope == nil {
		return nil, refusal.Wrap(ErrNoDSSEEnvelope)
	}
	if envelope.GetPayloadType() != format.DSSEPayloadType {
		return nil, refusal.Newf("payload type %q is not %q", envelope.GetPayloadType(), format.DSSEPayloadType)
	}
	return envelope.GetPayload(), nil
}
