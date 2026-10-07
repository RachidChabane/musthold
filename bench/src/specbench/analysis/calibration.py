"""Limit 4 per confidence source: reliability, ECE, Brier and coverage at <= 5 % error, against executed truth.

Every decision carries the source that answered (endpoint, host votes, or undetermined), its
confidence p that the claim holds (permille) and the executed truth. Metrics are additive over
fixed bins, so the repository-cluster bootstrap applies to each. Source (3), the UNDETERMINED
rate, is the share of all decisions that no source answered.
"""

from __future__ import annotations

from typing import Final

import numpy as np

from specbench.analysis.bootstrap import Columns, Floats, Statistic
from specbench.analysis.estimate import add, column, rate
from specbench.domain.records import PERMILLE, ConfidenceSource, SourceDecision, TrunkRecord

RELIABILITY_BINS: Final = 10
CONFIDENCE_BINS: Final = 10
MAX_ERROR_FOR_COVERAGE: Final = 0.05
ANSWERING_SOURCES: Final = (ConfidenceSource.ENDPOINT, ConfidenceSource.HOST_VOTES)


def _bin(value: float, bins: int) -> int:
    return min(int(value * bins), bins - 1)


def decision_counts(h: str, decision: SourceDecision, counts: dict[str, float]) -> None:
    add(counts, column(h, "cal", "decisions"))
    if decision.source is ConfidenceSource.UNDETERMINED:
        add(counts, column(h, "cal", "undetermined"))
        return
    if decision.confidence_permille is None or decision.truth is None:
        raise ValueError("an answered decision carries a confidence and an executed truth")
    s, p, truth = decision.source.value, decision.confidence_permille / PERMILLE, float(decision.truth)
    add(counts, column(h, "cal", s, "n"))
    add(counts, column(h, "cal", s, "squared_error"), (p - truth) ** 2)
    index = _bin(p, RELIABILITY_BINS)
    add(counts, column(h, "cal", s, "rel", str(index), "n"))
    add(counts, column(h, "cal", s, "rel", str(index), "p"), p)
    add(counts, column(h, "cal", s, "rel", str(index), "truth"), truth)
    confidence, predicted_true = max(p, 1.0 - p), p >= 0.5
    index = _bin((confidence - 0.5) * 2.0, CONFIDENCE_BINS)
    add(counts, column(h, "cal", s, "cov", str(index), "n"))
    add(counts, column(h, "cal", s, "cov", str(index), "wrong"), predicted_true != decision.truth)


def trunk_counts(trunk: TrunkRecord) -> dict[str, float]:
    counts: dict[str, float] = {}
    for decision in trunk.decisions:
        decision_counts(trunk.harness.value, decision, counts)
    return counts


def ece(h: str, source: ConfidenceSource) -> Statistic:
    s = source.value

    def statistic(columns: Columns) -> Floats:
        gap = sum(
            np.abs(
                columns[column(h, "cal", s, "rel", str(i), "truth")] - columns[column(h, "cal", s, "rel", str(i), "p")]
            )
            for i in range(RELIABILITY_BINS)
        )
        return rate(np.asarray(gap, dtype=np.float64), columns[column(h, "cal", s, "n")])

    return statistic


def brier(h: str, source: ConfidenceSource) -> Statistic:
    s = source.value

    def statistic(columns: Columns) -> Floats:
        return rate(columns[column(h, "cal", s, "squared_error")], columns[column(h, "cal", s, "n")])

    return statistic


def coverage_at_error(h: str, source: ConfidenceSource) -> Statistic:
    """The largest share of the source's decisions answerable by confidence threshold with error <= 5 %."""
    s = source.value

    def statistic(columns: Columns) -> Floats:
        total = columns[column(h, "cal", s, "n")]
        best = np.zeros_like(total)
        answered = np.zeros_like(total)
        wrong = np.zeros_like(total)
        for i in reversed(range(CONFIDENCE_BINS)):
            answered = answered + columns[column(h, "cal", s, "cov", str(i), "n")]
            wrong = wrong + columns[column(h, "cal", s, "cov", str(i), "wrong")]
            within = (answered > 0) & (wrong <= MAX_ERROR_FOR_COVERAGE * answered)
            best = np.where(within, np.maximum(best, answered), best)
        return np.where(total > 0, best / np.where(total > 0, total, 1.0), np.nan)

    return statistic


def undetermined_rate(h: str) -> Statistic:
    def statistic(columns: Columns) -> Floats:
        return rate(columns[column(h, "cal", "undetermined")], columns[column(h, "cal", "decisions")])

    return statistic
