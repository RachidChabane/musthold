"""Agreement coefficients with cluster-bootstrap intervals (PREREG 11, inter-rater agreement)."""

from __future__ import annotations

from collections.abc import Sequence
from dataclasses import dataclass

import numpy as np

from pilot.analysis.settings import AnalysisSettings
from pilot.analysis.stats import (
    bootstrap_interval,
    cohen_kappa,
    cohen_kappa_statistic,
    contingency,
    fleiss_kappa,
    fleiss_kappa_statistic,
    fleiss_sufficient,
)

NAN = float("nan")


@dataclass(frozen=True)
class Agreement:
    name: str
    n: int
    raw: float
    kappa: float
    lo: float
    hi: float


def cohen_agreement(
    name: str,
    first: Sequence[str],
    second: Sequence[str],
    clusters: Sequence[int],
    categories: Sequence[str],
    settings: AnalysisSettings,
) -> Agreement:
    n = len(first)
    if n == 0:
        return Agreement(name, 0, NAN, NAN, NAN, NAN)
    grouped: dict[int, tuple[list[str], list[str]]] = {}
    for a, b, c in zip(first, second, clusters, strict=True):
        grouped.setdefault(c, ([], []))
        grouped[c][0].append(a)
        grouped[c][1].append(b)
    array = np.array([contingency(x, y, categories).ravel() for x, y in grouped.values()])
    interval = bootstrap_interval(array, cohen_kappa_statistic(len(categories)), settings.resamples, settings.seed)
    raw = sum(1 for a, b in zip(first, second, strict=True) if a == b) / n
    return Agreement(name, n, raw, cohen_kappa(first, second, categories), interval.lo, interval.hi)


def fleiss_agreement(
    name: str,
    ratings: Sequence[Sequence[str]],
    clusters: Sequence[int],
    categories: Sequence[str],
    settings: AnalysisSettings,
) -> Agreement:
    if not ratings:
        return Agreement(name, 0, NAN, NAN, NAN, NAN)
    grouped: dict[int, list[Sequence[str]]] = {}
    for item, cluster in zip(ratings, clusters, strict=True):
        grouped.setdefault(cluster, []).append(item)
    array = np.array([fleiss_sufficient(rows, categories) for rows in grouped.values()])
    interval = bootstrap_interval(array, fleiss_kappa_statistic(len(ratings[0])), settings.resamples, settings.seed)
    raw = float(np.mean([max(sum(1 for r in item if r == c) for c in categories) == len(item) for item in ratings]))
    return Agreement(name, len(ratings), raw, fleiss_kappa(ratings, categories), interval.lo, interval.hi)
