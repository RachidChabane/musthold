# Run records

Data contract between the run verbs (`canary`, `import-run-01`, `relabel`, `run-core`,
`run-blindspot`, `audit-sample`, `run-secondary`, `run-luna`, `log-deviation`), which write a run directory,
and `analysis`, which reads it. Paths are relative to
`runs/<run-id>/`. JSON files are UTF-8, keys sorted; `.jsonl` files hold one
JSON object per line, sorted by their key fields. Criterion references are
`PARK-n~1`; test identifiers are pytest node ids relative to the fixture root
(`tests/test_tariff.py::test_name`). Model-call identifiers (`call_id`) name a
directory under `raw/`.

| File | Writer | Content |
|---|---|---|
| `manifest.json` | `relabel`, `run-core`, `run-blindspot`, `run-secondary`, `run-luna` | `{run_id, prereg_tag, prereg_commit, superseded_prereg_tag, superseded_prereg_commit, imports_digest, harness_digest, fixture_digest, cli_versions, models}`; the commits are the commits the tags peel to |
| `DEVIATIONS.md` | `relabel`, `run-core`, `run-blindspot`, `run-secondary`, `run-luna`, `canary`, `log-deviation` | title `# Deviations`, then one entry per line: `- <kind> \| endpoints: <scope> \| <text>`; kinds and scopes as in PREREG 4.4 |
| `raw/<call_id>/` | model adapters | verbatim request, response, argv, stderr, `result.json` |
| `canary/<model>__<cli>__<call_type>.json` | `canary` | `{input_tokens, num_turns, cli_version}` |
| `traceability.json` | `run-core` | the Shallnot JSON report |
| `bindings.json` | `run-core` | `{nodeid: ref}` from the Shallnot report |
| `reference.json` | `run-core` | `{runs: [{nodeid: outcome}] × 3, passed_all: bool}` |
| `mutants/<mutant_id>.json` | `run-core` (T, P), `run-blindspot` (X, R, O), `run-luna` (L) | `{mutant_id, arm: T\|P\|O\|X\|R\|L, target: ref\|null, slot: S1\|S2\|S3\|null, generator: {model, cli, call_id, nonce}\|null, edits, normalized_edits, mechanism_summary\|null, merged_from: [mutant_id]}` |
| `processing.jsonl` | both | `{mutant_id, stillborn: bool, stillborn_reason\|null, status: OK\|TRIVIALLY_EQUIVALENT\|DUPLICATE, duplicate_of\|null, changed_lines}` |
| `kills.jsonl` | both | `{mutant_id, results: {nodeid: killed\|survived\|error\|flaky}}` for non-stillborn, status OK mutants |
| `differential.jsonl` | both | `{mutant_id, status: DIFFERS\|NO_DIFF_FOUND\|SKIPPED, function\|null, args\|null, original_out\|null, mutant_out\|null, reason\|null}` |
| `validity.jsonl` | `run-core` (V-H1, V-H2), `run-secondary` (A-OPUS) | `{mutant_id, judge: V-H1\|V-H2\|A-OPUS, call_id, status: ok\|missing, payload\|null}` |
| `witness.jsonl` | same as validity | `{mutant_id, judge, criterion, confirmed: bool, reference_out, mutant_out, error\|null}` |
| `mutant_classes.json` | `run-core` | per mutant (T, P): `{mutant_id, arm, target, per_criterion: {ref: VALID_W\|NOT_VIOLATING\|UNDETERMINED}, valid_x: [ref], valid_confidence: {ref: number}, equivalent: bool, unattributed: bool, off_target: bool, collateral: bool, primary: {included: bool, criterion: ref\|null}, discarded: bool, survived_all: bool}` |
| `audit_sample.json` | `audit-sample` | `{seed, mutant_ids: [...], strata: {mutant_id: stratum}}` |
| `alignment.jsonl` | `run-core` (J-H masked), `run-secondary` (J-H shown, J-X masked) | per call `{judge: J-H\|J-X, arm: NAME-MASKED\|NAME-SHOWN, nodeid, rep, call_id, status, verifies_ref: ref\|NONE\|null, coverage\|null, confidence\|null, rationale\|null}` |
| `alignment_aggregate.json` | both | per `(judge, arm, nodeid)`: `{modal_ref, modal_share, modal_coverage, mean_confidence, verdict: ALIGNED\|MISALIGNED\|UNDETERMINED, verdict_s1, missing_reps}` |
| `token_maps/<judge>/<arm>/<nodeid-slug>/rep-<k>.json` | both | `{token: ref}` |
| `probe.jsonl` | `run-core` (J-H) | `{judge, nodeid, rep, call_id, status, guess\|null, confidence\|null}` |
| `relabel.jsonl` | `relabel` | `{rater: R-OAI\|R-AUTHOR, nodeid, label, checks}` |
| `proofs/alignment/<judge>/<arm>/<nodeid-slug>.intoto.json` | both | ADR 0001 alignment Statement |
| `proofs/discriminating-power/<ref>.intoto.json` | `run-core` | ADR 0001 discriminating-power Statement (arm T) |
| `timing.json` | both | `{stage: {started, finished, seconds}}` |

A node-id slug replaces every character outside `[A-Za-z0-9_.-]` with `_`.
`mutant_id` is the first 16 hex characters of the SHA-256 of the canonical JSON
of the normalized edit set.

Files of the amended protocol:

| File | Writer | Content |
|---|---|---|
| `imports.json` | `import-run-01` | `{source_run, sums_digest, entries: [{source_path, target_path, digest, role: contrast-generation\|terra-generation\|relabel\|canary-baseline, call_id\|null}], retired: [call_id], nonces: {call_id: nonce}}` |
| `imports/run-01/canary/*.json` | `import-run-01` | the baselines of the imported calls, byte for byte; never read as baselines of this run |
| `plan.json` | the first verb that needs it | `{arm_t, contrast_x, terra_x, writers, luna}`, each a list of `{call_id, criterion, model, cli, nonce, imported, writer\|null, kind\|null}` |
| `mutant_sources.json` | `run-blindspot`, `run-luna` | `{S\|O\|op\|terra\|luna: [mutant_id]}`: membership per source, also for a mutant whose document sits under another arm |
| `suites/<W>.json` | `run-blindspot` (S, O), `run-luna` (L) | `{criterion: {writer, criterion, call_id, status: ok\|missing, reference_checked, tests: [{name, source, status: kept\|rejected_static\|dropped_reference, reason\|null}]}}` |
| `writer_kills.jsonl` | `run-blindspot`, `run-luna` | `{writer, mutant_id, results: {nodeid: killed\|survived\|error\|flaky}}` for every evaluated mutant; node ids are `tests/test_park_<n>.py::<name>` |
| `exploratory_status.json` | `run-secondary`, `run-luna` | `{secondary\|luna: {call_id: complete\|blocked\|pending}}` |
| `call_budget.json` | every model-calling verb | `{<UTC date>: {claude\|codex: new calls}}` |
| `SHA256SUMS` | the seal of a run | `sha256sum` lines of every other file of the run directory |

A resumed call (its `raw/<call_id>/result.json` exists) counts neither against the call budget nor
against the run window.

## Pacing

Model calls are made only inside the run window: every day from 21:00 to 07:30 local time by default
(`--run-window HH:MM-HH:MM`, or the environment variable `PILOT_RUN_WINDOW`; `always` disables it). The
window may cross midnight. Outside the window a verb makes no new call, stops with exit code 3 and logs
`- quota-pause | endpoints: none | run window closed`; a rerun inside the window resumes it. The daily
call budget (`--call-budget`, default 250 new calls per UTC day and CLI) is a second cap with the entry
`call budget reached`. `script/run-pilot` starts a model-calling step only inside the window; it asks
`script/run-window`, which exits 0 inside and 3 outside.

## Sealing run-01 and importing it

The seal is a procedure of the operator, in this order:

1. `log-deviation --run-id run-01 protocol all "superseded by prereg-limits-1-3-a1; no analysis output exists"`.
2. Write `runs/run-01/SHA256SUMS`: `(cd runs/run-01 && find . -type f ! -name SHA256SUMS | sed 's|^\./||' | LC_ALL=C sort | xargs shasum -a 256 | sed 's/ \*/  /' > SHA256SUMS)`.
3. `script/import-run-01 --run-id run-02 --plan-only` prints the digests of the files an import would carry;
   they become the `import_digests` object of `prereg/amendment.json`.
4. Commit run-01 as it is, then the amendment, then create the tag `prereg-limits-1-3-a1`.
5. After the tag and the timestamp: `script/canary --run-id run-02`, then `script/import-run-01 --run-id run-02`.
