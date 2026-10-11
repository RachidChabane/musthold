# Discriminating power (`spec-proof-dp`)

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
   `.spec-proof/cache/discriminating-power/<ID>~<REV>/<cacheKey>.json`, and its
   facts record under the state directory. Mutant bodies stay in
   `$XDG_CACHE_HOME/spec-proof/<repo-digest>/mutants/`, never in the proof or the
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

`spec-proof-dp facts show <ID~REV>` prints the facts record of the last Statement
of a requirement: every witness with its runs and scope votes, and every mutant
with its kill vector, witness outcomes, compiled form and votes.

Exit codes of `prove`: 0 when no requirement blocks or escalates, 1 when one does
(or on a refusal; see [commands](commands.md#prove)), 2 on a tool failure, 3 while
tickets wait.

## Ecosystems

| Ecosystem | Detected by | Test run | Requirements on the project |
|---|---|---|---|
| pytest | `pytest.ini`, `conftest.py`, or a pytest section in `tox.ini`, `setup.cfg` or `pyproject.toml` | `python -m pytest`, bound tests selected by node id, bytecode writing disabled. The interpreter comes from the configuration, the virtual environment, `uv run python` for a uv project, then `python3`. | Shallnot's `conftest.py` hook that copies the `verifies` marker into the JUnit XML. |
| Go | `go.mod` | `go tool gotestsum` with a JUnit file, bound tests selected by an anchored `-run` expression. | `tool gotest.tools/gotestsum` in `go.mod`. |
| Jest | `jest.config.*`, a `jest` key in `package.json`, or a jest dependency | `jest --ci --reporters=jest-junit`, tests selected by path and one anchored title regex. | `jest-junit` installed. |
| Vitest | `vitest.config.*`, `vitest.workspace.*` or a vitest dependency | `vitest run --reporter=junit` with one worker. | none beyond Vitest. |
| Maven (Java, Kotlin) | `pom.xml` | Surefire run directly, filtered to the selection, JUnit XML read from the build. | JUnit 5; `./mvnw` or `mvn`. |
| Gradle (Java, Kotlin) | `build.gradle`, `build.gradle.kts`, `settings.gradle*` | a clean `test` task filtered to the selection. | JUnit 5; `./gradlew` or `gradle`. |

Before running tests the module checks that the changed code builds: `tsc
--noEmit -p tsconfig.json` and `node --check` on Node projects, an offline
compile of main and test sources with no daemon left behind on the JVM, an
import of the changed modules on pytest, and a compile of the test binaries on
Go. A mutant that does not build is classified `error` and never counts.

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
| `spec-proof-dp:generator` | host mutants and witnesses |
| `spec-proof-dp:judge` | the scope of witnesses, and the validity of a surviving mutant and its equivalence, with a confidence and a witness |
| `spec-proof-dp:decider` | answers to closed questions, such as the screen of surviving mutants |

## Configuration

`.spec-proof/config.yaml` holds `proofs.discriminating-power`, `ecosystems`,
`operations`, `policy` and `host`. See [configuration](configuration.md).

## Files

| Path | Content |
|---|---|
| `.spec-proof/cache/discriminating-power/` | the module's Statements |
| `$XDG_STATE_HOME/spec-proof/discpower/<repo-digest>/host/` | tickets, responses, `journal.jsonl`, the per-install secret |
| `$XDG_STATE_HOME/spec-proof/discpower/<repo-digest>/records/` | the facts record of each Statement |
| `$XDG_CACHE_HOME/spec-proof/<repo-digest>/mutants/` | mutant bodies |
