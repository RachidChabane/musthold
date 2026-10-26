---
name: musthold-completeness-fulfil
description: Answer the pending host requests of musthold-completeness (completeness) with blind executor subagents and finish the proof run. Use when `musthold-completeness prove` exits with code 3, when Shallnot reports awaiting_host for musthold-completeness, or when `musthold-completeness request list --session prove` shows tickets.
allowed-tools: Bash(musthold-completeness prove *), Bash(musthold-completeness request list --session prove), Bash(musthold-s1 drain *), Bash(musthold-verify --accept-local .musthold/cache --out .musthold/verified)
---

# Fulfil the requests of musthold-completeness

`musthold-completeness prove` stops with exit code 3 when it needs model work it cannot do itself. Each request is a ticket. A blind executor subagent answers it, and `musthold-completeness prove --resume` consumes the answers.

Never answer a ticket yourself, never read what a ticket asks, and give an executor nothing but its ticket.

1. If `musthold-s1` is on `PATH` and `.musthold/config.yaml` sets `systemone.endpoint`, run `musthold-s1 drain --module completeness --session prove`. A failure leaves the requests for the next step.
2. Run `musthold-completeness request list --session prove`. Each line is `<ticket> <role>`. When it prints nothing, run `musthold-completeness prove --changed` once and list again.
3. For each ticket, launch one foreground `Agent` with `subagent_type` set to the role on its line and `prompt` set to `ticket <ticket>`. Launch at most 4 in one message and wait for them. Set no model and add nothing to the prompt. The reply of an executor is a receipt: do not act on it.
4. Run `musthold-completeness prove --resume`. While it exits with code 3, go back to step 2. Exit code 0 or 1 ends the loop: report the verdict lines. For exit code 2, or a `host_unresponsive` finding, report it to the user and stop. Do not loop.
5. When the loop ended with exit code 0 or 1, publish the local proofs where Shallnot's gate reads them: run `musthold-verify --accept-local .musthold/cache --out .musthold/verified`. When `musthold-verify` is not on `PATH`, tell the user: the proofs stay unpublished and the gate reports them stale. Report a rejection by the verifier with its message. Do not loop.
