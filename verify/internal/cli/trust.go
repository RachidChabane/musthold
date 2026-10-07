package cli

import (
	"context"
	"path/filepath"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/verify/internal/adapters/policyfile"
	"github.com/RachidChabane/spec-proof/verify/internal/adapters/signing/ed25519verifier"
	"github.com/RachidChabane/spec-proof/verify/internal/adapters/signing/sigstoreverifier"
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

// newBundleTrust loads the trust policy and builds the verifier of each scheme it
// pins. The trusted root digest is the one of the verified trusted root when the
// policy pins a Sigstore signer, and the digest of the empty input otherwise.
func newBundleTrust(environment Environment, opts options, policyPath string) (bundleTrust, error) {
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
		sigstoreVerifier, rootDigest, err := newSigstoreVerifier(environment, *policy.Sigstore, opts)
		if err != nil {
			return bundleTrust{}, err
		}
		schemes.Sigstore = sigstoreVerifier
		trusted.verifier, trusted.trustedRootDigest = schemes, rootDigest
	}
	return trusted, nil
}

// newSigstoreVerifier loads the trusted root through TUF, back to the root embedded
// in the binary (or the one the environment gives): from the trust snapshot of the
// exported ledger when offline, from the public-good mirror otherwise.
func newSigstoreVerifier(environment Environment, pinned trustpolicy.Sigstore, opts options) (app.SignatureVerifier, string, error) {
	tufRoot := environment.TUFRoot
	if tufRoot == nil {
		tufRoot = sigstoreverifier.PublicGoodRoot()
	}
	source := sigstoreverifier.FromMirror(tufRoot)
	if opts.offline {
		source = sigstoreverifier.FromSnapshot(tufRoot, filepath.Join(opts.bundles, format.TrustDirectory))
	}
	trusted, err := source.Load(context.Background())
	if err != nil {
		return nil, "", err
	}
	verifier, err := sigstoreverifier.New(trusted.Material, pinned)
	if err != nil {
		return nil, "", err
	}
	return verifier, trusted.Digest, nil
}
