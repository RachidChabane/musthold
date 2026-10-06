# ledger requirements

The requirements of the `ledger` module, which signs proofs and keeps them, in
Shallnot's Markdown spec format. `shallnot gate` traces them to the Go tests of
the module in CI.

## Binary

- **SP-103~1**: WHEN the `spec-proof-ledger` binary is run with the `version`
  argument THE SYSTEM SHALL print its version.

## Signing

- **SP-48~1**: WHEN a statement is wrapped for signing THE SYSTEM SHALL carry
  the exact canonical bytes as the payload of a DSSE envelope inside a bundle.
