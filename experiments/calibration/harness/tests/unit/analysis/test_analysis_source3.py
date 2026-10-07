"""Source (3): the operating points of PREREG section 7.3 and the UNDETERMINED figures."""

from __future__ import annotations

from dataclasses import replace
from typing import Final

import pytest

from calibration.analysis.model import CellData, Decision, Kind
from calibration.analysis.source3 import (
    RULE_CONFIDENCE,
    RULE_UNANIMOUS,
    OperatingPoint,
    decided,
    operating_points,
    undetermined_figures,
)
from calibration.domain.constants import (
    GATE_THRESHOLD_JH3,
    GATE_THRESHOLD_JH5,
    GATE_THRESHOLD_S1,
    VH2_JUDGE_THRESHOLD,
)
from calibration.domain.sources import SourceId
from support.metric_cases import CLUSTERS, event_decision, make_cell, top_label_decision

HOST: Final = "host"
K1_THRESHOLD: Final = OperatingPoint(RULE_CONFIDENCE, GATE_THRESHOLD_S1, "x")
VH2_POINT: Final = OperatingPoint(RULE_UNANIMOUS, VH2_JUDGE_THRESHOLD, "x")


@pytest.mark.parametrize(
    ("source", "kind", "threshold"),
    [
        (SourceId.S1, Kind.K3, GATE_THRESHOLD_S1),
        (SourceId.S1, Kind.K1, GATE_THRESHOLD_S1),
        (SourceId.JH5, Kind.K1, GATE_THRESHOLD_JH5),
        (SourceId.JH5, Kind.K2, GATE_THRESHOLD_JH5),
        (SourceId.JH5, Kind.K3, GATE_THRESHOLD_JH5),
        (SourceId.JH3, Kind.K1, GATE_THRESHOLD_JH3),
        (SourceId.JH3, Kind.K3, GATE_THRESHOLD_JH3),
        (SourceId.VH2, Kind.K4, VH2_JUDGE_THRESHOLD),
    ],
)
def test_each_source_and_kind_has_the_operating_point_of_the_table(
    source: SourceId, kind: Kind, threshold: float
) -> None:
    (point,) = operating_points(source, kind)
    assert point.threshold == threshold


def test_the_thresholds_are_the_numbers_of_the_pre_registration() -> None:
    assert (GATE_THRESHOLD_S1, GATE_THRESHOLD_JH5, GATE_THRESHOLD_JH3, VH2_JUDGE_THRESHOLD) == (0.90, 0.80, 0.90, 0.70)


def test_the_screen_has_no_undetermined_outcome_under_source_one() -> None:
    assert operating_points(SourceId.S1, Kind.K4) == []
    assert operating_points(SourceId.S1, Kind.K2) == []
    assert operating_points(SourceId.JH5, Kind.K4) == []


def test_the_vote_sources_use_the_confidence_rule_and_vh2_the_unanimity_rule() -> None:
    assert operating_points(SourceId.JH5, Kind.K3)[0].rule == RULE_CONFIDENCE
    assert operating_points(SourceId.VH2, Kind.K4)[0].rule == RULE_UNANIMOUS


def test_a_decision_is_decided_from_the_threshold_up() -> None:
    at = top_label_decision(0, CLUSTERS[0], GATE_THRESHOLD_S1, correct=True)
    below = top_label_decision(1, CLUSTERS[0], 0.8999, correct=True)
    assert decided(at, K1_THRESHOLD, SourceId.S1)
    assert not decided(below, K1_THRESHOLD, SourceId.S1)


def test_a_decision_without_prediction_is_never_decided() -> None:
    silent = Decision("u", CLUSTERS[0], 1.0, True, has_prediction=False)
    assert not decided(silent, K1_THRESHOLD, SourceId.S1)
    assert not decided(replace(silent, alt_confidence=1.0), VH2_POINT, SourceId.VH2)


def test_vh2_reads_the_weakest_stated_confidence_of_two_agreeing_votes() -> None:
    agreed = event_decision(0, CLUSTERS[0], 1.0, truth=True, alt=VH2_JUDGE_THRESHOLD)
    weak = event_decision(1, CLUSTERS[0], 1.0, truth=True, alt=0.69)
    split = event_decision(2, CLUSTERS[0], 1.0, truth=True, alt=None)
    assert decided(agreed, VH2_POINT, SourceId.VH2)
    assert not decided(weak, VH2_POINT, SourceId.VH2)
    assert not decided(split, VH2_POINT, SourceId.VH2)


def _k1_cell() -> CellData:
    decisions = [
        top_label_decision(0, CLUSTERS[0], 0.95, correct=True),
        top_label_decision(1, CLUSTERS[1], 0.92, correct=False),
        top_label_decision(2, CLUSTERS[2], 0.5, correct=True),
        top_label_decision(3, CLUSTERS[3], 0.3, correct=False),
        Decision("silent", CLUSTERS[0], 0.0, False, has_prediction=False),
    ]
    return make_cell(Kind.K1, decisions)


def test_k1_figures_count_decided_error_and_the_composition_of_the_rest() -> None:
    figures = undetermined_figures(_k1_cell(), K1_THRESHOLD)
    assert figures["decisions"] == 5
    assert figures["decided"] == 2
    assert figures["undetermined"] == 3
    assert figures["undetermined_rate"] == pytest.approx(0.6)
    assert figures["error_rate_among_decided"] == pytest.approx(0.5)
    assert figures["composition"] == {"would_be_correct": 1, "would_be_wrong": 1, "no_prediction": 1}
    assert figures["threshold"] == GATE_THRESHOLD_S1


def test_k2_to_k4_figures_split_the_rest_by_truth() -> None:
    decisions = [
        event_decision(0, CLUSTERS[0], 0.95, truth=True),
        event_decision(1, CLUSTERS[1], 0.95, truth=False),
        event_decision(2, CLUSTERS[2], 0.7, truth=True),
        event_decision(3, CLUSTERS[3], 0.2, truth=False),
        Decision("silent", CLUSTERS[0], 0.0, False, has_prediction=False),
    ]
    figures = undetermined_figures(make_cell(Kind.K3, decisions), K1_THRESHOLD)
    assert figures["decided"] == 2
    assert figures["undetermined_rate"] == pytest.approx(3 / 5)
    assert figures["error_rate_among_decided"] == pytest.approx(0.5)
    assert figures["composition"] == {"truth_positive": 1, "truth_negative": 1, "no_prediction": 1}


def test_a_cell_without_decisions_has_no_rates() -> None:
    figures = undetermined_figures(make_cell(Kind.K3, []), K1_THRESHOLD)
    assert figures["undetermined_rate"] is None
    assert figures["error_rate_among_decided"] is None
    assert figures["decided"] == 0


def test_a_cell_decided_nowhere_has_no_error_rate() -> None:
    decisions = [top_label_decision(0, CLUSTERS[0], 0.1, correct=False)]
    figures = undetermined_figures(make_cell(Kind.K1, decisions), K1_THRESHOLD)
    assert figures["undetermined_rate"] == 1.0
    assert figures["error_rate_among_decided"] is None
