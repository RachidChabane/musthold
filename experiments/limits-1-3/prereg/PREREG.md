# Pre-registration: pilot on limits 1–3

Pilot experiment for three limits of the per-criterion proof chain defined in
ADR 0001:

- **Limit 1, error multiplication.** End-to-end false blocks and false passes
  of the gate when traceability, discriminating power and alignment are chained.
- **Limit 2, mutant validity.** Generated mutants that do not violate their
  criterion, or are equivalent.
- **Limit 3, alignment judge errors.** An isolated judge that misreads what a
  test verifies, and whether the tests × mutants matrix catches it.

The pilot tests the host-delegated design of ADR 0005: one host model executes every product role in
isolated, blind calls, a mutant is valid only by execution, and a judged signal never blocks alone. It is
descriptive except for the decisional endpoints of §10. It uses one author-built fixture: 10 criteria, 38
tests, one Python domain. No human labels any data; every relabel and audit is done by a model and is
reported as such.

The pre-registration is frozen at the commit tagged `prereg-limits-1-3-a1`; all results cite that commit
and its peeled hash. Any change to the fixture, labels, oracle, prompts, schemas, bands or analysis code
after that commit is a new pre-registration; the first complete run is still reported.

The relabels R-AUTHOR and R-OAI and 15 generation calls were executed on 2026-10-06 under the commit tagged `prereg-limits-1-3`, before any judge call and before any outcome file existed; they enter the decisional run by the digests in `prereg/amendment.json`.

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
importing `analysis`. The pipeline verbs (`run-core`, `run-blindspot`, `run-secondary`, `run-luna`) never open
`prereg/labels.yaml`, `prereg/oracle/`, `prereg/violations/` or
`prereg/refactor/`, `prereg/planted/`, `prereg/arm_p/labels.yaml` or
`prereg/arm_p/items/planted_map.json`, `prereg/isolation_table.json` or `prereg/label_matrix.csv`, and read the arm-P bodies only from the
files of `prereg/arm_p/items/` named `<mutant_id>.json`; a harness test asserts
that no module reachable from those verbs names these paths. The
pre-registration tools (`verify-labels`, `runner-crosscheck`, `assemble-arm-p`)
and `analysis` are the only readers of the ground truth.

Entry points, each `uv run --frozen --project harness python -m pilot.cli <verb>`,
mirrored as `script/<verb>` and as Makefile targets. Pre-registration tools:
`verify-labels`, `runner-crosscheck`, `leak-check`, `traceability-check`,
`assemble-arm-p`, `leak-audit`, `precision-table`, `isolation-table`. Run: `canary`, `import-run-01`,
`relabel`, `run-core`, `run-blindspot`, `run-secondary`, `run-luna`, `log-deviation`, `run-window`. Analysis:
`oracle-classify`, `audit-sample`, `amendment-check`, `analyze`, `report-lint`. The run directory format is
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

### 3.1 Artifacts, all committed before any run

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
- `prereg/isolation_table.json`: C_D8 and the violations that a test bound to another criterion kills, written by `isolation-table`; read only by `analysis`.
- `prereg/amendment.json`: the superseded tag and its peeled commit, the declared paths and the digests of the imported files.
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

Before the pre-registration is frozen, a cross-family model (`gpt-5.6-terra`, isolated
as in §4.3) reads only `fixture/parkfee/` and lists any name, string or
structure that hints at test quality. Every flag is fixed or rebutted in
`prereg/leak-audit.md`. An audit round that leads to a fix is followed by a
new round on the fixed fixture, up to three rounds; the raw records of round
*k* are kept in `prereg/leak_audit_round<k>/`.

### 3.4 Blind relabels (before any judge run)

Two raters, k=1 per test, high effort: R-OAI (`gpt-5.6-terra`) and R-AUTHOR
(`claude-opus-5-5`). Neither is the primary judge model. Each sees the codebook,
the spec, the full source and each test with its declared link; neither sees
labels, label counts, oracle, violations or the poisoned set. Output per test:
`{label, checks}`. Disagreements are never adjudicated into the ground truth.

Reference sets: PRIMARY = author labels that passed §3.2. CONSENSUS = tests on
which author, R-OAI and R-AUTHOR agree on the 6-class label.

## 4. Models and invocation

### 4.1 Pins (`prereg/models.lock.json`)

One frozen host model, H = `claude-sonnet-5-5`, executes every product role. The contrast model, X =
`claude-opus-5-5`, is a measurement contrast and never a product role.

| Role | Model | CLI |
|---|---|---|
| Arm-T generation G-H | H | claude |
| Validity votes V-H1, V-H2 | H | claude |
| Alignment judge J-H (primary) | H | claude |
| Contrast generation M_X, test writer T_O, alignment judge J-X, mutant audit A-OPUS | X | claude |
| Test writer T_S | H | claude |
| Relabel R-AUTHOR (executed once, imported) | X | claude |
| Relabel R-OAI, terra mutants, leak audit (executed once, imported or recorded) | `gpt-5.6-terra` | codex |
| Test writer T_L and mutant generator M_L (exploratory) | `gpt-6-luna` | codex |

Every Claude call aborts unless the only `modelUsage` key equals its pin. Codex reports no resolved model
id; the requested slug, passed with `-m`, is recorded with `provenance: requested_only`. CLI versions are
recorded, not pinned: `prereg/models.lock.json` holds the versions at the pre-registration commit and no
verb rewrites it; the canary records the installed versions in `runs/<run-id>/canary/`, the run manifest
records them at every run verb, and every call's raw record holds its CLI version. A version that differs
from the lock or changes during the run is a `provenance` entry of `DEVIATIONS.md` (§4.4), never an abort.

The lock marks each model `active` (calls in the decisional run), `completed` (terra: its calls were
executed once and enter by digest) or `exploratory` (luna).

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

`codex exec --ephemeral --skip-git-repo-check --ignore-user-config --ignore-rules -s read-only -C <dir> -m <slug> -c model_reasoning_effort=<medium|high> -c web_search=disabled --disable shell_tool --disable hooks --disable plugins --disable apps --disable browser_use --disable computer_use --disable in_app_browser --disable skill_search --disable tool_suggest --disable sleep_tool --disable shell_snapshot --output-schema <schema file> -o out.json --json < item.txt > events.jsonl`

Codex has no system-prompt flag: stdin carries the system prompt and the item,
joined by a blank line.

Isolation checks per call: `events.jsonl` contains only `agent_message` and
`reasoning` items (no command, file, web or tool item); input tokens ≤ the
(model, call type) canary baseline + ceil(chars(item)/2.5) + 500.

### 4.4 Common rules

- Raw request, response, argv, stderr, `total_cost_usd` (Claude), token usage
  (both) stored verbatim under `runs/<run-id>/raw/`.
- An invalid or failed call is retried once, then recorded as missing.
- Call ids are unique: no new call reuses the id of an imported or retired raw directory, and a retired
  call (an attempt without a result) is never retried.
- Concurrency: at most 2 model calls in flight, and at most 2 local test, witness or oracle subprocesses
  at once, behind one process-wide semaphore. No local model runs in the pilot.
- The run is resumable: a call whose raw result exists is not repeated, counts neither against the call
  budget nor against the run window, and needs no canary baseline.
- Canary (`canary`), the first logged step of a run: one synthetic item unrelated to the fixture per
  (model, call type) that the run calls on the Claude CLI, with that call type's real schema; it measures the
  token and turn baselines and verifies pins, flags and schemas. The pairs are: H on generate, validity,
  alignment, probe and write_tests; X on generate, write_tests, alignment and audit. Terra is never
  canaried; luna is canaried by `run-luna` on generate and write_tests. Any failure of the canary of the Claude pairs aborts;
  the fix is a `protocol` entry of `DEVIATIONS.md`. A failed luna canary marks every luna call blocked in
  `exploratory_status.json` and logs no deviation. `canary` and `import-run-01` write `timing.json` entries.
- Pacing. Model calls are made only inside the run window: every day from 21:00 to 07:30 local time by
  default, configurable by `--run-window HH:MM-HH:MM` or `PILOT_RUN_WINDOW`; a window whose end precedes
  its start crosses midnight, and the window is open at its start and closed at its end. A verb that
  finds the window closed makes no new call, stops with exit code 3 and logs a `quota-pause` entry with
  the text `run window closed`; a rerun inside the window resumes it. A second cap, `--call-budget N`
  (default 250 new calls per run-window night and CLI; a night is identified by the local date on which its
  window opened, so one night spends one budget across midnight), stops a verb the same way with the text
  `call budget reached`. The budget is a ledger in `runs/<run-id>/call_budget.json` that counts only new
  calls. `script/run-pilot` starts a model-calling step only inside the window.
- On subscription quota or rate-limit exhaustion the verb stops and logs a
  `quota-pause` entry; rerunning the same verb resumes it. A stage that cannot
  complete is logged as a `quota-blocked` entry naming the endpoints whose
  inputs it produces, and is reported BLOCKED with its partial data. The exploratory streams of §14
  (secondary judges, luna) record a per-call status in `exploratory_status.json` instead and never log
  `quota-blocked`.
- `runs/<run-id>/DEVIATIONS.md` is the run's deviation log: the title
  `# Deviations`, then one entry per line,
  `- <kind> | endpoints: <scope> | <text>`, where `<scope>` is `none`, `all`, or
  endpoint ids separated by commas (`D3, D4`). The kinds form a closed set:

  | Kind | Logged when | Scope | Effect (§10) |
  |---|---|---|---|
  | `provenance` | a CLI version differs from `models.lock.json` or changes between run verbs (pipeline) | `none` | none |
  | `quota-pause` | a run verb stops at a quota, the call budget or the run window; a rerun resumes it (pipeline) | `none` | none |
  | `quota-blocked` | a stage cannot complete (operator) | `all` or ids | P7 fails for the named endpoints |
  | `protocol` | the pre-registration commit, the harness digest, the fixture digest or the imports digest changes between run verbs (pipeline, scope `all`); any other departure from this document, such as a canary fix or a rerun (operator, scope = the endpoints whose inputs it changes) | `none`, `all` or ids | P6 fails for the named endpoints |

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
| test writing | `write_tests.md` | `write_tests.schema.json` | high |
| validity vote, mutant audit | `validity.md` | `validity.schema.json` | medium (votes), high (audit) |
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

### 6.1 Generation: arm T and the contrast sources

Per criterion, 5 independent calls of H form arm T (`generate__<ref>__6` to `__10`), each with a recorded
uuid4 nonce in the prompt. Input: the full spec, the target `ID~REV`, the full `src/` tree. Never tests,
conftest, JUnit, labels, oracle or other mutants. Each call returns exactly 3 mutants in fixed slots:
S1 direct violation; S2 violation only at a boundary or edge input; S3 partial
violation on a strict subset of valid inputs. Each keeps the other criteria
satisfied, uses a mechanism distinct from its siblings, and reads like a
plausible developer bug. Schema `prereg/schemas/generate.schema.json`:
`{mutants: [ {slot, edits: [ {file, qualname, new_source} ] (1–2), mechanism_summary} ] (exactly 3)}`.
In an edit, `file` is relative to `fixture/parkfee/`, `qualname` names a
module-level function, a module-level class or a method `Class.method`, and
`new_source` replaces that whole definition; any other qualname is absent (F0).
Yield: 150 candidates. `mechanism_summary` is never shown to any judge.

The same call, with the same prompt and schema, defines the other model sources (arm codes in
`mutants/<id>.json`):

- Contrast source M_X (arm X): X, 3 calls per criterion (`generate__<ref>__1` to `__3`). The 10 calls
  completed before the decisional run enter by digest under their original ids and nonces
  (PARK-1 to PARK-3 slots 1 to 3, PARK-4 slot 1); the 20 others are new.
- Terra source (arm R, exploratory): 5 completed `gpt-5.6-terra` calls (PARK-1 and PARK-2 slots 4 and 5,
  PARK-3 slot 4), imported, oracle-only, never judged and never in arm T.
- Luna source M_L (arm L, exploratory): `gpt-6-luna`, 1 call per criterion (`xgen__<ref>__L`).

M_X, terra and luna mutants feed the oracle classification and the blind-spot estimand only; no judge sees
them and none enters arm T.


### 6.2 Arm P: planted controls

The 20 violations `PARK-n_a`, `PARK-n_b` (declared target n) and the 10 `PARK-n_n1`
items (declared target n), read from `prereg/arm_p/items/` under neutral ids and
injected into the pipeline stream. They are excluded from arm-T statistics
and from the primary matrix. A `mutant_id` is shared across arms: a mutant
identical after normalization to one of an earlier arm, in the order T, P, X, R, O, L,
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

Arms X, R, O and L run F0 to F2 only; F3 runs on arms T and P.

### 6.5 Validity votes and witnesses (arms T and P)

V-H1 and V-H2 are two blind votes of H, one isolated call each per surviving candidate, with identical
inputs: the spec, the normalized original source of the edited definitions and of every function whose
call chain reaches one, the normalized mutant source of the edited definitions, the unified diff, and the
F3 witness if DIFFERS. Never shown: tests, kills, declared target, slot, arm, generator,
`mechanism_summary`. They are the product's validity rule: two blind host votes, AND, τ = 0.70.

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

Classes per (mutant m, criterion c), τ = 0.70, V-H1 and V-H2 being the two votes:

| Class | Rule | May reach |
|---|---|---|
| VALID_w,c | both votes VIOLATED for c with confidence ≥ τ, and ≥ 1 confirmed witness for c; judged confidence = min of the two | matrix pool, D7, D8-all |
| VALID_x,c | VALID_w,c; F0 passed; no reproduced kill and no error on any test whose Shallnot binding D(t) ≠ c | gate (BLOCK, pass credit), D6, D8 |
| EQUIVALENT | no reproduced kill, F3 NO_DIFF_FOUND or SKIPPED, both votes `equivalent=true` with confidence ≥ τ | as stated |
| UNATTRIBUTED | a reproduced kill or F3 DIFFERS, and both votes NOT_VIOLATED for every criterion with confidence ≥ τ | as stated |
| NOT_VIOLATING_c | both votes NOT_VIOLATED for c with confidence ≥ τ | as stated |
| OFF_TARGET (arm T) | VALID_w for some d ≠ target and not VALID_w,target | as stated |
| COLLATERAL | VALID_w for ≥ 2 criteria | as stated |
| UNDETERMINED | otherwise | as stated |

D(t) comes from `bindings.json`, never from labels. Isolation costs no execution, because F2 already runs
the full suite: a mutant that another criterion's test kills or errors on is not evidence about c, and
execution, not a judgment, decides it. A flaky result counts as a survival.

Primary matrix: an arm-T mutant enters as a VALID_w mutant of its target when it is VALID_w,target and not
COLLATERAL. UNATTRIBUTED mutants feed `none(t)`. ADR 0001 discard rule: a mutant that survived every test
and is EQUIVALENT or NOT_VIOLATING for its target is discarded and reported.

One discriminating-power in-toto Statement v1 per criterion is written from arm T, per ADR 0001 (validity
class VALID_x, VALID_w or the other classes, judged with the min confidence; per-test results class
executed; generator models listed in `extensions.mutantGenerators`).


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

Sample drawn after the validity votes with seed 20261006: 20 of the
non-stillborn, status-OK arm-T mutants, stratified by pipeline verdict (VALID_w
for the target vs not) × oracle (valid for the target vs not), allocated
proportionally by largest remainder with a floor of 3 per non-empty cell; plus
every arm-T mutant that survives every test and is VALID_w for its target; plus
the 30 planted items. The three groups are united, so a mutant appears once.
Isolated call of X, same input and schema as §6.5, without target, arm, tests, kills,
votes or oracle. An unconfirmed witness downgrades the verdict to
UNDETERMINED. Reported as "model audit with executed witnesses; no human
labelled any data".

### 6.8 Test writers and the writer suites

Writers: S (H) and O (X) are primary; L (`gpt-6-luna`) and F (the gold-good fixture tests) are
exploratory. Per criterion, one call of each model writer (`write_tests__<ref>__S|O|L`, high effort, a
recorded nonce).

- Input: the spec, the target `ID~REV`, the full `src/` tree and the public API signatures. Writers never
  see fixture tests, the oracle, mutants or labels.
- Output (`prereg/schemas/write_tests.schema.json`): `{tests: [{name, source}]}`, 2 to 5 tests. `source` is
  the import lines the test needs and one function named `name`. The harness adds
  `@pytest.mark.verifies(<ref>)` and writes one module per test, `tests/test_park_<n>__<name>.py`, so that a test whose import fails to collect
  errors that test alone.
- Static check: a deterministic AST check allows imports only from the Python standard library, `pytest`
  and `parkfee`, at any depth of the source, and nothing at top level but imports and the one function. A
  test that fails the check is rejected and kept in the record.
- Reference filter: every suite runs on the reference 3 times; a test that fails or errors in any run is
  dropped. A suite with no remaining test is empty.
- Kill vectors: every evaluated mutant of every source runs against the full set of kept tests of a writer
  with the F2 rules (kills reproduced once; flaky counts as survived), written to `writer_kills.jsonl`.

Mutant sources: each mutant belongs to the source that generated it (S: arm T; O: M_X; op: arm O;
terra; luna). Membership is recorded per source, also for a mutant whose document sits under another arm
because two sources produced the same normalized edit set.


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

k = 5 repetitions per (judge, test, arm): J-H NAME-MASKED (primary, a decisional input), J-H NAME-SHOWN and J-X NAME-MASKED
(judge = test-author model; it feeds no endpoint and gives "G3 with J-X" and the error overlap of J-H and J-X against labels). Effort medium.

### 7.4 Aggregation and verdict (label-blind)

Modal criterion over the 5 repetitions (NONE is a class; ties to the higher mean
confidence, then lexicographic); modal coverage among repetitions on the modal
criterion; `modal_share` = count/5. With D(t) the declared criterion:
UNDETERMINED if `modal_share` < 0.8; ALIGNED if modal criterion = D(t) and
coverage ∈ {FULL, PARTIAL}; MISALIGNED otherwise. Sensitivity S1: PARTIAL →
UNDETERMINED. One alignment Statement per (judge, test, arm) with verdict
`{value, class: judged, confidence: modal_share}`.

### 7.5 Leakage probe

J-H, 3 repetitions per test, NAME-MASKED test body only, no
criteria, no API: "Was this test written to verify the behaviour its assertions
target, or something else?" → `{guess: TARGETED | OTHER, confidence: 0..100}`.

## 8. Matrix classifier

Inputs: arm-T primary-matrix VALID_w mutants and UNATTRIBUTED mutants (VALID_c below means VALID_w,c). For test t
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
criterion whose VALID_w mutants t kills exclusively; null if none; AMBIGUOUS
otherwise. Predicted diagonal: GOOD → SPECIFIC, GOOD_BROAD → OVERBROAD, WEAK →
PARTIAL, VACUOUS → INERT, MIS_OTHER → OFF_TARGET, OFFSPEC → INERT or OFF_TARGET.

The same classifier also runs with the 30 violations as the mutant pool
(oracle-verified kill vectors).

## 9. Gate simulation

### 9.1 Scenarios

For each criterion c, every non-empty subset S of the tests citing c: 134
scenarios (15 for each 4-test criterion, 7 for each 3-test criterion), plus the
10 full sets. Scenarios are clustered by criterion.

### 9.2 Gate rules (one implementation in `pilot.domain`)

In order, with BLOCK > ESCALATE > PASS. The pool of c is the set of arm-T mutants generated for c,
excluding discarded ones (the mutants listed in c's discriminating-power Statement, §6.5). Kills and
survivals are those of §6.4 F2: a mutant survives every test in S when each test's result is survived or
flaky; an error is neither a kill nor a survival.

1. Traceability not pass: BLOCK.
2. The reference fails a test in S: BLOCK.
3. A VALID_x,c mutant (COLLATERAL included, judged confidence ≥ τ) survives every test in S: BLOCK.
4. No VALID_x,c mutant of the pool is killed by a test in S, an empty pool included: ESCALATE
   `no_valid_mutant`. Absent evidence escalates and negative evidence blocks; a pool whose mutants S
   neither kills nor lets survive (every result an error) gives no pass credit.
5. A surviving mutant that is UNDETERMINED for c, or VALID_w,c but not VALID_x,c: ESCALATE.
6. A test in S is MISALIGNED or UNDETERMINED (J-H NAME-MASKED): ESCALATE.
7. Otherwise PASS. Pass credit is a VALID_x,c killed by S.

Every BLOCK carries executed evidence: rules 1 and 2 and the survival of a VALID_x mutant are executed
results. A separate rule "VALID_x exist and none is killed" is implied by rules 3 and 4.

Policies. Primary and component policies: G0 (rule 1 only), G1 (rules 1 to 5), G2 (rules 1, 2 and 6) and G3
(all rules, J-H NAME-MASKED; primary). Sensitivities:

- G3-prereg: rules 1, 2, 3, 5 and 6 over VALID_w in place of VALID_x, and the rule "no VALID_w mutant is
  killed by a test in S" (an empty pool included) blocking;
- G3-noiso: G3 with VALID_w in place of VALID_x;
- G3-strict (ESCALATE counts as BLOCK), labelled "not an admissible product policy";
- G3-lenient (ESCALATE counts as PASS);
- G3-S1 (PARTIAL → UNDETERMINED in the alignment verdict);
- G3 with J-X in place of J-H;
- CONSENSUS (§3.4) and label-based truth (§9.3).

### 9.3 Scenario truth

Primary, oracle-adjudicated: S is ADEQUATE for c iff S contains a gold-good test
for c and every arm-T mutant that is oracle-valid-for-c is killed by some test
in S; otherwise INADEQUATE. Secondary, label-based: ADEQUATE iff S contains a
gold-good test for c.

False pass = PASS on an INADEQUATE scenario. False block = BLOCK on an ADEQUATE
scenario. Escalation rate = share of ESCALATE.

### 9.4 Error multiplication and attribution

On the 38 single-test scenarios under G3 with label-based truth: per-link error
rates (traceability; J-H alignment against labels; mutant validity against
the oracle; matrix verdict against labels), the end-to-end error predicted under
independence, the observed end-to-end error, and their ratio with a
cluster-bootstrap interval (a ratio above 1 reads as correlated errors). For
every gate error, each link is replaced in turn by its truth (labels for
alignment; the oracle class for validity, whose VALID_x veto then runs on the substituted set; the
oracle class for isolation, which replaces the veto of the judged VALID_w set; all links together); the
substitutions that remove the error are listed per error.


## 10. Endpoints

Three outcomes: GO, NO-GO, otherwise INCONCLUSIVE; an endpoint with no unit
(n = 0) or an undefined estimate is INCONCLUSIVE. Intervals: Wilson 95% and the
percentile cluster bootstrap by criterion (B = 10,000, seed 20261006); for a
rate over scenarios, mutants or tests the decision interval is the one of the
two with the larger width. D1, D2, D7 and D8 count criteria or items and use
Wilson only; D11 uses the bootstrap only; for D10 the Wilson interval is the
mean of the sensitivity and specificity Wilson bounds. The endpoint ids are D1 to D11.

| ID | Limit | Measure (policy G3, primary truth) | GO | NO-GO | Switch fed |
|---|---|---|---|---|---|
| D1 | 1 | full-set PASS on an uncovered criterion {6, 7, 10} | 0/3 | ≥ 1/3 | none |
| D2 | 1 | full-set false block over ADEQUATE full sets (n_A) | ≤ floor(0.15·n_A) | ≥ ceil(0.5·n_A) | none |
| D3 | 1 | scenario false-pass rate over INADEQUATE scenarios | point ≤ 0.05 and upper ≤ 0.20 | lower > 0.20 | none |
| D4 | 1 | scenario false-block rate over ADEQUATE scenarios | point ≤ 0.10 and upper ≤ 0.25 | lower > 0.25 | none |
| D5 | 1 | escalation rate over all scenarios | point ≤ 0.30 and upper ≤ 0.45 | lower > 0.50 | none |
| D6 | 2 | VALID_x precision against oracle-valid-for-target, arm T | point ≥ 0.80, lower ≥ 0.70, and precision − N7 ≥ 0.10 (waived if N7 ≥ 0.90) | upper < 0.70 | SW1 |
| D7 | 2 | planted controls | ≥ 16/20 r-items VALID_w for their criterion and ≤ 1/10 n1-items VALID_w for any | ≤ 10/20 r-items, or ≥ 3/10 n1-items | SW1 |
| D8 | 2 | criteria of C_D8 with ≥ 2 VALID_x arm-T mutants | every criterion of C_D8 | three or more criteria of C_D8 short | SW2a |
| D9 | 3 | J-H NAME-MASKED bad-test detection (predicted bad = MISALIGNED ∪ UNDETERMINED; n = 24 gold bad) | recall and precision point ≥ 0.80, lower ≥ 0.60, precision − N2 ≥ 0.10 | either upper < 0.70 | SW3, SW4 |
| D10 | 3 | matrix binary verdict, balanced accuracy over the 38 tests (predicted bad = MISALIGNED ∪ UNDETERMINED) | point ≥ 0.80, lower ≥ 0.65, BA lower bound − N6 mean > 0, mean DC above the N3 95th percentile | upper < 0.70, or mean DC ≤ N3 mean | SW3 |
| D11 | 3 | Cohen's κ, J-H binary verdict vs matrix binary verdict (determinate items) | point ≥ 0.60, lower ≥ 0.30 | upper < 0.40 | SW3 |

Endpoint populations. D6 counts the arm-T primary-matrix VALID_x mutants (§6.5) and the share of them
oracle-valid for their target; the precision of the VALID_w primary-matrix mutants is reported beside it, and
N7 is the oracle-valid base rate of the host's arm T. D7 counts an r-item when it is VALID_w for its
declared criterion and an n1-item when it is VALID_w for any criterion, COLLATERAL included; the r-items
that no other binding's test kills (16 of 20) are reported beside it. D8 counts primary-matrix VALID_x
mutants per criterion over C_D8: GO when every criterion of C_D8 has at least two, NO-GO when three or more
have fewer. C_D8 is the set of criteria with at least one
hand-written violation (`PARK-n_{a,b,c}`) that no test bound to another criterion kills; the `isolation-table`
verb computes it from `label_matrix.csv` and the Shallnot bindings into `prereg/isolation_table.json`, and it
is the ten criteria minus PARK-3.
PARK-3's VALID_x count is reported by name; PARK-5 and PARK-8, with one surviving violation each, stay in
the denominator and are named in advance as exposed. D8-all (≥ 2 VALID_w primary-matrix
arm-T mutants over all 10 criteria: GO 10/10, NO-GO ≤ 7/10) is reported and feeds no switch. D9 to D11 use
J-H and the matrix over the VALID_w pool. Mean DC is taken over the tests whose criterion has at least one
VALID_w mutant.

Nulls: N0 traceability only (G0); N2 always-MISALIGNED judge (precision 24/38,
about 0.632, used unrounded); N3 10,000 permutations of mutant-to-criterion
attribution stratified by per-criterion VALID_w counts, kill vectors fixed
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
- P3 leakage probe not significant for J-H: one-sided exact binomial
  against 0.5, α = 0.05, over the gold-good and gold MIS_OTHER ∪ OFFSPEC tests
  with a probe answer; a test counts as separated when its modal guess over the
  repetitions (ties as §7.4) is TARGETED for gold good and OTHER for MIS_OTHER ∪
  OFFSPEC; caps D9 and D11.
- P4 the run's canaries pass (§4.4) and every pin matches (else abort); all endpoints.
- P5 judge missing rate ≤ 10%, per vote: V-H1 and V-H2 over the non-stillborn,
  status-OK arm-T and arm-P mutants, capping D1–D8, D10 and D11; J-H
  NAME-MASKED over 38 tests × 5 repetitions, capping D1–D5, D9 and D11. J-X and
  NAME-SHOWN feed no decisional endpoint.
- P6 no `protocol` entry of `DEVIATIONS.md` (§4.4) names a decisional endpoint;
  caps the endpoints the entries name.
- P7 no `quota-blocked` entry; the endpoints it names are INCONCLUSIVE (blocked).

Switch rules, fixed without access to any pilot output. If an endpoint is INCONCLUSIVE, the default stays.

| Rule | Condition | Effect |
|---|---|---|
| SW1 | D6 NO-GO on VALID_x, or D7 NO-GO on VALID_w | A judged VALID only escalates. BLOCK and pass credit then require owner attribution, and the report states that criteria without examples cannot PASS. |
| SW2a | D8 NO-GO, on the D8 over C_D8 | `max_rounds` = 5. Every DIFFERS operator survivor in the footprint is adjudicated (screen bypassed), so the operator pool also becomes PASS-bearing. |
| SW2b | §11 arm-T oracle-valid rate per candidate ≤ arm-O rate | For criteria whose owner-attributed mutants already meet `min_valid` and `min_sites`, a round becomes one call (family chosen by seed). |
| SW3 | D10 or D11 NO-GO | Two-signal ALIGNED is disabled: ALIGNED needs the reasoning judge (J-H), and the matrix keeps only negative and contamination roles. If either is INCONCLUSIVE: `shadow_rate` = 0.25. |
| SW4 | D9 NO-GO | No judge settles ALIGNED alone; ALIGNED needs the judge and a SPECIFIC or OVERBROAD matrix, otherwise UNDETERMINED. `align_k` = 3 if, in the exploratory analysis, the k = 3 subsample verdict equals the k = 5 verdict on ≥ 95 % of tests. |
| SW5 (exploratory) | V-H1 alone meets D6's GO band on pilot data | Pass-credit judgments on killed mutants use V-H1 only. Survivors keep two votes. |
| SW6 | F7 amber or red | `max_rounds` = 2, `shadow_rate` = 0.05, `max_reasoning_calls` = 24. |

Switch parameters. `min_valid` = 3, `min_sites` = 2, `max_rounds` = 3, `shadow_rate` = 0.10, `align_k` = 5 and
`max_reasoning_calls` = 40 are the defaults that SW2 to SW6 change. The footprint of a criterion is the union
of the functions executed by its bound tests and examples and those the diff touches, closed over the static
call graph. Owner attribution is a verdict of the criterion's owner, or an example of the specification
that the mutant alone fails; a BLOCK or pass credit that needs it is not given on a judged VALID.

SW1 reads D6 on VALID_x because blocks use VALID_x, and D7 on VALID_w because D7 measures the judges. SW2a
reads D8 over C_D8 because D8-all is non-decisional. SW5 reads V-H1 because the two host votes
are the product's validity rule.

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

### 11.1 Blind spot (reported first)

The central descriptive estimand answers one question: when the same model writes the tests and the
mutants, do they share the same blind spots? A comparison of same-model mutants against operator mutants
alone confounds mutant subtlety with shared blind spots. A crossover over two test writers and two model
mutant sources, anchored on operator mutants (which use no model), isolates the shared part. Truth is
model-free: oracle-valid-for-c. The fixture tests cannot carry the estimand, because they were constructed
to their labels.

- Writers W ∈ {S, O}: the suites T_W,c of §6.8. Mutant sources M ∈ {S, O, op}: S = arm T (host), O = M_X
  (contrast), op = arm-O operator mutants (§6.3). Exploratory cells: writer L (luna) and writer F (the
  gold-good fixture tests bound to c); mutant sources terra (PARK-1 to PARK-3) and luna.
- Population. U(M, c) is the set of mutants of source M that are non-stillborn, deduplicated within the
  source (F1, status OK) and oracle-valid-for-c (§6.6), whatever their declared target. A DUPLICATE takes
  the class of its representative and is not counted twice.
- Escape. For m ∈ U(M, c) the F2 rules apply to T_W,c, where a missing result is an error. m escapes T_W,c
  iff every test of T_W,c survived or was flaky; m is caught iff some test has a reproduced kill; a pair
  (m, W) with an error on any test and no reproduced kill is excluded and counted.
- Common set. C* is the set of criteria c for which T_S,c and T_O,c are non-empty after the reference
  filter, U(S, c), U(O, c) and U(op, c) are all non-empty, and every cell (W, M) has at least one evaluable
  pair. A criterion for which a cell has no evaluable pair has an undefined rate and is outside C*.
  Exclusions are reported per criterion with their reasons.
- Rates. e_c(W, M) is the share of evaluable U(M, c) that escapes T_W,c. e(W, M) is the mean over
  c ∈ C* of e_c(W, M): each criterion weighs 1, so operator volume cannot dominate.
- Estimands. Owner's contrast Δ_W = e(W, W) − e(W, op). Per-writer shared blind spot
  SB_S = Δ_S − [e(O, S) − e(O, op)], and symmetrically SB_O = Δ_O − [e(S, O) − e(S, op)]. Primary
  SB = ½(SB_S + SB_O) = ½[e(S,S) + e(O,O) − e(S,O) − e(O,S)].
- Interval and reading. A percentile cluster bootstrap resamples the criteria of C* with every cell attached
  (B = 10,000, seed 20261006, 95%). The reading is fixed in advance and the first rule that holds wins:
  lower bound > 0 reads "shared blind spot indicated" (the report adds when the upper bound is also below
  0.10); otherwise upper bound < 0.10 reads "no material shared blind spot"; otherwise "undetermined", the
  expected reading at |C*| ≤ 10. The reading is "undetermined" whenever |C*| < 3, whatever the interval, and for an empty C*.
- Also reported: the 2 × 3 table e(W, M) with counts; the location split (the share of escaping model
  mutants whose edited definition also hosts an escaping operator mutant of the same criterion and writer, a
  coverage hole, versus one that does not, a model-specific blind spot); the exploratory cells, labelled
  cross-family where luna or terra is involved and BLOCKED where luna calls are incomplete.
- Caveats, stated in advance: S and O share a family; one writer call per criterion; the truth oracle's
  author model is not recorded.

### 11.2 Other measures

- Confusion matrices: 6-class label × matrix signature (arm T, violation pool),
  with the predicted diagonal; 6-class label × judge verdict per judge × arm;
  6-class label × judge answer type (declared, true B, other, NONE) × coverage;
  6-class label × single-test gate outcome under G0–G3; per-criterion full-set
  outcome under every policy; oracle class × pipeline class per arm.
- Judge/matrix cross-check: 3×3 table, κ, raw agreement, P(matrix correct |
  judge wrong), P(judge correct | matrix wrong), both-wrong count, matrix-silent
  count.
- Isolation table: VALID_w against VALID_x per criterion, with the predicted exposure {3, 5, 8}.
- Same-model correlation: P(V-H2 wrong | V-H1 wrong) against the oracle; J-H and J-X error overlap against labels.
- Source-(2) calibration: reliability, ECE and risk–coverage of J-H `modal_share` against labels, plus the
  UNDETERMINED rate (source 3).
- Limit 2: class shares per arm, VALID_x and VALID_w shares backed by a confirmed witness,
  false-discard rate (oracle-valid but not VALID_w), arm-T vs arm-O
  oracle-valid rate, per-slot yield, per-generator yield, per-source oracle-valid yield (host, contrast,
  terra, luna, operators), audit κ (A-OPUS, pipeline, oracle).
- Limit 3: modal accuracy against PRIMARY and CONSENSUS per judge, stability
  (share with `modal_share` ≥ 0.8, Fleiss' κ over repetitions), name anchoring
  (NAME-SHOWN vs NAME-MASKED false ALIGNED on gold MIS_OTHER ∪ OFFSPEC, exact
  McNemar for J-H).
- Contrasts: G3 false-block rate on poisoned {3, 5, 8} vs clean {1, 2, 4, 9};
  GOOD_BROAD vs GOOD signatures; G3 vs G3-prereg vs G3-noiso.
- Inter-rater agreement: Cohen's κ per pair, Fleiss' κ, 6-class and binary,
  bootstrap 95% CIs.
- Cost: Claude `total_cost_usd` and codex tokens per criterion, split into
  production path (arm-T generation, V-H1 and V-H2, J-H NAME-MASKED
  k=5) and experiment overhead; wall clock; pytest CPU-seconds.
- Failure-mode catalogue: every misclassified test, gate-error scenario and
  pipeline/oracle-discordant mutant with its causal chain.


## 12. Feasibility

F1 valid outputs, over the model calls (one per `raw/<call_id>/`), ≥ 95% green,
85–95% amber, else red. F2 isolation violations: 0 green, else red. F3 missing
rate of the worst judge (as P5) ≤ 5% green, ≤ 15% amber. F4 arm-T stillborn ≤
20% green, ≤ 40% amber. F5 criteria with ≥ 2 VALID_w primary-matrix arm-T mutants, counted over all 10 criteria (D8-all): 10 green, 8–9 amber. F6
runner cross-check exact: green, else red. F7 mean Claude cost per criterion on
the production path (G-H, V-H1, V-H2, J-H NAME-MASKED) ≤ 5 USD green, ≤ 15 USD amber (the maximum is reported). F8
wall clock, the sum of the stage durations in `timing.json`, ≤ 12 h green, ≤ 36
h amber. Any red → STOP_AND_REDESIGN_PROTOCOL; any amber → AMEND; all green →
PROTOCOL_READY.

## 13. Reporting

`results/decisions.json` holds every endpoint `{id, n, point, lo, hi, band_go,
band_nogo, outcome, capped_by, attribution}`, the composite, feasibility, the blind-spot estimand, the
pre-registration commit and the run id. `results/REPORT.md` is rendered from
`prereg/REPORT.tmpl.md` in this order: provenance; conclusions (templates only);
decision table, least favourable bound first; feasibility; preconditions;
blind spot; confusion matrices; error multiplication and attribution; failure modes; cost;
sensitivity analyses (CONSENSUS, S1, label-based truth, G3-prereg, G3-noiso, G3-strict labelled "not an
admissible product policy", G3-lenient, J-X); limits; exploratory discussion of at most 300 words,
labelled exploratory. The provenance section lists both tags, both genTimes, the digest of
`prereg/amendment.json`, the imports table and the counts of the sealed run per kind.

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
set and the oracle; relabels and audits done by models; one model executes every product role; the
contrast model shares its family; the fixture, its tests and labels were authored in a session of the
contrast model; the oracle's author model is not recorded; unadjusted intervals.


## 14. Run order

The decisional run is the first complete run after the commit tagged `prereg-limits-1-3-a1`. Every heavy
verb runs under `script/mem-guard` and `nice`; model calls follow the run window and the call budget of §4.4.

1. `canary` (§4.4): about 9 Claude calls (5 of H, 4 of X).
2. `import-run-01`, then `relabel` as a zero-call resume. `import-run-01` refuses unless every source file
   matches the `SHA256SUMS` of the sealed `runs/run-01`, refuses if any target path exists, copies by digest
   and writes `imports.json` (source path, digest, role). The expected set is exact: 10 completed X
   generations, 5 completed terra generations, both relabels and 12 canary baselines are copied; the one
   retired attempt (a call without a result) is listed in `imports.json` and not copied. It reads
   `plan.json` of `runs/run-01` for the original nonces of the imported calls. `runs/run-01` is sealed by a
   `protocol | all` entry in its `DEVIATIONS.md` and a `SHA256SUMS`, committed before the tag, and never
   analysed; the report gives its counts per kind.
3. `run-core`: traceability, then arm-T generation (50 calls of H), arm P, F0 to F3, V-H1 and V-H2 with
   witnesses, isolation, J-H NAME-MASKED (190 calls) and the leakage probe (114 calls).
4. `run-blindspot`: new M_X calls (20), T_S and T_O (10 each), the reference filter; arm-O enumeration;
   F0 to F2 for M_X, terra and arm O; writer-suite kill vectors for every source.
5. `oracle-classify`, then `audit-sample`.
6. `run-secondary`: J-H NAME-SHOWN (190), J-X NAME-MASKED (190), A-OPUS on the §6.7 sample. It closes on
   2026-10-24 23:59 UTC; the per-call status (`complete`, `blocked` or `pending`) goes to
   `runs/<run-id>/exploratory_status.json`, never to `DEVIATIONS.md`.
7. `run-luna`: window from 2026-10-10 11:01 Europe/Paris (09:01 UTC; the Codex free quota resets then) to
   2026-10-17 23:59 UTC. Content: luna canary, T_L and M_L (10 calls each), F0 to F2, writer suites,
   writer kill vectors; `oracle-classify` then classifies the luna mutants. Each call's status goes to
   `exploratory_status.json`. Anything incomplete at the close is reported BLOCKED in the exploratory tables
   only.
8. Once the secondary stream and luna are both complete or closed: `amendment-check`, `analyze`,
   `report-lint`. Steps 6 and 7 are independent streams: `script/run-pilot` tries each on every invocation, and
   a pause of one never keeps the other from starting.

`amendment-check` runs first in `analyze` and refuses unless all of the following hold: both tags exist;
`git diff --name-only --relative prereg-limits-1-3 prereg-limits-1-3-a1 -- .`, run from the experiment
root, is a subset of the paths declared in `prereg/amendment.json`; each `freeze/*-commit.txt` equals its
peeled tag; each token's message imprint (`openssl ts -reply -text`) equals the SHA-256 of its
`*-commit.txt`; the genTime of the `prereg-limits-1-3-a1` token precedes the earliest `started` in the
run's `timing.json`; `runs/run-01` matches its `SHA256SUMS`; `imports.json` matches the run and
`amendment.json`; `bindings.json` and `reference.json` of the run have the SHA-256 recorded for the same
paths in the `SHA256SUMS` of `runs/run-01`, and the `traceability.json` of the run equals the sealed run's in
its verdict, summary, finding categories and links (the report embeds the path it was produced under, so
the comparison is over that canonical form). The check verifies the imprint and the genTime ordering, not the timestamp authority's
certificate chain.

A rerun is allowed only for a P4 or P7 infrastructure failure, before any analysis output exists, and is
logged in the new run as a `protocol` entry with scope `none` that names the failed run.

