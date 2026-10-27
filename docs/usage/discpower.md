# Discriminating power (`musthold-dp`)

Discriminating power answers one question per requirement: would the tests bound
to this requirement fail on code that violates it? The module answers it by
execution. It builds mutants, runs the bound tests against them and records which
tests kill which mutants. Its proof blocks the gate.

The module works alone with Shallnot. Commands that every proof module shares
(`init`, `enable`, `prove`, `request`, `doctor`, `host`, ...) are in
[commands](commands.md).

## What it does

For each requirement in scope, the module:

1. reads the requirement, its statement digest, its inline examples, its bound
   tests and the spec file that declares it from Shallnot's report
   (`shallnot gate --advisory --json-out`); it parses no spec itself;
2. runs the bound tests and the examples on the unmodified code. A test or an
   example that fails there blocks the requirement;
3. asks two blind `generator` executors for **witnesses**: three to six examples
   and one to three derandomized properties, each a test of the project's own
   framework tagged with the clause of the requirement it checks. A writer sees
   the spec documents, with every requirement relabelled by position, the
   requirement to check, the public signatures of the code the spec mentions and
   the signatures of the test helpers; never a body of the code, a test or a
   mutant. The digest of each witness set is journaled before any mutant exists.
   In CI, two more writers seeded by the CI run write the only witnesses that
   earn pass credit there;
4. runs each witness on the unmodified code three times. A witness that fails
   every run is a reading conflict, reported as `reading_conflict`; one that
   passes every run goes to two blind `judge` executors, who see the spec and the
   witnesses and say, per witness, whether the requirement's text alone fixes what
   it asserts. A witness both hold fixed is block-bearing;
5. computes the requirement's footprint: the functions its witnesses reach on the
   unmodified code, plus their static closure and the module-level assignments
   they read;
6. makes mutants of the footprint from three sources:
   - **operator mutants**: relational, arithmetic, numeric constant, boolean and
     logical, return value, module constant, constructor argument, decimal string,
     min and max, and/or, boolean literal and exception class, capped at `k_op` and
     drawn by a seed;
   - **sentinels**: two per footprint file that change no behaviour (dead
     assignment, alpha-renaming, unused private constant). A test that kills a
     sentinel is sensitive to the text of the code, not its behaviour;
   - **host mutants**: small realistic faults that a blind `generator` executor
     writes from the requirement and the code alone, never the tests or the
     witnesses;
7. runs the bound tests and every requirement's witnesses on every mutant, in a
   clone of an already built reference workspace, and repeats every kill and
   every witness failure once;
8. classifies each mutant by execution. A mutant that fails, reproduced, a
   block-bearing witness of its requirement and is killed by no test of another
   requirement is **VALID_e**; one that fails an example of its requirement alone
   is owner-attributed. A mutant that compiles to the reference's form, or that
   fails an example or a witness of another requirement only, is excluded. Two
   blind `judge` executors vote on the survivors no witness settles; their
   EQUIVALENT excludes a survivor only when the full-resolution differential
   search found no difference and the survivor fails no witness the unmodified
   code passes;
9. writes one proof Statement per requirement under
   `.musthold/cache/discriminating-power/<ID>~<REV>/<cacheKey>.json`, and its
   facts record under the state directory. Mutant bodies stay in
   `$XDG_CACHE_HOME/musthold/<repo-digest>/mutants/`, never in the proof or the
   work tree.

## Outcome

| Result | When |
|---|---|
| `BLOCK` | the tests fail on the reference code; an example of the requirement fails on it; an owner-attributed or VALID_e mutant survives every bound test |
| `ESCALATE` | a surviving mutant is unsettled; no credit-bearing mutant was killed; a test killed a sentinel; the host budget is exhausted; the host stopped answering |
| `PASS` | at least one credit-bearing mutant was killed and nothing else remains |

A block comes before an escalation. Every block rests on an executed failure: a
survivor blocks only by failing an example or a block-bearing witness, never on a
judgment. A credit-bearing mutant is owner-attributed, or VALID_e from a witness
whose writer and scope judges answered at the isolation floor and witnessed for no
other requirement. Details and the effect of `signed_pass_basis` are in
[trust model](trust-model.md#claim-power). The Statement groups the mutants that
block or escalate by fault in `extensions.faults`.

The search stops once the pool holds `min_valid` witnessed host mutants over
`min_sites` distinct sites; a requirement left with fewer than two host mutants
that fail a block-bearing witness without being owner-attributed gets one more
witness writer.

`prove` prints `<ID~REV> <status> <result>`, where status is `proved`, `fresh`
(the Statement of the current inputs already exists) or `awaiting_host`, followed
by `  finding <name>` lines. Findings: `correct_code_fails`, `example_failed`,
`surviving_valid_mutant`, `no_valid_mutant`, `no_killed_valid_mutant`,
`mutation_sensitive_test`, `budget_exhausted`, `host_unresponsive`,
`unexcluded_survivor`, `example_executor_unavailable`. Module findings such as
`reading_conflict` and `witness_runner_unavailable` are printed after the
requirements.

Under a blocked requirement, a local run prints one `  survivor` line per mutant
that blocks it: the mutant's reference, how it was drawn (a targeted mutant of a
generation slot, or an operator mutant of a family) and the input every test passes
while the mutant fails it. That input is the requirement's example, written as the
spec writes it, or a block-bearing witness, shown with its name, its clause and its
code, cut at 48 KiB:

```
BOX-2~1 proved BLOCK
  finding surviving_valid_mutant
  survivor 10edb8fdacce: an operator mutant of family relational, which every test of the criterion passes, fails its example 1: ticket_price(age=65) => 8
```

A survivor line never shows the mutant's code: a test written to that input does
real testing work, and one fitted to the code would not. The Statement records the
same in its mutant entry (`extensions.counterexample`), and Shallnot's
`surviving_valid_mutant` finding repeats it under `proofs.trust: local`. A
Statement produced by CI records none.

`musthold-dp facts show <ID~REV>` prints the facts record of the last Statement
of a requirement: every witness with its runs and scope votes, and every mutant
with its kill vector, witness outcomes, compiled form and votes.

Exit codes of `prove`: 0 when no requirement blocks or escalates, 1 when one does
(or on a refusal; see [commands](commands.md#prove)), 2 on a tool failure, 3 while
tickets wait.

## Ecosystems

| Ecosystem | Detected by | Test run | Requirements on the project |
|---|---|---|---|
| pytest | `pytest.toml`, `.pytest.toml`, `pytest.ini`, `.pytest.ini` or `conftest.py` at the root, or a pytest section in `tox.ini`, `setup.cfg` or `pyproject.toml`; without them, any `test_*.py`, `*_test.py` or `conftest.py` that pytest collects from the root with its default settings | `python -m pytest`, bound tests selected by node id, bytecode writing disabled. The interpreter comes from the configuration, the virtual environment, `uv run python` for a uv project, then `python3`. | Shallnot's `conftest.py` hook that copies the `verifies` marker into the JUnit XML. |
| Go | `go.mod` | `go tool gotestsum` with a JUnit file, bound tests selected by an anchored `-run` expression. | `tool gotest.tools/gotestsum` in `go.mod`. |
| Jest | `jest.config.*`, a `jest` key in `package.json`, or a jest dependency | `jest --ci --reporters=jest-junit`, tests selected by path and one anchored title regex. | `jest-junit` installed. |
| Vitest | `vitest.config.*`, `vitest.workspace.*` or a vitest dependency | `vitest run --reporter=junit` with one worker. | none beyond Vitest. |
| Maven (Java, Kotlin) | `pom.xml` | Surefire run directly, filtered to the selection, JUnit XML read from the build. | JUnit 5; `./mvnw` or `mvn`. |
| Gradle (Java, Kotlin) | `build.gradle`, `build.gradle.kts`, `settings.gradle*` | a clean `test` task filtered to the selection. | JUnit 5; `./gradlew` or `gradle`. |

Each runner is detected by what it accepts with its defaults: Go by the
`go.mod` that `go test ./...` needs, Jest and Vitest by their configuration or
a dependency the package runner resolves, Maven and Gradle by their build
files, pytest also by the test files it collects without a configuration. A
tree a configuration or a manifest names a runner for is scanned with it before
a runner detected only by such test files.

Before running tests the module checks that the changed code builds: `tsc
--noEmit -p <project>` and `node --check` on Node projects, an offline
compile of main and test sources with no daemon left behind on the JVM, an
import of the changed modules on pytest, and a compile of the test binaries on
Go. A mutant that does not build is classified `error` and never counts.
The TypeScript project of a changed file is the nearest `tsconfig.json` above
it; a `tsconfig.json` that lists no file of its own (`"files": []` with
`references`, as at the root of a monorepo or of a Vite package) stands for the
projects it references, each checked in turn. The check judges a mutant by what
it adds: a project whose `tsc` already reports diagnostics on the unmodified
code, as a deprecated option in `tsconfig.json`, a root `tsconfig.json` that
cannot check its packages alone, or a type error that Vitest never checks does,
has them recorded once per project before any mutant runs, and a mutant fails
the check only on a diagnostic its project does not report on the reference,
compared by file, code and message wherever it sits in the file.

Operator mutants and sentinels exist for Python, Go, JavaScript and TypeScript, Java
and Kotlin sources. The module probes code by calling the targets of your
`operations` in the workspace (see [configuration](configuration.md#operations)).
A project that binds no operation, or an ecosystem that cannot probe calls, gets no
example run and no differential search: the module reports the skipped searches as
findings, still asks for witnesses and host mutants, and escalates every
requirement that has no credit-bearing mutant killed.

Witnesses run in every ecosystem above: a witness is a test of the project's own
framework, laid out beside its tests (a pytest module, an internal Go test file, a
Jest or Vitest test file, a JUnit 5 class in the package of the code under test).
An ecosystem with no witness host asks for none, reports
`witness_runner_unavailable`, takes the footprint from the bound tests, and a
requirement earns credit through its examples alone.

## Roles

The plugin ships three executor agents. Each holds exactly the two executor tools
and sees only its ticket.

| Role | Writes |
|---|---|
| `musthold-dp:generator` | host mutants and witnesses |
| `musthold-dp:judge` | the scope of witnesses, and the validity of a surviving mutant and its equivalence, with a confidence and a witness |
| `musthold-dp:decider` | answers to closed questions, such as the screen of surviving mutants |

## Scan

`musthold-dp scan` asks of any pytest, Go, Jest or Vitest repository, with no
spec, no model and no configuration, which tests still pass when the code they
run is broken (ADR 0014). `musthold scan` runs it through the [umbrella](umbrella.md).
A tree that declares several of these test tools is scanned on the first, in the
order pytest, Go, Jest, Vitest, whose suite collects a test; a pytest suite
detected only by its test files comes after the others, and at the root of a Go
workspace it gives way to the workspace's refusal. A runner that fails on the
unmodified code before it reports a test, as a configuration that does not load
or a missing interpreter makes it, gives way to the next configured suite but
to no suite detected by its test files alone; when no suite collects a test,
the scan fails with the first such runner's command, how it ended and the end
of its output. A runner killed from outside, as the out-of-memory killer ends a
process, fails the scan and names `--workers` and the memory. On Linux every
test process a scan starts is the out-of-memory killer's first choice, so that
the scan outlives it and says so.

```
musthold-dp scan [--full] [--host [--resume]] [--base REV] [--seed N] [--sample N] [--mutants N] [--budget DURATION] [--workers N] [--coverage-timeout DURATION] [--format text|md|json] [--root DIR] [<PR>]
```

The work tree must have no uncommitted change to a tracked file: a scan names
the commit it ran on. A Go workspace is scanned one module at a time: at the
root of a `go.work`, the refusal names its modules, and `--root <module>` scans
one of them. It copies the tree into a workspace outside it, runs the
whole suite three times and records, per test, the lines its run executed and,
among them, the lines the test owns, which exclude what it shares with other
tests. For Go, a test owns the blocks it ran more often than its parent test
run alone with no subtest, or than its binary's start-up for a top-level test,
each run three times; a block whose count differs between those runs is no
test's own, and a test whose parent fails with no subtest owns nothing. A line
is owned when every block holding code on it that the test ran is owned, so a
`case` condition, which no block holds, is never owned. For pytest, a test owns
the lines its own thread ran in its setup and call, outside the setup of a
fixture wider than the function and outside the body of a module it imported;
a teardown is never owned. For Jest and Vitest, a test whose owned lines the
scan reads runs alone in three processes, each recording V8 coverage mapped to
the sources through the source maps the runner's transform writes. Its window
opens before its first `beforeEach` hook and closes when its test function
settles; a callback scheduled by setup files, module evaluation, describe
bodies or another hook, and a fake timer set before the window, run in the
window but are none of its own. A line is owned when every code unit of it the
test ran counted in that window, with the same counts in the three processes,
so a line `afterEach` also runs is never owned; a window in which a module is
evaluated owns nothing. A process that cannot check one of these conditions
owns less, never more: without an inspector session the test gets the whole
of each file it ran and owns nothing.
Without `--base` it samples green tests, stratified by file; with `--base` it
takes the green tests that run a function changed since that revision. It draws
operator mutants on the lines the judged tests own, round-robin over those
tests so that each gets the mutants of its own lines its verdict needs, plus up
to 12 sentinels, one file at a time.

| | Quick (default) | `--full` |
|---|---|---|
| Tests sampled | 12 | 40 |
| Operator mutants | 72, six per test | 150 |
| Coverage recorded | the sampled tests (every green test for pytest, which records them in one run) | every green test; Jest and Vitest record the tests not sampled in one run of their files, each with the lines of its file's segment |
| Tests run on a mutant | the tests with recorded coverage that ran its lines | every green test that ran its lines |
| Budget | 90 s, then running mutants stop and count as not run | none |

`--sample`, `--mutants` and `--budget` replace what either depth sets. Mutants
run on as many workers as the machine allows: one per two processors and per
GiB of memory above the 40 % the memory guard keeps free, at most four, the
memory being the limit of the scan's control group when one bounds it, as in a
container; `--workers` sets another number. The sentinels start first, then the mutants in
the order they were drawn, except that a mutant only settled tests run waits
behind every mutant an open test runs. Once every sentinel has finished, a test
that killed a mutant on its lines keeps its class whatever else runs, as does a
test that failed a sentinel; every other judged test is open. A budget that cuts
the scan therefore spends its time on the open verdicts first, and the mutant
count names how many mutants it left unrun. It never mutates a test, vendored,
generated or migration file, a file of a `__tests__`, `__mocks__` or
`__fixtures__` directory, nor a line that logs, prints, calls `console` or
records a metric, and runs each mutant against every test that ran one of its
lines, three times.

A mutant counts only when executed evidence proves it changes behaviour: another
green test kills it three runs out of three, or a differential run of the
function it edits, called on inputs drawn from the literals of the code and the
tests, gives another result than the reference, reproduced, on an input where the
reference agrees with itself. A test **verifies nothing** when it passes all of at
least 5 proven changes, on at least 2 lines, in the lines it ran. A pass counts
only on a line the test owns: a line its process ran only at start-up records a
kill or an inconclusive outcome, never a survival.

Before a test is named, the scan confirms its claim alone, since a mutant runs
every test that ran its lines in one selection. The test keeps only the changes
on lines it also owns when it runs alone, recorded again in a session of its
own for pytest and read from the recording alone for Go, Jest and Vitest; a
change only kills prove stays only when one of its killers,
run alone, passes the reference and fails the mutant three times; and the test
runs alone three times on each change left, a change it does not pass three
times making it unproven. The number it prints is the confirmed one.

A mutant can stop a test's process before any test starts: a Go test binary
that panics in a package variable or `init`, a pytest module, `conftest.py`
or `__init__.py` that raises on import, a Jest or Vitest test file whose import,
describe body or setup file throws, or a global setup that throws. When every run of a test on a mutant
stopped so, the change counts neither for nor against the test: the statement
lists it apart from the test's changes, under `notStarted`. The other tests of
the run still run: a pytest session names the test files, keeps the selected
tests alone and never stops at a failure, whatever the project's options say,
and runs again without the tests below a `conftest.py` that stopped it. A Go
process that exits early with tests left unrun counts as one run, and the
package runs again in fresh processes until each test has three. A Go subtest
that passes while its parent fails on its own did not pass; when a failing
sibling may have failed the parent, the subtest runs again without it. Jest
runs in band and Vitest in one fork, each run one process with coverage,
bail and retries off, and each recording of a test alone is read as its process
ends and its record removed; a Jest or Vitest test is named `file::titles`, its
describe titles and its own joined by ` > `, and a hook failure, a retry, or a
name another test shares, for Jest ignoring case, gives an error. A Vitest
project that runs its tests in a browser is refused. The
headline gives each class over its denominator and never reads as a pass:

```
acme/invoicing @ 3f2a9c1 · pytest · 412 tests (409 green 3/3) · 40 sampled · seed 7
  full scan · no budget · 4m12s · 40 of 409 green tests sampled · 4 workers
  7/40 verify nothing   21/40 caught ≥ 1 change   12/40 unproven   369/409 not sampled
  unproven: 9 with < 5 proven changes or < 2 sites, 3 with an inconclusive outcome
  they lack: 2 ran no drawn mutant · 31 unproven mutants on their lines: 18 searched without a difference, 9 with no function to call, 4 identical when compiled · no witness without a host
  Not a pass: unproven and unsampled tests are unjudged.
  tests/test_rates.py::test_rate_table: 2 proven changes in lines it ran stopped it from starting; they count neither way

#1 tests/test_parse.py::test_parse_ok passes on all 9 proven changes in lines it ran; 1 more stopped it from starting
   src/invoicing/parse.py:41  >= → >   input line="a,": reference "a", mutant ""
   proven by: a differential run of invoicing.parse:split_fields, reproduced, on a reference that agrees with itself
   repro: .musthold/scan/3f2a9c1/1/repro.sh
150 mutants (150 ran: 61 proven) · 4m12s (reference 41s, coverage 1m2s, draw 1s, mutants 2m16s, confirmation 12s) · no model used · promises not checked
7/40 sampled tests stay green on every proven change in lines they ran: on those lines, their green is no evidence the code is right.
```

"They lack" says why the unproven tests are unproven: how many ran no drawn
mutant, and, for the mutants on their lines that no route proves, whether the
differential search found no difference, had no function to call or could not
call it, whether the only killing test fails a sentinel, or whether the mutant
compiled to the reference's form, did not build or start, or did not run within
the budget. The Markdown report lists each unproven test on its own row. The
mutant count is the number of mutants drawn, the sum of those that ran, those
that did not build or start and those the budget left unrun; of the ones that ran
it names the proven ones, those identical when compiled and the sentinels that
stopped a test binary: a sentinel keeps behaviour, so such a stop is a fault of
the harness, never of a test.

When a sampled test verifies nothing, the last line says what its green
leaves unshown, over the sampled tests; a scan in which none does ends on the
mutant count.

The scan exits 0 whatever it finds. `--budget` stops starting mutants after that
long and marks the counts partial; the claims are confirmed once the mutants
stop, whatever the budget. Each run of tests under coverage is bounded
by ten times one reference run of the suite, between 2 and 30 minutes;
`--coverage-timeout` sets another bound, and a run past its bound fails the scan
with an error that names it. `--format md` prints the report as a
pull-request comment and `--format json` the scan statement, which no reader of
proofs admits.

| Path | Content |
|---|---|
| `.musthold/scan/<commit>/report.md` | the Markdown report |
| `.musthold/scan/<commit>/scan.json` | the scan statement |
| `.musthold/scan/<commit>/<n>/mutant.patch` | the change the test passes on |
| `.musthold/scan/<commit>/<n>/repro.sh` | extracts the commit into a temporary directory, applies the patch and runs the one test |
| `.musthold/scan/<commit>/<n>/finding.json` | the record a replay re-executes |

`musthold scan replay <dir>` re-executes, with no model, the finding of a
finding folder or every finding of a scan folder, and exits 1 when a claim no
longer holds.

### Scan with a host

`--host` asks the host, through the exit-3 loop of the `/musthold:scan` skill
([umbrella](umbrella.md#the-scan-skill)), for the promises of the code. A scan
without it calls no model. `<PR>` scans a pull request: its head must be the
checked-out commit, and the scan takes the merge base with its base commit.
`--base` and `<PR>` exclude each other.

The quotable sources are the README at head and, for a pull request, its title
and body and each issue it closes. Their bytes are saved under
`.musthold/scan/<commit>/sources/`. A change's code is never a source. A scan
with no quotable source prints "no quotable promise in the README, description or
closing issues" beside its headline.

Each invocation runs the stages (sources, writers, admission, final) as far as
the answers go:

| Exit code | Meaning |
|---|---|
| 0 | the scan finished and wrote its reports |
| 2 | a refusal, such as a `--resume` whose commit, tree, mode, base, pull request, seed, counts, budget, module version or sources differ from the open session's, each difference named |
| 3 | tickets wait: the scan prints `wave N`, `session scan:<key>` and `awaiting_host N tickets` |

The tickets of the session are listed with
`musthold-dp request list --session scan:<key>`. `--resume` continues the open
session of the same scan and implies `--host`. A scan asks at most 18 calls: 4
for each of at most 3 promises, 1 for the promises and 1 for the labels; a promise
the budget cannot cover is reported unchecked. A finding folder also holds the
test that kills the mutant by its route, which `repro.sh` runs.

## Configuration

`.musthold/config.yaml` holds `proofs.discriminating-power`, `ecosystems`,
`operations`, `policy` and `host`. See [configuration](configuration.md).

## Files

| Path | Content |
|---|---|
| `.musthold/cache/discriminating-power/` | the module's Statements |
| `$XDG_STATE_HOME/musthold/discpower/<repo-digest>/host/` | tickets, responses, `journal.jsonl`, the per-install secret |
| `$XDG_STATE_HOME/musthold/discpower/<repo-digest>/records/` | the facts record of each Statement |
| `$XDG_CACHE_HOME/musthold/<repo-digest>/mutants/` | mutant bodies |
