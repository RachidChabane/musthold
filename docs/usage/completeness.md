# Completeness (`spec-proof-completeness`)

Completeness asks what a specification leaves unstated about what a change does.
It is advisory: its proof has no verdict field, so the gate reports its questions
and never blocks on them. Shallnot refuses `completeness` in `proofs.require`,
and `enable` says so and writes no entry.

The module works alone with Shallnot, and with no `shallnot.yaml` for `enable`.
Commands that every proof module shares are in [commands](commands.md).

## What it does

For each spec file in scope the module raises questions from four signals:

| Question | Raised when | Source of the answer |
|---|---|---|
| dimension | a checklist dimension is `MISSING` from the spec at or above `tau_adv` (below it, a question flagged as low confidence) | one closed question per dimension |
| `unjustified_change` | a changed unit of code is justified by no example and no ALIGNED test of any requirement, and a tournament over the requirements and NONE ends on NONE at or above `tau_adv` | closed question over the project's requirements |
| `unconstrained_behaviour` | a discriminating-power proof lists mutants that no requirement owns and executes a changed file; one question per requirement and changed file | the proof itself, no model |
| `clarity` | a requirement is answered `AMBIGUOUS` or `UNTESTABLE`, at any confidence | closed question per requirement, in groups of at most 64 |

The checklist has eight dimensions: `error_handling`, `boundaries`, `permissions`,
`concurrency`, `compatibility`, `input_validation`, `time_and_locale` and
`state_and_persistence`. Each is `COVERED`, `NOT_APPLICABLE` or `MISSING`. A spec
that declares a dimension out of scope with `Out of scope (<dimension>): <reason>`
is not asked about it.

A unit counts as changed when its syntax tree differs from the one at the base:
formatting, comments and documentation do not make a change. The base is the merge
base of `HEAD` and the default branch (`origin/HEAD`, then `init.defaultBranch`,
then `main`, then `master`), or `HEAD` when none exists, or the reference given with
`prove --base REF`. Untracked files count as added; renames are followed; the
repository and its index are untouched. A project with no base to compare with
does not fail the run.

A changed unit is justified when an example probe or an ALIGNED test of some
requirement may execute it. The module reads that evidence from the
discriminating-power and alignment Statements, using only valid ones for the
requirement's current text whose inputs still match the project. With no such
Statement present it asks about every changed unit and raises no
`unconstrained_behaviour` question. At most `max_hunks` changed units are asked
about per spec, in file and line order; the rest are reported.

Questions are written from deterministic templates, name the requirements they
concern, and are merged and sorted, so the same work gives the same Statement.

## Output and exit codes

`prove` prints `<spec path> <status> <N> questions`, where status is `proved`,
`fresh` or `awaiting_host`, and ` host_unresponsive` when the host stopped
answering.

| Code | Meaning |
|---|---|
| 0 | every spec is proved, whatever questions were raised |
| 1 | the host stopped answering for a spec |
| 2 | tool failure or usage error |
| 3 | tickets wait for the host |

One Statement is written per spec file, about the spec path and the hash of the
file. Shallnot reports each question as a `completeness_question` finding.

## Languages

Units are indexed for Python, Go, JavaScript, TypeScript, Java and Kotlin sources.
A test of Jest, Vitest or JUnit 5 is a unit of kind test, found by the label its
runner reports, so a changed unit that a test calls is justified through the test.
Example operations bind to a function written as `module:function`,
`module.function`, `package.Class#method` or a bare name.

## Role

| Role | Writes |
|---|---|
| `spec-proof-completeness:decider` | answers to the closed questions above (`decide_k` votes when no System One endpoint answers) |

The module has no generator and no judge.

## Configuration

Policy lives in `.spec-proof/completeness.yaml`; the proof is switched on in
`.spec-proof/config.yaml`. See [configuration](configuration.md#spec-proofcompletenessyaml).

## Files

| Path | Content |
|---|---|
| `.spec-proof/cache/completeness/<spec path>/<cacheKey>.json` | the module's Statements; the spec path is percent-encoded into one directory name |
| `.spec-proof/pending/completeness.json` | requests awaited |
| `$XDG_STATE_HOME/spec-proof/completeness/<repo-digest>/host/` | tickets, responses, journal, secret |
