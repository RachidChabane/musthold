from __future__ import annotations

import json
from pathlib import Path

from specbench.adapters.gate_session_stream import ticket_launches
from specbench.domain.agent_events import TokenUsage

CANARY_STREAM = Path(__file__).parent.parent / "fixtures" / "streams" / "claude_session.jsonl"
TICKET = "864f5e696da12d6944d9380733d5c5a9fba0f0140f3b243c3efae9a6cc74cb38.0.1"


def event(body: dict[str, object]) -> str:
    return json.dumps(body)


def launch(tool_use_id: str, prompt: str) -> str:
    block = {"type": "tool_use", "id": tool_use_id, "name": "Agent", "input": {"prompt": prompt}}
    return event({"type": "assistant", "message": {"content": [block]}, "parent_tool_use_id": None})


def subagent_message(parent: str, tokens: tuple[int, int], model: str | None = "claude-sonnet-5-5") -> str:
    usage = {"input_tokens": tokens[0], "output_tokens": tokens[1], "cache_read_input_tokens": 1}
    message = {"content": [{"type": "text", "text": "r"}], "usage": usage, "model": model}
    return event({"type": "assistant", "message": message, "parent_tool_use_id": parent})


def test_a_launch_is_joined_to_its_ticket_and_to_the_messages_of_its_subagent() -> None:
    stream = "\n".join(
        [
            launch("toolu_1", f"ticket {TICKET}"),
            subagent_message("toolu_1", (10, 4)),
            subagent_message("toolu_1", (7, 2)),
        ]
    )
    [found] = ticket_launches(stream)
    assert (found.tool_use_id, found.ticket) == ("toolu_1", TICKET)
    assert found.tokens == TokenUsage(input_tokens=17, output_tokens=6, cache_read_tokens=2)
    assert found.resolved_model == "claude-sonnet-5-5"


def test_two_launches_keep_their_own_costs() -> None:
    other = TICKET.replace("864f", "964f")
    stream = "\n".join(
        [
            launch("toolu_1", f"ticket {TICKET}"),
            launch("toolu_2", f"ticket {other}"),
            subagent_message("toolu_2", (5, 5)),
            subagent_message("toolu_1", (1, 1), model=None),
        ]
    )
    first, second = ticket_launches(stream)
    assert first.tokens == TokenUsage(input_tokens=1, output_tokens=1, cache_read_tokens=1)
    assert first.resolved_model is None
    assert second.tokens.input_tokens == 5


def test_an_agent_launch_without_a_ticket_and_other_tools_are_not_tickets() -> None:
    other_tool = {"type": "tool_use", "id": "toolu_9", "name": "Edit", "input": {"prompt": f"ticket {TICKET}"}}
    stream = "\n".join(
        [
            launch("toolu_1", "no ticket in this prompt"),
            event({"type": "assistant", "message": {"content": [other_tool]}, "parent_tool_use_id": None}),
        ]
    )
    assert ticket_launches(stream) == ()


def test_messages_of_the_main_session_are_not_attributed_to_a_ticket() -> None:
    stream = "\n".join([launch("toolu_1", f"ticket {TICKET}"), subagent_message("toolu_other", (9, 9))])
    [found] = ticket_launches(stream)
    assert found.tokens == TokenUsage()


def test_the_launch_of_the_recorded_canary_session_is_joined_to_its_padded_ticket() -> None:
    [found] = ticket_launches(CANARY_STREAM.read_text())
    assert (found.tool_use_id, found.ticket) == ("toolu_01Agent", TICKET)
    assert found.resolved_model == "claude-sonnet-5-5"
