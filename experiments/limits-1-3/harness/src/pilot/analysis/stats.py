"""Interval estimators, agreement coefficients and exact tests (PREREG 10, 11).

Everything here is a pure function of counts and a seed; no file or model access.
"""

from __future__ import annotations

import math
from collections.abc import Callable, Sequence
from dataclasses import dataclass

import numpy as np
from numpy.typing import NDArray
from scipy.stats import binomtest
from statsmodels.stats.proportion import proportion_confint

from pilot.domain.analysis_constants import (
    ANALYSIS_SEED,
    BOOTSTRAP_RESAMPLES,
    NULL_PERCENTILE,
    WILSON_ALPHA,
)

FloatArray = NDArray[np.float64]
Statistic = Callable[[FloatArray], FloatArray]
WILSON_METHOD = "wilson"
LOWER_PERCENTILE = 100.0 * WILSON_ALPHA / 2
UPPER_PERCENTILE = 100.0 - LOWER_PERCENTILE
NAN = float("nan")


@dataclass(frozen=True)
class Interval:
    lo: float
    hi: float

    @property
    def width(self) -> float:
        return self.hi - self.lo

    @property
    def is_defined(self) -> bool:
        return not (math.isnan(self.lo) or math.isnan(self.hi))


UNDEFINED_INTERVAL = Interval(NAN, NAN)


@dataclass(frozen=True)
class DecisionInterval:
    """The Wilson and bootstrap intervals, and the wider of the two (PREREG 10)."""

    wilson: Interval | None
    bootstrap: Interval | None

    @property
    def decision(self) -> Interval:
        defined = [i for i in (self.wilson, self.bootstrap) if i is not None and i.is_defined]
        return max(defined, key=lambda i: i.width) if defined else UNDEFINED_INTERVAL


def wilson(successes: int, total: int) -> Interval:
    if total <= 0:
        return Interval(0.0, 1.0)
    bounds = proportion_confint(successes, total, alpha=WILSON_ALPHA, method=WILSON_METHOD)
    return Interval(float(np.asarray(bounds[0])), float(np.asarray(bounds[1])))


def percentile_interval(samples: FloatArray) -> Interval:
    finite = samples[~np.isnan(samples)]
    if finite.size == 0:
        return UNDEFINED_INTERVAL
    lo, hi = np.percentile(finite, [LOWER_PERCENTILE, UPPER_PERCENTILE])
    return Interval(float(lo), float(hi))


def null_threshold(samples: FloatArray) -> float:
    finite = samples[~np.isnan(samples)]
    return float(np.percentile(finite, NULL_PERCENTILE)) if finite.size else NAN


def null_mean(samples: FloatArray) -> float:
    """Chance level of a permutation null: the mean of its finite samples."""
    finite = samples[~np.isnan(samples)]
    return float(finite.mean()) if finite.size else NAN


def cluster_bootstrap(
    cluster_counts: FloatArray,
    statistic: Statistic,
    resamples: int = BOOTSTRAP_RESAMPLES,
    seed: int = ANALYSIS_SEED,
) -> FloatArray:
    """Statistic over `resamples` draws of whole clusters with replacement.

    `cluster_counts` has one row of additive counts per cluster; the statistic receives
    the summed counts of each draw as a (resamples, columns) array.
    """
    counts = np.asarray(cluster_counts, dtype=np.float64)
    if counts.shape[0] == 0:
        return np.full(resamples, NAN)
    rng = np.random.default_rng(seed)
    draws = rng.integers(0, counts.shape[0], size=(resamples, counts.shape[0]))
    return statistic(counts[draws].sum(axis=1))


def bootstrap_interval(
    cluster_counts: FloatArray,
    statistic: Statistic,
    resamples: int = BOOTSTRAP_RESAMPLES,
    seed: int = ANALYSIS_SEED,
) -> Interval:
    return percentile_interval(cluster_bootstrap(cluster_counts, statistic, resamples, seed))


def safe_divide(numerator: FloatArray, denominator: FloatArray) -> FloatArray:
    out = np.full(numerator.shape, NAN)
    np.divide(numerator, denominator, out=out, where=denominator != 0)
    return out


def ratio_statistic(numerator_column: int, denominator_column: int) -> Statistic:
    def statistic(sums: FloatArray) -> FloatArray:
        return safe_divide(sums[:, numerator_column], sums[:, denominator_column])

    return statistic


def balanced_accuracy_statistic(sums: FloatArray) -> FloatArray:
    """Columns: positives correct, positives total, negatives correct, negatives total."""
    sensitivity = safe_divide(sums[:, 0], sums[:, 1])
    specificity = safe_divide(sums[:, 2], sums[:, 3])
    return (sensitivity + specificity) / 2


def cohen_kappa_tables(tables: FloatArray) -> FloatArray:
    """Cohen's kappa of contingency tables with shape (..., k, k); NaN when chance agreement is 1."""
    total = tables.sum(axis=(-2, -1))
    observed = np.trace(tables, axis1=-2, axis2=-1)
    expected = (tables.sum(axis=-1) * tables.sum(axis=-2)).sum(axis=-1)
    with np.errstate(divide="ignore", invalid="ignore"):
        agreement = observed / total
        chance = expected / total**2
        kappa = (agreement - chance) / (1 - chance)
    return np.where((total == 0) | (chance == 1), NAN, kappa)


def cohen_kappa_statistic(categories: int) -> Statistic:
    def statistic(sums: FloatArray) -> FloatArray:
        return cohen_kappa_tables(sums.reshape(-1, categories, categories))

    return statistic


def contingency(
    first: Sequence[str], second: Sequence[str], categories: Sequence[str]
) -> FloatArray:
    index = {category: i for i, category in enumerate(categories)}
    table = np.zeros((len(categories), len(categories)))
    for a, b in zip(first, second, strict=True):
        table[index[a], index[b]] += 1
    return table


def cohen_kappa(first: Sequence[str], second: Sequence[str], categories: Sequence[str]) -> float:
    return float(cohen_kappa_tables(contingency(first, second, categories)))


def fleiss_sufficient(ratings: Sequence[Sequence[str]], categories: Sequence[str]) -> FloatArray:
    """Additive Fleiss statistics of one cluster: [items, sum of item agreement, category totals]."""
    raters = len(ratings[0]) if ratings else 0
    agreement = 0.0
    totals = np.zeros(len(categories))
    for item in ratings:
        counts = np.array([sum(1 for r in item if r == c) for c in categories], dtype=np.float64)
        totals += counts
        agreement += (float((counts**2).sum()) - raters) / (raters * (raters - 1))
    return np.concatenate([[len(ratings), agreement], totals])


def fleiss_kappa_statistic(raters: int) -> Statistic:
    def statistic(sums: FloatArray) -> FloatArray:
        items = sums[:, 0]
        with np.errstate(divide="ignore", invalid="ignore"):
            observed = sums[:, 1] / items
            proportions = sums[:, 2:] / (items[:, None] * raters)
            chance = (proportions**2).sum(axis=1)
            kappa = (observed - chance) / (1 - chance)
        return np.where((items == 0) | (chance == 1), NAN, kappa)

    return statistic


def fleiss_kappa(ratings: Sequence[Sequence[str]], categories: Sequence[str]) -> float:
    raters = len(ratings[0])
    sums = fleiss_sufficient(ratings, categories)[None, :]
    return float(fleiss_kappa_statistic(raters)(sums)[0])


def exact_mcnemar_p(only_first_wrong: int, only_second_wrong: int) -> float:
    discordant = only_first_wrong + only_second_wrong
    if discordant == 0:
        return 1.0
    return float(binomtest(only_first_wrong, discordant, 0.5, alternative="two-sided").pvalue)


def binomial_greater_p(successes: int, total: int, null_probability: float = 0.5) -> float:
    if total == 0:
        return 1.0
    return float(binomtest(successes, total, null_probability, alternative="greater").pvalue)
