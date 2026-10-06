# ADR 0002: Product architecture and distribution

Status: accepted

## Context

ADR 0001 fixes the proof format and the trust model. It leaves open how the
product that produces, signs, stores and verifies those proofs is built,
packaged and wired to Shallnot: the implementation language, where slow model
work runs, how each ecosystem executes mutants, how the format is enforced on the
gate side, how CI generates and signs proofs, and how the unreleased Shallnot
branch reaches this repository's CI without entering its public history.

Constraints the architecture must hold:

- Shallnot's core stays deterministic, with no network and no model. Its
  architecture test (SN-66) rejects any `net/` import in the gate binary, and it
  gives up after `MaxAttempts = 3` consecutive blocks.
- The stop hook runs under a bounded time. The 600-second bound is written only
  for harnesses configured through the scaffolded stop hook and for Copilot; the
  Cursor hook and the OpenCode plugin run under the harness's own default.
- Model work is slow and quota-bound. A quota pause inside a hook would surface
  as an unexplained stale proof, because `app.Gate` only logs a command's exit
  status.
- Shallnot's unreleased branch must never enter this repository's public history.
- CI holds no model key and runs deterministic checks only.
- Adopters already have a JVM, Node or Go toolchain and must get one file with no
  extra runtime.
- Business rules must be testable without a database, a network or a framework
  container.

The open choices (implementation language and module split, model-work placement,
format enforcement, mutant execution, Shallnot delivery) were settled by
structured debates between opposing advocates and a blind judge; this record
consolidates their verdict.

## Decisions

### Language: Go for every shipped module, Python only for the benchmark

- One Go 1.26.4 module, `github.com/RachidChabane/spec-proof`, holds every shipped
  component: the format library, alignment, discriminating power, completeness,
  routing, cache, signing, ledger, the verifier, and the `init` and orchestration
  CLI. Builds are static (`CGO_ENABLED=0`).
- Source parsing: `go/parser` (standard library) for Go;
  `github.com/odvcencio/gotreesitter` (pure Go, MIT, cgo confined to its
  `cgo_harness/` directory), pinned, for Python, JavaScript, TypeScript/TSX, Java
  and Kotlin. Only those grammars are linked, through the library's build-tag
  grammar subset.
- Python 3.13 (uv) lives only in `bench/`, one uv project (package `specbench`)
  holding the whole benchmark: driver and statistical analysis. It drives the
  shipped Go binaries as black boxes, is never shipped, and is never imported by
  shipped code (ADR 0004).
- No other runtime ships. Each ecosystem's toolchain is invoked from the project
  the product works on.

Rationale:

- Distribution and the trust boundary favour Go. Adopters get one file with no
  runtime and no `uv`. `sigstore-go` provides `sign.DSSEData`, which signs the
  exact JCS bytes in process, so ADR 0001's open `cosign attest-blob --statement`
  question does not arise. The same library provides
  `testing/ca.NewVirtualSigstore`, so the Sigstore path is tested end to end
  before publication. The trusted root is embedded in the verifier binary, as
  ADR 0001 states. The gate is already Go, with `go list` architecture tests and a
  `script/dist` release.
- Reuse of the pilot's Python harness does not decide the language. Its
  transferable parts (argv, isolation rules, quota strings, prompts) are data and
  port with recorded-output contract tests; its execution parts are bound to one
  runner and one fixture.
- Python's real advantage, scipy and statsmodels, applies to the benchmark, which
  is not shipped.

### Model work never runs in the hook

Slow, quota-bound model work never runs inside or under Shallnot's stop hook, and
Shallnot never invokes spec-proof. The agent runs `spec-proof prove` as an
ordinary, visible tool call; the hook only reads the verified set.

A fallback with a deadline below the hook timeout that resumes later was rejected:
it still spends hook time, quota and blocks opaquely. The cost of an agent that
forgets to run `prove` is bounded by agent instructions and by actionable block
messages (see "Shallnot changes"), not by moving model calls into the hook.

### Packaging, install, `init` and `enable`

Two binaries ship in one archive.

`spec-proof`:

- `init [--dir] [--hooks]`
- `enable|disable <alignment|discriminating-power|completeness>... [--ci]`
- `prove [<module>...] [--changed|--all] [--local|--ci] [--resume] [--record DIR|--replay DIR] [--jobs N]`
- `sign --identity ed25519|sigstore`
- `ledger append|export --to DIR|audit|fetch`
- `cache ls|gc`
- `models probe`
- `policy derive` (specified in ADR 0003)
- `doctor`
- `schema <predicate>`
- `version`

`spec-proof-verify`: `--bundles DIR`, `--accept-local DIR`, `--offline`,
`--out .spec-proof/verified`, and nothing else.

Exit codes of `spec-proof`: 0 clean; 1 blocked or proof run failed; 2 tool
failure; 3 paused on model quota (resumable).

**Install.** `script/dist` builds archives for linux, darwin and windows on amd64
and arm64, with `checksums.txt`, mirroring Shallnot's script.

- Before publication, the owner installs with
  `GOPRIVATE=github.com/RachidChabane go install github.com/RachidChabane/spec-proof/cmd/...@<ref>`
  or from locally built archives. A composite `action.yml` installs from a pinned
  archive. `go install` of a private module is not blocked: `GOPRIVATE` with git
  credentials suffices.
- Shallnot is installed its own way. A Shallnot release that carries report 1.2 is
  blocked on the owner's publication decision; until it exists, the binary is
  built from the private mirror (see "Shallnot's unreleased branch").

**`init` ownership is layered.** `shallnot init` stays complete and usable alone.
`spec-proof init`:

1. runs `doctor`, which requires a `shallnot` whose `shallnot schema report`
   declares `schema_version` of at least 1.2;
2. runs `shallnot init --dir … --hooks …`, passing those flags through;
3. writes `.spec-proof/config.yaml` with every proof disabled and the detected
   ecosystems recorded;
4. appends `.spec-proof/cache/` and `.spec-proof/verified/` to `.gitignore`.

Nothing else is written. The result is traceability only. A golden test asserts
that the result equals `shallnot init`'s output plus exactly those two changes.

**`enable <proof>`:**

- sets the proof to enabled in `.spec-proof/config.yaml`;
- adds or extends the `proofs:` block of `shallnot.yaml` through a
  comment-preserving yaml.v3 node edit, validated by running
  `shallnot check --config` on the result;
- writes its own `spec-proof:begin/end` block in `AGENTS.md`, never touching
  Shallnot's block. The block says: after any change to code, tests or specs, run
  `spec-proof prove --changed` before finishing; on exit 3, stop and report that
  the proof run is paused on quota;
- with `--ci`, writes `.github/workflows/spec-proof.yml`, a caller of the reusable
  `prove.yml` pinned by commit SHA.

Completeness is enabled the same way and stays advisory. `disable` reverses each
edit.

### Ecosystems and mutant execution

Supported runners are all those Shallnot binds: pytest, Jest, Vitest, JUnit 5 on
Maven, JUnit 5 on Gradle (Java and Kotlin), and Go `testing`. Alignment and
completeness execute nothing, so they work for any ecosystem Shallnot binds.
Discriminating power uses the strategies below.

**Port** (`internal/app/ports/ecosystem.go`; Strategy pattern, with a Factory in
`internal/adapters/ecosystem/registry.go` choosing by `Detect`):

```go
type Ecosystem interface {
    Name() domain.EcosystemName
    Detect(root fs.FS) (Detection, bool)
    Toolchain() ToolchainResolver   // interpreter / package runner / wrapper resolution
    Locator() DefinitionLocator     // definitions with byte spans
    Assignments() AssignmentLocator // module-level assignments with byte spans; nil when unsupported
    BuildCheck() BuildCheck         // compile/import smoke; failure => every test "error"
    Selector() TestSelector         // bound tests from the Shallnot report -> native selection
    Runner() SuiteRunner            // runs the selection, returns per-test outcomes from JUnit XML
}
```

An `EcosystemContract` suite (see "Test strategy") is mandatory for every
strategy. Each ecosystem has a mandatory build-check stage before its mutant
matrix.

**Edit format.** Mutants are `EditSet {edits:[{file, qualname, target, new_source}]}`.
Each ecosystem has its own qualname grammar: `Class.method`, `pkg.Func`,
`(*T).Method`, `Outer.inner`. Operator mutants and model-generated mutants both
compile to `EditSet`s.

Each edit has a target kind, written `target` and omitted for a definition:

- `definition`: the qualname locates a function or method, and `new_source`
  replaces the whole definition. An edit inside a nested definition is written
  on the outermost definition, so every other definition stays byte-identical.
- `module_assignment`: the qualname is the assigned name (`GRACE`, `pkg.rate`),
  and `new_source` replaces the whole assignment statement (Python) or the
  const or var spec (Go).
- `module_addition`: the qualname is the name a new module-level statement
  declares, and `new_source` is appended on its own line after the last
  statement.

The mutant id is the SHA-256 of the RFC 8785 canonical form of the edit set with
its edits ordered by file, target and qualname.

**Splicing** (pure domain, `internal/domain/mutation`; one `Splicer` serves every
target kind and every caller):

1. resolve the qualname among the units of the target kind: definitions come
   from the ecosystem's definition locator and module assignments from its
   assignment locator; a module addition resolves nothing and is refused when
   the name is already declared;
2. splice into the byte span, or append the addition after the last statement;
3. re-parse the spliced file and check the post-splice invariant: the target
   unit's text equals `new_source` (a module addition has no target before the
   splice), and every other definition and module assignment is byte-identical;
4. run the build check.

The applier refuses paths outside source roots and any test file.

**Per-ecosystem execution:**

| Ecosystem | Toolchain resolution | Build check | Runner (outcomes from JUnit XML) | Selection |
|---|---|---|---|---|
| pytest | config, then `.venv/bin/python`, then `uv run python`, then `python3` | `py -m py_compile <changed>` then import of the changed module | `py -m pytest -p no:cacheprovider -o junit_family=xunit1 --junitxml=<f> <nodeids>` | node ids |
| Jest | package runner from the lockfile (`npx`, `pnpm exec`, `yarn`) | `tsc --noEmit -p <tsconfig>` when one exists, else `node --check` | `jest --ci --reporters=jest-junit -t <re> <files>`, with `JEST_JUNIT_OUTPUT_FILE=<f>` | file plus anchored title regex |
| Vitest | same | same | `vitest run --reporter=junit --outputFile=<f> -t <re> <files>` | file plus title regex |
| Maven | `./mvnw`, else `mvn` | `-o -q test-compile` | `-o -q surefire:test -Dtest=<C#m1+m2> -Dsurefire.failIfNoSpecifiedTests=false`, reading `target/surefire-reports/TEST-*.xml` | class#method |
| Gradle | `./gradlew`, else `gradle` | `--offline -q testClasses` | `--offline test --tests <filter>`, reading `build/test-results/test/*.xml` | `--tests` filter |
| Go | `go` | `go test -count=1 -run '^$' <pkgs>` (compiles the test binaries) | `go tool gotestsum --junitfile <f> -- -count=1 -run <re> <pkgs>` | anchored `-run` regex per subtest path |

Rationale for the runner column:

- A plugin goal invoked directly runs no lifecycle phase, so the build check must
  precede it (`test-compile` before `surefire:test`); otherwise stale or absent
  bytecode is tested.
- Timeouts are enforced by the process adapter (process group, deadline, kill),
  never by a runner plugin: pytest's `--timeout` needs the third-party
  `pytest-timeout`.
- `.pyc` staleness is keyed on source mtime and size, so a same-size mutant
  written within the same second can run old bytecode; hence
  `PYTHONDONTWRITEBYTECODE=1`, a later mtime on every splice, and the stale
  bytecode contract case.
- Mutant runs use the reporter that Shallnot's binding already requires for that
  ecosystem (`jest-junit`, `gotestsum`), so mutant-run test identities equal the
  bound names at no extra install cost.

**Selection mapping.** The report names a bound test by its display text:
`@DisplayName`, a parametrized id, nested `describe` titles or a subtest path. The
selector maps each bound test through the report's `source file:line` and the
locator to the enclosing test definition, then builds the native selector.
Outcomes map back through the same reporter's names.

**Outcomes:**

- `killed`: a bound test failed.
- `survived`: every bound test passed.
- `error`: the build check failed, collection failed, the run timed out, or the
  bound test is absent or skipped in the results.

`correctCodePasses` is executed on the unmutated workspace.

**Validity witnesses.** The generator writes each witness as one test in the
target framework. It runs through the same `SuiteRunner` on the reference
workspace and on the mutant workspace. No ecosystem gets a bespoke evaluation
bridge.

**Workspaces** live under `$TMPDIR/spec-proof-work/<run>/`, outside the
repository, so mutant bodies never enter the working tree.

- The reference workspace is copied once from the tree, then built.
- Mutant workspaces are cloned from that built reference: APFS `clonefile`, Linux
  reflink, else plain copy. Dependency directories are cloned the same way, or
  symlinked where cloning is unavailable.
- `~/.m2`, Gradle home, `GOCACHE` and `GOMODCACHE` are shared through the
  environment.
- Python runs with `PYTHONDONTWRITEBYTECODE=1`; `__pycache__` and `.pytest_cache`
  are excluded from copies.
- The splice always gives the file a modification time later than every build
  output in the workspace.

A strategy that fails the stale-bytecode contract case switches to clean-build
mode, in which build outputs are excluded from clones, and the contract re-runs.

**Parallelism.** `--jobs` defaults to `min(2, NumCPU/4)`. One semaphore bounds
suite processes; a lease of capacity 1 bounds the local System One model.

### Model ports and routing

Two ports, split by role (Interface Segregation). Execution is never routed to a
model.

```go
type Decider interface { // System One: closed questions, one pass
    Decide(ctx context.Context, req decision.Request) (decision.Answers, error)
}
type Generator interface { // reasoning models: schema-constrained JSON
    Generate(ctx context.Context, req generation.Request) (generation.Result, error)
}
```

`Decider` adapters:

- `systemone`: one HTTP client for `POST /v1/systemone`, configured per server
  (Ollama 0.35.1 with `keep_alive`, or the Kev server).
- `jev` and `openaidecisions`: built and contract-tested on documented shapes.
  Live use is blocked (paid access, waitlist).

`Generator` adapters:

- `claudecli` and `codexcli`: the pilot's isolation argv, ported as data; a fresh
  empty directory outside the repository for each call; nesting environment
  variables scrubbed; pure checks in `internal/domain/isolation` (model pin,
  input-token ceiling from the canary baseline, `num_turns`, empty directory
  afterwards, Codex event kinds). Codex reports no resolved model id, so the
  predicate records the configured pin with `resolved: unknown`.
- `anthropicapi` and `openaiapi`: built on the official Go SDKs for adopters with
  pay-per-use keys. The benchmark never uses them.

Decorators (Decorator pattern) wrap either port:

- `record`: request digest is the SHA-256 of the JCS request.
- `replay`: a miss is a hard error, never a live fallback.
- `resume`: skips calls already journaled.
- `quota`: detects exhaustion, persists the job queue under
  `$XDG_STATE_HOME/spec-proof/<repo-digest>/journal/`, and exits 3.

**Routing** is a pure domain policy, Chain of Responsibility, configured per
decision kind (`alignment.verdict`, `mutant.validity`,
`completeness.dimension`, `criterion.testability`):

1. a System One model answers;
2. below threshold, the same closed question escalates once to a configured
   reasoning model with a JSON schema that includes a stated confidence;
3. if the answer is still below threshold, the claim is `UNDETERMINED`.

Which claims a System One answer may settle is bounded by the evidence-grade rule
of ADR 0003; routing is the mechanism, that rule is the limit.

**Judged claims.** For `noul` questions, confidence is `max(p, 1-p)`. The mapping
id and the confidence source (`system-one` or `self-reported`) are recorded in
the predicate's `extensions`. Thresholds are configured per confidence source and
ship as conservative defaults.

**Generation routing** is weighted over configured pins; the pins are fixed in
ADR 0003.

**Blindness.** Judge inputs are built by pure domain builders ported from the
pilot's `alignment_input`. A unit test asserts that no `ID~REV` of the declared
binding appears in the serialized judge state.

### Shallnot changes

Work happens on branch `feat/proof-gate` in the Shallnot repository and is pushed
only to the private mirror. SN-66 and SN-67 hold.

1. **`internal/domain/statement.go`.** Canonicalization `c1`: NFC through
   `golang.org/x/text/unicode/norm`, LF line endings, joined continuation lines,
   collapsed whitespace, trimmed. Pure; specified normatively in
   `docs/spec-format.md`.
   - Report schema 1.2 adds `requirements[].statement_digest`.
   - `check --baseline <report.json>` raises `statement_changed`: a warning by
     default, configurable through `severities`.
2. **`internal/domain/proof`** (pure) holds hand-written structs for the Statement
   envelope and the three predicates, plus explicit invariant checks:
   - `_type` is Statement v1;
   - the subject rule holds per predicate;
   - a judged claim has a confidence in [0,1];
   - an executed claim has no confidence;
   - each claim's class is the one allowed for it;
   - enums are closed;
   - arrays are sorted as ADR 0001 specifies.

   A proof that violates an invariant is classified `malformed` and counts as
   missing. Unknown fields are ignored, because minor versions are additive.

   Policy is ADR 0001's per-criterion rules, plus finding categories where that
   ADR is silent: `stale_proof`, `statement_changed`, `malformed_proof`,
   `correct_code_fails`, `surviving_valid_mutant`, `no_killed_valid_mutant`,
   `escalated_mutant`, `misaligned_test`, `completeness_question`. ADR 0003 adds
   the categories its policy needs.
3. **`internal/adapters/proofset`** reads `.spec-proof/verified/manifest.json` and
   the listed statement files with `os` and `encoding/json` only. It uses no
   schema library.
   - It recomputes the subject digest from its own `statement_digest`.
   - It recomputes the derivable input digests (`spec`, `test_source`,
     `code_tree`) with the file-set digest rule: SHA-256 over sorted
     `path NUL content NUL`, expanded with doublestar.
   - The file sets come from Shallnot's own config: `specs:`, `tests:` and a new
     `proofs.code` glob list. A descriptor whose `annotations.globs` differs from
     those sets is `stale_proof`, so a proof can never narrow its own staleness
     scope.
4. **`shallnot.yaml`** gains one `proofs` block, and the config schema is updated:

   ```yaml
   proofs:
     verified_dir: …
     require: [predicate…]
     code: [glob…]
     trust: signed|local
     thresholds: {validity: 0.70, alignment: …}
     escalation: error|warning     # default: error
   ```

   `thresholds.validity` is the confidence threshold for judged claims that block
   or grant pass credit (default 0.70). `escalation` decides whether an ESCALATE
   finding fails the verdict (`error`, the default) or is reported only
   (`warning`). This block is the single definition; ADR 0003 references it.
   `spec-proof enable` writes `proofs.code` from the detected source roots, and
   spec-proof reads the same key when it builds descriptors.
5. **Report `extensions.proofs`** records the consumed proof digests, the manifest
   digest and the trust level. The same commit and the same proof files give a
   byte-identical report.
6. **Trust levels.**
   - `trust: local` (hook mode) admits `producedBy: local` statements. Findings
     drawn from them send the agent back as in-loop feedback. The report labels
     them `trust: local`; they never satisfy `trust: signed`, and they never
     enter the ledger or a published claim. This is ADR 0001's "advisory, fast
     feedback, not proof."
   - `trust: signed` is required by the CI configuration.
7. **Pending entries.** The manifest may carry
   `pending[] {module, criterion, reason: quota_paused}`, written by
   `spec-proof-verify` from the resume journal.
   - Shallnot still reports `stale_proof`, because a pending proof never passes.
   - The block message for a pending entry says the run is paused on quota and
     must not be retried. After `MaxAttempts` the hook lets go and tells the
     user.
   - Every other `stale_proof` message names `spec-proof prove --changed`.
8. **Shallnot never runs spec-proof.** It gains no `prepare` key, and the hook's
   cost stays `test_commands` plus file reads.
9. **Fixtures.** Shallnot's `script/sync-proof-fixtures <spec-proof checkout>`
   copies the predicate schemas and golden proofs from spec-proof and checks them
   against a SHA-256 manifest. Predicate schemas and goldens are owned by
   spec-proof; Shallnot copies them in. Nothing flows the other way.

### Format enforcement: pure-domain invariants on the gate side, schemas on the writer side

ADR 0001's claim rules are enforced in Shallnot as pure Go domain invariants with
negative golden proofs. JSON Schema validation runs only in spec-proof.

- Generated structs cannot carry the claim rules: `go-jsonschema` documents
  `const`, `if`/`then`/`else`, `allOf`/`anyOf`/`oneOf`/`not`, `uniqueItems`,
  `patternProperties` and nested `$defs` references as unimplemented, so
  "confidence required for judged claims, forbidden for executed ones" cannot be
  generated.
- A schema validator in the gate binary breaks SN-66: `santhosh-tekuri/jsonschema/v6`
  depends on `net/url` and `net/netip`, and the architecture test rejects any
  `net/` prefix. Shallnot's binary links no `jsonschema` package; the library
  appears only in its tests.

### Format library

`format/` is a public, importable package for third-party Go modules. It contains:

- the Statement and predicate types;
- JCS (RFC 8785) through `github.com/gowebpki/jcs`;
- the file-set digest;
- `cacheKey` (SHA-256 over the JCS of subject and inputs);
- subject rules and sort orders;
- the embedded predicate schemas (`format/schemas/*.json`, `$id` = predicate URI);
- golden vectors in `format/testdata/golden/`, covering valid, stale, malformed
  and per-policy-outcome cases, with expected gate outcomes.

`format` links no `net`, `os`, `os/exec` or `io/fs`.

Writer-side schema validation lives in `internal/adapters/schemacheck`, on
`santhosh-tekuri/jsonschema/v6`. Schemas are compiled from embedded bytes with no
URL loader, so no `$id` is ever fetched. It runs before every cache write, every
sign and every ledger append.

### Cache

- Statements are stored as
  `.spec-proof/cache/<module>/<ID>~<REV>/<cacheKey>.json`: JCS bytes,
  `producedBy: local`, unsigned, git-ignored.
- Mutant bodies and raw model records are stored under
  `$XDG_CACHE_HOME/spec-proof/<repo-digest>/`, outside the repository.
- `prove --changed` selects criteria with no entry for their current `cacheKey`.
- `prove` takes criteria, `statement_digest` values and bound tests from
  `shallnot gate --advisory --json-out <tmp>`, which provides fresh results. That
  report is validated at runtime against the schema from `shallnot schema report`.
  Modules never re-parse specs.
- `prove --local` ends by running
  `spec-proof-verify --accept-local .spec-proof/cache --out .spec-proof/verified`.

### CI `prove`, signing, ledger

The reusable workflow `.github/workflows/prove.yml` (`workflow_call`) is
SHA-pinned by callers. It has four jobs.

0. **`tools`.** Builds `spec-proof` from the pinned commit and `shallnot` with
   `script/shallnot-build`. Before Shallnot publishes, only this job holds the
   read-only mirror token. It uploads both binaries as a workflow artifact;
   same-run artifacts download through the runner's runtime token, so later jobs
   need no `GITHUB_TOKEN` permission for them.
1. **`generate`.** `permissions: {}`, a container pinned by digest, no OIDC token,
   no repository token, no secrets.
   - Downloads the `tools` artifact, or the pinned release archives once they
     exist, and installs the target toolchain.
   - Runs `spec-proof prove --ci --changed`.
   - Mutants are generated fresh with a per-run seed and age-encrypted
     (`filippo.io/age`) to the audit recipient's public key.
   - Uploads the statements, the encrypted mutants and a digest manifest.
2. **`sign`.** `id-token: write` (Sigstore) or the `spec-proof-ed25519`
   environment restricted to default-branch workflows. Never `contents: write`.
   Executes no repository code. Checks the artifact against the manifest digest,
   re-validates schemas and `producedBy: ci`, wraps the exact JCS bytes in DSSE
   and signs in process.
3. **`ledger`.** `contents: write` only. `spec-proof ledger append` uses git
   plumbing only (`hash-object`, `mktree`, `commit-tree`, fast-forward push) to
   write `proofs/<module>/<ID>~<REV>/<cacheKey>.dsse.json`,
   `mutants/<cacheKey>.age` and `trust/` (the TUF snapshot) on the orphan branch
   `spec-proof/ledger`. It refuses to rewrite any existing path.

The job that holds `id-token: write` never holds `contents: write`.

`ledger audit` checks three properties: linear history, an empty
`--diff-filter=DM`, and every blob named by its content digest. It stands in for
deletion and force-push rulesets, which need GitHub Pro or an organisation.

A self-test caller runs `generate` in `--replay fixtures/recordings/<case>` mode
and signs with a test Ed25519 identity.

### Verifier

`spec-proof-verify` reads files only.

- **Ed25519:** DSSE PAE with stdlib `crypto/ed25519`. Keys are pinned in
  `.spec-proof/trust-policy.yaml`.
- **Sigstore:** `sigstore-go` `verify` under a policy pinning the build signer URI
  and digest to released `prove.yml` versions. It checks against a trusted-root
  snapshot verified through `tuf` back to a root embedded with `go:embed`.
  `--offline` uses the snapshot exported from the ledger's `trust/`.
- **Output:** `.spec-proof/verified/` holds bare statements plus a JCS
  `manifest.json`: verifier version, trusted-root digest, policy digest, bundle
  digests, `pending[]`.
- **Replay:** `git fetch origin spec-proof/ledger`,
  `spec-proof ledger export --to D`, `spec-proof-verify --bundles D --offline`,
  `shallnot check`. The resulting report is byte-identical.

The verifier keeps its import rule (no `os/exec`, no model or ecosystem adapter)
by data flow: `ledger export` in the main CLI materializes ledger blobs with git,
and the verifier reads the exported files.

### Shallnot's unreleased branch

`script/shallnot-build` builds `shallnot` from:

- `$SHALLNOT_SRC` (a local checkout) on the laptop, or
- in CI, the private mirror repository `RachidChabane/shallnot-proof-gate`,
  checked out with a read-only fine-grained token at the SHA recorded in
  `script/shallnot.pin`.

No Shallnot source, report-1.2 schema copy or full Shallnot report is committed to
this repository, and nothing is pushed to any ref of it. Golden subject digests
are produced by `script/gen-golden`, which runs the mirror-built
`shallnot check --format json` on the golden spec files and commits only the spec
text and the resulting hex digests. Report contract fixtures in spec-proof are
authored from ADR 0001's field definitions, and the live report schema is read
from the built binary.

Vendoring Shallnot source into this repository, in either form, is excluded: the
public history of this repository must never contain the unreleased branch. A
private mirror read with a non-model token satisfies both that constraint and
"CI holds no model key".

### Repository layout and dependency rules

```
go.mod                    module github.com/RachidChabane/spec-proof
cmd/spec-proof/  cmd/spec-proof-verify/
format/                   public contract package; schemas/; testdata/golden/
internal/
  domain/                 criterion/ mutation/ matrix/ alignment/ completeness/
                          routing/ isolation/ ledger/ trust/ policy/      (pure)
  app/                    ports/ initproject/ enable/ prove/ verify/ sign/ ledger/
  adapters/               ecosystem/{pytest,jest,vitest,maven,gradle,golang}/
                          sourceparse/{treesitter,goparser}/ workspace/ process/ junitxml/
                          model/{systemone,jev,openaidecisions,claudecli,codexcli,
                                 anthropicapi,openaiapi,record,replay,resume,quota}/
                          shallnot/ git/ signing/{ed25519,sigstore}/ schemacheck/
                          cachefs/ config/ scaffold/
  cli/                    composition root, wiring only
  architecture/  testkit/
specs/spec-proof.md       self-gated requirements SP-n~REV
fixtures/ecosystems/<runner>/   in-universe projects, three criteria each
fixtures/e2e/  fixtures/recordings/
bench/                    Python (uv) project, package specbench: the whole benchmark,
                          driving the shipped binaries as black boxes; not shipped
experiments/limits-1-3/   frozen at its pre-registration tag, never edited or imported
script/                   ci lint test integration contract e2e fuzz dist record
                          gen-golden shallnot-build install-friction
Makefile                  native front door over script/*; mise.toml may wrap it
.github/workflows/        ci.yml prove.yml prove-selftest.yml
```

Architecture tests (`internal/architecture`, using `go list -deps`, Shallnot's
technique), each citing an `SP-n~REV`:

- `format` and `internal/domain` import no adapter, `os`, `io/fs`, `os/exec` or
  `net`;
- `app` imports no adapter;
- adapters import no other adapter (wiring only in `cli`);
- `spec-proof-verify` links no `os/exec` and none of
  `adapters/{model,ecosystem,process,git,workspace}`.

### Pilot reuse

Ported to Go as data and logic: the claude and codex isolation argv and checks,
quota strings, the canary, the blind-input builders, definition-resolution
semantics, and the proof writer's layout. Prompts and schemas become embedded
data as ADR 0003 fixes them. Pilot run records become contract fixtures for the
CLI adapters. `bench/` copies the pilot's statistics ideas, and the code is re-tested there.

### Test strategy per layer

**Unit** (table-driven Go, every subtest tagged `[verifies SP-n~REV]`): splicing
and the post-splice invariant; matrix aggregation and outcome classification;
`cacheKey` and subject rules; routing and confidence mapping; ledger paths and the
manifest; isolation checks; judge blindness.

**Property-based** (`pgregory.net/rapid`):

- JCS is idempotent and invariant to key order.
- `cacheKey` is invariant under input permutation.
- Splicing a definition's own source is the identity.
- Routing is monotone in confidence.
- Generated predicates pass `schemacheck` exactly when the domain invariants hold.
- In Shallnot (rapid or `testing/quick`), harmless reformatting keeps the `c1`
  digest and real edits change it.
- `script/gen-golden` exports rapid-generated vectors into
  `format/testdata/golden/`, so randomized cases cross into Shallnot's suite.

**Fuzz** (native Go): statement and manifest decoders, the report reader,
`/v1/systemone` responses, CLI outputs, `EditSet`s and tree-sitter span
extraction.

**Contract:**

- Both repositories run the golden proofs, including negative ones: judged without
  confidence, executed with confidence, wrong class, wrong subject.
- `EcosystemContract`, parametrized over every strategy, checks six cases:
  1. a known-killing mutant is killed;
  2. a known-equivalent mutant survives;
  3. a syntax-breaking mutant gives `error`;
  4. selection runs only the bound tests;
  5. stale bytecode: the workspace is cloned from an already-built reference, and
     the known-killing mutant must still be killed;
  6. a display-named or parametrized bound test is selected and its outcome
     attributed to the right criterion.
- `Decider` and `Generator` contracts run on recorded wire data from Ollama and
  Kev, with stub `claude` and `codex` executables from `testkit` for argv,
  isolation violations, quota exit 3 and resume with no repeated call.

**Integration** (real I/O):

- real pytest, Jest, Vitest, Maven, Gradle and Go toolchains on
  `fixtures/ecosystems/*`;
- a differential locator test against reference parsers: Python `ast`, `tsc`, the
  JDK tree API through `java Helper.java`, and `go/parser`; Kotlin uses splice
  identity plus compilation;
- real git for the ledger, real Ed25519, Sigstore through `NewVirtualSigstore`,
  and the real Shallnot built from the mirror;
- a `live` build tag for Ollama, Kev and the CLIs, run on the laptop only.

**End-to-end** (`script/e2e`, a temporary git repository built from
`fixtures/e2e`):

1. `init` produces exactly traceability.
2. `enable` all three proofs.
3. Simulate an agent turn: a code change, then the hook sees `stale_proof`.
4. `prove --replay`, after which a surviving valid mutant blocks.
5. Fix the code and `prove --replay` again, after which the gate passes.
6. `sign --identity ed25519`, `ledger append`, `export`, `verify --offline`, then
   `shallnot check` twice: the reports are byte-identical, and the verdict and
   categories are asserted.
7. Tamper with the verified set: `malformed_proof` or `stale_proof` blocks.
8. Write a quota-pause journal: the pending message appears.

**Self-gating:** `script/ci` runs `shallnot gate` on `specs/spec-proof.md`.

**Benchmark:** `bench` invokes only the binaries built by `script/dist` from the
same commit, and records their digests in provenance (ADR 0004).

### CI jobs

Free Ubuntu runners, deterministic only. Every job calls a `script/*` file, so
laptops run the same commands.

| Job | What it runs |
|---|---|
| `lint` | gofmt, `go vet`, staticcheck, actionlint, ruff on `bench` |
| `test` | `go test -race ./...`, including architecture, unit, property and contract tests |
| `ecosystems` | a matrix of six runners with setup-python, setup-node, setup-java and setup-go, plus caches |
| `shallnot-gate` | builds Shallnot from the mirror and runs Shallnot-side contract tests over the synced goldens, plus self-gating |
| `e2e` | the end-to-end suite |
| `fuzz` | short budget per target |
| `cross-compile` | `script/dist ci`, with a binary-size budget |
| `install-friction` | in `node:22`, `eclipse-temurin:21` and `golang:1.26` images: fetch the archive and run `spec-proof init`; records seconds and bytes and fails over budget |
| `prove-selftest` | the reusable workflow under replay with the test Ed25519 identity |
| `bench` | `uv run pytest` on the benchmark project |

## Consequences

- The hook is a reader of the verified set, never a producer. An agent that
  forgets `prove` still spends one of Shallnot's three blocks on staleness; the
  rate at which the budget is exhausted, and the number of blocks per resolved
  finding, are measured in the benchmark.
- The product ships tree-sitter-based source parsing from a young pure-Go runtime
  maintained mostly by one person. The post-splice invariant, the build check and
  the differential locator tests contain the risk; mutants rejected or all-`error`
  are reported per ecosystem in the benchmark.
- Self-reported confidence from reasoning models is uncalibrated. It is separated
  by `confidenceSource` and measured by the calibration experiment.
- Clone-from-built plus incremental compilation may still be slow per mutant on
  JVM and Node. Wall time per (mutant, suite) is recorded per ecosystem.
- The `c1` canonicalization, the file-set digest and the claim invariants are
  implemented in two repositories. Synced goldens with SHA-256 manifests guard
  them, but a drift surfaces only when both CIs run.
- The private mirror and its token are operational dependencies of CI. A revoked
  token fails the `shallnot-gate` and `e2e` jobs loudly, not silently.
- Porting from the pilot is guarded by stub-CLI contract tests, and by pilot-record
  replay once pilot data exists.
- Two representations of the claim rules exist (Go invariants in Shallnot,
  JSON Schema in spec-proof). The negative golden proofs and the property test
  that generated predicates pass `schemacheck` exactly when the invariants hold
  keep them aligned.

Unverified at decision time, to settle before relying on them:

- the grammar-subset build tags of `gotreesitter` and the resulting binary size (a
  build with the full grammar registry measured 34 MB);
- whether fork pull-request workflows can obtain `id-token: write`;
- whether test code in `generate` can escape the container and reach a token; both
  this and the previous item are part of the cheating-agent experiment, as ADR 0001
  states.

Blocked, external:

- A Shallnot release that carries report 1.2 is blocked on the owner's publication
  decision.
- Public release downloads, Sigstore-signed release archives, a Homebrew tap and
  Marketplace use of the action are blocked until the repository is public.
- Signing against public-good Sigstore is blocked until the repository is public;
  before then the path is tested through `NewVirtualSigstore`.
- Live use of the `jev` and `openaidecisions` adapters is blocked: paid access and
  a waitlist.
- Live CI `prove` is blocked: CI holds no model credential.
- Deletion and force-push rulesets on the ledger branch are blocked (GitHub Pro or
  an organisation); `ledger audit` covers the gap.
- Tuning confidence thresholds needs run history; defaults are conservative until
  it exists.
- Pilot-record fixtures for the CLI adapters need pilot data.
