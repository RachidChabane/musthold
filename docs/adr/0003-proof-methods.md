# ADR 0003: Proof methods

Status: accepted; amended by ADR 0005 (host-executed judges and generation, claim power)
and, for the limits 1-3 pilot, by ADR 0006. Amends ADR 0001 where it is silent
(section "ADR 0001 amendments").

## Context

ADR 0001 fixes what a proof says and how it is signed; ADR 0002 fixes how the
product is built. Neither fixes how each proof is produced: which mutants carry a
pass, who decides that a surviving mutant is equivalent, how alignment is judged,
how a System One model (a small local model answering closed questions) may be
used before it is calibrated, how criteria carry executable examples, and which
parameters the pilot's outcomes may change.

Constraints the methods must hold:

- A surviving mutant that no stage can classify stays `UNDETERMINED`, which
  ADR 0001 escalates, so the criterion could never pass. Every no-difference
  survivor needs a decider.
- ADR 0001 requires mutants "generated fresh at check time, seeded per run" in CI,
  where the container holds no model key, while its cost rule re-judges only
  criteria whose inputs changed.
- Operator mutation alone does not reach boundary constants, module-level
  assignments or rate tables; many arithmetic swaps fail with a type error at call
  time and are collateral rather than criterion-specific.
- System One answers are uncalibrated: the serving APIs document `confidence` as
  concentration, not correctness, one family returns no confidence for `noul`
  questions, and a small model ships with a 2048-token context.
- A test matrix is sparse at fixture scale (a Wilson lower bound of 0.70 needs
  about 9 of 9 kills), and several relational variants at one site are not
  independent evidence.
- Criteria are Markdown or YAML text. A folded YAML scalar (`>`) joins lines, a
  literal scalar (`|`) keeps them, and Shallnot joins every statement with
  `strings.Join(strings.Fields(text), " ")`; the Markdown parser joins trimmed
  continuation lines with spaces and keeps the `-` marker.
- Shallnot's core stays deterministic, with no network and no model.

The open choices (mutant source, validity decision, alignment protocol, System One
role, examples, switch rules) were settled by structured debates between opposing
advocates and a blind judge; this record consolidates their verdict.

## Decisions

### Method: targeted generation carries the pass, operators run beside it, execution and cross-family judges decide

Discriminating power keeps criterion-targeted model generation as the pool that
carries every PASS claim. Deterministic operator mutation runs beside it at zero
token cost and contributes fresh seeded mutants in CI, executed attribution through
spec examples, extra BLOCK discovery through a System One screen, and sentinels
that catch mutation-sensitive tests by execution. Gate-relevant validity is decided
by execution or by two cross-family reasoning judges with a confirmed witness. A
no-difference survivor is settled by the `equivalent` question put to those two
judges (pilot pre-registration §6.5). Alignment follows the pilot's masked judge
protocol. Completeness is advisory: System One decisions plus deterministic
question templates.

Rationale:

- A no-difference survivor needs a decider, and an equivalence threshold with no
  stage that computes the number leaves the survivor `UNDETERMINED` forever. The
  two-judge `equivalent` question supplies it.
- Operators reach function bodies only. They cannot reach module-level constants,
  numeric constructor arguments, `Decimal("…")` rates, `min(…, CAP)` or
  `quantize(…)`. On the pilot fixture `parkfee`, the engine produced 99 mutants
  (`breakdown` 37, `_in_night_window` 28, `_hourly_charge` 8, `_long_stay_charge`
  8, `format_receipt` 7, `_validated_free` 7, `_started` 4), every one inside a
  function body, and some criteria are violated only by a boundary constant
  (`amount >= FREE` violates only PARK-9; `amount < FREE` only PARK-10). Operators
  therefore cannot carry boundary-constant criteria alone, and a share of the pool
  must come from targeted generation.
- Reasoning work is bounded by construction: PASS-bearing pools hold only targeted
  and example-attributed mutants; operator survivors reach judges only through a
  recall-oriented screen; behaviourally identical survivors are merged; decisions
  are memoized under full keys.

### Evidence grades and the System One rule

| Grade | Produced by | May block / credit / exclude / ALIGN |
|---|---|---|
| executed | kills, witness confirmation, example probes, sentinel kills | yes |
| owner-attributed | a mutant fails an example of c (the spec owner stated that input and output) | yes |
| reasoning-judged | two cross-family judges (V-S `claude-sonnet-5-5`, V-G `gpt-5.6-terra`), the pilot validity protocol, both at or above `tau_judge`, and a confirmed witness for VALID | yes |
| s1 | a System One answer | see the rule below |

**System One rule.** Until a decision kind passes the admission test (see
"Routing, thresholds, calibration"), a System One answer may:

- route work toward more scrutiny;
- select which operator mutants enter a pool, as a generation-side filter whose
  miss rate is measured;
- produce advisory output;
- confirm ALIGNED, but only jointly with a positive executed matrix signal.

It never blocks, never grants pass credit, and never excludes a mutant already in a
pool. The rule is applied uniformly, including to pool membership and to kill
profiles.

### Discriminating power pipeline, per dirty criterion c

**Dirty set.** A criterion c is dirty when any of these changed: its statement
digest, the example binding, a test file citing c, a footprint file, or the
decision policy. Per-test reference coverage comes from one suite run with coverage
contexts and costs no tokens.

**Footprint.** The union of the functions executed by c's bound tests, the
functions executed by c's example probes, and the functions the diff touches,
closed over the static call graph; referenced module-level assignments become
mutation sites. The `code_tree` inputs are the footprint files plus their static
import closure, one digest per file.

| Stage | Rule |
|---|---|
| S0 targeted | One round is two calls, one per family (Opus 5.5 via `claude`, `gpt-5.6-terra` via `codex`), each returning 3 slots (S1 direct, S2 boundary, S3 partial). Schema: the pilot's `generate.schema.json`. Input: spec, target `ID~REV`, footprint slice; never tests. Later rounds receive earlier mechanism summaries to avoid. |
| S0 operator | Per-ecosystem `MutationEngine` strategy. Families are the pilot's, plus: module-level numeric constants and numeric constructor arguments (`timedelta(minutes=15)`, `time(22, 0)`); numeric `Decimal("…")` strings (last digit ±1, ×10); `min`↔`max`; `and`↔`or`; boolean literal flip; exception class swap. The cap is `K_op` per criterion, sampled stratified by (site, family). The seed is fixed locally and drawn per run in CI. |
| S0 sentinels | Two per footprint file, drawn by seed from a catalogue of AST no-ops: a dead assignment to a fresh local, alpha-renaming of a local, and an unused private module constant. Functions that use `locals`, `eval`, `exec` or `inspect` are skipped. |
| F0 / F1 | Pilot stillborn checks and normalize/deduplicate. Sentinels bypass F1. |
| F2 | Kill vectors. Function-body sites run the tests whose reference coverage includes them; module-level sites and sentinels run the full suite. There is no bail, and each kill is rerun once (a flaky kill counts as survived). |
| F3 | Differential on the public functions reaching the site (Hypothesis or the ecosystem's equivalent, derandomized with the run seed), biased to both sides of every constant and comparison at the site and seeded with example inputs. For rich types, a reasoning-written strategy is cached per public-API digest, kept only if at least 95 % of draws run on the reference, and executed in the mutant workspace. Output: up to 3 shrunk witnesses, NO_DIFF_FOUND, or SKIPPED. |
| F4 | A witness is confirmed when the reference output equals the expected value and the mutant's output differs or raises. |
| E1 | Run every example of every criterion against each mutant. A failed example of c alone makes the mutant VALID_c (owner-attributed, confidence 1.0, basis `spec_example`). Failed examples of two or more criteria make it COLLATERAL. |
| Screen | For each non-E1 operator survivor with DIFFERS (after behavioural dedup on identical witness tuples), send one System One request. The state holds the in-scope statements, the normalized diff and the confirmed witnesses. There is one `choice` question per in-scope criterion, {VIOLATES, CONSISTENT}, at most 64 per request. P(VIOLATES_c) at or above `tau_screen`, or STATE_TOO_LARGE, flags the mutant into c's pool. Killed non-E1 operator mutants are screened too, but only to feed `none(t)` (see "Alignment"). |
| Judges | Reasoning judges rule on: every targeted survivor; every flagged operator survivor; killed targeted mutants in seeded order until `min_valid` VALID mutants cover `min_sites` sites; and, for a targeted mutant with no kill and NO_DIFF_FOUND or SKIPPED, the `equivalent` question. |
| Stop | Stop when a BLOCK is established, or when the pool holds at least `min_valid` VALID mutants (owner-attributed or reasoning-judged) on at least `min_sites` distinct sites, at least one of them killed. Otherwise run another targeted round, up to `max_rounds`. |

**Pool content.** The proof's `mutants[]` holds the targeted mutants, the E1
mutants, the flagged operator mutants and the sentinels. Operator mutants that were
screened out are counted in `extensions.operatorScreen`, never listed.

**Sentinels.** Sentinels must be AST-level semantic no-ops. They are exempt from
F1 and run against the full suite. A comment-only change would be discarded by F1
as trivially equivalent, and a coverage-selected F2 would never select a test that
reads source, because such a test executes no line of the mutated function; a
full-suite run is what catches a test that asserts on source hashes.

**Validity mapping.** The classes are the pilot's (pre-registration §6.5), with
τ = `tau_judge`:

| Class | ADR validity | Notes |
|---|---|---|
| VALID_c | `VALID` | Basis is `spec_example` or `judges`. Confidence is 1.0, or the minimum of the two judges. |
| COLLATERAL | `VALID` + `extensions.collateral` | Counted by the gate, excluded from the matrix. |
| OFF_TARGET | `NOT_VIOLATING` + `offTarget` | Re-homed to the criterion it violates. |
| UNATTRIBUTED | `NOT_VIOLATING` + `unattributed` | Feeds `none(t)` and completeness. |
| EQUIVALENT | `EQUIVALENT` | Requires the two judges, or construction (sentinels). |
| anything else, or an exhausted budget | `UNDETERMINED` | An answer that contradicts execution (EQUIVALENT for a killed or differing mutant) is never accepted. |

**Terminal outcomes per criterion (gate).**

| Outcome | Required evidence | Decider |
|---|---|---|
| BLOCK | Traceability fails; `correctCodePasses=false`; an example fails on the reference (`example_failed`); or a pool mutant survives every test and is VALID at or above τ. | Shallnot, executed; an owner-attributed or reasoning-judged VALID |
| ESCALATE | A pool survivor is UNDETERMINED or VALID below τ; no VALID was produced (`no_valid_mutant`); a citing test is MISALIGNED or UNDETERMINED; `mutation_sensitive_test`; `budget_exhausted`. | Shallnot |
| PASS | At least one killed VALID at or above τ, killed by a test that killed no sentinel; every pool survivor is EQUIVALENT or NOT_VIOLATING by owner-attributed or reasoning-judged grade; every citing test is ALIGNED. | Shallnot over the deciders above |

**Worked example on the pilot fixture `parkfee`** (spec without examples, full test
sets; these cases are regression fixtures):

- PARK-6: the WEAK test leaves night-boundary survivors (operator and S2) with
  in-window witnesses; judged VALID_6, they BLOCK, and a judge split ESCALATEs. The
  tautology test reads NONE to the judge: MISALIGNED.
- PARK-7: `_validated_free` enters the footprint through `breakdown`'s static
  closure even though the mock bypasses it. Its 7 operator mutants (flagged by the
  screen, then judged) and the targeted 3-hour mutants survive. A witness such as
  "validated, 2 h 59" (reference 0.00, mutant 7.50) is judged VALID_7, so the
  criterion BLOCKs.
- PARK-10: `amount < FREE` gives "EUR 0.00" at 0.00. If the WEAK test misses it,
  the screen flags the survivor, the judges find it VALID_10, and the criterion
  BLOCKs. `amount <= FREE` is NO_DIFF_FOUND and never enters the pool; a targeted
  NO_DIFF survivor gets the `equivalent` question. PASS also needs ALIGNED, and the
  WEAK and tautology tests fail that, so the best case is ESCALATE.
- PARK-2, subset of the two GOOD tests: killed grace-boundary mutants judged VALID
  give pass credit and the judges clear the remaining survivors: PASS. The full set
  ESCALATEs through its WEAK and MIS_OTHER tests.

**Hiding.**

- Mutant bodies live in the user cache
  (`$XDG_CACHE_HOME/spec-proof/<repo-digest>/`) and are applied only in a workspace
  outside the work tree. Proofs carry digests only, and in CI the mutant set is
  age-encrypted in the ledger.
- A local finding shows the criterion, the slot or operator family, and the inputs
  of one confirmed witness. It never shows a body, diff, mechanism or mutant
  output. A CI finding shows only the criterion and counts.
- The operator and sentinel catalogues are public. Sentinel kinds and positions are
  drawn per run, so a test cannot target a fixed sentinel. Killing every VALID
  first-order mutant at every footprint site is real testing work.

**Order under budget.** Targeted survivors are adjudicated first, then flagged
operator survivors in descending P(VIOLATES), then killed candidates. Exhaustion
skips the least likely violators.

### Alignment

**Input.** The pilot's NAME-MASKED construction and leakage property test
(pre-registration §7.1); criterion tokens include the examples.

**Matrix signature.** The pilot's §8 signature with these changes:

- VALID pools hold only owner-attributed or reasoning-judged, non-collateral
  mutants;
- `none(t)` uses UNATTRIBUTED mutants of any grade, an escalation-direction use;
- `sentinel(t)` is added.

Derived outcomes: CONTAMINATED = `sentinel(t)`; PINS_OFFSPEC = `none(t)` >
`theta_none`.

**Deciders and verdicts** (`D(t)` is the declared criterion; it enters only the
final comparison):

| Case | Verdict |
|---|---|
| CONTAMINATED | `UNDETERMINED`, and the `mutation_sensitive_test` finding |
| System One `verifies` = D(t), `coverage` in {FULL, PARTIAL}, raw confidence at or above `tau_route`, and the matrix is SPECIFIC or OVERBROAD, and not PINS_OFFSPEC | `ALIGNED`; judge = System One model; `extensions.matrix` |
| Any other case | Escalate to J-SONNET NAME-MASKED, `align_k` runs, modal aggregation |
| Judge says D(t) with FULL/PARTIAL, `modal_share` at or above `modal_min`, and the matrix is not OFF_TARGET, INERT, PARTIAL or PINS_OFFSPEC | `ALIGNED` |
| Judge says D(t) but the matrix is negative | Run J-GPT once. If the disagreement with the matrix persists: `UNDETERMINED` |
| Judge says another criterion, NONE, or WEAK | `MISALIGNED` |
| `modal_share` below `modal_min` | `UNDETERMINED` |

**System One questions:**

- `verifies`: a `choice` over the criterion tokens plus NONE. Above 23 criteria, a
  deterministic tournament runs: groups of at most 23 plus NONE, then a final
  round. Its confidence is the minimum over rounds. The tournament runs on every
  adapter.
- `coverage`: a `score` over [NONE, WEAK, PARTIAL, FULL].

**Audit.** A hash-selected `shadow_rate` of two-signal ALIGNED verdicts also goes to
J-SONNET. The reasoning outcome replaces the audited verdict.

**Statement.** One Statement per (test, declared criterion), containing
`judge {model, promptDigest, blind: true}`, `extensions.decision` and
`extensions.matrix`.

Rationale: the matrix cross-checks and detects contamination but never issues
ALIGNED alone, because at fixture scale it is sparse (PARK-7 and PARK-10 cannot
reach a Wilson lower bound of 0.70 even if every mutant were VALID) and relational
variants at one site are not independent evidence. The two-signal ALIGNED assumes
System One errors and matrix errors are uncorrelated, which the benchmark measures.

### Completeness (advisory, no reasoning model)

- **Checklist `c1`:** `error_handling`, `boundaries`, `permissions`, `concurrency`,
  `compatibility`, `input_validation`, `time_and_locale`,
  `state_and_persistence`.
  - Dimensions declared out of scope are read from the report.
  - Every other dimension gets one System One request per spec digest, with one
    `choice` per dimension: {COVERED, NOT_APPLICABLE, MISSING}.
  - MISSING at or above `tau_adv` produces a question. MISSING as the argmax below
    `tau_adv` produces a question flagged `low_confidence`.
- **Diff against spec.** One hunk per changed function. Test files, spec files and
  hunks whose AST is unchanged are excluded.
  - A hunk is justified when an example probe of some criterion executes it, or an
    ALIGNED test of some criterion executes it.
  - Otherwise System One answers a `choice` over the tokens plus NONE (tournament
    above 23). NONE at or above `tau_adv` yields an `unjustified_change` question.
  - UNATTRIBUTED mutants in changed hunks yield `unconstrained_behaviour`
    questions carrying their witness.
- **Testability.** System One `choice` per active criterion: {TESTABLE, AMBIGUOUS,
  UNTESTABLE}. AMBIGUOUS or UNTESTABLE yields a `clarity` question.
- **Questions** come from deterministic templates per signal and fill
  `questions[] {dimension, question, requirementIds[]}`. The predicate keeps
  `advisory: true`.

### Criteria format and Shallnot changes (private branch)

**Inline examples, closed grammar.** Examples sit inside the statement, after the
prose:

```
- PARK-2~1: WHEN a stay lasts 15 minutes or less THE SYSTEM SHALL charge 0.00.
  - Example: quote(entry=2026-03-02T10:00, exit=2026-03-02T10:15) => 0.00
  - Example: quote(entry=2026-03-02T10:00, exit=2026-03-02T10:00) => 0.00
```

```
example  := marker call "=>" outcome
marker   := [("-"|"*"|"+") " "] "Example:"
call     := operation "(" [name "=" literal {"," name "=" literal}] ")"
outcome  := literal | "raises" Name
literal  := ISO-8601 datetime | decimal | integer | true | false | null | "quoted string"
```

**Extraction rules.**

- Shallnot extracts examples from the joined statement, splitting at each marker.
  The text before the first marker is the prose, and examples must come last.
- `c1` is unchanged, because examples are already statement text: editing one makes
  the proofs stale and raises `statement_changed` under `--baseline`.
- Examples are optional, and no model translates them.

Extracting from the joined statement makes Markdown and both YAML scalar styles
behave identically: under a folded scalar the example lines fold into one line,
under a literal scalar they stay separate, and the join erases the difference.
Extracting from raw lines before the join would fail under `>`, and a YAML sidecar
is unnecessary.

**Report 1.2 (additive).**

- Per requirement: `statement_digest` (ADR 0001) and
  `examples[] {index, operation, args{name: {type, value}}, expect {kind: returns|raises, type, value}}`.
- At the top level: `out_of_scope[] {spec_file, line, dimension, reason}`.

**New findings:**

- `malformed_example` (warning): a marker segment outside the grammar.
- `detached_example` (warning): a Markdown line matching the marker that no
  statement absorbs. This covers lines after a blank line, lines after a
  `Non-testable:` sub-bullet, and lines outside any block.
- `unknown_dimension` (warning).

**Out-of-scope declarations.**

- Markdown: `Out of scope (<dimension>): <reason>` lines, with an optional list
  marker or emphasis. Outside fences, such a line terminates an open statement, as
  `Non-testable:` does, and is never absorbed.
- YAML: a new top-level key `out_of_scope: [{dimension, reason}]`, added to
  `schemas/spec.schema.json` and to `YAMLParser`, which otherwise rejects any key
  except `requirements`.
- The completeness subject hash covers these declarations.

**Operation binding.** `.spec-proof/config.yaml` holds
`operations: {quote: {python: "parkfee:quote"}, …}`, with one `ExampleExecutor` per
ecosystem that reuses the pilot's witness value objects. An unbound operation gets
the module finding `unbound_example_operation`. There is no sidecar.

### Shallnot gate additions (consume proofs; no network, no model)

- Shallnot applies the amendments A3 to A6 and their findings below. It reads
  `sentinel` and `examples[]` from the verified proofs.
- The rule that a judged-only VALID is written as UNDETERMINED when pilot switch
  SW1 fires is enforced in the module, so Shallnot stays generic.
- The `proofs` block of `shallnot.yaml` is defined once in ADR 0002.
  `thresholds.validity` is `tau_judge` (default 0.70) and `escalation` defaults to
  `error`. The block is added to `schemas/config.schema.json`, because unknown keys
  are refused.
- **Finding categories added by this ADR:** `example_failed`, `no_valid_mutant`,
  `mutation_sensitive_test`, `budget_exhausted`, `unbound_example_operation`,
  `malformed_example`, `detached_example`, `unknown_dimension`.
  `no_killed_valid_mutant` (ADR 0002) applies when VALID mutants exist and none was
  killed; `no_valid_mutant` applies when none was produced.
- **Escalation semantics.** `escalation` defaults to `error`: an ESCALATE finding
  fails the verdict. In hook mode, the existing `MaxAttempts = 3` release is the
  hand-off to a human. A human resolves an escalation by changing tests, spec or
  committed configuration; there is no acknowledge command. Set to `warning`,
  escalation is lenient, and the benchmark reports both.

### Cost controls and cache keys

| Store | Key | Scope |
|---|---|---|
| Proof (ADR 0001) | JCS of subject and inputs. The inputs are: `spec`; `example_binding`; `test_source` (citing files and files covering the footprint); `code_tree` (per-file, footprint plus import closure); `junit_result`; `mutant_set`; `model_id`; `prompt_template`; `decision_policy`. | local and ledger |
| Generation | statement digest, footprint-slice digest, generator model, prompt digest, round, seed | local only; CI always generates fresh |
| Decision memo | normalized mutant digest, normalized original digest of the touched functions, witness-set digest, in-scope statement digests, decider model, prompt digest, policy digest | local cache, or the CI ledger; never from local to CI |
| Kill-vector memo | mutant digest, touched-function digest, digests of the selected test files | local only; CI executes |
| Alignment memo | masked-input digest, digests of the criteria tokens and statements, model, prompt, policy | local and ledger |

CI freshness and the cost rule are reconciled as follows: a criterion that is not
dirty keeps its ledger proof; a dirty criterion's mutants are regenerated fresh.
Targeted generation in CI needs a model, so running it live is blocked; operator
enumeration, sentinels, kill vectors and example probes need none.

**Execution and load limits.**

- The reasoning budget is `max_reasoning_calls` per criterion per run. Exhausting it
  gives UNDETERMINED (`budget_exhausted`), never a pass.
- Quota exhaustion pauses the run, which then resumes.
- One System One model is resident at a time, and its requests are grouped per
  model.
- At most 4 reasoning calls are in flight, and at most `max_workers` test processes
  run under `nice`.

### Routing, thresholds, calibration

Configuration; its digest is the `decision_policy` input.

| Key | Default | Tuned by |
|---|---|---|
| `tau_judge` (block and pass credit, two judges, confirmed witness) | 0.70 | SW1 |
| `validity_judges` | 2 cross-family | SW5 |
| `tau_route` (System One alignment, two-signal) | 0.90 raw | admission test |
| `tau_screen` (P(VIOLATES)) | 0.20 | screen miss rate |
| `tau_adv` | 0.50 | admission test |
| `align_k`, `modal_min` | 5, 0.8 | SW4 |
| `theta_own`, `theta_other`, `theta_none` | 0.50, 0.20, 0.20 | SW3 |
| `min_valid`, `min_sites`, `max_rounds` | 3, 2, 3 | SW2, SW6 |
| `K_op` | 400 | none |
| `shadow_rate` | 0.10 | SW3, SW6 |
| `max_reasoning_calls` | 40 | SW6 |
| `s1_model` | `nimble:9b` | limit 4 |
| `max_workers` | min(4, cores/2) | none |

**Shadow audit.** A hash-selected `shadow_rate` sample of screened-out operator
survivors, two-signal ALIGNED verdicts and completeness decisions goes to the
reasoning judges.

**Admission test, per (decision kind, model, prompt digest).** Blocked on data:

1. Fit an isotonic map on at least 200 labelled decisions. Labels are
   owner-attributed outcomes and oracle-labelled benchmark items. Agreement with
   reasoning judges is reported separately, never used as truth.
2. On held-out labels, the Wilson upper bound of the error among decisions at or
   above the threshold must be at most 0.10, with a point estimate at most 0.05 and
   coverage at least 0.30.
3. If the test passes, that kind may decide alone at its calibrated threshold.

Admission changes the policy digest.

**Adapters.** Ollama and Kev (`/v1/systemone`), reasoning CLIs and recorded
responses. Jev and OpenAI Decisions are built against recorded contracts; live use
is blocked (paid, no access).

### ADR 0001 amendments

Only where ADR 0001 is silent.

| # | Amendment |
|---|---|
| A1 | Additive input roles `decision_policy` and `example_binding`. |
| A2 | The `junit_result` digest is computed over the canonical map (test id to outcome), never over raw XML, which carries timings. |
| A3 (G2) | Pass credit requires the killed VALID at or above the block threshold. |
| A4 | When no VALID was produced, the gate escalates (`no_valid_mutant`): absent evidence escalates, negative evidence blocks. This diverges from the pilot's rule 4 (G3), and the benchmark reports both. |
| A5 (G1) | A mutant entry may carry `sentinel: true` (EQUIVALENT, basis `construction`). Any test that kills a sentinel raises `mutation_sensitive_test` (escalating), loses all kill credit, and has its alignment capped at UNDETERMINED. |
| A6 (G3) | The discriminating-power predicate carries `examples[] {index, result: {value: passed|failed, class: executed}}`. A failure on the reference blocks (`example_failed`). |
| A7 | `extensions.decision {basis, deciders[], rawConfidence, calibratedConfidence?, threshold, escalationReason?}` per judged claim, plus `extensions.operatorScreen`. |
| A8 | Model-dependent CI stages go through a broker in the outer job over a local socket; the container never holds a key. Requests and responses are recorded in the ledger. Live use is blocked; the workflow is tested with recorded responses. |
| A9 | Decisions may be reused under the full decision key; mutant generation in CI never is. |

### Pilot-dependent parameters and switch rules (pre-registered)

`spec-proof policy derive` reads the pilot's `results/decisions.json` and its
descriptive tables, and writes the policy file deterministically. These rules are
fixed without access to any pilot output. If an endpoint is INCONCLUSIVE, the
default stays. The D- and F-labels name endpoints of the pilot pre-registration.

| Rule | Condition | Effect |
|---|---|---|
| SW1 | D6 or D7 NO-GO | A judged VALID only escalates. BLOCK and pass credit then require owner attribution, and the report states that criteria without examples cannot PASS. |
| SW2a | D8 NO-GO | `max_rounds` = 5. Every DIFFERS operator survivor in the footprint is adjudicated (screen bypassed), so the operator pool also becomes PASS-bearing. |
| SW2b | §11 arm-T oracle-valid rate per candidate ≤ arm-O rate | For criteria whose owner-attributed mutants already meet `min_valid`/`min_sites`, a round becomes one call (family chosen by seed). |
| SW3 | D10 or D11 NO-GO | Two-signal ALIGNED is disabled: ALIGNED needs the reasoning judge, and the matrix keeps only negative and contamination roles. If either is INCONCLUSIVE: `shadow_rate` = 0.25. |
| SW4 | D9 NO-GO | No judge settles ALIGNED alone; ALIGNED needs the judge and a SPECIFIC or OVERBROAD matrix, otherwise UNDETERMINED. `align_k` = 3 if, in the exploratory analysis, the k = 3 subsample verdict equals the k = 5 verdict on ≥ 95 % of tests. |
| SW5 (exploratory) | V-S alone meets D6's GO band on pilot data | Pass-credit judgments on killed mutants use V-S only. Survivors keep two judges. |
| SW6 | F7 amber or red | `max_rounds` = 2, `shadow_rate` = 0.05, `max_reasoning_calls` = 24. |

Arm O's engine is narrower than the product engine, so its yield is a floor. The
product engine's reach on `parkfee` is classified against the hidden oracle, with
no model, as the benchmark's first step; it is reported, not a switch.

### Measurement per limit

| Limit | Measure |
|---|---|
| 1 | False pass and false block from end to end, with cluster-bootstrap intervals, under these policies: traceability only; hybrid; targeted only; operator plus screen only; A4 against the pilot's rule 4. Per gate error, substitution attribution replaces each link by its truth, System One included. |
| 2 | VALID precision against hidden oracles by basis (`spec_example`, `judges`; targeted against flagged operator); planted controls; false-EQUIVALENT rate; screen miss rate (screened-out survivors that the shadow judges find VALID with a confirmed witness); sentinel kills in honest suites (target 0); yield per slot, family and source. |
| 3 | Judges (System One, J-SONNET) and matrix against labels; κ; P(both wrong); false ALIGNED among two-signal verdicts, estimated from the shadow audit with a Wilson interval; hash and snapshot tests caught by the sentinel rule or PINS_OFFSPEC against those caught by the judge. |
| 4 | Per (decision kind, model in {nimble 9B, tev1 4B, tev1 0.8B, Kev 9B}): reliability diagram, ECE and risk–coverage. The first run uses pilot artefacts (masked inputs of the 38 tests against `labels.yaml`; arm T, O and P mutants against oracle classes) and costs zero tokens. |
| 5 | A cheat arm: the agent is told to defeat the gate and is given the local cache, the operator catalogue and the module source. Measured: false pass under a simulated CI (fresh seeds, recorded responses); transfer gap (kill rate on hidden oracle-valid mutants, cheaters against honest agents); share of hash tests caught by sentinels; witness inputs echoed in tests. |
| 7 | Recall of MISSING on specs with dimensions removed; recall of `unjustified_change` and `unconstrained_behaviour` on specs with criteria removed; question precision rated by a blind model, labelled as such. |
| 8 | Per changed criterion: reasoning calls by purpose, tokens, notional `total_cost_usd`, System One calls and latency per model, test CPU-seconds, wall time, cache hit rate per store, escalation rate and `budget_exhausted` rate. |
| 9 | A ledger replay gives a byte-identical report; regenerating the operator stage with the same seed is byte-identical; repeated System One requests agree; cache hits never change a verdict. |

### Test strategy

| Layer | Content |
|---|---|
| Unit | footprint closure; operator families; sentinel catalogue; E1 attribution; validity mapping; stop rule; terminal-outcome table; alignment combination; tournament; template questions; policy derivation (SW1 to SW6) from fixture `decisions.json` files |
| Property-based | Every operator mutant parses and differs; sentinels show no difference from the reference; coverage-selected kills equal full-suite kills on samples. No `mutants[]` entry is VALID, EQUIVALENT or NOT_VIOLATING on an s1-grade answer alone, and no ALIGNED is issued on System One without a positive matrix. An answer that contradicts execution is never accepted. Raising any `tau` never reduces escalations. Every cache key changes when any input changes. Masked inputs never leak. The same requirement in Markdown, YAML `>` and YAML `|` yields an identical `statement_digest` and identical `examples[]`. |
| Fuzz (Go) | example grammar; out-of-scope lines; `detached_example` |
| Contract | one shared `/v1/systemone` suite (types, option limits, confidence presence) across Ollama, Kev, Jev and Decisions with recorded responses; generator and judge schemas; ADR 0001 golden predicates including A5 to A7; report 1.2 schema |
| Integration (local) | live Ollama and Kev; CLI isolation checks; engines, runners and executors on Shallnot's real-runner fixtures |
| End-to-end (CI) | `parkfee` with inline examples added in a copy: `prove`, verifier, `shallnot check`, with recorded responses, byte-identical against golden output; the pilot's arm-P planted controls as a regression suite |

### Layout

The methods are packages of the Go module laid out in ADR 0002: each of
alignment, discriminating power, completeness, routing and policy is a pure domain
package under `internal/domain`, a use case under `internal/app`, and adapters
under `internal/adapters`, with the boundaries enforced by the architecture tests.
The Go source is the implementation; the pilot's Python modules are the reference
for logic that is ported.

## Consequences

- System One may fail even as a screen: a 9B model reading code diffs is outside
  its vendor-reported evaluation sets. A high screen miss rate means bypassing the
  screen, which moves the cost to reasoning judges.
- `nimble:9b` needs about 9.3 GB of RAM alongside daily work; its latency on the
  development laptop is a measured quantity (limit 8).
- Below `K_op`, operator mutants are enumerable whatever the seed. In CI, only
  targeted mutants and seeded positions stay hidden, and generating targeted
  mutants live in CI is blocked. The authoritative CI proof therefore lacks its
  unpredictable component until a broker exists, and limit-5 claims are scoped to
  simulated CI.
- Open-weight System One models need no key, but the project rule that CI runs
  deterministic checks only keeps them out of CI. Judged CI stages stay blocked
  live.
- The footprint can miss behaviour reached through dynamic dispatch, data files or
  plugins, and staleness then goes undetected.
- The example grammar is code-like, so product owners may not write it. Criteria
  without examples rest on reasoning judges, and under SW1 they cannot PASS.
- With ten pilot criteria, most pilot endpoints will be INCONCLUSIVE, so defaults
  rather than measured settings govern most switches.
- Survivor adjudication on weak suites can exhaust budgets. That costs
  escalations, not false passes.
- Two-signal ALIGNED assumes System One errors and matrix errors are uncorrelated.
  Limit 3 measures this.
- A4 makes absent evidence escalate where the pilot's rule blocks; the benchmark
  reports both policies, so the choice is revisable on data.

Blocked, external:

- Calibration-gated autonomy for System One (the admission test) needs run history
  and at least 200 labelled decisions.
- Live use of Jev and OpenAI Decisions: paid access and a waitlist.
- Live model-dependent CI stages (A8): no model may run in CI.
- Tuning `tau_screen`, `tau_adv` and `tau_route` needs run history.
