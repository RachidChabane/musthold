# archtest requirements

The architecture rules that span the modules of the repository, in Shallnot's
Markdown spec format. `shallnot gate` traces them to the Go tests of this
module in CI.

## Architecture

- **SP-106~1**: WHEN the packages of `format` other than its test support are
  inspected THE SYSTEM SHALL import no `os`, `io/fs`, `os/exec` or `net`, and
  link no package of the `net` family.
- **SP-107~1**: WHEN the packages of a module and their tests are inspected THE
  SYSTEM SHALL import no other module than `format`, and every module of the
  repository SHALL be inspected.
- **SP-108~1**: WHEN the binaries of discriminating power, alignment and
  completeness are linked THE SYSTEM SHALL contain none of `net`, `net/http` and
  `crypto/tls`.
- **SP-109~1**: WHEN the packages of every module are inspected THE SYSTEM SHALL
  find `net/http` imported only in `s1relay` and in the signing adapters of
  `ledger` and `verify`.
- **SP-39~1**: WHEN the `spec-proof-verify` binary is linked THE SYSTEM SHALL
  contain no `os/exec`.
