"""Ports of `pin`: registry and Hub metadata, the Kev environment and the downloads of admitted models."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from typing import Protocol

from calibration.domain.canonical import JsonValue


@dataclass(frozen=True)
class OllamaManifest:
    """The manifest of a library tag as the registry serves it, before any download."""

    library: str
    tag: str
    digest: str
    size_bytes: int


class OllamaRegistry(Protocol):
    def manifest(self, library: str, tag: str) -> OllamaManifest:
        """The digest (SHA-256 of the manifest bytes) and the size (config plus layers) from registry metadata only."""
        ...


@dataclass(frozen=True)
class HubFile:
    path: str
    size_bytes: int


@dataclass(frozen=True)
class HubRepository:
    repository: str
    revision: str
    files: tuple[HubFile, ...]

    @property
    def weight_files(self) -> tuple[HubFile, ...]:
        """The weight files of §5.3: `*.safetensors` and `*.pt` (the pointer head)."""
        return tuple(f for f in self.files if f.path.endswith((".safetensors", ".pt")))


class ModelHub(Protocol):
    def repository(self, repository: str, revision: str | None = None) -> HubRepository:
        """The commit sha behind `revision` (the default branch when None) and the files with their sizes, from
        Hub metadata only."""
        ...


@dataclass(frozen=True)
class HeadMeta:
    """What `head.pt` of a Kev checkpoint names: its base model and the base revision (None when it names none)."""

    base: str
    base_revision: str | None


@dataclass(frozen=True)
class KevEnvironmentInfo:
    """The Kev environment as set up: the commit, the digest of its `uv.lock` and the versions of §5.3."""

    commit: str
    uv_lock_sha256: str
    versions: Mapping[str, str]


class KevEnvironment(Protocol):
    def setup(self) -> KevEnvironmentInfo:
        """Clones the repository at the pinned commit and runs `uv sync --frozen --extra serve`."""
        ...

    def head_meta(self, repository: str, revision: str) -> HeadMeta:
        """Reads `head.pt` of the pinned revision in the Kev environment; only that file is downloaded."""
        ...

    def download(self, repository: str, revision: str) -> None:
        """Downloads the pinned revision of an admitted repository into the Hub cache."""
        ...


@dataclass(frozen=True)
class OllamaModelRecord:
    """What `/api/show` reports for a pulled tag."""

    identifier: str
    digest: str
    details: Mapping[str, JsonValue]
    parameters: Mapping[str, JsonValue]


@dataclass(frozen=True)
class OllamaProvision:
    version: str
    models: Mapping[str, OllamaModelRecord]


class OllamaProvisioner(Protocol):
    def provision(self, tags: Sequence[str]) -> OllamaProvision:
        """Starts the experiment's own Ollama server (port 11435), pulls each tag, records `/api/show`, then stops
        the server and checks that the port is free. Pulls nothing when `tags` is empty."""
        ...
