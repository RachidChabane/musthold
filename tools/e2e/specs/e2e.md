# e2e requirements

The end-to-end requirements of the signing, ledger and verification chain, in
Shallnot's Markdown spec format. They drive the built binaries of `ledger` and
`verify` as black boxes: this module imports neither of them. `shallnot gate`
traces them to the Go tests of this module in CI.

## Proof chain

- **SP-219~1**: WHEN the valid statements of the golden vectors are signed with an
  Ed25519 identity, appended to a bare repository, exported and verified offline
  against a trust policy that pins the identity THE SYSTEM SHALL write bare
  statements equal to the golden ones and a manifest whose members, sort orders
  and digests are the contract of the gate, and SHALL find the ledger append-only
  and content-addressed.
- **SP-220~1**: WHEN a byte of an exported bundle is changed THE SYSTEM SHALL
  reject the verification and keep the verified directory of the earlier run.
- **SP-221~1**: WHEN the same exported ledger is verified twice THE SYSTEM SHALL
  write byte-identical manifests and statements, and WHEN the same statements are
  signed twice THE SYSTEM SHALL produce byte-identical bundles.
- **SP-222~1**: WHEN a proof is appended to a ledger that a second runner
  fast-forwards and publishes THE SYSTEM SHALL verify the proofs of both runners
  from the exported remote ledger.

## Proofs and the gate

These requirements drive the built binaries of the discriminating-power,
alignment and verify modules and the pinned Shallnot over a copy of the parcel
pytest project, and need that Shallnot: without it the tests record a skip that
names the reason, and the gate reports the skipped requirement.

- **SP-412~2**: WHEN both modules are initialized and enabled with their own
  `init` and `enable`, the code is edited, the discriminating-power proof and the
  alignment proof are each made with an honest host through the module's real hooks
  and MCP executor server (the alignment proof from the verified set), and the
  local statements are accepted by the verifier THE SYSTEM
  SHALL give `shallnot check` the verdict the proofs reached for each criterion:
  PASS for the line total, which is adequately tested and aligned; ESCALATE for the
  discount, whose parametrized tests have ids that repeat numbers of the statement
  and so are refused as `masked_input_leaks` by the alignment module; and BLOCK for
  the shipping fee, which a mutant that violates it survives; none of them
  `stale_proof` or `malformed_proof`, with the digests the alignment proofs record
  for the consumed discriminating-power statements equal to the digests of the
  manifest.
- **SP-413~1**: WHEN an alignment host judges every assertion too weak THE SYSTEM
  SHALL give `shallnot check` an ESCALATE for the criteria whose tests it judged,
  and a block for the weak criterion.
- **SP-414~1**: WHEN a file outside every footprint is edited THE SYSTEM SHALL keep
  every proof valid; WHEN a footprint file is edited THE SYSTEM SHALL make the
  proofs of the criteria that depend on it stale; WHEN a test file that holds a
  test bound to a criterion is added THE SYSTEM SHALL make the discriminating-power
  proof of that criterion stale, and the alignment proofs that consume it, and
  keep the alignment proofs of the other criteria valid.
- **SP-415~1**: WHEN a forger answers every ticket of both modules from the command
  line THE SYSTEM SHALL give no criterion a PASS, in the modules or in the gate.

## Alignment through the host

These requirements drive the built binary of the alignment module alone, with the
pinned Shallnot and a simulated Claude Code over a copy of the parcel pytest
project, and need that Shallnot: without it the tests record a skip that names the
reason, and the gate reports the skipped requirement.

- **SP-840~1**: WHEN the alignment module is initialized and enabled with its own
  `init` and `enable` and the code is edited THE SYSTEM SHALL let a simulated
  Claude Code host answer every ticket through the module's real hooks and
  executor server, journal each executor as started, fetched, accepted and
  attested with no leak, exposure or conflict, write the alignment Statements, and
  deny and journal a call of an executor tool made by the main session.
- **SP-841~1**: WHEN executors run whose hooks never fire THE SYSTEM SHALL give
  them no capability, refuse their fetch and their answer at the executor server,
  keep every ticket pending with exit code 3, accept no answer, and have
  `shallnot check` refuse the project.
- **SP-842~1**: WHEN a forger answers every ticket of the alignment module from
  the command line THE SYSTEM SHALL refuse or journal its answers as an exposure, give no test
  the verdict ALIGNED and no criterion a PASS, and have `shallnot check` refuse the
  project.
