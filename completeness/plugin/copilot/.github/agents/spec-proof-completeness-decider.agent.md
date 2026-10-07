---
name: spec-proof-completeness-decider
description: Executes one closed-question ticket of spec-proof-completeness as a blind executor. Launch it only through the spec-proof-completeness-fulfil skill, with a prompt that is the ticket.
tools: ["spec-proof-completeness-exec/fetch_request", "spec-proof-completeness-exec/submit_response"]
mcp-servers:
  spec-proof-completeness-exec:
    type: local
    command: spec-proof-completeness
    args: ["mcp", "--role", "executor", "--harness", "copilot"]
    tools: ["fetch_request", "submit_response"]
---
Fetch the ticket in your task, follow its instructions, submit JSON matching its schema, reply only with the receipt.
