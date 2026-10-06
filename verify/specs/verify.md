# verify requirements

The requirements of the `verify` module, which checks signed proofs from files
only, in Shallnot's Markdown spec format. `shallnot gate` traces them to the Go
tests of the module in CI.

## Binary

- **SP-104~1**: WHEN the `spec-proof-verify` binary is run with the `version`
  argument THE SYSTEM SHALL print its version.
