from __future__ import annotations

import math

import numpy as np

from calibration.analysis.stats.arrays import Floats, Rows
from calibration.analysis.stats.bootstrap import ClusterIndex, Resamples, percentile_interval
from calibration.analysis.stats.calibration import brier
from stats_arrays import flags, floats

RESAMPLES = 200
CLUSTERS = ["b", "a", "b", "c", "a", "c", "c"]
P = floats([0.1, 0.9, 0.4, 0.6, 0.8, 0.2, 0.5])
Y = flags([0, 1, 0, 1, 1, 0, 1])


def _brier_of(rows: Rows) -> float:
    return brier(P[rows], Y[rows])


def test_index_orders_clusters_by_id_and_rows_by_position() -> None:
    index = ClusterIndex.build(CLUSTERS)
    assert index.ids == ("a", "b", "c")
    assert [rows.tolist() for rows in index.rows] == [[1, 4], [0, 2], [3, 5, 6]]
    assert index.row_count == 7
    assert index.all_rows().tolist() == list(range(7))


def test_cluster_sums_adds_row_columns_per_cluster() -> None:
    index = ClusterIndex.build(CLUSTERS)
    columns = np.column_stack((np.ones(7), Y.astype(np.float64)))
    assert index.cluster_sums(columns).tolist() == [[2.0, 2.0], [2.0, 0.0], [3.0, 2.0]]


def test_draw_is_deterministic_and_seed_dependent() -> None:
    index = ClusterIndex.build(CLUSTERS)
    first = Resamples.draw(index, RESAMPLES, 7)
    again = Resamples.draw(index, RESAMPLES, 7)
    other = Resamples.draw(index, RESAMPLES, 8)
    assert np.array_equal(first.multiplicities, again.multiplicities)
    assert not np.array_equal(first.multiplicities, other.multiplicities)


def test_every_resample_draws_as_many_clusters_as_there_are() -> None:
    draws = Resamples.draw(ClusterIndex.build(CLUSTERS), RESAMPLES, 7)
    assert draws.count == RESAMPLES
    assert (draws.multiplicities.sum(axis=1) == 3).all()


def test_rows_of_repeats_a_cluster_once_per_multiplicity() -> None:
    index = ClusterIndex.build(CLUSTERS)
    draws = Resamples(index, np.array([[2.0, 0.0, 1.0]]))
    assert draws.rows_of(0).tolist() == [1, 4, 1, 4, 3, 5, 6]


def test_draw_depends_on_cluster_order_not_on_labels() -> None:
    renamed = [{"a": "x1", "b": "x2", "c": "x3"}[label] for label in CLUSTERS]
    original = Resamples.draw(ClusterIndex.build(CLUSTERS), RESAMPLES, 7)
    relabelled = Resamples.draw(ClusterIndex.build(renamed), RESAMPLES, 7)
    assert np.array_equal(original.multiplicities, relabelled.multiplicities)


def test_values_and_additive_path_agree() -> None:
    index = ClusterIndex.build(CLUSTERS)
    draws = Resamples.draw(index, RESAMPLES, 7)
    squared = ((P - Y) ** 2).astype(np.float64)
    per_cluster = index.cluster_sums(np.column_stack((np.ones(7), squared)))

    def combine(sums: Floats) -> Floats:
        return sums[:, 1] / sums[:, 0]

    general = draws.values(_brier_of)
    additive = draws.count_values(per_cluster, combine)
    assert np.allclose(general, additive, atol=1e-12)


def test_interval_is_percentile_of_the_values() -> None:
    draws = Resamples.draw(ClusterIndex.build(CLUSTERS), RESAMPLES, 7)
    expected = percentile_interval(draws.values(_brier_of))
    assert draws.interval(_brier_of) == expected
    assert expected.lo <= expected.hi


def test_interval_ignores_undefined_resamples_and_is_undefined_when_all_are() -> None:
    draws = Resamples.draw(ClusterIndex.build(CLUSTERS), RESAMPLES, 7)
    assert not draws.interval(lambda rows: math.nan).is_defined
    sometimes = draws.interval(lambda rows: math.nan if rows.size % 2 else 1.0)
    assert sometimes.lo == sometimes.hi == 1.0


def test_empty_index_gives_undefined_values() -> None:
    draws = Resamples.draw(ClusterIndex.build([]), 5, 7)
    assert np.isnan(draws.values(_brier_of)).all()
    assert not draws.interval(_brier_of).is_defined


def test_paired_difference_of_a_statistic_with_itself_is_zero() -> None:
    draws = Resamples.draw(ClusterIndex.build(CLUSTERS), RESAMPLES, 7)
    interval = draws.paired_difference(_brier_of, _brier_of)
    assert (interval.lo, interval.hi) == (0.0, 0.0)


def test_paired_difference_uses_the_same_draws_for_both_statistics() -> None:
    draws = Resamples.draw(ClusterIndex.build(CLUSTERS), RESAMPLES, 7)

    def doubled(rows: Rows) -> float:
        return 2.0 * _brier_of(rows)

    expected = percentile_interval(draws.values(doubled) - draws.values(_brier_of))
    assert draws.paired_difference(doubled, _brier_of) == expected


def test_percentile_interval_of_a_ramp() -> None:
    interval = percentile_interval(np.arange(101, dtype=np.float64))
    assert (interval.lo, interval.hi) == (2.5, 97.5)
