"""Cluster bootstrap by repository over additive counts, and Wilson intervals. Pure functions of counts and a seed.

Each cluster is one row of additive counts. Rows are ordered by content before any
draw, so the resampling depends neither on cluster labels nor on the order in which
rows were supplied: relabelling or shuffling clusters leaves every interval unchanged.
"""

from __future__ import annotations

import math
from collections.abc import Callable, Iterator, Mapping, Sequence
from dataclasses import dataclass
from typing import Final

import numpy as np
from numpy.typing import NDArray

Floats = NDArray[np.float64]
Columns = Mapping[str, Floats]
Statistic = Callable[[Columns], Floats]

BOOTSTRAP_RESAMPLES: Final = 10_000
ANALYSIS_SEED: Final = 20261007
CONFIDENCE_LEVEL: Final = 0.95
WILSON_Z: Final = 1.959963984540054
LOWER_PERCENTILE: Final = 100.0 * (1.0 - CONFIDENCE_LEVEL) / 2.0
UPPER_PERCENTILE: Final = 100.0 - LOWER_PERCENTILE
NAN: Final = float("nan")


@dataclass(frozen=True)
class Interval:
    lo: float
    hi: float

    @property
    def is_defined(self) -> bool:
        return not (math.isnan(self.lo) or math.isnan(self.hi))


UNDEFINED: Final = Interval(NAN, NAN)


@dataclass(frozen=True)
class ClusterTable:
    """Additive counts per cluster; rows are kept in content order."""

    columns: tuple[str, ...]
    rows: tuple[tuple[float, ...], ...]

    @classmethod
    def build(cls, columns: Sequence[str], clusters: Mapping[str, Mapping[str, float]]) -> ClusterTable:
        rows = sorted(tuple(float(values.get(name, 0.0)) for name in columns) for values in clusters.values())
        return cls(tuple(columns), tuple(rows))

    @property
    def size(self) -> int:
        return len(self.rows)

    def matrix(self) -> Floats:
        return np.array(self.rows, dtype=np.float64).reshape(len(self.rows), len(self.columns))


class CountColumns(Mapping[str, Floats]):
    """Summed counts by column name; a column no cluster ever contributed to reads as zeros."""

    def __init__(self, values: Mapping[str, Floats], length: int) -> None:
        self._values = values
        self._zeros = np.zeros(length, dtype=np.float64)

    def __getitem__(self, name: str) -> Floats:
        return self._values.get(name, self._zeros)

    def __iter__(self) -> Iterator[str]:
        return iter(self._values)

    def __len__(self) -> int:
        return len(self._values)

    def __contains__(self, name: object) -> bool:
        return True


def _columns(names: Sequence[str], summed: Floats) -> CountColumns:
    return CountColumns({name: summed[:, index] for index, name in enumerate(names)}, summed.shape[0])


def point_estimate(table: ClusterTable, statistic: Statistic) -> float:
    """The statistic over the totals of every cluster; totals are exact sums, whatever the platform."""
    totals = np.array([[math.fsum(values) for values in zip(*table.rows, strict=True)]], dtype=np.float64)
    return float(statistic(_columns(table.columns, totals.reshape(1, len(table.columns))))[0])


def accumulate(draws: Floats, matrix: Floats) -> Floats:
    """`draws @ matrix` as a fixed-order sum over clusters: elementwise operations only, so no BLAS and no platform drift."""
    sums = np.zeros((draws.shape[0], matrix.shape[1]), dtype=np.float64)
    for cluster in range(matrix.shape[0]):
        sums += draws[:, cluster : cluster + 1] * matrix[cluster]
    return sums


@dataclass(frozen=True)
class Resamples:
    """The summed counts of `resamples` draws of whole clusters with replacement, drawn once for every statistic."""

    columns: tuple[str, ...]
    sums: Floats
    draws: int

    @classmethod
    def draw(cls, table: ClusterTable, resamples: int, seed: int) -> Resamples:
        if table.size == 0:
            return cls(table.columns, np.zeros((resamples, len(table.columns))), resamples)
        rng = np.random.default_rng(seed)
        counts = rng.multinomial(table.size, np.full(table.size, 1.0 / table.size), size=resamples)
        return cls(table.columns, accumulate(counts.astype(np.float64), table.matrix()), resamples)

    def of(self, statistic: Statistic) -> Floats:
        """The statistic over every draw; undefined (NaN) throughout when the table had no cluster."""
        return statistic(_columns(self.columns, self.sums))


def resample(table: ClusterTable, statistic: Statistic, resamples: int, seed: int) -> Floats:
    """The statistic over `resamples` draws of whole clusters with replacement."""
    if table.size == 0:
        return np.full(resamples, NAN)
    return Resamples.draw(table, resamples, seed).of(statistic)


def percentile_interval(samples: Floats) -> Interval:
    finite = samples[~np.isnan(samples)]
    if finite.size == 0:
        return UNDEFINED
    lo, hi = np.percentile(finite, [LOWER_PERCENTILE, UPPER_PERCENTILE])
    return Interval(float(lo), float(hi))


def bootstrap_interval(
    table: ClusterTable, statistic: Statistic, resamples: int = BOOTSTRAP_RESAMPLES, seed: int = ANALYSIS_SEED
) -> Interval:
    return percentile_interval(resample(table, statistic, resamples, seed))


def wilson(successes: int, total: int) -> Interval:
    """Wilson score interval of a single proportion; the whole unit interval when nothing was observed."""
    if total <= 0:
        return Interval(0.0, 1.0)
    p = successes / total
    z2 = WILSON_Z**2
    centre = (p + z2 / (2 * total)) / (1 + z2 / total)
    half = WILSON_Z * math.sqrt(p * (1 - p) / total + z2 / (4 * total**2)) / (1 + z2 / total)
    return Interval(max(0.0, centre - half), min(1.0, centre + half))


def safe_divide(numerator: Floats, denominator: Floats) -> Floats:
    out = np.full(numerator.shape, NAN)
    np.divide(numerator, denominator, out=out, where=denominator != 0)
    return out
