"""What a gate-host session's stream says about its tickets: which executor launch answered which ticket, at what cost.

An executor launch is an `Agent` tool use whose prompt holds one ticket (the product's hook rewrites the prompt to
`ticket <t>`). The launch's subagent messages carry `parent_tool_use_id` equal to the launch's tool-use id, and their
`message.usage` and `message.model` give the tokens and the model the ticket cost.
"""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass
from typing import Final

from specbench.adapters.event_lines import (
    EventLine,
    count_field,
    decode_event_lines,
    optional_mapping,
    optional_sequence,
)
from specbench.domain.agent_events import TokenUsage
from specbench.domain.host_requests import ticket_in_prompt
from specbench.domain.jsonread import mapping, optional_text

STREAM_NAME: Final = "gate session stream"
AGENT_TOOL: Final = "Agent"
BLOCK_TOOL_USE: Final = "tool_use"
TYPE_ASSISTANT: Final = "assistant"


@dataclass(frozen=True)
class TicketLaunch:
    """One executor launch: the ticket it was given, the tokens its messages spent and the model that served them."""

    tool_use_id: str
    ticket: str
    tokens: TokenUsage
    resolved_model: str | None


def _launches_of(event: EventLine) -> list[tuple[str, str]]:
    where = f"{STREAM_NAME} line {event.line}"
    message = optional_mapping(event.body, "message", where)
    found: list[tuple[str, str]] = []
    for item in optional_sequence(message, "content", where):
        block = mapping(item, f"{where} content block")
        if block.get("type") != BLOCK_TOOL_USE or block.get("name") != AGENT_TOOL:
            continue
        prompt = optional_text(optional_mapping(block, "input", where).get("prompt"), where) or ""
        tool_use_id, ticket = optional_text(block.get("id"), where), ticket_in_prompt(prompt)
        if tool_use_id is not None and ticket is not None:
            found.append((tool_use_id, ticket))
    return found


def _usage_of(message: Mapping[str, object]) -> TokenUsage:
    usage = optional_mapping(message, "usage", STREAM_NAME)
    return TokenUsage(
        count_field(usage, "input_tokens"),
        count_field(usage, "output_tokens"),
        count_field(usage, "cache_read_input_tokens"),
        count_field(usage, "cache_creation_input_tokens"),
    )


def ticket_launches(stream: str) -> tuple[TicketLaunch, ...]:
    """The executor launches of the stream in the order they were made, each with the cost its subagent messages show."""
    events = decode_event_lines(stream, STREAM_NAME)
    launched: dict[str, str] = {}
    for event in events:
        if event.body.get("type") == TYPE_ASSISTANT:
            launched.update(_launches_of(event))
    tokens: dict[str, TokenUsage] = {}
    models: dict[str, str] = {}
    for event in events:
        parent = event.body.get("parent_tool_use_id")
        if event.body.get("type") != TYPE_ASSISTANT or not isinstance(parent, str) or parent not in launched:
            continue
        message = optional_mapping(event.body, "message", STREAM_NAME)
        tokens[parent] = tokens.get(parent, TokenUsage()) + _usage_of(message)
        model = optional_text(message.get("model"), STREAM_NAME)
        if model is not None:
            models[parent] = model
    return tuple(
        TicketLaunch(tool_use_id, ticket, tokens.get(tool_use_id, TokenUsage()), models.get(tool_use_id))
        for tool_use_id, ticket in launched.items()
    )
