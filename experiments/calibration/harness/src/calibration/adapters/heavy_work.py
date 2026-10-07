"""`HeavyWorkProbe` of §6.1: foreign model servers, other guarded jobs and a model loaded in the installed Ollama."""

from __future__ import annotations

import json
import os
from collections.abc import Sequence
from typing import Final

from calibration.adapters.ollama_server import listed_model_names
from calibration.adapters.process_tree import (
    ProcessEntry,
    ProcessTable,
    ancestors_of,
    descendants_of,
)
from calibration.adapters.server_http import ServerError
from calibration.application.ports import HeavyWorkReport
from calibration.domain.canonical import JsonValue
from calibration.domain.constants import HEAVY_PROCESS_PATTERNS, HTTP_OK, LOOPBACK, OLLAMA_INSTALLED_PORT
from calibration.domain.ports import HttpTransport, ProcessRunner, TransportFailure

GUARD_PATTERN: Final = "script/mem-guard run"
PS_PATH: Final = "/api/ps"
INSTALLED_OLLAMA_URL: Final = f"http://{LOOPBACK}:{OLLAMA_INSTALLED_PORT}"
INSTALLED_OLLAMA_TIMEOUT_SECONDS: Final = 5.0
COMMAND_DISPLAY_LIMIT: Final = 120


def _describe(entry: ProcessEntry) -> str:
    return f"{entry.command[:COMMAND_DISPLAY_LIMIT]} (pid {entry.pid})"


def _is_heavy(entry: ProcessEntry) -> bool:
    return any(pattern in entry.command for pattern in HEAVY_PROCESS_PATTERNS)


def _is_guard(entry: ProcessEntry) -> bool:
    return GUARD_PATTERN in entry.command


def _own_processes(entries: Sequence[ProcessEntry], owner_pid: int) -> frozenset[int]:
    """The owner, its ancestors and descendants, and the descendants of the guard ancestors (the guard's subshells)."""
    ancestors = ancestors_of(entries, owner_pid)
    by_pid = {entry.pid: entry for entry in entries}
    own = {owner_pid, *ancestors, *descendants_of(entries, owner_pid)}
    for pid in ancestors:
        entry = by_pid.get(pid)
        if entry is not None and _is_guard(entry):
            own |= descendants_of(entries, pid)
    return frozenset(own)


def foreign_heavy_processes(entries: Sequence[ProcessEntry], owner_pid: int) -> tuple[str, ...]:
    own = _own_processes(entries, owner_pid)
    return tuple(_describe(entry) for entry in entries if entry.pid not in own and _is_heavy(entry))


def other_guard_processes(entries: Sequence[ProcessEntry], owner_pid: int) -> tuple[str, ...]:
    own = _own_processes(entries, owner_pid)
    return tuple(_describe(entry) for entry in entries if entry.pid not in own and _is_guard(entry))


class SystemHeavyWorkProbe:
    def __init__(
        self,
        runner: ProcessRunner,
        transport: HttpTransport,
        *,
        owner_pid: int | None = None,
        installed_ollama_url: str = INSTALLED_OLLAMA_URL,
    ) -> None:
        self._table = ProcessTable(runner)
        self._transport = transport
        self._owner_pid = os.getpid() if owner_pid is None else owner_pid
        self._installed_ollama_url = installed_ollama_url

    def check(self) -> HeavyWorkReport:
        entries = self._table.snapshot()
        return HeavyWorkReport(
            foreign_processes=foreign_heavy_processes(entries, self._owner_pid),
            installed_ollama_models=self._installed_models(),
            other_guard_processes=other_guard_processes(entries, self._owner_pid),
        )

    def _installed_models(self) -> tuple[str, ...]:
        try:
            reply = self._transport.get(self._installed_ollama_url + PS_PATH, INSTALLED_OLLAMA_TIMEOUT_SECONDS)
        except TransportFailure:
            return ()
        if reply.status != HTTP_OK:
            return ()
        return _model_names(reply.body)


def _model_names(body: bytes) -> tuple[str, ...]:
    try:
        document: JsonValue = json.loads(body)
        return listed_model_names(document, PS_PATH)
    except (ValueError, ServerError):
        return ()
