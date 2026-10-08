"""Counts the model executions a coding agent's stream shows (`ExecutionCounter`).

An execution is the session itself plus every subagent it started: an `Agent` tool use on Claude, a `spawn_agent` call
on Codex. A session killed in mid-write leaves a truncated last line, which is ignored; a stream with no event at all
means the session never started and counts as nothing.
"""

from __future__ import annotations

import json
from collections.abc import Sequence
from typing import Final

from specbench.adapters.claude_stream import parse_claude_stream
from specbench.adapters.codex_stream import parse_codex_exec
from specbench.domain.arms import Harness

SESSION_EXECUTIONS: Final = 1
CLAUDE_SUBAGENT_TOOL: Final = "Agent"


def intact_stream(events: Sequence[str]) -> str:
    """The events as one stream, without a last line that a killed process left half written."""
    lines = [line for line in events if line.strip()]
    if lines:
        try:
            json.loads(lines[-1])
        except json.JSONDecodeError:
            lines.pop()
    return "\n".join(lines)


class ClaudeExecutionCounter:
    def count(self, events: Sequence[str]) -> int:
        stream = intact_stream(events)
        if not stream:
            return 0
        started = {
            action.pointer.call_id or f"line-{action.pointer.line}"
            for action in parse_claude_stream(stream).actions
            if action.tool == CLAUDE_SUBAGENT_TOOL
        }
        return SESSION_EXECUTIONS + len(started)


class CodexExecutionCounter:
    def count(self, events: Sequence[str]) -> int:
        stream = intact_stream(events)
        if not stream:
            return 0
        return SESSION_EXECUTIONS + len(parse_codex_exec(stream).spawns)


def execution_counter_for(harness: Harness) -> ClaudeExecutionCounter | CodexExecutionCounter:
    return ClaudeExecutionCounter() if harness is Harness.CLAUDE_CODE else CodexExecutionCounter()
