---
name: spec-proof-alignment-fulfil
description: Answer the pending host requests of spec-proof-alignment (alignment) and finish the proof run. Use when `spec-proof-alignment prove` exits with code 3, when Shallnot reports awaiting_host for spec-proof-alignment, or when `spec-proof-alignment request list` shows tickets.
---

# Fulfil the requests of spec-proof-alignment

`spec-proof-alignment prove` stops with exit code 3 when it needs model work it cannot do itself. Each request is a ticket, and `spec-proof-alignment prove --resume` consumes the answers.

1. If `spec-proof-s1` is on `PATH` and `.spec-proof/config.yaml` sets `systemone.endpoint`, run `spec-proof-s1 drain --module alignment`. A failure leaves the requests for the next step.
2. Run `spec-proof-alignment request list`. Each line is `<ticket> <role>`. When it prints nothing, run `spec-proof-alignment prove --changed` once and list again.
3. For each ticket, answer it in one of two ways. When your client has a subagent named like the role on the ticket's line, with its colon replaced by a dash (`spec-proof-alignment:judge` is `spec-proof-alignment-judge`), ask it to handle `ticket <ticket>`, run at most 4 at a time, add nothing to the request, and do not act on its reply, a receipt. Otherwise answer inline through the `spec-proof-alignment` MCP server: call `fetch_request` with `ticket` set to the ticket and `cap` set to an empty string, follow the instructions it returns, then call `submit_response` with `ticket`, `nonce` from the fetch, `cap` set to an empty string and `output` set to a JSON object that matches the schema it returned. When `submit_response` returns `schemaErrors`, correct the output once. An answer given inline is an answer of the main agent: the module records it as inline, which has block power only.
4. Run `spec-proof-alignment prove --resume`. While it exits with code 3, go back to step 2. Exit code 0 or 1 ends the loop: report the verdict lines. For exit code 2, or a `host_unresponsive` finding, report it to the user and stop. Do not loop.
5. When the loop ended with exit code 0 or 1, publish the local proofs where Shallnot's gate reads them: run `spec-proof-verify --accept-local .spec-proof/cache --out .spec-proof/verified`. When `spec-proof-verify` is not on `PATH`, tell the user: the proofs stay unpublished and the gate reports them stale. Report a rejection by the verifier with its message. Do not loop.
