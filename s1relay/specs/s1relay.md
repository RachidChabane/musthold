# s1relay requirements

The requirements of the `s1relay` module, the only module that reaches a System
One endpoint, in Shallnot's Markdown spec format. `shallnot gate` traces them
to the Go tests of the module in CI.

## Binary

- **SP-105~1**: WHEN the `spec-proof-s1` binary is run with the `version`
  argument THE SYSTEM SHALL print its version.
