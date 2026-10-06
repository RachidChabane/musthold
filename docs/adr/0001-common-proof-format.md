# ADR 0001: Common proof format

Status: accepted

## Context

Each proof beyond traceability (alignment, discriminating power, completeness)
is produced by an independent module. Shallnot is the final gate: it reads every
module's proofs and decides, and its core stays deterministic, with no network
and no model. The modules and the gate are chained only by a common, versioned
proof format, which is therefore the contract the whole project rests on.

Constraints the format must hold:

- Traceability is already computed by Shallnot from JUnit XML and needs no proof
  file.
- Proofs differ in subject: alignment is per (test, criterion), discriminating
  power per criterion with a tests × mutants matrix, completeness per spec and
  advisory only.
- Executed evidence blocks; judged evidence aggregates and escalates. A
  discriminating-power proof mixes both: kill results are executed, mutant
  validity is judged. A block that rests on both combines an executed result
  with a judgment at or above the configured confidence threshold, such as a
  mutant that survived every test and is judged valid.
- Shallnot does not hash requirement text, so a statement edited without a
  revision bump is invisible to it.
- In hook mode the agent has the same filesystem write access as any module, so
  a proof file on disk proves nothing about who wrote it.

The three open choices (envelope, trust and storage, statement-edit detection)
were settled by structured debates between opposing advocates and a blind judge;
this record consolidates their verdicts where they diverged.

## Decisions

### Envelope: in-toto Statement v1 with project-owned predicates

Every proof is one in-toto Statement v1
(`_type: https://in-toto.io/Statement/v1`, `subject`, `predicateType`,
`predicate`). The project defines three predicate types:

- `https://rachid-chabane.com/attestations/alignment/v1`
- `https://rachid-chabane.com/attestations/discriminating-power/v1`
- `https://rachid-chabane.com/attestations/completeness/v1`

The namespace is independent of the project's name, so a rename never breaks a
stored proof. Each predicate has a JSON Schema 2020-12 whose `$id` is its
predicate type URI, served at that URI. The major version lives in the URI;
minor versions are additive and readers ignore unknown fields, the same rules as
Shallnot's report and as the in-toto specification.

A native format with an in-toto exporter was rejected: it maintains two
representations and the exporter still has to fix the subject mapping. The
existing in-toto `test-result` predicate covers one suite run, not one
acceptance criterion.

Serialization is RFC 8785 (JCS): UTF-8, no timestamps, no host or user names,
relative POSIX paths, arrays sorted by requirement ID, then test name, then
mutant digest. Golden fixtures back a contract test run by every module and by
Shallnot's reader.

### Subject: the criterion and its statement digest

- Alignment and discriminating power:
  `subject = [{name: "<ID>~<REV>", digest: {sha256: <statement digest>}}]`.
- Completeness: `subject = [{name: <spec path>, digest: {sha256: <file hash>}}]`.

The hash of all inputs is the cache key, recorded in the predicate, never the
subject. Subject rules are fixed per predicate in the format specification;
module authors never choose them.

### Statement digest: one implementation, in Shallnot

Shallnot is the only spec parser, so it alone computes the statement digest:
SHA-256 over the canonical form of the requirement's meaning-bearing text
(title, statement, status, justification), normalized to Unicode NFC, LF line
endings, joined continuation lines, collapsed whitespace, trimmed. The
canonicalization is specified once, normatively, in Shallnot's spec-format
documentation, and carries a version identifier (`c1`). Shallnot emits
`statement_digest` per requirement in its JSON report (additive, report
schema 1.2). Modules copy the digest from that report and never re-parse specs.

The canonicalization version is recorded in the predicate
(`requirement.statementCanonicalization`), not in the digest key: in-toto
consumers must ignore unrecognized digest algorithms, so a custom key would
make the subject unmatchable by standard tooling.

A changed canonicalization is a major change of every predicate.

### Common predicate core

Required in all three predicates:

- `requirement {id, revision, statementCanonicalization}` (absent for
  completeness)
- `module {name, version}`
- `inputs[]`: sorted resource descriptors with digests and a `role`
  (`spec`, `test_source`, `code_tree`, `junit_result`, `mutant_set`,
  `model_id`, `prompt_template`)
- `cacheKey`: SHA-256 over the JCS of subject and inputs; with `ID~REV` it keys
  the cache, so only criteria whose inputs changed are re-judged or re-mutated
- `producedBy`: `local` or `ci`
- `extensions {}`

### Evidence class per claim

Every verdict-bearing claim is `{value, class: executed | judged, confidence?}`.
`confidence` is required for judged claims and forbidden for executed ones. Each
predicate schema constrains the class each claim may take.

- alignment: `test`, `verdict` (judged: `ALIGNED`, `MISALIGNED`,
  `UNDETERMINED`), `judge {model, promptDigest, blind: true}`.
- discriminating power: `tests[]`, `correctCodePasses` (executed), `mutants[]`
  each with `digest`, `validity` (judged: `VALID`, `EQUIVALENT`,
  `NOT_VIOLATING`, `UNDETERMINED`) and per-test `results` (executed: `killed`,
  `survived`, `error`), `generator {model}`. Mutant bodies are never part of the
  proof.
- completeness: `advisory: true`, `questions[] {dimension, question,
  requirementIds[]}`, `outOfScopeDeclared[]`. The schema has no verdict field, so
  the gate cannot block on it.

### Gate policy in Shallnot

Shallnot decodes proofs with the standard JSON library, requires the Statement
v1 `_type`, ignores unknown predicate types, recomputes the subject digest and
every input digest it can derive from the tree, and classifies each proof as
valid, stale or missing. Stale or missing never yields a pass. Per criterion:

- traceability blocks as it does from JUnit;
- `correctCodePasses = false` blocks;
- a mutant that survived every test and is judged `VALID` at or above the
  configured confidence threshold blocks;
- a survived mutant judged `VALID` below threshold, or `UNDETERMINED`,
  escalates;
- survived `EQUIVALENT` or `NOT_VIOLATING` mutants are excluded and reported;
- a pass requires at least one killed mutant judged `VALID`;
- `MISALIGNED` escalates and never blocks alone;
- completeness questions are reported, never gated.

New finding categories: `stale_proof` (blocking when the expected predicate
type carries executed evidence, escalating otherwise) and `statement_changed`. Results go
into the report's reserved `extensions`, with the digest of every consumed proof
file; the same commit and the same proof files give a byte-identical report.

### Statement edits without a revision bump

Proofs bound to an old statement digest become stale, which means absent. For
traceability-only users, `shallnot check --baseline <base report.json>` raises
`statement_changed` (warning by default, configurable) for any `ID~REV` whose
digest differs from the baseline. No reference-hash lockfile and no acknowledge
command: the gate keeps no committed mutable state, and a revision keeps its
meaning of "the meaning changed".

### Trust and storage: CI is the authority, made portable by keyless signatures

- Local: `.spec-proof/cache/`, git-ignored, unsigned, `producedBy: local`,
  advisory only. It gives fast feedback inside the agent loop.
- CI: a reusable, SHA-pinned `prove` workflow in the monorepo regenerates proofs
  in an unprivileged container that holds no OIDC request token, repository
  token or model key. It wraps the exact JCS Statement bytes in a DSSE envelope
  and signs them with Sigstore keyless signing from the CI identity. The trust
  policy pins the build signer URI and digest to released versions of that
  workflow.
- Authoritative storage: an append-only, content-addressed orphan branch
  `spec-proof/ledger`, laid out as
  `proofs/<module>/<ID>~<REV>/<cacheKey>.dsse.json`. Deletion and force-push are
  blocked. Nothing is committed to the working tree.
- An Ed25519 identity serves private, self-hosted and air-gapped adopters; its
  key lives only in a CI environment restricted to default-branch workflow
  definitions. A key on a developer machine is never an accepted identity.
- A verifier module in the monorepo checks signatures against the trust policy
  and a trusted-root snapshot verified back to a root embedded in its binary,
  then writes `.spec-proof/verified/` with a manifest (verifier version,
  trusted-root digest, policy digest, bundle digests). Shallnot reads only the
  verified set; X.509 verification imports `net` in Go and stays out of its
  core.
- Replay: checkout, fetch the ledger, `verify --offline`, `shallnot check`
  gives a byte-identical report.

### Mutants

Mutant bodies never enter the working tree. Locally they live in the module's
cache outside the repository. In CI they are generated fresh at check time,
seeded per run, and stored in the ledger encrypted with age to an audit
recipient key that CI needs no secret for, so a human can audit a sample.

## Consequences

- The in-loop guardrail is fast feedback, not proof: an adversarial agent can
  edit the local cache or disable the hook. The authoritative block happens in
  CI after a push. Published claims are scoped accordingly.
- Spec weakening (weaken a statement, bump its revision, re-tag the tests) is
  caught by no deterministic check; it relies on spec-owner review, and
  published claims exclude it.
- A test that asserts on source hashes kills every mutant without checking
  behaviour; only the alignment judge and its cross-check against the mutant
  matrix catch it.
- The canonicalization is a cross-repository contract: changing it invalidates
  every stored proof. It needs property-based tests on both sides of the
  normalization (harmless reformatting keeps the digest, real edits change it).
- Meaning carried outside the requirement text (glossary, referenced documents)
  is seen by neither the digest nor the proofs.
- Proofs (in-toto) and the gate report (native) are two document families with
  identical versioning rules, kept identical by the format module's contract
  tests.
- The CI container is the trust boundary; whether test code can escape it and
  reach the OIDC request token is part of the cheating-agent experiment.

Unverified at decision time, to settle before relying on them:

- whether `cosign attest-blob --statement` signs an on-disk Statement with an
  arbitrary name-and-digest subject;
- whether fork pull-request workflows can obtain `id-token: write` (fork proofs
  are treated as pending until a maintainer-triggered run);
- whether a private repository can sign through public-good Sigstore (moot for
  the project's own measurement campaign, which runs after the repository goes
  public).

Blocked, external:

- Default-branch rulesets on a private personal repository need GitHub Pro;
  "require workflows to pass" rulesets need an organisation.
- Confidence thresholds for judged claims need run history; they are explicit
  configuration with conservative defaults until that history exists.
