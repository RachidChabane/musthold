"""Sensitivity analyses (PREREG 13): each one is reported next to the primary figures and feeds no band."""

from __future__ import annotations

from typing import Final

import pytest

from calibration.analysis.cell_metrics import MetricSettings
from calibration.analysis.model import Decision, Kind
from calibration.analysis.sensitivity import (
    cell_sensitivities,
    clustered_by_criterion,
    consensus_only,
    strata_values,
    stratum,
    summary,
    with_lenient_truth,
    with_returned_probability,
)
from calibration.domain.jsonread import json_object
from support.check_cases import cell, decision

SETTINGS: Final = MetricSettings(resamples=30)
CLUSTERS: Final = ("c1", "c2", "c3", "c4")
ROWS: Final = 16
ALTERNATIVE: Final = 0.3
STATED_EVENT: Final = 0.8
SCREEN_REFS: Final = ("PARK-1~1", "PARK-2~1")


def _alignment_decisions(*, with_alt: bool = True) -> list[Decision]:
    return [
        decision(
            f"t{index}",
            CLUSTERS[index % len(CLUSTERS)],
            0.5 + 0.03 * index,
            index % 3 != 0,
            event=0.2 + 0.04 * index,
            truth=index % 2 == 0,
            predicted=index % 2 == 0,
            alt=0.4 + 0.02 * index if with_alt else None,
            alt_event=0.1 + 0.05 * index if with_alt else None,
            lenient=index % 3 != 0 or index % 2 == 0,
            consensus=index % 4 != 0,
        )
        for index in range(ROWS)
    ]


def _screen_decisions() -> list[Decision]:
    return [
        decision(
            f"m{index}|{SCREEN_REFS[index % len(SCREEN_REFS)]}",
            CLUSTERS[index % len(CLUSTERS)],
            0.6 + 0.02 * index,
            index % 3 != 0,
            event=0.1 + 0.05 * index,
            truth=index % 2 == 0,
            predicted=index % 2 == 0,
            tags={
                "arm": "T" if index % 2 else "P",
                "f3": "DIFFERS" if index % 3 else "NO_DIFF_FOUND",
                "target": "yes" if index % 4 else "no",
                "kill": "no",
                "screen": "yes",
            },
        )
        for index in range(ROWS)
    ]


class TestSummary:
    def test_an_empty_selection_has_no_units(self) -> None:
        assert summary(Kind.K1, [], SETTINGS) == {"n": 0}

    def test_decisions_without_a_prediction_are_not_counted(self) -> None:
        decisions = [*_alignment_decisions(), decision("x", "c1", 0.0, False, has_prediction=False)]
        assert summary(Kind.K1, decisions, SETTINGS)["n"] == ROWS

    def test_the_summary_reports_the_primary_figures_with_their_clusters(self) -> None:
        record = summary(Kind.K3, _alignment_decisions(), SETTINGS)
        assert record["clusters"] == len(CLUSTERS)
        assert {"accuracy", "auroc", "ece", "brier"} <= set(record)


class TestTransformations:
    def test_the_returned_probability_replaces_the_confidence_and_drops_decisions_without_one(self) -> None:
        kept = decision("a", "c", 0.9, True, event=STATED_EVENT, alt=ALTERNATIVE)
        dropped = decision("b", "c", 0.9, True, event=STATED_EVENT)
        swapped = with_returned_probability([kept, dropped])
        assert [(d.unit, d.confidence, d.event_probability) for d in swapped] == [("a", ALTERNATIVE, STATED_EVENT)]

    def test_the_alternative_event_probability_replaces_the_event_when_there_is_one(self) -> None:
        swapped = with_returned_probability([decision("a", "c", 0.9, True, event=STATED_EVENT, alt=0.4, alt_event=0.1)])
        assert swapped[0].event_probability == pytest.approx(0.1)

    def test_lenient_truth_replaces_correctness_and_drops_undefined_ones(self) -> None:
        decisions = [decision("a", "c", 0.5, False, lenient=True), decision("b", "c", 0.5, True)]
        assert [(d.unit, d.correct) for d in with_lenient_truth(decisions)] == [("a", True)]

    def test_k4_is_reclustered_by_the_criterion_of_the_pair(self) -> None:
        reclustered = clustered_by_criterion(_screen_decisions())
        assert {d.cluster for d in reclustered} == set(SCREEN_REFS)

    def test_the_consensus_selection_keeps_only_agreed_tests(self) -> None:
        decisions = [decision("a", "c", 0.5, True, consensus=True), decision("b", "c", 0.5, True, consensus=False)]
        assert [d.unit for d in consensus_only(decisions)] == ["a"]

    def test_a_stratum_keeps_the_decisions_with_the_tag_value(self) -> None:
        decisions = _screen_decisions()
        selected = stratum(decisions, "arm", "T")
        assert selected
        assert all(d.tags["arm"] == "T" for d in selected)
        assert strata_values(decisions, "arm") == ["P", "T"]


class TestCellSensitivities:
    def test_k1_reports_consensus_lenient_and_the_alternative_measure(self) -> None:
        record = cell_sensitivities(cell(Kind.K1, _alignment_decisions()), SETTINGS)
        assert set(record) == {"primary", "consensus", "lenient", "alternative_measure"}

    def test_k2_and_k3_have_no_lenient_variant(self) -> None:
        for kind in (Kind.K2, Kind.K3):
            record = cell_sensitivities(cell(kind, _alignment_decisions()), SETTINGS)
            assert set(record) == {"primary", "consensus", "alternative_measure"}

    def test_a_source_without_the_alternative_measure_has_no_such_entry(self) -> None:
        record = cell_sensitivities(cell(Kind.K3, _alignment_decisions(with_alt=False)), SETTINGS)
        assert set(record) == {"primary", "consensus"}

    def test_k4_has_no_consensus_and_reports_the_strata(self) -> None:
        record = cell_sensitivities(cell(Kind.K4, _screen_decisions()), SETTINGS)
        assert {"primary", "clustered_by_criterion", "f3_differs", "strata"} <= set(record)
        assert "consensus" not in record
        assert "lenient" not in record
        strata = json_object(record["strata"], "strata")
        assert {"arm=T", "arm=P", "f3=DIFFERS", "f3=NO_DIFF_FOUND", "target=yes", "target=no", "kill=no"} <= set(strata)
        assert "screen=yes" not in strata

    def test_a_cell_without_predictions_has_empty_summaries(self) -> None:
        record = cell_sensitivities(cell(Kind.K3, [decision("a", "c", 0.0, False, has_prediction=False)]), SETTINGS)
        assert record["primary"] == {"n": 0}
