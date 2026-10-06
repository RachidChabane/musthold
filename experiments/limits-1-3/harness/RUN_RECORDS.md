# Run records

Data contract between the pipeline verbs (`run-core`, `run-secondary`), which
write a run directory, and `analysis`, which reads it. Paths are relative to
`runs/<run-id>/`. JSON files are UTF-8, keys sorted; `.jsonl` files hold one
JSON object per line, sorted by their key fields. Criterion references are
`PARK-n~1`; test identifiers are pytest node ids relative to the fixture root
(`tests/test_tariff.py::test_name`). Model-call identifiers (`call_id`) name a
directory under `raw/`.

| File | Writer | Content |
|---|---|---|
| `manifest.json` | both verbs | `{run_id, prereg_commit, harness_digest, fixture_digest, cli_versions, models}` |
| `DEVIATIONS.md` | both verbs | appended log of logged deviations |
| `raw/<call_id>/` | model adapters | verbatim request, response, argv, stderr, `result.json` |
| `canary/<model>__<cli>__<call_type>.json` | `canary` | `{input_tokens, num_turns, cli_version}` |
| `traceability.json` | `run-core` | the Shallnot JSON report |
| `bindings.json` | `run-core` | `{nodeid: ref}` from the Shallnot report |
| `reference.json` | `run-core` | `{runs: [{nodeid: outcome}] × 3, passed_all: bool}` |
| `mutants/<mutant_id>.json` | `run-core` (T, P), `run-secondary` (O) | `{mutant_id, arm: T\|P\|O, target: ref\|null, slot: S1\|S2\|S3\|null, generator: {model, cli, call_id, nonce}\|null, edits, normalized_edits, mechanism_summary\|null, merged_from: [mutant_id]}` |
| `planted_map.json` | `run-core` | `{mutant_id: planted source id}`; read only by `analysis` |
| `processing.jsonl` | both | `{mutant_id, stillborn: bool, stillborn_reason\|null, status: OK\|TRIVIALLY_EQUIVALENT\|DUPLICATE, duplicate_of\|null, changed_lines}` |
| `kills.jsonl` | both | `{mutant_id, results: {nodeid: killed\|survived\|error\|flaky}}` for non-stillborn, status OK mutants |
| `differential.jsonl` | both | `{mutant_id, status: DIFFERS\|NO_DIFF_FOUND\|SKIPPED, function\|null, args\|null, original_out\|null, mutant_out\|null, reason\|null}` |
| `validity.jsonl` | `run-core` (V-S, V-G), `run-secondary` (A-OPUS) | `{mutant_id, judge: V-S\|V-G\|A-OPUS, call_id, status: ok\|missing, payload\|null}` |
| `witness.jsonl` | same as validity | `{mutant_id, judge, criterion, confirmed: bool, reference_out, mutant_out, error\|null}` |
| `mutant_classes.json` | `run-core` | per mutant (T, P): `{mutant_id, arm, target, per_criterion: {ref: VALID\|NOT_VIOLATING\|UNDETERMINED}, valid_confidence: {ref: number}, equivalent: bool, unattributed: bool, off_target: bool, collateral: bool, primary: {included: bool, criterion: ref\|null}, discarded: bool, survived_all: bool}` |
| `audit_sample.json` | `run-secondary` | `{seed, mutant_ids: [...], strata: {mutant_id: stratum}}` |
| `alignment.jsonl` | `run-core` (J-SONNET masked), `run-secondary` (others) | per call `{judge: J-SONNET\|J-GPT, arm: NAME-MASKED\|NAME-SHOWN, nodeid, rep, call_id, status, verifies_ref: ref\|NONE\|null, coverage\|null, confidence\|null, rationale\|null}` |
| `alignment_aggregate.json` | both | per `(judge, arm, nodeid)`: `{modal_ref, modal_share, modal_coverage, mean_confidence, verdict: ALIGNED\|MISALIGNED\|UNDETERMINED, verdict_s1, missing_reps}` |
| `token_maps/<judge>/<arm>/<nodeid-slug>/rep-<k>.json` | both | `{token: ref}` |
| `probe.jsonl` | `run-secondary` | `{judge, nodeid, rep, call_id, status, guess\|null, confidence\|null}` |
| `relabel.jsonl` | `run-secondary` | `{rater: R-OAI\|R-AUTHOR, nodeid, label, checks}` |
| `proofs/alignment/<judge>/<arm>/<nodeid-slug>.intoto.json` | both | ADR 0001 alignment Statement |
| `proofs/discriminating-power/<ref>.intoto.json` | `run-core` | ADR 0001 discriminating-power Statement (arm T) |
| `timing.json` | both | `{stage: {started, finished, seconds}}` |

A node-id slug replaces every character outside `[A-Za-z0-9_.-]` with `_`.
`mutant_id` is the first 16 hex characters of the SHA-256 of the canonical JSON
of the normalized edit set.
