# ADR 0012: Selecting H by execution

Status: accepted. Refines ADR 0004 ("Defect ground truth", part H) and the
bundle schema version of ADR 0011, decision 6.

## Context

ADR 0004 has H written by an isolated session from the spec and its Interface
only, and certifies the whole oracle by two-way verification: the merge passes
U, R, H and O; the base fails every U and H test of each new or revised
criterion; the violations form an exactly diagonal U∪H matrix. A task fails
verification as a whole, so one H test that fails at merge, passes at base or
is flaky drops the task, and it does so after every later construction stage
has spent its model calls.

Two designs were weighed, each by an advocate, with a blind judge who checked
the claims against the code:

- **Repair from feedback.** Run H, then show the writer sanitized failure
  details and let it fix, remove or defend each test. The details are derived
  from the merge (verdict, line, exception type), the limit on what a repair
  may change is a request in a prompt rather than an enforced rule, and shared
  helpers keep one broken guess failing every test that uses it.
- **Selection from blind pools.** The writer stays blind and writes more
  candidates than needed; a deterministic step runs them and keeps those that
  meet verification's own rules. The only information about the merge that
  reaches the sealed H is one keep-or-drop bit per candidate and one refill bit
  per criterion: the filter verification already applies to whole tasks,
  applied to single tests and recorded.

Selection is adopted, with a seal guard and an exposure-sensitivity report.

## Decisions

1. **Blind pools.** After the final spec and before the U-to-O assignment, one
   writer call returns 4 candidates per criterion: at least 1 property test and
   at least 2 example tests. The tests of a criterion live in files of their
   own, with every helper they use; no H file imports another and there is no
   shared test configuration. The writer receives the rendered spec, with its
   Interface, and the layout rules of its language, never an outcome.
2. **Structural checks.** Besides layout and determinism, `h_problems` checks:
   - the pool's composition per criterion asked for (4 candidates, at least
     1 property and 2 example tests), the kind being read from the code
     (`@given` in Python; `quick.Check` or a seeded `rand.New` in the test's
     own body in Go), never from a claim of the writer;
   - that no file mixes criteria: a file's name states its criterion and every
     test in it names the same one;
   - in Go, that no top-level identifier repeats among the H files of a
     package, the H files already kept included;
   - in Python, that imports stay within `sys.stdlib_module_names`, pytest,
     Hypothesis and the top-level packages of the Interface.

   A writing that breaks them is repaired once from its own problems, as every
   construction role is (`FIX_ATTEMPTS = 1`).
3. **Execution.** Every candidate runs `VERIFICATION_RUNS` times at merge and
   at base on the verifier's own tree factory, oracle runner, overlay layout
   and stabiliser, under the same network denial. The merge tree carries the
   human test files, the H files already kept and the writing's candidates.
   - A Python file none of whose tests any merge run reports did not collect.
   - A Go package that reports none of its candidates at merge did not build;
     its files are added one at a time, in path order, and a file whose
     addition leaves every test of the group unreported is rejected. The merge
     then runs again without the rejected files.
   - At base a missing new symbol is the expected failure and nothing is
     bisected. Each Go candidate file has a base tree of its own, so a
     candidate's base outcome never depends on which other files survive;
     Python files share one base tree.
4. **Verdicts.** In order of precedence, a candidate is `rejected-file` when
   its file did not collect or build at merge, `rejected-flaky` when it is
   FLAKY on either tree, `rejected-merge` when it is not PASSED at merge, and
   `rejected-base-pass` when it is PASSED at base and its criterion is new or
   revised. Violation and planted outcomes are never an input: the diagonal
   stays held out.
5. **Selection.** A pure function of the candidates, their outcomes and the
   seed of the run: per criterion, the surviving property tests first, then
   the rest in the order of `sha256(seed:h-selection:task:test)`. At most 3
   are `kept`; the others are `surplus`. A criterion needs at least 2 kept.
6. **Refill.** A criterion with fewer than 2 kept tests gets a fresh writer
   call that receives the spec, the numbers of the short criteria, the pool
   size and the round, nothing else. Its files and Go identifiers carry the
   round prefix `r<k>`. At most 2 rounds; after them a short criterion drops
   the task at `h-tests`, with each class of rejection counted in the reason.
7. **Sealed equals selected.** Python files are pruned to their kept tests by
   `ast` span, decorators included, and a class left with only pruned tests
   goes with them. Go files stay whole and only kept ids enter the manifest. A
   file with no kept test is not sealed.
8. **Record and seal guard.** `construction/h-selection.json` holds the seed,
   the call ids of every writing and its round, and per candidate its
   criterion, kind, round, file, 3+3 outcomes and verdict, then per criterion
   the kept count, the merge rejects (`rejected-merge` or `rejected-file`),
   whether it was refilled and whether it is exposed, and the pruned candidates
   that failed at merge as spec–merge divergences. The unpruned candidates are
   sealed under `construction/h-candidates/`. The bundle schema version is 3,
   and sealing refuses a bundle below it or whose H tests are not exactly the
   tests its record kept.
9. **Verification is unchanged.** Its checks and their criteria stay as ADR
   0004 states them; `verify-task` on the exact sealed overlay is the closing
   run of the selected H set.
10. **Reporting.**
    - **Disclosure**, logged at the start of the run: `protocol | arms: none |
      H is selected deterministically from blind pools by execution at merge
      and base (ADR 0012): the writer sees no outcome; a candidate is kept only
      if it passes at merge, fails at base for a new or revised criterion and
      is stable on both trees; H-inclusive X is also reported without the
      criteria that had a merge reject or a refill.`
    - **Required secondary:** H-inclusive X with and without the exposed
      criteria, those that had a merge reject or a refill. The scored truth of
      a tree carries the full truth without the H tests of those criteria.
11. **Rebuild.** A task dropped under an earlier construction protocol is
    reopened (`bench/script/reopen`): its last attempt is moved aside, never
    deleted, and the next `construct` builds it again. Every model call whose
    request is unchanged replays from the archive. A sealed task is final.

## Consequences

- **Cost.** At most `(1 + 2) × (1 + FIX_ATTEMPTS) = 6` H-writer calls per task;
  no later prompt takes H as input, so a rebuild makes new calls for H only.
  CPU grows by the merge and base runs of each writing, plus Go bisection and
  one base tree per Go file, all inside construction's single machine slot.
- **Earlier drops.** A task whose H cannot meet verification's rules drops at
  `h-tests`, before the assignment, violations and planted refactors spend
  their calls.
- **Exposure is bounded and measured.** Selection conditions H on the merge
  only through the recorded verdicts; at least 2 tests per criterion remain;
  the secondary shows how far the exposed criteria move H-inclusive X. U and R,
  the primary truth, are untouched.
- **Systematic misreads are not repaired.** A criterion whose every blind
  writing misreads the Interface the same way drops the task: the Interface
  does not say enough to test it.
- **Violation-side defects remain.** A violation that fails no test of its
  criterion still drops the task at verification.
- **Go base trees are strict.** A Go test that builds and passes at base in its
  own tree is rejected even when another kept file of its package would stop
  the package from building at base.
- **Older bundles are not sealed.** A plain bundle below version 3 is refused
  by `reverify`, which names the reopen step.
