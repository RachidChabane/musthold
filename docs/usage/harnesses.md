# Harnesses

Every statement here about what a harness does holds as of Claude Code 2.1.295, Codex CLI 0.155.1 and Copilot CLI 1.0.92, the versions pinned in
`format/hostproto/harness/harness.go` and recorded in [ADR 0005](../adr/0005-host-delegated-proofs-and-independent-modules.md).

| Harness | Files | Executor isolation | Consequence for judged pass credit |
|---|---|---|---|
| Claude Code | plugin per module and the `musthold` plugin | `restricted`: the executor holds only `fetch_request` and `submit_response`; hooks issue a per-subagent capability and attest the transcript | full, under the default `proofs.min_isolation: restricted` |
| Codex CLI | `.codex/agents`, `.codex/hooks.json` and a skill, written by `init` | `unrestricted`: Codex has no per-agent allowlist of built-in tools; attestation is waived | none: judged answers keep their power to block and escalate, and criteria without examples reach no PASS unless the floor is lowered |
| Copilot CLI | `.github/agents`, `.github/hooks` and a skill, written by `init` | `restricted` for an unmarked executor run verified by the stop hook from the session log, otherwise `restricted-unverified`; never `restricted-attested` | a verified run meets the default `restricted` floor and is eligible for judged pass credit with executed corroboration; an unverified run earns none at that floor |

The Claude Code integration relies on plugin hooks firing in the session. When
they do not fire, no capability is issued, the executor server refuses every
answer and the tickets stay pending. Criteria with inline examples pass on
executed evidence on every harness. Details: [trust model](trust-model.md#harness-isolation).

What the test suite exercises, so you can weigh each harness:

- **Claude Code.** End-to-end tests (SP-250, SP-751, SP-791, SP-840) drive each proof
  module's real binary, hooks and executor server with a simulated Claude Code host
  that sends the hook payloads the harness documents; honest, forging, hookless and
  other host personas get the verdicts the design asserts.
- **Codex and Copilot CLI.** Tests compare the generated files with the formats each
  harness documents (SP-242, SP-744, SP-784) and exercise the hook payload decoders
  with documented payloads.
- **Live sessions.** No test of the suite starts a live Claude Code, Codex or Copilot
  session, so behaviour that only a live session shows is not established by it.
  The opt-in canary (below) is the only code that starts one. The behaviours in question:
  plugin hooks firing under non-interactive `claude -p`, Codex hooks under
  `codex exec`, whether Copilot honours an MCP-only tool allowlist and what it does
  when every name in the list fails to resolve, and the agent type Copilot reports to
  a pre-tool hook.

## Live canary

`tools/canary` and the `script/canary-*` scripts (`make canary-claude`,
`make canary-codex`, `make canary-copilot`) are an opt-in maintainer tool. Each runs
one short real session of a harness, with the harness's own CLI under the
maintainer's own account, records it and evaluates the record. A run spends that
account's model quota. No workflow in `.github/workflows` calls it, so it is never
part of CI or of the suite, and a contributor never needs it. See
[ADR 0005](../adr/0005-host-delegated-proofs-and-independent-modules.md) and `script/canary`, which runs only in the nightly window unless forced.
