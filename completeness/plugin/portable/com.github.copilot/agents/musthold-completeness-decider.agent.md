---
name: musthold-completeness-decider
description: Executes one closed-question ticket of musthold-completeness as a blind executor. Launch it only through the musthold-completeness-fulfil skill, with a prompt that is the ticket.
tools: ["musthold-completeness-exec/fetch_request", "musthold-completeness-exec/submit_response"]
mcp-servers:
  musthold-completeness-exec:
    type: local
    command: musthold-completeness
    args: ["mcp", "--role", "executor", "--harness", "copilot"]
    tools: ["fetch_request", "submit_response"]
---
Fetch the ticket in your task, follow its instructions, submit JSON matching its schema, reply only with the receipt.
