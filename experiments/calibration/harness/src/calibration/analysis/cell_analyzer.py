"""Everything `analyze` derives for one cell: metrics, source (3), admission, band, sensitivities and its files."""

from __future__ import annotations

from collections.abc import Sequence
from dataclasses import dataclass
from typing import Final

from calibration.analysis.admission_cell import cell_admission
from calibration.analysis.bands import Band, BandVerdict, admission_capped, band_of
from calibration.analysis.cell_metrics import CellMetrics, MetricSettings, compute_cell_metrics
from calibration.analysis.cell_record import (
    CellRecordParts,
    blocked_record,
    cell_record,
    key_identifiers,
    model_label,
    reliability_csv,
    risk_coverage_csv,
)
from calibration.analysis.conclusions import conclusion
from calibration.analysis.determinism import DeterminismFigures
from calibration.analysis.model import CellData
from calibration.analysis.sensitivity import cell_sensitivities
from calibration.analysis.source3 import operating_points
from calibration.analysis.stats.admission import AdmissionOutcome
from calibration.domain.canonical import JsonValue
from calibration.domain.constants import (
    GATE_THRESHOLD_JH3,
    GATE_THRESHOLD_JH5,
    GATE_THRESHOLD_S1,
    RELIABILITY_DIR,
    RISK_COVERAGE_DIR,
)
from calibration.domain.deviations import DeviationEntry, standing_of
from calibration.domain.sources import SourceId

GATE_THRESHOLDS: Final = {
    SourceId.S1: GATE_THRESHOLD_S1,
    SourceId.JH5: GATE_THRESHOLD_JH5,
    SourceId.JH3: GATE_THRESHOLD_JH3,
    SourceId.VH2: GATE_THRESHOLD_S1,
}
ANNOTATION_UNSTABLE: Final = "UNSTABLE"


@dataclass(frozen=True)
class CellOutcome:
    """A cell's `decisions.json` record, its metrics (None when it has none) and its CSV files."""

    record: dict[str, JsonValue]
    metrics: CellMetrics | None
    verdict: BandVerdict
    files: dict[str, str]


def _csv_name(cell: CellData) -> str:
    return f"{cell.kind.value}__{cell.source.value}__{cell.model}.csv"


def _files(cell: CellData, metrics: CellMetrics) -> dict[str, str]:
    name = _csv_name(cell)
    return {
        f"{RELIABILITY_DIR}/{name}": reliability_csv(metrics.reliability),
        f"{RISK_COVERAGE_DIR}/{name}": risk_coverage_csv(metrics.curve),
    }


def analyze_cell(
    cell: CellData,
    deviations: Sequence[DeviationEntry],
    settings: MetricSettings,
    determinism: DeterminismFigures | None,
) -> CellOutcome:
    """The band of a cell is the first rule that applies (§11); a blocked cell carries no metric."""
    identifiers = key_identifiers(cell)
    standing = standing_of(deviations, identifiers)
    source_name, model_name = cell.source.value, model_label(cell)
    if cell.blocked_reason is not None or standing.blocked:
        reason = cell.blocked_reason or "a blocked entry is in scope"
        verdict = BandVerdict(Band.BLOCKED, reason, ())
        text = conclusion(Band.BLOCKED, source_name, model_name, cell.kind.value, None, reason)
        return CellOutcome(blocked_record(cell, reason, text), None, verdict, {})
    metrics = compute_cell_metrics(
        cell, settings, equal_mass=cell.source is SourceId.S1, gate_threshold=GATE_THRESHOLDS[cell.source]
    )
    verdict = band_of(cell, metrics, None, standing)
    admission, outcome = cell_admission(cell, settings) if metrics else ({}, AdmissionOutcome.INCONCLUSIVE)
    outcome, admission_cap = admission_capped(outcome, standing)
    annotations = (ANNOTATION_UNSTABLE,) if determinism is not None and determinism.unstable else ()
    parts = CellRecordParts(
        cell=cell,
        metrics=metrics,
        verdict=verdict,
        annotations=annotations,
        points=operating_points(cell.source, cell.kind),
        admission=admission,
        admission_outcome=outcome.value,
        capped_by=tuple(sorted({*verdict.capped_by, *admission_cap})),
        conclusion=conclusion(verdict.band, source_name, model_name, cell.kind.value, metrics, verdict.reason),
        determinism=determinism.to_record() if determinism is not None else None,
    )
    record = cell_record(parts)
    record["sensitivity"] = cell_sensitivities(cell, settings)
    return CellOutcome(record, metrics, verdict, _files(cell, metrics) if metrics else {})
