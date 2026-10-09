# ADR 0005: Host-delegated proofs and independent modules

Status: accepted. Supersedes the ADR 0002 sections "Packaging, install, `init`
and `enable`", "Model ports and routing", "Repository layout and dependency
rules" and "Pilot reuse", and the parts of "Shallnot changes", "Cache", "CI
`prove`, signing, ledger", "Test strategy per layer" and "CI jobs" that concern
model calls or a single product binary; amends ADR 0001 and ADR 0003 (section
"Revisions").

## Context

spec-proof is tooling for agent harnesses, in the manner of Shallnot: a
deterministic binary plus per-harness plugins. ADR 0001 holds: each proof beyond
traceability is produced by an independent module, chained to the others only by
the common, versioned proof format, and Shallnot is the final gate. ADR 0002
drifted from that shape: it described one product binary that calls models through
Generator and Decider adapters, routes between model families and brokers model
calls from CI.

The project owner decided the following, and the decisions are the ground this
record stands on.

- **No model call in any binary.** spec-proof depends on no model and no harness.
  It makes no LLM call itself: no `claude` or `codex` CLI invocation, no API call,
  no required Ollama. When a proof needs model work (generating mutants, judging
  alignment, asking completeness questions), the binary emits a structured request
  (prompt plus JSON schema). The host harness executes it with its own model, in an
  isolated, blind subagent. The binary validates the response by schema and by
  execution, then decides alone.
- **Independence comes from isolation, blindness and execution**, not from a
  second model family. A mutant counts only if it compiles, passes the tests of the
  other criteria and violates its criterion. A judged signal never has the power to
  block on its own.
- **Independent modules.** One module per proof beyond traceability (alignment,
  discriminating power, completeness), each with its own binary, harness plugin,
  version, release and tests, installable and usable alone. The only contract
  between them is the proof format. The gate is Shallnot: it reads the proofs of
  every module present and decides per criterion, with no model and no network, and
  an absent module never counts as a passed proof. An umbrella plugin installs the
  modules and Shallnot. The signature verifier stays a separate module.
- **System One is an optional accelerator** provided by the environment, through a
  closed-question protocol with a resolution chain.
- **The conflict of interest between the agent and the mutants is limit 5**: the
  benchmark keeps a cheating-agent arm.
- **Machine safety has absolute priority.** The owner's work laptop (24 GB) must
  never crash or slow down.
- **The measurement bench is separate from the product.** Experiment harnesses may
  call specific models to measure; that is not the product.

Constraints the design must hold:

- Claude Code is the first target, then Codex and Copilot CLI.
- Under `claude -p`, the harness the benchmark drives, inline `mcpServers` in a
  repository-supplied subagent are not used, and `-p` auto-trust does not count as
  trust. A mechanism that depends on them is dead in the benchmark.
- `tools` in a subagent definition takes exact names only; a `disallowedTools`
  entry with a specifier removes the whole tool. A role that holds `Bash` is
  therefore only held back by a hook.
- The Bash tool has no stdin.
- Sampling carries nothing on the required targets: Claude Code does not declare it
  (anthropics/claude-code#1785, open); the Codex maintainers declined it
  (openai/codex#4929); Copilot declines it under `-p` (github/copilot-cli#2882,
  open).
- The Go MCP SDK (`github.com/modelcontextprotocol/go-sdk/mcp` v1.8.0) links `net`,
  `net/http`, `crypto/tls` and `os/exec`, which a no-network architecture test
  forbids in a proof module.
- In hook mode the agent has the same filesystem write access as any module, so
  nothing on disk proves who wrote it.
- CI holds no model key and runs deterministic checks only.

The open choices (the host protocol, its trust model, per-harness execution, CI
authority) were settled by structured debates between opposing advocates and a
blind judge; this record consolidates the verdict.

## Decisions

### Independent modules with one shared package

There is one Go module per directory, each with its own `go.mod`, `cmd/`, plugin,
version, release and tests. No module imports another. A monorepo remains
possible on exactly these terms, and architecture tests (`go list -deps`) enforce
them. The proof format library is the only shared package.

| Module | Binary | Role |
|---|---|---|
| `format` | none | the only shared library: the proof format (ADR 0001) and `hostproto` |
| `discpower` | `spec-proof-dp` | discriminating power; owns ecosystem execution |
| `alignment` | `spec-proof-alignment` | alignment; reads discriminating-power proofs |
| `completeness` | `spec-proof-completeness` | advisory |
| `ledger` | `spec-proof-ledger` | `sign`, `ledger append/export/audit` |
| `verify` | `spec-proof-verify` | signature verifier, files only |
| `s1relay` | `spec-proof-s1` | the only code that opens a connection to a System One endpoint |
| `umbrella` | none | marketplace and a thin plugin |

**`format/hostproto`** is pure (no `os`, `net`, `os/exec`): request, ticket and
response types and schemas, `RequestID`, the lifecycle state machine, the binding
check, the `corroboratedBy` invariant, decoders for the documented hook payloads
of the three harnesses, and golden vectors.

**`format/hostproto/mcpstdio`** is a hand-rolled, pure MCP stdio framing over
`io.Reader` and `io.Writer` (`initialize`, `tools/list`, `tools/call`, `ping`,
notifications) plus the one authoritative definition of the executor tools. It
lives in `format` because one shared package is allowed and three copies of the
codec would encode the same rule three times. The `format` root and `hostproto`
never import it, and Shallnot imports nothing from `format`, so an MCP revision is
a `format` minor version that modules adopt independently.

**`format/hostproto/hostprototest`** is test support, never linked into a shipped
binary. It holds the `Store` contract suite, golden JSON-RPC transcripts, the
fake-host personas and `cmd/fakehost`.

**Architecture tests:**

- nothing reachable from a proof module's `cmd/` imports `net`, `net/http` or
  `crypto/tls`;
- `net/http` appears only in `s1relay` and in the signing adapters of `ledger` and
  `verify`;
- `verify` links no `os/exec`;
- no module imports another module.

**Cross-module evidence travels only as proofs.** A consumed Statement's digest is
an input with the additive role `proof`. When that proof is absent from the
verified set, Shallnot marks the consumer stale. The only contract between modules
is the common, versioned proof format, with shared golden fixtures and contract
tests; the benchmark drives each module's binary as a black box and also measures
each module alone (ADR 0007).

**Standalone use** means Shallnot plus one module. Each module's `init` requires
report schema 1.2 or later and writes only its own `proofs.require` entry,
`AGENTS.md` block and harness files.

### Host protocol

#### Request

A request lives at `https://rachid-chabane.com/host-protocol/request/v1`, encoded
in JCS.

| Field | Content |
|---|---|
| `module` | `{name, version}` |
| `kind` | `generate.targeted`, `generate.witnessStrategy`, `judge.validity`, `judge.equivalent`, `judge.alignment`, `decide.closed` |
| `role` | plugin-scoped executor role, e.g. `spec-proof-dp:judge` |
| `shape` | `open`, or `closed` (`/v1/systemone` question types) |
| `prompt` | `{system, user, templateDigest}`, built by ADR 0003's pure blind builders: generation never sees tests; alignment is name-masked |
| `responseSchema` | embedded JSON Schema 2020-12 (no loader) |
| `closed` | closed requests only: the question set of the `/v1/systemone` types: a state and ordered, named questions of the type `choice`, `noul` or `score`, each with its instructions and options that carry their criteria text; the host protocol library defines the set once, with its response schema |
| `round`, `seed` | generation only |
| `limits` | `{maxOutputBytes: 262144}` |
| `inputs[]` | `{role, digest}` |
| `cacheKey` | the cache key |

- **`requestId`** is the SHA-256 of the request's JCS bytes with `module.version`
  removed. It is content-addressed, so identical work is never asked twice, and a
  version bump that leaves templates unchanged keeps the ids. The memo keys of ADR
  0003 are `requestId`.
- **Ticket**: `{requestId, vote, emission, nonce}`. The nonce is 128 bits from
  `crypto/rand` and stays outside the id. Votes are separate tickets.
- **Queue-only metadata**: `meta` (criterion, wave) lives only in queue state and
  is never shown to an executor.
- **Response**: `{requestId, vote, nonce, output, executor}`, where `executor` is
  `{channel, harness{name,version}, isolation, attestation{agentId, capDigest,
  transcriptDigest}, model{name, source: resolved|declared|unknown}}`.

#### Store

`$XDG_STATE_HOME/spec-proof/<module>/<repo-digest>/host/` holds `queue.json`,
`requests/`, `tickets/`, `responses/`, a digest-chained `journal.jsonl` and a
per-install `secret` (mode 0600). A location outside the work tree is hygiene, not
a security boundary.

#### Lifecycle

The state machine lives in `hostproto`.

| State | Entered by | Next |
|---|---|---|
| `emitted` | `prove` reaches a model step with no cached response | `fetched`, `superseded` |
| `fetched` | `fetch_request(ticket, cap)` | `accepted`, or `expired` after 20 min |
| `accepted` | `submit_response`: known ticket, unused nonce, valid cap, output valid against the schema. A schema error is returned to the executor; one correction is allowed. | `validated`, `rejected` |
| `rejected` | execution contradicts the answer, or the schema still fails | re-`emitted` with a new nonce while emissions remain (default 3), else `abandoned` |
| `validated` | at `--resume`: EditSet splice and post-splice invariant, build inside the footprint slice, kill vectors, other criteria's tests, examples, F3, F4 (ADR 0003) | `consumed` |
| `superseded` | an input changed, giving a new id | terminal |
| `abandoned` | emissions exhausted | the claim is UNDETERMINED with reason `host_unresponsive` |

A second, different output for an accepted `(requestId, vote, nonce)` is refused
and journaled as `resubmission_conflict`.

#### Waves, versioning, CLI, exit codes

**Waves.** `prove` is a resumable state machine (deterministic stages, a wave,
exit 3). `--resume` runs the next wave, and the skill loops in-turn until the exit
code is not 3.

**Versioning.** The major version is in the URI and minors are additive. Unknown
fields are ignored and unknown majors refused. Template digests enter `cacheKey`
through the role `prompt_template`.

**CLI, per module:**

- `prove [--changed|--all] [--resume] [--ci --import-host DIR]`;
- `request list --json`;
- `request show|submit <ticket>` (scripts, relay, fake host; it journals `exposed`
  when used outside an executor);
- `mcp --role executor`;
- `hook <pre-tool|subagent-start|subagent-stop|post-tool> --harness H`;
- `host push|pull [--ci-run ID]`;
- `systemone export|import`;
- `doctor`;
- `--record|--replay DIR` (a replay miss is a hard error).

**Exit codes:** 0 clean, 1 blocked, 2 tool failure, 3 `awaiting_host`.

The `Generator` and `Decider` ports, every `model/*` adapter, the `quota`
decorator, `models probe` and the A8 broker leave the product. The CLI adapters
for measurement move to `bench/`.

#### Executor tools

Both MCP tools have closed schemas:

- `fetch_request{ticket, cap}` returns the prompt, the schema and the ticket;
- `submit_response{ticket, nonce, output, cap}` returns a receipt or schema errors.

`cap` is documented to the executor as "set by the harness; leave empty". The
server verifies that `cap = HMAC(secret, agentId)` and that `agentId` is journaled
as a started executor of the request's role. It binds one cap to one ticket.

When `initialize` declares `sampling` and `host.sampling: on`, a third tool,
`drain{module}`, serves pending tickets through `sampling/createMessage` with
`includeContext: "none"` and no tools. Such responses have channel `host-sampled`.

### Execution per harness

Marks: **V** verified in official documentation or CLI help at the pinned harness
version; **D** inferred from documentation, to be settled by a live canary before
reliance; **U** unverified; **B** blocked externally.

The chosen executor is a *plugin* subagent whose `tools` allowlist names exactly
`fetch_request` and `submit_response`, served by the plugin's own `.mcp.json`
server. Rationale: its blindness fails closed (no Bash, no Read, no launch without
the server); its submission is typed JSON; it loads under `claude -p` because a
user-installed plugin is not repository-supplied content; and provenance fails
closed because a plugin `PreToolUse` hook injects a per-subagent capability that
the server requires. Two alternatives lost. A Bash-role executor with a stdin
submit fails open (only a hook holds it back) and cannot submit (no stdin). An
inline server in `.claude/agents/` is not used under `-p`.

#### Claude Code (pinned 2.1.293)

**Plugin `spec-proof-dp`.** The alignment and completeness plugins mirror it.

- **`.mcp.json`**: server `exec`, command `spec-proof-dp mcp --role executor`.
- **`agents/{generator,judge,decider}.md`**:
  - `tools: mcp__plugin_spec-proof-dp_exec__fetch_request,
    mcp__plugin_spec-proof-dp_exec__submit_response`;
  - `model: inherit`, `omitClaudeMd: true`, `maxTurns: 4`;
  - system prompt: "Fetch the ticket in your task, follow its instructions, submit
    JSON matching its schema, reply only with the receipt."
- **`hooks/hooks.json`**:
  1. **PreToolUse on `Agent`**, when `subagent_type` matches
     `^spec-proof-dp:(generator|judge|decider)$`. It returns `allow` with an
     `updatedInput` that replaces the whole object:
     `{subagent_type, description, prompt: "ticket <t>", run_in_background: false}`.
     This drops `model` and any padding. A prompt that holds no single ticket is
     denied.
  2. **SubagentStart on the same anchored matcher** journals `agent_id`.
  3. **PreToolUse on `mcp__plugin_spec-proof-dp_exec__.*`.** If `agent_type` is the
     role of the ticket, `allow` with an `updatedInput` adding `agentId` and `cap`
     (`allow` also approves the call under `-p` with no allow rule). Otherwise
     `deny`, and journal `exposure_attempt`.
  4. **SubagentStop** checks the transcript: the tool calls are exactly `fetch(t)`,
     `submit(t)` and at most one corrective `submit(t)`. If so, the response is
     `attested`.
  5. **PostToolUse on `Agent`** records `resolvedModel` and `usage`. A reply other
     than the bare receipt marks the response `leaked`.
- **`skills/spec-proof-dp-fulfil/SKILL.md`**, with `allowed-tools` pre-approving
  `Bash(spec-proof-dp prove *)` and `Bash(spec-proof-s1 drain *)`. Its steps:
  1. relay closed requests if an endpoint is configured;
  2. `request list --json`;
  3. one foreground `Agent` call per ticket, at most 4 in one message;
  4. `prove --resume`, repeated while the exit code is 3.

**Failure modes are closed.** A missing server means the executor fails to launch.
Missing hooks mean no cap, so the server refuses and tickets stay pending
(`doctor` and the Shallnot message name the hooks). The explicit opt-out
`host.require_attestation: false` records responses as `restricted`.

| Fact | Mark |
|---|---|
| Plugin agents honour `tools`, ignore `hooks`, `mcpServers` and `permissionMode`; plugin `.mcp.json` servers start with the plugin; plugin-scoped names are valid in `tools` and matchers | V |
| An unresolvable `tools` list makes the subagent fail to launch | V ("usually") |
| `-p` without `--bare` loads installed plugins; `system/init` lists `plugins`, `plugin_errors` and MCP servers | V |
| Plugin hooks run inside subagents with `agent_id` and `agent_type` (`plugin:name`) | V |
| `updatedInput` replaces the whole input; `allow` skips the prompt | V |
| `Agent` input carries `model`; PostToolUse on `Agent` gives `resolvedModel` (foreground) | V |
| SubagentStart `additionalContext`; SubagentStop `agent_transcript_path` and `last_assistant_message` | V |
| Skill `allowed-tools` is never gated by workspace trust | V |
| A git-status snapshot reaches every custom subagent and cannot be disabled per agent | V; a residual leak of file names, recorded |
| Plugin hooks and Stop fire under `-p`; `allow` with `updatedInput` on plugin MCP tools inside a subagent | D (canary) |
| Sampling | not declared (anthropics/claude-code#1785 open): B |

#### Codex CLI (pinned 0.155.1)

`init` writes `.codex/agents/spec-proof-dp-<role>.toml` (with
`developer_instructions` and `sandbox_mode = "read-only"`), the executor server as
a marked `[mcp_servers.spec-proof-dp-exec]` table in the project's
`.codex/config.toml` (with `default_tools_approval_mode = "approve"`), the skill in
`.agents/skills`, and hook entries.

**V:** custom agent files accept `sandbox_mode`; skill instructions can request
delegation; SubagentStart and SubagentStop carry `agent_id`, `agent_type` and
`agent_transcript_path`; PreToolUse `updatedInput` works for MCP tools; non-managed
hooks need a per-hash trust review, and project hooks load only in a trusted
`.codex/`; tool hooks are "a guardrail, not a complete enforcement boundary".

**D (live canary, 2026-10-08):** `spawn_agent` takes `agent_type` (the role file's
`name`) and `fork_turns`; with `fork_turns` omitted the child inherits the parent's
whole history, so the skill requires `fork_turns: "none"`. PreToolUse payloads of a
spawned agent carry `agent_id` and `agent_type`, those of the main agent carry
neither, so the pre-tool hook tells the executor from the main agent. A
`mcp_servers` table in a role file is not started for a spawned agent; the same
table in the project's `config.toml` starts once per agent, the executor included.
Tool names replace every character of the server name outside `[A-Za-z0-9_]` with
`_` (`mcp__spec_proof_dp_exec__fetch_request`). Under `codex exec` an MCP tool call
needs `default_tools_approval_mode = "approve"`, and `--ignore-user-config` runs no
hook at all. A role file's `sandbox_mode` is not applied to an agent spawned under
`codex exec -s workspace-write`: the executor runs with the parent's sandbox, which
the `unrestricted` isolation already assumes. The `codex exec --json` stream omits `spawn_agent`; the session's
rollout files record it.

**Not documented:** a per-agent allowlist of built-in tools, so the executor keeps
the shell and isolation is `unrestricted`. Because the executor server sits in the
project configuration, the main agent sees the executor tools too; the pre-tool
hook refuses them to any caller that is not an executor of the module.

**Consequence:** block power only, no local judged pass credit,
`require_attestation: false`.

**B:** none since the owner's ChatGPT Plus plan serves the live canary, in the
nightly window, on pre-listed calls and no spending.

#### GitHub Copilot CLI (pinned 1.0.92)

The plugin ships agents with `tools: ["spec-proof-dp-exec/fetch_request",
"spec-proof-dp-exec/submit_response"]`, the server, the skill, and `preToolUse`
and `subagentStop` hooks. **V:** the agent profile reference documents
`server/tool` names, `tools: []`, and that unrecognized names are ignored.
GitHub's reference also states that an unset `tools` list defaults to all tools.

**D (live canary, 2026-10-08, model `gpt-5.6-luna`):** the main agent starts a
plugin agent through the `task` tool with `agent_type` set to the plugin-prefixed
name (`spec-proof-dp:spec-proof-dp-decider`); the executor fetched, submitted and
had its answer accepted. `preToolUse` payloads carry `cwd`, `sessionId`,
`timestamp`, `toolArgs` and `toolName` only, for the main agent and the executor
alike, so a hook cannot tell them apart (`IdentifiesCaller` stays false);
`subagentStop` carries `agentId`, `agentName` and `agentType`. Hooks see an MCP tool
as `<server>-<tool>` (`spec-proof-dp-exec-fetch_request`). `copilot -p` streams no
`session.start`: the model is in `model.call_start` and a final `result` event
closes the session. The `subagentStop` payload's `agentType` is plugin-prefixed, and
its `transcriptPath` is the CLI's session log. The log carries `copilotVersion`, and
for each agent the `subagent.selected` tool list, its system and user messages, and
its tool starts. Under a bait asking it to read a file and run a shell command, the
executor selected exactly its two tools and ran nothing else.

An answer is recorded `restricted-unverified`. It is raised to `restricted` when
the `subagentStop` hook finds every condition of `allowlist_observed` and
`launch_bare` in the session log for the answering agent:
- the pinned CLI version;
- exactly the two executor tools selected;
- only the fetch and submit of its own ticket;
- a submitted output that matches the stored response;
- a bare receipt as the reply;
- a launch prompt that is the bare ticket, with the agent body as the only system
  message.

Copilot keeps `require_attestation: false` and never reaches
`restricted-attested`, because no hook issues a capability. A collapsed or
unresolved `tools` list is not exactly the two names, so its run gets no raise.

How the run is read:
- **Messages.** The run needs exactly one system message and exactly one user
  message. Its system message is the shipped agent body followed by the CLI's own
  sections, so the rule compares the leading block, up to the first blank line,
  with the executor instructions.
- **The log at hook time** ends at the `subagentStop` hook's start. The decoder
  therefore tolerates an incomplete last line, and drops the CLI's
  `report_intent` bookkeeping call.
- **Outcomes:**
  - a selection beyond the two tools, a call other than fetch and submit, or a
    call for another ticket marks the run `exposed`;
  - a reply other than the receipt marks it `leaked`;
  - any other failed condition leaves the answer `restricted-unverified`, with no
    mark. These are: a wrong version, a short, missing or duplicate selection, a
    missing stored response, an output that differs from it, or a launch that is
    not bare.
- **Size caps.** A transcript over 32 MB, or a line over 4 MB, earns no raise. In interactive
sessions that declare `sampling`, `drain` is available, and each request then needs
one human review; that is measured, not assumed.

#### Other harnesses

Cursor, Gemini, Qwen, Factory, Goose and OpenCode use the `host-inline` channel:
the main agent answers through `request show|submit`. Such answers have block
power only.

**Channel selection** is a pure `SelectChannel(observed, policy)` over observed
state (plugin and server present in `system/init` or `doctor`; `initialize`
capabilities), never over declared files.

**Shallnot provides for free** the end-of-turn hooks of nine harnesses, the
merge-never-rewrite technique (ported into each module's `hostinstall` Strategy per
harness) and the marketplace pattern. Modules install no stop hook.

### System One chain

System One is an optional accelerator provided by the environment, reached through
a closed-question protocol. The chain applies to every `decide.closed` request:

1. **Endpoint.** If `systemone.endpoint` is configured, the skill runs
   `spec-proof-s1 drain --module dp`.
   - `spec-proof-s1` is a separate, optional Go module with its own release,
     installed by the umbrella or by `go install`. Module plugins do not declare it
     as a dependency; the skill runs it only when it is on `PATH` and an endpoint
     is configured.
   - It renders each question set that `spec-proof-dp systemone export` lists as a
     `/v1/systemone` request body (questions named in order; a choice maps each
     option to its criteria text, a score lists the levels from the lowest, a noul
     gives the two meanings), posts it to the endpoint and pipes the response into
     `systemone import --source <model>@<endpointDigest>`. The import translates the
     response with the question set of the ticket: a choice keeps its option and its
     `confidence`, a noul is `true` from probability one half with the confidence
     `max(p, 1-p)`, and a score is the nearest level with the `confidence` of the
     endpoint. A response that leaves a question unanswered is refused. Unlike a
     shell relay, it runs on Windows.
   - The relay is the environment's client of the user's service. No proof-module
     binary opens a model connection, which an architecture test checks, so the
     rule "the binary never calls a model itself" holds without exception. A relay
     failure leaves the request for step 2.
2. **Classification (Claude Code).** Otherwise the mod of the module's plugin may
   classify the request, as described below. A classification that does not answer
   leaves the request for step 3.
3. **Host votes.** Otherwise `decide_k` tickets go to fresh `decider` executors
   (default 3). Confidence is the modal share, and the source is `host-votes(k)`.
4. **Below threshold**, the answer is UNDETERMINED.

Thresholds and ADR 0003's admission test are keyed by `(kind, confidenceSource,
model)`. `confidenceSource` is recorded in `extensions.decision` as `execution`
(the module fixed the confidence), `host-votes(k)`, `host-classify(k,scheme):voters`
or `system-one:<model>@<endpointDigest>`,
with `confidenceMapping`, the id of the mapping that derived the confidence from
what the answerers gave (`host-votes.modal-share`, `host-votes.weakest-stated`,
`host-classify.share-isotonic@<tableDigest>`,
`system-one.choice.confidence`, `system-one.noul.max-p`, `system-one.score.confidence`);
`deciders` names only the signals that decided. A module installed without the
relay runs the full chain from step 2.

**Claude Code classification (`host-classify`).** The plugin of each module ships a
mod. On a Bash call of `<binary> prove` and at the start of a turn, the mod runs
`systemone classify-plan`, which dictates, per closed request, five classifier calls
per question: the text, the model and the label order. The order is a seeded
permutation of the canonical order, rotated by the call index (scheme `rot1`), the
seed being the first 16 bytes of `sha256("host-classify|rot1|" + closedSetDigest +
"|" + questionId)`; a score alternates ascending and descending levels. The mod makes
the calls with `$.model.classify` and pipes the labels to `systemone import --source
host-classify(5,rot1):<voters>`. The binary calls no model.

- **Reading.** The share is the modal count over all calls, a missing vote counting
  against it. A tie gives no prediction. The confidence is the value of the shipped
  isotonic table at the count. A classification answers a request only when every
  question has a confidence of at least 0.95; otherwise it records no answer and the
  request goes to the host votes.
- **Names.** The source is `host-classify(<k>,<scheme>):<model>*<n>[+<model>*<n>]`
  and the mapping `host-classify.share-isotonic@<table digest>`. The decision records
  every call, the seed input of every question and the version of the host.
- **Switch.** `systemone.classify` takes `auto` (the default), `shadow` or `off`.
  `auto` answers only the (decision kind, exact source, session model) entries of the
  admission table, which ships empty; each entry names the version of the host it was
  measured on, and `doctor` reports a mismatch. `shadow` journals and never answers.
- **Provenance.** The binary dictates each call's text, model and label order, and the
  host returns only labels. A classification is recorded on the channel
  `host-classify` at the isolation `restricted-unverified`, with the requested model
  as declared: no harness-written record ties a label to the mod. It is raised to
  `restricted`, its model resolved, only by the opt-in check of the raw API log of the
  user's own host (`host.classify.witness_dir`, equal to the directory of
  `OTEL_LOG_RAW_API_BODIES=file:<dir>`; the product never sets it). The log must hold,
  at `--resume`, for the pinned version of the host, each dictated call exactly once
  inside the window from the emission of the ticket to the import, with the dictated
  system prompt and text, `max_tokens` 20, no tools and no history; no log entry may
  have been used by an earlier witness; and the responses, normalised by the rule of
  the host, must be as a multiset the submitted labels. A logged call that carries the
  dictated text with history, tools or another system prompt marks the classification
  `exposed`; responses that read as other labels are journaled as
  `classify_label_contradicted`; anything else raises and marks nothing. A
  classification never reaches `restricted-attested`: no hook issues a capability for
  a mod's model call.
- **Power.** An unraised classification can only flag a survivor into the pool or
  escalate; a classification that does not flag goes to the deciders, and it never
  counts as a screen-out. A screen-out needs a response at the floor and a threshold
  for the exact source; so does the closed signal of alignment (`tau_route`).

| Fact (Claude Code 2.1.293) | Mark |
|---|---|
| Mods load under `-p`; `--bare`, `--safe-mode` and `disableAllHooks` stop them; `session.start` runs before the first prompt and after each reload; mods are not sandboxed | V |
| Every mods API method is an event that earlier mods rewrite, refuse or answer; no hook payload or transcript records `$.model.*` | V |
| `$.model.classify(text, labels, { model })` returns one of the labels or `undefined`, and rejects on an HTTP failure, an abort or an empty answer; its only option is the model, checked against the allowlist | V |
| The classifier request is separate, with no tools and no history, `maxTokens` 20, `query_source` `hook_prompt`; its system prompt lists the labels JSON-quoted in the given order, and its user message is the text with every line prefixed `> ` between `<text>` tags | V |
| The answer is trimmed and stripped of quotes and trailing dots; a case-insensitive exact match returns the label, else the longest label that appears in it as a whole word, else `undefined` | V |
| `OTEL_LOG_RAW_API_BODIES=file:<dir>` writes untruncated request and response bodies, which hold the whole conversation, and `index.jsonl` (`timestamp`, `session_id`, `query_source`, `model`, `request_id`, `request_file`, `response_file`); it is ignored in project and local settings | V |
| File mode needs `CLAUDE_CODE_ENABLE_TELEMETRY=1` and works with the logs exporter `none`; classifier requests are indexed under `-p` | D (canary) |

**Codex has no classification primitive (CLI 0.155.1).** A closed answer on Codex
comes only from `codex exec --output-schema`, which gives a structured answer with
no confidence. Such an answer is a host vote, not a source of its own. OpenAI's
Decisions API (Luna, with a probability per option) would be a true System One
endpoint, but it is a limited, paid preview behind the API. It is excluded while no
free access exists; when it is reachable, it plugs in as a `systemone.endpoint`
like any other.

### Trust model

**Forgers.** Locally, the main agent and the developer can read the secret, queue
and journal, forge caps, responses and transcripts, edit hooks, and skip dispatch.
The nonce gives freshness and binding only. Caps and attestation are
tamper-evident, not tamper-proof. Provenance only ever removes credit.

**A module verifies** the schema, the binding (ticket, nonce, cap, schema digest,
state; in CI, ids recomputed from the tree), execution, and attestation as
provenance.

**Claim power.** τ is `tau_judge`. `judge_k` is 2 blind tickets, both at or above
τ. A judged signal never blocks alone. The policy `proofs.signed_pass_basis` takes
the values `executed-only`, `corroborated` (the default) or `judged`. Rationale for
the default: a basis that accepts NOT_VIOLATING justified only by "the witness
differs" lets a forged label buy a signed pass, which breaks the owner's rule that
a judged signal never has power alone. Under `trust: signed`, pass credit and
judged exclusions of pool survivors therefore need named executed corroboration.

| Claim | Effect | Executed corroboration required | Otherwise |
|---|---|---|---|
| traceability, `correctCodePasses=false`, `example_failed` | BLOCK | itself | none |
| owner-attributed VALID_c (fails an example of c only), any origin | BLOCK if it survived; pass credit if killed | itself | none |
| judged VALID_c, survivor | BLOCK | confirmed F4 witness; builds; passes every other criterion's tests; `judge_k` VALID | ESCALATE |
| judged VALID_c, killed | pass credit | specific kill (a c-bound test that killed no sentinel); builds; passes the other criteria's tests; F4 witness; judge VALID | no credit |
| EQUIVALENT, survivor | exclusion | F3 `NO_DIFF_FOUND` | ESCALATE; a contradicting answer is rejected |
| NOT_VIOLATING / OFF_TARGET, survivor | exclusion | fails an example of another criterion d and none of c | ESCALATE (`corroborated`); accepted with `judge_k` and counted in `hostExclusions` (`judged`) |
| `decide.closed` screen | pool membership | none. Screen-outs are counted in `screenExclusions`. In CI a missing answer flags the survivor into the pool. | `executed-only` bypasses the screen |
| ALIGNED | clears alignment | judge, plus a SPECIFIC or OVERBROAD matrix that is not CONTAMINATED and not PINS_OFFSPEC | discriminating power absent: `uncorroborated`, which clears under `trust: local` and ESCALATEs under signed unless `judged` |
| MISALIGNED, UNDETERMINED | ESCALATE only | none | none |
| completeness | advisory | none | none |

Alignment without the discriminating-power module escalates rather than passes
under `trust: signed`: an ALIGNED verdict on unanimous votes alone would be a
judged pass.

**Isolation floor.** The setting is `proofs.min_isolation`, with the levels
`restricted-attested`, `restricted` (the default floor), `restricted-unverified`,
`unrestricted` and `inline`. Below the floor, a response loses all judged pass
credit but keeps its block power. So does a response marked `leaked`, `exposed` or
`resubmission_conflict`. The floor applies under both trust levels. The `executor`
metadata is forgeable, so the floor constrains honest drift, not forgers.
Consequence: on Codex and on inline harnesses, criteria without examples reach no
PASS, local or signed, unless the user lowers the floor, which the report flags;
criteria with examples pass on executed evidence everywhere.

**The cheater can still** generate easily killed mutants, forge screen-outs and
forge VALID on genuine specific kills. These are measured, not prevented.
`executed-only` closes all three; under it, criteria without examples rarely pass.

### CI authority, with no model in CI

1. **Push.** `host push` writes `refs/spec-proof/host/<module>/<pr-head-sha>` with
   git plumbing. It holds responses keyed by `requestId`, the journal digest and the
   module version. CI checks out the PR head SHA, not the merge commit, so
   recomputed ids match; a template change between the laptop and CI's pinned
   version fails as `host_version_skew`, naming both versions. Host mutant bodies
   travel there in plaintext and are known to the agent; hiding applies only to
   CI-fresh operator and sentinel mutants.
2. **Fetch.** The `tools` job fetches the ref as an artifact.
3. **Generate.** `generate` runs with `permissions: {}`, in module order
   (discriminating power, then alignment, then completeness), using
   `prove --ci --import-host`. It recomputes every id from the tree, ignores unknown
   ids, reports missing ones as `awaiting_host`, re-validates schemas, re-executes
   every executed claim, and draws fresh operator and sentinel mutants.
4. **Seed.** The seed is `H(head-sha, first CI run id for that SHA)`. It is recorded
   in the ledger under `seeds/`, and re-runs reuse it, which stops seed shopping by
   re-triggering CI. A new commit gives a new seed; that residual is measured under
   limit 5.
5. **CI-issued requests.**
   - A CI-fresh survivor that needs judgment becomes a CI-issued request: an
     artifact with outcome `awaiting_host_judgment`, which escalates. Without
     CI-issued requests such survivors would escalate forever, because the seed
     changes on every run.
   - The developer runs `host pull --ci-run ID`, which shells out to `gh run
     download`, so the module stays socket-free.
   - The subagents answer, `host push` publishes, and CI re-runs.
   - The screen request for a CI-fresh operator survivor is a CI-issued
     `decide.closed` ticket. A screen-out keeps it out of the pool, and that is the
     measured residual. Once in the pool, under `corroborated`, the survivor clears
     only by a kill or by executed corroboration. `executed-only` bypasses the
     screen, so every differing survivor enters.
6. **Statements.** Imported judged claims carry `executor` and `corroboratedBy[]`
   and keep `producedBy: ci`. Signing, the ledger (host mutants age-encrypted after
   use) and `spec-proof-verify` are unchanged.

### Shallnot changes

These go on the private branch. Shallnot stays deterministic, with no `net`, no
model and no MCP, and it never invokes a module.

1. **Pending reasons** `awaiting_host`, `awaiting_proof`, `awaiting_host_judgment`
   and `host_unresponsive` replace `quota_paused`. Each message names the module,
   its fulfil skill (or `spec-proof:prove-all`) and "never answer requests
   yourself"; `host_unresponsive` says "report to the user, do not loop".
2. **`MaxHostWaits = 3`.** A block whose only blocking findings are pending reasons
   increments a separate consecutive counter in Shallnot's own attempts directory
   instead of `MaxAttempts`. Neither kind of block resets the other counter; only a
   pass or a give-up resets both. A gate therefore sends the agent back at most
   `MaxAttempts + MaxHostWaits` = 6 times in a row, with no dependence on
   module-writable state. An exemption keyed on a user-writable `queueGeneration`
   would be unbounded, and the continuation cap of the harness resets each time a
   tool is called.
3. **Per-criterion composition.** Traceability and every module in `proofs.require`
   must pass. A required module with no proof is `stale_proof`, never a pass. A
   stale `proof` input makes the consumer stale. The report names absent modules per
   criterion.
4. **The `corroboratedBy` invariant.** A judged claim with block or credit effect
   and no corroboration is `malformed`, checked against goldens.
5. **Configuration.** `proofs` gains `signed_pass_basis`, `min_isolation` and
   `max_host_exclusions`, plus thresholds per `confidenceSource`.
   `extensions.proofs` reports per criterion `channels{}`, `isolation{}`,
   `hostExclusions` and `screenExclusions`. Reports stay byte-identical.

### Umbrella plugin

The marketplace `spec-proof` lists Shallnot's plugin, the three module plugins,
`spec-proof-s1` and a thin plugin, `spec-proof`. The thin plugin has `dependencies`
on all of them (V on Claude Code) and holds one skill, `prove-all`. That skill runs
each present module's fulfil loop in discriminating-power, alignment, completeness
order, and interleaves the waves because alignment's judge tickets do not wait.

It holds no logic. On Codex and Copilot, where plugin dependencies are unverified,
the skill's first step runs each module's `doctor` and names the missing modules.
The umbrella plugin replaces the umbrella CLI; `init` and `enable` are per module.

### Machine safety

The rules below bind the product's local work (mutant execution, builds, tests,
the optional local System One endpoint) and the bench's local work alike.

- A deterministic script (`script/mem-guard`) checks free memory (`memory_pressure`,
  `vm_stat`) before each heavy batch and refuses to start below about 40 % free.
  During the batch it stops cleanly when memory pressure reaches warn.
- Heavy work runs under `nice`, with at most 2 heavy workers. Go builds and tests
  run with a bounded `go build -p`; the same rule covers every build.
- Local models, wherever the user configures one as a System One endpoint or the
  bench replays closed questions through one: one model loaded at a time
  (`OLLAMA_MAX_LOADED_MODELS=1`, `OLLAMA_NUM_PARALLEL=1`), a short `keep_alive`,
  an explicit unload after each batch, 9B parameters at most, no 20B model, and
  never two local-model jobs in parallel.
- The product never requires a resident local model.

### Test strategy

| Layer | Content |
|---|---|
| Unit | `RequestID`; every lifecycle transition, including the illegal ones; binding; cap check; `SelectChannel`; each claim-power row under each `signed_pass_basis`; vote aggregation; transcript checker; hook decoders; `MaxHostWaits` |
| Property (`rapid`) | the id is invariant to key order and changes with any input; a stateful queue never consumes an unvalidated response and never revives a superseded one; tampered ticket, nonce, cap or digest is always rejected; **monotonicity**: adding any block- or escalate-direction response never turns BLOCK into PASS; the most pass-favourable assignment of judged outputs never yields credit without `corroboratedBy`; payloads never contain a declared `ID~REV` or test text; consume order never changes a Statement; any interleaving of pending and other blocks ends within `MaxAttempts + MaxHostWaits` |
| Fuzz | MCP framing, response, hook and transcript decoders; `submit` stdin; relay output |
| Contract | `hostprototest` `Store` suite per module; golden JSON-RPC transcripts per module; schema and claim-power goldens shared with Shallnot; generated agent, skill, hook and `.mcp.json` files against the documented formats per harness; the relay against recorded Ollama and Kev wire data; cross-module goldens |
| Architecture | the rules of "Independent modules with one shared package" |
| Integration | real stdio server driven by a real client process; concurrent submits; `host push` and pull against a bare repository; Shallnot reading `pending[]`; relay against a stub server |
| End to end | `fakehost` drives each module **alone** and all of them **through the umbrella**: `init`, an edit, the real `shallnot hook claude-stop` blocks, exit 3, fakehost serves, waves, `host push`, simulated CI `--import-host`, a CI-issued round trip, `sign`, `verify --offline`, two byte-identical reports. Also: an absent module never passes; a stale discriminating-power proof makes alignment stale |
| Live canary (laptop, per harness version; no-call checks first) | Claude Code under `-p`: plugin and server in `system/init`; executor with no Bash; main-session `fetch` denied; rewrite drops `model`; cap refused without hooks; Stop fires. Copilot: allowlist honoured under bait; the product journals the observed selection and refuses the raise for a non-bare launch. Codex: blocked until the quota resets |

**Fake-host personas** (asserted verdicts): `honest` (recordings); `forger`
(uncapped, VALID everywhere, EQUIVALENT and NOT_VIOLATING on every survivor,
screen-outs); `omitter`; `stale` (old nonces); `breaker` (schema); `leaker`;
`padder`; `model-swapper`; `hookless`; `unrestricted` (reads tests); `selector`
(trivially killed mutants); `churner` (`MaxHostWaits`); `slow`.

### Measurement: limits 5 and 8

The benchmark design is ADR 0007; the measurements this protocol requires are:

**Limit 5.** The cheat arm runs in Claude Code with the umbrella and the harness's
single frozen model, and with each module alone. It reports false pass on the
target criterion per `signed_pass_basis` and `min_isolation`, under `trust: local`
and simulated CI, with Wilson intervals; a journal census (exposure attempts,
uncapped submits, rewrites, leaks, conflicts, chain breaks); the share of PASS
resting on `hostExclusions` or `screenExclusions`; the transfer gap on CI-fresh
operator mutants; seed shopping across commits; and the `forger` persona's
zero-token upper bound per task.

**Same-model judge.** Accuracy and κ against oracle labels with `model: inherit`.
Other models are compared on the bench only.

**Blind spots.** Do tests and mutants written by the same model share blind spots?
The kill rate of host-generated mutants is compared with that of operator mutants
(no model) on the same suites.

**Limit 8**, per changed criterion and module: tickets by kind and channel; votes
multiplier; waves; executor tokens from PostToolUse `usage` and stream-json subagent
messages (`parent_tool_use_id`) plus spawn overhead; main-context tokens the
protocol adds; relay latency; wall time per wave; stop blocks and host waits; cache
hits per `requestId`; reject, expiry and abandon rates; CI round trips; escalation
rate per `signed_pass_basis`, with and without examples. Y is reported for the
gate-host session and for the in-session sample (ADR 0007).

## Revisions

The ADR 0001, 0002 and 0003 texts stay as written; the revisions below apply on
top of them.

**ADR 0001**

- `judge {model}` and `generator {model}` become `executor {channel, harness,
  isolation, attestation, model{name,source}, promptDigest, blind}`.
- Judged claims gain `corroboratedBy[]`.
- Additive input roles `host_response` and `proof`.
- The discriminating-power predicate gains `unattributed`.
- The gate policy adopts the claim-power table and `signed_pass_basis`.
- "Generated fresh at check time" narrows to operator and sentinel mutants; host
  mutants are transported, re-executed and treated as known.
- Local stays advisory.

**ADR 0002**

- One `go.mod` and one `cmd/` per module.
- `format/hostproto` replaces the model ports, adapters, quota decorator, routing
  chain, `models probe` and the A8 broker.
- Exit 3 means `awaiting_host`.
- The umbrella plugin replaces the umbrella CLI, and `init` and `enable` become per
  module.
- The Shallnot changes of this record replace ADR 0002's.
- The architecture tests of this record apply.

**ADR 0003**

- `judge_k` same-host blind tickets replace V-S and V-G.
- S0 targeted generation becomes host generation tickets per round.
- The System One chain of this record replaces routing to `nimble` and reasoning
  escalation.
- Discriminating-power PASS does not fold in ALIGNED; Shallnot composes.
- The J-GPT tie-break becomes one more same-host ticket.
- A8 is removed.
- A9 becomes reuse from local to CI under `requestId` with the executor recorded,
  plus CI-issued requests.
- The pilot switch rules are restated by ADR 0006, and SW1 to SW4 are re-derived by
  the amended pilot.

ADR 0004 is superseded in part by ADR 0007, which also fixes the host execution
of the benchmark's arms.

## Consequences

- The Claude Code design rests on items marked **D**: plugin hooks and Stop under
  `-p`, `allow` with `updatedInput` on plugin MCP tools inside subagents, and the
  "usually fails to launch" behaviour when no tool resolves. If one fails,
  provenance fails closed (tickets stay pending) until the user sets
  `host.require_attestation: false`, which records `restricted`; blindness still
  holds.
- Any Copilot selection other than exactly the two tools, and any launch other than
  the bare ticket, leaves the answer `restricted-unverified`.
- Codex has no executor blindness, so its contribution is limited to block power.
- The git-status snapshot and the environment details leak file names to every
  executor. This is unavoidable per subagent.
- The screen is a forgeable pass-enabling surface under the default basis. Its size
  is measured; only `executed-only` closes it, at the cost of criteria without
  examples.
- Alignment alone cannot reach a signed PASS under the default.
- Host mutants are known to the agent. Authoritative unpredictability rests on
  CI-fresh operator and sentinel mutants plus the targeted mutants the CI loop
  requests. New commits re-seed.
- The hand-rolled MCP codec must track protocol revisions. Its placement in
  `format` is one definition at the cost of `format` minors for transport changes.
- Harness documentation changes often. Every V mark is pinned to the harness
  version and re-checked by `doctor` and the canaries per version.
- Codex `exec` and hook trust review may skip both the module hooks and Shallnot's
  own hooks until the user trusts them.
- A same-model judge shares the coder's blind spots; independence rests on
  isolation, blindness and execution. The cost of that choice is measured by the
  benchmark (ADR 0007), not assumed away.

Unverified at decision time, to settle before relying on them:

- plugin hooks and Stop under `claude -p`;
- the behaviour of an all-unresolved `tools` list;
- `agentType` in Copilot's `preToolUse`;
- Codex `agent_type` in PreToolUse and hooks under `codex exec`;
- plugin dependencies on Codex and Copilot.

Blocked, external:

- Live CI `prove`: CI holds no model credential.
- Tuning confidence thresholds per `confidenceSource` needs run history.
