from __future__ import annotations

import math

import numpy as np
import pytest

from calibration.analysis.stats.bootstrap import ClusterIndex, Resamples, percentile_interval
from calibration.analysis.stats.intervals import wilson
from calibration.analysis.stats.risk_coverage import (
    aurc,
    c05,
    c05u,
    decision_upper_bounds,
    false_aligned_curve,
    false_aligned_rate,
    flag_rate,
    miss_rate,
    risk_coverage_curve,
    risk_upper_bounds,
)
from stats_arrays import flags, floats, mask

P = floats([0.9, 0.9, 0.8, 0.5])
CORRECT = flags([1, 0, 1, 0])


def test_tied_confidences_form_one_block() -> None:
    curve = risk_coverage_curve(P, CORRECT)
    assert [(point.threshold, point.n, point.errors, point.coverage) for point in curve] == [
        (0.9, 2, 1, 0.5),
        (0.8, 3, 1, 0.75),
        (0.5, 4, 2, 1.0),
    ]
    assert [point.risk for point in curve] == [0.5, 1 / 3, 0.5]


def test_empty_curve_has_no_summary() -> None:
    curve = risk_coverage_curve(floats([]), flags([]))
    assert curve == ()
    assert math.isnan(aurc(curve))
    assert math.isnan(c05(curve))
    assert math.isnan(c05u(curve, floats([])))


def test_aurc_uses_right_continuous_steps() -> None:
    expected = 0.5 * 0.5 + 0.25 * (1 / 3) + 0.25 * 0.5
    assert aurc(risk_coverage_curve(P, CORRECT)) == pytest.approx(expected)


def test_aurc_of_an_errorless_and_of_an_always_wrong_set() -> None:
    assert aurc(risk_coverage_curve(floats([0.9, 0.5]), flags([1, 1]))) == 0.0
    assert aurc(risk_coverage_curve(floats([0.9, 0.5]), flags([0, 0]))) == 1.0


def test_c05_is_zero_when_no_point_is_clean_enough() -> None:
    assert c05(risk_coverage_curve(P, CORRECT)) == 0.0


def test_c05_takes_the_largest_coverage_within_the_limit() -> None:
    p = floats([0.9, 0.8, 0.7, 0.6])
    curve = risk_coverage_curve(p, flags([1, 1, 1, 0]))
    assert c05(curve) == 0.75
    assert c05(curve, 0.25) == 1.0


def test_c05u_needs_the_decision_upper_bound_within_the_limit() -> None:
    clean = risk_coverage_curve(floats([0.9] * 100), flags([1] * 100))
    assert c05u(clean, floats([0.0])) == 1.0
    assert c05u(clean, floats([0.2])) == 0.0
    small = risk_coverage_curve(floats([0.9] * 10), flags([1] * 10))
    assert c05u(small, floats([0.0])) == 0.0


def test_decision_upper_bound_is_the_wider_one_and_undefined_without_a_bootstrap() -> None:
    curve = risk_coverage_curve(floats([0.9] * 100), flags([1] * 100))
    wilson_upper = wilson(0, 100).hi
    assert decision_upper_bounds(curve, floats([0.0]))[0] == pytest.approx(wilson_upper)
    assert decision_upper_bounds(curve, floats([0.3]))[0] == pytest.approx(0.3)
    assert math.isnan(decision_upper_bounds(curve, floats([math.nan]))[0])
    assert c05u(curve, floats([math.nan])) == 0.0


def test_risk_upper_bounds_match_a_brute_force_evaluation() -> None:
    clusters = ["a", "a", "b", "b", "c", "c", "d", "d", "e", "e"]
    p = floats([0.9, 0.8, 0.9, 0.5, 0.7, 0.7, 0.6, 0.95, 0.4, 0.85])
    correct = flags([1, 1, 0, 1, 1, 0, 1, 1, 0, 1])
    draws = Resamples.draw(ClusterIndex.build(clusters), 200, 11)
    thresholds = [0.95, 0.7, 0.4]
    bounds = risk_upper_bounds(draws, p, correct, thresholds)
    for threshold, bound in zip(thresholds, bounds.tolist(), strict=True):

        def risk(rows: np.ndarray, limit: float = threshold) -> float:
            kept = rows[p[rows] >= limit]
            return float((1 - correct[kept]).sum() / kept.size) if kept.size else math.nan

        assert bound == pytest.approx(percentile_interval(draws.values(risk)).hi, abs=1e-12)


def test_risk_upper_bounds_cover_more_thresholds_than_one_chunk() -> None:
    clusters = [f"c{i % 4}" for i in range(200)]
    p = floats([i / 200 for i in range(200)])
    correct = flags([i % 2 for i in range(200)])
    draws = Resamples.draw(ClusterIndex.build(clusters), 50, 3)
    thresholds = sorted(set(p.tolist()), reverse=True)
    assert risk_upper_bounds(draws, p, correct, thresholds).size == 200


def test_miss_rate_counts_positive_truth_below_tau() -> None:
    p = floats([0.1, 0.3, 0.6, 0.05])
    y = flags([1, 1, 1, 0])
    rate = miss_rate(p, y, 0.2)
    assert (rate.successes, rate.total) == (1, 3)
    assert miss_rate(p, flags([0, 0, 0, 0]), 0.2).total == 0


def test_flag_rate_counts_all_rows_at_or_above_tau() -> None:
    p = floats([0.1, 0.3, 0.6, 0.05])
    rate = flag_rate(p, flags([1, 1, 1, 0]), 0.3)
    assert (rate.successes, rate.total) == (2, 4)


def test_false_aligned_rate_among_confident_positive_predictions() -> None:
    confidence = floats([0.95, 0.9, 0.85, 0.99, 0.92])
    predicted = mask([True, True, True, False, True])
    truth = mask([True, False, False, False, True])
    rate = false_aligned_rate(confidence, predicted, truth, 0.9)
    assert (rate.successes, rate.total) == (1, 3)
    curve = false_aligned_curve(confidence, predicted, truth, [0.9, 0.99])
    assert [(r.successes, r.total) for r in curve] == [(1, 3), (0, 0)]
