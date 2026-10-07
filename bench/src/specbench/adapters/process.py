"""Process execution: plain, and refusing to start below the free-memory floor."""

from __future__ import annotations

import os
import re
import subprocess
import sys
from collections.abc import Callable, Mapping, Sequence
from pathlib import Path
from typing import Final

from specbench.application.ports import ProcessResult, ProcessRunner

MIN_FREE_PERCENT: Final = 40
MIN_FREE_PERCENT_ENV: Final = "MEM_GUARD_MIN_FREE_PCT"
FREE_PERCENT_LINE: Final = re.compile(r"System-wide memory free percentage:\s*(\d+)%")
DEFAULT_TIMEOUT_SECONDS: Final = 1800
MODEL_TIMEOUT_SECONDS: Final = 1200


class MachineUnsafe(RuntimeError):
    """Memory is short on the owner's laptop: the command is not started."""


class SubprocessRunner:
    def __init__(self, timeout_seconds: int = DEFAULT_TIMEOUT_SECONDS) -> None:
        self._timeout = timeout_seconds

    def run(self, argv: Sequence[str], cwd: Path | None = None, env: Mapping[str, str] | None = None) -> ProcessResult:
        completed = subprocess.run(
            list(argv),
            cwd=cwd,
            env={**os.environ, **(env or {})},
            capture_output=True,
            text=True,
            timeout=self._timeout,
            check=False,
        )
        return ProcessResult(completed.returncode, completed.stdout, completed.stderr)


class SubprocessPromptRunner:
    """Runs a command with a prompt on standard input; the memory floor applies like for every other command."""

    def __init__(
        self,
        free_percent: Callable[[], int],
        timeout_seconds: int = MODEL_TIMEOUT_SECONDS,
        minimum_percent: int | None = None,
    ) -> None:
        self._free_percent = free_percent
        self._timeout = timeout_seconds
        self._minimum = minimum_percent if minimum_percent is not None else min_free_percent()

    def run_with_input(
        self, argv: Sequence[str], stdin: str, cwd: Path, env: Mapping[str, str] | None = None
    ) -> ProcessResult:
        free = self._free_percent()
        if free < self._minimum:
            raise MachineUnsafe(f"free memory is {free} %, below the {self._minimum} % floor")
        completed = subprocess.run(
            list(argv),
            input=stdin,
            cwd=cwd,
            env=dict(env) if env is not None else None,
            capture_output=True,
            text=True,
            timeout=self._timeout,
            check=False,
        )
        return ProcessResult(completed.returncode, completed.stdout, completed.stderr)


def min_free_percent() -> int:
    """The free-memory floor `script/mem-guard` uses: `MEM_GUARD_MIN_FREE_PCT`, 40 by default."""
    return int(os.environ.get(MIN_FREE_PERCENT_ENV, MIN_FREE_PERCENT))


class MemoryFloorRunner:
    """Decorator: refuses to start a command while free memory is below the floor.

    The whole verb already runs under `script/mem-guard`, which applies nice and stops
    the batch when memory pressure leaves normal; this check refuses each new command
    between the guard's polls.
    """

    def __init__(
        self, inner: ProcessRunner, free_percent: Callable[[], int], minimum_percent: int | None = None
    ) -> None:
        self._inner = inner
        self._free_percent = free_percent
        self._minimum = minimum_percent if minimum_percent is not None else min_free_percent()

    def run(self, argv: Sequence[str], cwd: Path | None = None, env: Mapping[str, str] | None = None) -> ProcessResult:
        free = self._free_percent()
        if free < self._minimum:
            raise MachineUnsafe(f"free memory is {free} %, below the {self._minimum} % floor")
        return self._inner.run(argv, cwd, env)


def system_free_percent() -> int:
    """System-wide free memory percentage: `memory_pressure` on macOS, `/proc/meminfo` elsewhere."""
    if sys.platform == "darwin":
        output = subprocess.run(["memory_pressure"], capture_output=True, text=True, check=False).stdout
        match = FREE_PERCENT_LINE.search(output)
        if match is None:
            raise MachineUnsafe("memory_pressure printed no free percentage")
        return int(match.group(1))
    fields = dict(line.split(":", 1) for line in Path("/proc/meminfo").read_text().splitlines() if ":" in line)
    total, available = (int(fields[key].split()[0]) for key in ("MemTotal", "MemAvailable"))
    return 100 * available // total
