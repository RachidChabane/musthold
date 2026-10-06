# Pre-registration: pilot on limits 1–3

Pilot experiment for three limits of the per-criterion proof chain defined in
ADR 0001:

- **Limit 1, error multiplication.** End-to-end false blocks and false passes
  of the gate when traceability, discriminating power and alignment are chained.
- **Limit 2, mutant validity.** Generated mutants that do not violate their
  criterion, or are equivalent.
- **Limit 3, alignment judge errors.** An isolated judge that misreads what a
  test verifies, and whether the tests × mutants matrix catches it.

The pilot is descriptive except for the decisional endpoints of §10. It uses one
author-built fixture: 10 criteria, 38 tests, one Python domain. No human labels
any data; every relabel and audit is done by a model and is reported as such.

The pre-registration is frozen at the commit tagged `prereg-limits-1-3`; all
results cite that commit, and no model call on the fixture other than the §3.3
leak audit happens before the tag exists. Any change to the fixture, labels,
oracle, prompts, schemas, bands or analysis code after that commit is a new
pre-registration; the first complete run is still reported.

## 1. Layout and front door

Root: `experiments/limits-1-3/`.

| Path | Content | Read by |
|---|---|---|
| `fixture/parkfee/` | the in-universe project: spec, source, tests | pipeline |
| `prereg/` | this file, codebook, labels, oracle, violations, refactor, planted items, prompts, schemas, model pins | pipeline reads only `prereg/prompts`, `prereg/schemas`, `prereg/models.lock.json` and the `<mutant_id>.json` bodies in `prereg/arm_p/items/`; the ground truth is read only by the pre-registration tools and `analysis` |
| `harness/` | uv project, package `pilot` with `domain`, `application`, `adapters`, `analysis`, `cli` | — |
| `script/` | shebang'd entry points | — |
| `runs/<run-id>/` | raw model I/O, mutants, kill vectors, judgments, proofs, manifest | analysis |
| `results/` | decisions, tables, report | — |

An import-linter contract forbids `domain`, `application` and `adapters` from
importing `analysis`. The pipeline verbs (`run-core`, `run-secondary`) never open
`prereg/labels.yaml`, `prereg/oracle/`, `prereg/violations/` or
`prereg/refactor/`, `prereg/planted/`, `prereg/arm_p/labels.yaml` or
`prereg/arm_p/items/planted_map.json`, and read the arm-P bodies only from the
files of `prereg/arm_p/items/` named `<mutant_id>.json`; a harness test asserts
that no module reachable from those verbs names these paths. The
pre-registration tools (`verify-labels`, `runner-crosscheck`, `assemble-arm-p`)
and `analysis` are the only readers of the ground truth.

Entry points, each `uv run --frozen --project harness python -m pilot.cli <verb>`,
mirrored as `script/<verb>` and as Makefile targets. Pre-registration tools:
`verify-labels`, `runner-crosscheck`, `leak-check`, `traceability-check`,
`assemble-arm-p`, `leak-audit`, `precision-table`. Run: `canary`, `relabel`,
`run-core`, `run-secondary`, `log-deviation`. Analysis: `oracle-classify`,
`audit-sample`, `analyze`, `report-lint`. The run directory format is
`harness/RUN_RECORDS.md`.

Python 3.13.14. One interpreter, the harness virtual environment, runs the
fixture tests, the oracle and the harness; the fixture runtime is stdlib only.

## 2. Fixture

### 2.1 Domain and API

`parkfee`, a car-park tariff, in `fixture/parkfee/src/parkfee/`:

- `quote(entry: datetime, exit: datetime, *, lost_ticket: bool = False, validated: bool = False) -> Decimal`
- `breakdown(entry: datetime, exit: datetime, *, lost_ticket: bool = False, validated: bool = False) -> Breakdown`
  (a dataclass whose structure no criterion specifies)
- `format_receipt(amount: Decimal) -> str`

Private helpers with stable names: `_hourly_charge`, `_in_night_window`,
`_long_stay_charge`, `_validated_free`.

### 2.2 Spec

`fixture/parkfee/spec/parking.md`, Shallnot Markdown, criteria `PARK-1~1` to
`PARK-10~1`, with a stated precedence order. Remaining couplings, avoided by test
design except at their own boundary: (PARK-2, PARK-3) at 15 minutes;
(PARK-3, PARK-4) where 2.50 per started hour reaches the 18.00 cap;
(PARK-9, PARK-10) at 0.00.

Behaviour no criterion covers, used by OFFSPEC tests and changed by the refactor
R: the `ValueError` message text, the Decimal exponent, `Breakdown` field names,
order and values, `repr`, receipt whitespace, rounding of sub-cent amounts,
naive versus aware datetimes.

### 2.3 Test classes (codebook in `prereg/codebook.md`)

| Class | Definition |
|---|---|
| GOOD(A) | aligned to A and discriminating for A |
| GOOD_BROAD(A,B) | aligned to A and also depends on a declared B |
| WEAK(A) | checks a strictly weaker property of A |
| VACUOUS | no behaviour can make it fail: assertion on a mock, tautology, swallowed exception |
| MIS_OTHER(A→B) | cites A, checks B |
| OFFSPEC(A) | cites A, pins behaviour no criterion covers |

Gold good = GOOD ∪ GOOD_BROAD (14 tests). Gold bad = WEAK ∪ VACUOUS ∪
MIS_OTHER ∪ OFFSPEC (24 tests).

### 2.4 Allocation (38 tests, all passing on the reference implementation)

| Criterion | GOOD | GOOD_BROAD | WEAK | VACUOUS | MIS_OTHER | OFFSPEC | Total |
|---|---|---|---|---|---|---|---|
| PARK-1 | 2 | 0 | 0 | 1 swallowed | 0 | 1 message | 4 |
| PARK-2 | 2 | 0 | 1 | 0 | 1 (→3) | 0 | 4 |
| PARK-3 | 2 | 0 | 0 | 1 mock `_hourly_charge` | 0 | 1 Decimal exponent | 4 |
| PARK-4 | 1 | 1 (B=3) | 1 | 0 | 1 (→3) | 0 | 4 |
| PARK-5 | 2 | 0 | 0 | 0 | 1 (→8) | 1 frozen `Breakdown` | 4 |
| PARK-6 | 0 | 0 | 1 | 1 tautology | 1 (→5) | 1 naive datetime | 4 |
| PARK-7 | 0 | 0 | 0 | 1 mock `_validated_free` | 1 (→8) | 1 frozen `Breakdown` | 3 |
| PARK-8 | 1 | 0 | 0 | 1 tautology | 1 (→5) | 1 `repr` | 4 |
| PARK-9 | 2 | 1 (B=3) | 0 | 0 | 0 | 1 whitespace | 4 |
| PARK-10 | 0 | 0 | 1 | 1 tautology | 0 | 1 sub-cent rounding | 3 |

Covered criteria (at least one gold-good test): {1, 2, 3, 4, 5, 8, 9}.
Uncovered: {6, 7, 10}. Poisoned targets of MIS_OTHER and GOOD_BROAD: {3, 5, 8}.

### 2.5 In-universe rules

- Tests split by feature across `test_intervals.py`, `test_tariff.py`,
  `test_night.py`, `test_tickets.py`, `test_receipt.py`; order within each file
  shuffled with seed 20261006.
- Every test name claims its cited criterion's behaviour, as an agent would name it.
- No comment and no docstring in any test file (tokenize check).
- No pytest fixtures, no parametrize; module-level helpers with neutral names only.
- One assertion style across classes.
- GOOD tests stay off coupling boundaries other than their own criterion's; at a
  coupling boundary they use relational assertions.

`leak-check` checks the file set, the absence of comments and docstrings, the
absence of fixtures (no fixture or `usefixtures` decorator, no module-level
`pytestmark`, no test parameter) and of parametrize, the single `verifies`
marker per test and the conftest hook of §2.6. The within-file order, the test
and helper names, the assertion style and the boundary placement are not
decidable from the source and are author rules.

### 2.6 Binding

Each test carries exactly one `@pytest.mark.verifies("PARK-n~1")`. `conftest.py`
holds Shallnot's documented hook verbatim. Traceability comes only from:

`shallnot check --no-config --strict --specs spec --focus spec --tests tests --results <junit.xml> --format json --json-out <run>/traceability.json`

Precondition: 38/38 test cases bound, no `tag_not_in_results`, `orphan_tag`,
`malformed_tag` or `revision_mismatch`, 10/10 criteria covered, verdict pass.
This is the null gate G0: it passes all 10 criteria.

## 3. Ground truth

### 3.1 Artifacts, all committed before this file

- `prereg/violations/PARK-n_{a,b,c}.json`: 30 hand-written violations, each
  violating exactly one criterion (boundary shift, wrong constant, missing branch).
- `prereg/planted/PARK-n_n1.json`: 10 hand-written changes that violate no
  criterion (behaviour outside every criterion, or an equivalent rewrite).
- `prereg/refactor/R.json`: preserves the spec, changes every behaviour of §2.2's
  uncovered list, keeps helper names.
- `prereg/arm_p/items/<id>.json`: the 30 arm-P bodies (violations `a`, `b` and
  the n1 items), each named by its `mutant_id`, written by `assemble-arm-p`;
  `prereg/arm_p/labels.yaml` maps each id to its source, declared criterion and
  kind (r or n1). `assemble-arm-p` also writes the id-to-source map
  `prereg/arm_p/items/planted_map.json`, which no run or analysis verb opens.
- Violations, planted items and R use the mutant edit format of §6.1,
  `{edits: [{file, qualname, new_source}]}`, applied by the same code path as
  generated mutants.
- `prereg/oracle/test_oracle_park_n.py`: one Hypothesis suite per criterion
  (`derandomize=True`, `max_examples=1000`, an explicit `@example` at each
  boundary), written by a model given only the spec and the public API
  signatures, without access to the tests, source bodies or labels.
- `prereg/labels.yaml`: per test `{nodeid, cites, label, shape, checks,
  alignment_truth}`; `alignment_truth` is ALIGNED for gold good, NOT_ALIGNED
  otherwise.
- `prereg/label_matrix.csv`: 38 tests × (30 violations + R), pass/fail.
- `prereg/precision-table.csv`: Wilson 95% intervals at the pilot's
  denominators and reference proportions, generated by `precision-table`.

### 3.2 Label verification by execution (`verify-labels`, no model call)

Uses its own plain pytest invocation, separate from the harness matrix runner.
Every test outcome must be pass or fail: an error, a skip, a timeout or a
missing outcome fails the verification.

1. The reference passes all 38 tests and all 10 oracles; R passes all oracles.
2. Oracle × violation matrix is exactly diagonal: oracle n fails on every
   `PARK-n_*` and passes on every other violation.
3. Every n1 passes every oracle.
4. GOOD(A): fails under every V_A, passes under every V_B (B ≠ A), passes under R.
5. GOOD_BROAD(A,B): fails under every V_A, fails under at least one V_B, passes under R.
6. WEAK(A): fails under at least one V_A but not all, passes under every V_B, passes under R.
7. MIS_OTHER(A→B): passes under every V_A, fails under at least one V_B.
8. VACUOUS: passes under all 30 violations and under R.
9. OFFSPEC: passes under all 30 violations, fails under R.

Any failure is fixed in the fixture or the violations, never by relabelling.
The harness matrix runner applied to the 30 violations and R must reproduce
`label_matrix.csv` exactly (runner cross-check).

### 3.3 Leak audit

Before this file is committed, a cross-family model (`gpt-5.6-terra`, isolated
as in §4.3) reads only `fixture/parkfee/` and lists any name, string or
structure that hints at test quality. Every flag is fixed or rebutted in
`prereg/leak-audit.md`. An audit round that leads to a fix is followed by a
new round on the fixed fixture, up to three rounds; the raw records of round
*k* are kept in `prereg/leak_audit_round<k>/`.

### 3.4 Blind relabels (after this commit, before any judge run)

Two raters, k=1 per test, high effort: R-OAI (`gpt-5.6-terra`) and R-AUTHOR
(`claude-opus-5-5`). Neither is the primary judge model. Each sees the codebook,
the spec, the full source and each test with its declared link; neither sees
labels, label counts, oracle, violations or the poisoned set. Output per test:
`{label, checks}`. Disagreements are never adjudicated into the ground truth.

Reference sets: PRIMARY = author labels that passed §3.2. CONSENSUS = tests on
which author, R-OAI and R-AUTHOR agree on the 6-class label.

## 4. Models and invocation

### 4.1 Pins (`prereg/models.lock.json`)

| Role | Model | CLI |
|---|---|---|
| Mutant generator (3 of 5 calls) | `claude-opus-5-5` | claude |
| Mutant generator (2 of 5 calls) | `gpt-5.6-terra` | codex |
| Validity judge V-S | `claude-sonnet-5-5` | claude |
| Validity judge V-G | `gpt-5.6-terra` | codex |
| Alignment judge J-SONNET (primary) | `claude-sonnet-5-5` | claude |
| Alignment judge J-GPT | `gpt-5.6-terra` | codex |
| Relabel R-AUTHOR, mutant audit A-OPUS | `claude-opus-5-5` | claude |
| Relabel R-OAI, leak audit | `gpt-5.6-terra` | codex |

Every Claude call aborts unless the only `modelUsage` key equals its pin.
Codex reports no resolved model id; the requested slug is recorded with
`provenance: requested_only`. CLI versions are recorded, not pinned:
`prereg/models.lock.json` holds the versions of the pre-registration commit and
no verb rewrites it; the canary records the installed versions in
`runs/<run-id>/canary/`, the run manifest records them at `relabel`, `run-core`
and `run-secondary`, and every call's raw record holds its CLI version. A
version that differs from the lock or changes during the run is a `provenance`
entry of `DEVIATIONS.md` (§4.4), never an abort.

### 4.2 Claude call

From a fresh empty temporary directory outside the repository:

`claude -p --model <pin> --effort <medium|high> --system-prompt "<prereg/prompts/system.txt>" --tools "" --setting-sources "" --strict-mcp-config --disable-slash-commands --no-session-persistence --output-format json --json-schema '<schema>' < item.txt`

The `$schema` keyword is removed from the schema passed to `claude --json-schema`; the schema file itself is unchanged.

Isolation checks per call: pinned `modelUsage` key; total input tokens
(`input_tokens` + `cache_creation_input_tokens` + `cache_read_input_tokens`) ≤
the (model, call type) canary baseline + ceil(chars(item)/2.5) + 500; `num_turns`
equal to the canary value; the directory is empty afterwards.

### 4.3 Codex call

From a fresh empty temporary directory, with the user's normal `CODEX_HOME`:

`codex exec --ephemeral --skip-git-repo-check --ignore-user-config --ignore-rules -s read-only -C <dir> -m gpt-5.6-terra -c model_reasoning_effort=<medium|high> -c web_search=disabled --disable shell_tool --disable hooks --disable plugins --disable apps --disable browser_use --disable computer_use --disable in_app_browser --disable skill_search --disable tool_suggest --disable sleep_tool --disable shell_snapshot --output-schema <schema file> -o out.json --json < item.txt > events.jsonl`

Codex has no system-prompt flag: stdin carries the system prompt and the item,
joined by a blank line.

Isolation checks per call: `events.jsonl` contains only `agent_message` and
`reasoning` items (no command, file, web or tool item); input tokens ≤ the
(model, call type) canary baseline + ceil(chars(item)/2.5) + 500.

### 4.4 Common rules

- Raw request, response, argv, stderr, `total_cost_usd` (Claude), token usage
  (both) stored verbatim under `runs/<run-id>/raw/`.
- An invalid or failed call is retried once, then recorded as missing.
- At most 4 model calls and 4 pytest processes in flight.
- The run is resumable: a call whose raw file exists is not repeated.
- Canary (`canary`), first logged step after this commit: one synthetic item
  unrelated to the fixture per (model, call type) with that call type's real
  schema; measures the token and turn baselines per (model, call type) and
  verifies pins, flags and schemas. Any failure aborts; the fix is a `protocol`
  entry of `DEVIATIONS.md`.
- On subscription quota or rate-limit exhaustion the verb stops and logs a
  `quota-pause` entry; rerunning the same verb resumes it. A stage that cannot
  complete is logged as a `quota-blocked` entry naming the endpoints whose
  inputs it produces, and is reported BLOCKED with its partial data.
- `runs/<run-id>/DEVIATIONS.md` is the run's deviation log: the title
  `# Deviations`, then one entry per line,
  `- <kind> | endpoints: <scope> | <text>`, where `<scope>` is `none`, `all`, or
  endpoint ids separated by commas (`D3, D4`). The kinds form a closed set:

  | Kind | Logged when | Scope | Effect (§10) |
  |---|---|---|---|
  | `provenance` | a CLI version differs from `models.lock.json` or changes between run verbs (pipeline) | `none` | none |
  | `quota-pause` | a run verb stops at a quota or rate limit; a rerun resumes it (pipeline) | `none` | none |
  | `quota-blocked` | a stage cannot complete (operator) | `all` or ids | P7 fails for the named endpoints |
  | `protocol` | the pre-registration commit, the harness digest or the fixture digest changes between run verbs (pipeline, scope `all`); any other departure from this document, such as a canary fix or a rerun (operator, scope = the endpoints whose inputs it changes) | `none`, `all` or ids | P6 fails for the named endpoints |

  Operator entries are written only with `log-deviation`, which accepts
  `quota-blocked` and `protocol` only. Any other non-blank line counts as a
  `protocol` entry with scope `all`.

### 4.5 Prompts, schemas and effort

System prompt for every call: `prereg/prompts/system.txt`, passed with
`--system-prompt` to Claude and on stdin to Codex (§4.3). User message: the
template, filled by `str.format`-style substitution of the named fields.

| Call | Template | Schema | Effort |
|---|---|---|---|
| mutant generation | `generate.md` | `generate.schema.json` | high |
| validity judge, mutant audit | `validity.md` | `validity.schema.json` | medium (judges), high (audit) |
| alignment judge | `alignment.md` | `alignment.tmpl.schema.json`, `__TOKENS__` replaced by the call's tokens | medium |
| leakage probe | `probe.md` | `probe.schema.json` | medium |
| relabel | `relabel.md` | `relabel.schema.json` | high |
| leak audit | `leak_audit.md` | `leak_audit.schema.json` | high |

In `validity.md`, `{differential}` is "An automated comparison found this
distinguishing call: <function>(<args>) returned <original_out> before the change
and <mutant_out> after it." when F3 reports DIFFERS, and empty otherwise.

## 5. Traceability

Run the fixture tests on the reference implementation, then Shallnot as §2.6.
The report's bindings are the declared links D(t) used everywhere downstream.
The pipeline never re-parses JUnit for bindings.

## 6. Mutants

### 6.1 Arm T: criterion-targeted generation

Per criterion, 5 independent calls (3 `claude-opus-5-5`, 2 `gpt-5.6-terra`),
each with a recorded uuid4 nonce in the prompt. Input: the full spec, the
target `ID~REV`, the full `src/` tree. Never tests, conftest, JUnit, labels,
oracle or other mutants. Each call returns exactly 3 mutants in fixed slots:
S1 direct violation; S2 violation only at a boundary or edge input; S3 partial
violation on a strict subset of valid inputs. Each keeps the other criteria
satisfied, uses a mechanism distinct from its siblings, and reads like a
plausible developer bug. Schema `prereg/schemas/generate.schema.json`:
`{mutants: [ {slot, edits: [ {file, qualname, new_source} ] (1–2), mechanism_summary} ] (exactly 3)}`.
In an edit, `file` is relative to `fixture/parkfee/`, `qualname` names a
module-level function, a module-level class or a method `Class.method`, and
`new_source` replaces that whole definition; any other qualname is absent (F0).
Yield: 150 candidates. `mechanism_summary` is never shown to any judge.

### 6.2 Arm P: planted controls

The 20 violations `PARK-n_a`, `PARK-n_b` (declared target n) and the 10 `PARK-n_n1`
items (declared target n), read from `prereg/arm_p/items/` under neutral ids and
injected into the pipeline stream. They are excluded from arm-T statistics
and from the primary matrix. A `mutant_id` is shared across arms: a mutant
identical after normalization to one of an earlier arm, in the order T, P, O,
is processed and judged once under the earlier arm, and D7 reads each planted
item through its id.

### 6.3 Arm O: operator mutants (oracle-only null comparator)

Deterministic AST operator mutants of `src/parkfee/` enumerated at every site
inside function and method bodies (an edit replaces a whole definition, so
module-level statements are out of reach): comparison operator replacement,
arithmetic operator replacement, int and float literal ±1 and ×10 (the string
argument of `Decimal("…")` is not a numeric literal), boolean negation,
return-branch deletion. A replacement that leaves the expression unchanged
(such as 0 × 10) is not a mutant. No model call; they go through F0–F2 and the
oracle classification (§6.6) only.

### 6.4 Deterministic processing (all arms, label-blind)

- **F0 STILLBORN**: edit does not parse, names an absent qualname, changes more
  than 10 lines of the normalized module (difflib opcodes; a replaced block
  counts its larger side), fails an import smoke test (`python -I` with `src/`
  first on `sys.path`, then `import parkfee`), or test collection errors.
  Excluded, counted.
- **F1 NORMALIZE / DEDUPLICATE**: strip comments and docstrings via `ast`,
  re-emit with `ast.unparse`. Identical to the original → TRIVIALLY_EQUIVALENT.
  Identical to an earlier mutant in the same arm → DUPLICATE (merged, provenance kept).
- **F2 KILL VECTOR**: the reference passes the full suite 3/3. Each mutant runs
  the full suite (`pytest -p no:cacheprovider --timeout=10 --junitxml`, no `-x`).
  Per test: killed, survived or error (timeout, collection error and a missing
  outcome count as error). Every kill is rerun once; a non-reproducing kill is
  recorded flaky and counts as survived. A mutant is killed by a test with a
  reproduced kill and survives a test whose result is survived or flaky; an
  error is neither. "Survived every test" means survived or flaky on every test.
- **F3 DIFFERENTIAL**: Hypothesis, `derandomize=True`, `max_examples=2000`, on
  each public function the mutant touches (directly or through a helper),
  comparing return value (`==`, so a `Decimal` compares by value) and exception
  type against the reference. Arguments follow the reference signature.
  Strategies: datetimes in [2020-01-01, 2030-12-31] at minute resolution,
  signed durations of at most 96 hours (an exit before the entry included),
  booleans; amounts as `Decimal` with two places in [0, 1000].
  Result: DIFFERS{function, args, original_out, mutant_out}, NO_DIFF_FOUND or
  SKIPPED(reason).

### 6.5 Validity judges and witnesses (arms T and P)

V-S and V-G, one call each per surviving candidate. Input: the spec, the
normalized original source of the edited definitions and of every function
whose call chain reaches one, the normalized mutant source of the edited
definitions, the unified diff, and the F3 witness if DIFFERS. Never shown: tests,
kills, declared target, slot, arm, generator, `mechanism_summary`.

Output (`prereg/schemas/validity.schema.json`):
`equivalent {value: bool, confidence: 0..1}` and, for each of the 10 criteria,
`{verdict: VIOLATED | NOT_VIOLATED | UNDETERMINED, confidence: 0..1, witness}`,
where a witness is required for VIOLATED:
`{function: quote | format_receipt, entry, exit, lost_ticket, validated, amount, expected: {kind: RETURNS | RAISES, value}}`,
parameters the function does not take being null. Arguments are converted by
type (datetime from ISO 8601, Decimal from string, bool); a RETURNS value
compares as Decimal for `quote` and as a string for `format_receipt`; a RAISES
value is an exception class name. Nothing is evaluated as code. A witness
is confirmed when the reference output equals `expected` and the mutant output
differs or raises (subprocess, 5 s timeout).

Classes per (mutant m, criterion c), τ = 0.70:

| Class | Rule |
|---|---|
| VALID_c | both judges VIOLATED for c with confidence ≥ τ, and ≥ 1 confirmed witness for c; judged confidence = min of the two |
| EQUIVALENT | no reproduced kill, F3 NO_DIFF_FOUND or SKIPPED, both judges `equivalent=true` with confidence ≥ τ |
| UNATTRIBUTED | a reproduced kill or F3 DIFFERS, and both judges NOT_VIOLATED for every criterion with confidence ≥ τ |
| NOT_VIOLATING_c | both judges NOT_VIOLATED for c with confidence ≥ τ |
| OFF_TARGET (arm T) | VALID_d for some d ≠ target and not VALID_target |
| COLLATERAL | VALID for ≥ 2 criteria |
| UNDETERMINED | otherwise |

Primary matrix: an arm-T mutant enters as a VALID mutant of its target when it is
VALID_target and not COLLATERAL. UNATTRIBUTED mutants feed `none(t)`. ADR 0001
discard rule: a mutant that survived every test and is EQUIVALENT or
NOT_VIOLATING for its target is discarded and reported.

Variant F_brief (reported, not primary): drop a VALID_c mutant killed by any
test whose declared binding is a criterion other than c.

One discriminating-power in-toto Statement v1 per criterion is written from arm
T, per ADR 0001 (validity class judged with the min confidence; per-test results
class executed; generator models listed in `extensions.mutantGenerators`).

### 6.6 Oracle classification (analysis only)

For every non-stillborn mutant of every arm: oracle-valid-for-C (fails ≥ 1
oracle test of C and none of other criteria), oracle-collateral (fails tests of
C and of another criterion), oracle-silent (fails no oracle test), split by F3
into silent-equivalent-probable (NO_DIFF_FOUND), silent-different (DIFFERS) and
silent-untested (F3 SKIPPED or not run). An oracle test that times out or whose
file fails collection counts as failing. An oracle file without a reported
outcome leaves the mutant unclassified and `oracle-classify` fails. A DUPLICATE
takes the class of the mutant it duplicates; a TRIVIALLY_EQUIVALENT mutant is
oracle-silent without running the oracle.

### 6.7 Model audit of mutants (A-OPUS)

Sample drawn after the validity judges with seed 20261006: 20 of the
non-stillborn, status-OK arm-T mutants, stratified by pipeline verdict (VALID
for the target vs not) × oracle (valid for the target vs not), allocated
proportionally by largest remainder with a floor of 3 per non-empty cell; plus
every arm-T mutant that survives every test and is VALID for its target; plus
the 30 planted items. The three groups are united, so a mutant appears once.
Isolated call, same input and schema as §6.5, without target, arm, tests, kills,
judge verdicts or oracle. An unconfirmed witness downgrades the verdict to
UNDETERMINED. Reported as "model audit with executed witnesses; no human
labelled any data".

## 7. Alignment judges

### 7.1 Input (built deterministically from `fixture/` only)

- Criteria: all 10 statements without revision; each ID replaced by an opaque
  token `K-xxxx` (first 4 hex of sha256(run_seed‖test_uid‖arm‖rep‖ID), redrawn
  until unique); order permuted by sha256(test_uid‖arm‖rep). Identical across
  judges for the same (test, arm, rep). The token map is stored, never shown.
- Test unit: the test function plus every module-level helper it reaches,
  resolved with `ast`; the imports that bind a name the unit uses kept; every
  `verifies` decorator, comment and docstring removed; re-emitted with
  `ast.unparse` and re-parsed.
  - NAME-MASKED: the function renamed `test_case`, helpers `helper_1..n`.
  - NAME-SHOWN: original names.
- Code under test: public signatures of the imported `parkfee` names, no bodies.
- Never shown: declared criterion, file path, other tests, results, mutants,
  matrix, labels.
- A property test asserts that no NAME-MASKED input contains `verifies` or
  `PARK-`, and that neither its test unit nor its signatures contain a run of 3
  or more consecutive identifier tokens from the original test name. The
  criteria statements are exempt from the name-run check: they are the same for
  every test, so they carry no information about which test is judged.

### 7.2 Output (`prereg/schemas/alignment.tmpl.schema.json`)

`{verifies: <token> | NONE, coverage: FULL | PARTIAL | WEAK | NONE, confidence: 0..100, rationale: ≤ 60 words}`,
with `verifies = NONE` iff `coverage = NONE`. Definitions in the prompt: FULL
checks the required outcome under the stated condition; PARTIAL a correct strict
subset of the clauses; WEAK an implementation violating the criterion would
plausibly still pass.

### 7.3 Runs

k = 5 repetitions per (judge, test, arm): J-SONNET and J-GPT, NAME-MASKED and
NAME-SHOWN. Effort medium.

### 7.4 Aggregation and verdict (label-blind)

Modal criterion over the 5 repetitions (NONE is a class; ties to the higher mean
confidence, then lexicographic); modal coverage among repetitions on the modal
criterion; `modal_share` = count/5. With D(t) the declared criterion:
UNDETERMINED if `modal_share` < 0.8; ALIGNED if modal criterion = D(t) and
coverage ∈ {FULL, PARTIAL}; MISALIGNED otherwise. Sensitivity S1: PARTIAL →
UNDETERMINED. One alignment Statement per (judge, test, arm) with verdict
`{value, class: judged, confidence: modal_share}`.

### 7.5 Leakage probe

J-SONNET and J-GPT, 3 repetitions per test, NAME-MASKED test body only, no
criteria, no API: "Was this test written to verify the behaviour its assertions
target, or something else?" → `{guess: TARGETED | OTHER, confidence: 0..100}`.

## 8. Matrix classifier

Inputs: arm-T primary-matrix VALID mutants and UNATTRIBUTED mutants. For test t
with declared criterion c: own(t) = share of VALID_c mutants killed; other(t) =
max over c′ ≠ c of the share of VALID_c′ killed; none(t) = share of UNATTRIBUTED
killed; DC(t) = own(t) − mean over c′ ≠ c of the share of VALID_c′ killed.
θ_own = 0.50, θ_other = 0.20.

| Signature | Rule |
|---|---|
| UNDEFINED_OWN | c has no VALID mutant |
| INERT | t kills no VALID or UNATTRIBUTED mutant |
| OFF_TARGET | own(t) = 0, and other(t) > 0 or none(t) > 0 |
| PARTIAL | 0 < own(t) < θ_own |
| SPECIFIC | own(t) ≥ θ_own and other(t) ≤ θ_other |
| OVERBROAD | own(t) ≥ θ_own and other(t) > θ_other |

Binary verdict: ALIGNED = SPECIFIC ∪ OVERBROAD; MISALIGNED = OFF_TARGET ∪ INERT
∪ PARTIAL; UNDETERMINED = UNDEFINED_OWN. Matrix criterion label: the single
criterion whose VALID mutants t kills exclusively; null if none; AMBIGUOUS
otherwise. Predicted diagonal: GOOD → SPECIFIC, GOOD_BROAD → OVERBROAD, WEAK →
PARTIAL, VACUOUS → INERT, MIS_OTHER → OFF_TARGET, OFFSPEC → INERT or OFF_TARGET.

The same classifier also runs with the 30 violations as the mutant pool
(oracle-verified kill vectors).

## 9. Gate simulation

### 9.1 Scenarios

For each criterion c, every non-empty subset S of the tests citing c: 134
scenarios (15 for each 4-test criterion, 7 for each 3-test criterion), plus the
10 full sets. Scenarios are clustered by criterion.

### 9.2 Gate rules (ADR 0001, one implementation in `pilot.domain`)

In order, with BLOCK > ESCALATE > PASS:

1. traceability not pass → BLOCK
2. the reference fails a test in S → BLOCK
3. a VALID_c mutant (judged confidence ≥ τ) survives every test in S → BLOCK
4. no VALID_c mutant is killed by a test in S → BLOCK
5. a survived mutant UNDETERMINED for c → ESCALATE
6. a test in S is MISALIGNED or UNDETERMINED in alignment → ESCALATE
7. otherwise → PASS

Rules 3 to 5 read the arm-T mutants generated for c, the mutants listed in c's
discriminating-power Statement (§6.5), excluding discarded ones. Rules 3 and 4
use those VALID for c, COLLATERAL ones included; rule 5 uses those UNDETERMINED
for c. Kills and survivals are those of §6.4 F2.

Policies: G0 traceability only; G1 rules 1–5; G2 rules 1 and 6 with J-SONNET;
G3 all rules with J-SONNET NAME-MASKED (primary); G3-strict (ESCALATE counts as
BLOCK); G3-lenient (ESCALATE counts as PASS); G3 with J-GPT; G3-F_brief; G3-S1.

### 9.3 Scenario truth

Primary, oracle-adjudicated: S is ADEQUATE for c iff S contains a gold-good test
for c and every arm-T mutant that is oracle-valid-for-c is killed by some test
in S; otherwise INADEQUATE. Secondary, label-based: ADEQUATE iff S contains a
gold-good test for c.

False pass = PASS on an INADEQUATE scenario. False block = BLOCK on an ADEQUATE
scenario. Escalation rate = share of ESCALATE.

### 9.4 Error multiplication and attribution

On the 38 single-test scenarios under G3 with label-based truth: per-link error
rates (traceability; J-SONNET alignment against labels; mutant validity against
the oracle; matrix verdict against labels), the end-to-end error predicted under
independence, the observed end-to-end error, and their ratio with a
cluster-bootstrap interval (a ratio above 1 reads as correlated errors). For
every gate error, each link is replaced in turn by its truth (labels for
alignment, oracle class for validity); the substitutions that remove the error
are listed per error.

## 10. Endpoints

Three outcomes: GO, NO-GO, otherwise INCONCLUSIVE; an endpoint with no unit
(n = 0) or an undefined estimate is INCONCLUSIVE. Intervals: Wilson 95% and the
percentile cluster bootstrap by criterion (B = 10,000, seed 20261006); for a
rate over scenarios, mutants or tests the decision interval is the one of the
two with the larger width. D1, D2, D7 and D8 count criteria or items and use
Wilson only; D11 uses the bootstrap only; for D10 the Wilson interval is the
mean of the sensitivity and specificity Wilson bounds.

| ID | Limit | Measure (policy G3, primary truth) | GO | NO-GO |
|---|---|---|---|---|
| D1 | 1 | full-set PASS on an uncovered criterion {6, 7, 10} | 0/3 | ≥ 1/3 |
| D2 | 1 | full-set false block over ADEQUATE full sets (n_A) | ≤ floor(0.15·n_A) | ≥ ceil(0.5·n_A) |
| D3 | 1 | scenario false-pass rate over INADEQUATE scenarios | point ≤ 0.05 and upper ≤ 0.20 | lower > 0.20 |
| D4 | 1 | scenario false-block rate over ADEQUATE scenarios | point ≤ 0.10 and upper ≤ 0.25 | lower > 0.25 |
| D5 | 1 | escalation rate over all scenarios | point ≤ 0.30 and upper ≤ 0.45 | lower > 0.50 |
| D6 | 2 | VALID precision against oracle-valid-for-target, arm T | point ≥ 0.80, lower ≥ 0.70, and precision − N7 ≥ 0.10 (waived if N7 ≥ 0.90) | upper < 0.70 |
| D7 | 2 | planted controls | ≥ 16/20 r-items VALID for their criterion and ≤ 1/10 n1-items VALID for any | ≤ 10/20 r-items, or ≥ 3/10 n1-items |
| D8 | 2 | criteria with ≥ 2 VALID arm-T mutants | 10/10 | ≤ 7/10 |
| D9 | 3 | J-SONNET NAME-MASKED bad-test detection (predicted bad = MISALIGNED ∪ UNDETERMINED; n = 24 gold bad) | recall and precision point ≥ 0.80, lower ≥ 0.60, precision − N2 ≥ 0.10 | either upper < 0.70 |
| D10 | 3 | matrix binary verdict, balanced accuracy over the 38 tests (predicted bad = MISALIGNED ∪ UNDETERMINED) | point ≥ 0.80, lower ≥ 0.65, BA lower bound − N6 mean > 0, mean DC above the N3 95th percentile | upper < 0.70, or mean DC ≤ N3 mean |
| D11 | 3 | Cohen's κ, J-SONNET binary verdict vs matrix binary verdict (determinate items) | point ≥ 0.60, lower ≥ 0.30 | upper < 0.40 |

Endpoint populations: D6 counts the arm-T primary-matrix VALID mutants
(§6.5) and the share of them oracle-valid for their target. D7 counts an
r-item when it is VALID for its declared criterion and an n1-item when it is
VALID for any criterion, COLLATERAL included. D8 counts primary-matrix VALID
mutants per criterion. Mean DC is taken over the tests whose criterion has at
least one VALID mutant.

Nulls: N0 traceability only (G0); N2 always-MISALIGNED judge (precision 24/38,
about 0.632, used unrounded); N3 10,000 permutations of mutant-to-criterion
attribution stratified by per-criterion VALID counts, kill vectors fixed
(one-sided); N6 10,000 permutations of test-to-criterion binding within the
citing-count structure; N7 oracle-valid base rate of non-stillborn arm-T
candidates, DUPLICATE and TRIVIALLY_EQUIVALENT candidates included. The chance
level of N3 and of N6 is the mean of its permutation distribution.

Preconditions. A failed or unevaluable precondition caps its endpoints at
INCONCLUSIVE: GO becomes INCONCLUSIVE and NO-GO stands. A failed or
unevaluable P1, P4 or P7 makes its endpoints INCONCLUSIVE whatever their band.

- P1 §3.2 passes and the runner cross-check is exact (else no pre-registration);
  all endpoints.
- P2 Fleiss' κ on binary `alignment_truth` (author, R-OAI, R-AUTHOR): point ≥
  0.60 and lower ≥ 0.40; caps D9 and D11.
- P3 leakage probe not significant for J-SONNET: one-sided exact binomial
  against 0.5, α = 0.05, over the gold-good and gold MIS_OTHER ∪ OFFSPEC tests
  with a probe answer; a test counts as separated when its modal guess over the
  repetitions (ties as §7.4) is TARGETED for gold good and OTHER for MIS_OTHER ∪
  OFFSPEC; caps D9 and D11.
- P4 canaries pass and every pin matches (else abort); all endpoints.
- P5 judge missing rate ≤ 10%, per judge: V-S and V-G over the non-stillborn,
  status-OK arm-T and arm-P mutants, capping D1–D8, D10 and D11; J-SONNET
  NAME-MASKED over 38 tests × 5 repetitions, capping D1–D5, D9 and D11. J-GPT and
  NAME-SHOWN feed no decisional endpoint.
- P6 no `protocol` entry of `DEVIATIONS.md` (§4.4) names a decisional endpoint;
  caps the endpoints the entries name.
- P7 no `quota-blocked` entry; the endpoints it names are INCONCLUSIVE (blocked).

Composite: REDESIGN(components) if any NO-GO, the component named by the §9.4
substitutions (fallback map D1–D5 gate, D6–D8 mutant validity, D9 judge, D10
matrix, D11 cross-check); PROCEED_TO_POWERED_STUDY if every endpoint is GO, no
precondition failed; INCONCLUSIVE(endpoints, n_needed) otherwise, where n_needed
is the smallest n at which the observed estimate's interval would fall inside
the GO or NO-GO band. At a larger n a proportion keeps its observed value with
the Wilson interval at n (D1–D6, D9), and the BA and κ intervals keep their
shape with their distance from the point scaled by √(n_observed/n) (D10, D11).
D7 and D8, and an endpoint no n decides, report "unreachable"; an endpoint
capped by a precondition reports "blocked". With 11 endpoints at this sample
size, INCONCLUSIVE is the expected composite even if every component performs
well.

## 11. Descriptive measures

- Confusion matrices: 6-class label × matrix signature (arm T, violation pool),
  with the predicted diagonal; 6-class label × judge verdict per judge × arm;
  6-class label × judge answer type (declared, true B, other, NONE) × coverage;
  6-class label × single-test gate outcome under G0–G3; per-criterion full-set
  outcome under every policy; oracle class × pipeline class per arm.
- Judge/matrix cross-check: 3×3 table, κ, raw agreement, P(matrix correct |
  judge wrong), P(judge correct | matrix wrong), both-wrong count, matrix-silent
  count.
- Limit 2: class shares per arm, VALID share backed by a confirmed witness,
  false-discard rate (oracle-valid but not VALID), arm-T vs arm-O
  oracle-valid rate, per-slot yield, per-generator yield, audit κ (A-OPUS,
  pipeline, oracle).
- Limit 3: modal accuracy against PRIMARY and CONSENSUS per judge, stability
  (share with `modal_share` ≥ 0.8, Fleiss' κ over repetitions), name anchoring
  (NAME-SHOWN vs NAME-MASKED false ALIGNED on gold MIS_OTHER ∪ OFFSPEC, exact
  McNemar per judge), J-SONNET vs J-GPT error overlap.
- Contrasts: G3 false-block rate on poisoned {3, 5, 8} vs clean {1, 2, 4, 9};
  GOOD_BROAD vs GOOD signatures; G3 vs G3-F_brief.
- Inter-rater agreement: Cohen's κ per pair, Fleiss' κ, 6-class and binary,
  bootstrap 95% CIs.
- Cost: Claude `total_cost_usd` and codex tokens per criterion, split into
  production path (arm-T generation, two validity judges, J-SONNET NAME-MASKED
  k=5) and experiment overhead; wall clock; pytest CPU-seconds.
- Failure-mode catalogue: every misclassified test, gate-error scenario and
  pipeline/oracle-discordant mutant with its causal chain.

## 12. Feasibility

F1 valid outputs, over the model calls (one per `raw/<call_id>/`), ≥ 95% green,
85–95% amber, else red. F2 isolation violations: 0 green, else red. F3 missing
rate of the worst judge (as P5) ≤ 5% green, ≤ 15% amber. F4 arm-T stillborn ≤
20% green, ≤ 40% amber. F5 criteria with ≥ 2 VALID: 10 green, 8–9 amber. F6
runner cross-check exact: green, else red. F7 mean Claude cost per criterion on
the production path ≤ 5 USD green, ≤ 15 USD amber (the maximum is reported). F8
wall clock, the sum of the stage durations in `timing.json`, ≤ 12 h green, ≤ 36
h amber. Any red → STOP_AND_REDESIGN_PROTOCOL; any amber → AMEND; all green →
PROTOCOL_READY.

## 13. Reporting

`results/decisions.json` holds every endpoint `{id, n, point, lo, hi, band_go,
band_nogo, outcome, capped_by, attribution}`, the composite, feasibility, the
pre-registration commit and the run id. `results/REPORT.md` is rendered from
`prereg/REPORT.tmpl.md` in this order: provenance; conclusions (templates only);
decision table, least favourable bound first; feasibility; preconditions;
confusion matrices; error multiplication and attribution; failure modes; cost;
sensitivity analyses (CONSENSUS, S1, F_brief, label-based truth, G3-strict,
G3-lenient, J-GPT); limits; exploratory discussion of at most 300 words,
labelled exploratory.

Conclusion templates:

- GO: "<endpoint> met its pre-registered band at pilot scale (estimate [lo, hi], n). The fixture is author-built (10 criteria, 38 tests, one Python domain); this does not establish performance on other repositories."
- NO-GO: "<endpoint> fell in its pre-registered NO-GO band (estimate [lo, hi]); <component> must be redesigned before a powered study."
- INCONCLUSIVE: "The pilot cannot decide <endpoint> (estimate [lo, hi]); a decision needs about <n_needed> units."

`report-lint` fails on 'works', 'proves', 'proven', 'validated', 'confirm',
'human audit', 'generalizes', matched as whole words in any case, outside the
conclusion templates and quoted run text, and on a CONSENSUS figure without its
PRIMARY figure. The report never echoes the text of a `DEVIATIONS.md` entry; it
gives the counts per kind and points to the file.

Limits paragraph, verbatim in the report: one language; one domain; 38 tests
and 10 criteria; author-assigned labels; ground truth relative to the violation
set and the oracle; relabels and audits done by models; model family and
capability confounded; unadjusted intervals.

## 14. Run order

1. `canary`.
2. `relabel`: R-OAI and R-AUTHOR.
3. `run-core`: traceability; arm T generation; arm P; F0–F3; validity judges
   and witnesses; J-SONNET NAME-MASKED; J-SONNET leakage probe. These, with the
   relabels and the oracle classification, determine D1–D11 and P1–P7.
4. `oracle-classify` (§6.6), then `audit-sample` (analysis side, reads the
   oracle classification): writes the §6.7 sample as mutant ids only.
5. `run-secondary`: J-SONNET NAME-SHOWN; J-GPT both arms; J-GPT leakage probe;
   arm O; mutant audit on the sample.
6. `oracle-classify` for the arm-O mutants, `analyze`, then `report-lint`.

The decisional run is the first complete run after this commit. A rerun is
allowed only for a P4 or P7 infrastructure failure, before any analysis output
exists, and is logged in the new run as a `protocol` entry with scope `none`
that names the failed run.
