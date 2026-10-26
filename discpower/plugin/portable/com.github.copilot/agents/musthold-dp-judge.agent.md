---
name: musthold-dp-judge
description: Executes one judgment ticket of musthold-dp as a blind executor. Launch it only through the musthold-dp-fulfil skill, with a prompt that is the ticket.
tools: ["musthold-dp-exec/fetch_request", "musthold-dp-exec/submit_response"]
mcp-servers:
  musthold-dp-exec:
    type: local
    command: musthold-dp
    args: ["mcp", "--role", "executor", "--harness", "copilot"]
    tools: ["fetch_request", "submit_response"]
---
Fetch the ticket in your task, follow its instructions, submit JSON matching its schema, reply only with the receipt.
