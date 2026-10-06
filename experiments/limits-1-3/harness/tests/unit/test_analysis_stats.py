"""Interval estimators, agreement coefficients and exact tests against published values."""

from __future__ import annotations

import math

import numpy as np
import pytest
from hypothesis import given, settings, strategies as st

from pilot.analysis.stats import (
    DecisionInterval,
    Interval,
    balanced_accuracy_statistic,
    binomial_greater_p,
    bootstrap_interval,
    cluster_bootstrap,
    cohen_kappa,
    cohen_kappa_tables,
    exact_mcnemar_p,
    fleiss_kappa,
    null_threshold,
    ratio_statistic,
    wilson,
)

# Fleiss (1971), 30 patients rated by 6 psychiatrists into 5 diagnostic categories, overall kappa 0.430.
# The table is the one reproduced in statsmodels' inter-rater tests.
FLEISS_1971_DIAGNOSES = (
    (4, 4, 4, 4, 4, 4),
    (2, 2, 2, 5, 5, 5),
    (2, 3, 3, 3, 3, 5),
    (5, 5, 5, 5, 5, 5),
    (2, 2, 2, 4, 4, 4),
    (1, 1, 3, 3, 3, 3),
    (3, 3, 3, 3, 5, 5),
    (1, 1, 3, 3, 3, 4),
    (1, 1, 4, 4, 4, 4),
    (5, 5, 5, 5, 5, 5),
    (1, 4, 4, 4, 4, 4),
    (1, 2, 4, 4, 4, 4),
    (2, 2, 2, 3, 3, 3),
    (1, 4, 4, 4, 4, 4),
    (2, 2, 4, 4, 4, 5),
    (3, 3, 3, 3, 3, 5),
    (1, 1, 1, 4, 5, 5),
    (1, 1, 1, 1, 1, 2),
    (2, 2, 4, 4, 4, 4),
    (1, 3, 3, 5, 5, 5),
    (5, 5, 5, 5, 5, 5),
    (2, 4, 4, 4, 4, 4),
    (2, 2, 4, 5, 5, 5),
    (1, 1, 4, 4, 4, 4),
    (1, 4, 4, 4, 4, 5),
    (2, 2, 2, 2, 2, 4),
    (1, 1, 1, 1, 5, 5),
    (2, 2, 4, 4, 4, 4),
    (1, 3, 3, 3, 3, 3),
    (5, 5, 5, 5, 5, 5),
)
FLEISS_CATEGORIES = ("1", "2", "3", "4", "5")
FLEISS_1971_KAPPA = 0.430

# Newcombe (1998), Statistics in Medicine 17:857-872, Table I: Wilson score interval without continuity correction.
NEWCOMBE_WILSON = (
    (81, 263, 0.2553, 0.3662),
    (15, 148, 0.0624, 0.1605),
    (0, 20, 0.0000, 0.1611),
    (1, 29, 0.0061, 0.1718),
)
SEED = 20261006


@pytest.mark.parametrize(("successes", "total", "lo", "hi"), NEWCOMBE_WILSON)
def test_wilson_matches_newcombe_1998(successes, total, lo, hi):
    interval = wilson(successes, total)
    assert interval.lo == pytest.approx(lo, abs=5e-5)
    assert interval.hi == pytest.approx(hi, abs=5e-5)


def test_wilson_of_zero_successes_has_the_closed_form_upper_bound():
    z2 = 1.959963984540054**2
    assert wilson(0, 10).hi == pytest.approx(z2 / (10 + z2))


def test_wilson_with_no_units_is_vacuous():
    assert wilson(0, 0) == Interval(0.0, 1.0)


@given(st.integers(0, 200).flatmap(lambda n: st.tuples(st.integers(0, n), st.just(n))))
def test_wilson_interval_contains_the_point_and_stays_in_unit_range(pair):
    successes, total = pair
    interval = wilson(successes, total)
    assert 0.0 <= interval.lo <= interval.hi <= 1.0
    if total:
        assert interval.lo - 1e-12 <= successes / total <= interval.hi + 1e-12


def test_cohens_kappa_matches_the_sas_published_value():
    # statsmodels test data from the SAS example: kappa 0.4842.
    table = np.array([[0, 4, 1], [0, 8, 0], [0, 1, 5]], dtype=float)
    assert float(cohen_kappa_tables(table)) == pytest.approx(0.4842, abs=5e-5)


def test_cohens_kappa_from_labels_matches_the_table_form():
    first = ["a"] * 20 + ["b"] * 5 + ["a"] * 10 + ["b"] * 15
    second = ["a"] * 20 + ["a"] * 5 + ["b"] * 10 + ["b"] * 15
    # observed 35/50, chance (25*30 + 25*20)/2500 = 0.5, kappa 0.4
    assert cohen_kappa(first, second, ["a", "b"]) == pytest.approx(0.4)


def test_cohens_kappa_is_undefined_when_chance_agreement_is_one():
    assert math.isnan(cohen_kappa(["a", "a"], ["a", "a"], ["a", "b"]))


def test_fleiss_kappa_matches_fleiss_1971():
    ratings = [[str(x) for x in row] for row in FLEISS_1971_DIAGNOSES]
    assert fleiss_kappa(ratings, FLEISS_CATEGORIES) == pytest.approx(FLEISS_1971_KAPPA, abs=5e-4)


def test_fleiss_kappa_of_perfect_agreement_with_two_categories_is_one():
    assert fleiss_kappa([["a", "a", "a"], ["b", "b", "b"]], ["a", "b"]) == pytest.approx(1.0)


def test_exact_mcnemar_is_the_two_sided_binomial_on_the_discordant_pairs():
    assert exact_mcnemar_p(0, 0) == 1.0
    assert exact_mcnemar_p(5, 5) == pytest.approx(1.0)
    assert exact_mcnemar_p(0, 8) == pytest.approx(2 * 0.5**8)
    assert exact_mcnemar_p(3, 9) == exact_mcnemar_p(9, 3)


def test_one_sided_exact_binomial_matches_the_tail_sum():
    expected = sum(math.comb(28, i) for i in range(20, 29)) / 2**28
    assert binomial_greater_p(20, 28) == pytest.approx(expected)
    assert binomial_greater_p(0, 0) == 1.0


def test_cluster_bootstrap_is_seeded_and_resamples_whole_clusters():
    clusters = np.array([[1.0, 2.0], [0.0, 3.0], [2.0, 2.0], [0.0, 5.0]])
    statistic = ratio_statistic(0, 1)
    first = cluster_bootstrap(clusters, statistic, 500, SEED)
    second = cluster_bootstrap(clusters, statistic, 500, SEED)
    assert np.array_equal(first, second, equal_nan=True)
    assert not np.array_equal(first, cluster_bootstrap(clusters, statistic, 500, SEED + 1), equal_nan=True)


def test_cluster_bootstrap_of_identical_clusters_has_no_spread():
    clusters = np.tile([1.0, 4.0], (6, 1))
    interval = bootstrap_interval(clusters, ratio_statistic(0, 1), 200, SEED)
    assert interval == Interval(0.25, 0.25)


def test_bootstrap_ignores_undefined_resamples():
    clusters = np.array([[0.0, 0.0], [1.0, 2.0]])
    interval = bootstrap_interval(clusters, ratio_statistic(0, 1), 300, SEED)
    assert interval.is_defined and interval.lo == interval.hi == 0.5


def test_decision_interval_is_the_wider_of_wilson_and_bootstrap():
    narrow, wide = Interval(0.2, 0.3), Interval(0.1, 0.5)
    assert DecisionInterval(narrow, wide).decision == wide
    assert DecisionInterval(wide, narrow).decision == wide
    assert DecisionInterval(None, narrow).decision == narrow


def test_balanced_accuracy_statistic_averages_sensitivity_and_specificity():
    sums = np.array([[9.0, 10.0, 15.0, 20.0]])
    assert balanced_accuracy_statistic(sums)[0] == pytest.approx((0.9 + 0.75) / 2)


def test_null_threshold_is_the_95th_percentile_ignoring_nan():
    values = np.concatenate([np.arange(101, dtype=float), [np.nan]])
    assert null_threshold(values) == pytest.approx(95.0)


@settings(max_examples=25, deadline=None)
@given(st.lists(st.tuples(st.integers(0, 5), st.integers(1, 6)), min_size=1, max_size=8))
def test_bootstrap_interval_stays_between_zero_and_one_for_ratio_counts(pairs):
    clusters = np.array([[min(a, b), b] for a, b in pairs], dtype=float)
    interval = bootstrap_interval(clusters, ratio_statistic(0, 1), 100, SEED)
    assert 0.0 <= interval.lo <= interval.hi <= 1.0
