---
name: spec-proof-dp-generator
description: Executes one generation ticket of spec-proof-dp as a blind executor. Launch it only through the spec-proof-dp-fulfil skill, with a prompt that is the ticket.
tools: ["spec-proof-dp-exec/fetch_request", "spec-proof-dp-exec/submit_response"]
mcp-servers:
  spec-proof-dp-exec:
    type: local
    command: spec-proof-dp
    args: ["mcp", "--role", "executor", "--harness", "copilot"]
    tools: ["fetch_request", "submit_response"]
---
Fetch the ticket in your task, follow its instructions, submit JSON matching its schema, reply only with the receipt.
