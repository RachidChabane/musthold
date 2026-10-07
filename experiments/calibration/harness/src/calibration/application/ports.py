"""Ports of the run side: every dependency of a use case that touches the outside world."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass, field
from pathlib import Path
from typing import Protocol

from calibration.domain.canonical import JsonValue
from calibration.domain.models import ModelSpec


class RunStore(Protocol):
    """The files of one run directory, addressed by their path below it."""

    @property
    def root(self) -> Path: ...

    def exists(self, relative: str) -> bool: ...

    def read_bytes(self, relative: str) -> bytes: ...

    def read_text(self, relative: str) -> str: ...

    def write_bytes(self, relative: str, data: bytes) -> None:
        """Writes atomically: the file appears complete or not at all."""
        ...

    def write_text(self, relative: str, text: str) -> None: ...

    def append_text(self, relative: str, text: str) -> None: ...

    def list_dir(self, relative: str) -> list[str]: ...


@dataclass(frozen=True)
class MemoryReading:
    """One reading of the guard's measures: free memory percentage and whether the pressure level is normal."""

    free_percent: int
    pressure_normal: bool


class MemoryProbe(Protocol):
    def total_memory_bytes(self) -> int: ...

    def reading(self) -> MemoryReading: ...


@dataclass(frozen=True)
class HeavyWorkReport:
    """The conditions of §6.1 for "no other heavy local work"."""

    foreign_processes: tuple[str, ...]
    installed_ollama_models: tuple[str, ...]
    other_guard_processes: tuple[str, ...]

    @property
    def clear(self) -> bool:
        return not (self.foreign_processes or self.installed_ollama_models or self.other_guard_processes)

    def describe(self) -> str:
        parts = [
            *(f"process {name}" for name in self.foreign_processes),
            *(f"installed Ollama holds {name}" for name in self.installed_ollama_models),
            *(f"guard process {name}" for name in self.other_guard_processes),
        ]
        return "; ".join(parts)


class HeavyWorkProbe(Protocol):
    def check(self) -> HeavyWorkReport:
        """Looks for heavy work other than the block's own process group, its mem-guard ancestor and its server."""
        ...


@dataclass(frozen=True)
class ShutdownReport:
    """What the end of a block found (§6.2, §6.3): the model unloaded, the server stopped and its port free."""

    unloaded: bool
    models_after_unload: tuple[str, ...]
    server_stopped: bool
    port_free: bool

    @property
    def clean(self) -> bool:
        return self.unloaded and not self.models_after_unload and self.server_stopped and self.port_free


@dataclass(frozen=True)
class ServerCard:
    """The record of a started server (§5.3): version, model digest and the cards of its endpoints."""

    server_version: str
    model_digest: str
    environment: Mapping[str, str]
    documents: Mapping[str, JsonValue]
    loaded_context_tokens: int | None = None

    def to_record(self) -> dict[str, JsonValue]:
        return {
            "server_version": self.server_version,
            "model_digest": self.model_digest,
            "environment": dict(self.environment),
            "documents": dict(self.documents),
            "loaded_context_tokens": self.loaded_context_tokens,
        }


class ModelServer(Protocol):
    """The experiment's own server of one flavour, started and stopped inside the block's process group."""

    @property
    def endpoint_url(self) -> str:
        """The `/v1/systemone` address requests are posted to."""
        ...

    def start(self, model: ModelSpec, run_reference: str | None) -> None:
        """Starts the server and waits until it answers; raises when it does not come up or when a model is loaded."""
        ...

    def card(self) -> ServerCard:
        """The server card as it stands now; `loaded_context_tokens` is known once a model is loaded."""
        ...

    def prefix_cache_hits(self) -> int | None:
        """Kev's cumulative prefix-cache hit counter; None for a server without one."""
        ...

    def stop(self) -> ShutdownReport:
        """Unloads the model, checks that nothing stays loaded, stops the server and checks that its port is free."""
        ...


class ServerFactory(Protocol):
    def server_for(self, model: ModelSpec) -> ModelServer: ...


@dataclass(frozen=True)
class ProvenanceReading:
    """Recorded values the document does not pin (§14.2 `provenance`)."""

    values: Mapping[str, str] = field(default_factory=dict[str, str])


class ProvenanceProbe(Protocol):
    def reading(self) -> ProvenanceReading: ...
