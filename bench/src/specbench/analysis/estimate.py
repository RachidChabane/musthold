"""Estimates with cluster-bootstrap intervals, and the shared column vocabulary of the analysis."""

from __future__ import annotations

import math
from dataclasses import dataclass
from typing import Final

import numpy as np

from specbench.analysis.bootstrap import (
    ANALYSIS_SEED,
    BOOTSTRAP_RESAMPLES,
    ClusterTable,
    Floats,
    Statistic,
    bootstrap_interval,
    point_estimate,
    safe_divide,
)
from specbench.domain.canonical import JsonValue

COLUMN_SEPARATOR: Final = "|"


def column(*parts: str) -> str:
    """The name of a count column: its parts joined, e.g. `claude-code|TR|std|acc`."""
    return COLUMN_SEPARATOR.join(parts)


def add(counts: dict[str, float], name: str, amount: float = 1.0) -> None:
    counts[name] = counts.get(name, 0.0) + amount


@dataclass(frozen=True)
class AnalysisConfig:
    resamples: int = BOOTSTRAP_RESAMPLES
    seed: int = ANALYSIS_SEED


@dataclass(frozen=True)
class Estimate:
    value: float
    lo: float
    hi: float

    def to_record(self) -> dict[str, JsonValue]:
        return {"estimate": _finite(self.value), "hi": _finite(self.hi), "lo": _finite(self.lo)}


def _finite(value: float) -> float | None:
    return value if math.isfinite(value) else None


def estimate(table: ClusterTable, statistic: Statistic, config: AnalysisConfig) -> Estimate:
    """Point estimate on the pooled counts and its percentile interval over repository-cluster resamples."""
    interval = bootstrap_interval(table, statistic, config.resamples, config.seed)
    return Estimate(point_estimate(table, statistic), interval.lo, interval.hi)


def rate(numerator: Floats, denominator: Floats) -> Floats:
    """`numerator / denominator`, NaN where the denominator is zero."""
    return safe_divide(numerator, denominator)


def one_minus_ratio(rate_a: Floats, rate_b: Floats) -> Floats:
    """`1 - rate_a / rate_b`, NaN where either rate is undefined or `rate_b` is zero."""
    positive = np.where(np.isnan(rate_b), 0.0, rate_b) > 0
    out = np.full(rate_a.shape, np.nan)
    np.divide(rate_a, rate_b, out=out, where=positive)
    return 1.0 - out


def total(table: ClusterTable, name: str) -> int:
    """The pooled count of one column; zero when the column does not exist."""
    if name not in table.columns:
        return 0
    index = table.columns.index(name)
    return int(sum(row[index] for row in table.rows))
