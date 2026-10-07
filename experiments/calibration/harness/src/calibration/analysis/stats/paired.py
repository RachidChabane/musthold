"""Paired comparisons of two scorers evaluated on the same rows and the same clusters."""

from __future__ import annotations

from calibration.analysis.stats.arrays import Mask
from calibration.analysis.stats.bootstrap import Resamples, Statistic
from calibration.analysis.stats.intervals import BOOTSTRAP_METHOD, Estimate, Proportion


def paired_metric_difference(resamples: Resamples, metric_a: Statistic, metric_b: Statistic) -> Estimate:
    """Point difference a - b over all rows, with the paired cluster-bootstrap interval of the difference.

    Both metrics are evaluated on the same resampled rows of the same draws, so the
    interval reflects the correlation between the two scorers.
    """
    all_rows = resamples.index.all_rows()
    point = metric_a(all_rows) - metric_b(all_rows)
    interval = resamples.paired_difference(metric_a, metric_b)
    return Estimate(point, interval.lo, interval.hi, BOOTSTRAP_METHOD)


def both_wrong_count(a_wrong: Mask, b_wrong: Mask) -> int:
    return int((a_wrong & b_wrong).sum())


def wrong_given_other_wrong(a_wrong: Mask, b_wrong: Mask) -> Proportion:
    """P(a wrong | b wrong) as a count of both-wrong rows among the rows where b is wrong."""
    return Proportion(both_wrong_count(a_wrong, b_wrong), int(b_wrong.sum()))
