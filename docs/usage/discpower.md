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

1. reads the requirement, its statement digest, its inline examples and its bound
   tests from Shallnot's report (`shallnot gate --advisory --json-out`); it parses
   no spec itself;
2. runs the bound tests and the examples on the unmodified code. A test or an
   example that fails there blocks the requirement;
3. computes the requirement's footprint: the functions its tests and examples
   reach, plus their static closure, and the module-level assignments they read;
4. makes mutants of the footprint from three sources:
   - **operator mutants**: relational, arithmetic, numeric constant, boolean and
     logical, return value, module constant, constructor argument, decimal string,
     min and max, and/or, boolean literal and exception class, capped at `k_op` and
     drawn by a seed;
   - **sentinels**: two per footprint file that change no behaviour (dead
     assignment, alpha-renaming, unused private constant). A test that kills a
     sentinel is sensitive to the text of the code, not its behaviour;
   - **host mutants**: small realistic faults that a blind `generator` executor
     writes from the requirement and the code alone, never the tests;
5. validates each mutant by execution. A mutant counts only if it compiles, passes
   the tests of the other requirements, and violates its own: it fails an example
   of its requirement, or a differential search finds an input where the original
   and the mutant behave differently and a blind `judge` executor confirms the
   witness contradicts the requirement;
6. runs the bound tests on every mutant, in a clone of an already built reference
   workspace, and repeats every kill once;
7. writes one proof Statement per requirement under
   `.spec-proof/cache/discriminating-power/<ID>~<REV>/<cacheKey>.json`. Mutant
   bodies stay in `$XDG_CACHE_HOME/spec-proof/<repo-digest>/mutants/`, never in the
   proof or the work tree.

## Outcome

| Result | When |
|---|---|
| `BLOCK` | the tests fail on the reference code; an example of the requirement fails on it; a valid mutant survives every bound test |
| `ESCALATE` | a surviving mutant is undetermined; no valid mutant was found or none was killed; a test killed a sentinel; the host budget is exhausted; the host stopped answering |
| `PASS` | at least one valid mutant was killed and credited, and nothing else remains |

A block comes before an escalation. A judged signal never blocks alone: a judged
`VALID` survivor blocks only with a confirmed witness, a mutant that builds and
passes the other requirements' tests, and agreeing judges. Otherwise it
escalates. Details and the effect of `signed_pass_basis` are in
[trust model](trust-model.md#claim-power).

The search stops on a block, and otherwise once `min_valid` valid mutants cover
`min_sites` distinct sites with at least one killed.

`prove` prints `<ID~REV> <status> <result>`, where status is `proved`, `fresh`
(the Statement of the current inputs already exists) or `awaiting_host`, followed
by `  finding <name>` lines. Findings: `correct_code_fails`, `example_failed`,
`surviving_valid_mutant`, `undetermined_survivor`, `no_valid_mutant`,
`no_killed_valid_mutant`, `mutation_sensitive_test`, `budget_exhausted`,
`host_unresponsive`, `unexcluded_survivor`, `example_executor_unavailable`.

Exit codes of `prove`: 0 when no requirement blocks or escalates, 1 when one does,
2 on a tool failure, 3 while tickets wait.

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
example run and no differential search: the module reports the skipped searches
as findings and escalates every requirement that has no valid mutant.

## Roles

The plugin ships three executor agents. Each holds exactly the two executor tools
and sees only its ticket.

| Role | Writes |
|---|---|
| `spec-proof-dp:generator` | host mutants |
| `spec-proof-dp:judge` | validity of a mutant and equivalence of a surviving one, with a confidence and a witness |
| `spec-proof-dp:decider` | answers to closed questions, such as the screen of surviving mutants |

## Configuration

`.spec-proof/config.yaml` holds `proofs.discriminating-power`, `ecosystems`,
`operations`, `policy` and `host`. See [configuration](configuration.md).

## Files

| Path | Content |
|---|---|
| `.spec-proof/cache/discriminating-power/` | the module's Statements |
| `$XDG_STATE_HOME/spec-proof/discpower/<repo-digest>/host/` | tickets, responses, `journal.jsonl`, the per-install secret |
| `$XDG_CACHE_HOME/spec-proof/<repo-digest>/mutants/` | mutant bodies |
