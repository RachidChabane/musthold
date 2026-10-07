"""The experiment's own Kev server (§5.2, §6.3): `kev.serve` through `uv run`, stopped with its whole process tree."""

from __future__ import annotations

from collections.abc import Mapping
from pathlib import Path
from typing import Final

from calibration.adapters.process_tree import ProcessTable, ProcessTreeTerminator, ServerCommand, Spawn, spawn_process
from calibration.adapters.server_http import JsonClient, ServerError, object_of
from calibration.adapters.server_process import ServerProcess
from calibration.adapters.subprocess_runner import SubprocessRunner
from calibration.adapters.system_time import SystemClock, SystemSleeper
from calibration.application.ports import ServerCard, ShutdownReport
from calibration.domain.canonical import JsonValue
from calibration.domain.constants import (
    HTTP_OK,
    KEV_OFFLINE_ENVIRONMENT,
    KEV_PORT,
    KEV_UNSET_ENVIRONMENT,
    LOOPBACK,
    SERVER_START_TIMEOUT_SECONDS,
    SERVER_STOP_GRACE_SECONDS,
    SYSTEM_ONE_ROUTE,
)
from calibration.domain.models import ModelSpec
from calibration.domain.ports import Clock, HttpTransport, ProcessRunner, Sleeper

DEFAULT_UV: Final = "uv"
MODELS_PATH: Final = "/v1/models"
REVISION_SEPARATOR: Final = "@"
SERVE_ARGUMENTS: Final = ("run", "--frozen", "--extra", "serve", "python", "-m", "kev.serve")


def kev_command(uv: str, run_reference: str, host: str, port: int, directory: Path) -> ServerCommand:
    """`uv run --frozen --extra serve python -m kev.serve ...` in the Kev repository, with the environment of §5.2."""
    return ServerCommand(
        (uv, *SERVE_ARGUMENTS, "--run", run_reference, "--host", host, "--port", str(port)),
        env=dict(KEV_OFFLINE_ENVIRONMENT),
        unset=KEV_UNSET_ENVIRONMENT,
        cwd=directory,
    )


def revision_of(run_reference: str) -> str:
    """The revision after the `@` of `<repository>@<revision>`."""
    _, separator, revision = run_reference.partition(REVISION_SEPARATOR)
    if not separator or not revision:
        raise ServerError(f"the run reference {run_reference!r} names no revision")
    return revision


class KevServer:
    def __init__(
        self,
        transport: HttpTransport,
        *,
        kev_directory: Path,
        kev_commit: str,
        spawn: Spawn = spawn_process,
        clock: Clock | None = None,
        sleeper: Sleeper | None = None,
        uv_binary: str = DEFAULT_UV,
        host: str = LOOPBACK,
        port: int = KEV_PORT,
        start_timeout_seconds: float = SERVER_START_TIMEOUT_SECONDS,
        stop_grace_seconds: float = SERVER_STOP_GRACE_SECONDS,
        process_runner: ProcessRunner | None = None,
    ) -> None:
        clock = clock or SystemClock()
        sleeper = sleeper or SystemSleeper()
        terminator = ProcessTreeTerminator(
            ProcessTable(process_runner or SubprocessRunner()), clock, sleeper, stop_grace_seconds
        )
        self._directory = kev_directory
        self._commit = kev_commit
        self._uv = uv_binary
        self._host = host
        self._port = port
        self._api = JsonClient(transport, f"http://{host}:{port}")
        self._process = ServerProcess(
            spawn=spawn,
            terminator=terminator,
            clock=clock,
            sleeper=sleeper,
            host=host,
            port=port,
            start_timeout_seconds=start_timeout_seconds,
        )
        self._run_reference: str | None = None

    @property
    def endpoint_url(self) -> str:
        return f"http://{self._host}:{self._port}{SYSTEM_ONE_ROUTE}"

    def start(self, model: ModelSpec, run_reference: str | None) -> None:
        if run_reference is None:
            raise ServerError(f"{model.model_id} needs a run reference")
        revision_of(run_reference)
        self._process.launch(kev_command(self._uv, run_reference, self._host, self._port, self._directory))
        self._run_reference = run_reference
        try:
            self._process.wait_until_ready(lambda: self._api.status_of(MODELS_PATH) == HTTP_OK, "kev.serve")
        except ServerError:
            self._process.stop()
            raise

    def card(self) -> ServerCard:
        if self._run_reference is None:
            raise ServerError("no model was started")
        document = self._api.get(MODELS_PATH)
        entry = self._first_model(document)
        tokens = entry.get("max_state_tokens")
        return ServerCard(
            server_version=self._commit,
            model_digest=revision_of(self._run_reference),
            environment=dict(KEV_OFFLINE_ENVIRONMENT),
            documents={MODELS_PATH.removeprefix("/"): document},
            loaded_context_tokens=tokens if isinstance(tokens, int) and not isinstance(tokens, bool) else None,
        )

    def prefix_cache_hits(self) -> int | None:
        cache = self._first_model(self._api.get(MODELS_PATH)).get("prefix_cache")
        hits = object_of(cache, "prefix_cache").get("hits") if cache is not None else None
        return hits if isinstance(hits, int) and not isinstance(hits, bool) else None

    def stop(self) -> ShutdownReport:
        stop = self._process.stop()
        self._run_reference = None
        return ShutdownReport(stop.stopped, (), stop.stopped, stop.port_free)

    @staticmethod
    def _first_model(document: JsonValue) -> Mapping[str, JsonValue]:
        models = object_of(document, MODELS_PATH).get("models")
        if not isinstance(models, list) or not models:
            raise ServerError(f"{MODELS_PATH} lists no model")
        return object_of(models[0], f"{MODELS_PATH} model")
