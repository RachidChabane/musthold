"""Subprocess execution behind a small port-like interface (adapter-internal)."""

from __future__ import annotations

import os
import subprocess
import threading
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from pathlib import Path
from typing import IO, Protocol

from pilot.domain.constants import LOCAL_SUBPROCESS_LIMIT

DEADLINE_RETURNCODE = -1
_NESTED_PREFIXES = ("PYTEST_", "PYTHONPATH")


@dataclass(frozen=True)
class ProcessResult:
    returncode: int
    stdout: str
    stderr: str
    deadline_exceeded: bool = False


class ProcessRunner(Protocol):
    def run(
        self,
        argv: Sequence[str],
        cwd: Path,
        *,
        stdin_path: Path | None = None,
        env: Mapping[str, str] | None = None,
        deadline_seconds: float | None = None,
    ) -> ProcessResult: ...


class SubprocessRunner:
    """Runs a real subprocess, capturing text output, with an optional wall-clock deadline."""

    def run(
        self,
        argv: Sequence[str],
        cwd: Path,
        *,
        stdin_path: Path | None = None,
        env: Mapping[str, str] | None = None,
        deadline_seconds: float | None = None,
    ) -> ProcessResult:
        if stdin_path is None:
            return self._execute(argv, cwd, subprocess.DEVNULL, env, deadline_seconds)
        with stdin_path.open("rb") as handle:
            return self._execute(argv, cwd, handle, env, deadline_seconds)

    @staticmethod
    def _execute(
        argv: Sequence[str],
        cwd: Path,
        stdin: int | IO[bytes],
        env: Mapping[str, str] | None,
        deadline_seconds: float | None,
    ) -> ProcessResult:
        try:
            completed = subprocess.run(
                list(argv),
                cwd=cwd,
                stdin=stdin,
                capture_output=True,
                env=dict(env) if env is not None else None,
                timeout=deadline_seconds,
                check=False,
            )
        except subprocess.TimeoutExpired as expired:
            return ProcessResult(
                DEADLINE_RETURNCODE, _text(expired.stdout), _text(expired.stderr), True
            )
        return ProcessResult(
            completed.returncode, _text(completed.stdout), _text(completed.stderr)
        )


LOCAL_SUBPROCESS_GATE = threading.BoundedSemaphore(LOCAL_SUBPROCESS_LIMIT)


class GatedProcessRunner:
    """Decorator: at most LOCAL_SUBPROCESS_LIMIT local test, witness or oracle subprocesses run at once.

    The gate is process-wide and held only for the lifetime of one subprocess, so a caller that
    fans work out over threads can never deadlock on it.
    """

    def __init__(self, inner: ProcessRunner, gate: threading.BoundedSemaphore = LOCAL_SUBPROCESS_GATE) -> None:
        self._inner = inner
        self._gate = gate

    def run(
        self,
        argv: Sequence[str],
        cwd: Path,
        *,
        stdin_path: Path | None = None,
        env: Mapping[str, str] | None = None,
        deadline_seconds: float | None = None,
    ) -> ProcessResult:
        with self._gate:
            return self._inner.run(argv, cwd, stdin_path=stdin_path, env=env, deadline_seconds=deadline_seconds)


def local_process_runner() -> GatedProcessRunner:
    return GatedProcessRunner(SubprocessRunner())


def _text(data: bytes | str | None) -> str:
    if data is None:
        return ""
    if isinstance(data, str):
        return data
    return data.decode("utf-8", errors="replace")


def child_environment() -> dict[str, str]:
    """Environment for a pytest child: no inherited pytest or path overrides, no bytecode."""
    inherited = {
        key: value
        for key, value in os.environ.items()
        if not key.startswith(_NESTED_PREFIXES)
    }
    return {**inherited, "PYTHONDONTWRITEBYTECODE": "1"}
