"""Run manifest and logged deviations (PREREG 4.1, RUN_RECORDS)."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from typing import Any, Protocol

from pilot.application.ports import CliVersionProbe, RunStore
from pilot.domain.model_call import Cli, ModelPin
from pilot.domain.pipeline_constants import DEVIATIONS_FILENAME, RUN_MANIFEST_FILENAME

DEVIATIONS_HEADER = "# Deviations\n\n"


class ProvenanceProbe(Protocol):
    def prereg_commit(self) -> str: ...

    def harness_digest(self) -> str: ...

    def fixture_digest(self) -> str: ...


@dataclass(frozen=True)
class ManifestInputs:
    run_id: str
    pins: Sequence[ModelPin]
    locked_versions: Mapping[Cli, str]


def ensure_manifest(
    store: RunStore, provenance: ProvenanceProbe, versions: CliVersionProbe, inputs: ManifestInputs
) -> dict[str, Any]:
    """Create the manifest on the first call; later calls log every drift as a deviation."""
    current = {
        "run_id": inputs.run_id,
        "prereg_commit": provenance.prereg_commit(),
        "harness_digest": provenance.harness_digest(),
        "fixture_digest": provenance.fixture_digest(),
        "cli_versions": _installed(versions, inputs.pins),
        "models": [
            {"model": p.model, "cli": p.cli.value, "provenance": p.provenance.value}
            for p in sorted(inputs.pins, key=lambda p: p.key)
        ],
    }
    _ensure_deviations_file(store)
    previous = store.read_json(RUN_MANIFEST_FILENAME) if store.exists(RUN_MANIFEST_FILENAME) else None
    for line in _drift(previous, current, inputs.locked_versions):
        log_deviation(store, line)
    store.write_json(RUN_MANIFEST_FILENAME, current)
    return current


def _installed(versions: CliVersionProbe, pins: Sequence[ModelPin]) -> dict[str, str]:
    return {cli.value: versions.installed_version(cli) for cli in sorted({p.cli for p in pins}, key=lambda c: c.value)}


def _drift(
    previous: Mapping[str, Any] | None, current: Mapping[str, Any], locked: Mapping[Cli, str]
) -> list[str]:
    if previous is None:
        return [
            f"CLI {cli} installed {version}, models.lock.json records {locked[Cli(cli)]}"
            for cli, version in current["cli_versions"].items()
            if Cli(cli) in locked and locked[Cli(cli)] != version
        ]
    changed = [
        f"{key} changed from {previous.get(key)} to {current[key]}"
        for key in ("prereg_commit", "harness_digest", "fixture_digest")
        if previous.get(key) != current[key]
    ]
    changed += [
        f"CLI {cli} changed from {previous['cli_versions'].get(cli)} to {version}"
        for cli, version in current["cli_versions"].items()
        if previous["cli_versions"].get(cli) != version
    ]
    return changed


def _ensure_deviations_file(store: RunStore) -> None:
    if not store.exists(DEVIATIONS_FILENAME):
        store.write_text(DEVIATIONS_FILENAME, DEVIATIONS_HEADER)


def log_deviation(store: RunStore, line: str) -> None:
    _ensure_deviations_file(store)
    store.append_text(DEVIATIONS_FILENAME, f"- {line}\n")
