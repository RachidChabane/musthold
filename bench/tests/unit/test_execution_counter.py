from __future__ import annotations

import json
from pathlib import Path

from specbench.adapters.execution_counter import ClaudeExecutionCounter, CodexExecutionCounter, execution_counter_for
from specbench.domain.arms import Harness

FIXTURES = Path(__file__).parent.parent / "fixtures" / "streams"


def agent_call(tool_use_id: str) -> str:
    block = {"type": "tool_use", "id": tool_use_id, "name": "Agent", "input": {"prompt": f"ticket {tool_use_id}"}}
    return json.dumps({"type": "assistant", "message": {"content": [block]}})


def read_lines(name: str) -> list[str]:
    return (FIXTURES / name).read_text().splitlines()


def test_a_claude_session_counts_once_plus_one_per_agent_tool_use() -> None:
    counter = ClaudeExecutionCounter()
    assert counter.count([agent_call("a"), agent_call("b"), agent_call("c")]) == 4
    assert counter.count(read_lines("claude_session.jsonl")) == 2


def test_a_tool_use_seen_twice_counts_once() -> None:
    assert ClaudeExecutionCounter().count([agent_call("a"), agent_call("a")]) == 2


def test_a_stream_cut_in_the_middle_of_its_last_line_counts_what_came_before() -> None:
    cut = agent_call("c")[:20]
    assert ClaudeExecutionCounter().count([agent_call("a"), agent_call("b"), cut]) == 3


def test_a_session_that_streamed_nothing_never_started() -> None:
    assert ClaudeExecutionCounter().count([]) == 0
    assert CodexExecutionCounter().count(["", "  "]) == 0


def test_a_codex_session_counts_once_plus_one_per_spawn_agent() -> None:
    assert CodexExecutionCounter().count(read_lines("codex_exec.jsonl")) == 2


def test_the_counter_follows_the_harness() -> None:
    assert isinstance(execution_counter_for(Harness.CLAUDE_CODE), ClaudeExecutionCounter)
    assert isinstance(execution_counter_for(Harness.CODEX), CodexExecutionCounter)
