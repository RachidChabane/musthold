"""Descriptive comparisons (§9): two scorers on shared items, the two-signal ALIGNED simulation and model sizes."""

from __future__ import annotations

from collections.abc import Callable, Mapping
from dataclasses import dataclass
from typing import Final

import numpy as np

from calibration.analysis.cell_metrics import MetricInput, MetricSettings
from calibration.analysis.model import CellData, Decision
from calibration.analysis.stats.arrays import Rows, exact_mean
from calibration.analysis.stats.bootstrap import ClusterIndex, Resamples
from calibration.analysis.stats.calibration import brier, ece_equal_width
from calibration.analysis.stats.intervals import Proportion
from calibration.analysis.stats.paired import (
    both_wrong_count,
    paired_metric_difference,
    wrong_given_other_wrong,
)
from calibration.analysis.stats.ranking import auroc, cohen_kappa
from calibration.domain.canonical import JsonValue
from calibration.domain.constants import ECE_BINS
from calibration.domain.ports import MatrixSignature

PINS_OFFSPEC_THRESHOLD: Final = 0.20
SUPPORTING_SIGNATURES: Final = ("SPECIFIC", "OVERBROAD")
MODEL_SIZE_PAIRS: Final = (
    ("M3", "M2"),
    ("M4", "M5"),
    ("M5", "M6"),
    ("M1", "M6"),
    ("M1", "M2"),
    ("M2", "M1"),
    ("M3", "M1"),
    ("M4", "M1"),
    ("M5", "M1"),
    ("M6", "M1"),
)
METRIC_NAMES: Final = ("accuracy", "auroc", "ece", "brier")


def aligned_pairs(first: CellData, second: CellData) -> tuple[list[Decision], list[Decision]]:
    """The decisions of both cells that carry a prediction on the same unit, in unit order."""
    left = {d.unit: d for d in first.predicted}
    right = {d.unit: d for d in second.predicted}
    shared = sorted(set(left) & set(right))
    return [left[unit] for unit in shared], [right[unit] for unit in shared]


Metric = Callable[[Rows], float]


def _metric_functions(data: MetricInput) -> dict[str, Metric]:
    p, y = data.primary
    correct = data.correct
    return {
        "accuracy": lambda rows: exact_mean(correct[rows]),
        "auroc": lambda rows: auroc(p[rows], y[rows]),
        "ece": lambda rows: ece_equal_width(p[rows], y[rows], ECE_BINS),
        "brier": lambda rows: brier(p[rows], y[rows]),
    }


def _estimate(point: float) -> JsonValue:
    return None if np.isnan(point) else point


def compare_cells(first: CellData, second: CellData, settings: MetricSettings) -> dict[str, JsonValue]:
    """Accuracy, AUROC, ECE and Brier of both scorers on the shared items, the paired cluster-bootstrap difference
    first - second of each, the both-wrong count and P(first wrong | second wrong)."""
    left, right = aligned_pairs(first, second)
    if not left:
        return {"shared": 0}
    kind = first.kind
    data_a, data_b = MetricInput.of(kind, left), MetricInput.of(kind, right)
    index = ClusterIndex.build(data_a.clusters)
    resamples = Resamples.draw(index, settings.resamples, settings.seed)
    metrics_a, metrics_b = _metric_functions(data_a), _metric_functions(data_b)
    everything = index.all_rows()
    record: dict[str, JsonValue] = {"shared": len(left), "clusters": index.size}
    for name in METRIC_NAMES:
        difference = paired_metric_difference(resamples, metrics_a[name], metrics_b[name])
        record[name] = {
            "first": _estimate(metrics_a[name](everything)),
            "second": _estimate(metrics_b[name](everything)),
            "difference": dict(difference.to_record()),
        }
    first_wrong, second_wrong = data_a.correct == 0, data_b.correct == 0
    record["both_wrong"] = both_wrong_count(first_wrong, second_wrong)
    conditional = wrong_given_other_wrong(first_wrong, second_wrong)
    record["first_wrong_given_second_wrong"] = dict(conditional.estimate().to_record())
    return record


def model_size_comparisons(cells: Mapping[str, CellData], settings: MetricSettings) -> dict[str, JsonValue]:
    """Paired differences for the model pairs of §9, one entry per pair; a pair with a blocked member is BLOCKED."""
    record: dict[str, JsonValue] = {}
    for first_id, second_id in MODEL_SIZE_PAIRS:
        first, second = cells.get(first_id), cells.get(second_id)
        key = f"{first_id}-{second_id}"
        if first is None or second is None or first.blocked_reason or second.blocked_reason:
            record[key] = "BLOCKED"
            continue
        record[key] = compare_cells(first, second, settings)
    return record


@dataclass(frozen=True)
class TwoSignalInputs:
    """The tests of K3 with their declared criterion and the pilot's matrix signature."""

    signatures: Mapping[str, MatrixSignature]


def _matrix_aligned(signature: MatrixSignature) -> bool:
    return signature.signature in SUPPORTING_SIGNATURES


def two_signal(cell: CellData, inputs: TwoSignalInputs, threshold: float) -> dict[str, JsonValue]:
    """ADR 0005's two-signal ALIGNED for one K3 cell: the source says ALIGNED with enough confidence, the matrix is
    SPECIFIC or OVERBROAD and does not pin an off-spec test."""
    decisions = [d for d in cell.predicted if d.unit in inputs.signatures]
    chosen = [
        d
        for d in decisions
        if d.predicted_positive
        and d.confidence >= threshold
        and _matrix_aligned(inputs.signatures[d.unit])
        and inputs.signatures[d.unit].none_share <= PINS_OFFSPEC_THRESHOLD
    ]
    false_aligned = Proportion(sum(1 for d in chosen if d.truth_positive is False), len(chosen))
    source_says = [bool(d.predicted_positive) for d in decisions]
    matrix_says = [_matrix_aligned(inputs.signatures[d.unit]) for d in decisions]
    table = (
        (
            sum(1 for s, m in zip(source_says, matrix_says, strict=True) if s and m),
            sum(1 for s, m in zip(source_says, matrix_says, strict=True) if s and not m),
        ),
        (
            sum(1 for s, m in zip(source_says, matrix_says, strict=True) if not s and m),
            sum(1 for s, m in zip(source_says, matrix_says, strict=True) if not s and not m),
        ),
    )
    matrix_wrong = np.array([m != bool(d.truth_positive) for d, m in zip(decisions, matrix_says, strict=True)])
    source_wrong = np.array([not d.correct for d in decisions])
    return {
        "threshold": threshold,
        "tests": len(decisions),
        "two_signal_aligned": len(chosen),
        "false_aligned": dict(false_aligned.estimate().to_record()),
        "table_source_by_matrix": [list(row) for row in table],
        "kappa": _estimate(cohen_kappa(table)),
        "source_wrong_given_matrix_wrong": dict(
            wrong_given_other_wrong(source_wrong, matrix_wrong).estimate().to_record()
        ),
        "both_wrong": both_wrong_count(source_wrong, matrix_wrong),
    }
