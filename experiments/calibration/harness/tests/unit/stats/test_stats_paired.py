from __future__ import annotations

import math

from calibration.analysis.stats.arrays import Flags, Rows
from calibration.analysis.stats.bootstrap import ClusterIndex, Resamples, Statistic
from calibration.analysis.stats.paired import both_wrong_count, paired_metric_difference, wrong_given_other_wrong
from stats_arrays import flags, mask

CLUSTERS = ["a", "a", "b", "b", "c", "c", "d", "d"]
A_CORRECT = flags([1, 1, 1, 0, 1, 1, 0, 1])
B_CORRECT = flags([1, 0, 1, 0, 0, 1, 0, 0])


def _statistic(correct: Flags) -> Statistic:
    def statistic(rows: Rows) -> float:
        return float(correct[rows].sum() / rows.size)

    return statistic


def test_difference_of_accuracies_on_the_same_rows() -> None:
    draws = Resamples.draw(ClusterIndex.build(CLUSTERS), 200, 5)
    estimate = paired_metric_difference(draws, _statistic(A_CORRECT), _statistic(B_CORRECT))
    assert estimate.point == 0.75 - 0.375
    assert estimate.method == "cluster bootstrap"
    assert estimate.lo <= estimate.hi


def test_difference_of_a_scorer_with_itself_is_a_point_mass_at_zero() -> None:
    draws = Resamples.draw(ClusterIndex.build(CLUSTERS), 200, 5)
    estimate = paired_metric_difference(draws, _statistic(A_CORRECT), _statistic(A_CORRECT))
    assert (estimate.point, estimate.lo, estimate.hi) == (0.0, 0.0, 0.0)


def test_difference_over_no_clusters_is_undefined() -> None:
    draws = Resamples.draw(ClusterIndex.build([]), 10, 5)
    estimate = paired_metric_difference(draws, lambda rows: math.nan, lambda rows: math.nan)
    assert math.isnan(estimate.point)
    assert math.isnan(estimate.lo)


def test_both_wrong_and_conditional_wrong_counts() -> None:
    a_wrong = mask([True, True, False, False])
    b_wrong = mask([True, False, True, True])
    assert both_wrong_count(a_wrong, b_wrong) == 1
    proportion = wrong_given_other_wrong(a_wrong, b_wrong)
    assert (proportion.successes, proportion.total) == (1, 3)
    assert math.isnan(wrong_given_other_wrong(a_wrong, mask([False] * 4)).point)
