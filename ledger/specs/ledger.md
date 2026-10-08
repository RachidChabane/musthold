# ledger requirements

The requirements of the `ledger` module, which signs proofs and keeps them, in
Shallnot's Markdown spec format. `shallnot gate` traces them to the Go tests of
the module in CI.

## Binary

- **SP-103~1**: WHEN the `spec-proof-ledger` binary is run with the `version`
  argument THE SYSTEM SHALL print its version.
- **SP-209~1**: WHEN a command ends THE SYSTEM SHALL exit 0 when it succeeded, 1
  when it refused a statement, a path or a ledger that breaks a rule, and 2 for a
  command line that does not parse or a failure of the tool, and SHALL explain
  every non-zero exit on the error stream.
- **SP-1650~1**: WHEN the binary is run with `-h`, `--help` or `help`, a command or
  subcommand with `-h` or `--help`, `ledger` with a request for help, or `help`
  followed by a command THE SYSTEM SHALL print the forms of the binary, the
  subcommands of `ledger`, or the flags and the synopsis of that command, on the
  output stream and exit 0, listing exactly the flags that command defines; and
  WHEN a flag is unknown THE SYSTEM SHALL print the failure and the usage of that
  command on the error stream and exit 2.

## Signing

- **SP-48~1**: WHEN a statement is wrapped for signing THE SYSTEM SHALL carry
  the exact canonical bytes as the payload of a DSSE envelope inside a bundle.
- **SP-200~1**: WHEN statements are signed THE SYSTEM SHALL admit only statements
  that are canonical, satisfy the invariants of the format and its JSON Schema,
  are produced by `ci`, and sit at the module, subject key and cache key their
  content dictates; SHALL compare them with the digest manifest when one is given
  and refuse a missing, extra or altered statement; and SHALL sign and write
  nothing when any statement is refused.
- **SP-201~1**: WHEN the identity is `ed25519` THE SYSTEM SHALL sign the DSSE
  pre-authentication encoding of the exact statement bytes with the standard
  library, read the PKCS#8 PEM key from the environment or from a file readable
  by its owner only, and refuse a key given both ways, a missing key, a key file
  that group or others can read and a key that is not Ed25519.
- **SP-202~1**: WHEN the identity is `sigstore` THE SYSTEM SHALL sign each
  statement with a fresh ephemeral key, put the Fulcio certificate and the Rekor
  entry in the bundle, take the identity token from the environment or from the
  GitHub Actions token service, and refuse to sign without a token.

## Ledger

- **SP-203~1**: WHEN signed proofs are appended THE SYSTEM SHALL write
  `proofs/<module>/<ID>~<REV>/<cacheKey>.dsse.json` on the orphan branch
  `spec-proof/ledger` with git plumbing only, as one commit that fast-forwards
  the branch, accept the same bytes at an existing path as a no-op, and refuse
  the whole run when an existing path would get other bytes, when a bundle does
  not carry an admitted CI-produced statement at its own path, or when two
  additions meet at one path.
- **SP-204~1**: WHEN mutants, seeds and the trust snapshot are appended THE
  SYSTEM SHALL accept `mutants/<cacheKey>.age` only as age ciphertext, record
  `seeds/<head sha>.json` once with the seed that the head commit and the first
  run give and keep the first record, and store a trust snapshot under
  `trust/<snapshot id>/`, the id being the SHA-256 of its timestamp file, with a
  target named by the digest of its content, so that a refreshed snapshot lands at
  paths of its own and no path is rewritten.
- **SP-205~1**: WHEN a remote is given THE SYSTEM SHALL fetch its ledger branch
  and fast-forward the local one to it, keep a local branch that is ahead, refuse
  a branch that has diverged, push without forcing and push again a branch that
  an earlier run could not publish.
- **SP-206~1**: WHEN the ledger is exported THE SYSTEM SHALL write the files
  below `proofs/` of the tip and the files of its trust snapshot with the highest
  timestamp version, flat below `trust/` and chosen by the greater snapshot id
  among equal versions, into an absent or empty directory, and refuse a directory
  that holds files.
- **SP-226~1**: WHEN a trust snapshot is fetched THE SYSTEM SHALL update the TUF
  metadata of the trusted root from the root embedded in the library, verifying
  every signature, expiry and hash, write every file the TUF client read in the
  layout the mirror serves into an absent or empty directory, with a timestamp
  file, and write nothing for a mirror that does not verify, so that
  `ledger append --trust` can store it.
- **SP-207~1**: WHEN the ledger is audited THE SYSTEM SHALL report a merge commit
  or a history that has not exactly one root, every path some commit deleted or
  modified, and every file that is not what its name says.
- **SP-208~1**: WHEN the seed of a head commit is asked THE SYSTEM SHALL print the
  seed the ledger records for it, and otherwise the seed the given run would
  record, which is the SHA-256 of the head commit and the run joined by a newline.

## Release version

- **SP-1854~1**: WHEN `spec-proof-ledger` is run with the lone argument `--version` THE SYSTEM
  SHALL print the bare version, the one a release build linked with
  `-X main.version`, else the one `go install` recorded, else `dev`, and the
  `version` command SHALL print the binary name, that version and the linked
  commit when the build has one.
