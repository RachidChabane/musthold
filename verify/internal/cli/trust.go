package cli

import (
	"errors"

	"github.com/RachidChabane/spec-proof/verify/internal/adapters/policyfile"
	"github.com/RachidChabane/spec-proof/verify/internal/adapters/signing/ed25519verifier"
	"github.com/RachidChabane/spec-proof/verify/internal/app"
	"github.com/RachidChabane/spec-proof/verify/internal/domain/proofset"
	"github.com/RachidChabane/spec-proof/verify/internal/domain/trustpolicy"
)

// bundleTrust is what verifying signed bundles needs: the verifier of each
// signature scheme and the digests the manifest states.
type bundleTrust struct {
	verifier          app.SignatureVerifier
	trustedRootDigest string
	policyDigest      string
}

var errSigstoreUnavailable = errors.New("sigstore verification is not available")

// newBundleTrust loads the trust policy and builds the verifier of each scheme it
// pins. The trusted root digest is the one of the verified trusted root when the
// policy pins a Sigstore signer, and the digest of the empty input otherwise.
func newBundleTrust(opts options, policyPath string) (bundleTrust, error) {
	policy, err := policyfile.File{Path: policyPath}.Load()
	if err != nil {
		return bundleTrust{}, err
	}
	schemes := app.SchemeSwitch{}
	if len(policy.Ed25519) > 0 {
		schemes.Envelope = ed25519verifier.New(policy)
	}
	trusted := bundleTrust{verifier: schemes, trustedRootDigest: proofset.NoTrustedRootDigest, policyDigest: policy.Digest}
	if policy.Sigstore != nil {
		sigstoreVerifier, rootDigest, err := newSigstoreVerifier(policy, opts)
		if err != nil {
			return bundleTrust{}, err
		}
		schemes.Sigstore = sigstoreVerifier
		trusted.verifier, trusted.trustedRootDigest = schemes, rootDigest
	}
	return trusted, nil
}

func newSigstoreVerifier(_ trustpolicy.Policy, _ options) (app.SignatureVerifier, string, error) {
	return nil, "", errSigstoreUnavailable
}
