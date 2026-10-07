"""Calibration metrics: expected calibration error, reliability table, Brier score and accuracy."""

from __future__ import annotations

import math
from dataclasses import dataclass
from typing import Final

import numpy as np

from calibration.analysis.stats.arrays import NAN, Flags, Floats, Mask, exact_mean, exact_sum, safe_ratio
from calibration.analysis.stats.intervals import Interval, Proportion, wilson
from calibration.domain.constants import ECE_BINS

MAX_ECE: Final = 1.0


@dataclass(frozen=True)
class ReliabilityRow:
    lo: float
    hi: float
    n: int
    mean_confidence: float
    frequency: float
    frequency_interval: Interval


def equal_width_bin_indices(p: Floats, bins: int) -> Flags:
    """Bin of each value on [0, 1] in `bins` equal-width bins; the last bin is closed."""
    return np.minimum(np.floor(p * bins).astype(np.int64), bins - 1)


def _bin_gap(p: Floats, y: Flags, members: Mask) -> tuple[int, float]:
    count = int(members.sum())
    return count, abs(exact_mean(p[members]) - exact_mean(y[members]))


def _weighted_gap(p: Floats, y: Flags, groups: list[Mask]) -> float:
    total = p.size
    terms = [count / total * gap for count, gap in (_bin_gap(p, y, members) for members in groups) if count > 0]
    return min(math.fsum(terms), MAX_ECE)


def ece_equal_width(p: Floats, y: Flags, bins: int = ECE_BINS) -> float:
    """sum_b n_b/N |mean p_b - frequency_b| over equal-width bins; empty bins are skipped."""
    if p.size == 0:
        return NAN
    indices = equal_width_bin_indices(p, bins)
    return _weighted_gap(p, y, [indices == bin_index for bin_index in range(bins)])


def equal_mass_groups(p: Floats, y: Flags, bins: int) -> list[Mask]:
    """Masks of `bins` groups of near-equal size over the values ordered by (p, y), a position-free total order."""
    order = np.lexsort((y, p))
    masks: list[Mask] = []
    for chunk in np.array_split(order, bins):
        mask = np.zeros(p.size, dtype=np.bool_)
        mask[chunk] = True
        masks.append(mask)
    return masks


def ece_equal_mass(p: Floats, y: Flags, bins: int) -> float:
    if p.size == 0:
        return NAN
    return _weighted_gap(p, y, equal_mass_groups(p, y, bins))


def reliability_table(p: Floats, y: Flags, bins: int = ECE_BINS) -> list[ReliabilityRow]:
    """One row per equal-width bin, empty bins included with undefined mean and frequency."""
    indices = equal_width_bin_indices(p, bins)
    rows: list[ReliabilityRow] = []
    for bin_index in range(bins):
        members = indices == bin_index
        count = int(members.sum())
        positives = int(y[members].sum())
        rows.append(
            ReliabilityRow(
                lo=bin_index / bins,
                hi=(bin_index + 1) / bins,
                n=count,
                mean_confidence=exact_mean(p[members]),
                frequency=safe_ratio(positives, count),
                frequency_interval=wilson(positives, count),
            )
        )
    return rows


def brier(p: Floats, y: Flags) -> float:
    if p.size == 0:
        return NAN
    return exact_sum((p - y) ** 2) / p.size


def brier_skill(p: Floats, y: Flags) -> float:
    """1 - Brier / Brier of the constant base-rate predictor of the same set; undefined when that one is perfect."""
    if p.size == 0:
        return NAN
    base_rate = exact_mean(y)
    deviation = y.astype(np.float64) - base_rate
    reference = exact_sum(deviation * deviation) / p.size
    if reference == 0:
        return NAN
    return 1.0 - brier(p, y) / reference


def accuracy_in_window(p: Floats, y: Flags, low: float, high: float) -> Proportion:
    """Observed accuracy among decisions whose confidence lies in [low, high)."""
    members = (p >= low) & (p < high)
    return Proportion(int(y[members].sum()), int(members.sum()))


def accuracy(y: Flags) -> Proportion:
    return Proportion(int(y.sum()), int(y.size))
