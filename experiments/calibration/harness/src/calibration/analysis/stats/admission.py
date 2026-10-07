"""Admission test of section 10: isotonic calibration on half of the clusters, a verdict on the other half."""

from __future__ import annotations

import hashlib
import math
from collections.abc import Callable, Iterable, Sequence
from dataclasses import dataclass
from enum import StrEnum
from typing import Final

import numpy as np
from numpy.typing import NDArray

from calibration.analysis.stats.arrays import NAN, Flags, Floats, Mask, Rows, json_number, safe_ratio
from calibration.analysis.stats.bootstrap import ClusterIndex, Resamples, percentile_interval
from calibration.analysis.stats.intervals import Proportion, wilson
from calibration.domain.constants import BOOTSTRAP_RESAMPLES, HASH_SEPARATOR, SEED, SEED_TEXT

MIN_FIT_DECISIONS: Final = 200
MIN_FIT_CLUSTERS: Final = 10
CALIBRATED_THRESHOLD: Final = 0.95
ERROR_POINT_LIMIT: Final = 0.05
ERROR_UPPER_LIMIT: Final = 0.10
MIN_COVERAGE: Final = 0.30
HALF_DIVISOR: Final = 2


class AdmissionOutcome(StrEnum):
    MET = "MET"
    NOT_MET = "NOT_MET"
    UNREACHABLE = "UNREACHABLE"
    INCONCLUSIVE = "INCONCLUSIVE"


@dataclass(frozen=True)
class IsotonicFit:
    """Non-decreasing step values at the sorted distinct confidences of the fit half."""

    confidences: Floats
    values: Floats


@dataclass(frozen=True)
class _Block:
    correct: int
    weight: int
    width: int

    def mean_exceeds(self, other: _Block) -> bool:
        return self.correct * other.weight > other.correct * self.weight


def isotonic_fit(confidence: Floats, correct: Flags) -> IsotonicFit:
    """Pool-adjacent-violators on the per-confidence correct counts, in exact integer arithmetic."""
    distinct, inverse = np.unique(confidence, return_inverse=True)
    weights = np.bincount(inverse, minlength=distinct.size)
    corrects = np.bincount(inverse, weights=correct, minlength=distinct.size)
    stack: list[_Block] = []
    for weight, hits in zip(weights.tolist(), corrects.tolist(), strict=True):
        block = _Block(int(hits), int(weight), 1)
        while stack and stack[-1].mean_exceeds(block):
            previous = stack.pop()
            block = _Block(
                previous.correct + block.correct, previous.weight + block.weight, previous.width + block.width
            )
        stack.append(block)
    values = np.repeat([b.correct / b.weight for b in stack], [b.width for b in stack]).astype(np.float64)
    return IsotonicFit(distinct.astype(np.float64), values)


def calibrated_values(fit: IsotonicFit, confidence: Floats) -> Floats:
    """Fitted value of the largest fit confidence at or below each confidence; the lowest fitted value below all."""
    if fit.values.size == 0:
        return np.full(confidence.shape, NAN)
    positions = np.searchsorted(fit.confidences, confidence, side="right") - 1
    return fit.values[np.maximum(positions, 0)]


def calibrated_value(fit: IsotonicFit, confidence: float) -> float:
    return float(calibrated_values(fit, np.array([confidence], dtype=np.float64))[0])


@dataclass(frozen=True)
class ClusterSplit:
    fit_half: tuple[str, ...]
    held_out: tuple[str, ...]


def _split_key(cluster_id: str) -> tuple[str, str]:
    digest = hashlib.sha256(f"{SEED_TEXT}{HASH_SEPARATOR}{cluster_id}".encode()).hexdigest()
    return digest, cluster_id


def split_clusters(cluster_ids: Iterable[str]) -> ClusterSplit:
    """Order the distinct clusters by sha256 of seed and id; the first ceil(k/2) fit, the rest are held out."""
    ordered = sorted(set(cluster_ids), key=_split_key)
    fit_size = (len(ordered) + 1) // HALF_DIVISOR
    return ClusterSplit(tuple(ordered[:fit_size]), tuple(ordered[fit_size:]))


StringArray = NDArray[np.str_]
ErrorUpperBound = Callable[[Rows], float]


def wilson_error_upper(correct: Flags) -> ErrorUpperBound:
    """Wilson upper bound of the error among the given rows."""

    def upper(rows: Rows) -> float:
        errors = int((1 - correct[rows]).sum())
        return wilson(errors, int(rows.size)).hi

    return upper


def bootstrap_error_upper(
    clusters: Sequence[str], correct: Flags, resamples: int = BOOTSTRAP_RESAMPLES, seed: int = SEED
) -> ErrorUpperBound:
    """Cluster-bootstrap 97.5th percentile of the error among the given rows."""

    def upper(rows: Rows) -> float:
        positions: list[int] = rows.tolist()
        index = ClusterIndex.build([clusters[position] for position in positions])
        counts = np.column_stack((np.ones(rows.size), (1 - correct[rows]).astype(np.float64)))
        draws = Resamples.draw(index, resamples, seed)
        errors = draws.count_values(index.cluster_sums(counts), lambda sums: _ratio_column(sums[:, 1], sums[:, 0]))
        return percentile_interval(errors).hi

    return upper


def _ratio_column(numerator: Floats, denominator: Floats) -> Floats:
    out = np.full(numerator.shape, NAN)
    np.divide(numerator, denominator, out=out, where=denominator != 0)
    return out


@dataclass(frozen=True)
class AdmissionResult:
    outcome: AdmissionOutcome
    fit_decisions: int
    fit_clusters: int
    held_out_decisions: int
    held_out_clusters: int
    selected_decisions: int
    selected_errors: int
    error_point: float
    error_upper: float
    coverage: float

    def to_record(self) -> dict[str, float | int | str | None]:
        return {
            "outcome": self.outcome.value,
            "fit_decisions": self.fit_decisions,
            "fit_clusters": self.fit_clusters,
            "held_out_decisions": self.held_out_decisions,
            "held_out_clusters": self.held_out_clusters,
            "selected_decisions": self.selected_decisions,
            "selected_errors": self.selected_errors,
            "error_point": json_number(self.error_point),
            "error_upper": json_number(self.error_upper),
            "coverage": json_number(self.coverage),
        }


@dataclass(frozen=True)
class _Selection:
    n: int = 0
    errors: int = 0
    point: float = NAN
    upper: float = NAN
    coverage: float = NAN


_NO_SELECTION: Final = _Selection()


def _result(
    outcome: AdmissionOutcome, fit: Mask, held: Mask, clusters: StringArray, selection: _Selection = _NO_SELECTION
) -> AdmissionResult:
    return AdmissionResult(
        outcome=outcome,
        fit_decisions=int(fit.sum()),
        fit_clusters=len(set(clusters[fit].tolist())),
        held_out_decisions=int(held.sum()),
        held_out_clusters=len(set(clusters[held].tolist())),
        selected_decisions=selection.n,
        selected_errors=selection.errors,
        error_point=selection.point,
        error_upper=selection.upper,
        coverage=selection.coverage,
    )


def _preconditions_failed(fit: Mask, clusters: StringArray, item_set_could_supply: bool) -> AdmissionOutcome | None:
    enough = int(fit.sum()) >= MIN_FIT_DECISIONS and len(set(clusters[fit].tolist())) >= MIN_FIT_CLUSTERS
    if enough:
        return None
    return AdmissionOutcome.INCONCLUSIVE if item_set_could_supply else AdmissionOutcome.UNREACHABLE


def _verdict(point: float, upper: float, coverage: float) -> AdmissionOutcome:
    if math.isnan(upper):
        return AdmissionOutcome.INCONCLUSIVE
    met = point <= ERROR_POINT_LIMIT and upper <= ERROR_UPPER_LIMIT and coverage >= MIN_COVERAGE
    return AdmissionOutcome.MET if met else AdmissionOutcome.NOT_MET


def admission_test(
    clusters: Sequence[str],
    confidence: Floats,
    correct: Flags,
    *,
    item_set_could_supply: bool,
    row_mask: Mask | None = None,
    error_upper: ErrorUpperBound | None = None,
) -> AdmissionResult:
    """The four steps of section 10 on labelled decisions.

    The fit and held-out halves are split over the clusters of every row, so each
    restricted variant (`row_mask`) shares the halves of the unrestricted test. Counts,
    preconditions and coverage then refer to the decisions inside the mask. `error_upper`
    replaces the Wilson upper bound of the error (default) with a caller-supplied one.
    """
    upper_bound = error_upper if error_upper is not None else wilson_error_upper(correct)
    cluster_array = np.array(clusters)
    active = np.ones(len(clusters), dtype=np.bool_) if row_mask is None else row_mask
    fit_clusters = set(split_clusters(clusters).fit_half)
    in_fit_half = np.array([cluster in fit_clusters for cluster in clusters], dtype=np.bool_)
    fit_rows, held_rows = active & in_fit_half, active & ~in_fit_half
    failed = _preconditions_failed(fit_rows, cluster_array, item_set_could_supply)
    if failed is not None:
        return _result(failed, fit_rows, held_rows, cluster_array)
    fit = isotonic_fit(confidence[fit_rows], correct[fit_rows])
    held_positions = np.flatnonzero(held_rows)
    if held_positions.size == 0:
        return _result(AdmissionOutcome.INCONCLUSIVE, fit_rows, held_rows, cluster_array)
    calibrated = calibrated_values(fit, confidence[held_positions])
    chosen = held_positions[calibrated >= CALIBRATED_THRESHOLD]
    coverage = chosen.size / held_positions.size
    if chosen.size == 0:
        return _result(AdmissionOutcome.NOT_MET, fit_rows, held_rows, cluster_array, _Selection(coverage=coverage))
    errors = int((1 - correct[chosen]).sum())
    point = safe_ratio(errors, chosen.size)
    upper = upper_bound(chosen.astype(np.intp))
    selection = _Selection(int(chosen.size), errors, point, upper, coverage)
    return _result(_verdict(point, upper, coverage), fit_rows, held_rows, cluster_array, selection)


def majority_rule_error_at_coverage(correct_flags: Flags, coverage: float) -> Proportion:
    """Error of the majority-class rule over its first ceil(coverage * n) decisions, in the order given.

    The rule carries no ranking of its own, so the caller supplies the decisions in the
    order in which the compared rule would admit them.
    """
    admitted = min(int(correct_flags.size), math.ceil(coverage * correct_flags.size))
    return Proportion(int((1 - correct_flags[:admitted]).sum()), admitted)
