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

- **SP-412~5**: WHEN both modules are initialized and enabled with their own
  `init` and `enable`, with no other edit of `shallnot.yaml` than theirs, the code is edited, the discriminating-power proof and the
  alignment proof are each made with an honest host through the module's real hooks
  and MCP executor server (the alignment proof from the verified set), and the
  local statements are accepted by the verifier THE SYSTEM
  SHALL give `shallnot check` the verdict the proofs reached for each criterion:
  PASS for the line total, which is adequately tested and aligned; ESCALATE for the
  discount, whose rounding survivors no witness fails and no execution excludes,
  the ids of the parameters of the discount's parametrized tests, which repeat
  numbers of the statement, being no part of the names the alignment module masks
  against; and BLOCK for the shipping fee, which a mutant that violates it survives; none of them
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

## Documentation

These requirements read the user documentation (the README, the READMEs of the
modules and the pages under `docs/usage`) against the binaries built from the
modules, and need no Shallnot and no model. The built binaries are driven as black
boxes; the checks skip with a recorded reason when the memory guard refuses a build.

- **SP-1400~1**: WHEN the documentation shows a command line of a spec-proof binary
  THE SYSTEM SHALL find the command it names among the commands the binary lists,
  and the verb it names among the verbs of that command.
- **SP-1401~1**: WHEN the documentation shows a flag on a command line of a
  spec-proof binary THE SYSTEM SHALL find that flag accepted by that command of the
  binary.
- **SP-1402~1**: WHEN the documentation links to a file, a directory or a heading of
  the repository THE SYSTEM SHALL find the file or directory and, for a Markdown
  page, a heading that has the anchor.

## Closed local loop

- **SP-1670~1**: WHEN each of the six built binaries is run with `-h`, `--help` or
  `help`, each of its commands with `-h` or `--help`, `help` followed by a
  command, and each verb of a command that takes one with `-h` or `--help` THE
  SYSTEM SHALL print a usage on the output stream and exit 0, list for a verb
  only the flags that verb defines, and refuse a flag another verb owns as a usage
  error on the error stream with exit 2.
- **SP-1671~1**: WHEN both proof modules are initialized and enabled in a project
  that has no spec-proof state THE SYSTEM SHALL leave `shallnot.yaml` with both
  proofs required, the trust `local` and the source root of the project as code,
  keep the cache, the verified set and the pending files out of version control,
  write the commented example of the operations block once, and plan no change on
  a second `init`.
- **SP-1672~1**: WHEN the verifier accepts a local cache that holds no statement
  and the pinned Shallnot reads the result under the trust `local` THE SYSTEM
  SHALL write a manifest of trust `local` with no digest of a trusted root or of a
  policy and no statement, and SHALL give no `malformed_proof` finding.

## Scan

- **SP-2124~2**: WHEN `spec-proof scan --budget 0` runs, with no configuration,
  in a pytest project committed in its own repository, one of whose tests runs a
  function and asserts nothing THE SYSTEM SHALL exit 0 and print that test as one
  that verifies nothing, and `spec-proof scan replay` SHALL re-execute the scan's
  findings with every claim held.
