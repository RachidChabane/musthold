"""A process whose standard output is handed over line by line as it arrives, killed as a group at its deadline."""

from __future__ import annotations

import os
import signal
import subprocess
import threading
from collections.abc import Callable, Mapping, Sequence
from dataclasses import dataclass
from pathlib import Path
from typing import Final

from specbench.adapters.process import MachineUnsafe, min_free_percent, system_free_percent

ENCODING: Final = "utf-8"
SESSION_TIMEOUT_SECONDS: Final = 3600


@dataclass(frozen=True)
class StreamedProcess:
    """`returncode` is negative when a signal ended the process; `timed_out` says the deadline did."""

    returncode: int
    stderr: str
    timed_out: bool


class StreamingRunner:
    """Runs one command with a prompt on standard input and passes every standard-output line to `on_line`.

    The command leads its own process group, so the deadline takes down everything it started. The memory floor
    applies like for every other command of the bench.
    """

    def __init__(
        self,
        free_percent: Callable[[], int] = system_free_percent,
        timeout_seconds: int = SESSION_TIMEOUT_SECONDS,
        minimum_percent: int | None = None,
    ) -> None:
        self._free_percent = free_percent
        self._timeout = timeout_seconds
        self._minimum = minimum_percent if minimum_percent is not None else min_free_percent()

    def run(
        self,
        argv: Sequence[str],
        stdin: str,
        cwd: Path,
        env: Mapping[str, str],
        on_line: Callable[[str], None],
    ) -> StreamedProcess:
        free = self._free_percent()
        if free < self._minimum:
            raise MachineUnsafe(f"free memory is {free} %, below the {self._minimum} % floor")
        process = subprocess.Popen(
            list(argv),
            cwd=cwd,
            env=dict(env),
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            encoding=ENCODING,
            start_new_session=True,
        )
        expired = threading.Event()
        deadline = threading.Timer(self._timeout, _expire, args=(process, expired))
        stderr_lines: list[str] = []
        feeder = threading.Thread(target=_feed, args=(process, stdin), daemon=True)
        drainer = threading.Thread(target=_drain, args=(process, stderr_lines), daemon=True)
        deadline.start()
        feeder.start()
        drainer.start()
        try:
            assert process.stdout is not None
            for line in process.stdout:
                on_line(line.rstrip("\n"))
        except BaseException:
            _kill_group(process)
            raise
        finally:
            returncode = process.wait()
            deadline.cancel()
            feeder.join()
            drainer.join()
        return StreamedProcess(returncode, "".join(stderr_lines), timed_out=expired.is_set())


def _feed(process: subprocess.Popen[str], stdin: str) -> None:
    assert process.stdin is not None
    try:
        process.stdin.write(stdin)
        process.stdin.close()
    except (BrokenPipeError, ValueError):
        return


def _drain(process: subprocess.Popen[str], sink: list[str]) -> None:
    assert process.stderr is not None
    sink.extend(process.stderr)


def _expire(process: subprocess.Popen[str], expired: threading.Event) -> None:
    expired.set()
    _kill_group(process)


def _kill_group(process: subprocess.Popen[str]) -> None:
    try:
        os.killpg(process.pid, signal.SIGKILL)
    except ProcessLookupError:
        return
