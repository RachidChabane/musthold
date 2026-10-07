"""`compute_cell_metrics` on small cells whose figures are worked out by hand."""

from __future__ import annotations

import math

import numpy as np
import pytest

from calibration.analysis.cell_metrics import (
    baseline_figures,
    compute_cell_metrics,
    majority_accuracy,
    screen_grid,
)
from calibration.analysis.model import CellData, Decision, Kind
from calibration.analysis.stats.bootstrap import ClusterIndex, Resamples
from calibration.domain.constants import SCREEN_GRID, SEED
from stats_arrays import flags, floats
from support.metric_cases import (
    CLUSTERS,
    NO,
    REF_A,
    REF_B,
    SCREEN_TAG,
    SETTINGS,
    TEST_RESAMPLES,
    YES,
    event_decision,
    make_cell,
    mixed_event_cell,
    spread_clusters,
    top_label_decision,
)

HIGH: float = 0.9
LOW: float = 0.1
GRID_TAU_DEFAULT: float = 0.20


def _event_cell(kind: Kind, events: list[float], truths: list[bool]) -> CellData:
    clusters = spread_clusters(len(events))
    decisions = [
        event_decision(index, cluster, event, truth=truth)
        for index, (cluster, event, truth) in enumerate(zip(clusters, events, truths, strict=True))
    ]
    return make_cell(kind, decisions)


def test_a_perfect_event_cell_has_unit_accuracy_and_auroc_and_a_small_brier() -> None:
    cell = _event_cell(Kind.K2, [HIGH, LOW] * 4, [True, False] * 4)
    metrics = compute_cell_metrics(cell, SETTINGS, equal_mass=False)
    assert metrics is not None
    assert metrics.n == 8
    assert metrics.clusters == len(CLUSTERS)
    assert metrics.accuracy.point == 1.0
    assert metrics.auroc.point == 1.0
    assert metrics.brier.point == pytest.approx(0.01)
    assert metrics.ece.point == pytest.approx(0.1)
    assert metrics.base_rate == 0.5


def test_an_anti_correlated_event_cell_has_zero_auroc_and_accuracy() -> None:
    cell = _event_cell(Kind.K3, [HIGH, LOW] * 4, [False, True] * 4)
    metrics = compute_cell_metrics(cell, SETTINGS, equal_mass=False)
    assert metrics is not None
    assert metrics.accuracy.point == 0.0
    assert metrics.auroc.point == 0.0
    assert metrics.brier.point == pytest.approx(0.81)
    assert metrics.ece.point == pytest.approx(0.9)


def test_a_constant_event_probability_has_auroc_one_half_and_no_calibration_gap() -> None:
    cell = _event_cell(Kind.K2, [0.5] * 8, [True, False] * 4)
    metrics = compute_cell_metrics(cell, SETTINGS, equal_mass=False)
    assert metrics is not None
    assert metrics.auroc.point == 0.5
    assert metrics.brier.point == pytest.approx(0.25)
    assert metrics.ece.point == pytest.approx(0.0)
    assert metrics.brier_skill.point == pytest.approx(0.0)


def test_a_top_label_cell_reads_confidence_against_correctness() -> None:
    clusters = spread_clusters(8)
    correct = [True, True, True, True, True, True, False, False]
    decisions = [
        top_label_decision(index, cluster, HIGH if ok else 0.3, correct=ok)
        for index, (cluster, ok) in enumerate(zip(clusters, correct, strict=True))
    ]
    metrics = compute_cell_metrics(make_cell(Kind.K1, decisions), SETTINGS, equal_mass=False)
    assert metrics is not None
    assert metrics.accuracy.point == 0.75
    assert metrics.auroc.point == 1.0
    assert metrics.ece.point == pytest.approx(0.75 * 0.1 + 0.25 * 0.3)
    assert metrics.brier.point == pytest.approx((6 * 0.01 + 2 * 0.09) / 8)
    assert metrics.ece_top_label is None


def test_no_prediction_gives_no_metrics() -> None:
    silent = Decision("u", CLUSTERS[0], 0.0, False, has_prediction=False)
    assert compute_cell_metrics(make_cell(Kind.K2, [silent]), SETTINGS, equal_mass=False) is None
    assert compute_cell_metrics(make_cell(Kind.K2, []), SETTINGS, equal_mass=False) is None


def test_a_decision_without_prediction_is_left_out_of_the_counts() -> None:
    cell = _event_cell(Kind.K2, [HIGH, LOW] * 2, [True, False] * 2)
    silent = Decision("silent", CLUSTERS[0], 0.0, False, has_prediction=False)
    metrics = compute_cell_metrics(make_cell(Kind.K2, [*cell.decisions, silent]), SETTINGS, equal_mass=False)
    assert metrics is not None
    assert metrics.n == 4


def test_k3_carries_the_false_aligned_rate_at_the_gate_threshold() -> None:
    metrics = compute_cell_metrics(mixed_event_cell(Kind.K3), SETTINGS, equal_mass=False, gate_threshold=0.8)
    assert metrics is not None
    assert metrics.false_aligned is not None
    assert metrics.false_aligned_threshold == 0.8
    other = compute_cell_metrics(mixed_event_cell(Kind.K2), SETTINGS, equal_mass=False)
    assert other is not None
    assert other.false_aligned is None


def test_false_aligned_counts_wrong_alignments_among_confident_ones() -> None:
    decisions = [
        event_decision(0, CLUSTERS[0], 0.95, truth=True),
        event_decision(1, CLUSTERS[1], 0.95, truth=False),
        event_decision(2, CLUSTERS[2], 0.95, truth=True),
        event_decision(3, CLUSTERS[3], 0.6, truth=False),
    ]
    metrics = compute_cell_metrics(make_cell(Kind.K3, decisions), SETTINGS, equal_mass=False, gate_threshold=0.9)
    assert metrics is not None
    assert metrics.false_aligned is not None
    assert metrics.false_aligned.point == pytest.approx(1 / 3)


def test_k4_carries_both_screen_grids_over_the_whole_grid() -> None:
    tags = {SCREEN_TAG: YES}
    metrics = compute_cell_metrics(mixed_event_cell(Kind.K4, tags=tags), SETTINGS, equal_mass=False)
    assert metrics is not None
    assert [point.tau for point in metrics.screen_all] == list(SCREEN_GRID)
    assert [point.tau for point in metrics.screen_stratum] == list(SCREEN_GRID)
    assert metrics.ece_top_label is not None


def test_the_screen_stratum_is_empty_when_no_pair_is_tagged_into_it() -> None:
    metrics = compute_cell_metrics(mixed_event_cell(Kind.K4, tags={SCREEN_TAG: NO}), SETTINGS, equal_mass=False)
    assert metrics is not None
    assert metrics.screen_stratum == ()
    assert len(metrics.screen_all) == len(SCREEN_GRID)


def test_other_kinds_have_no_screen_grid() -> None:
    metrics = compute_cell_metrics(mixed_event_cell(Kind.K2), SETTINGS, equal_mass=False)
    assert metrics is not None
    assert metrics.screen_all == ()
    assert metrics.false_aligned_threshold is None


def test_equal_mass_ece_is_computed_only_on_request() -> None:
    cell = mixed_event_cell(Kind.K2)
    plain = compute_cell_metrics(cell, SETTINGS, equal_mass=False)
    assert plain is not None
    assert plain.ece_equal_mass is None
    requested = compute_cell_metrics(cell, SETTINGS, equal_mass=True)
    assert requested is not None
    assert requested.ece_equal_mass is not None
    assert 0.0 <= requested.ece_equal_mass.point <= 1.0


def test_metrics_are_reproducible_for_a_fixed_seed() -> None:
    cell = mixed_event_cell(Kind.K4)
    first = compute_cell_metrics(cell, SETTINGS, equal_mass=True)
    second = compute_cell_metrics(cell, SETTINGS, equal_mass=True)
    assert first is not None
    assert second is not None
    assert first.auroc == second.auroc
    assert first.ece == second.ece
    assert first.screen_all == second.screen_all


def test_the_test_resamples_stay_small() -> None:
    assert TEST_RESAMPLES <= 200


def _grid_resamples() -> Resamples:
    return Resamples.draw(ClusterIndex.build(["a", "b", "c", "d"]), TEST_RESAMPLES, SEED)


def test_screen_grid_miss_and_flag_rates_match_hand_counts() -> None:
    resamples = _grid_resamples()
    p = floats([0.1, 0.3, 0.6, 0.9])
    y = flags([1, 1, 1, 0])
    points = {point.tau: point for point in screen_grid(resamples, p, y, np.ones(4, dtype=np.bool_))}
    assert points[0.05].miss.point == 0.0
    assert points[0.05].flag.point == 1.0
    assert points[GRID_TAU_DEFAULT].miss.point == pytest.approx(1 / 3)
    assert points[GRID_TAU_DEFAULT].flag.point == pytest.approx(3 / 4)
    assert points[0.5].miss.point == pytest.approx(2 / 3)
    assert points[0.5].flag.point == pytest.approx(2 / 4)
    assert points[0.5].positives == 3
    assert points[0.5].rows == 4


def test_screen_grid_restricts_to_the_member_rows() -> None:
    resamples = _grid_resamples()
    p = floats([0.1, 0.3, 0.6, 0.9])
    y = flags([1, 1, 1, 0])
    members = np.array([True, True, False, False])
    points = {point.tau: point for point in screen_grid(resamples, p, y, members)}
    assert points[GRID_TAU_DEFAULT].miss.point == pytest.approx(1 / 2)
    assert points[GRID_TAU_DEFAULT].rows == 2


def test_screen_grid_without_members_is_empty() -> None:
    resamples = _grid_resamples()
    nobody = np.zeros(4, dtype=np.bool_)
    assert screen_grid(resamples, floats([0.1] * 4), flags([1] * 4), nobody) == ()


def test_majority_accuracy_of_k1_is_the_best_constant_reference() -> None:
    decisions = [
        top_label_decision(0, CLUSTERS[0], HIGH, correct=True, accepted=REF_A),
        top_label_decision(1, CLUSTERS[1], HIGH, correct=True, accepted=REF_A),
        top_label_decision(2, CLUSTERS[2], HIGH, correct=True, accepted=REF_B),
    ]
    assert majority_accuracy(decisions, Kind.K1) == pytest.approx(2 / 3)


def test_majority_accuracy_of_a_binary_kind_is_the_larger_class_share() -> None:
    decisions = [event_decision(index, CLUSTERS[0], HIGH, truth=index < 3) for index in range(4)]
    assert majority_accuracy(decisions, Kind.K2) == 0.75
    assert math.isnan(majority_accuracy([], Kind.K2))


def test_k1_baselines_hold_the_majority_and_always_declared_accuracies() -> None:
    decisions = [
        top_label_decision(0, CLUSTERS[0], HIGH, correct=True, accepted=REF_A),
        top_label_decision(1, CLUSTERS[1], HIGH, correct=True, accepted=REF_A),
        top_label_decision(2, CLUSTERS[2], HIGH, correct=True, accepted=REF_B),
    ]
    figures = baseline_figures(decisions, Kind.K1)
    assert figures == pytest.approx({"majority_accuracy": 2 / 3, "always_declared_accuracy": 2 / 3})


def test_k2_baselines_hold_the_base_rate_and_its_brier_score() -> None:
    decisions = [event_decision(index, CLUSTERS[0], HIGH, truth=index < 1) for index in range(4)]
    figures = baseline_figures(decisions, Kind.K2)
    assert figures["base_rate"] == 0.25
    assert figures["majority_brier"] == pytest.approx(0.25 * 0.75)
    assert "flag_everything_accuracy" not in figures


def test_k4_baselines_add_flag_everything_and_flag_nothing() -> None:
    decisions = [event_decision(index, CLUSTERS[0], HIGH, truth=index < 1) for index in range(4)]
    figures = baseline_figures(decisions, Kind.K4)
    assert figures["flag_everything_accuracy"] == 0.25
    assert figures["flag_nothing_accuracy"] == 0.75


def test_baselines_of_no_decision_are_empty() -> None:
    assert baseline_figures([], Kind.K3) == {}
