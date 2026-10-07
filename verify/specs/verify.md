# verify requirements

The requirements of the `verify` module, which checks signed proofs from files
only, in Shallnot's Markdown spec format. `shallnot gate` traces them to the Go
tests of the module in CI.

## Binary

- **SP-104~1**: WHEN the `spec-proof-verify` binary is run with the `version`
  argument THE SYSTEM SHALL print its version.
- **SP-218~1**: WHEN the binary is run THE SYSTEM SHALL take `--bundles DIR`,
  `--accept-local DIR`, `--offline` and `--out DIR` and nothing else, require
  `--out` and at least one of `--bundles` and `--accept-local`, read the trust
  policy `trust-policy.yaml` and the pending directory from the directory that
  holds `--out`, and exit 0 when the verified set was written, 1 when a bundle,
  a statement or a pending file was rejected, and 2 for a command line that does
  not parse or a failure of the tool, explaining every non-zero exit on the error
  stream.

## Trust policy

- **SP-210~1**: WHEN the trust policy is read THE SYSTEM SHALL accept only the
  version 1 document whose members it knows, pinning Ed25519 public keys as
  base64 raw keys and, optionally, a Sigstore issuer with build signers that each
  carry a URI and a lowercase hex commit digest, SHALL refuse a policy without
  trust anchor, a malformed or repeated key or signer, and SHALL digest the exact
  bytes of the file.

## Verification

- **SP-211~1**: WHEN a bundle is a DSSE envelope THE SYSTEM SHALL accept it only
  when one signature of a key pinned in the policy verifies over the DSSE
  pre-authentication encoding of the payload with the standard library, and
  SHALL refuse an unknown key, a tampered payload, signature or payload type, and
  an envelope without a valid signature of a pinned key.
- **SP-212~1**: WHEN a bundle is a Sigstore bundle THE SYSTEM SHALL accept it
  only when the certificate chain leads to the trusted root, a Rekor entry gives
  the time the certificate is checked at, the signature over the DSSE envelope
  verifies, and the certificate carries the pinned issuer, the pinned build
  signer URI as its subject alternative name and as its build signer, and the
  pinned commit digest, SHALL return the exact statement bytes of the envelope,
  and SHALL refuse another signer, issuer, digest or authority, a bundle without
  certificate or log entry, a changed byte and a payload swapped under the
  signature.
- **SP-213~1**: WHEN a signed bundle is admitted THE SYSTEM SHALL require its
  statement to be canonical, to satisfy the format, to be produced by `ci` and to
  sit at the module, subject key and cache key its content dictates, SHALL route
  each bundle to the verifier of its signature scheme and refuse a scheme the
  policy does not pin, and SHALL reject the whole run, writing nothing and
  keeping the previous verified directory, when any bundle is rejected.
- **SP-214~1**: WHEN `--accept-local` names a cache THE SYSTEM SHALL admit its
  canonical valid statements produced by `local` without a signature, make the
  trust of the manifest `local`, refuse a statement produced by `ci` in the
  cache, and keep a signed statement in place of a local one for the same
  location without downgrading the trust.

## Trusted root

- **SP-223~1**: WHEN a policy pins a Sigstore signer THE SYSTEM SHALL load the
  trusted root through TUF, verifying every signature, expiry and hash back to
  the root embedded in the binary, from the `trust/` snapshot of the exported
  ledger without a connection when `--offline` is given and from the public-good
  mirror otherwise, keep the metadata in memory, state the SHA-256 of the exact
  bytes of the trusted root in the manifest, and refuse a missing, tampered,
  unsigned or foreign snapshot; and WHEN the policy pins no Sigstore signer THE
  SYSTEM SHALL read no snapshot and state the digest of the empty input.
- **SP-224~1**: WHEN the binary is built THE SYSTEM SHALL embed the TUF root of the
  public-good Sigstore instance and hand it out as a copy.

## Verified set

- **SP-215~1**: WHEN the manifest is written THE SYSTEM SHALL state, in RFC 8785
  canonical form, `verifierVersion`, `trust` (`signed` or `local`),
  `trustedRootDigest` and `policyDigest` for a run that verifies bundles,
  `statements` as path and the SHA-256 of the exact file bytes sorted by path,
  `bundles` as the signed files they came from, and `pending` sorted by module,
  criterion and reason, with empty lists in place of null.
- **SP-216~1**: WHEN the pending directory holds `<module>.json` files THE SYSTEM
  SHALL fold their entries into the `pending` list of the manifest, treat an
  absent directory as nothing pending, and reject a file that is not named by
  its module, that is not JSON or that breaks the pending format.
- **SP-217~1**: WHEN the verified directory is written THE SYSTEM SHALL build it
  beside its destination and swap it in, leave no file of an earlier run and no
  staging directory behind, keep the previous directory when the write fails, and
  give a byte-identical manifest when the same inputs are verified again.
