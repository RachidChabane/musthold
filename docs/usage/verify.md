# Verify (`spec-proof-verify`)

The verifier turns signed proof bundles, or local unsigned statements, into the
verified set that Shallnot reads. It works from files only: it runs no other
program, calls no model and, with `--offline`, opens no connection.

```
spec-proof-verify version
spec-proof-verify --bundles DIR [--accept-local DIR] [--offline] --out DIR
spec-proof-verify --accept-local DIR --out DIR
```

| Flag | Meaning |
|---|---|
| `--bundles DIR` | The directory `spec-proof-ledger ledger export` wrote. |
| `--accept-local DIR` | A local proof cache whose unsigned statements are admitted; `.spec-proof/cache` for a developer. |
| `--offline` | Read the Sigstore trusted root from the trust snapshot inside `--bundles`, not from the network. Needs `--bundles`. |
| `--out DIR` | The verified directory to write; `.spec-proof/verified` is what Shallnot reads by default. |

`--out` and at least one of `--bundles` and `--accept-local` are required. No
other flag exists.

Exit codes: 0 when the verified set was written, 1 when a bundle, a statement or a
pending file was rejected, 2 for a command line that does not parse or a tool
failure. Every non-zero exit is explained on standard error.

## Trust policy

The verifier reads `trust-policy.yaml` from the directory that holds `--out`, and
the pending directory `pending/` from the same place. For `--out
.spec-proof/verified` these are `.spec-proof/trust-policy.yaml` and
`.spec-proof/pending/`. The file format is in
[configuration](configuration.md#spec-prooftrust-policyyaml).

## Signed bundles

A bundle is admitted only when:

- **DSSE envelope (Ed25519).** One signature of a key pinned in the policy verifies
  over the DSSE pre-authentication encoding of the payload. An unknown key, a
  tampered payload, signature or payload type, and an envelope without a valid
  signature of a pinned key are refused.
- **Sigstore bundle.** The certificate chain leads to the trusted root, a Rekor
  entry gives the time the certificate is checked at, the signature over the DSSE
  envelope verifies, and the certificate carries the pinned issuer, the pinned
  build signer URI and the pinned commit digest. The trusted root is loaded through
  TUF, back to the root embedded in the binary: from the `trust/` snapshot of the
  exported ledger with `--offline`, from the public-good mirror otherwise.
- **Statement.** The statement is canonical, satisfies the format, was produced by
  `ci` and sits at the module, subject key and cache key its content dictates.

When any bundle is rejected the whole run is rejected: nothing is written and the
previous verified directory stays.

## Local statements

`--accept-local DIR` admits the canonical, valid statements of the cache that were
produced by `local`, without a signature, and makes the trust of the manifest
`local`. A statement produced by `ci` in the cache is refused. A signed statement
is kept in place of a local one for the same location without downgrading the
trust. Shallnot reads a `local` manifest only under `proofs.trust: local`.

## Output

The verified directory holds `manifest.json` and the statement files it lists. The
manifest is RFC 8785 canonical JSON: `verifierVersion`, `trust` (`signed` or
`local`), `trustedRootDigest` and `policyDigest` for a run that verifies bundles,
`statements` (path and the SHA-256 of the exact file bytes, sorted by path),
`bundles`, and `pending`, which folds the `<module>.json` files of the pending
directory, sorted by module, criterion and reason. The directory is built beside its
destination and swapped in, so a failed run leaves the earlier directory, and the
same inputs give byte-identical output.

On success it prints `verified N statements (S signed, L local), trust T, P pending,
written to DIR`.
