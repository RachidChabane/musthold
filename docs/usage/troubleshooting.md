# Troubleshooting

## Exit codes

### Proof modules (`spec-proof-dp`, `spec-proof-alignment`, `spec-proof-completeness`)

| Command | 0 | 1 | 2 | 3 |
|---|---|---|---|---|
| `prove` (discriminating power) | no requirement blocks or escalates | a requirement blocks or escalates | tool failure, bad flags | tickets wait for the host |
| `prove` (alignment) | every test is ALIGNED | a test is not ALIGNED, or a proof it needs is awaited | tool failure, bad flags | tickets wait |
| `prove` (completeness) | proved, whatever questions were raised | the host stopped answering | tool failure, bad flags | tickets wait |
| `request`, `systemone`, `host`, `mcp`, `hook` | success | a refusal: a protocol or user error such as a wrong nonce, a bad capability, an unknown ticket, an unknown role or hook, nothing to push | tool failure | not used |
| `doctor` | every check is `ok` | not used | a check failed | not used |
| `init`, `enable`, `disable` | done | a refusal (Shallnot too old, no `shallnot.yaml`, a layout it cannot extend), or with `--check` a file would change | tool failure | not used |
| `version` | printed | not used | not used | not used |

### Other programs

| Program | 0 | 1 | 2 | 4 |
|---|---|---|---|---|
| `spec-proof-ledger` | success | a statement, path or ledger that breaks a rule, `ledger audit` found a violation | command line or tool failure | not used |
| `spec-proof-verify` | verified set written | a bundle, statement or pending file was rejected | command line or tool failure | not used |
| `spec-proof-s1 drain` | no request unanswered | some requests unanswered | tool failure, including no usable endpoint | the memory rule refused |

Shallnot's exit codes: 0 clean, 1 blocked, 2 tool failure.

## Pending reasons

A pending reason says that a proof is not finished. The gate never counts a pending
proof as a pass.

| Reason | Meaning | What to do |
|---|---|---|
| `awaiting_host` | A module has tickets the host has not answered. | Run the module's fulfil skill (or `prove-all`); or `prove` and answer the tickets. |
| `awaiting_proof` | A proof the module consumes is not there yet. Alignment reports it for a test while the discriminating-power proof of the same requirement is pending. | Finish the discriminating-power proof. |
| `awaiting_host_judgment` | CI drew a surviving mutant that needs a judgment of the host. | `host pull --ci-run ID`, answer, `host push`, re-run CI ([CI](ci.md)). |
| `host_unresponsive` | The host did not answer after every emission (3 by default). | Report it. Do not loop: check `doctor`, that the plugin and its hooks are installed, and the journal. |

`awaiting_host`, `awaiting_proof` and `awaiting_host_judgment` send the agent back
to work; Shallnot's end-of-turn hook lets a turn end after a bounded number of
consecutive blocks whose only findings are pending reasons.

## Common findings

| Finding | Meaning | Fix |
|---|---|---|
| `stale_proof` | A required proof is missing or stale: an input file changed, a bound test file is not among the proof's inputs, the statement changed, or a consumed proof is not valid. | Run the command the message names, or `prove-all`, then `spec-proof-verify --accept-local ...`. Never edit a proof file. |
| `malformed_proof` | The manifest or a statement breaks the format. | Re-run `spec-proof-verify`. A `local` manifest needs `proofs.trust: local`. |
| `correct_code_fails`, `example_failed`, `surviving_valid_mutant` | The tests or the code do not meet the requirement. | Fix the code, or strengthen the tests until they fail on the mutant. |
| `escalated_mutant`, `no_valid_mutant`, `no_killed_valid_mutant`, `mutation_sensitive_test`, `misaligned_test` | A judged signal that the tests may not check the requirement. | Make the tests check the statement. |
| `completeness_question` | An advisory question about the spec. | Raise it with the spec's owner. |

## Symptoms

**`init` or `enable` fails with "shallnot must speak report schema 1.2 or later".**
The `shallnot` on `PATH` predates report schema 1.2. Install a Shallnot that has it,
or point `SPEC_PROOF_SHALLNOT` at one. `doctor` shows the `shallnot` path and the
`shallnot-schema` check.

**`prove` stays at exit 3.** Tickets are unanswered. `spec-proof-dp request list`
shows them. Each run of `prove`, including `--resume`, advances one wave; answers
must exist before `--resume` can consume them.

**The executor server refuses every answer, or tickets never leave `awaiting_host`.**
The harness hooks did not issue a capability. Check that the module's plugin is
installed and enabled (`.claude/settings.json` lists it), that `spec-proof-dp` is on
`PATH` for the hooks, and `spec-proof-dp doctor`. The journal in
`$XDG_STATE_HOME/spec-proof/<module>/<repo-digest>/host/journal.jsonl` records
`exposure_attempt` and related events.

**`prove --resume` says "nothing to resume: no request was ever emitted".** Run
`prove` first.

**`--ci needs the CI run`.** Set `SPEC_PROOF_CI_RUN_ID` or `GITHUB_RUN_ID`.

**`--import-host needs --ci`**, **`--changed and --all exclude each other`**,
**`--record and --replay exclude each other`**. These flag combinations are refused
before anything runs.

**`host push` exits 1 with "the host store holds no answered request to push".**
Answer the tickets first.

**`host_version_skew`.** The bundle was pushed by another version of the module than
the one CI runs and requests were left unanswered. The message names both versions.
Use the same version of the module on the laptop and in CI.

**Statements are stale after an unrelated edit.** A proof is stale when a file it
names changed. A module computes the footprint of a requirement; an edit outside it
leaves the proof valid. A new test file that holds a test bound to the requirement
makes the discriminating-power proof, and the alignment proofs that consume it,
stale.

## Inspecting state

- `spec-proof-dp doctor` checks Shallnot, the ecosystem, the host store and the
  journal chain.
- `spec-proof-dp request list --json` shows every pending ticket with its role.
- `.spec-proof/cache/` holds the statements, `.spec-proof/verified/manifest.json`
  the verified set, `.spec-proof/pending/` the pending files.
- `spec-proof-ledger ledger audit` checks a ledger.
