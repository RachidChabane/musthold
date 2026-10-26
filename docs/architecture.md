# Architecture

This page is for contributors. It describes how the repository is organised and
the contracts the code keeps. Decisions and their rationale are in
[the ADRs](adr): [0001](adr/0001-common-proof-format.md) (proof format),
[0003](adr/0003-proof-methods.md) (proof methods),
[0005](adr/0005-host-delegated-proofs-and-independent-modules.md) (host protocol
and modules) and [0008](adr/0008-input-descriptors-and-staleness.md) (input
descriptors).

## Module layout

The repository is a Go workspace (`go.work`). Each directory below is a Go module
with its own `go.mod`, tests and, where it ships one, binary, plugin and release.

| Directory | Binary | Role |
|---|---|---|
| `format` | none | the proof format and the host protocol: the only shared package |
| `discpower` | `musthold-dp` | discriminating power; owns ecosystem execution |
| `alignment` | `musthold-alignment` | alignment; reads discriminating-power proofs |
| `completeness` | `musthold-completeness` | advisory |
| `ledger` | `musthold-ledger` | `sign` and the append-only ledger |
| `verify` | `musthold-verify` | signature verifier, files only |
| `s1relay` | `musthold-s1` | the only code that opens a connection to a System One endpoint |
| `umbrella` | none | marketplace and the thin `musthold` plugin |
| `tools/archtest` | none | architecture tests that span the modules |
| `tools/e2e` | none | black-box tests of the built binaries and of the documentation |
| `tools/release` | `musthold-release` | the release pipeline: tags, pre-flight, archives, checksums, SBOMs, formula, notes (never released) |

`bench/` (the benchmark core) and `experiments/` (the pilot and calibration
harnesses) measure the product and are not part of it.

### Inside a proof module

The three proof modules share one layout, which mirrors clean architecture:

```
<module>/
  cmd/<binary>/          main: calls cli.Main
  internal/domain/       business rules; no framework, no I/O
  internal/app/          use cases; depend on ports, never on adapters
  internal/adapters/     one adapter per external concern (filesystem, git, process, ecosystems, ...)
  internal/cli/          composition root: parses arguments, wires use cases to adapters
  internal/platform/     shared technical helpers
  internal/architecture/ tests of the dependency rules
  internal/testkit/      test support
  internal/version/      the version variable and its one-line rendering
  plugin/{claude,codex,copilot}/   generated host files, committed
  specs/<module>.md      requirements, in Shallnot's Markdown spec format
  shallnot.yaml          the module's self-gate configuration
```

## Requirements and traceability

Every module states its requirements in `specs/<module>.md` with ids `SP-<n>~<rev>`
and traces them to its tests with Shallnot (`shallnot gate` runs from the module
directory using its `shallnot.yaml`). A test cites a requirement in its name:
`... [verifies SP-110~1]`. A requirement without a passing test fails the gate.
Ids are unique across the repository.

## The proof format

`format` defines the three Statements (alignment, discriminating power,
completeness), their JSON Schemas, RFC 8785 canonicalization, the cache key, the
input descriptors, the executor profile and the pending file. A module and
Shallnot's reader run the same golden vectors under `format/testdata/golden`. See
[format](usage/format.md) for the shape and ADR 0001 and 0008 for the decisions.

Rules the format keeps:

- **Cross-module evidence travels only as proofs.** A consumed Statement is an input
  with the role `proof`, named by its kind, carrying the requirement in
  `annotations.criterion` and the SHA-256 of the Statement file. When that file is
  not in the verified set Shallnot marks the consumer stale.
- **Inputs are per file.** `code_tree` and `test_source` descriptors name a file by
  its path relative to the directory holding `shallnot.yaml` and the SHA-256 of its
  exact bytes. The module computes the footprint; Shallnot parses no source.
- **Additive minors.** The major version is in the URI; unknown members are
  ignored; unknown majors are refused.
- **Determinism.** The same inputs give byte-identical Statements: no time, host or
  user name, arrays sorted by a fixed key, and the same bytes whatever order the host
  answers in.

## Host protocol

`format/hostproto` is pure: request, ticket and response types and schemas, the
request id, the lifecycle state machine, the binding check, the capability, decoders
for the hook payloads of the three harnesses, and golden vectors.
`format/hostproto/mcpstdio` is the MCP stdio codec and the one definition of the
executor tools. `format/hostproto/hostprototest` is test support and is never linked
into a shipped binary.

**Request.** A JCS document with the module, a `kind` (`generate.targeted`,
`generate.witnessStrategy`, `generate.witness`, `generate.promises`, `judge.validity`,
`judge.equivalent`, `judge.alignment`, `judge.scope`, `judge.labels`, `decide.closed`), a plugin-scoped `role`, a `shape` (`open` or `closed`), the
`prompt`, an embedded `responseSchema`, the question set of a closed request, the
`inputs` and a `cacheKey`. Its `requestId` is the SHA-256 of its canonical bytes
with `module.version` removed, so identical work is never asked twice and a version
bump that leaves the templates unchanged keeps the ids.

**Ticket.** `<requestId>.<vote>.<emission>` plus a 128-bit nonce from `crypto/rand`
that stays outside the id.

**Store.** `$XDG_STATE_HOME/musthold/<module>/<repo-digest>/host/` holds
`queue.json`, `requests/`, `tickets/`, `responses/`, a digest-chained
`journal.jsonl` and a per-install `secret` (mode 0600). A location outside the work
tree is hygiene, not a security boundary.

**Sessions.** One store serves every run of the repository that asks the host:
`prove`, and one `scan:<key>` per scan. The queue tags each request with the
sessions that want it and counts waves per session; a request two sessions want
has one set of tickets. A session sees only its own tickets, supersedes only what
no live session wants, and ends in the wave that leaves none of its tickets
waiting. A new scan ends the older scans; nothing but `prove` ends `prove`.

**Lifecycle.**

| State | Entered by | Next |
|---|---|---|
| `emitted` | a session reaches a model step with no cached response | `fetched`, `superseded` |
| `fetched` | `fetch_request(ticket, cap)` | `accepted`, `rejected`, or `expired` after 20 minutes |
| `accepted` | `submit_response` with a known ticket, an unused nonce, a valid capability and output valid against the schema; one schema correction is allowed | `validated`, `rejected` |
| `rejected`, `expired` | execution contradicts the answer, or the schema still fails, or the time ran out | `emitted` again with a new nonce while emissions remain (3 by default), else `abandoned` |
| `validated` | at `--resume`: the splice, the build, the kills and the invariants of the module hold | `consumed` |
| `superseded` | no live session wants the request any more | `emitted` again as a new emission when a session wants it again, never counted toward abandonment |
| `abandoned` | emissions exhausted | the claim is UNDETERMINED with `host_unresponsive` |

**Waves.** `prove` is a resumable state machine. It does its deterministic stages,
emits the tickets of one wave and exits 3. `prove --resume` runs the next wave. The
fulfil skill loops in the session until the exit code is not 3, listing the
tickets of the prove session with `request list --session prove`.

**Executor.** The host runs a plugin subagent whose tools are exactly `fetch_request`
and `submit_response`, served by the plugin's own MCP server (`mcp --role executor`).
Hooks (`hook pre-tool|subagent-start|subagent-stop|post-tool`) rewrite the executor
launch to the bare ticket, inject a per-subagent capability (the HMAC of the agent id
under the secret) and attest the transcript. With `host.sampling: on` and a client
that declares MCP sampling, a third tool `drain` serves pending tickets through
`sampling/createMessage` with no context and no tools.

**No model in a binary.** The binary validates an answer by schema and by execution
and decides alone. A judged signal never has power alone; see
[trust model](usage/trust-model.md).

## Dependency rules

Architecture tests enforce these with `go list -deps`:

- `internal/domain` imports no adapter, `os`, `io/fs`, `os/exec` or `net`;
  `internal/app` imports no adapter; an adapter imports no other adapter.
- No module imports another module except `format`. Every module is inspected.
- The packages of `format` other than test support import no `os`, `io/fs`,
  `os/exec` or `net`; the `format` root and `hostproto` do not import the MCP codec;
  none of the three links test support.
- `net`, `net/http` and `crypto/tls` are not linked into the binaries of the three
  proof modules. `net/http` appears only in `s1relay` and in the signing adapters
  of `ledger` and `verify`.
- `musthold-verify` holds no `os/exec` symbol of its own; the packages reachable
  from it that import `os/exec` equal a reviewed allowlist.
- No shipped binary contains the test support of `format`.

The rule that no binary calls a model follows: the proof modules open no
connection, and `musthold-s1` is the only client of a System One endpoint.

## Generated files

The plugin trees under `<module>/plugin/`, the marketplace files and the
umbrella plugin are generated from one definition per module. A test per module
fails when a committed tree differs from its generator. Regenerate with
`script/plugin-gen` (`make plugin-gen`).

Per harness, the generator produces:

- **Claude Code**: `.claude-plugin/plugin.json`, `.mcp.json`, `agents/*.md` holding
  exactly the two executor tools with `model: inherit`, a blind context and four
  turns, `hooks/hooks.json` (four hooks under matchers that select the executor
  alone) and `skills/<module>-fulfil/SKILL.md`.
- **Codex**: `.codex/agents/*.toml` (read-only, carrying the executor server), hook
  entries and `.agents/skills/<module>-fulfil/SKILL.md`.
- **Copilot**: `.github/agents/*.agent.md` naming server and tool pairs,
  `.github/hooks/<module>.json` and a skill.

`init` installs these files into a project by merging into every file another tool
shares and writing the files it owns. It is idempotent.

## Documentation

User documentation is `README.md`, the module READMEs and `docs/usage/`. A test in
`tools/e2e` checks that every command, subcommand and flag named in them exists in
the built binaries and that their relative links resolve.
