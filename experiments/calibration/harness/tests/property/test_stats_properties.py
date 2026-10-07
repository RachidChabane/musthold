from __future__ import annotations

import math
from random import Random

import numpy as np
import pytest
from hypothesis import given, settings
from hypothesis import strategies as st

from calibration.analysis.stats.admission import isotonic_fit, split_clusters
from calibration.analysis.stats.arrays import Flags, Floats
from calibration.analysis.stats.bootstrap import ClusterIndex, Resamples
from calibration.analysis.stats.calibration import brier, ece_equal_width
from calibration.analysis.stats.intervals import wilson
from calibration.analysis.stats.ranking import auroc
from calibration.analysis.stats.risk_coverage import c05, flag_rate, miss_rate, risk_coverage_curve
from calibration.domain.constants import SCREEN_GRID
from stats_arrays import flags, floats

PROFILE = settings(max_examples=60, deadline=None)
PROBABILITY = st.floats(min_value=0.0, max_value=1.0, allow_nan=False)
LABEL = st.integers(min_value=0, max_value=1)
ROW = st.tuples(PROBABILITY, LABEL)
ROWS = st.lists(ROW, min_size=1, max_size=60)
RESAMPLES = 40


def _split(rows: list[tuple[float, int]]) -> tuple[Floats, Flags]:
    return floats([p for p, _ in rows]), flags([y for _, y in rows])


@PROFILE
@given(ROWS)
def test_ece_and_brier_stay_in_the_unit_interval(rows: list[tuple[float, int]]) -> None:
    p, y = _split(rows)
    assert 0.0 <= ece_equal_width(p, y) <= 1.0
    assert 0.0 <= brier(p, y) <= 1.0


@PROFILE
@given(st.lists(st.tuples(st.integers(1, 8), st.integers(0, 8), st.integers(1, 3)), min_size=1, max_size=6))
def test_ece_of_perfectly_calibrated_frequencies_is_zero(groups: list[tuple[int, int, int]]) -> None:
    confidence: list[float] = []
    truth: list[int] = []
    for denominator, requested, repeats in groups:
        numerator = min(requested, denominator)
        confidence += [numerator / denominator] * (denominator * repeats)
        truth += [1] * (numerator * repeats) + [0] * ((denominator - numerator) * repeats)
    assert ece_equal_width(floats(confidence), flags(truth)) == pytest.approx(0.0, abs=1e-12)


@PROFILE
@given(ROWS)
def test_auroc_is_a_probability_and_complements_under_label_flip(rows: list[tuple[float, int]]) -> None:
    p, y = _split(rows)
    value = auroc(p, y)
    if len({label for _, label in rows}) < 2:
        assert math.isnan(value)
        return
    assert 0.0 <= value <= 1.0
    assert value + auroc(p, 1 - y) == pytest.approx(1.0)


@PROFILE
@given(ROWS)
def test_auroc_is_invariant_under_strictly_increasing_transforms(rows: list[tuple[float, int]]) -> None:
    p, y = _split(rows)
    transformed = p * 2.0
    original, moved = auroc(p, y), auroc(transformed, y)
    assert (math.isnan(original) and math.isnan(moved)) or original == moved


@PROFILE
@given(st.integers(0, 60), st.integers(0, 60))
def test_wilson_contains_its_point_and_stays_in_the_unit_interval(successes: int, extra: int) -> None:
    total = successes + extra
    interval = wilson(successes, total)
    assert 0.0 <= interval.lo <= interval.hi <= 1.0
    if total:
        assert interval.lo <= successes / total <= interval.hi


CLUSTERED = st.lists(st.tuples(st.integers(0, 4), PROBABILITY, LABEL), min_size=2, max_size=40)


def _clustered(rows: list[tuple[int, float, int]]) -> tuple[list[str], Floats, Flags]:
    clusters = [f"c{c}" for c, _, _ in rows]
    return clusters, floats([p for _, p, _ in rows]), flags([y for _, _, y in rows])


def _ece_interval(clusters: list[str], p: Floats, y: Flags) -> tuple[float, float]:
    draws = Resamples.draw(ClusterIndex.build(clusters), RESAMPLES, 3)
    interval = draws.interval(lambda rows: ece_equal_width(p[rows], y[rows]))
    return interval.lo, interval.hi


@PROFILE
@given(CLUSTERED, st.randoms(use_true_random=False))
def test_bootstrap_is_deterministic_and_row_order_free(rows: list[tuple[int, float, int]], random: Random) -> None:
    clusters, p, y = _clustered(rows)
    baseline = _ece_interval(clusters, p, y)
    assert _ece_interval(clusters, p, y) == baseline
    order = list(range(len(rows)))
    random.shuffle(order)
    shuffled = _ece_interval([clusters[i] for i in order], p[order], y[order])
    assert shuffled == baseline


@PROFILE
@given(CLUSTERED)
def test_bootstrap_ignores_cluster_labels_that_keep_the_sorted_order(rows: list[tuple[int, float, int]]) -> None:
    clusters, p, y = _clustered(rows)
    renamed = [f"zz-{label}-renamed" for label in clusters]
    assert _ece_interval(renamed, p, y) == _ece_interval(clusters, p, y)


@PROFILE
@given(ROWS)
def test_pav_is_non_decreasing(rows: list[tuple[float, int]]) -> None:
    p, y = _split(rows)
    fit = isotonic_fit(p, y)
    assert (np.diff(fit.values) >= 0).all()
    assert (np.diff(fit.confidences) > 0).all()


@PROFILE
@given(st.lists(LABEL, min_size=1, max_size=40))
def test_pav_leaves_already_sorted_labels_unchanged(labels: list[int]) -> None:
    ordered = sorted(labels)
    fit = isotonic_fit(floats([float(i) for i in range(len(ordered))]), flags(ordered))
    assert fit.values.tolist() == [float(v) for v in ordered]


@PROFILE
@given(ROWS)
def test_risk_coverage_coverage_grows_as_the_threshold_drops_and_c05_is_monotone(
    rows: list[tuple[float, int]],
) -> None:
    p, y = _split(rows)
    curve = risk_coverage_curve(p, y)
    thresholds = [point.threshold for point in curve]
    coverages = [point.coverage for point in curve]
    assert thresholds == sorted(set(thresholds), reverse=True)
    assert coverages == sorted(set(coverages))
    assert coverages[-1] == 1.0
    limits = [0.0, 0.05, 0.1, 0.3, 0.6, 1.0]
    values = [c05(curve, limit) for limit in limits]
    assert values == sorted(values)


@PROFILE
@given(st.lists(st.text(min_size=1, max_size=4), min_size=1, max_size=30))
def test_cluster_split_is_a_deterministic_partition(ids: list[str]) -> None:
    split = split_clusters(ids)
    distinct = set(ids)
    assert set(split.fit_half) | set(split.held_out) == distinct
    assert not set(split.fit_half) & set(split.held_out)
    assert len(split.fit_half) == (len(distinct) + 1) // 2
    assert split_clusters(list(reversed(ids))) == split


@PROFILE
@given(ROWS)
def test_miss_rate_never_falls_and_flag_rate_never_rises_with_tau(rows: list[tuple[float, int]]) -> None:
    p, y = _split(rows)
    misses = [miss_rate(p, y, tau).successes for tau in SCREEN_GRID]
    flags_ = [flag_rate(p, y, tau).successes for tau in SCREEN_GRID]
    assert misses == sorted(misses)
    assert flags_ == sorted(flags_, reverse=True)
