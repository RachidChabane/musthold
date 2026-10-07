from __future__ import annotations

import math

import pytest

from calibration.analysis.stats.intervals import (
    UNDEFINED_INTERVAL,
    Estimate,
    Interval,
    Proportion,
    decision_interval,
    wilson,
)


def test_wilson_over_zero_units_is_the_unit_interval() -> None:
    assert wilson(0, 0) == Interval(0.0, 1.0)


def test_wilson_with_no_success_starts_at_zero() -> None:
    interval = wilson(0, 10)
    assert interval.lo == 0.0
    assert interval.hi == pytest.approx(0.2775, abs=1e-3)


def test_wilson_with_every_success_ends_at_one() -> None:
    interval = wilson(10, 10)
    assert interval.hi == 1.0
    assert interval.lo == pytest.approx(0.7225, abs=1e-3)


def test_wilson_upper_bound_of_zero_errors_in_nineteen_matches_the_preregistration() -> None:
    assert wilson(0, 19).hi == pytest.approx(0.168, abs=1e-3)


def test_wilson_of_a_half_is_symmetric() -> None:
    interval = wilson(50, 100)
    assert interval.lo == pytest.approx(0.404, abs=1e-3)
    assert interval.hi == pytest.approx(0.596, abs=1e-3)


def test_decision_interval_is_the_wider_of_the_two() -> None:
    assert decision_interval(Interval(0.2, 0.6), Interval(0.1, 0.5)) == Interval(0.1, 0.6)


def test_decision_interval_is_undefined_when_one_side_is() -> None:
    assert not decision_interval(Interval(0.2, 0.6), UNDEFINED_INTERVAL).is_defined


def test_undefined_interval_is_not_defined() -> None:
    assert not UNDEFINED_INTERVAL.is_defined
    assert Interval(0.0, 1.0).is_defined


def test_estimate_record_serialises_nan_as_none() -> None:
    record = Estimate(float("nan"), 0.1, float("nan"), "wilson").to_record()
    assert record == {"point": None, "lo": 0.1, "hi": None, "method": "wilson"}


def test_proportion_exposes_counts_point_and_interval() -> None:
    proportion = Proportion(1, 4)
    assert proportion.point == 0.25
    assert proportion.wilson == wilson(1, 4)
    estimate = proportion.estimate()
    assert (estimate.point, estimate.lo, estimate.hi) == (0.25, wilson(1, 4).lo, wilson(1, 4).hi)


def test_proportion_over_zero_units_has_no_point_estimate() -> None:
    proportion = Proportion(0, 0)
    assert math.isnan(proportion.point)
    assert math.isnan(proportion.estimate().point)
