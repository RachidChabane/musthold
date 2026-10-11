# archtest requirements

The architecture rules that span the modules of the repository, in Shallnot's
Markdown spec format. `shallnot gate` traces them to the Go tests of this
module in CI.

## Architecture

- **SP-106~2**: WHEN the packages of `format` other than its test support (the
  vector builders and the host protocol test support) are inspected THE SYSTEM
  SHALL import no `os`, `io/fs`, `os/exec` or `net`, and link no package of the
  `net` family.
- **SP-107~1**: WHEN the packages of a module and their tests are inspected THE
  SYSTEM SHALL import no other module than `format`, and every module of the
  repository SHALL be inspected.
- **SP-108~1**: WHEN the binaries of discriminating power, alignment and
  completeness are linked THE SYSTEM SHALL contain none of `net`, `net/http` and
  `crypto/tls`.
- **SP-109~1**: WHEN the packages of every module are inspected THE SYSTEM SHALL
  find `net/http` imported only in `s1relay` and in the signing adapters of
  `ledger` and `verify`.
- **SP-143~1**: WHEN the `format` root, the host protocol and its MCP codec are
  linked THE SYSTEM SHALL leave the root and the host protocol without the MCP
  codec, and SHALL leave all three without test support.
- **SP-144~3**: WHEN the binaries of every module that ships one are linked THE
  SYSTEM SHALL contain no package of the test support of `format`, the format
  library alone shipping none.
- **SP-1830~1**: WHEN the packages of `tools/release` and their tests are
  inspected THE SYSTEM SHALL find no import of a product module, the `format`
  library included, and no non-test package of its domain importing `os`,
  `io/fs`, `os/exec` or a package of the `net` family.
- **SP-39~2**: WHEN the `spec-proof-verify` binary is linked THE SYSTEM SHALL
  hold no symbol of `os/exec`, with and without cgo, no package of the `verify`
  module SHALL import `os/exec`, and the set of packages reachable from the
  binary on linux, darwin and windows that import `os/exec` SHALL equal the
  allowlist of the test, which names `github.com/digitorus/pkcs7`,
  `github.com/google/certificate-transparency-go/x509` and
  `github.com/in-toto/in-toto-golang/in_toto` with a reason each, so that a
  dependency change that adds or removes an importer fails until reviewed.
