# ledger (`spec-proof-ledger`)

Signs the proofs a CI run produced and keeps them in an append-only git branch,
`spec-proof/ledger`, with the data needed to verify them offline. It runs in CI,
calls no model and reads no project code.

## Alone

Needs only `git` and the statements to sign. Signs with an Ed25519 key or with a
Sigstore identity.

## Install

```
nice -n 10 go install -C ledger -p 1 ./cmd/spec-proof-ledger
```

Run from the repository root; the binary lands in `$GOBIN`, or
`$(go env GOPATH)/bin`.

## Commands

```
spec-proof-ledger sign --identity ed25519 --in .spec-proof/cache --out signed
spec-proof-ledger ledger append --signed signed --head-sha SHA --run-id ID
spec-proof-ledger ledger export --to bundles
spec-proof-ledger ledger audit
spec-proof-ledger ledger seed --head-sha SHA --run-id ID
spec-proof-ledger ledger fetch-trust --out trust
```

Exit codes: 0 success, 1 a refusal, 2 a command line or tool failure.

## Documentation

- [Ledger](../docs/usage/ledger.md)
- [CI](../docs/usage/ci.md)
- [Trust model](../docs/usage/trust-model.md)
- [Project README](../README.md)
