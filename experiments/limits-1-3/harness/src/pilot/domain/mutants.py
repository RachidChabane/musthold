"""Deterministic mutant processing records (PREREG 6.4: F0 to F3)."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass, field
from enum import Enum
from typing import Any

from pilot.domain.edits import EditSet
from pilot.domain.outcomes import KillResult


class MutantStatus(Enum):
    STILLBORN = "stillborn"
    TRIVIALLY_EQUIVALENT = "trivially_equivalent"
    DUPLICATE = "duplicate"
    EVALUATED = "evaluated"


class StillbornReason(Enum):
    IMPORT_SMOKE_FAILED = "import_smoke_failed"
    COLLECTION_ERROR = "collection_error"


class DifferentialKind(Enum):
    DIFFERS = "DIFFERS"
    NO_DIFF_FOUND = "NO_DIFF_FOUND"
    SKIPPED = "SKIPPED"


@dataclass(frozen=True)
class DifferentialResult:
    kind: DifferentialKind
    function: str | None = None
    args: Any = None
    original_out: Any = None
    mutant_out: Any = None
    reason: str | None = None


@dataclass(frozen=True)
class Candidate:
    arm: str
    mutant_id: str
    edit_set: EditSet

    @property
    def key(self) -> str:
        return f"{self.arm}/{self.mutant_id}"


@dataclass
class MutantRecord:
    candidate: Candidate
    status: MutantStatus
    reason: str | None = None
    duplicate_of: str | None = None
    merged_ids: list[str] = field(default_factory=list)
    digest: str | None = None
    changed_lines: int = 0
    kill_vector: Mapping[str, KillResult] = field(default_factory=dict)
    differential: DifferentialResult | None = None

    @property
    def killed_by(self) -> list[str]:
        return sorted(n for n, k in self.kill_vector.items() if k is KillResult.KILLED)
