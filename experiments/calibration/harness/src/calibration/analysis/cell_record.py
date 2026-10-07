"""One cell of `decisions.json` (§15.2) and the CSV files that hold its reliability and risk-coverage tables."""

from __future__ import annotations

import csv
import io
import math
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from typing import Final

from calibration.analysis.bands import Band, BandVerdict
from calibration.analysis.cell_metrics import CellMetrics, GridPoint
from calibration.analysis.model import FALL_THROUGH_TYPES, CellData
from calibration.analysis.source3 import OperatingPoint, undetermined_figures
from calibration.analysis.stats.calibration import ReliabilityRow
from calibration.analysis.stats.intervals import Estimate
from calibration.analysis.stats.risk_coverage import RiskCoverageCurve
from calibration.domain.canonical import JsonValue
from calibration.domain.models import MODEL_BY_ID
from calibration.domain.sources import SourceId

CSV_NEWLINE: Final = "\n"
HOST_LABEL: Final = "claude-sonnet-5-5"


def model_label(cell: CellData) -> str:
    """The model as the conclusion sentences name it."""
    if cell.model in MODEL_BY_ID:
        return f"{cell.model}, {MODEL_BY_ID[cell.model].description}"
    return HOST_LABEL


def key_identifiers(cell: CellData) -> tuple[str, ...]:
    """The identifiers that put a cell in the scope of a deviation entry (§14.2): its kind, its model or source."""
    if cell.source is SourceId.S1:
        return (cell.kind.value, cell.model)
    return (cell.kind.value, cell.source.value)


def _record(estimate: Estimate | None) -> JsonValue:
    return None if estimate is None else dict(estimate.to_record())


def _grid(points: Sequence[GridPoint]) -> list[JsonValue]:
    return [
        {
            "tau": point.tau,
            "positives": point.positives,
            "rows": point.rows,
            "miss_rate": dict(point.miss.to_record()),
            "flag_rate": dict(point.flag.to_record()),
        }
        for point in points
    ]


def _counts(cell: CellData, metrics: CellMetrics | None) -> dict[str, JsonValue]:
    return {
        "units": cell.units,
        "decisions": len(cell.decisions),
        "with_prediction": len(cell.predicted),
        "clusters": metrics.clusters if metrics else 0,
        "fall_through": {status: cell.fall_through.get(status, 0) for status in FALL_THROUGH_TYPES},
        "units_without_votes": cell.units_without_votes,
    }


def _metrics_record(metrics: CellMetrics) -> dict[str, JsonValue]:
    return {
        "accuracy": _record(metrics.accuracy),
        "auroc": _record(metrics.auroc),
        "ece": _record(metrics.ece),
        "ece_top_label": _record(metrics.ece_top_label),
        "ece_equal_mass": _record(metrics.ece_equal_mass),
        "brier": _record(metrics.brier),
        "brier_skill": _record(metrics.brier_skill),
        "q09": _record(metrics.q09),
        "q09_n": metrics.q09_n,
        "aurc": _record(metrics.aurc),
        "c05": _record(metrics.c05),
        "c05u": metrics.c05u,
        "majority_accuracy": metrics.majority_accuracy,
        "base_rate": metrics.base_rate,
        "baselines": dict(metrics.baselines),
        "false_aligned": _record(metrics.false_aligned),
        "false_aligned_threshold": metrics.false_aligned_threshold,
        "false_aligned_counts": None
        if metrics.false_aligned_counts is None
        else {"successes": metrics.false_aligned_counts[0], "total": metrics.false_aligned_counts[1]},
        "screen_all": _grid(metrics.screen_all),
        "screen_stratum": _grid(metrics.screen_stratum),
    }


@dataclass(frozen=True)
class CellRecordParts:
    cell: CellData
    metrics: CellMetrics | None
    verdict: BandVerdict
    annotations: tuple[str, ...]
    points: Sequence[OperatingPoint]
    admission: Mapping[str, JsonValue]
    admission_outcome: str
    capped_by: tuple[str, ...]
    conclusion: str
    determinism: JsonValue


def cell_record(parts: CellRecordParts) -> dict[str, JsonValue]:
    """The `decisions.json` entry of a cell: counts, every metric, source (3), the band and the admission outcome."""
    cell = parts.cell
    return {
        "kind": cell.kind.value,
        "source": cell.source.value,
        "model": cell.model,
        "confidence_source": cell.confidence_source,
        "identifiers": list(key_identifiers(cell)),
        "counts": _counts(cell, parts.metrics),
        "metrics": _metrics_record(parts.metrics) if parts.metrics else None,
        "source3": [dict(undetermined_figures(cell, point)) for point in parts.points] if cell.decisions else [],
        "admission": dict(parts.admission),
        "admission_outcome": parts.admission_outcome,
        "band": parts.verdict.band.value,
        "band_reason": parts.verdict.reason,
        "annotations": list(parts.annotations),
        "capped_by": list(parts.capped_by),
        "conclusion": parts.conclusion,
        "determinism": parts.determinism,
    }


def blocked_record(cell: CellData, reason: str, conclusion: str) -> dict[str, JsonValue]:
    return {
        "kind": cell.kind.value,
        "source": cell.source.value,
        "model": cell.model,
        "confidence_source": cell.confidence_source,
        "identifiers": list(key_identifiers(cell)),
        "counts": _counts(cell, None),
        "metrics": None,
        "source3": [],
        "admission": {},
        "admission_outcome": "BLOCKED",
        "band": Band.BLOCKED.value,
        "band_reason": reason,
        "annotations": [],
        "capped_by": [],
        "conclusion": conclusion,
        "determinism": None,
    }


def _csv(header: Sequence[str], rows: Sequence[Sequence[object]]) -> str:
    buffer = io.StringIO()
    writer = csv.writer(buffer, lineterminator=CSV_NEWLINE)
    writer.writerow(header)
    writer.writerows(rows)
    return buffer.getvalue()


def _number(value: float) -> object:
    return "" if math.isnan(value) else repr(value)


def reliability_csv(rows: Sequence[ReliabilityRow]) -> str:
    return _csv(
        ("bin_lo", "bin_hi", "n", "mean_confidence", "observed_frequency", "wilson_lo", "wilson_hi"),
        [
            (
                row.lo,
                row.hi,
                row.n,
                _number(row.mean_confidence),
                _number(row.frequency),
                _number(row.frequency_interval.lo),
                _number(row.frequency_interval.hi),
            )
            for row in rows
        ],
    )


def risk_coverage_csv(curve: RiskCoverageCurve) -> str:
    return _csv(
        ("threshold", "n", "errors", "coverage", "risk", "undetermined"),
        [(point.threshold, point.n, point.errors, point.coverage, point.risk, 1.0 - point.coverage) for point in curve],
    )
