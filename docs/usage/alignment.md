# Alignment (`spec-proof-alignment`)

Alignment answers one question per test and requirement: does this test verify
what the requirement states? A test can run, pass and carry the right tag while
asserting something else. The module combines a blind judgment with evidence from
mutation. Its verdicts are judged evidence: a verdict that is not ALIGNED
escalates in the gate and never blocks alone.

The module works alone with Shallnot. Commands that every proof module shares are
in [commands](commands.md).

## What it does

For each test bound to a requirement, the module:

1. **Masks** the test. The judge sees the test and the code it reaches under
   neutral names, without comments, docstrings, display names or citation tags,
   and signatures (no bodies) of the project code the test uses. Criteria are shown
   under opaque tokens in an order that depends only on the test and the criteria;
   no payload names a requirement. A masked input that still holds a requirement
   reference, the word `verifies` or a run of three consecutive words of the test
   name is refused, and the test is decided UNDETERMINED with the reason that says
   why. Parametrized tests keep their parameter values.
2. **Reads the matrix** of the discriminating-power proof of the requirement, when
   one is present and current: which valid mutants the test kills, which mutants of
   other requirements, which unattributed mutants and which sentinels.
3. **Asks blind judge tickets** (`align_k`, default 5) which requirement among those
   shown, or none, the test verifies, and how fully (FULL, PARTIAL, WEAK, NONE).
   With more than 23 requirements the question is played as a tournament in groups
   of at most 23 plus NONE.
4. **Decides**.

## The matrix

| Class | Meaning |
|---|---|
| `UNDEFINED_OWN` | the requirement has no valid mutant |
| `INERT` | the test kills no valid or unattributed mutant |
| `OFF_TARGET` | it kills none of its requirement's mutants |
| `PARTIAL` | it kills fewer than `theta_own` of its requirement's valid mutants |
| `SPECIFIC` | it kills at least `theta_own` of them, with few kills elsewhere (`theta_other`) |
| `OVERBROAD` | it kills enough of its own mutants and many elsewhere |

A test that kills a sentinel is marked contaminated, and one whose kills are mostly
unattributed mutants (`theta_none`) is marked as pinning off-spec behaviour. Neither
is ever eligible to corroborate an ALIGNED verdict.

## Verdicts

| Verdict | When |
|---|---|
| `ALIGNED` | the judge names the declared requirement with a supporting coverage, at the modal share `modal_min` |
| `MISALIGNED` | the judge's modal answer is another requirement, NONE or a weak coverage |
| `UNDETERMINED` | the modal share is below `modal_min`, the test kills a sentinel (`mutation_sensitive_test`, decided before anything is asked), the judge budget is spent (`budget_exhausted`), the host stopped answering (`host_unresponsive`), or the test cannot be masked |

An ALIGNED verdict is corroborated by the matrix exactly when the matrix is
SPECIFIC or OVERBROAD and the test is neither contaminated nor pinning off-spec
behaviour. Without corroboration it is marked `uncorroborated`: it clears the gate
under `proofs.trust: local` and escalates under `trust: signed` unless
`signed_pass_basis` is `judged`. When the judge supports alignment against an
OFF_TARGET, INERT, PARTIAL or off-spec-pinning matrix, one more ticket is asked: if
it supports too the verdict is UNDETERMINED, otherwise MISALIGNED. Evidence added
later never turns a verdict that is not ALIGNED into ALIGNED.

When an optional System One endpoint answers the closed question first and names the
declared requirement at or above `tau_route`, ALIGNED is issued on two signals
(endpoint and matrix). A share of such verdicts (`shadow_rate`) is audited by the
judge, whose outcome replaces the verdict. See [System One](system-one.md).

Without a discriminating-power Statement for the requirement the judge alone may
issue ALIGNED, uncorroborated, and System One never does. When the
discriminating-power module still waits for the requirement, the test is listed as
`awaiting_proof`.

## Output and exit codes

`prove` prints `<ID~REV> <test> <status> <verdict> [reason]`, where status is
`proved`, `fresh`, `awaiting_host` or `awaiting_proof`.

| Code | Meaning |
|---|---|
| 0 | every test is ALIGNED |
| 1 | a test is not ALIGNED, a proof it needs is awaited, or a refusal (see [commands](commands.md#prove)) |
| 2 | tool failure or usage error |
| 3 | tickets wait for the host |

The pending file `.spec-proof/pending/alignment.json` lists tests that wait as
`awaiting_host`, `awaiting_proof` or `host_unresponsive`; `spec-proof-verify` folds
it into the manifest Shallnot reads.

## Languages

Tests in Python (pytest), Go, JavaScript and TypeScript (Jest, Vitest), Java and
Kotlin (JUnit 5) are masked, each by one masker. A test is found by the name its
runner reports, including display names, parametrized and repeated expansions, and
`@Nested` classes.

## Roles

| Role | Writes |
|---|---|
| `spec-proof-alignment:judge` | the criterion a masked test verifies, with its coverage |
| `spec-proof-alignment:decider` | answers to closed questions when no System One endpoint answers (`decide_k` votes) |

## Configuration

Policy lives in `.spec-proof/alignment.yaml`; the proof is switched on in
`.spec-proof/config.yaml`. See [configuration](configuration.md#spec-proofalignmentyaml).

## Files

| Path | Content |
|---|---|
| `.spec-proof/cache/alignment/<ID>~<REV>/<cacheKey>.json` | the module's Statements |
| `.spec-proof/pending/alignment.json` | requests awaited |
| `$XDG_STATE_HOME/spec-proof/alignment/<repo-digest>/host/` | tickets, responses, journal, secret |
