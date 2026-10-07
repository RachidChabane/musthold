"""What changes between blocks and against the lock (§14.2): `provenance` and `protocol` entries."""

from __future__ import annotations

from collections.abc import Mapping
from typing import Final

from calibration.domain.lock import LockedModel, ModelsLock
from calibration.domain.models import ModelSpec

DIGEST_PREFIX: Final = "sha256:"


def bare_digest(digest: str) -> str:
    return digest.removeprefix(DIGEST_PREFIX).lower()


def lock_changes(
    model: ModelSpec, lock: ModelsLock, locked: LockedModel, server_version: str, model_digest: str
) -> list[str]:
    """Departures of a started server from the lock: its version (the Ollama version or the Kev commit) and the
    digest of the model it holds."""
    expected_version = lock.ollama_version if model.is_ollama else lock.kev_commit
    changes: list[str] = []
    if server_version != expected_version:
        changes.append(f"{model.model_id}: the server is {server_version}, the lock pins {expected_version}")
    if bare_digest(model_digest) != bare_digest(locked.model_digest):
        changes.append(f"{model.model_id}: the model digest is {model_digest}, the lock pins {locked.model_digest}")
    return changes


def provenance_changes(previous: Mapping[str, str], current: Mapping[str, str]) -> list[str]:
    """Recorded values the document does not pin that differ from the previous block's."""
    return [
        f"{key} changed from {previous[key]} to {current[key]}"
        for key in sorted(set(previous) & set(current))
        if previous[key] != current[key]
    ]


def harness_changes(recorded: Mapping[str, str], current: Mapping[str, str]) -> list[str]:
    """Departures of the harness commit and the builder digests from the ones the manifest records."""
    return [
        f"{key} is {current.get(key)}, the manifest records {recorded[key]}"
        for key in sorted(recorded)
        if current.get(key) != recorded[key]
    ]
