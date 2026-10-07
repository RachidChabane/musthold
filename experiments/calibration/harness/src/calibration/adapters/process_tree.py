"""Process tables, process trees and ports: reading `ps`, spawning a server in the caller's group, ending a whole tree."""

from __future__ import annotations

import contextlib
import os
import re
import signal
import socket
import subprocess
from collections.abc import Callable, Mapping, Sequence
from dataclasses import dataclass, field
from pathlib import Path
from typing import Final, Protocol

from calibration.domain.ports import Clock, ProcessRunner, Sleeper

PS_ARGV: Final = ("ps", "-axo", "pid=,ppid=,pgid=,command=")
PS_TIMEOUT_SECONDS: Final = 30.0
TERMINATION_POLL_SECONDS: Final = 0.1
KILL_SETTLE_SECONDS: Final = 5.0
PORT_CONNECT_TIMEOUT_SECONDS: Final = 1.0
_PS_LINE: Final = re.compile(r"^\s*(\d+)\s+(\d+)\s+(\d+)(?:\s+(.*?))?\s*$")


@dataclass(frozen=True)
class ProcessEntry:
    pid: int
    ppid: int
    pgid: int
    command: str


class ProcessTableError(RuntimeError):
    """`ps` that did not list the processes."""


def parse_process_table(text: str) -> tuple[ProcessEntry, ...]:
    """The entries of `ps -axo pid=,ppid=,pgid=,command=`; a line that does not read as one is skipped."""
    entries: list[ProcessEntry] = []
    for line in text.splitlines():
        found = _PS_LINE.match(line)
        if found is not None:
            entries.append(ProcessEntry(int(found[1]), int(found[2]), int(found[3]), found[4] or ""))
    return tuple(entries)


def descendants_of(entries: Sequence[ProcessEntry], pid: int) -> frozenset[int]:
    """Every process below `pid`, however deep, without `pid` itself."""
    children: dict[int, list[int]] = {}
    for entry in entries:
        children.setdefault(entry.ppid, []).append(entry.pid)
    found: set[int] = set()
    pending = list(children.get(pid, ()))
    while pending:
        current = pending.pop()
        if current not in found and current != pid:
            found.add(current)
            pending.extend(children.get(current, ()))
    return frozenset(found)


def ancestors_of(entries: Sequence[ProcessEntry], pid: int) -> tuple[int, ...]:
    """The parents of `pid`, nearest first, up to the top of the table."""
    parent_of = {entry.pid: entry.ppid for entry in entries}
    chain: list[int] = []
    current = parent_of.get(pid)
    while current is not None and current not in chain and current != pid:
        chain.append(current)
        current = parent_of.get(current)
    return tuple(chain)


class ProcessTable:
    def __init__(self, runner: ProcessRunner) -> None:
        self._runner = runner

    def snapshot(self) -> tuple[ProcessEntry, ...]:
        result = self._runner.run(PS_ARGV, timeout_seconds=PS_TIMEOUT_SECONDS)
        if result.returncode != 0:
            raise ProcessTableError(f"ps exited with {result.returncode}")
        return parse_process_table(result.stdout.decode("utf-8", errors="replace"))


@dataclass(frozen=True)
class ServerCommand:
    """What starts a server: the command, the variables laid over the environment, those removed and the directory."""

    argv: tuple[str, ...]
    env: Mapping[str, str] = field(default_factory=dict[str, str])
    unset: tuple[str, ...] = ()
    cwd: Path | None = None


class ManagedProcess(Protocol):
    @property
    def pid(self) -> int: ...

    def poll(self) -> int | None: ...

    def terminate(self) -> None: ...

    def kill(self) -> None: ...

    def wait(self, timeout: float) -> int: ...


class PopenProcess:
    """A child that stays in the caller's process group, so that a stop of the group reaches it."""

    def __init__(self, process: subprocess.Popen[bytes]) -> None:
        self._process = process

    @property
    def pid(self) -> int:
        return self._process.pid

    def poll(self) -> int | None:
        return self._process.poll()

    def terminate(self) -> None:
        self._process.terminate()

    def kill(self) -> None:
        self._process.kill()

    def wait(self, timeout: float) -> int:
        return self._process.wait(timeout=timeout)


Spawn = Callable[[ServerCommand], ManagedProcess]


def spawn_process(command: ServerCommand) -> ManagedProcess:
    environment = dict(os.environ)
    environment.update(command.env)
    for name in command.unset:
        environment.pop(name, None)
    process = subprocess.Popen(
        list(command.argv),
        cwd=command.cwd,
        env=environment,
        stdin=subprocess.DEVNULL,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    return PopenProcess(process)


def is_alive(pid: int) -> bool:
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    return True


def _signal(pid: int, number: int) -> None:
    with contextlib.suppress(ProcessLookupError, PermissionError):
        os.kill(pid, number)


class ProcessTreeTerminator:
    """Ends a process and everything below it: SIGTERM to the tree, a grace period, then SIGKILL to what remains."""

    def __init__(self, table: ProcessTable, clock: Clock, sleeper: Sleeper, grace_seconds: float) -> None:
        self._table = table
        self._clock = clock
        self._sleeper = sleeper
        self._grace_seconds = grace_seconds

    def terminate(self, process: ManagedProcess) -> bool:
        """Whether the process and all of its descendants are gone at the end."""
        members = self._tree(process)
        self._signal_all(process, members, signal.SIGTERM)
        if self._wait_until_gone(process, members, self._grace_seconds):
            return True
        members = members | self._tree(process)
        self._signal_all(process, members, signal.SIGKILL)
        return self._wait_until_gone(process, members, KILL_SETTLE_SECONDS)

    def _tree(self, process: ManagedProcess) -> frozenset[int]:
        if process.poll() is not None:
            return frozenset()
        return descendants_of(self._table.snapshot(), process.pid)

    def _signal_all(self, process: ManagedProcess, members: frozenset[int], number: int) -> None:
        for pid in members:
            _signal(pid, number)
        if process.poll() is None:
            _signal(process.pid, number)

    def _wait_until_gone(self, process: ManagedProcess, members: frozenset[int], seconds: float) -> bool:
        deadline = self._clock.monotonic() + seconds
        while True:
            if process.poll() is not None and not any(is_alive(pid) for pid in members):
                return True
            if self._clock.monotonic() >= deadline:
                return False
            self._sleeper.sleep(TERMINATION_POLL_SECONDS)


def port_is_free(host: str, port: int) -> bool:
    """A TCP port is free when a connection to it is refused."""
    try:
        with socket.create_connection((host, port), timeout=PORT_CONNECT_TIMEOUT_SECONDS):
            return False
    except ConnectionRefusedError:
        return True
    except OSError:
        return False
