# ADR 0013: Witness-based mutant validity (G3-W)

Status: accepted. Supersedes the "judged VALID_c, survivor" and "judged VALID_c,
killed" rows of the claim-power table of ADR 0005, and its NOT_VIOLATING and
EQUIVALENT exclusion rows, with the rows below. Every other part of ADR 0005 stands.

## Context

The discriminating-power module turns a surviving mutant into a block when the
mutant violates its criterion. Under gate G3 that decision was a judgment: host
votes read the mutant's diff, chose an input, and a confirmed F4 witness only
showed that the mutant and the reference differ on that input. Nothing executed
tied the input to the criterion. A judge steered by the diff can call VALID a
mutant whose difference the spec never fixes, and the survivor then blocks an
adequate test set.

The design was decided by a debate between two positions, with a judge who
checked the decisive claims against the pilot's data
(`notes/debates/debate-mutant-validity-verdict.md`). It is measured by the
pre-registration `experiments/validity-w/PREREG.md`.

## Decisions

### Witnesses, written before and blind to every mutant

- **Writers (W).** For each criterion the module emits one `generate.witness`
  request of two votes, so two isolated tickets with distinct nonces, at the
  `restricted` floor of ADR 0005.
  - A writer sees the spec documents, with every criterion relabelled `R1`,
    `R2`, … by position so that the precedence the spec states stays readable
    and no criterion is named by its identifier; the statement of the criterion
    to check; the public signatures of the code the spec mentions; and the
    signatures of the helpers of every test file.
  - A writer never sees a body of the code, a test, a mutant, a diff or a kill.
  - A writer returns 3 to 6 examples and 1 to 3 derandomized properties drawn at
    the full resolution of their types, each a complete test function of the
    project's test framework, tagged with the clause it checks. A set outside
    those bounds is rejected and its ticket reissued.
  - The request's seed comes from the criterion's statement and the draw, never
    from the run seed, so CI asks the requests the developer's host answered and
    the pushed answers serve them.
- **Credit draw in CI.** A CI run also asks, of every criterion, a credit draw of
  two fresh writers seeded by the CI run seed. In CI only its witnesses earn pass
  credit; the witnesses the developer's host wrote still block and escalate. The
  footprint, the stop rule and the further draw never read the credit draw, so
  the search asks the same requests in CI as locally; every pool runs against
  its witnesses, and no Statement is written before it is settled.
- **Journal.** The digest of each set a writer returns is journaled as
  `witness_set_committed` with the criterion and the draw, once, before the set
  runs on any mutant and before the criterion's first generation request.
- **Further draw.** A criterion whose search ends with fewer than two VALID_e
  candidates asks one further writer, equally blind. A VALID_e candidate is a
  targeted mutant of the pool that fails, reproduced, a block-bearing witness of
  the criterion and is neither owner-attributed nor identical to the reference
  in compiled form: VALID_e as far as it can be told without a kill vector or a
  binding. Isolation and the operator survivors that enter the pool read the
  tests, so the further draw reads neither.
- **Request invariance.** No request of the writers, the scope judges or the
  generation reads a test, a binding or a kill vector. Two test trees whose
  helpers are equal give the same `generate.witness`, `judge.scope` and
  `generate.targeted` requests, so the product's store serves the second tree.
  - The helper signatures come from every test file of the project, not from the
    files of the bound tests.
  - The footprint of a criterion, which bounds the operator mutants and the code a
    generation request shows, is the code its runnable witnesses execute on the
    reference; every unit of the code when they execute none or the ecosystem
    records no coverage.
  - The search stops on test-independent facts: the targeted and owner-attributed
    mutants of the pool that fail a block-bearing witness or an example of the
    criterion, `min_valid` of them over `min_sites` sites. The further draw counts
    the VALID_e candidates, equally test-independent.

### Admission and scope

- **Reference.** Each set runs alone on the reference three times. A witness that
  passes every run passes the reference; one that fails every run is a reading
  conflict; a mix is flaky; an error is unrunnable.
- **Reading conflicts** are questions about the spec. The module reports each as a
  `reading_conflict` finding naming the criterion, the witness and its clause,
  and counts them in the Statement. They never block and do not change the
  criterion's outcome: the gate reads the outcome from the Statement's mutants,
  and a reading conflict concerns no mutant.
- **Scope judges (C).** One `judge.scope` request of `judge_k` votes per draw
  shows the spec and every witness the reference passes, and asks, per witness,
  whether the criterion's text alone, under the precedence, fixes the output the
  witness asserts. A witness that every vote answers FIXED at or above `tau_judge`
  is held in scope. The scope judges see no mutant; they gate a block and never
  create a pass, so their isolation does not limit that power.
- **Planted refactors.** A project may name, in `admission.refactors`, a directory
  of edit sets that preserve every criterion. The sets that hold a passing witness
  run on each, after the scope verdicts, and the result is recorded per witness.
  `init` and `enable` never write the setting; it serves the benchmark.
- **Status.** A witness that passes the reference, and every planted refactor
  when some are named, is admitted. An admitted witness held in scope is
  block-bearing; any other admitted witness is escalate-only. A witness is
  credit-eligible when its writer and every scope judge answered at or above the
  isolation floor, unmarked.

### Classes of a mutant for a criterion c

| Class | Rule |
|---|---|
| sentinel | drawn from the sentinel catalogue: EQUIVALENT by construction |
| excluded, E0 | compiles to the reference's form on every edited site |
| owner-attributed | fails an example of c and no example of another criterion |
| VALID_e,c | fails, reproduced, a block-bearing witness of c, and no test bound to another criterion killed it or errored on it |
| collateral | VALID_e for c and for another criterion |
| excluded, other criterion | fails an example or an admitted witness of another criterion and no example and no admitted witness of c |
| excluded, F3 | the full-resolution, literal-seeded differential search found no difference, at least `judge_k` validity votes answer EQUIVALENT at or above `tau_judge` from responses at the floor, no vote confirms a witness, no test killed it, it fails no witness the reference passes and the basis is not `executed-only` |
| unsettled | everything else |

A credit-bearing mutant is owner-attributed, or VALID_e,c, not collateral,
failing a credit-eligible witness, under a basis other than `executed-only`.

### Gate G3-W, for a criterion c and its test set S

In order, BLOCK before ESCALATE before PASS:

1. Traceability does not pass: BLOCK (Shallnot).
2. The reference fails a test of S or an example of c: BLOCK.
3. An owner-attributed, VALID_e,c or collateral mutant survives every test of S:
   BLOCK.
4. No credit-bearing mutant of the pool is killed by S, an empty pool included:
   ESCALATE.
5. Any other survivor that execution did not exclude: ESCALATE, shown with its
   validity votes and their witness.
6. Alignment: unchanged (Shallnot).
7. Otherwise PASS. Pass credit is a credit-bearing mutant killed by a test of S.

A test of S that killed a sentinel, an exhausted ticket budget and an unresponsive
host also escalate, and a kill earns credit only from a test that killed no
sentinel: the gate that reads the Statement applies the same rules.

No judgment alone settles a survivor. The validity votes (V-H) are asked only on
survivors no witness settles; they never block, and their EQUIVALENT only
corroborates an F3 exclusion. The `judged` pass basis therefore excludes nothing
more than `corroborated`.

### Claim power

| Claim | Effect | Executed corroboration required | Otherwise |
|---|---|---|---|
| VALID_e,c, survivor | BLOCK | a reproduced failure of a block-bearing witness of c, the reference passing that witness 3/3, a build, every other criterion's tests passing | ESCALATE |
| VALID_e,c, killed | pass credit | the same, a specific kill, a writer and scope judges at the floor, not collateral | no credit |
| EQUIVALENT, survivor | exclusion | compiled-form identity (E0); or full-resolution F3 `NO_DIFF_FOUND` with every validity vote EQUIVALENT, no confirmed F4 witness and no failed witness the reference passes | ESCALATE |
| NOT_VIOLATING / OFF_TARGET, survivor | exclusion | fails an example or an admitted witness of another criterion and none of c's | ESCALATE |
| judged VALID or NOT_VIOLATING with no witness failure | none | — | ESCALATE |

### Encoding in the Statement

The Statement keeps the vocabulary the gate reads, so that the gate's own
claim-power table gives G3-W's outcome:

- VALID_e and collateral are VALID claims on the basis `execution`, with full
  confidence and the corroboration `builds`, `other_criteria_pass` and
  `f4_witness`, plus `specific_kill` when a credit-bearing mutant is killed. The
  claim's executor is the weakest of the writer and the scope judges of the
  witness it rests on, so the gate withholds credit below the floor.
- An exclusion by another criterion is NOT_VIOLATING, off target, corroborated by
  `other_example_failed`; an E0 or F3 exclusion is EQUIVALENT corroborated by
  `no_diff_found`.
- Every unsettled mutant is UNDETERMINED with the reason its votes give: VALID or
  NOT_VIOLATING from the votes alone is never written as such.
- The alignment matrix reads a non-collateral VALID claim on the basis
  `execution` into its criterion's pool, beside the owner-attributed ones, so a
  test's kills of VALID_e mutants corroborate its alignment.
- `extensions.witnesses` counts the criterion's witnesses by status and names the
  universe of witnesses the pool ran against; each mutant's extensions carry its
  class, its exclusion route, the block-bearing witnesses it fails and the other
  criteria it is VALID_e for.

### Execution

- **Witness runs.** A set is laid out as one test file of the project by the
  ecosystem's witness host, each witness renamed after its short id. Witness
  files are written only into the workspaces of the program they run on: the
  reference, a planted refactor or a mutant. The sets that hold a passing witness
  run whole, so that their files build as they did on the reference; only the
  admitted witnesses are read. The missing sets of a program run in one batch,
  each set alone when the whole batch errors; a set one of whose witnesses failed
  is rerun, and a failure the rerun does not repeat is flaky. Results are
  memoized per program and set.
- **Universe.** No criterion goes past its witnesses while the writers or scope
  judges of a draw any search reads owe an answer, and no criterion is proved
  before every search ended. The validity votes of the survivors no witness
  settles are asked as the generation rounds go, never holding back the next
  round. Every pool mutant then runs against the witnesses of every
  criterion. An operator survivor of c's tests that fails a block-bearing witness
  of c enters the pool whatever the differential search and the screen say.
- **F3 at full resolution, seeded with literals.** Every number of the code, the
  examples and the spec's text enters the draws with its neighbours one unit, one
  written step, one step a decimal finer and half a written step away; a datetime
  parameter draws the anchor day, every time of day and duration the spec names
  and every example datetime, each with neighbours a microsecond, a second and a
  minute away.
- **E0.** Python compares code objects with their line tables, first lines and
  file names left out; Go compares the assembly the compiler gives a function and
  its closures with every source position left out. An ecosystem with no form
  compiler leaves identity unknown.
- **E3.** The mutants that block or escalate are grouped into fault classes by the
  sites they edit and the witnesses they fail, or the inputs they differ on when
  they fail none, and listed in `extensions.faults`.
- **Facts record.** Beside each Statement the module keeps, under its state
  directory, the record of every executed fact and answer the outcome rests on:
  policy, admission, reference results, bindings, witnesses with their runs and
  scope votes, and every pool mutant with its kill vector, examples, differential
  search, compiled form, witness outcomes and votes. The outcome of any test set
  and binding recomputed from the record through the same rules equals the one the
  module computes. `spec-proof-dp facts show ID~REV` prints it.

### Ecosystems

Each ecosystem implements a witness host: the brief a writer is given and the
layout of a set as a test file named the way its JUnit report names it. An
ecosystem without one asks no witness, reports `witness_runner_unavailable`,
takes the footprint from the criterion's tests, and reaches PASS through its
examples alone.

## Consequences

- A survivor blocks only by failing, on execution, a test written before and
  blind to it, whose scope two judges who never saw the mutant approved. A diff
  can no longer steer the reading of the criterion.
- Criteria without examples earn credit only from witnesses: an ecosystem without
  a witness host, or a host below the floor, escalates them.
- Execution grows: every pool mutant and every operator survivor runs the
  witnesses of the criterion, and the pool runs every criterion's witnesses once.
  The memo keeps a second pass free.
- The F3 and F4 stages need bound operations. Without them, as in a project that
  binds none, survivors no witness settles escalate.
- The scope judges are same-host tickets and correlate; they gate a block and
  never create one.
- Witness echo by a coder who copies a witness it was shown is closed in CI by the
  credit draw, not locally.
