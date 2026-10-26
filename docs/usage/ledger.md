# Ledger (`musthold-ledger`)

The ledger module signs the proofs a CI run produced and keeps them, with the data
needed to verify them offline, in an append-only git branch. It runs in CI. It
calls no model and reads no project code.

```
musthold-ledger version
musthold-ledger sign --identity ed25519|sigstore --in DIR --out DIR [--digests FILE] [--key-file FILE] [--fulcio-url URL] [--rekor-url URL]
musthold-ledger ledger append --signed DIR [--mutants DIR] [--trust DIR] [--head-sha SHA --run-id ID] [--repo DIR] [--remote NAME]
musthold-ledger ledger export --to DIR [--repo DIR]
musthold-ledger ledger audit [--repo DIR]
musthold-ledger ledger seed --head-sha SHA --run-id ID [--repo DIR]
musthold-ledger ledger fetch-trust --out DIR [--mirror URL]
```

Exit codes: 0 success, 1 a refusal (a statement, a path or a ledger that breaks a
rule), 2 a command line that does not parse or a failure of the tool. Every
non-zero exit is explained on standard error.

`-h`, `--help` and `help` print the forms above and exit 0, `ledger -h` lists the
subcommands of `ledger`, and `<command> -h` or `ledger <subcommand> -h` prints the
flags of that command only. A flag a command does not define prints the failure and
the usage of the command on standard error and exits 2.

## sign

Reads statements from `--in` in the layout of the proof cache,
`<module>/<subject key>/<cacheKey>.json`, and writes one DSSE bundle per
statement to `--out`.

A statement is admitted only when it is canonical (RFC 8785), satisfies the
invariants and JSON Schema of the proof format, was produced by `ci`, and sits at
the module, subject key and cache key its content dictates. With `--digests FILE`
(a `sha256sum` manifest) every statement must match it: a missing, extra or altered
statement is refused. When any statement is refused nothing is signed or written.

Identities:

- `ed25519` signs the DSSE pre-authentication encoding of the exact statement bytes.
  The PKCS#8 PEM key comes from `MUSTHOLD_ED25519_KEY` or from `--key-file`
  (readable by its owner only), never both. A missing key, a key file that group or
  others can read and a key that is not Ed25519 are refused.
- `sigstore` signs each statement with a fresh ephemeral key, puts the Fulcio
  certificate and the Rekor entry in the bundle, and takes its identity token from
  `SIGSTORE_ID_TOKEN` or from the GitHub Actions token service. It refuses to sign
  without a token. `--fulcio-url` and `--rekor-url` default to the public-good
  instance.

## ledger append

Adds files to the orphan branch `musthold/ledger` of the repository `--repo`
(default `.`), with git plumbing only, as one commit that fast-forwards the
branch:

- `proofs/<module>/<ID>~<REV>/<cacheKey>.dsse.json` for each bundle of `--signed`;
- `mutants/<cacheKey>.age` for each file of `--mutants`, accepted only as age
  ciphertext;
- `seeds/<head sha>.json`, recorded once, with `--head-sha` and `--run-id` together;
  the first record wins;
- `trust/<snapshot id>/` for the TUF snapshot of `--trust`.

The same bytes at an existing path are a no-op. The whole run is refused when an
existing path would get other bytes, when a bundle does not carry an admitted
CI-produced statement at its own path, or when two additions meet at one path.
With `--remote NAME` the module fetches that remote's ledger branch, fast-forwards
the local one to it, keeps a local branch that is ahead, refuses a branch that has
diverged, pushes without forcing and pushes again a branch an earlier run could not
publish.

## ledger export

Writes the files below `proofs/` of the ledger tip, and the files of the trust
snapshot with the highest timestamp version, flat below `trust/`, into `--to`. The
directory must be absent or empty. This is what `musthold-verify --bundles`
reads.

## ledger audit

Checks the ledger is append-only and content-addressed. It reports a merge commit
or a history without exactly one root, every path some commit deleted or modified,
and every file that is not what its name says. Exit 1 when it finds any.

## ledger seed

Prints the seed the ledger records for `--head-sha`, and otherwise the seed the
given run would record: the SHA-256 of the head commit and the run joined by a
newline. The seed fixes the operator mutants and sentinels of a commit, so
re-running CI cannot shop for a favourable draw.

## ledger fetch-trust

Updates the TUF metadata of the Sigstore trusted root from the root embedded in
the binary, verifying every signature, expiry and hash, and writes the files it
read into `--out`, with a timestamp file, ready for `ledger append --trust`.
`--mirror` defaults to the public-good mirror. Nothing is written for a mirror
that does not verify.

See [CI](ci.md) for where these commands sit in a workflow and [trust
model](trust-model.md) for what signing guarantees.
