# Trust model

spec-proof separates what a developer machine can prove from what CI can prove, and
separates evidence that was executed from evidence that was judged.

## Local and signed

| Level | `proofs.trust` | Admits | Guarantee |
|---|---|---|---|
| local | `local` | unsigned statements of the local cache, accepted by `spec-proof-verify --accept-local` | feedback for the agent and the developer, labelled `local` in the report. It proves nothing about who wrote it. |
| signed | `signed` (default) | statements produced by CI, in a set whose manifest the verifier wrote after checking signatures against the trust policy | the statement was produced by a CI run whose signing identity the policy pins |

On a developer machine the agent and the module have the same filesystem access, so
nothing on disk proves who wrote a proof. A local proof is therefore never a pass
for a signed gate. CI re-executes every executed claim, recomputes every request id
from the commit, draws fresh mutants and signs what it produced; it holds no model
key, so judged answers reach it only as a bundle the developer's host pushed, which
CI checks like any other answer ([CI](ci.md)).

## Executed and judged evidence

Every claim in a proof is `executed` (a test ran, a mutant was killed or survived, an
example failed) or `judged` (a model's answer with a confidence).

- Executed evidence blocks.
- A judged signal never blocks or earns pass credit alone. Its power needs executed
  corroboration, or it escalates.
- A binary never calls a model. The harness's model answers in an isolated subagent;
  the binary validates the answer by schema and by execution and decides.

## Claim power

τ is `tau_judge` (discriminating power) or the `proofs.thresholds` entry for the
confidence source of the claim. `judge_k` is the number of blind judge tickets
(2 by default), all of which must reach τ.

| Claim | Effect | Executed corroboration required | Otherwise |
|---|---|---|---|
| tests fail on the reference code; an example fails on it | block | the claim itself | none |
| valid survivor that fails an example of its requirement | block | the example | none |
| judged valid survivor | block | a confirmed witness, the mutant builds, the tests of the other requirements pass, `judge_k` judges agree | escalate |
| judged valid mutant, killed | pass credit | a specific kill (a test of the requirement that killed no sentinel), the mutant builds, the other requirements' tests pass, a confirmed witness, the judge agrees | no credit |
| equivalent survivor | excluded from the pool | a differential search that found no difference | escalate; a contradicting answer is rejected |
| not-violating or off-target survivor | excluded | a failing example of another requirement and none of its own | escalate under `corroborated`; accepted with `judge_k` and counted in `hostExclusions` under `judged` |
| closed screen of a survivor | pool membership | none; only answers at the isolation floor and not marked can screen a survivor out, and screen-outs are counted in `screenExclusions`; a survivor the screen would otherwise have let go, kept in the pool only by the floor, is counted in `screenFloorEntries` | `executed-only` bypasses the screen |
| undetermined survivor, no valid mutant, test that killed a sentinel | escalate | none | none |
| alignment ALIGNED | clears the test | judge plus a SPECIFIC or OVERBROAD matrix that is neither contaminated nor pinning off-spec behaviour | without the discriminating-power module: `uncorroborated`, which clears under `trust: local` and escalates under `trust: signed` unless the basis is `judged` |
| alignment MISALIGNED, UNDETERMINED | escalate | none | none |
| completeness question | reported | none | none |

### Pass basis

`signed_pass_basis` says what a signed pass may rest on.

| Basis | A pass credit or an exclusion may rest on |
|---|---|
| `executed-only` | executed evidence only: an example that fails on the mutant. A judged validity alone gives none. |
| `corroborated` (default) | also a judged claim at τ whose executed corroboration is listed. A survivor judged not violating without an example escalates. |
| `judged` | also a judged claim at τ with no further corroboration; at most `max_host_exclusions` survivors per requirement are excluded on a judgment alone. |

Under `trust: local` the basis is `judged`. A basis that accepts a judgment alone
lets a forged label buy a pass, so the default asks for corroboration, and
`executed-only` closes every judged route at the price that requirements without
examples rarely pass.

## Harness isolation

The harness behaviour stated on this page holds as of Claude Code 2.1.295, Codex CLI
0.155.1 and Copilot CLI 1.0.92, the versions pinned in
`format/hostproto/harness/harness.go`.

A judged answer is credited with the isolation of the executor that gave it. From
strongest to weakest:

| Level | Meaning |
|---|---|
| `restricted-attested` | a restricted, blind executor whose transcript and capability the harness hooks attested |
| `restricted` | a restricted, blind executor. The default floor. |
| `restricted-unverified` | a restricted executor whose restriction is not verified |
| `unrestricted` | an executor that holds the tools of the main agent |
| `inline` | the main agent answered |

`min_isolation` (default `restricted`) sets the floor. Below it, a judged answer
loses all pass credit and exclusion power but keeps its power to block and
escalate. So does an answer marked `leaked`, `exposed` or `resubmission_conflict`,
whatever its level. A floor weaker than `restricted` is flagged in the report.

| Harness | Isolation | Why |
|---|---|---|
| Claude Code | `restricted`, `restricted-attested` when attested | the executor is a plugin subagent whose `tools` list names exactly `fetch_request` and `submit_response`; it holds no shell and no file tool |
| Codex CLI | `unrestricted` | Codex has no per-agent allowlist of built-in tools; the executor keeps the shell, and attestation is waived |
| Copilot CLI | `restricted-unverified` | the allowlist is written, but its enforcement is not verified |
| any other harness | `inline` | the main agent answers through `request show` and `request submit` |

Consequences: on Codex, Copilot and inline harnesses, requirements without examples
reach no PASS, local or signed, unless the floor is lowered; requirements with
examples pass on executed evidence everywhere. Judged answers there still block and
escalate.

## Classification by the host

A classification (`host-classify`) is made of classifier calls that a mod of the
host makes with the session's model; see [System One](system-one.md#classification-by-the-host).

- The module dictates every call: its text, its model and its label order. The host
  returns only labels.
- It is recorded on the channel `host-classify` at the isolation
  `restricted-unverified`: nothing the harness writes ties a label to the module's
  call. It is never `restricted-attested`, because no hook issues a capability for a
  mod's model call.
- Unraised, a classification can only flag a survivor into the pool or escalate. A
  classification that does not flag goes to the deciders: the module rejects it and
  asks the request again.
- It is raised to `restricted`, with the model the log resolved, only by the opt-in
  check of the raw API log of the user's own host. `host.classify.witness_dir` names
  the directory of `OTEL_LOG_RAW_API_BODIES=file:<dir>`. The product never sets the
  log, because it holds the whole conversation.
- At `--resume` the check raises a classification when, for the version of the host
  it was established for, the log holds each dictated call exactly once, with the
  dictated system prompt and text, the token limit of the classifier, no tools and
  no history, inside the window from the emission of the ticket to the import; no
  entry was used by an earlier witness; and the responses, read as the host reads an
  answer, are as a multiset the labels that were submitted.
- A logged call that carries the dictated text with history, tools or another system
  prompt marks the classification `exposed`. Responses that read as other labels are
  journaled as `classify_label_contradicted`. A missing or extra entry, another
  version, an index over 32 MiB, a line or body over 4 MiB, or a file outside the log
  settles nothing.
- A screen-out needs a response at the floor and a threshold for the exact source of
  the classification; otherwise it only flags. The closed signal of alignment needs
  the same threshold in `tau_route`.
- A raised response is forgeable like a transcript. It earns no pass credit without
  executed corroboration.

## What a module checks

A module accepts an executor answer only when:

- the ticket is known, the nonce is the one issued, the schema digest matches, and
  the output validates against the response schema (one correction is allowed);
- when attestation is required, it carries a capability: the HMAC, under a
  per-install secret, of the agent id of an executor started for the request's
  role, bound to one ticket;
- a second, different output for the same ticket is refused as
  `resubmission_conflict`.

On Claude Code the plugin hooks do the rest: a `PreToolUse` hook rewrites the launch
of an executor to the bare ticket with no model override, adds the capability to the
executor's tool calls, and denies the same tools to the main session;
`SubagentStart` and `SubagentStop` journal and attest the executor, and a stop check
requires its tool calls to be exactly `fetch_request(t)`, `submit_response(t)` and at
most one correction; `PostToolUse` records the resolved model and flags a reply that
is more than the bare receipt as a leak. Every event goes to a digest-chained
`journal.jsonl` in the host store; `doctor` verifies the chain.

If the hooks do not fire, no capability is issued, the executor server refuses every
answer and the tickets stay pending: the design fails closed. Setting
`host.require_attestation: false` accepts such answers as `restricted` without
attestation.

## Limits

- Locally, the main agent and the developer can read the secret, the queue and the
  journal, forge capabilities and responses, and skip dispatch. Capabilities and
  attestation are tamper-evident, not tamper-proof, and provenance only ever
  removes credit.
- A cheating agent can still write easily killed mutants, forge screen-outs, and
  forge `VALID` on genuine specific kills. The corroboration rules bound this;
  `executed-only` closes all three.
- Every executor receives the environment details of its harness session, such as a
  snapshot of the git status, which can include file names.
- The `executor` metadata of a response is forgeable, so the isolation floor
  constrains honest drift, not a forger.
