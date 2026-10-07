"""The life of one server process: refuse a busy port, spawn, wait until it answers, end the whole tree."""

from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass
from typing import Final

from calibration.adapters.process_tree import (
    ManagedProcess,
    ProcessTreeTerminator,
    ServerCommand,
    Spawn,
    port_is_free,
)
from calibration.adapters.server_http import ServerStartError
from calibration.domain.ports import Clock, Sleeper

READY_POLL_SECONDS: Final = 0.1
PORT_RELEASE_SECONDS: Final = 5.0


@dataclass(frozen=True)
class ProcessStop:
    """What ending a server found: whether its whole tree is gone and whether its port is free."""

    stopped: bool
    port_free: bool


class ServerProcess:
    def __init__(
        self,
        *,
        spawn: Spawn,
        terminator: ProcessTreeTerminator,
        clock: Clock,
        sleeper: Sleeper,
        host: str,
        port: int,
        start_timeout_seconds: float,
    ) -> None:
        self._spawn = spawn
        self._terminator = terminator
        self._clock = clock
        self._sleeper = sleeper
        self._host = host
        self._port = port
        self._start_timeout_seconds = start_timeout_seconds
        self._process: ManagedProcess | None = None

    @property
    def is_running(self) -> bool:
        return self._process is not None and self._process.poll() is None

    def launch(self, command: ServerCommand) -> None:
        if self._process is not None:
            raise ServerStartError("the server was started already")
        if not port_is_free(self._host, self._port):
            raise ServerStartError(f"port {self._port} on {self._host} is not free")
        self._process = self._spawn(command)

    def wait_until_ready(self, is_ready: Callable[[], bool], description: str) -> None:
        process = self._process
        if process is None:
            raise ServerStartError("the server was not launched")
        deadline = self._clock.monotonic() + self._start_timeout_seconds
        while True:
            exit_code = process.poll()
            if exit_code is not None:
                raise ServerStartError(f"{description}: the process exited with {exit_code} before it answered")
            if is_ready():
                return
            if self._clock.monotonic() >= deadline:
                raise ServerStartError(f"{description}: no answer within {self._start_timeout_seconds} s")
            self._sleeper.sleep(READY_POLL_SECONDS)

    def stop(self) -> ProcessStop:
        process, self._process = self._process, None
        stopped = True if process is None else self._terminator.terminate(process)
        return ProcessStop(stopped, self._wait_until_port_free())

    def _wait_until_port_free(self) -> bool:
        deadline = self._clock.monotonic() + PORT_RELEASE_SECONDS
        while not port_is_free(self._host, self._port):
            if self._clock.monotonic() >= deadline:
                return False
            self._sleeper.sleep(READY_POLL_SECONDS)
        return True
