"""Precision table of Wilson intervals (PREREG 13, `precision-table` verb)."""

from __future__ import annotations

import math

from pilot.analysis.stats import wilson
from pilot.analysis.table import Table
from pilot.domain.analysis_constants import (
    PRECISION_DENOMINATORS,
    PRECISION_FULL_COUNT_DENOMINATORS,
    PRECISION_PROPORTIONS,
)

HEADER = ("table", "n", "requested_proportion", "k", "proportion", "wilson_lo", "wilson_hi", "width")
DECIMALS = 4
GRID = "grid"
FULL = "full"
HALF = 0.5


def count_at(proportion: float, n: int) -> int:
    """Nearest count to the proportion; halves round up."""
    return math.floor(proportion * n + HALF)


def precision_table() -> Table:
    rows: list[list[object]] = []
    for n in PRECISION_DENOMINATORS:
        for proportion in PRECISION_PROPORTIONS:
            rows.append(_row(GRID, n, count_at(proportion, n), proportion))
    for n in PRECISION_FULL_COUNT_DENOMINATORS:
        for count in range(n + 1):
            rows.append(_row(FULL, n, count, None))
    return Table("precision-table", "Wilson 95% intervals by denominator and count", list(HEADER), rows, DECIMALS)


def _row(kind: str, n: int, count: int, requested: float | None) -> list[object]:
    interval = wilson(count, n)
    return [kind, n, requested, count, count / n, interval.lo, interval.hi, interval.width]
