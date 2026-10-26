---
name: musthold-completeness-fulfil
description: Answer the pending host requests of musthold-completeness (completeness) and finish the proof run. Use when `musthold-completeness prove` exits with code 3, when Shallnot reports awaiting_host for musthold-completeness, or when `musthold-completeness request list --session prove` shows tickets.
---

# Fulfil the requests of musthold-completeness

`musthold-completeness prove` stops with exit code 3 when it needs model work it cannot do itself. Each request is a ticket, and `musthold-completeness prove --resume` consumes the answers.

1. If `musthold-s1` is on `PATH` and `.musthold/config.yaml` sets `systemone.endpoint`, run `musthold-s1 drain --module completeness --session prove`. A failure leaves the requests for the next step.
2. Run `musthold-completeness request list --session prove`. Each line is `<ticket> <role>`. When it prints nothing, run `musthold-completeness prove --changed` once and list again.
3. For each ticket, answer it in one of two ways. When your client can start a custom agent named like the role on the ticket's line, with its colon replaced by a dash (`musthold-completeness:decider` is `musthold-completeness-decider`), start it through the client's subagent tool (for GitHub Copilot CLI, the `task` tool with `agent_type` set to the agent's name) with `ticket <ticket>` as the whole prompt and no conversation history, run at most 4 at a time, and do not act on its reply, a receipt. Otherwise answer inline through the `musthold-completeness` MCP server: call `fetch_request` with `ticket` set to the ticket and `cap` set to an empty string, follow the instructions it returns, then call `submit_response` with `ticket`, `nonce` from the fetch, `cap` set to an empty string and `output` set to a JSON object that matches the schema it returned. When `submit_response` returns `schemaErrors`, correct the output once. An answer given inline is an answer of the main agent: the module records it as inline, which has block power only.
4. Run `musthold-completeness prove --resume`. While it exits with code 3, go back to step 2. Exit code 0 or 1 ends the loop: report the verdict lines. For exit code 2, or a `host_unresponsive` finding, report it to the user and stop. Do not loop.
5. When the loop ended with exit code 0 or 1, publish the local proofs where Shallnot's gate reads them: run `musthold-verify --accept-local .musthold/cache --out .musthold/verified`. When `musthold-verify` is not on `PATH`, tell the user: the proofs stay unpublished and the gate reports them stale. Report a rejection by the verifier with its message. Do not loop.
