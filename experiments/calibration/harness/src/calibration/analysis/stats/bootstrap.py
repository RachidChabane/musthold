"""Cluster bootstrap with common random numbers, identical on every platform.

Clusters are ordered by sorted id before any draw, so the draws depend only on the
number of clusters, the number of resamples and the seed. Counts are accumulated
cluster by cluster with elementwise operations in a fixed order (no BLAS), and every
other sum is an exact `math.fsum`.
"""

from __future__ import annotations

import math
from collections.abc import Callable, Sequence
from dataclasses import dataclass
from typing import Final

import numpy as np

from calibration.analysis.stats.arrays import NAN, Floats, Rows
from calibration.analysis.stats.intervals import UNDEFINED_INTERVAL, Interval
from calibration.domain.constants import BOOTSTRAP_RESAMPLES, SEED

LOWER_PERCENTILE: Final = 2.5
UPPER_PERCENTILE: Final = 97.5

Statistic = Callable[[Rows], float]
CountCombiner = Callable[[Floats], Floats]


@dataclass(frozen=True)
class ClusterIndex:
    """Row positions grouped by cluster: clusters by sorted id, rows by ascending position."""

    ids: tuple[str, ...]
    rows: tuple[Rows, ...]
    row_count: int

    @classmethod
    def build(cls, clusters: Sequence[str]) -> ClusterIndex:
        positions: dict[str, list[int]] = {}
        for position, cluster in enumerate(clusters):
            positions.setdefault(cluster, []).append(position)
        ids = tuple(sorted(positions))
        rows = tuple(np.array(positions[cluster], dtype=np.intp) for cluster in ids)
        return cls(ids, rows, len(clusters))

    @property
    def size(self) -> int:
        return len(self.ids)

    def all_rows(self) -> Rows:
        return np.arange(self.row_count, dtype=np.intp)

    def cluster_sums(self, row_columns: Floats) -> Floats:
        """Exact per-cluster sums of row-level columns (rows x columns) -> (clusters x columns)."""
        width = row_columns.shape[1]
        sums = np.zeros((self.size, width), dtype=np.float64)
        for cluster, rows in enumerate(self.rows):
            block = row_columns[rows]
            for column in range(width):
                sums[cluster, column] = math.fsum(block[:, column].tolist())
        return sums


def percentile_interval(values: Floats) -> Interval:
    """Percentile interval [2.5, 97.5] ignoring undefined values; undefined when none remain."""
    finite = values[~np.isnan(values)]
    if finite.size == 0:
        return UNDEFINED_INTERVAL
    lo, hi = np.percentile(finite, [LOWER_PERCENTILE, UPPER_PERCENTILE])
    return Interval(float(lo), float(hi))


@dataclass(frozen=True)
class Resamples:
    """Multiplicities of whole clusters drawn once with replacement, reused by every statistic."""

    index: ClusterIndex
    multiplicities: Floats

    @classmethod
    def draw(cls, index: ClusterIndex, resamples: int = BOOTSTRAP_RESAMPLES, seed: int = SEED) -> Resamples:
        if index.size == 0:
            return cls(index, np.zeros((resamples, 0), dtype=np.float64))
        generator = np.random.default_rng(seed)
        draws = generator.multinomial(index.size, np.full(index.size, 1.0 / index.size), size=resamples)
        return cls(index, draws.astype(np.float64))

    @property
    def count(self) -> int:
        return self.multiplicities.shape[0]

    def rows_of(self, resample: int) -> Rows:
        """Concatenated row positions of one resample; a cluster appears once per multiplicity."""
        order = np.repeat(np.arange(self.index.size), self.multiplicities[resample].astype(np.intp))
        if order.size == 0:
            return np.empty(0, dtype=np.intp)
        return np.concatenate([self.index.rows[cluster] for cluster in order])

    def values(self, statistic: Statistic) -> Floats:
        if self.index.size == 0:
            return np.full(self.count, NAN)
        return np.array([statistic(self.rows_of(resample)) for resample in range(self.count)], dtype=np.float64)

    def summed_counts(self, per_cluster: Floats) -> Floats:
        """Multiplicity-weighted sums of per-cluster additive columns (clusters x columns) -> (resamples x columns)."""
        sums = np.zeros((self.count, per_cluster.shape[1]), dtype=np.float64)
        for cluster in range(self.index.size):
            sums += self.multiplicities[:, cluster : cluster + 1] * per_cluster[cluster]
        return sums

    def count_values(self, per_cluster: Floats, combine: CountCombiner) -> Floats:
        """A statistic that is a function of additive counts, evaluated for every resample at once."""
        if self.index.size == 0:
            return np.full(self.count, NAN)
        return combine(self.summed_counts(per_cluster))

    def interval(self, statistic: Statistic) -> Interval:
        return percentile_interval(self.values(statistic))

    def paired_difference(self, statistic_a: Statistic, statistic_b: Statistic) -> Interval:
        """Percentile interval of the per-resample differences a - b; resamples where either is undefined drop out."""
        return percentile_interval(self.values(statistic_a) - self.values(statistic_b))
