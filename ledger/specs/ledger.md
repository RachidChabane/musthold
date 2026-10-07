# ledger requirements

The requirements of the `ledger` module, which signs proofs and keeps them, in
Shallnot's Markdown spec format. `shallnot gate` traces them to the Go tests of
the module in CI.

## Binary

- **SP-103~1**: WHEN the `spec-proof-ledger` binary is run with the `version`
  argument THE SYSTEM SHALL print its version.
- **SP-160~1**: WHEN a command ends THE SYSTEM SHALL exit 0 when it succeeded, 1
  when it refused a statement, a path or a ledger that breaks a rule, and 2 for a
  command line that does not parse or a failure of the tool, and SHALL explain
  every non-zero exit on the error stream.

## Signing

- **SP-48~1**: WHEN a statement is wrapped for signing THE SYSTEM SHALL carry
  the exact canonical bytes as the payload of a DSSE envelope inside a bundle.
- **SP-151~1**: WHEN statements are signed THE SYSTEM SHALL admit only statements
  that are canonical, satisfy the invariants of the format and its JSON Schema,
  are produced by `ci`, and sit at the module, subject key and cache key their
  content dictates; SHALL compare them with the digest manifest when one is given
  and refuse a missing, extra or altered statement; and SHALL sign and write
  nothing when any statement is refused.
- **SP-152~1**: WHEN the identity is `ed25519` THE SYSTEM SHALL sign the DSSE
  pre-authentication encoding of the exact statement bytes with the standard
  library, read the PKCS#8 PEM key from the environment or from a file readable
  by its owner only, and refuse a key given both ways, a missing key, a key file
  that group or others can read and a key that is not Ed25519.
- **SP-153~1**: WHEN the identity is `sigstore` THE SYSTEM SHALL sign each
  statement with a fresh ephemeral key, put the Fulcio certificate and the Rekor
  entry in the bundle, take the identity token from the environment or from the
  GitHub Actions token service, and refuse to sign without a token.

## Ledger

- **SP-154~1**: WHEN signed proofs are appended THE SYSTEM SHALL write
  `proofs/<module>/<ID>~<REV>/<cacheKey>.dsse.json` on the orphan branch
  `spec-proof/ledger` with git plumbing only, as one commit that fast-forwards
  the branch, accept the same bytes at an existing path as a no-op, and refuse
  the whole run when an existing path would get other bytes, when a bundle does
  not carry an admitted CI-produced statement at its own path, or when two
  additions meet at one path.
- **SP-155~1**: WHEN mutants, seeds and the trust snapshot are appended THE
  SYSTEM SHALL accept `mutants/<cacheKey>.age` only as age ciphertext, record
  `seeds/<head sha>.json` once with the seed that the head commit and the first
  run give and keep the first record, and store the trust snapshot under `trust/`
  with a target named by the digest of its content.
- **SP-156~1**: WHEN a remote is given THE SYSTEM SHALL fetch its ledger branch
  and fast-forward the local one to it, keep a local branch that is ahead, refuse
  a branch that has diverged, push without forcing and push again a branch that
  an earlier run could not publish.
- **SP-157~1**: WHEN the ledger is exported THE SYSTEM SHALL write the files
  below `proofs/` and `trust/` of the tip into an absent or empty directory and
  refuse a directory that holds files.
- **SP-158~1**: WHEN the ledger is audited THE SYSTEM SHALL report a merge commit
  or a history that has not exactly one root, every path some commit deleted or
  modified, and every file that is not what its name says.
- **SP-159~1**: WHEN the seed of a head commit is asked THE SYSTEM SHALL print the
  seed the ledger records for it, and otherwise the seed the given run would
  record, which is the SHA-256 of the head commit and the run joined by a newline.
