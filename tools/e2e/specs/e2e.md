# e2e requirements

The end-to-end requirements of the signing, ledger and verification chain, in
Shallnot's Markdown spec format. They drive the built binaries of `ledger` and
`verify` as black boxes: this module imports neither of them. `shallnot gate`
traces them to the Go tests of this module in CI.

## Proof chain

- **SP-170~1**: WHEN the valid statements of the golden vectors are signed with an
  Ed25519 identity, appended to a bare repository, exported and verified offline
  against a trust policy that pins the identity THE SYSTEM SHALL write bare
  statements equal to the golden ones and a manifest whose members, sort orders
  and digests are the contract of the gate, and SHALL find the ledger append-only
  and content-addressed.
- **SP-171~1**: WHEN a byte of an exported bundle is changed THE SYSTEM SHALL
  reject the verification and keep the verified directory of the earlier run.
- **SP-172~1**: WHEN the same exported ledger is verified twice THE SYSTEM SHALL
  write byte-identical manifests and statements, and WHEN the same statements are
  signed twice THE SYSTEM SHALL produce byte-identical bundles.
- **SP-173~1**: WHEN a proof is appended to a ledger that a second runner
  fast-forwards and publishes THE SYSTEM SHALL verify the proofs of both runners
  from the exported remote ledger.
