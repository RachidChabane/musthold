"""The experiment's own Ollama server (§6.2): `ollama serve` on its own port, one model, unloaded and stopped at the end."""

from __future__ import annotations

from collections.abc import Mapping
from typing import Final

from calibration.adapters.process_tree import ProcessTable, ProcessTreeTerminator, ServerCommand, Spawn, spawn_process
from calibration.adapters.server_http import JsonClient, ServerError, ServerStartError, object_of
from calibration.adapters.server_process import ServerProcess
from calibration.adapters.subprocess_runner import SubprocessRunner
from calibration.adapters.system_time import SystemClock, SystemSleeper
from calibration.application.ports import ServerCard, ShutdownReport
from calibration.domain.canonical import JsonValue
from calibration.domain.constants import (
    HTTP_OK,
    LOOPBACK,
    OLLAMA_ENVIRONMENT,
    OLLAMA_PORT,
    SERVER_START_TIMEOUT_SECONDS,
    SERVER_STOP_GRACE_SECONDS,
    SYSTEM_ONE_ROUTE,
)
from calibration.domain.models import ModelSpec
from calibration.domain.ports import Clock, HttpTransport, ProcessRunner, Sleeper

DEFAULT_BINARY: Final = "ollama"
HOST_VARIABLE: Final = "OLLAMA_HOST"
VERSION_PATH: Final = "/api/version"
PS_PATH: Final = "/api/ps"
TAGS_PATH: Final = "/api/tags"
SHOW_PATH: Final = "/api/show"
PULL_PATH: Final = "/api/pull"
GENERATE_PATH: Final = "/api/generate"
PULL_TIMEOUT_SECONDS: Final = 7200.0
UNLOAD_WAIT_SECONDS: Final = 30.0
UNLOAD_POLL_SECONDS: Final = 0.2
UNLOAD_KEEP_ALIVE: Final = 0


def ollama_environment(host: str, port: int) -> dict[str, str]:
    """`OLLAMA_ENVIRONMENT` with the address of this server."""
    environment = dict(OLLAMA_ENVIRONMENT)
    environment[HOST_VARIABLE] = f"{host}:{port}"
    return environment


def _entries(document: JsonValue, where: str) -> list[Mapping[str, JsonValue]]:
    models = object_of(document, where).get("models")
    if models is None:
        return []
    if not isinstance(models, list):
        raise ServerError(f"{where}: models is not a list")
    return [object_of(model, f"{where} model") for model in models]


def listed_model_names(document: JsonValue, where: str) -> tuple[str, ...]:
    """The names of the models an `/api/ps` or `/api/tags` document lists."""
    entries = _entries(document, where)
    return tuple(str(entry.get("name") or entry.get("model") or "") for entry in entries)


class OllamaServer:
    """A `ModelServer` for the Ollama flavour; `start_empty` is the start without a model, as `pin` uses it."""

    def __init__(
        self,
        transport: HttpTransport,
        *,
        spawn: Spawn = spawn_process,
        clock: Clock | None = None,
        sleeper: Sleeper | None = None,
        binary: str = DEFAULT_BINARY,
        host: str = LOOPBACK,
        port: int = OLLAMA_PORT,
        start_timeout_seconds: float = SERVER_START_TIMEOUT_SECONDS,
        stop_grace_seconds: float = SERVER_STOP_GRACE_SECONDS,
        process_runner: ProcessRunner | None = None,
    ) -> None:
        clock = clock or SystemClock()
        sleeper = sleeper or SystemSleeper()
        terminator = ProcessTreeTerminator(
            ProcessTable(process_runner or SubprocessRunner()), clock, sleeper, stop_grace_seconds
        )
        self._host = host
        self._port = port
        self._binary = binary
        self._clock = clock
        self._sleeper = sleeper
        self._environment = ollama_environment(host, port)
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
        self._tag: str | None = None

    @property
    def endpoint_url(self) -> str:
        return f"http://{self._host}:{self._port}{SYSTEM_ONE_ROUTE}"

    def start_empty(self) -> None:
        """Starts `ollama serve` and requires that no model is loaded; a failure leaves no process behind."""
        self._process.launch(ServerCommand((self._binary, "serve"), env=self._environment))
        try:
            self._process.wait_until_ready(lambda: self._api.status_of(VERSION_PATH) == HTTP_OK, "ollama serve")
            loaded = self.loaded_models()
            if loaded:
                raise ServerStartError(f"the new server already holds {', '.join(loaded)}")
        except ServerError:
            self._process.stop()
            raise

    def start(self, model: ModelSpec, run_reference: str | None) -> None:
        self.start_empty()
        self._tag = model.identifier

    def version(self) -> str:
        document = object_of(self._api.get(VERSION_PATH), VERSION_PATH)
        version = document.get("version")
        if not isinstance(version, str):
            raise ServerError(f"{VERSION_PATH} names no version")
        return version

    def loaded_models(self) -> tuple[str, ...]:
        return listed_model_names(self._api.get(PS_PATH), PS_PATH)

    def pull(self, tag: str) -> None:
        document = object_of(
            self._api.post(PULL_PATH, {"model": tag, "stream": False}, PULL_TIMEOUT_SECONDS), PULL_PATH
        )
        if document.get("error") is not None:
            raise ServerError(f"pulling {tag} failed: {document['error']}")

    def show(self, tag: str) -> Mapping[str, JsonValue]:
        return object_of(self._api.post(SHOW_PATH, {"model": tag}), SHOW_PATH)

    def digest(self, tag: str) -> str:
        return self._digest_of(self._tag_entry(tag))

    def card(self) -> ServerCard:
        tag = self._tag
        if tag is None:
            raise ServerError("no model was started")
        ps_document = self._api.get(PS_PATH)
        entry = self._tag_entry(tag)
        loaded = _entries(ps_document, PS_PATH)
        return ServerCard(
            server_version=self.version(),
            model_digest=self._digest_of(entry),
            environment=dict(self._environment),
            documents={
                VERSION_PATH.lstrip("/"): self._api.get(VERSION_PATH),
                SHOW_PATH.lstrip("/"): self._api.post(SHOW_PATH, {"model": tag}),
                PS_PATH.lstrip("/"): ps_document,
                TAGS_PATH.lstrip("/"): dict(entry),
            },
            loaded_context_tokens=self._context_length(loaded),
        )

    def prefix_cache_hits(self) -> int | None:
        return None

    def stop(self) -> ShutdownReport:
        unloaded, remaining = self._unload()
        stop = self._process.stop()
        self._tag = None
        return ShutdownReport(unloaded, remaining, stop.stopped, stop.port_free)

    def _tag_entry(self, tag: str) -> Mapping[str, JsonValue]:
        for entry in _entries(self._api.get(TAGS_PATH), TAGS_PATH):
            if tag in (entry.get("name"), entry.get("model")):
                return entry
        raise ServerError(f"{TAGS_PATH} lists no model {tag}")

    @staticmethod
    def _digest_of(entry: Mapping[str, JsonValue]) -> str:
        digest = entry.get("digest")
        if not isinstance(digest, str):
            raise ServerError(f"{TAGS_PATH} names no digest")
        return digest

    @staticmethod
    def _context_length(loaded: list[Mapping[str, JsonValue]]) -> int | None:
        length = loaded[0].get("context_length") if loaded else None
        return length if isinstance(length, int) and not isinstance(length, bool) else None

    def _unload(self) -> tuple[bool, tuple[str, ...]]:
        """Whether every loaded model was unloaded, and the models still listed afterwards."""
        if not self._process.is_running:
            return True, ()
        try:
            names = {*self.loaded_models(), *([self._tag] if self._tag else [])}
            unloaded = all(self._unload_one(name) for name in sorted(names))
            return unloaded, self._wait_until_empty()
        except ServerError:
            return False, ()

    def _unload_one(self, name: str) -> bool:
        try:
            self._api.post(GENERATE_PATH, {"model": name, "keep_alive": UNLOAD_KEEP_ALIVE})
        except ServerError:
            return False
        return True

    def _wait_until_empty(self) -> tuple[str, ...]:
        deadline = self._clock.monotonic() + UNLOAD_WAIT_SECONDS
        while True:
            loaded = self.loaded_models()
            if not loaded or self._clock.monotonic() >= deadline:
                return loaded
            self._sleeper.sleep(UNLOAD_POLL_SECONDS)
