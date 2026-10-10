# Pre-registration: witness-based mutant validity (G3-W)

Measurement of gate policy G3-W of the discriminating-power module. Under G3-W a
surviving mutant blocks a criterion only by failing, on execution, a witness written
blind to the mutant, and only when two scope judges who never see the mutant hold that
the criterion's text alone fixes the witness's output. No judgment alone blocks, and no
judgment alone clears a survivor.

The experiment asks, for G3-W as the product implements it:

- whether it blocks adequate full test sets rarely (D2W) while still blocking
  inadequate ones (RW), the two co-primary endpoints;
- whether it never passes an uncovered criterion (D1W) and rarely passes an
  inadequate test set (D3W);
- how often it blocks adequate test sets and escalates over all test sets (D4W, D5W);
- how often a mutant it calls VALID_e violates its criterion (D6W), and whether the
  witnesses supply at least two VALID_e mutants per criterion (D8W).

Units come from four kinds of source: the pilot fixture `parkfee`, two fixtures
written by `gpt-5.6-terra` (one Python, one Go), and tasks built from real merged pull
requests of repositories that the benchmark's selection did not select. Truth is
executed: oracle tests written by `gpt-5.6-terra`, a model family that writes no
primary witness and casts no gate vote, under a per-source domain statement sealed
before any witness exists. No human labels any item.

Claim power is outside this experiment. Whatever its outcome, the claim-power table
of ADR 0005, as the ADR recording G3-W restates its "judged VALID_c, survivor" row,
governs what a witness, a scope verdict or a host vote may do. The experiment edits
no policy, threshold or configuration of the product.

The benchmark sample does not wait for this experiment. This experiment is an
independent validation that runs beside it, and the benchmark sample takes precedence
for budget (§10).

Freeze: §14.

## 1. Layout and front door

Root: `experiments/validity-w/`.

| Path | Content |
|---|---|
| `PREREG.md` | this document |
| `prompts/` | the truth-role prompts and schemas (domain statement, oracle writer, fixture author), the Codex canary items |
| `truthkit/` | the literal-pool and domain-filter helpers, one Python package and one Go module (§3.6, §3.11) |
| `builder/` | the Go `main` packages of the evaluators (§4.5) |
| `harness/` | uv project, package `validityw` with `domain`, `application`, `adapters`, `analysis`, `cli` |
| `script/` | shebang'd entry points, one per verb |
| `Makefile` | thin targets over `script/<verb>` |
| `units/` | `selection.json`, `units.json`, `truth.lock.json`, one `sealed.age` per unit, `recipients.txt` |
| `freeze/` | the frozen commit hashes and their RFC 3161 timestamps (§14) |
| `runs/<run-id>/` | records of one run (§13.1) |
| `results/` | decisions, tables, report |

The work directory `$VALIDITYW_WORK` lies outside the repository. It holds the
construction work and the construction corpus of the repository tasks, the plaintext
unit files while §2.4 allows them, the clean rooms and the product stores. Nothing
under it is committed except through `seal-truth` (§2.4) and the publication of §14.

Python 3.13.14. Go is the toolchain that the product commit's `go.work` names. Each
verb runs as `uv run --frozen --project harness python -m validityw.cli <verb>`, as
`script/<verb>` and as a Makefile target. The verbs are:

- truth side: `draw`, `snapshot-parkfee`, `author-fixture`, `construct-tasks`,
  `write-domain`, `write-oracle`, `verify-truth`, `bind`, `build-trees`,
  `seal-truth`;
- run side: `product-check`, `unseal`, `canary`, `run-unit`, `run-luna`,
  `count-check`, `log-deviation`;
- analysis: `classify-truth`, `evaluate`, `freeze-check`, `analyze`, `report-lint`.

**Tooling written after the freeze.** These parts are written after the tag
`prereg-validity-w` (§14, step 2), and their digests enter `truth.lock.json` or the run
manifest:

- the harness, `truthkit`, the evaluators and `prompts/`;
- the per-criterion `gpt-5.6-terra` H writer, a writer port of the bench's
  construction H stage, and the domain-statement stage, both bench code in the commit
  tagged `validity-w-product` (§3.2);
- the `validity-w` job of `bench/script/night-budget` (§10).

Ground-truth isolation:

- **Truth files** are the domain statements, the oracle tests H, the reference
  violations, the truth matrices of `verify-truth` and the outputs of
  `classify-truth`. Only the truth-side verbs, `classify-truth`, `evaluate` and
  `analyze` read them.
- **Product inputs** are a unit's spec, source, test trees of §3.9 and planted
  refactors (§4.1, admission). The run-side verbs hand the product nothing else; the
  H tests that tree A* carries are the one truth file a product input holds (§2.4).
- A harness test asserts that no module reachable from `product-check`, `canary`,
  `run-unit` or `run-luna` names a truth file, and that no clean room holds a truth
  path or the plaintext digest of a truth file, the H tests of tree A* excepted.
- `count-check` (§3.2) may read truth labels and gate facts as counts. An
  import-linter contract forbids it from importing the evaluators, the outcome module
  or `analysis`.
- An import-linter contract forbids `domain`, `application` and `adapters` from
  importing `analysis`.

## 2. Inputs

### 2.1 Product source

The product is the tree of the commit tagged `validity-w-product`. It qualifies when
`product-check` finds all of the following at that commit:

- the ADR recording G3-W is accepted, and it supersedes the "judged VALID_c,
  survivor" row of ADR 0005's claim-power table as §4.3 states;
- the discriminating-power module implements, with their tests passing:
  - witness tickets (W), the witness store and the admission of §4.1, including a
    bench-only admission input naming planted refactors, absent from every
    configuration `init` or `enable` writes;
  - scope tickets (C) on every witness that passes the reference, cached per
    (criterion digest, Interface digest), with the refactor results applied after C
    and recorded per witness;
  - the classes of §4.2 and the rules of §4.3 in `internal/domain/power`, with no
    survivor cleared by judgment alone;
  - full-resolution, literal-seeded F3 and compiled-form equivalence E0 in
    `internal/domain/differential`;
  - E3 fault grouping in its report;
  - a record, per witness, per mutant and per test, of every execution result the
    evaluators of §4.5 read;
- **request invariance:** run twice in one clean room with two different test trees
  whose helper functions are equal, the product's canary project emits generation, W
  and C requests with identical `requestId`s, so that the product's own store serves
  them on the second run (ADR 0005: identical work is never asked twice);
- the module's request templates for generation, W, C and V-H, whose digests
  `product-check` records from the module's own template digest function;
- the `allowed-tools` of the `prove-all` skill and of every module's fulfil skill,
  and the executor roles of every module plugin, which `product-check` records as the
  documented tool set of I7 (§5.5);
- the version in `script/claude-code.pin` equals the product's `ClaudeCodeVersion`
  (`script/check-claude-code-pin` passes);
- **bench:** at the same commit, `bench verify-task` passes on at least one task that
  the bench's own construction sealed, and the bench's construction H stage is the
  one §3.2 runs.

`run-unit` exports the tree with `git archive validity-w-product` into
`$VALIDITYW_WORK/product-<commit>/`, builds the module binaries and plugins with the
product's own build script under `script/mem-guard run`, and records the SHA-256 of
every built binary and plugin file. No exported file is edited, except that the
evaluator packages are copied beside the module code (§4.5).

### 2.2 Pilot fixture

`snapshot-parkfee` runs `git archive prereg-limits-1-3-a1` over
`experiments/limits-1-3/fixture/parkfee`, `experiments/limits-1-3/prereg/violations`,
`experiments/limits-1-3/prereg/refactor` and `experiments/limits-1-3/prereg/planted`,
unpacks the result into `$VALIDITYW_WORK/units/parkfee/` and records the SHA-256 of
every extracted file in `units/units.json`.

- The pilot's oracle, labels, prompts and harness are not inputs.
- No file under `experiments/limits-1-3/runs/` and no archived record of a pilot run
  is opened for this experiment.
- Nothing under `experiments/limits-1-3/` is written.

### 2.3 Repository candidates and the sealed benchmark set

- **Candidates.** The records of `bench/selection/20261007/attrition.jsonl` whose
  stage is `not-selected`: 24 subjects in 24 repositories, 9 Python and 15 Go. Their
  mining records come from `bench/mining/20261007/`.
- **Never used.** Every task and every reserve of
  `bench/selection/20261007/selection.json`, every task under `bench/corpus/`, and
  every repository any of them comes from. `draw` refuses a candidate whose
  repository matches one of them, and P10 (§7) checks the whole unit list again.
- **Burned repositories.** The repositories of `units/units.json` are listed there
  as used by this experiment and are ineligible for any later benchmark draw.

### 2.4 Sealing and blindness

- **Sealing.** Every unit file that the product never ships (specs, sources, tests,
  bindings, transplant choices, domain statements, H, reference violations, planted
  refactors) is sealed by `seal-truth` with the bench's sealer
  (`specbench.adapters.sealed_store`, age encryption through the `pyrage` library) as
  one ciphertext per unit, `units/<unit>/sealed.age`, encrypted to
  `units/recipients.txt`, together with `units/truth.lock.json`, which lists the
  plaintext SHA-256 of every sealed file and of every file of `prompts/` and
  `truthkit/` that the wave used. The private key stays outside every path an agent
  session can reach and is committed with the results.
- **Plaintext deletion.** After sealing, `seal-truth` decrypts every ciphertext,
  compares each file with `truth.lock.json`, deletes the wave's plaintext from
  `$VALIDITYW_WORK` and records the deletion in `units/units.json`.
- **Product blindness.** Until the tag `validity-w-product` exists, no product work
  happens with access to the plaintext or the key. The `parkfee` fixture itself is
  public; its domain statement, H and transplant choices are sealed like any other
  unit's.
- **Unsealing, in three phases.** `unseal` runs only after the tag
  `validity-w-product` exists and refuses any file whose digest differs from
  `truth.lock.json`.
  1. Before a unit's runs, it decrypts that unit's product inputs except H (spec,
     source, tests with their bindings, transplant choices and planted refactors) into
     `$VALIDITYW_WORK/cleanroom/<unit>/main/`.
  2. After the unit's runs N, B* and luna (§5.2), it decrypts the unit's H tests into
     the clean room of run A* only.
  3. After the wave's last product run, `classify-truth` decrypts the remaining truth
     files.
- **Gate blindness.** Every product run happens in a clean room under
  `$VALIDITYW_WORK/cleanroom/<unit>/`, which holds the unit's product inputs for that
  run and nothing else (§1).

## 3. Units, clusters and truth

### 3.1 Sources and clusters

A **cluster** is a source project. Every interval resamples clusters (§6.2).

| Source | Clusters | Language | Spec and tests | Criteria (planning) |
|---|---|---|---|---|
| `parkfee` | 1 | Python | the pilot fixture: spec, source, 38 tests with `verifies` markers | 10 |
| `fx-py` | 1 | Python | written by `gpt-5.6-terra` (§3.3) | 8 to 12 |
| `fx-go` | 1 | Go | written by `gpt-5.6-terra` (§3.3) | 8 to 12 |
| repository tasks | 1 per task, at least 9 | Python and Go | real merged pull request; spec by the benchmark's construction (§3.2) | about 3 per task |

The study needs at least 57 criteria in at least 12 clusters.

### 3.2 Repository tasks

**Draw.** `draw` orders the candidates of each language by
`sha256("20261010|" + subject)`, ascending. It takes candidates alternately from the
Python and the Go order, Python first, and continues with the other language when one
is exhausted. It writes `units/selection.json` in the selection format the bench
reads.

**Construction.** `construct-tasks` runs the benchmark's construction (`bench
construct` and `bench verify-task`, ADR 0004 "Corpus" and "Defect ground truth") at
the bench commit of the product tag, with `units/selection.json` as its selection and
with `--corpus`, `--work` and a sealing identity all under `$VALIDITYW_WORK`. It never
writes under `bench/`. It runs every stage the selection skipped (environment build,
offline tests) and then the construction stages. Roles and models are those of ADR
0007's construction table, with two changes, both bench code written after the freeze
and present in the commit tagged `validity-w-product`:

- the domain-statement stage (§3.5) runs before H;
- H is produced by the construction H stage of the bench at the product tag, through
  its writer port set to the per-criterion `gpt-5.6-terra` writer of §3.6 and the Codex
  call of §5.3, never the bench's default H writer.

A task's criteria are those its spec author writes. A candidate dropped at any stage is
recorded in `units/units.json` with its stage and reason, and the next candidate of the
same language replaces it.

**Stopping.** Wave 1 stops when at least 9 tasks pass `verify-task` and §3.8, with at
least 4 per language, or when the candidates are exhausted.

**Extension.** After the product runs of a wave and `classify-truth`, `count-check`
computes four counts and nothing else:

- n_A, the units of D2W's population (§6.1) that are not contested, and the clusters
  they span;
- n_I, the units of RW's population that are not contested;
- n_U, the units of D1W's population;
- the number of VALID_e,c pairs of run N.

When any count is below its requirement (P8, §7), another wave draws the next
candidates in the same order, at most 4 tasks per wave, and is frozen (§14) and run like
wave 1. Waves stop when every count meets its requirement or the candidates are
exhausted. `count-check` never reads a gate outcome, an endpoint estimate or an
interval. A wave's units enter every endpoint under the same rules as wave 1's.

### 3.3 Fixtures `fx-py` and `fx-go`

`author-fixture` makes one `gpt-5.6-terra` call per fixture (high effort, §5.3), with
`prompts/fixture-author.md`. The call returns a self-contained project in the target
language:

- a spec in Shallnot Markdown with 8 to 12 criteria `<PREFIX>-n~1`, each one observable
  behaviour, a stated precedence order and an **Interface** section naming the public
  signatures;
- a reference implementation that uses the standard library only;
- a test suite of 30 to 50 tests written as a project's developers would write them,
  each bound to one criterion by Shallnot's syntax for the language (pytest
  `verifies` marker; `[verifies ID~REV]` subtest suffix in Go), covering criteria
  unevenly, as a real project's suite does;
- at least three single-criterion violations per criterion, in the edit format of the
  pilot (`{edits: [{file, qualname, new_source}]}`);
- two planted refactors that preserve every criterion and change behaviour no
  criterion states.

In-universe rules, checked by `verify-truth` with the language's parser:

- no comment and no docstring in any test file;
- no test name, helper name or string mentions test quality, coverage, mutants or
  violations;
- one assertion style per file.

The fixture's domain must differ from `parkfee`'s and from each other's. A fixture that
fails §3.8 after the repairs of §3.6 is rewritten by one further call, which receives
the failure report; a second failure drops the source and is a `protocol` entry.

### 3.4 `parkfee`

- Spec, source and tests are the snapshot's (§2.2): criteria `PARK-1~1` to
  `PARK-10~1`, 38 tests with one `verifies` marker each.
- Reference violations: the 30 hand-written violations `PARK-n_{a,b,c}`.
- Planted refactors: `R` and the ten `PARK-n_n1` items.
- Domain statement and H are written as for every source (§3.5, §3.6).

### 3.5 Domain statements

`write-domain` makes one `gpt-5.6-terra` call per cluster (high effort, §5.3), with
`prompts/domain.md`. The call sees the spec, its precedence order and the Interface
signatures, and nothing else of the unit. It returns:

- `domain.md`: for every Interface parameter and return value, the type, the
  resolution, the range and the inclusive or exclusive bounds that the spec and the
  Interface support, and every cross-parameter constraint;
- an executable predicate in the unit's language, `accepts(parameter, value)` per
  parameter and `in_domain(operation, args)` per call, which implements `domain.md`
  and returns true for a parameter whose type the statement leaves unconstrained.

The domain statement of a cluster is fixed before its H writer is called and never
changes afterwards. It is never shown to a product role.

### 3.6 Oracle H

`write-oracle` makes one `gpt-5.6-terra` call per criterion (high effort, §5.3), with
`prompts/oracle.md`. The call sees the spec with its precedence, the target criterion,
the Interface signatures, the domain statement and the `truthkit` API, and never a
source body, a test, a violation, a refactor or any product output. It returns
property and example tests for the target criterion:

- **Derandomized.** Python: Hypothesis with `derandomize=True` and
  `max_examples=1000`. Go: `rapid` with a fixed seed and at least 1000 checks.
- **Full resolution.** Strategies draw at the resolution and over the ranges of the
  domain statement, never at a coarser resolution.
- **Boundaries.** An explicit example at every bound the domain statement and the
  criterion state, and at one resolution step on each side of it.
- **Literal pool.** Every value strategy is built with `truthkit.values(kind, base)`.
  With an empty pool it returns `base`. With a pool (§3.11, step A) it draws from the
  pool with probability one half and from `base` otherwise, and it offers a literal
  only to parameters whose `accepts` holds for it.
- **No helpers.** A test file defines test functions only: no module-level helper
  function and no fixture, so that adding H to a tree leaves the project's test-helper
  signatures, an input of W, unchanged.
- **Binding.** One test file per criterion; the harness writes the binding in
  Shallnot's syntax for the language (§3.9).

H_c denotes the oracle tests of criterion c. A call whose tests fail §3.8 receives the
failure report, without any source, in at most two repair calls. A criterion whose H
still fails is excluded with its reason (§3.8).

### 3.7 Reference violations and planted refactors

- **Reference violations V(c).** For `parkfee`, the pilot's violations of §3.4. For
  the fixtures, the author's violations (§3.3). For repository tasks, the
  single-criterion violations of `verify-task` step 3. Each criterion keeps at least
  two after §3.8.
- **Planted refactors.** For `parkfee`, §3.4. For the fixtures, the author's two. For
  repository tasks, the two of `verify-task` step 4.

Planted refactors are product inputs: the admission of §4.1 runs every witness on them.
Reference violations never reach the product.

### 3.8 Truth verification (`verify-truth`, no model call)

Every step runs three times; every test outcome must be pass or fail, and an error, a
skip, a timeout or a missing outcome fails the step.

1. The reference passes every test of the unit and every H test.
2. **Diagonal.** For every criterion c, H_c fails on every member of V(c), and passes
   on every member of V(d), d ≠ c.
3. Every planted refactor passes every H test and every test of the unit.
4. **Domain consistency.** Run under the domain filter of §3.11 in counting mode, H
   makes no call that `in_domain` rejects, on the reference and on every member of
   every V(c).
5. **Repository tasks:** every step of `verify-task` passes with the H of §3.6.
6. **Fixtures:** the in-universe rules of §3.3. **Every source:** the H files obey the
   no-helper rule of §3.6.

A failure is repaired in H (§3.6), in the fixture (§3.3), or by dropping a reference
violation or a planted refactor that cannot pass; never by relabelling. A criterion
left with fewer than two members of V(c), or with an H that fails after its repairs, is
excluded from every endpoint with its reason. A unit left with fewer than two criteria
is dropped and, for a repository task, replaced (§3.2).

`verify-truth` also writes the **truth kill matrix**: every member of every V(c)
against every test of the unit, its pass-to-pass tests included, and every H test, with
the F2 rules of §4.1 (a kill must reproduce once; a flaky result counts as survived; an
error is neither).

### 3.9 Bindings and trees

**Bindings.**

- `parkfee` and the fixtures keep their declared bindings.
- In a repository task, `bind` binds each upstream test (U) to the criterion whose
  members of V it kills most often in the truth kill matrix, ties to the lower
  criterion number; a U test that kills no member of any V(c) is unbound. `bind`
  writes the binding in Shallnot's syntax and changes no assertion; in Go, a test
  function without a subtest is wrapped in one subtest that carries the binding. The
  reference passes every rebound test three times out of three.

A test bound to c is **covering** for c when it kills at least one member of V(c).

**Transplant choice.** For each criterion c whose bound tests are all covering,
`build-trees` chooses one transplant test t_c: in the order
`sha256("20261010|" + unit + "|" + c + "|" + test id)`, the first test of the unit that
kills no member of V(c), taken from the tests bound to other criteria first, then the
unbound U tests, then the task's pass-to-pass tests (R). Criteria choose in spec order;
a test already chosen is skipped. A criterion that needs a transplant and has no
candidate has no B_c unit.

**Per-criterion sets.** For each criterion c:

- **N_c**, the natural full set: the tests bound to c.
- **A_c**, the augmented full set: N_c ∪ H_c, with H_c bound to c and every other
  binding as in N.
- **B_c**, the uncovered full set: the non-covering tests bound to c, plus t_c when
  there is none; t_c is bound to c only, and every other binding is as in N. B_c kills
  no member of V(c) and passes traceability.

**Run trees.** The product runs on three trees per unit, which together carry every
fact the evaluators need for these sets (§4.5):

| Tree | Content |
|---|---|
| N | the unit's tests with the bindings above |
| B* | N without every test that is covering for its own criterion, plus every transplant test t_c, bound to the criterion that chose it |
| A* | N plus every H test, each bound to its criterion |

Every tree keeps every helper function of the unit's test files; removing a test removes
its test function and binding only. A repository tree holds the task's U tests and, of
its R tests, only the transplant tests; every other test file of the merge tree is left
out of the clean room. R tests stay in the truth runs of §3.8.

### 3.10 Scenarios

For every criterion c with k tests in N_c, the scenarios of N are:

- when k ≤ 4: every non-empty subset of N_c;
- when k > 4: the k singletons, the k leave-one-out sets, N_c itself, and 10 further
  subsets drawn without replacement, by seed 20261010, among the subsets of size 2 to
  k − 2 not already listed (fewer when fewer exist).

The scenario population is the scenarios of N together with every A_c and every B_c. A
unit of analysis is a pair (criterion c, test set S) with the bindings of its set.

### 3.11 Oracle classification and truth resolution (`classify-truth`, no model call)

`classify-truth` runs after the last product run of a wave. For every criterion c,
P(c) is the pool of c in run N: the mutants listed in c's discriminating-power
Statement, non-stillborn, one per normalized edit set, whatever their origin
(host-generated, operator or sentinel; origin is recorded). Mutants that runs B* and A*
add are outside P(c). The luna stratum's pool is classified the same way, with its own
witnesses, for its sensitivity only.

**Oracle run.** Each pool mutant is applied to the reference and every H test runs on
it. An H test that times out, or whose file fails to collect or compile, counts as
failing.

**Labels** are computed once per (unit, m, c) and hold for every set, run and policy.
They are assigned in three steps.

1. **Oracle label**, from the oracle run:

   | Label | Rule |
   |---|---|
   | `valid` | m fails at least one test of H_c and no test of H_d, d ≠ c |
   | `collateral` | m fails tests of H_c and of some H_d |
   | `off-target` | m fails some H_d and no test of H_c |
   | `silent` | m fails no H test |

2. **Step A, literal-seeded rerun.** Every mutant labelled `off-target` or `silent`
   reruns H_c with the literal pool of c (below), flagged or not. A mutant that then
   fails H_c is relabelled `valid`, or `collateral` when it fails some H_d, and is
   recorded `gap-closed`.
3. **Step B, domain filter.** A mutant still labelled `silent` that is witness-flagged
   for c is relabelled `out-of-domain` when every failing block-bearing witness of c
   passes under the domain filter (below), and `contested` otherwise. When the filter
   cannot run, the label is `contested`, recorded `unclassified`.

- **Witnesses of c.** The union of the witnesses of c admitted under G3-W in the
  unit's runs N, B* and A*.
- **Witness-flagged.** m is witness-flagged for c when it fails, reproduced, at least
  one witness of c that is block-bearing under G3-W in run N. Witness-flagging uses
  the block-bearing witnesses of run N; the literal pool uses the union.
- **Literal pool of c.** `truthkit` extracts, from every witness of c, every literal and
  every call of the language's standard value constructors whose arguments are all
  literals (Python: `Decimal`, `datetime`, `date`, `time`, `timedelta`; Go:
  `time.Date`, `time.Duration` expressions of constants), adds each value's neighbours
  at one resolution step of the domain statement, and keeps the values `accepts` holds
  for. H_c reruns on m with the pool passed through `truthkit.values`.
- **Domain filter.** The filter wraps every Interface operation of the unit: a call
  whose arguments `in_domain` accepts runs the mutant; any other call returns the
  reference's result for the same arguments, or raises the reference's exception.
  Every failing block-bearing witness of c reruns on m under the filter. A witness that
  still fails is in domain; one that passes is out of domain. Python wraps the
  operations at import; Go dispatches through generated wrappers of package-level
  functions. An operation the filter cannot wrap leaves the rerun `unclassified`.

**Resolutions.** A contested label resolves two ways. Resolution (i), witness right:
the mutant counts as `valid`. Resolution (ii), oracle right: it counts as `silent`.
Every other label is the same under both.

### 3.12 Unit truth

For a unit (c, S), with "kills" in the F2 sense of §4.1:

- **ADEQUATE** when S kills every member of V(c), read from the truth kill matrix, and
  every member of P(c) labelled `valid` or `collateral`; **INADEQUATE** otherwise.
- **Uncovered** when S kills no member of V(c). Every B_c is uncovered by
  construction.
- **Contested** when the unit is ADEQUATE under one resolution of §3.11 and
  INADEQUATE under the other.

A pool mutant whose results on S are all errors is not killed by S.

### 3.13 Not measured

- The CI redraw of witnesses with a fresh nonce, CI-issued requests and simulated CI
  round trips: every gate here runs locally under `trust: local`.
- Forging, the cheat arm and the fake-host personas.
- Owner verdicts as a source of owner attribution: only spec examples attribute.
- A paired comparison with policy G3.
- Witness echo by a coder: no coder session runs.

## 4. The measured gate

### 4.1 Roles and executed stages

Every model role of the product is a ticket of the host protocol (ADR 0005), built by
the product's own blind builders and templates at the frozen commit, and executed by
the module plugins' executor subagents (§5.2).

- **G-H, generation.** The product's generation tickets for each criterion, with the
  product's rounds and seeds. Generation never sees tests or witnesses.
- **W, witness writers.** Two tickets per criterion at the `restricted` floor or above,
  with distinct nonces. Input: every criterion with its precedence, the target
  criterion, the Interface signatures and the signatures of the project's test
  helpers, always those of tree N (§3.6, §3.9). Never shown: implementation bodies,
  mutants, diffs, tests or kills. Output: 3 to 6 examples and at least one derandomized
  full-resolution property, each tagged with the clause it checks. The digest of a
  criterion's witness set is journaled before any generation ticket of that criterion.
  One further W ticket, equally blind, is emitted for a criterion with fewer than two
  VALID_e mutants; its digest is journaled before it runs on any mutant.
- **Reference check, executed.** A witness that imports and passes the reference three
  times out of three goes to the scope judges. A witness the reference fails is a
  reading conflict: it escalates as a spec question and never blocks.
- **C, scope judges.** Two blind tickets per criterion see only the spec and the
  witnesses that pass the reference, and answer per witness: does the criterion's text
  alone, under the precedence, fix this output?
- **Refactor admission, executed after C.** Each witness that passes the reference
  runs on every planted refactor of the unit (the bench-only admission input of §2.1);
  its results are recorded per witness. Under G3-W a witness is **admitted** when it
  passes the reference and every planted refactor; an admitted witness both scope
  judges approve is **block-bearing**, and any other admitted witness is
  escalate-only.
- **V-H, validity votes.** Two tickets on each survivor that fails no block-bearing
  witness. They never block; their EQUIVALENT only corroborates an F3 exclusion.
- **J-H, alignment.** The alignment module's name-masked judge tickets, `align_k` per
  test as the product configures it.
- **Executed stages** (no model): F0 stillborn, F1 normalization, F2 kill vectors (a
  kill must reproduce once; a flaky result counts as survived; an error is neither a
  kill nor a survival), F3 full-resolution literal-seeded differential, F4 witness
  confirmation, E0 compiled-form equivalence and E3 fault grouping, as the product
  implements them.

### 4.2 Mutant classes, per criterion c

| Class | Rule |
|---|---|
| VALID_e,c | fails, reproduced, at least one block-bearing witness of c; F0 passed; no reproduced kill and no error on any test bound to d ≠ c |
| owner-attributed | fails a spec example of c and no example of another criterion |
| collateral | VALID_e for several criteria; blocks each criterion it survives, earns no credit |
| excluded | E0 identity with the reference; or F3 NO_DIFF_FOUND at full resolution, both V-H votes EQUIVALENT and no confirmed F4 witness; or fails an example or witness of d ≠ c and none of c's |

### 4.3 Rules, in order, BLOCK > ESCALATE > PASS

For a criterion c and its test set S, with the pool P(c):

1. Traceability does not pass: BLOCK.
2. The reference fails a test in S: BLOCK.
3. A VALID_e,c or owner-attributed mutant survives every test in S: BLOCK.
4. No credit-bearing mutant (rule 7) of the pool is killed by S, an empty pool
   included: ESCALATE.
5. Any other survivor not excluded by execution (§4.2): ESCALATE, shown with its V-H
   votes and its F4 witness.
6. A test in S is MISALIGNED or UNDETERMINED for the alignment module: ESCALATE.
7. Otherwise PASS. A credit-bearing mutant is an owner-attributed mutant, or a
   VALID_e,c mutant that is not collateral and fails a block-bearing witness written at
   the floor; pass credit is such a mutant killed by S.

A witness below the floor blocks and earns no credit. Every BLOCK rests on an executed
failure.

### 4.4 Policies

- **G3-W (primary).** §4.1 to §4.3 as stated, with the refactor admission.
- **G3-W-shipped.** G3-W without the refactor admission, as the product runs on a
  user's repository: every witness that passes the reference is admitted, and the scope
  verdicts alone make it block-bearing. Its D2W enters the composite (§6.5); every
  other figure of it is a sensitivity.
- **G3-W-noalign (sensitivity).** Rule 6 removed; it isolates mutant validity.
- **G3-W-strict** (ESCALATE counts as BLOCK), labelled "not an admissible product
  policy", and **G3-W-lenient** (ESCALATE counts as PASS), sensitivities.
- **G3-W-luna (sensitivity).** The luna run of tree N (§5.3), with luna's witnesses in
  place of the W tickets'; luna's witnesses are below the floor.

### 4.5 Evaluators and parity

- **Gate evaluator.** The Go `main` package `builder/evaluate`, copied into
  `discpower/cmd/validity-w-evaluate/` of the product export, the only place from
  which the module's `internal` packages can be imported. It imports
  `internal/domain/power` and computes the outcome of one (criterion, S, bindings,
  policy) from recorded facts: pool, witnesses with their reference, refactor and scope
  results, V-H votes, E0, F3 and F4 results, kill vectors per test, reference results,
  traceability and alignment verdicts.
- **Alignment evaluator.** The Go `main` package `builder/align`, copied into
  `alignment/cmd/validity-w-align/`, recomputes a test's alignment verdict for a
  declared criterion from its recorded J-H votes with the module's own aggregation.
- **Facts of a set.** Pool, witnesses and classes come from run N, with VALID_e,c's
  isolation recomputed on the set's own bindings. Kill vectors and J-H votes of a test
  come from the run that executed it: run N for the tests of N, run B* for transplant
  tests that N does not bind, run A* for H tests. V-H votes come from the union of runs
  N, B* and A*.
- **Sources of outcomes.** Every outcome of every policy, for every unit of §3.10,
  comes from the gate evaluator. The evaluator computes classes and outcomes only
  through `internal/domain/power`, with the set's binding map as input; it implements
  no rule of §4.2–§4.3.

**Parity.** For every full set of runs N, B* and A*, the gate evaluator's G3-W
outcome on that run's own facts and bindings must equal the product's own
per-criterion verdict, read from the Shallnot report of that run: `pass` is PASS, a
blocking finding is BLOCK, and any escalation or pending reason is ESCALATE.

**Counterfactual parity.** After run A*, for one criterion per cluster, drawn by seed
20261010, `run-unit` runs the A_c and B_c trees in the same store, under the input
guard of §5.2; the product's verdict must equal the evaluator's, or P9 fails. Parity
and counterfactual parity feed P9 and Q6.

**Missing outcomes.** A unit whose outcome cannot be computed, because a fact is
missing or the product ended a ticket `host_unresponsive`, counts as an error when the
GO band is tested and as a non-error when the NO-GO band is tested; D1W with a missing
B_c cannot be GO. The number of such units is reported per endpoint.

## 5. Models, execution and isolation

### 5.1 Pins (`runs/<run-id>/models.lock.json`)

| Role | Model | CLI |
|---|---|---|
| G-H, W, C, V-H, J-H, the product's other tickets, the gate-host session | `claude-sonnet-5-5` | Claude Code, the private copy of `script/claude-code.pin` |
| Domain statements, H, fixture authors | `gpt-5.6-terra` | codex |
| W in the luna run | `gpt-6-luna` | codex |
| Construction roles of repository tasks other than H and the domain statement | as ADR 0007's construction table: `claude-opus-5-5`, `claude-haiku-4-5`, `claude-sonnet-5-5` | Claude Code, as the bench pins it |

- Every Claude result whose resolved model differs from its pin is an isolation
  violation (§5.5).
- When Codex reports no resolved model id, the requested slug is recorded with
  `provenance: requested_only`.
- CLI versions: Claude Code is pinned (ADR 0011, decision 13). Codex is recorded at
  every verb; a change between verbs is a `provenance` entry (§8).

### 5.2 Gate-host session (product path)

Every product run is one or more gate-host sessions in the bench's `IsolatedCliHost`
(ADR 0007, "Host execution"; ADR 0011, decisions 1, 9 and 13):

- the clean room holds the run's product inputs after `init` and `enable`, with
  `trust: local` and the product's default configuration apart from the admission
  input of §2.1;
- the session runs the pinned private copy of `claude` with `DISABLE_AUTOUPDATER=1`,
  model `claude-sonnet-5-5`, the module plugins loaded with `--plugin-dir`, a
  bench-owned `CLAUDE_CONFIG_DIR` with `--setting-sources user`, `HOME` set to an empty
  bench-owned directory and `GIT_CONFIG_GLOBAL=/dev/null`;
- it is prompted to run the umbrella skill `prove-all`, and every ticket is executed
  live by a plugin executor subagent (`model: inherit`) through `fetch_request` and
  `submit_response`. No response is imported from another store.

**Run order and the product's own cache.** For each unit, runs N, B* and A* use the same
clean-room path, `$VALIDITYW_WORK/cleanroom/<unit>/main/`, and so the same product
store; `run-unit` swaps the test tree between runs and changes nothing else. Run N
completes first, then B*, then A*. A request whose `requestId` the store already holds
is served by the product itself; every other request is a live ticket.

**W input guard.** Before each gate-host session of runs B* and A*, `run-unit` runs
`prove` without a host and lists the pending tickets with `request list --json`. It
refuses to start the session when a pending G-H, W or C ticket carries an input digest
that no G-H, W or C request of run N carries, role by role. A refusal is a `protocol` entry naming
the unit, and that unit's A_c or B_c units are missing (§4.5).

### 5.3 Codex calls

Every Codex call of this experiment (terra and luna) runs from a fresh empty temporary
directory, with `HOME` set to an empty directory the harness owns,
`GIT_CONFIG_GLOBAL=/dev/null` and the owner's `CODEX_HOME` as an absolute path:

`codex exec --ephemeral --skip-git-repo-check --ignore-user-config --ignore-rules -s read-only -C <dir> -m <slug> -c model_reasoning_effort=high -c web_search=disabled -c include_apps_instructions=false --disable shell_tool --disable hooks --disable plugins --disable apps --disable browser_use --disable computer_use --disable in_app_browser --disable skill_search --disable tool_suggest --disable sleep_tool --disable shell_snapshot --output-schema <schema file> -o out.json --json < item.txt > events.jsonl`

Stdin carries the system prompt and the item, joined by a blank line.

**Luna run.** For each unit, `run-luna` runs the discriminating-power module alone on
tree N in its own clean-room path, `$VALIDITYW_WORK/cleanroom/<unit>/luna/`, with a
fresh store, after runs N and B* and before run A*. It runs `prove` until it awaits the
W tickets, which precede every generation ticket, reads the two W tickets of each
criterion with `request show`, answers them by two luna calls with their distinct
nonces, and submits the answers with `request submit`, which records them `exposed`:
below the floor, they block and earn no credit (ADR 0005, isolation floor). Only then
does a gate-host session execute the remaining tickets live. The luna run has its own
pool; its alignment verdicts are those of run N, recomputed by the alignment evaluator.

### 5.4 Construction calls

The roles of `bench construct` other than H and the domain statement run under the
bench's own isolation rules: model pin, input tokens bounded per reported turn at 2.0
characters per token (`input_token_bound`), and the call's result discarded on a
violation. These calls enter Q2 (§9) through the bench's records.

### 5.5 Isolation checks and violations

A **call** is one logical model invocation: one ticket of the product, one Codex call,
or one construction call, with every attempt and retry it takes. A call is a
**violating call** when any check below fails on any of its attempts. Violations are
counted per call, never per failed check or per attempt.

For an executor ticket of kind k, with chars the length of the fetched prompt and
output_chars the length of its first submitted output:

- **I1 attestation.** The response is `attested`: the executor's tool calls are exactly
  `fetch_request(t)`, `submit_response(t)` and at most one corrective
  `submit_response(t)`. A response marked `leaked`, `exposed` (luna excepted, §5.3) or
  `resubmission_conflict` fails I1.
- **I2 model.** The resolved model equals the pin.
- **I3 turns.** The reported turns are at most T_k + 1, with T_k from the canary. The
  one tolerated extra turn is the corrective submit or a structured-output retry, and
  carries no tool call other than `submit_response(t)`.
- **I4 tokens.** Input tokens (`input_tokens` + `cache_creation_input_tokens` +
  `cache_read_input_tokens`) are at most
  turns × (B_k + ceil(chars / 2.0)) + (turns − 1) × ceil(output_chars / 2.0) + 500,
  with turns the reported number and B_k from the canary (§5.6).

For a Codex call, with turns the number of `turn.completed` events in `events.jsonl`,
chars the length of stdin and output_chars the length of `out.json`:

- **I5 events.** `events.jsonl` holds only `agent_message` and `reasoning` items.
- **I6 tokens.** Input tokens are at most
  turns × (B + ceil(chars / 2.0)) + (turns − 1) × ceil(output_chars / 2.0) + 500, with
  B from the Codex canary of its (model, call type).

For a gate-host session:

- **I7 session.** `system/init` lists exactly the module plugins and executor servers
  of the product commit and the pinned CLI version. Every tool call of the main agent
  belongs to the documented tool set that `product-check` records (§2.1): a command the
  skills' `allowed-tools` allow, a `Skill` call of those skills, or an `Agent` call whose
  `subagent_type` is a module executor role. No main-agent tool call names a path
  outside its clean room, the product export, the resolved toolchain directories and
  its bench-owned homes, and the product journals no `exposure_attempt`. A failed I7
  makes every ticket the session executed a violating call.

**Effect.** The product rejects a response that fails I1 and re-emits the ticket with a
fresh nonce while emissions remain (ADR 0005, lifecycle). A Codex or construction call
that fails a check is discarded and retried once with a fresh nonce, then recorded
missing. An accepted product response that fails I2, I3, I4 or I7 stays in the store;
every unit whose outcome consumed it is listed, and §12's isolation sensitivity
excludes those units.

### 5.6 Canary

`canary` is the first logged step of each wave's product runs.

- **Gate-host canary.** One gate-host session on the product's own canary project,
  outside every unit, runs at least one ticket of each kind (generation, W, C, V-H,
  alignment, `decide.closed`). It checks the live-canary list of ADR 0005 ("Test
  strategy": plugins and servers in `system/init`, executor without Bash, main-session
  `fetch` denied, rewrite drops `model`, cap refused without hooks, Stop fires), and
  records per kind T_k, the reported turns, and
  B_k = max(0, ceil((in_k − T_k × ceil(chars_k / 2.0) − (T_k − 1) ×
  ceil(output_chars_k / 2.0)) / T_k)), from its input tokens in_k, prompt length
  chars_k and output length output_chars_k.
- **Codex canary.** One synthetic item per (model, call type) with the call type's
  real schema: terra on domain, oracle and fixture-author; luna on witness. It records
  B by the same formula.
- A failure of the gate-host canary or of a terra canary aborts; the fix is a
  `protocol` entry. A failed luna canary marks the luna stratum BLOCKED and logs no
  deviation.

### 5.7 Common rules

- Raw request, response, argv, stderr, cost (`total_cost_usd` for Claude) and token
  usage of every call are stored verbatim under `runs/<run-id>/raw/`, and the product
  stores and journals of every run under `runs/<run-id>/stores/`.
- Call ids are unique; a retired attempt is never retried under its id.
- The run is resumable: a call whose raw result exists is not repeated and counts
  against no budget.

## 6. Endpoints

### 6.1 Populations

| Endpoint | Unit | Population |
|---|---|---|
| D2W | full set | the ADEQUATE sets among every N_c and every A_c |
| RW | full set | the INADEQUATE sets among every N_c |
| D1W | full set | every B_c (the uncovered sets) |
| D3W | scenario | the INADEQUATE units of the scenario population (§3.10) |
| D4W | scenario | the ADEQUATE units of the scenario population |
| D5W | scenario | every unit of the scenario population |
| D6W | (m, c) pair | the VALID_e,c pairs of run N, collateral pairs included |
| D8W | criterion | C_D8W: the criteria with at least one member of V(c) that no test bound to another criterion in N kills |

### 6.2 Estimators

- **Intervals.** Wilson 95 % and the percentile cluster bootstrap by cluster
  (B = 10,000, seed 20261010, clusters resampled with every unit inside them). For a
  rate, the decision interval is [min(lo), max(hi)] over the Wilson and the bootstrap
  intervals. D1W and D8W count units and use their count rules only.
- **Outcomes.** GO, NO-GO, otherwise INCONCLUSIVE. An endpoint with no unit or an
  undefined estimate is INCONCLUSIVE.
- **n_needed.** For an INCONCLUSIVE rate, the smallest n at which the observed
  proportion's Wilson interval would fall inside the GO or the NO-GO band; "unreachable"
  when no n does, and for D1W and D8W.
- Intervals are unadjusted for multiplicity and labelled so.

### 6.3 Endpoint table (policy G3-W, primary truth)

| ID | Measure | GO | NO-GO |
|---|---|---|---|
| D2W (co-primary) | full-set false block: BLOCK share over ADEQUATE full sets | point ≤ 0.10 and upper ≤ 0.20 | lower > 0.25 |
| RW (co-primary) | full-set block recall: BLOCK share over INADEQUATE natural full sets | point ≥ 0.80 and lower ≥ 0.65 | upper < 0.60 |
| D1W | PASS count over uncovered sets | 0 of n_U | ≥ 1 |
| D3W | scenario false pass: PASS share over INADEQUATE scenarios | point ≤ 0.05 and upper ≤ 0.20 | lower > 0.20 |
| D4W | scenario false block: BLOCK share over ADEQUATE scenarios | point ≤ 0.10 and upper ≤ 0.25 | lower > 0.25 |
| D5W | escalation: ESCALATE share over all scenarios | point ≤ 0.30 and upper ≤ 0.45 | lower > 0.50 |
| D6W | VALID_e precision: share of VALID_e,c pairs labelled `valid` or `collateral` | point ≥ 0.95 and lower ≥ 0.90 | upper < 0.85 |
| D8W | criteria of C_D8W with at least two VALID_e,c mutants | every criterion of C_D8W | three or more criteria short |

Errors that each endpoint can record, so that none is empty by construction:

- D2W and D4W: a BLOCK on an ADEQUATE set, through a surviving VALID_e,c labelled
  `off-target` or `out-of-domain`, an owner-attributed survivor, or rules 1 and 2.
  Because A_c binds H_c to c alone, an `off-target` mutant stays VALID_e,c there.
- RW: an ESCALATE or PASS on an INADEQUATE natural full set.
- D3W: a PASS on an INADEQUATE set, through a `valid` survivor that the gate excluded
  or a member of V(c) that S lets survive.
- D6W: a VALID_e,c pair labelled `off-target` or `out-of-domain`.

### 6.4 Contested units

- **Share.** For each endpoint, the contested share is the number of contested units
  over the units in the endpoint's population under either resolution, computed before
  any exclusion. A unit of D6W is contested when its pair is labelled `contested`.
- **Primary analysis.** D2W, RW, D3W, D4W and D6W exclude contested units. D1W, D5W and
  D8W do not depend on truth labels.
- **Threshold.** An endpoint whose contested share exceeds 0.10 is INCONCLUSIVE,
  whatever its band.
- **Both ways.** The decision table gives, beside each primary figure of D2W, RW, D3W,
  D4W and D6W, the figure under resolution (i) and under resolution (ii), each with its
  interval and its band outcome.

### 6.5 Co-primary rule and composite verdict

D2W and RW are co-primary. G3-W meets its primary claim, that it blocks adequate full
sets rarely while it still blocks inadequate ones, only when both are GO; no
multiplicity adjustment applies, because both must hold.

- **REDESIGN(components)** when any endpoint of §6.3 is NO-GO, or when the D2W of
  G3-W-shipped is NO-GO. The component is the one §6.6's substitutions name; otherwise
  the fallback map: D2W, D4W and D6W name witness scope; RW, D5W and D8W name witness
  supply; D1W and D3W name the gate rules.
- **ADOPT** when D2W, RW, D1W and D3W are GO, no endpoint is NO-GO, the D2W of
  G3-W-shipped is not NO-GO, and no precondition caps any of these four. D4W, D5W, D6W
  and D8W may be GO or INCONCLUSIVE; an INCONCLUSIVE one is named in the conclusion.
- **INCONCLUSIVE(endpoints, n_needed)** otherwise.

### 6.6 Attribution

For every error of D2W, D4W, RW and D3W, `analyze` replaces one link at a time by its
truth and lists the substitutions that remove the error:

- **S-valid:** a survivor blocks only when its label is `valid` or `collateral`;
- **S-domain:** witness failures that step B classifies out of domain are not
  block-bearing;
- **S-align:** rule 6 removed;
- **S-supply:** every pool mutant labelled `valid` counts as VALID_e,c;
- all four together.

S-valid names witness validity, S-domain witness scope, S-align the alignment judge and
S-supply witness supply. The component of a NO-GO endpoint is the one whose
substitution removes the most of its errors, ties going to the fallback map.

## 7. Preconditions

A failed or unevaluable precondition caps its endpoints: GO becomes INCONCLUSIVE and
NO-GO stands. A failed or unevaluable P1, P2, P4 or P7 makes its endpoints
INCONCLUSIVE whatever their band.

- **P1 truth.** Every unit entering an endpoint passed §3.8, and `analyze` reproduces
  `truth.lock.json` and the truth kill matrix digests. All endpoints.
- **P2 freeze order.** `freeze-check` (§14) passes. All endpoints.
- **P3 product.** `product-check` passes (§2.1), the bench item included. Otherwise,
  no run.
- **P4 canaries and pins.** The canaries pass and every pin matches (§5.6). All
  endpoints.
- **P5 missing rate.** Per role, the share of run-N tickets with no accepted response
  after re-emissions is at most 0.10: G-H, W and C cap every endpoint; V-H and J-H cap
  D2W, RW, D1W and D3W–D5W.
- **P6 protocol.** No `protocol` entry of `DEVIATIONS.md` names the endpoint.
- **P7 quota.** No `quota-blocked` entry names the endpoint.
- **P8 sample size**, counted on the units that enter the primary analysis, contested
  units excluded.
  - D2W: n_A ≥ 40 in at least 12 clusters.
  - RW: n_I ≥ 30 natural full sets.
  - D1W: n_U ≥ 30.
  - D6W: at least 150 VALID_e,c pairs.
- **P9 parity.** Parity of §4.5 is exact on every full set of runs N, B* and A*, and
  counterfactual parity holds on every A_c and B_c run it draws. Caps
  D2W, RW, D1W, D3W, D4W and D5W.
- **P10 disjointness.** No unit comes from a task, reserve or repository of §2.3's
  never-used list. Otherwise, no run.
- **P11 witness order.** For each criterion, its witness-set digest is journaled before
  its first generation ticket, and the digest of each further W ticket is journaled
  before that witness runs on any mutant. A criterion that fails is excluded from every
  endpoint, and the exclusion is listed.

## 8. Deviations

`runs/<run-id>/DEVIATIONS.md` holds the title `# Deviations`, then one entry per line:

`- <kind> | endpoints: <scope> | <text>`

`<scope>` is `none`, `all`, or endpoint ids separated by commas. The kinds form a
closed set:

| Kind | Logged when | Writer | Effect |
|---|---|---|---|
| `provenance` | a recorded value this document does not pin (Codex CLI version, OS build, Python patch, uv version) differs from the lock or changes between verbs | pipeline | none |
| `quota-pause` | a verb stops at the daily budget, a quota or the memory guard; a rerun resumes it | pipeline | none |
| `quota-blocked` | a stage cannot complete | operator | P7 fails for the named endpoints |
| `protocol` | the product commit, a built binary digest, the harness commit, the bench commit or `truth.lock.json` changes between verbs (pipeline, scope `all`); a W input guard refusal (pipeline, §5.2); any other departure from this document, such as a canary fix, a rerun, or a harness behaviour this document does not fix (operator, scope = the endpoints whose inputs it changes) | pipeline or operator | P6 fails for the named endpoints |

- Operator entries are written only with `log-deviation`, which accepts
  `quota-blocked` and `protocol`.
- **Timestamps.** Every `provenance` and `protocol` entry, whoever writes it, is
  timestamped: its writer stores the entry line in
  `runs/<run-id>/deviation-timestamps/<UTC date>-<slug>.txt` and an RFC 3161 token of
  that file from `http://timestamp.digicert.com` as `<…>.tsr`, with its query as
  `<…>.tsq`. A pipeline that cannot reach the authority records the entry and takes the
  token before `analyze`. `analyze` refuses when a token is missing or its message
  imprint differs from the SHA-256 of its `.txt`.
- Any other non-blank line counts as a `protocol` entry with scope `all`.
- A rerun of a unit's product runs is allowed only after an infrastructure failure of
  the session or the machine, before any `classify-truth` output exists for that wave,
  and is a `protocol` entry with scope `none` naming the failed run. A rerun after
  `unseal` phase 2 uses a fresh clean-room path and store.

## 9. Feasibility

| ID | Check | Green | Amber |
|---|---|---|---|
| Q1 | valid outputs over all calls | ≥ 95 % | 85–95 % |
| Q2 | isolation: violating calls (§5.5) | 0 | only calls whose output was discarded, or whose output fed no endpoint (canaries, luna run) |
| Q3 | missing rate of the worst role (P5) | ≤ 5 % | ≤ 15 % |
| Q4 | stillborn share of run-N pool candidates | ≤ 20 % | ≤ 40 % |
| Q5 | criteria of run N with at least one block-bearing witness | ≥ 90 % | ≥ 75 % |
| Q6 | parity (§4.5) | exact | — |
| Q7 | mean Claude cost per criterion on run N's production path (G-H, W, C, V-H, J-H) | ≤ 1.00 USD | ≤ 3.00 USD |
| Q8 | budget days with model calls, all waves (§10) | ≤ 15 | ≤ 30 |
| Q9 | witness flake: admitted witnesses with a non-reproducing failure on some pool mutant | ≤ 5 % | ≤ 15 % |
| Q10 | witness-flagged pairs that step B leaves `unclassified` | ≤ 10 % | ≤ 25 % |

Any value outside both columns is red. Any red gives STOP_AND_REDESIGN_PROTOCOL, any
amber REVISE_PROTOCOL, all green PROTOCOL_READY. Feasibility changes no endpoint
outcome.

## 10. Cost and budget

Planning figures, for 57 criteria in 12 clusters; the measured figures replace them in
the report.

| Source | Calls (planning) | Basis |
|---|---|---|
| Sonnet tickets, run N | about 2,000 | per criterion about 4 G-H, 2–3 W, 2 C, at most 4 V-H; J-H on about 250 tests × `align_k` |
| Sonnet tickets, runs B* and A* | about 1,400 | V-H on the survivors of B*; J-H on the transplant tests N does not bind and on about 230 H tests × `align_k`; extra W where VALID_e falls below two |
| Sonnet tickets, luna run | about 750 | G-H, C and V-H of the luna run, discriminating power only |
| Gate-host sessions | about 60 | one or more per run and unit, plus canaries |
| Construction, Claude | about 500 | about 40 calls per attempted repository task |
| terra | about 150 | 12 domain statements, about 57 H calls and their repairs, 2 fixture authors and their repairs, canaries |
| luna | about 120 | 2 W per criterion, canary |

- **Budget order.** Claude executions are shared in this order: the canary reserve,
  the benchmark's construction while a selected task is unsealed, the benchmark
  sample, then this experiment (its construction calls included). Every Claude
  call of this experiment takes its share from `bench/script/night-budget` under the
  job `validity-w` (tooling written after the freeze), which reads the same ledgers
  under the same lock (ADR 0011, decision 7). The `validity-w` job sets
  `NIGHT_BUDGET_PILOT_LEDGER` to a ledger under `$VALIDITYW_WORK`; an absent pilot
  ledger counts as zero executions.
- **Daily allowance.** A budget day is a local calendar date. The job's allowance is
  at most 500 Claude executions per budget day, within the daily cap the owner sets
  for the harness (`NIGHT_JOBS_CLAUDE_CAP`), and at most what the higher-priority jobs
  leave. A verb that reaches its allowance stops with a `quota-pause` entry and resumes
  on the next budget day.
- **Q8** counts only the budget days on which the `validity-w` job received a share
  above zero; a day the higher-priority jobs consume entirely is not counted.
- **Ceiling.** The experiment stops every model-calling verb at 6,000 Claude
  executions in total, construction included; reaching it is a `quota-blocked` entry
  naming the endpoints whose inputs are incomplete.
- **Codex.** terra and luna calls take their share of the Codex cap through the same
  job and resume on quota without loss. Codex calls run one at a time, and at a
  moderate pace from 07:30 to 21:00 local time: at most 10 calls started per hour.
- Cost per criterion, per role, per run and per source is reported (§12), with the
  maximum beside the mean.

## 11. Machine safety and pacing

The laptop is its owner's daily workstation. These rules implement ADR 0005, "Machine
safety", for this experiment. When a rule stops or defers work, that is a
`quota-pause` entry, never a reason to relax the rule.

- **Guard.** Every heavy verb (product builds, product runs, `verify-truth`,
  `classify-truth`, the construction of repository tasks, `evaluate`, `analyze`) runs
  under `script/mem-guard run` and `nice`: it refuses to start below 40 % free memory
  and stops cleanly when the memory pressure leaves normal.
- **Workers.** At most 2 heavy workers from 21:00 to 07:30 local time and 1 at other
  hours. Go builds use `go build -p 2` (`-p 1` at other hours) and Go tests
  `go test -p 1`; pytest runs without parallel workers; mutant, witness and oracle
  subprocesses run behind one process-wide semaphore of the same size. A gate-host
  session counts as one heavy worker, and the product runs with its worker setting at
  1 outside 21:00–07:30.
- **Slots.** Every heavy verb holds one slot of `script/with-slot`; the machine-wide
  limit of two slots stands, shared with the canaries, construction and the benchmark.
- **Hours.** Remote model calls (Claude and Codex) may run at any hour, within §10's
  allowance; Codex calls follow §10's pace. Local heavy work follows the guard and the worker limits above at every
  hour.
- **No local model** is loaded by this experiment. No container is started.
- **Cleanup.** At the end of each verb, no clean-room process, gate-host session,
  Codex process or test subprocess it started is left running.

## 12. Descriptive measures and sensitivity analyses

None of these feeds a band or the composite, except the D2W of G3-W-shipped (§6.5).

**Descriptive:**

- witness supply per criterion: written, reading conflicts, scope-approved, refactor
  drops, block-bearing, escalate-only; the scope rejection rate, the agreement of the
  two C judges, and the share of refactor-dropped witnesses that both C judges
  approved;
- VALID_e, owner-attributed, collateral and excluded counts per criterion, with the
  exclusion route (E0, F3 with V-H, other criterion's witness);
- truth labels, with the `gap-closed`, `out-of-domain`, `contested` and
  `unclassified` counts;
- E3 fault classes per criterion, and the endpoints recomputed with one unit per fault
  class;
- V-H agreement with truth on unwitnessed survivors, and P(V-H2 wrong | V-H1 wrong);
- per-source and per-language tables (`parkfee`, fixtures, repository tasks; Python,
  Go), and per-cluster figures;
- the confusion table of truth (ADEQUATE, INADEQUATE, uncovered) × outcome (BLOCK,
  ESCALATE, PASS) for N_c, A_c, B_c and the scenarios;
- missing-outcome counts per endpoint (§4.5) and W input guard refusals (§5.2);
- error attribution (§6.6) per error;
- cost, tokens and wall time per role, run and source; rejects, expiries and abandons
  per ticket kind.

**Sensitivity:**

- contested units both ways (§6.4), shown in the decision table;
- D2W on natural full sets only (every N_c);
- RW pooled over the INADEQUATE N_c and every B_c, shown beside RW;
- G3-W-shipped, G3-W-noalign, G3-W-strict ("not an admissible product policy"),
  G3-W-lenient and G3-W-luna (§4.4); G3-W-luna is BLOCKED where luna calls are
  incomplete;
- clustering by criterion in place of cluster;
- every endpoint without the units whose outcome consumed a response of a violating
  call (§5.5);
- every endpoint per wave.

## 13. Records and reporting

### 13.1 Run directory

| File | Content |
|---|---|
| `manifest.json` | run id; the commits tagged `prereg-validity-w`, `validity-w-product` and each `validity-w-truth-<k>`; the harness and bench commits; built binary and plugin digests; template digests per ticket kind; the documented tool set of I7; CLI versions; seeds; the unit digests |
| `models.lock.json` | §5.1 |
| `canary/` | gate-host and Codex canary records with T_k and B_k |
| `raw/<call_id>/` | §5.7 |
| `stores/<unit>/<run>/` | the product's store, journal, Statements and Shallnot report after each run |
| `facts/<unit>/<run>.json` | the facts the evaluators read (§4.5) |
| `truth/<unit>/` | oracle runs, labels, step A and step B records |
| `timing.json` | per verb, unit and run: start, end, calls, executions |
| `budget.json` | per budget day: the `validity-w` share and the executions spent |
| `DEVIATIONS.md`, `deviation-timestamps/` | §8 |
| `exploratory_status.json` | per-call status of the luna run |

### 13.2 Results

- `results/decisions.json`: every endpoint `{id, n, clusters, point, lo, hi, method,
  band_go, band_nogo, outcome, capped_by, missing, contested: {share, n, resolution_i,
  resolution_ii}, attribution}`, the D2W of G3-W-shipped, the composite, feasibility,
  the commits and the run id.
- `results/REPORT.md`, rendered from `harness/src/validityw/analysis/REPORT.tmpl.md`,
  in this order:
  1. provenance: tags, commits, genTimes, unit list with clusters and drop reasons;
  2. conclusions (templates only);
  3. decision table, least favourable bound first, with the contested columns and the
     D2W of G3-W-shipped;
  4. composite verdict;
  5. feasibility;
  6. preconditions;
  7. witness supply and truth labels;
  8. confusion tables;
  9. attribution;
  10. sensitivity analyses;
  11. cost, budget days and machine safety (pauses, deferrals);
  12. deviation counts per kind, with a pointer to the file;
  13. limits;
  14. an exploratory discussion of at most 300 words, labelled exploratory.

**Conclusion templates:**

- GO: "<endpoint> met its pre-registered band (estimate [lo, hi], n, <k> clusters). Units: one pilot fixture, two model-written fixtures and <t> repository tasks in Python and Go; this does not establish performance on other repositories."
- NO-GO: "<endpoint> fell in its pre-registered NO-GO band (estimate [lo, hi]); <component> must be redesigned."
- INCONCLUSIVE: "The study cannot decide <endpoint> (estimate [lo, hi]); a decision needs about <n_needed> units."
- INCONCLUSIVE (contested): "The study cannot decide <endpoint>: <share> of its units are contested (resolution (i): <estimate>; resolution (ii): <estimate>)."
- ADOPT: "G3-W met the bands of D2W, RW, D1W and D3W, no endpoint fell in its NO-GO band, and the shipped admission's D2W did not fall in its NO-GO band."
- REDESIGN: "G3-W fell in the NO-GO band of <endpoints>; <components> must be redesigned."
- Composite INCONCLUSIVE: "The study cannot decide G3-W; <endpoints> need about <n_needed> units."

`report-lint` fails on:

- the words 'works', 'proves', 'proven', 'validated', 'confirm', 'generalizes' or
  'human audit', matched as whole words in any case, outside the conclusion templates
  and quoted run text;
- a primary figure of D2W, RW, D3W, D4W or D6W without its two resolution figures;
- an RW figure without its pooled figure;
- the text of a `DEVIATIONS.md` entry: the report gives counts per kind and points to
  the file.

**Limits paragraph**, verbatim in the report:

- truth is relative to model-written oracles and domain statements (`gpt-5.6-terra`)
  and to model-written reference violations;
- the repository tasks' criteria are written by a model from the pull request, and
  their test bindings are derived from the truth kill matrix, so a product input
  carries truth-derived bindings;
- one host model writes the witnesses, judges their scope, votes on validity, judges
  alignment and generates the mutants, so the scope judges are correlated tickets;
- the augmented and uncovered sets are evaluated from the facts of runs N, B* and A*,
  not run as separate trees;
- `parkfee` informed the design of G3-W and is in-sample for it;
- at 12 clusters a percentile cluster bootstrap undercovers (ADR 0004 reports a
  simulated coverage of 0.89);
- the gate runs locally, without the CI redraw of witnesses;
- the luna stratum shares the oracle writer's family;
- intervals are unadjusted.

## 14. Run order and freeze

1. **Pre-registration freeze.** The tag `prereg-validity-w` is set on the commit that
   adds this document. `freeze/prereg-commit.txt` holds that commit's hash;
   `freeze/prereg-commit.tsr` is an RFC 3161 timestamp of that file from
   `http://timestamp.digicert.com`, with its query `freeze/prereg-commit.tsq`. Any
   change to this document after the tag is a separate pre-registration with its own
   tag, and the first complete run under this one is still reported.
2. **Tooling.** The tooling of §1 implements this document. Its tests pass on synthetic
   units before the first truth call, including the isolation property tests of §1;
   the parity test of the evaluators against the product's own test projects passes
   before the first product run. A harness behaviour this document does not fix is a
   `protocol` entry naming its scope.
3. **Truth, wave k** (beside the product work): `draw`, `snapshot-parkfee`,
   `author-fixture`, `construct-tasks`, `write-domain`, `write-oracle`, `verify-truth`,
   `bind`, `build-trees`, `seal-truth`. The tag `validity-w-truth-<k>` is set on the
   commit holding the sealed units and `truth.lock.json` of the wave, timestamped in
   `freeze/truth-<k>-commit.{txt,tsq,tsr}`.
4. **Product.** The tag `validity-w-product` is set on the commit that `product-check`
   accepts (P3), timestamped in `freeze/product-commit.{txt,tsq,tsr}`.
5. **Runs, wave k.** `canary`, then per unit: `unseal` phase 1, `run-unit` run N, run
   B*, `run-luna`, `unseal` phase 2, `run-unit` run A*; all within §10's allowance and
   §11's limits.
6. **Truth classification.** `classify-truth` (with `unseal` phase 3), then
   `count-check`; another wave starts at step 3 when §3.2's extension rule requires it.
7. **Analysis.** Once every wave is complete or blocked: `freeze-check`, `evaluate`,
   `analyze`, `report-lint`. The private age key and the unit plaintext are then
   committed with the results.

`freeze-check` refuses unless all of the following hold:

- every tag of steps 1, 3 and 4 exists, and each `freeze/*-commit.txt` equals its
  peeled tag;
- each token's message imprint (`openssl ts -reply -text`) equals the SHA-256 of its
  `*-commit.txt`;
- the genTime of `prereg-validity-w` precedes every other genTime;
- the genTime of `validity-w-truth-<k>` precedes the earliest `started` of any call of
  wave k's product runs in `timing.json`;
- the genTime of `validity-w-product` precedes the earliest `started` of any product
  run, and every commit before it holds the units only as ciphertext;
- `units/truth.lock.json` at each truth tag matches the unsealed files of its wave;
- the manifest of every run names the product commit tagged `validity-w-product`.

The check verifies the imprints and the genTime order, not the timestamp authority's
certificate chain.
