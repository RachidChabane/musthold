# ADR 0006: Pilot amendment to the host-delegated design

Status: accepted. Amends the pilot pre-registration `prereg-limits-1-3` by the
amendment `prereg-limits-1-3-a1`; restates the pilot switch rules of ADR 0003
(section "Pilot-dependent parameters and switch rules") for the amended endpoint
populations.

## Context

The pilot pre-registration `prereg-limits-1-3` tested a design whose second model
family was GPT: V-S and V-G validity votes, a Sonnet alignment judge and Opus and
`gpt-5.6-terra` generators. ADR 0005 removes the two-family guarantee: one host
model executes every product role in isolated, blind subagents, a mutant is valid
only by execution, and a judged signal never blocks alone. The pilot must test that
design, and the project owner required the amendment before any result exists. The
fixture, the oracle and the frozen bands stay; the amendment changes only what the
new design makes necessary.

State of the pilot run `run-01`, for which no outcome file, judge output or
analysis exists:

- 10 completed Opus generations, at PARK-1..3 slots 1 to 3 and PARK-4 slot 1;
- 5 completed `gpt-5.6-terra` generations, at PARK-1 and PARK-2 slots 4 and 5 and
  PARK-3 slot 4;
- one retired attempt, `generate__PARK-3~1__5`, which has an attempt but no
  `result.json`;
- both relabels completed (R-AUTHOR by Opus, R-OAI by `gpt-5.6-terra`);
- a completed canary.

Constraints the amendment must hold:

- The frozen machinery's integrity guard must keep its meaning. `ensure_manifest`
  diffs the previous `manifest.json` against the current one and logs `protocol |
  endpoints: all` on any change of `prereg_commit`, `harness_digest` or
  `fixture_digest`, then overwrites the manifest. Rebaselining the manifest would
  disable the guard by editing its only input.
- The provenance record must name the real tag. `GitProvenance.prereg_commit` runs
  `git log -1 -- prereg/PREREG.md` and recorded `3fad777`, while the tag
  `prereg-limits-1-3` peels to `e95f551`. `git_drift.preregistration_commit` peels
  correctly, but only `analyze` uses it.
- Isolation vetoes make one frozen band structurally near-unreachable. Computed
  from `label_matrix.csv` and `labels.yaml`, 7 of the 30 hand-written violations are
  killed by a test citing another criterion (PARK-3_a/b/c, PARK-5_b/c and
  PARK-8_a/c), and 16 of the 20 arm-P r-items survive. Surviving violations per
  criterion: PARK-3 has 0, PARK-5 and PARK-8 have 1 each, and the other seven have
  3 each. PARK-3_c falls to a GOOD_BROAD test alone, so legitimate coupling is
  penalised too.
- Fixture authorship is recorded only in the handoff notes: Opus authored the
  fixture, its tests and labels. Git does not record it (the fixture commit `a6dd0e0`
  is authored by the owner, with no model trailer), and the oracle's author model is
  not recorded.
- The machine is the owner's 24 GB work laptop, and the Claude quota is shared with
  daily work.
- The owner voided the earlier trunk counts and budgets (ADR 0007).

The choices (host model, run identity, bands, execution validation, the blind-spot
estimand, data disposition) were settled by structured debates between opposing
advocates and a blind judge; this record consolidates their verdict.

## Decisions

### Roles and models

One frozen host model, `claude-sonnet-5-5` (H), executes every product role:
generation, both validity votes and the primary alignment judge. Rationale:
plugin subagents run on the session model, so the product never computes a
two-model consensus, and a VALID defined as Sonnet and Opus with generation split
across both would re-create the cross-model check the owner removed. ADR 0007 pins
the Claude Code coder to the same model, so the pilot's product path is the
configuration the benchmark ships. Opus 5.5 (X) is a measurement contrast only.

| Role | Model | Call ids | Calls | Stage |
|---|---|---|---|---|
| G-H, arm-T generation: 5 per criterion, high | Sonnet | `generate__PARK-n~1__6..10` (fresh) | 50 | core |
| V-H1, V-H2: two blind validity votes, medium | Sonnet | `validity__<mutant>__H1/H2` | ≤ 360 | core |
| J-H: alignment, NAME-MASKED, k = 5 (frozen J-SONNET protocol) | Sonnet | frozen scheme | 190 | core |
| J-H leakage probe, 3 repetitions | Sonnet | frozen scheme | 114 | core |
| M_X, contrast generation: 3 per criterion, high | Opus | imported `__1..3` (10); new `__1..3` where absent (20) | 20 | blind spot |
| T_S, test writer: 1 per criterion, high | Sonnet | `write_tests__PARK-n~1__S` | 10 | blind spot |
| T_O, test writer: 1 per criterion, high | Opus | `write_tests__PARK-n~1__O` | 10 | blind spot |
| J-H NAME-SHOWN, k = 5 | Sonnet | frozen scheme | 190 | secondary |
| J-X: alignment, NAME-MASKED, k = 5 (judge = test-author model) | Opus | `alignment__…__JX` | 190 | secondary |
| A-OPUS audit (frozen) | Opus | frozen scheme | ≈ 70 | secondary |
| R-AUTHOR, R-OAI relabels | Opus, `gpt-5.6-terra` | imported unchanged | 0 | none |
| Terra mutants (exploratory source) | `gpt-5.6-terra` | imported `__4, __5` (5) | 0 | blind spot |
| T_L, M_L: 1 per criterion each | `gpt-6-luna` | `write_tests__…__L`, `xgen__PARK-n~1__L` | 20 + 2 canary | luna |

**Rules for the roles.**

- **Pins.** Every Claude call aborts unless `modelUsage` holds only its pin. Codex
  calls pass `-m gpt-6-luna` with `provenance: requested_only`.
- **Call ids.** No new call reuses an imported or retired id. The retired
  `generate__PARK-3~1__5` is never retried.
- **Inputs.** Prompts and schemas stay byte-identical. One prompt and one schema
  are added: `write_tests.md` and `write_tests.schema.json`.
- **Test-writer input.** The spec, the target `ID~REV`, the full `src/` tree and the
  public API.
- **Test-writer exclusions.** Writers never see fixture tests, the oracle, mutants
  or labels.
- **Test-writer output.** `{tests: [{name, source}]}`, 2 to 5 tests. The harness adds
  `@pytest.mark.verifies(c)`.
- **Static check.** A deterministic AST check allows imports only from the standard
  library, `pytest` and `parkfee`.
- **Execution check.** A test that fails or errors on the reference in any of 3 runs
  is dropped.
- **Validity votes.** V-H1 and V-H2 get identical inputs in separate isolated
  `claude -p` calls. They are the product's validity rule: two blind host votes,
  AND, τ = 0.70.
- **J-X.** It feeds no endpoint. It gives "G3 with J-X" and the error overlap
  between J-H and J-X against labels.

### Classes and gate

Classes per (m, c), τ = 0.70, with V-H1 and V-H2 in place of V-S and V-G:

| Class | Rule | May reach |
|---|---|---|
| VALID_w,c | both votes VIOLATED for c at ≥ τ, and ≥ 1 confirmed witness for c | matrix pool, D7, the D8 frozen reading |
| VALID_x,c | VALID_w,c; F0 passed; no reproduced kill and no error on any test whose Shallnot binding D(t) ≠ c | gate (BLOCK, pass credit), D6, D8 |
| EQUIVALENT, UNATTRIBUTED, NOT_VIOLATING, OFF_TARGET, COLLATERAL, UNDETERMINED | the frozen §6.5 rules with the two H votes, computed on VALID_w | as frozen |

D(t) comes from `bindings.json`, never from labels. Isolation costs no execution,
because F2 already runs the full suite. Rationale: a mutant that another
criterion's test kills is not evidence about c, and execution, not a judgment,
decides it. This is ADR 0005's "passes the tests of the other criteria", applied to
the pilot.

**Gate G3** (precedence BLOCK > ESCALATE > PASS):

1. Traceability not pass: BLOCK.
2. The reference fails a test in S: BLOCK.
3. A VALID_x,c (COLLATERAL included) survives every test in S: BLOCK.
4. No VALID_x,c in c's pool: ESCALATE `no_valid_mutant`.
5. A survivor that is UNDETERMINED for c, or VALID_w,c but not VALID_x,c: ESCALATE.
6. A test in S is MISALIGNED or UNDETERMINED (J-H NAME-MASKED): ESCALATE.
7. Otherwise: PASS. Pass credit is a VALID_x,c killed by S.

Every BLOCK carries executed evidence. Rule 4 is ESCALATE because absent evidence
escalates and negative evidence blocks (ADR 0003, A4); the frozen pilot rule 4
blocked, and G3-prereg keeps it as a reported sensitivity. A separate rule 4a ("VALID_x
exist and none is killed") is redundant: it implies a VALID_x survivor, so rule 3
fires.

**Policies.**

- **Primary and component policies:** G0, G1 (rules 1 to 5), G2 (rules 1, 2, 6) and
  G3 (primary).
- **Sensitivities:**
  - G3-prereg: the frozen rules, with rule 4 blocking and VALID_w in rules 3 and 4;
  - G3-noiso: VALID_w in place of VALID_x;
  - G3-strict, labelled "not an admissible product policy";
  - G3-lenient, G3-S1, G3 with J-X, CONSENSUS, and label-based truth.
- **§9.4 attribution** gains the link "isolation", whose truth is the oracle class.

### Endpoints and bands

Every band whose population is unchanged stays frozen. Bands keep their meaning
only where their population does: D2 to D5 and D9 to D11 keep their populations,
and D7's planted items are unchanged. D4 tightening and a D7 rebased to /16 would
move bands without a population reason; D7 measures the judges, and isolation is
not a judge property.

| ID | Population | Band | Switch fed |
|---|---|---|---|
| D1 to D5 | unchanged, under amended G3 | frozen | unchanged |
| D6 | precision of arm-T VALID_x primary-matrix mutants against oracle-valid-for-target; N7 = Sonnet arm-T base rate; VALID_w precision reported beside | frozen | SW1 (blocks use VALID_x, so its precision is the right input) |
| D7 | class level: r-items VALID_w for their criterion, n1-items VALID_w for any; the 16 isolation survivors reported beside | frozen: GO ≥ 16/20 and ≤ 1/10; NO-GO ≤ 10/20 or ≥ 3/10 | SW1 |
| D8 | criteria with ≥ 2 VALID_x arm-T mutants, over C_D8 | GO 9/9; NO-GO ≤ 6/9 | SW2a |
| D8, frozen reading | ≥ 2 VALID_w over 10 criteria | 10/10; ≤ 7/10 | reported, non-decisional |
| D9 to D11 | unchanged (J-H = J-SONNET protocol; matrix over VALID_w) | frozen | SW3, SW4 |

**Rule for C_D8.** D8 is the only re-based band. A frozen GO of 10/10 on the gate
pool is structurally near-unreachable under isolation, and a looser threshold
(at least 9 of 10) would let any one criterion fail, not only the exposed one, so
the rule fixes the denominator instead. C_D8 is the set of criteria with at least
one hand-written violation (`PARK-n_{a,b,c}`) that no test bound to another
criterion kills. The committed `isolation-table` verb computes it from
`label_matrix.csv` and the Shallnot bindings: C_D8 = the ten criteria minus
{PARK-3}.

- GO keeps "every criterion".
- NO-GO keeps "three or more criteria short".
- PARK-3's VALID_x count is reported by name.
- PARK-5 and PARK-8, with one surviving violation each, stay in the denominator
  and are named in advance as exposed.

**Remaining rules.**

- **SW2a.** Its remedy (more rounds, adjudicated operator survivors) still applies
  to isolation starvation.
- **SW5.** It reads "V-H1 alone meets D6's GO band" (see "Switch rules").
- **No new endpoint ids.** `ENDPOINT_IDS` stays D1 to D11, so the deviation grammar
  is unchanged.
- **Preconditions.**
  - P2 stays on author, R-OAI and R-AUTHOR (imported).
  - P3 stays on J-H.
  - P5 is computed per vote, V-H1 and V-H2 (capping D1 to D8, D10 and D11), and on
    J-H NAME-MASKED.
  - P4, P6 and P7 apply to run-02's records.
- **F7.** The production path is G-H + V-H1 + V-H2 + J-H NAME-MASKED.

**Limits paragraph**, in place of "model family and capability confounded":

> one model executes every product role; the contrast model shares its family; the
> fixture, its tests and labels were authored in a session of the contrast model;
> the oracle's author model is not recorded.

### Switch rules for the amended populations

ADR 0003's rules SW1 to SW6 are restated here, in full, for the amended endpoint
populations. They are fixed without access to any pilot output. If an endpoint is
INCONCLUSIVE, the default stays. The D- and F-labels name endpoints of the amended
pre-registration.

| Rule | Condition | Effect |
|---|---|---|
| SW1 | D6 NO-GO on VALID_x, or D7 NO-GO on VALID_w, as specified in "Endpoints and bands" | A judged VALID only escalates. BLOCK and pass credit then require owner attribution, and the report states that criteria without examples cannot PASS. |
| SW2a | D8 NO-GO, on the re-based D8 over C_D8 | `max_rounds` = 5. Every DIFFERS operator survivor in the footprint is adjudicated (screen bypassed), so the operator pool also becomes PASS-bearing. |
| SW2b | §11 arm-T oracle-valid rate per candidate ≤ arm-O rate | For criteria whose owner-attributed mutants already meet `min_valid` and `min_sites`, a round becomes one call (family chosen by seed). |
| SW3 | D10 or D11 NO-GO | Two-signal ALIGNED is disabled: ALIGNED needs the reasoning judge (J-H), and the matrix keeps only negative and contamination roles. If either is INCONCLUSIVE: `shadow_rate` = 0.25. |
| SW4 | D9 NO-GO | No judge settles ALIGNED alone; ALIGNED needs the judge and a SPECIFIC or OVERBROAD matrix, otherwise UNDETERMINED. `align_k` = 3 if, in the exploratory analysis, the k = 3 subsample verdict equals the k = 5 verdict on ≥ 95 % of tests. |
| SW5 (exploratory) | V-H1 alone meets D6's GO band on pilot data | Pass-credit judgments on killed mutants use V-H1 only. Survivors keep two votes. |
| SW6 | F7 amber or red | `max_rounds` = 2, `shadow_rate` = 0.05, `max_reasoning_calls` = 24. |

SW1 reads D6 on VALID_x because blocks use VALID_x, and D7 on VALID_w because D7
measures the judges. SW2a reads the re-based D8 because the frozen reading is
non-decisional. SW5 reads V-H1 because the two H votes are the product's validity
rule.

### Blind-spot estimand (§11, reported first)

The pilot's central descriptive estimand answers the owner's question: when the same
model writes the tests and the mutants, do they share the same blind spots? A
comparison of same-model mutants against operator mutants alone confounds mutant
subtlety with shared blind spots. A crossover over two test writers and two model
mutant sources, anchored on operator mutants (which use no model), isolates the
shared part. Truth is model-free: oracle-valid-for-c. The fixture tests cannot
carry the estimand, because they were constructed deliberately to their labels.

**Sources and writers.**

- **Writers** W ∈ {S, O}: suites T_W,c from "Roles and models".
- **Mutant sources** M ∈ {S, O, op}:
  - S = arm T (Sonnet);
  - O = M_X (Opus);
  - op = arm-O operator mutants (§6.3).
- **Exploratory cells:**
  - writer L (luna);
  - writer F, the gold-good fixture tests;
  - mutant sources terra (PARK-1 to PARK-3) and luna.

**Population.**

- U(M, c) is the set of mutants of source M that are non-stillborn, deduplicated
  within the source (F1), and oracle-valid-for-c (§6.6), whatever their declared
  target. Membership is per source even when cross-arm dedup (§6.2) shares
  processing.
- A DUPLICATE takes the class of its representative.

**Escape.** For m ∈ U(M, c), the F2 rules run on T_W,c (kills reproduced once;
flaky counts as survived).

- m escapes T_W,c iff every test in T_W,c survived or was flaky.
- m is caught iff some test has a reproduced kill.
- A pair (m, W) with an error on any test, and no reproduced kill, is excluded and
  counted.

**Common set.** C* is the set of criteria c for which T_S,c and T_O,c are non-empty
after the reference filter, and U(S, c), U(O, c) and U(op, c) are all non-empty.
Exclusions are reported per criterion.

**Rates.**

- e_c(W, M) is the share of evaluable U(M, c) that escapes T_W,c.
- e(W, M) is the mean over c ∈ C* of e_c(W, M): each criterion weighs 1, so
  operator volume cannot dominate.

**Estimands.**

- **Owner's contrast:** Δ_W = e(W, W) − e(W, op).
- **Per-writer shared blind spot:** SB_S = Δ_S − [e(O, S) − e(O, op)], and
  symmetrically SB_O.
- **Primary:** SB = ½(SB_S + SB_O) = ½[e(S,S) + e(O,O) − e(S,O) − e(O,S)].

**Interval and reading.**

- Interval: a percentile cluster bootstrap that resamples criteria of C* with every
  cell attached, B = 10,000, seed 20261006.
- Reading, fixed in advance:
  - lower bound > 0: shared blind spot indicated;
  - upper bound < 0.10: no material shared blind spot;
  - otherwise undetermined, which is the expected reading at |C*| ≤ 10.

**Also reported.**

- The 2 × 3 table e(W, M), with counts.
- The location split: the share of escaping model mutants whose edited qualname also
  hosts an escaping operator mutant (a coverage hole) versus one that does not (a
  model-specific blind spot).
- The exploratory cells, labelled cross-family where luna or terra is involved.

**Caveats, stated in advance.**

- S and O share a family.
- One writer call per criterion.
- The truth oracle's author model is not recorded.

### Other §11 additions

- **Isolation table:** VALID_w versus VALID_x per criterion, with the predicted
  exposure {3, 5, 8}.
- **Same-model correlation:** P(V-H2 wrong | V-H1 wrong) against the oracle, and
  J-H and J-X error overlap against labels.
- **Source-(2) calibration:** reliability, ECE and risk–coverage of J-H
  `modal_share` against labels, plus the UNDETERMINED rate (source 3).
- **Per-source oracle-valid yield:** Sonnet, Opus, terra, luna and operators.

### Data disposition and run identity

The decisional run is a fresh `run-02` under the new tag `prereg-limits-1-3-a1`. It
imports run-01's completed calls unopened, by SHA-256, under their original call
ids. Rationale: a fresh run that imports by digest costs the same number of calls
as adopting run-01 in place (zero re-spend) and leaves every manifest in run-02
under a1 from its first verb, so the drift guard keeps its meaning.

| Artifact (run-01) | Disposition |
|---|---|
| 12 canary baselines | Imported under `imports/run-01/canary/` only as the isolation baselines of imported calls. run-02 runs its own canary for every (model, call type) it calls. |
| R-AUTHOR, R-OAI raw records | Imported; `relabel.jsonl` re-derived by a zero-call `relabel` resume |
| 10 Opus generations | Imported as M_X under their original ids; oracle-only, never judged, never in arm T |
| 5 terra generations | Imported as the exploratory terra source; oracle-only |
| `generate__PARK-3~1__5` (attempt only) | Not imported; listed as retired |
| traceability, bindings, reference | Recomputed in run-02; equality with run-01's digests is checked |
| run-01 `DEVIATIONS.md`, `plan.json`, `manifest.json` | Stay in run-01 |

**run-01 is sealed.**

- `log-deviation --run-id run-01 protocol all "superseded by prereg-limits-1-3-a1;
  no analysis output exists"`.
- `runs/run-01/SHA256SUMS` is written.
- run-01 is committed as-is before the a1 tag. It is never analysed, and the report
  gives its counts per kind.

**`import-run-01`** is a deterministic verb that runs once, before any run-02 model
call.

- It refuses unless every source file matches `SHA256SUMS`.
- It refuses if any target path exists.
- It copies by digest and writes `imports.json`, listing source path, digest and
  role.

**run-02's `plan.json`** has these sections: `arm_t` (fresh uuid4 nonces),
`contrast_x` (imported entries with their original nonces; new entries with fresh
nonces), `terra_x`, `writers` and `luna`.

**The decisional run** is the first complete run after the a1 tag. The rewritten
PREREG states, timelessly:

> The relabels R-AUTHOR and R-OAI and 15 generation calls were executed on
> 2026-10-06 under the commit tagged `prereg-limits-1-3`, before any judge call and
> before any outcome file existed; they enter the decisional run by the digests in
> `prereg/amendment.json`.

This sentence replaces both the preamble's "no model call … before the tag" and
§3.4's "after this commit".

### Freeze, tag, timestamp, drift

1. **Harness changes, test-first**, with the suite green ("Harness changes").
2. **Documents.**
   - PREREG.md is rewritten as law, with no amendment narration.
   - `prereg/amendment.json` is machine-readable: the superseded tag and its peeled
     commit (`e95f551`), the declared changed paths, and the import digests.
   - `prereg/isolation_table.json` (ground-truth side, read only by `analysis`) holds
     C_D8.
   - The rationale is this record.
   - `models.lock.json` is replaced: terra is marked `completed`, luna is pinned
     `requested_only`, and the CLI versions are recorded.
3. **Commit, tag and timestamp.** Commit, then create the annotated tag
   `prereg-limits-1-3-a1`. Write `freeze/a1-commit.txt` with the peeled commit.
   Obtain `a1-commit.tsq` and `.tsr` from `http://timestamp.digicert.com` by the
   existing procedure. Commit the freeze files and push the tag and the commit.
4. **Provenance.**
   - `GitProvenance.prereg_commit` peels `PREREG_TAG` through
     `git_drift.preregistration_commit`.
   - The manifest records `prereg_tag`, `prereg_commit`, `superseded_prereg_tag`,
     `superseded_prereg_commit` and the `imports.json` digest.
5. **Drift guard.** `DRIFT_GUARDED_PATHS` adds:
   - `runs/run-01`, `freeze/prereg-commit.*` and `freeze/README.md` (never the
     post-tag `a1-commit.*`);
   - `harness/src/pilot/domain/mutant_classes.py`, `model_slate.py`,
     `execution_validation.py` and `endpoint_ids.py`;
   - `harness/src/pilot/application/blind_spot_stage.py`.
6. **`amendment-check`**, run by `analyze` before anything else. It refuses unless
   all of the following hold:
   - both tags exist;
   - `git diff --name-only --relative prereg-limits-1-3 prereg-limits-1-3-a1 -- .`,
     run from the experiment root, is a subset of the declared paths (which include
     `freeze/README.md`, `freeze/prereg-commit.*`, `script/run-pilot` and
     `Makefile`);
   - each `*-commit.txt` equals its peeled tag;
   - each token's message imprint (`openssl ts -reply -text`) equals the SHA-256 of
     its `*-commit.txt`;
   - the a1 token's genTime precedes the earliest `started` in run-02's
     `timing.json`;
   - run-01 matches `SHA256SUMS`;
   - `imports.json` matches run-02.
7. **Report.** The provenance section lists both tags, both genTimes, the
   `amendment.json` digest, the imports table and run-01's counts per kind.

### Run order

Every heavy verb runs through `script/mem-guard`, under `nice`.

1. `canary`: run-02's Claude pairs only, about 9 calls. Terra is never canaried;
   luna's canary runs inside `run-luna` and records to `exploratory_status.json`. P4
   reads this step only.
2. `import-run-01`, then `relabel` as a zero-call resume. `canary` and
   `import-run-01` write `timing.json` entries.
3. `run-core`: traceability, then arm-T generation, arm P, F0 to F3, V-H1 and V-H2
   with witnesses, isolation, J-H NAME-MASKED, and the probe.
4. `run-blindspot`:
   - new M_X calls, T_S and T_O, then the reference filter;
   - arm-O enumeration;
   - F0 to F2 for M_X, terra and arm O;
   - writer-suite kill vectors for every source.
5. `oracle-classify`, then `audit-sample`.
6. `run-secondary`: J-H NAME-SHOWN, J-X, A-OPUS. It closes on 2026-10-24 23:59 UTC;
   per-call status goes to `exploratory_status.json`.
7. `run-luna`:
   - **Window:** opens 2026-10-10 11:01 local time (the Codex free quota resets for
     every model at that instant); closes 2026-10-17 23:59 UTC.
   - **Content:** luna canary, T_L, M_L, F0 to F2, writer suites, and oracle
     classification.
   - **Status record:** each luna call's status goes to
     `runs/run-02/exploratory_status.json` (`complete`, `blocked` or `pending` per
     call id), never to `DEVIATIONS.md`.
   - **At close:** anything incomplete is reported BLOCKED in the exploratory tables
     only.
8. Once both are complete or closed: `amendment-check`, `analyze`, `report-lint`.

### Budget and pacing

| Stage | Sonnet | Opus | luna |
|---|---|---|---|
| Canary | ≈ 5 | ≈ 4 | 2 |
| Core | ≤ 714 | 0 | 0 |
| Blind spot | 10 | 30 | 0 |
| Secondary | 190 | ≈ 260 | 0 |
| Luna | 0 | 0 | 20 |
| **Total** | **≈ 920** | **≈ 295** | **≈ 22** |

That is about 1,215 Claude calls, about 300 of them Opus, and about 22 luna calls.

**Pacing and machine safety.**

- A `--call-budget N` option (default 250 per day per CLI) stops a verb cleanly with
  exit code 3 and logs `quota-pause | none | call budget reached`; a rerun resumes
  it.
- Model calls in flight: `MAX_CONCURRENT_MODEL_CALLS` = 2.
- `MAX_CONCURRENT_PYTEST`, `MAX_WITNESS_WORKERS` and `ORACLE_PARALLELISM` share one
  process-wide semaphore of 2. At most 2 local test, witness or oracle subprocesses
  run at once.
- No local model runs in the pilot.
- No call is paid: the Codex arm fits the free quota.

### Harness changes

| Area | Change |
|---|---|
| `domain/model_slate.py` | pins, judges, writers; canary scope = run-02's Claude pairs |
| `domain/execution_validation.py` (new; `domain/isolation.py` is taken) | VALID_x |
| `domain/mutant_classes.py`, `domain/gate.py`, `domain/variants.py` | classes, rules 3 to 5, policies |
| `adapters/provenance.py` | tag peeling |
| `adapters/model_cli.py` | Codex `-m` pin |
| `domain/constants.py`, `pipeline_constants.py`, `analysis_constants.py` | concurrency set to 2 with one shared semaphore; `PREREG_TAG`; guarded paths |
| `application/` | import, test-writing, contrast, writer-kill, blind-spot and luna stages |
| `analysis/` | endpoint populations, `blind_spot.py`, tables, sensitivities, provenance |
| `cli/` | verbs `import-run-01`, `run-blindspot`, `run-luna`, `isolation-table`, `amendment-check`; `--call-budget` |
| `script/run-pilot` | new steps, each under `script/mem-guard` |

### Tests

| Layer | Tests |
|---|---|
| Unit | classes; gate rules and policies; C_D8 = ten criteria minus {PARK-3}; D8 bands; SB, escape and reading rules; import refusals |
| Property | no BLOCK without an executed input; G3 BLOCKs ⊆ G3-prereg BLOCKs; adding a test never adds a VALID_x; no planned call id equals an imported or retired raw directory; no nonce reused; SB = ½(SB_S + SB_O) |
| Integration | import on a synthetic copy; `amendment-check` on a temporary two-tag repository; fake-CLI canary; semaphore ≤ 2 |
| Contract | lock, amendment, imports and status files; `write_tests` schema; deviation grammar |
| End to end | a recorded synthetic run through analyze gives byte-identical `decisions.json` and blind-spot table |

The existing suite, `lint-imports` and `pyright` stay green.

## Consequences

- **PARK-5 and PARK-8** each keep one isolation-surviving violation, so D8 at 9/9
  can still fail on them through fixture coupling. Their VALID_x counts are named in
  advance and the frozen reading is reported beside.
- **Two Sonnet votes are correlated.** VALID_w may be less conservative than the
  cross-family pair the D6 and D7 bands were set for. The bands are absolute, and
  the correlation is reported (P(V-H2 wrong | V-H1 wrong)).
- **Blind-spot power.** At ten criteria or fewer, with one writer call per criterion
  and same-family contrasts, SB will likely read "undetermined". The luna cells are
  exploratory and may be BLOCKED.
- **Truth shares a model.** The oracle's author model is not recorded; if it shares
  a model with a mutant source, oracle-valid truth shares that source's blind spots.
  The limits paragraph states this.
- **J-X is confounded.** Judge = test-author is measured on deliberately
  constructed tests, which an honest coder never writes.
- **Opus quota** per window is unverified. A secondary stage that exhausts it is
  paced across days, and its close date bounds the wait.
- **Timestamp verification.** `amendment-check` verifies the imprint and the genTime
  ordering, not the DigiCert certificate chain.

Unverified at decision time, to settle at canary:

- the availability and flags of `claude-haiku-4-5` (used by the benchmark's
  construction roles, ADR 0007);
- Opus quota per window.

Blocked, external:

- The luna stratum, until the Codex free quota resets on 2026-10-10 11:01; it is
  sized to the free quota and reported BLOCKED where incomplete at close.
- Paid Codex and any other spending: out of the question.
