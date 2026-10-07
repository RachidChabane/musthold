"""Rank-based statistics: AUROC, Spearman correlation and Cohen's kappa."""

from __future__ import annotations

import math

import numpy as np

from calibration.analysis.stats.arrays import NAN, Flags, Floats, exact_mean, exact_sum

ConfusionTable = tuple[tuple[int, int], tuple[int, int]]
UNIT_CORRELATION_BOUND = 1.0


def average_ranks(values: Floats) -> Floats:
    """1-based ranks where tied values share the mean of the ranks they span."""
    count = values.size
    ranks = np.empty(count, dtype=np.float64)
    if count == 0:
        return ranks
    order = np.argsort(values, kind="stable")
    ordered = values[order]
    starts = np.flatnonzero(np.concatenate(([True], ordered[1:] != ordered[:-1])))
    ends = np.concatenate((starts[1:], [count]))
    ranks[order] = np.repeat((starts + 1 + ends) / 2.0, ends - starts)
    return ranks


def auroc(p: Floats, y: Flags) -> float:
    """Mann-Whitney probability that a positive outranks a negative, ties counting one half."""
    positives = int(y.sum())
    negatives = int(y.size) - positives
    if positives == 0 or negatives == 0:
        return NAN
    ranks = average_ranks(p)
    positive_rank_sum = exact_sum(ranks[y == 1])
    u_statistic = positive_rank_sum - positives * (positives + 1) / 2.0
    return u_statistic / (positives * negatives)


def spearman(a: Floats, b: Floats) -> float:
    """Pearson correlation of the average ranks; undefined below two points or when either side is constant."""
    if a.size < 2:
        return NAN
    ranks_a = average_ranks(a)
    ranks_b = average_ranks(b)
    centred_a = ranks_a - exact_mean(ranks_a)
    centred_b = ranks_b - exact_mean(ranks_b)
    variance_a = exact_sum(centred_a * centred_a)
    variance_b = exact_sum(centred_b * centred_b)
    if variance_a == 0 or variance_b == 0:
        return NAN
    correlation = exact_sum(centred_a * centred_b) / math.sqrt(variance_a * variance_b)
    return max(-UNIT_CORRELATION_BOUND, min(UNIT_CORRELATION_BOUND, correlation))


def cohen_kappa(table: ConfusionTable) -> float:
    """Cohen's kappa of a 2x2 count table, computed in exact integer arithmetic until the final division."""
    (a, b), (c, d) = table
    total = a + b + c + d
    if total == 0:
        return NAN
    agreement = total * (a + d)
    chance = (a + b) * (a + c) + (c + d) * (b + d)
    denominator = total * total - chance
    if denominator == 0:
        return NAN
    return (agreement - chance) / denominator
