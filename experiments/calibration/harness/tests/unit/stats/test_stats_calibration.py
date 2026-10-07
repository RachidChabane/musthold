from __future__ import annotations

import math

import pytest

from calibration.analysis.stats.calibration import (
    accuracy,
    accuracy_in_window,
    brier,
    brier_skill,
    ece_equal_mass,
    ece_equal_width,
    reliability_table,
)
from calibration.analysis.stats.intervals import wilson
from stats_arrays import flags, floats


def test_equal_width_ece_by_hand_with_the_last_bin_closed() -> None:
    p = floats([0.05, 0.15, 0.95, 1.0])
    y = flags([0, 0, 1, 1])
    assert ece_equal_width(p, y) == pytest.approx(0.0625)


def test_a_confidence_of_one_falls_in_the_last_bin() -> None:
    assert ece_equal_width(floats([1.0]), flags([0])) == 1.0


def test_a_boundary_value_belongs_to_the_upper_bin() -> None:
    p = floats([0.5, 0.5])
    y = flags([1, 1])
    assert ece_equal_width(p, y, bins=2) == pytest.approx(0.5)
    assert ece_equal_width(floats([0.4999]), flags([1]), bins=2) == pytest.approx(0.5001)


def test_empty_input_has_no_ece() -> None:
    assert math.isnan(ece_equal_width(floats([]), flags([])))
    assert math.isnan(ece_equal_mass(floats([]), flags([]), 5))


def test_equal_mass_ece_by_hand() -> None:
    p = floats([0.1, 0.2, 0.8, 0.9])
    y = flags([0, 0, 1, 1])
    assert ece_equal_mass(p, y, 2) == pytest.approx(0.15)
    assert ece_equal_mass(p, y, 4) == pytest.approx(0.15)


def test_equal_mass_groups_split_unevenly_when_the_count_does_not_divide() -> None:
    p = floats([0.1, 0.2, 0.3])
    y = flags([0, 0, 1])
    assert ece_equal_mass(p, y, 2) == pytest.approx((2 / 3) * abs(0.15 - 0.0) + (1 / 3) * abs(0.3 - 1.0))


def test_equal_mass_ece_with_more_bins_than_rows_skips_empty_groups() -> None:
    assert ece_equal_mass(floats([0.5]), flags([1]), 10) == pytest.approx(0.5)


def test_equal_mass_ties_are_ordered_independently_of_position() -> None:
    p = floats([0.5, 0.5, 0.5, 0.5])
    forward = ece_equal_mass(p, flags([0, 0, 1, 1]), 2)
    backward = ece_equal_mass(p, flags([1, 1, 0, 0]), 2)
    assert forward == backward


def test_reliability_table_rows() -> None:
    rows = reliability_table(floats([0.2, 0.4, 0.6, 1.0]), flags([0, 1, 1, 1]), bins=2)
    assert [(row.lo, row.hi, row.n) for row in rows] == [(0.0, 0.5, 2), (0.5, 1.0, 2)]
    assert rows[0].mean_confidence == pytest.approx(0.3)
    assert rows[0].frequency == 0.5
    assert rows[0].frequency_interval == wilson(1, 2)
    assert rows[1].mean_confidence == pytest.approx(0.8)
    assert rows[1].frequency == 1.0


def test_reliability_table_keeps_empty_bins_undefined() -> None:
    rows = reliability_table(floats([0.9]), flags([1]), bins=2)
    assert rows[0].n == 0
    assert math.isnan(rows[0].mean_confidence)
    assert math.isnan(rows[0].frequency)
    assert (rows[0].frequency_interval.lo, rows[0].frequency_interval.hi) == (0.0, 1.0)


def test_brier_by_hand() -> None:
    assert brier(floats([0.5, 1.0]), flags([0, 1])) == 0.125
    assert math.isnan(brier(floats([]), flags([])))


def test_brier_skill_against_the_base_rate() -> None:
    assert brier_skill(floats([0.5, 1.0]), flags([0, 1])) == pytest.approx(0.5)


def test_brier_skill_is_undefined_when_the_base_rate_is_perfect() -> None:
    assert math.isnan(brier_skill(floats([0.5, 0.7]), flags([1, 1])))
    assert math.isnan(brier_skill(floats([]), flags([])))


def test_accuracy_in_window_is_half_open() -> None:
    window = accuracy_in_window(floats([0.85, 0.9, 0.95, 0.5]), flags([1, 0, 1, 1]), 0.85, 0.95)
    assert (window.total, window.successes, window.point) == (2, 1, 0.5)


def test_accuracy_in_an_empty_window_is_undefined() -> None:
    window = accuracy_in_window(floats([0.5]), flags([1]), 0.85, 0.95)
    assert window.total == 0
    assert math.isnan(window.point)


def test_accuracy_counts_correct_decisions() -> None:
    proportion = accuracy(flags([1, 0, 1, 1]))
    assert (proportion.successes, proportion.total, proportion.point) == (3, 4, 0.75)
    assert math.isnan(accuracy(flags([])).point)
