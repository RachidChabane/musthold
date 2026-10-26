# verify (`musthold-verify`)

Verifies signed proof bundles, or accepts local unsigned statements, and writes the
verified set that Shallnot reads. It works from files only: it runs no other program,
calls no model and, with `--offline`, opens no connection.

## Alone

Needs the bundles of an exported ledger (`--bundles`), a local proof cache
(`--accept-local`), or both, and a trust policy for signed bundles.

## Install

```
nice -n 10 go install -C verify -p 1 ./cmd/musthold-verify
```

Run from the repository root; the binary lands in `$GOBIN`, or
`$(go env GOPATH)/bin`.

## Commands

```
musthold-verify --accept-local .musthold/cache --out .musthold/verified
musthold-verify --bundles bundles --offline --out .musthold/verified
```

Exit codes: 0 the verified set was written, 1 a bundle, statement or pending file
was rejected, 2 a command line or tool failure.

## Documentation

- [Verify](../docs/usage/verify.md)
- [Configuration of the trust policy](../docs/usage/configuration.md#mustholdtrust-policyyaml)
- [CI](../docs/usage/ci.md)
- [Project README](../README.md)
