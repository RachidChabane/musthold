"""The admission test of §10 for one cell: the ADR rule, and for K4 the variants reported beside it."""

from __future__ import annotations

import numpy as np

from calibration.analysis.cell_metrics import MetricInput, MetricSettings
from calibration.analysis.model import CellData, Kind
from calibration.analysis.stats.admission import (
    MIN_FIT_CLUSTERS,
    MIN_FIT_DECISIONS,
    AdmissionOutcome,
    AdmissionResult,
    admission_test,
    bootstrap_error_upper,
    majority_rule_error_at_coverage,
    split_clusters,
)
from calibration.domain.canonical import JsonValue


def item_set_could_supply(cell: CellData) -> bool:
    """Whether the cell's item set (fall-throughs included) holds the fit-half minimum of §10: at least 200 units in
    at least 10 clusters."""
    fit = set(split_clusters(cell.unit_clusters).fit_half)
    units = sum(1 for cluster in cell.unit_clusters if cluster in fit)
    return units >= MIN_FIT_DECISIONS and len(fit) >= MIN_FIT_CLUSTERS


def _record(result: AdmissionResult) -> dict[str, JsonValue]:
    return dict(result.to_record())


def _majority_error(data: MetricInput, result: AdmissionResult) -> dict[str, JsonValue]:
    """The error of the majority-class rule over the same share of the held-out decisions, in confidence order."""
    held_clusters = set(split_clusters(data.clusters).held_out)
    held = np.array([cluster in held_clusters for cluster in data.clusters], dtype=np.bool_)
    if not held.any() or np.isnan(result.coverage):
        return {"errors": None, "n": 0, "rate": None}
    truth = data.truth[held]
    majority = int(truth.sum() * 2 >= truth.size)
    order = np.argsort(-data.confidence[held], kind="stable")
    flags = (truth[order] == majority).astype(np.int64)
    proportion = majority_rule_error_at_coverage(flags, result.coverage)
    return {"errors": proportion.successes, "n": proportion.total, "rate": proportion.point}


def cell_admission(cell: CellData, settings: MetricSettings) -> tuple[dict[str, JsonValue], AdmissionOutcome]:
    """The admission record of a cell: top-label confidence against correctness, split by cluster."""
    decisions = cell.predicted
    data = MetricInput.of(cell.kind, decisions)
    could_supply = item_set_could_supply(cell)
    main = admission_test(data.clusters, data.confidence, data.correct, item_set_could_supply=could_supply)
    record: dict[str, JsonValue] = {"main": _record(main), "item_set_could_supply": could_supply}
    if cell.kind is not Kind.K4 or not decisions:
        return record, main.outcome
    upper = bootstrap_error_upper(data.clusters, data.correct, settings.resamples, settings.seed)
    variants: dict[str, JsonValue] = {
        "bootstrap_upper": _record(
            admission_test(
                data.clusters, data.confidence, data.correct, item_set_could_supply=could_supply, error_upper=upper
            )
        ),
        "predicting_violates": _record(
            admission_test(
                data.clusters,
                data.confidence,
                data.correct,
                item_set_could_supply=could_supply,
                row_mask=data.predicted_positive,
            )
        ),
        "predicting_consistent": _record(
            admission_test(
                data.clusters,
                data.confidence,
                data.correct,
                item_set_could_supply=could_supply,
                row_mask=~data.predicted_positive,
            )
        ),
        "majority_rule": _majority_error(data, main),
    }
    record["variants"] = variants
    return record, main.outcome
