"""Use case `import-run-01`: carry the completed calls of the sealed run into the decisional run, by digest (ADR 0006)."""

from __future__ import annotations

import hashlib
from typing import Any, Protocol

from pilot.domain.import_manifest import (
    IMPORTS_FILENAME,
    PLAN_FILENAME,
    ImportPlan,
    ImportRefused,
    parse_sums,
    plan_import,
)


class ImportSource(Protocol):
    def listing(self) -> dict[str, str]: ...

    def sums_text(self) -> str: ...

    def read_bytes(self, relative: str) -> bytes: ...

    def read_json(self, relative: str) -> Any: ...


class ImportTarget(Protocol):
    def exists(self, relative: str) -> bool: ...

    def write_bytes(self, relative: str, data: bytes) -> None: ...

    def write_json(self, relative: str, value: Any) -> None: ...


def plan_from_source(source: ImportSource) -> tuple[ImportPlan, str]:
    """The import plan and the digest of the seal; refuses unless every file matches `SHA256SUMS`."""
    sums_text = source.sums_text()
    plan = plan_import(source.listing(), parse_sums(sums_text), source.read_json(PLAN_FILENAME))
    return plan, hashlib.sha256(sums_text.encode()).hexdigest()


def import_run(source: ImportSource, target: ImportTarget) -> ImportPlan:
    """Copies every planned file byte for byte, then writes `imports.json`; writes nothing when it refuses."""
    plan, sums_digest = plan_from_source(source)
    _refuse_existing(plan, target)
    copies = [(entry, source.read_bytes(entry.source_path)) for entry in plan.entries]
    for entry, data in copies:
        if hashlib.sha256(data).hexdigest() != entry.digest:
            raise ImportRefused(f"{entry.source_path} changed while it was read")
    for entry, data in copies:
        target.write_bytes(entry.target_path, data)
    target.write_json(IMPORTS_FILENAME, plan.to_mapping(sums_digest))
    return plan


def _refuse_existing(plan: ImportPlan, target: ImportTarget) -> None:
    present = [path for path in (IMPORTS_FILENAME, PLAN_FILENAME, *(e.target_path for e in plan.entries)) if target.exists(path)]
    if present:
        raise ImportRefused(f"the decisional run already holds {sorted(present)[:5]}")
