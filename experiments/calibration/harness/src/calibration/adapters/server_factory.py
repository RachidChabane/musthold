"""`ServerFactory`: the Ollama or the Kev server of a model, by its flavour."""

from __future__ import annotations

from pathlib import Path

from calibration.adapters.kev_server import DEFAULT_UV, KevServer
from calibration.adapters.ollama_server import DEFAULT_BINARY, OllamaServer
from calibration.adapters.process_tree import Spawn, spawn_process
from calibration.application.ports import ModelServer
from calibration.domain.constants import (
    KEV_PORT,
    LOOPBACK,
    OLLAMA_PORT,
    SERVER_START_TIMEOUT_SECONDS,
    SERVER_STOP_GRACE_SECONDS,
)
from calibration.domain.models import ModelSpec
from calibration.domain.ports import Clock, HttpTransport, ProcessRunner, Sleeper


class ExperimentServerFactory:
    def __init__(
        self,
        transport: HttpTransport,
        *,
        kev_directory: Path,
        kev_commit: str,
        spawn: Spawn = spawn_process,
        clock: Clock | None = None,
        sleeper: Sleeper | None = None,
        host: str = LOOPBACK,
        ollama_port: int = OLLAMA_PORT,
        kev_port: int = KEV_PORT,
        ollama_binary: str = DEFAULT_BINARY,
        uv_binary: str = DEFAULT_UV,
        start_timeout_seconds: float = SERVER_START_TIMEOUT_SECONDS,
        stop_grace_seconds: float = SERVER_STOP_GRACE_SECONDS,
        process_runner: ProcessRunner | None = None,
    ) -> None:
        self._transport = transport
        self._kev_directory = kev_directory
        self._kev_commit = kev_commit
        self._spawn = spawn
        self._clock = clock
        self._sleeper = sleeper
        self._host = host
        self._ollama_port = ollama_port
        self._kev_port = kev_port
        self._ollama_binary = ollama_binary
        self._uv_binary = uv_binary
        self._start_timeout_seconds = start_timeout_seconds
        self._stop_grace_seconds = stop_grace_seconds
        self._process_runner = process_runner

    def server_for(self, model: ModelSpec) -> ModelServer:
        if model.is_ollama:
            return self.ollama_server()
        return KevServer(
            self._transport,
            kev_directory=self._kev_directory,
            kev_commit=self._kev_commit,
            spawn=self._spawn,
            clock=self._clock,
            sleeper=self._sleeper,
            uv_binary=self._uv_binary,
            host=self._host,
            port=self._kev_port,
            start_timeout_seconds=self._start_timeout_seconds,
            stop_grace_seconds=self._stop_grace_seconds,
            process_runner=self._process_runner,
        )

    def ollama_server(self) -> OllamaServer:
        """The Ollama server on the experiment's port, also what `pin` provisions with."""
        return OllamaServer(
            self._transport,
            spawn=self._spawn,
            clock=self._clock,
            sleeper=self._sleeper,
            binary=self._ollama_binary,
            host=self._host,
            port=self._ollama_port,
            start_timeout_seconds=self._start_timeout_seconds,
            stop_grace_seconds=self._stop_grace_seconds,
            process_runner=self._process_runner,
        )
