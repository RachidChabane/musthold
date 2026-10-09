"""What the site is built from: the build mode, the run's sealed outputs and the resolved evidence of each case."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass
from enum import Enum
from typing import Final

from specbench.domain.cases import CaseRecord, MutantResult
from specbench.domain.deviations import DeviationEntry
from specbench.domain.manifest import Manifest
from specbench.domain.records import TrunkRecord

SYNTHETIC_BADGE: Final = "Synthetic data — not results"
SYNTHETIC_NOTICE: Final = (
    "Every figure, case and digest on this page comes from the benchmark's synthetic test fixtures. "
    "None of it is a measurement of the product."
)


class BuildMode(Enum):
    """A site is built from a sealed run, or from the synthetic fixtures; the second is badged on every page."""

    SEALED_RUN = "sealed-run"
    SYNTHETIC = "synthetic"

    @property
    def is_synthetic(self) -> bool:
        return self is BuildMode.SYNTHETIC


@dataclass(frozen=True)
class Witness:
    """An input on which the code and the mutant disagree, as the host's validity answer states it."""

    operation: str
    arguments: tuple[tuple[str, str], ...]
    expected_kind: str
    expected: str

    def call(self) -> str:
        return f"{self.operation}({', '.join(f'{name}={value}' for name, value in self.arguments)})"


@dataclass(frozen=True)
class MutantMaterial:
    file: str
    before: str
    after: str
    mechanism: str
    witness: Witness | None
    results: tuple[MutantResult, ...]


@dataclass(frozen=True)
class EvidenceFile:
    label: str
    path: str
    sha256: str
    size: int


@dataclass(frozen=True)
class CaseMaterial:
    """A case record with the content of every file it points to, read and digested by the adapter."""

    record: CaseRecord
    claim: str | None
    gate_report: Mapping[str, object]
    mutant: MutantMaterial | None
    repair_report: Mapping[str, object] | None
    repair_diff: str | None
    trunk: TrunkRecord | None
    evidence: tuple[EvidenceFile, ...]


@dataclass(frozen=True)
class SiteInputs:
    mode: BuildMode
    results: object
    manifest: Manifest
    deviations: tuple[DeviationEntry, ...]
    cases: tuple[CaseMaterial, ...]
    source_commit: str
    generator: str
