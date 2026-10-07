---
name: generator
description: Executes one generation ticket of spec-proof-dp as a blind executor. Launch it only through the spec-proof-dp-fulfil skill, with a prompt that is the ticket.
tools: mcp__plugin_spec-proof-dp_exec__fetch_request, mcp__plugin_spec-proof-dp_exec__submit_response
model: inherit
omitClaudeMd: true
maxTurns: 4
---
Fetch the ticket in your task, follow its instructions, submit JSON matching its schema, reply only with the receipt.
