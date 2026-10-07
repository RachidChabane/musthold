# spec-proof

spec-proof checks that the tests of a project verify what its requirements say.
A requirement-to-test trace tells you a cited test ran and passed. It does not
tell you the test checks the requirement, or that it would fail if the behaviour
were wrong. spec-proof produces proofs of those facts and hands them to
[Shallnot](https://github.com/RachidChabane/shallnot), the gate that composes
them with traceability.

## The four proofs

| Proof | Question it answers | Module | Binary | Gate effect |
|---|---|---|---|---|
| Traceability | Did a test that cites the requirement run and pass? | Shallnot | `shallnot` | blocks |
| Discriminating power | Would the tests of the requirement fail on a mutant that violates it? | `discpower` | `spec-proof-dp` | blocks (executed evidence) |
| Alignment | Does each test verify what its requirement states? | `alignment` | `spec-proof-alignment` | escalates (judged evidence) |
| Completeness | What does the spec leave unstated about what the change does? | `completeness` | `spec-proof-completeness` | advisory, never blocks |

A criterion passes only when traceability and every required proof pass. A
missing or stale proof never passes.

## Design

- **No binary calls a model.** When a proof needs model work (generating
  mutants, judging whether a mutant violates a requirement, judging alignment,
  asking completeness questions), the binary writes a request: a prompt and a
  JSON schema. The harness you already use (Claude Code, Codex, Copilot CLI)
  answers it with its own model, in an isolated subagent that sees the request
  and nothing else. The binary then validates the answer by schema and by
  execution.
- **Execution decides.** A mutant counts only if it compiles, passes the tests
  of the other requirements and violates its own requirement. A judged signal
  (a model's opinion with a confidence) never blocks alone: blocking and pass
  credit need executed corroboration, and an uncorroborated judgment escalates.
- **Independent modules.** Each proof is its own binary, plugin and test suite,
  usable alone with Shallnot. The only contract between modules is the shared,
  versioned [proof format](docs/architecture.md#the-proof-format). An absent
  module never counts as a passed proof.
- **Deterministic gate.** Shallnot reads the proofs, checks they are current for
  the tree, and decides with no model and no network.

## Modules

| Directory | Binary | Role | Documentation |
|---|---|---|---|
| [`format`](format/README.md) | none | the shared proof format and host protocol library | [format](docs/usage/format.md) |
| [`discpower`](discpower/README.md) | `spec-proof-dp` | discriminating power; executes tests against mutants | [discriminating power](docs/usage/discpower.md) |
| [`alignment`](alignment/README.md) | `spec-proof-alignment` | alignment of tests with requirements | [alignment](docs/usage/alignment.md) |
| [`completeness`](completeness/README.md) | `spec-proof-completeness` | advisory completeness questions | [completeness](docs/usage/completeness.md) |
| [`ledger`](ledger/README.md) | `spec-proof-ledger` | signs CI proofs and keeps them in an append-only ledger | [ledger](docs/usage/ledger.md) |
| [`verify`](verify/README.md) | `spec-proof-verify` | verifies signatures and writes the verified set Shallnot reads | [verify](docs/usage/verify.md) |
| [`s1relay`](s1relay/README.md) | `spec-proof-s1` | optional relay of closed questions to a System One endpoint | [System One](docs/usage/system-one.md) |
| [`umbrella`](umbrella/README.md) | none | marketplace and the `spec-proof` plugin that installs and runs the modules together | [umbrella](docs/usage/umbrella.md) |

## Supported ecosystems

| Ecosystem | Discriminating power | Alignment | Completeness |
|---|---|---|---|
| pytest (Python) | yes | yes | yes |
| Go (`go test`) | yes | yes | yes |
| Jest (JavaScript, TypeScript) | yes | yes | yes |
| Vitest (JavaScript, TypeScript) | yes | yes | yes |
| JUnit 5 with Maven (Java, Kotlin) | yes | yes | yes |
| JUnit 5 with Gradle (Java, Kotlin) | yes | yes | yes |

Tests bind to requirements with Shallnot's `[verifies ID~REV]` tag in the form
each runner reports ([binding documentation](https://github.com/RachidChabane/shallnot/blob/main/docs/binding.md)).

## Quickstart (Claude Code)

Prerequisites: Go, Claude Code, your project's test runner, and a Shallnot whose
JSON report schema is 1.2 or later (see [Status](#status)).

1. Build the binaries from a clone and put them on `PATH`:

   ```
   git clone https://github.com/RachidChabane/spec-proof
   cd spec-proof
   nice -n 10 go install -C discpower -p 1 ./cmd/spec-proof-dp
   nice -n 10 go install -C alignment -p 1 ./cmd/spec-proof-alignment
   nice -n 10 go install -C completeness -p 1 ./cmd/spec-proof-completeness
   nice -n 10 go install -C verify -p 1 ./cmd/spec-proof-verify
   ```

2. Install the plugins in Claude Code. The `spec-proof` plugin depends on
   Shallnot and on every module plugin:

   ```
   /plugin marketplace add RachidChabane/spec-proof
   /plugin install spec-proof@spec-proof
   ```

   To use one module, install only it, for example `/plugin install spec-proof-dp@spec-proof`.
   The marketplace also lists `shallnot`.

3. In your project, equip it with Shallnot, then with each module you want:

   ```
   shallnot init
   spec-proof-dp init
   spec-proof-dp enable
   spec-proof-alignment init
   spec-proof-alignment enable
   spec-proof-completeness init
   spec-proof-completeness enable
   ```

   `init` writes `.spec-proof/config.yaml`, ignores the local proof directories
   and enables the module's plugin in `.claude/settings.json`. `enable` turns the
   proof on, adds it to `proofs.require` in `shallnot.yaml` (except completeness,
   which is advisory) and adds the module's block to `AGENTS.md`.

4. Complete the configuration for local use, as described in
   [getting started](docs/usage/getting-started.md): `proofs.trust: local` and
   `proofs.code` in `shallnot.yaml`, and `operations` in `.spec-proof/config.yaml`.

5. Work as usual. Shallnot's end-of-turn hook runs the gate when the agent
   finishes a turn. While a module owes work, the gate reports `awaiting_host`
   and the agent runs the `prove-all` skill (or a module's `*-fulfil` skill).
   The skill runs `prove`, which exits with code 3 and lists tickets; one blind
   executor subagent answers each ticket; `prove --resume` consumes the answers.
   The loop repeats while `prove` exits 3.

6. Publish the local statements for the gate, and run the gate:

   ```
   spec-proof-verify --accept-local .spec-proof/cache --out .spec-proof/verified
   shallnot check
   ```

## System One

System One is an optional accelerator: a service of your environment that answers
the closed questions of the proof modules (a choice among options, a score, a
true/false) with a confidence. With an endpoint configured as `systemone.endpoint`
in `.spec-proof/config.yaml`, `spec-proof-s1 drain --module dp` posts the waiting
closed questions to it and imports the answers; the proof modules themselves open
no connection. Without it, blind host executors vote and the modal share is the
confidence. See [System One](docs/usage/system-one.md).

## Trust model

Proofs made on a developer machine are advisory feedback: the agent and the
developer share filesystem access, so nothing on disk proves who wrote a local
proof. Under `proofs.trust: local` Shallnot reads such proofs and labels them
`local`. Under `proofs.trust: signed` (Shallnot's default) it admits only
statements produced by CI, signed with a pinned key or a Sigstore identity,
checked by `spec-proof-verify` against a trust policy and kept in an
append-only ledger. CI holds no model: it re-executes every executed claim,
draws fresh mutants, and serves the judged answers the developer's host pushed
through the same checks as any answer. See [trust model](docs/usage/trust-model.md).

## Harnesses

| Harness | Files | Executor isolation | Consequence for judged pass credit |
|---|---|---|---|
| Claude Code | plugin per module and the `spec-proof` plugin | `restricted`: the executor holds only `fetch_request` and `submit_response`; hooks issue a per-subagent capability and attest the transcript | full, under the default `proofs.min_isolation: restricted` |
| Codex CLI | `.codex/agents`, `.codex/hooks.json` and a skill, written by `init` | `unrestricted`: Codex has no per-agent allowlist of built-in tools; attestation is waived | none: judged answers keep their power to block and escalate, and criteria without examples reach no PASS unless the floor is lowered |
| Copilot CLI | `.github/agents`, `.github/hooks` and a skill, written by `init` | `restricted-unverified` | none, as for Codex |

The Claude Code integration relies on plugin hooks firing in the session. When
they do not fire, no capability is issued, the executor server refuses every
answer and the tickets stay pending. Criteria with inline examples pass on
executed evidence on every harness. Details: [trust model](docs/usage/trust-model.md#harness-isolation).

What the test suite exercises, so you can weigh each harness:

- **Claude Code.** End-to-end tests (SP-250, SP-751, SP-791, SP-840) drive each proof
  module's real binary, hooks and executor server with a simulated Claude Code host
  that sends the hook payloads the harness documents; honest, forging, hookless and
  other host personas get the verdicts the design asserts.
- **Codex and Copilot CLI.** Tests compare the generated files with the formats each
  harness documents (SP-242, SP-744, SP-784) and exercise the hook payload decoders
  with documented payloads.
- **Live sessions.** No test of the suite starts a live Claude Code, Codex or Copilot
  session, so behaviour that only a live session shows is not established by it:
  plugin hooks firing under non-interactive `claude -p`, Codex hooks under
  `codex exec`, whether Copilot honours an MCP-only tool allowlist and what it does
  when every name in the list fails to resolve, and the agent type Copilot reports to
  a pre-tool hook.

## Status

- Every module requires a Shallnot at or above the release carrying report schema
  1.2. Shallnot v0.4.2 emits schema 1.1 and is refused. `doctor` of each module
  reports the `shallnot` it finds, and `init`, `enable` and `disable` refuse a
  Shallnot below 1.2.
- The install URLs above (`git clone` and `/plugin marketplace add`) resolve only
  once the repositories are public.
- The module `go.mod` files reference the `format` module by a local `replace`,
  so binaries are built from a clone as shown, not with `go install pkg@version`.
- The Codex and Copilot files are generated from the same definition as the
  Claude Code plugin and checked against the formats each harness documents.

## Documentation

- [Getting started](docs/usage/getting-started.md)
- [Configuration](docs/usage/configuration.md)
- [CI, host push and pull, ledger and verification](docs/usage/ci.md)
- [Trust model](docs/usage/trust-model.md)
- [System One](docs/usage/system-one.md)
- [Troubleshooting, exit codes and pending reasons](docs/usage/troubleshooting.md)
- [Architecture](docs/architecture.md) and [contributing](CONTRIBUTING.md)
- [Architecture decision records](docs/adr)

## License

Apache-2.0. See [LICENSE](LICENSE).
