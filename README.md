<p align="center">
  <img src="docs/logo.svg" width="96" alt="musthold logo">
</p>

<h1 align="center">musthold</h1>

<p align="center">
  Proof that a coding agent's tests check what your spec asks, requirement by requirement.
</p>

<p align="center">
  <a href="https://github.com/RachidChabane/musthold/actions/workflows/ci.yml"><img src="https://github.com/RachidChabane/musthold/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/RachidChabane/musthold/releases"><img src="https://img.shields.io/github/v/release/RachidChabane/musthold?filter=discpower/*&label=musthold-dp" alt="Latest musthold-dp release"></a>
  <a href="https://pkg.go.dev/github.com/RachidChabane/musthold/discpower"><img src="https://pkg.go.dev/badge/github.com/RachidChabane/musthold/discpower.svg" alt="Go reference"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/RachidChabane/musthold" alt="License: Apache-2.0"></a>
</p>

![A Claude Code session implements BOX-4. musthold-dp blocks BOX-2: three mutants pass every test of the criterion and fail its example ticket_price(age=65) => 8. The agent adds a test of the ages 65 and 120, the proof passes, the agent publishes the proofs and the gate allows the stop.](https://raw.githubusercontent.com/RachidChabane/musthold-demo/main/media/loop.gif)

An agent writes the code and the tests, and the tests pass. That says little. A
test can stay green while the code it runs is broken, and tests written by the
same model tend to assert what the code does rather than what was asked
([Konstantinou, Degiovanni and Papadakis, 2024](https://arxiv.org/abs/2410.21136)).

musthold reads a spec written in plain language and proves, for each
requirement, that its tests would fail on code that breaks it. It runs inside the
agent's loop, behind [Shallnot](https://github.com/RachidChabane/shallnot)'s
end-of-turn gate. When the tests miss a violation, the gate names the requirement
and an input they do not tell apart, and the agent strengthens them. The gate never shows
the broken code, so a test cannot be fitted to it.

The GIF above shows a real session, [recorded](demo/loop/recording/transcript.md)
in containers. It ran Claude Code with Sonnet 5.5, took 9 min 17 s and cost
2.38 USD.

## Scan any repository

The scan needs no spec, model or configuration.

```
go install github.com/RachidChabane/musthold/discpower/cmd/musthold-dp@latest
musthold-dp scan
```

It runs your pytest, Go, Jest or Vitest suite, breaks the code each sampled test
runs, and lists the tests that still pass, with the change they missed and a
script that replays it. The quick scan takes about a minute and a half;
`--full` judges more tests. See [scan](docs/usage/discpower.md#scan).

## The four proofs

| Proof | Question | Binary | Effect |
|---|---|---|---|
| Traceability | Did a test that cites the requirement run and pass? | `shallnot` | blocks |
| Discriminating power | Would its tests fail on a mutant that violates it? | `musthold-dp` | blocks |
| Alignment | Does each test check what its requirement states? | `musthold-alignment` | escalates |
| Completeness | What does the spec leave unstated about the change? | `musthold-completeness` | advisory |

A requirement passes only when traceability and every required proof pass. A
missing or stale proof never passes.

The proofs run on pytest, Go, Jest, Vitest, and JUnit 5 with Maven or Gradle (Java
and Kotlin).

## How it works

No binary calls a model. When a proof needs one, the binary writes a request, and
the harness you already use (Claude Code, Codex, Copilot CLI) answers it in a
subagent that sees nothing else.

Execution decides. A mutant counts only if it builds, passes the tests of the
other requirements and fails an example of its requirement, or a witness test
written from the spec before any mutant existed. A model's judgment alone never
blocks and never clears a survivor.

Each proof is a separate module with its own binary and plugin, usable alone with
Shallnot. The gate reads their proofs and decides with no model and no network.

## Quickstart (Claude Code)

You need Go, Claude Code, your project's test runner and
[Shallnot](https://github.com/RachidChabane/shallnot) v0.5.1 or later.

```
go install github.com/RachidChabane/musthold/discpower/cmd/musthold-dp@latest
go install github.com/RachidChabane/musthold/alignment/cmd/musthold-alignment@latest
go install github.com/RachidChabane/musthold/completeness/cmd/musthold-completeness@latest
go install github.com/RachidChabane/musthold/verify/cmd/musthold-verify@latest
```

In Claude Code:

```
/plugin marketplace add RachidChabane/musthold
/plugin install musthold@musthold
```

In your project:

```
shallnot init
musthold-dp init && musthold-dp enable
musthold-alignment init && musthold-alignment enable
musthold-completeness init && musthold-completeness enable
```

Then work as usual. When the agent ends its turn, the gate runs; while a proof is
owed, the agent runs the `musthold:prove-all` skill, which proves, answers the
modules' requests in blind subagents and publishes the proofs.
[Getting started](docs/usage/getting-started.md) covers binding the examples of a
spec to code, Codex and Copilot CLI.

## Documentation

- [Getting started](docs/usage/getting-started.md) and [configuration](docs/usage/configuration.md)
- [Trust model](docs/usage/trust-model.md): local proofs, signed CI proofs, the ledger
- [System One](docs/usage/system-one.md): an optional endpoint for the modules' closed questions
- [Harnesses](docs/usage/harnesses.md): Claude Code, Codex, Copilot CLI
- [Releases](docs/usage/releasing.md): verified installer, signatures, SBOM
- [Architecture](docs/architecture.md), [ADRs](docs/adr) and [contributing](CONTRIBUTING.md)
- [Troubleshooting](docs/usage/troubleshooting.md) and [security](SECURITY.md)

## License

Apache-2.0. Dependency licences are in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
