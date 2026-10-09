"""Runs one gate-host session and keeps what it streamed: every line goes to the call's events file, and a session whose
`system/init` lacks a plugin or an executor server is stopped as misconfigured (ADR 0011, decision 1)."""

from __future__ import annotations

import json
from collections.abc import Mapping
from dataclasses import dataclass
from pathlib import Path
from typing import Final, cast

from specbench.adapters.claude_stream import check_claude_harness, parse_claude_stream
from specbench.adapters.execution_counter import intact_stream
from specbench.application.ports import GateSessionRunner, GateSessionSpec
from specbench.domain.agent_events import HarnessOutcome
from specbench.domain.gate import GateStatus

EVENT_TYPE: Final = "type"
EVENT_SUBTYPE: Final = "subtype"
SYSTEM_EVENT: Final = "system"
INIT_SUBTYPE: Final = "init"


@dataclass(frozen=True)
class LiveRun:
    """How a session ended: its status (`COMPLETE` when the step that follows it may run), id and streamed lines."""

    status: GateStatus
    session_id: str | None
    lines: tuple[str, ...]


class _EventRecorder:
    """Appends every streamed line to the call's events file and stops the session whose init lacks a plugin or server."""

    def __init__(self, events_file: Path, plugins: set[str], servers: set[str]) -> None:
        events_file.parent.mkdir(parents=True, exist_ok=True)
        self._handle = events_file.open("a", encoding="utf-8")
        self._plugins = plugins
        self._servers = servers
        self.lines: list[str] = []
        self.init_seen = False
        self.misconfigured = False

    def accept(self, line: str) -> bool:
        self._handle.write(line + "\n")
        self._handle.flush()
        self.lines.append(line)
        if self.init_seen or not _is_init(line):
            return True
        self.init_seen = True
        check = check_claude_harness(parse_claude_stream(line).init, self._plugins, self._servers)
        self.misconfigured = check.outcome is HarnessOutcome.HARNESS_MISCONFIGURED
        return not self.misconfigured

    def close(self) -> None:
        self._handle.close()


def _is_init(line: str) -> bool:
    try:
        body: object = json.loads(line)
    except json.JSONDecodeError:
        return False
    if not isinstance(body, Mapping):
        return False
    fields = cast("Mapping[str, object]", body)
    return fields.get(EVENT_TYPE) == SYSTEM_EVENT and fields.get(EVENT_SUBTYPE) == INIT_SUBTYPE


class GateSessionDriver:
    """Starts a session through the runner and reads how it went from the events it streamed."""

    def __init__(self, sessions: GateSessionRunner) -> None:
        self._sessions = sessions

    def run(self, spec: GateSessionSpec, events_file: Path, plugins: set[str], servers: set[str]) -> LiveRun:
        recorder = _EventRecorder(events_file, plugins, servers)
        try:
            end = self._sessions.run(spec, recorder.accept)
        finally:
            recorder.close()
        stream = parse_claude_stream(intact_stream(recorder.lines))
        session_id = stream.init.session_id if stream.init else None
        lines = tuple(recorder.lines)
        if recorder.misconfigured or not recorder.init_seen:
            return LiveRun(GateStatus.HARNESS_MISCONFIGURED, session_id, lines)
        finished = end.exit_code == 0 and stream.result is not None and not stream.result.is_error
        return LiveRun(GateStatus.COMPLETE if finished else GateStatus.PAUSED, session_id, lines)
