// Package app holds the use case of the verifier: admit the statements of signed
// bundles and of the local cache into one verified set and write it. It reads
// files only; every file system and signature scheme sits behind a port.
package app

import (
	"errors"
	"fmt"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/verify/internal/domain/proofset"
	"github.com/RachidChabane/spec-proof/verify/internal/domain/refusal"
)

// BundleFile is a signed proof read from the exported ledger.
type BundleFile struct {
	Location format.ProofLocation
	Bytes    []byte
}

// LocalStatement is an unsigned statement read from a module's local cache.
type LocalStatement struct {
	Location format.ProofLocation
	Bytes    []byte
}

// BundleSource lists the signed proofs of an exported ledger.
type BundleSource interface {
	Bundles() ([]BundleFile, error)
}

// LocalSource lists the statements of a local cache.
type LocalSource interface {
	LocalStatements() ([]LocalStatement, error)
}

// PendingSource lists the pending files of the modules.
type PendingSource interface {
	Pending() ([]format.Pending, error)
}

// SignatureVerifier checks the signature of a bundle against the trust policy and
// returns the statement it carries. A bundle the policy does not accept is a
// refusal.
type SignatureVerifier interface {
	Verify(bundle []byte) ([]byte, error)
}

// VerifiedSink replaces the verified directory by the given statement files and
// manifest.
type VerifiedSink interface {
	Write(files []proofset.File, manifest []byte) error
}

// VerifyService builds the verified set.
type VerifyService struct {
	Version string
	// Signed is true when signed bundles are verified: the manifest then states the
	// digests of the trusted root and of the policy.
	Signed            bool
	TrustedRootDigest string
	PolicyDigest      string

	Bundles  BundleSource
	Local    LocalSource
	Pending  PendingSource
	Verifier SignatureVerifier
	Sink     VerifiedSink
}

// Report summarises a run.
type Report struct {
	Trust      proofset.Trust
	Signed     int
	Local      int
	Pending    int
	Statements int
}

// Run verifies every bundle, admits the local statements, folds the pending
// files and writes the verified directory. One bundle or statement that is
// rejected rejects the run: nothing is written, so the previous verified
// directory stays as it was and a half-trusted set never exists.
func (s VerifyService) Run() (Report, error) {
	set := proofset.New()
	signed, err := s.admitBundles(set)
	if err != nil {
		return Report{}, err
	}
	local, err := s.admitLocal(set)
	if err != nil {
		return Report{}, err
	}
	pending, err := s.admitPending(set)
	if err != nil {
		return Report{}, err
	}
	header := proofset.Header{VerifierVersion: s.Version, NoBundles: !s.Signed}
	if s.Signed {
		header.TrustedRootDigest, header.PolicyDigest = s.TrustedRootDigest, s.PolicyDigest
	}
	manifest := set.Manifest(header)
	encoded, err := manifest.Bytes()
	if err != nil {
		return Report{}, fmt.Errorf("encode the manifest: %w", err)
	}
	if err := s.Sink.Write(set.Files(), encoded); err != nil {
		return Report{}, fmt.Errorf("write the verified set: %w", err)
	}
	return Report{Trust: manifest.Trust, Signed: signed, Local: local, Pending: pending, Statements: len(manifest.Statements)}, nil
}

func (s VerifyService) admitBundles(set *proofset.Set) (int, error) {
	bundles, err := s.Bundles.Bundles()
	if err != nil {
		return 0, fmt.Errorf("read the bundles: %w", err)
	}
	var problems []error
	for _, bundle := range bundles {
		statement, err := s.verifyBundle(bundle)
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", bundle.Location.BundlePath(), err))
			continue
		}
		set.AddSigned(bundle.Location, statement, bundle.Bytes)
	}
	return len(bundles), errors.Join(problems...)
}

// verifyBundle checks the signature, then the statement: it must satisfy the
// format, come from CI and sit at the location its content dictates.
func (s VerifyService) verifyBundle(bundle BundleFile) ([]byte, error) {
	statement, err := s.Verifier.Verify(bundle.Bytes)
	if err != nil {
		return nil, refusal.Wrap(err)
	}
	admitted, err := format.Admit(statement)
	if err != nil {
		return nil, refusal.Wrap(err)
	}
	if producer := admitted.Proof.Common().ProducedBy; producer != format.ProducedByCI {
		return nil, refusal.Newf("a signed statement is produced by %q, not %q", producer, format.ProducedByCI)
	}
	if err := admitted.CheckLocation(bundle.Location); err != nil {
		return nil, refusal.Wrap(err)
	}
	return statement, nil
}

func (s VerifyService) admitLocal(set *proofset.Set) (int, error) {
	statements, err := s.Local.LocalStatements()
	if err != nil {
		return 0, fmt.Errorf("read the local statements: %w", err)
	}
	var problems []error
	for _, statement := range statements {
		if err := checkLocal(statement); err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", statement.Location.StatementPath(), err))
			continue
		}
		set.AddLocal(statement.Location, statement.Bytes)
	}
	return len(statements), errors.Join(problems...)
}

func checkLocal(statement LocalStatement) error {
	admitted, err := format.Admit(statement.Bytes)
	if err != nil {
		return refusal.Wrap(err)
	}
	if producer := admitted.Proof.Common().ProducedBy; producer != format.ProducedByLocal {
		return refusal.Newf("an unsigned statement is produced by %q, only %q statements are accepted unsigned", producer, format.ProducedByLocal)
	}
	return refusal.Wrap(admitted.CheckLocation(statement.Location))
}

func (s VerifyService) admitPending(set *proofset.Set) (int, error) {
	files, err := s.Pending.Pending()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, file := range files {
		set.AddPending(file)
		count += len(file.Pending)
	}
	return count, nil
}
