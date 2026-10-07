"""`OllamaProvisioner` over the experiment's own Ollama server: pull each tag, record what `/api/show` reports."""

from __future__ import annotations

from collections.abc import Callable, Mapping, Sequence
from typing import Final, Protocol

from calibration.adapters.server_http import ServerError, object_of
from calibration.application.pin_ports import OllamaModelRecord, OllamaProvision
from calibration.application.ports import ShutdownReport
from calibration.domain.canonical import JsonValue

NO_VERSION: Final = ""
_QUOTE: Final = '"'


def _scalar(token: str) -> JsonValue:
    for convert in (int, float):
        try:
            return convert(token)
        except ValueError:
            continue
    if len(token) >= 2 and token.startswith(_QUOTE) and token.endswith(_QUOTE):
        return token[1:-1]
    return token


def parse_parameters(text: str) -> dict[str, JsonValue]:
    """The `parameters` lines of `/api/show` (`num_ctx 2048`) as a mapping with numeric values; a repeated name is a list."""
    parsed: dict[str, list[JsonValue]] = {}
    for line in text.splitlines():
        name, _, value = line.strip().partition(" ")
        if name:
            parsed.setdefault(name, []).append(_scalar(value.strip()))
    return {name: values[0] if len(values) == 1 else values for name, values in parsed.items()}


class ProvisioningServer(Protocol):
    """What provisioning needs of an Ollama server: start without a model, pull, show, read digests, stop."""

    def start_empty(self) -> None: ...

    def version(self) -> str: ...

    def pull(self, tag: str) -> None: ...

    def show(self, tag: str) -> Mapping[str, JsonValue]: ...

    def digest(self, tag: str) -> str: ...

    def stop(self) -> ShutdownReport: ...


class ServerOllamaProvisioner:
    def __init__(self, make_server: Callable[[], ProvisioningServer]) -> None:
        self._make_server = make_server

    def provision(self, tags: Sequence[str]) -> OllamaProvision:
        if not tags:
            return OllamaProvision(NO_VERSION, {})
        server = self._make_server()
        server.start_empty()
        try:
            provision = self._record(server, tags)
        finally:
            report = server.stop()
        if not report.clean:
            raise ServerError("the provisioning server did not stop cleanly")
        return provision

    @staticmethod
    def _record(server: ProvisioningServer, tags: Sequence[str]) -> OllamaProvision:
        models: dict[str, OllamaModelRecord] = {}
        for tag in tags:
            server.pull(tag)
            shown = server.show(tag)
            details = shown.get("details")
            parameters = shown.get("parameters")
            models[tag] = OllamaModelRecord(
                identifier=tag,
                digest=server.digest(tag),
                details=dict(object_of(details, "details")) if details is not None else {},
                parameters=_parameters(parameters),
            )
        return OllamaProvision(server.version(), models)


def _parameters(raw: JsonValue) -> Mapping[str, JsonValue]:
    return parse_parameters(raw) if isinstance(raw, str) else {}
