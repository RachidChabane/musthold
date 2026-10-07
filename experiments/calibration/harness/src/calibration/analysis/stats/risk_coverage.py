"""Risk-coverage curve, its summaries and the screen and gate quantities that are proportions of counts."""

from __future__ import annotations

import math
from collections.abc import Sequence
from dataclasses import dataclass
from typing import Final

import numpy as np

from calibration.analysis.stats.arrays import NAN, Flags, Floats, Mask
from calibration.analysis.stats.bootstrap import Resamples, percentile_interval
from calibration.analysis.stats.intervals import (
    UNDEFINED_INTERVAL,
    Interval,
    Proportion,
    decision_interval,
    wilson,
)
from calibration.domain.constants import C05_RISK, C05U_UPPER

THRESHOLD_CHUNK: Final = 64
NO_COVERAGE: Final = 0.0


@dataclass(frozen=True)
class RiskPoint:
    """The decisions at or above `threshold`: how many, how many wrong, their share of all decisions."""

    threshold: float
    n: int
    errors: int
    coverage: float

    @property
    def risk(self) -> float:
        return self.errors / self.n


RiskCoverageCurve = tuple[RiskPoint, ...]


def risk_coverage_curve(p: Floats, correct: Flags) -> RiskCoverageCurve:
    """One point per distinct confidence, from the highest threshold down; ties form one block."""
    total = int(p.size)
    if total == 0:
        return ()
    order = np.argsort(-p, kind="stable")
    ordered = p[order]
    cumulative_errors = np.cumsum(1 - correct[order])
    block_ends = np.flatnonzero(np.concatenate((ordered[1:] != ordered[:-1], [True])))
    return tuple(
        RiskPoint(float(ordered[end]), int(end) + 1, int(cumulative_errors[end]), (int(end) + 1) / total)
        for end in block_ends
    )


def aurc(curve: RiskCoverageCurve) -> float:
    """Area under risk against coverage with right-continuous steps.

    Each block of tied confidences adds its coverage increment times the cumulative risk
    at its own threshold, i.e. sum_i (coverage_i - coverage_{i-1}) * risk_i with coverage_0 = 0.
    """
    if not curve:
        return NAN
    previous_coverage = 0.0
    terms: list[float] = []
    for point in curve:
        terms.append((point.coverage - previous_coverage) * point.risk)
        previous_coverage = point.coverage
    return math.fsum(terms)


def _largest_coverage(curve: RiskCoverageCurve, eligible: Sequence[bool]) -> float:
    if not curve:
        return NAN
    return max((point.coverage for point, ok in zip(curve, eligible, strict=True) if ok), default=NO_COVERAGE)


def c05(curve: RiskCoverageCurve, risk_limit: float = C05_RISK) -> float:
    """Largest coverage whose risk point estimate is within the limit; 0 when no point is."""
    return _largest_coverage(curve, [point.risk <= risk_limit for point in curve])


def decision_upper_bounds(curve: RiskCoverageCurve, bootstrap_upper: Floats) -> Floats:
    """Per point, the upper end of the wider of the Wilson and bootstrap intervals of its risk; nan when undefined."""
    uppers: list[float] = []
    for point, bootstrap_hi in zip(curve, bootstrap_upper.tolist(), strict=True):
        interval = decision_interval(wilson(point.errors, point.n), _upper_only(bootstrap_hi))
        uppers.append(interval.hi)
    return np.array(uppers, dtype=np.float64)


def _upper_only(upper: float) -> Interval:
    """An interval carrying only its upper end: the lower end is the least possible risk."""
    return UNDEFINED_INTERVAL if math.isnan(upper) else Interval(0.0, upper)


def c05u(curve: RiskCoverageCurve, bootstrap_upper: Floats, upper_limit: float = C05U_UPPER) -> float:
    """Largest coverage whose risk decision-interval upper bound is within the limit; 0 when no point is."""
    uppers = decision_upper_bounds(curve, bootstrap_upper)
    return _largest_coverage(curve, [bool(upper <= upper_limit) for upper in uppers.tolist()])


def risk_upper_bounds(resamples: Resamples, p: Floats, correct: Flags, thresholds: Sequence[float]) -> Floats:
    """97.5th percentile of the risk at each threshold over all resamples, evaluated for every threshold at once."""
    cut_offs = np.array(thresholds, dtype=np.float64)
    errors = (1 - correct).astype(np.float64)
    uppers = np.full(cut_offs.size, NAN)
    for start in range(0, cut_offs.size, THRESHOLD_CHUNK):
        chunk = cut_offs[start : start + THRESHOLD_CHUNK]
        covered = (p[:, np.newaxis] >= chunk[np.newaxis, :]).astype(np.float64)
        columns = np.concatenate((covered, covered * errors[:, np.newaxis]), axis=1)
        summed = resamples.summed_counts(resamples.index.cluster_sums(columns))
        decided, wrong = summed[:, : chunk.size], summed[:, chunk.size :]
        risk = np.full(decided.shape, NAN)
        np.divide(wrong, decided, out=risk, where=decided != 0)
        for offset in range(chunk.size):
            uppers[start + offset] = percentile_interval(risk[:, offset]).hi
    return uppers


def miss_rate(p: Floats, y: Flags, tau: float) -> Proportion:
    """Share of positive-truth rows whose positive-event probability is below tau."""
    positives = y == 1
    return Proportion(int((positives & (p < tau)).sum()), int(positives.sum()))


def flag_rate(p: Floats, y: Flags, tau: float) -> Proportion:
    """Share of all rows whose positive-event probability is at or above tau."""
    return Proportion(int((p >= tau).sum()), int(y.size))


def false_aligned_rate(
    confidence: Floats, predicted_positive: Mask, truth_positive: Mask, threshold: float
) -> Proportion:
    """Share of truth-negative rows among those predicted positive with confidence at or above the threshold."""
    selected = predicted_positive & (confidence >= threshold)
    return Proportion(int((selected & ~truth_positive).sum()), int(selected.sum()))


def false_aligned_curve(
    confidence: Floats, predicted_positive: Mask, truth_positive: Mask, thresholds: Sequence[float]
) -> list[Proportion]:
    return [false_aligned_rate(confidence, predicted_positive, truth_positive, t) for t in thresholds]
