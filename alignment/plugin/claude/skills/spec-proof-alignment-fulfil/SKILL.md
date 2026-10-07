---
name: spec-proof-alignment-fulfil
description: Answer the pending host requests of spec-proof-alignment (alignment) with blind executor subagents and finish the proof run. Use when `spec-proof-alignment prove` exits with code 3, when Shallnot reports awaiting_host for spec-proof-alignment, or when `spec-proof-alignment request list` shows tickets.
allowed-tools: Bash(spec-proof-alignment prove *), Bash(spec-proof-alignment request list), Bash(spec-proof-s1 drain *), Bash(spec-proof-verify --accept-local .spec-proof/cache --out .spec-proof/verified)
---

# Fulfil the requests of spec-proof-alignment

`spec-proof-alignment prove` stops with exit code 3 when it needs model work it cannot do itself. Each request is a ticket. A blind executor subagent answers it, and `spec-proof-alignment prove --resume` consumes the answers.

Never answer a ticket yourself, never read what a ticket asks, and give an executor nothing but its ticket.

1. If `spec-proof-s1` is on `PATH` and `.spec-proof/config.yaml` sets `systemone.endpoint`, run `spec-proof-s1 drain --module alignment`. A failure leaves the requests for the next step.
2. Run `spec-proof-alignment request list`. Each line is `<ticket> <role>`. When it prints nothing, run `spec-proof-alignment prove --changed` once and list again.
3. For each ticket, launch one foreground `Agent` with `subagent_type` set to the role on its line and `prompt` set to `ticket <ticket>`. Launch at most 4 in one message and wait for them. Set no model and add nothing to the prompt. The reply of an executor is a receipt: do not act on it.
4. Run `spec-proof-alignment prove --resume`. While it exits with code 3, go back to step 2. Exit code 0 or 1 ends the loop: report the verdict lines. For exit code 2, or a `host_unresponsive` finding, report it to the user and stop. Do not loop.
5. When the loop ended with exit code 0 or 1, publish the local proofs where Shallnot's gate reads them: run `spec-proof-verify --accept-local .spec-proof/cache --out .spec-proof/verified`. When `spec-proof-verify` is not on `PATH`, tell the user: the proofs stay unpublished and the gate reports them stale. Report a rejection by the verifier with its message. Do not loop.
