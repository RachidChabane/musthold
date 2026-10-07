"""Descriptive comparisons (PREREG 9): shared items, the two-signal ALIGNED simulation and the model-size pairs."""

from __future__ import annotations

from collections.abc import Sequence
from typing import Final

import pytest

from calibration.analysis.cell_metrics import MetricSettings
from calibration.analysis.comparisons import (
    MODEL_SIZE_PAIRS,
    PINS_OFFSPEC_THRESHOLD,
    TwoSignalInputs,
    aligned_pairs,
    compare_cells,
    model_size_comparisons,
    two_signal,
)
from calibration.analysis.model import CellData, Decision, Kind
from calibration.domain.jsonread import json_object
from calibration.domain.models import MODEL_IDS
from calibration.domain.ports import MatrixSignature
from support.check_cases import cell, decision

SETTINGS: Final = MetricSettings(resamples=50)
CLUSTERS: Final = ("c1", "c2", "c3", "c4")
FIRST_CORRECT: Final = (1, 1, 0, 0, 1, 0, 1, 1)
SECOND_CORRECT: Final = (1, 0, 0, 1, 1, 0, 1, 1)
THRESHOLD: Final = 0.9


def _k3(unit: str, cluster: str, correct: bool, *, truth: bool = True, has_prediction: bool = True) -> Decision:
    event = 0.9 if truth == correct else 0.1
    return decision(
        unit,
        cluster,
        0.9,
        correct,
        event=event,
        truth=truth,
        predicted=truth == correct,
        has_prediction=has_prediction,
    )


def _paired_cell(correct: Sequence[int], *, model: str) -> CellData:
    decisions = [
        _k3(f"u{index}", CLUSTERS[index % len(CLUSTERS)], bool(flag), truth=index % 2 == 0)
        for index, flag in enumerate(correct)
    ]
    return cell(Kind.K3, decisions, model=model)


class TestAlignedPairs:
    def test_only_units_predicted_by_both_cells_are_paired_in_unit_order(self) -> None:
        first = cell(Kind.K3, [_k3("b", "c", True), _k3("a", "c", True), _k3("x", "c", True)])
        second = cell(Kind.K3, [_k3("a", "c", False), _k3("b", "c", True, has_prediction=False), _k3("y", "c", True)])
        left, right = aligned_pairs(first, second)
        assert [d.unit for d in left] == ["a"]
        assert [d.correct for d in right] == [False]

    def test_no_shared_unit_gives_empty_lists(self) -> None:
        assert aligned_pairs(cell(Kind.K3, [_k3("a", "c", True)]), cell(Kind.K3, [_k3("b", "c", True)])) == ([], [])


class TestCompareCells:
    def test_cells_without_shared_items_report_zero(self) -> None:
        first = cell(Kind.K3, [_k3("a", "c", True)])
        second = cell(Kind.K3, [_k3("b", "c", True)])
        assert compare_cells(first, second, SETTINGS) == {"shared": 0}

    def test_both_wrong_and_conditional_error_come_from_the_shared_items(self) -> None:
        record = compare_cells(
            _paired_cell(FIRST_CORRECT, model="M1"), _paired_cell(SECOND_CORRECT, model="host"), SETTINGS
        )
        assert record["shared"] == len(FIRST_CORRECT)
        assert record["clusters"] == len(CLUSTERS)
        assert record["both_wrong"] == 2
        conditional = json_object(record["first_wrong_given_second_wrong"], "conditional")
        assert conditional["point"] == pytest.approx(2 / 3)

    def test_accuracy_of_each_scorer_and_their_difference(self) -> None:
        record = compare_cells(
            _paired_cell(FIRST_CORRECT, model="M1"), _paired_cell(SECOND_CORRECT, model="host"), SETTINGS
        )
        accuracy = json_object(record["accuracy"], "accuracy")
        assert accuracy["first"] == pytest.approx(sum(FIRST_CORRECT) / len(FIRST_CORRECT))
        assert accuracy["second"] == pytest.approx(sum(SECOND_CORRECT) / len(SECOND_CORRECT))
        difference = json_object(accuracy["difference"], "difference")
        assert difference["point"] == pytest.approx(0.0)

    def test_every_metric_of_section_nine_is_reported(self) -> None:
        record = compare_cells(
            _paired_cell(FIRST_CORRECT, model="M1"), _paired_cell(SECOND_CORRECT, model="host"), SETTINGS
        )
        assert {"accuracy", "auroc", "ece", "brier"} <= set(record)


class TestModelSizeComparisons:
    def test_a_missing_member_makes_the_pair_blocked(self) -> None:
        cells = {"M3": _paired_cell(FIRST_CORRECT, model="M3"), "M2": _paired_cell(SECOND_CORRECT, model="M2")}
        record = model_size_comparisons(cells, SETTINGS)
        assert record["M4-M5"] == "BLOCKED"
        assert json_object(record["M3-M2"], "pair")["shared"] == len(FIRST_CORRECT)

    def test_a_blocked_cell_makes_its_pairs_blocked(self) -> None:
        blocked = cell(Kind.K3, [], model="M2", blocked_reason="memory-fit")
        cells = {"M3": _paired_cell(FIRST_CORRECT, model="M3"), "M2": blocked}
        assert model_size_comparisons(cells, SETTINGS)["M3-M2"] == "BLOCKED"

    def test_one_entry_is_reported_per_pair(self) -> None:
        record = model_size_comparisons({}, SETTINGS)
        assert list(record) == [f"{first}-{second}" for first, second in MODEL_SIZE_PAIRS]

    def test_the_pairs_are_those_of_section_nine(self) -> None:
        pairs = set(MODEL_SIZE_PAIRS)
        assert len(pairs) == len(MODEL_SIZE_PAIRS)
        assert {("M3", "M2"), ("M4", "M5"), ("M5", "M6"), ("M1", "M6"), ("M1", "M2")} <= pairs
        assert {(model, "M1") for model in MODEL_IDS if model != "M1"} <= pairs


def _signature(nodeid: str, signature: str, none_share: float) -> MatrixSignature:
    return MatrixSignature(nodeid, 1, signature, "ALIGNED", none_share)


def _two_signal_case() -> tuple[CellData, TwoSignalInputs]:
    rows = (
        ("u1", 0.95, True, True, "SPECIFIC", 0.0),
        ("u2", 0.95, True, False, "OVERBROAD", 0.0),
        ("u3", 0.95, True, True, "SPECIFIC", PINS_OFFSPEC_THRESHOLD + 0.1),
        ("u4", 0.50, True, True, "SPECIFIC", 0.0),
        ("u5", 0.90, False, False, "PARTIAL", 0.0),
        ("u6", 0.95, True, True, "PARTIAL", 0.0),
    )
    decisions = [
        decision(
            unit,
            "c",
            confidence,
            predicted == truth,
            event=confidence,
            truth=truth,
            predicted=predicted,
        )
        for unit, confidence, predicted, truth, _, _ in rows
    ]
    signatures = {unit: _signature(unit, kind, share) for unit, _, _, _, kind, share in rows}
    return cell(Kind.K3, decisions), TwoSignalInputs(signatures)


class TestTwoSignal:
    def test_only_confident_aligned_decisions_with_a_specific_matrix_that_pins_nothing_are_chosen(self) -> None:
        source, inputs = _two_signal_case()
        record = two_signal(source, inputs, THRESHOLD)
        assert record["tests"] == 6
        assert record["two_signal_aligned"] == 2
        false_aligned = json_object(record["false_aligned"], "false_aligned")
        assert false_aligned["point"] == pytest.approx(0.5)

    def test_a_none_share_at_the_limit_does_not_pin_an_off_spec_test(self) -> None:
        source, inputs = _two_signal_case()
        at_limit = {**inputs.signatures, "u3": _signature("u3", "SPECIFIC", PINS_OFFSPEC_THRESHOLD)}
        assert two_signal(source, TwoSignalInputs(at_limit), THRESHOLD)["two_signal_aligned"] == 3

    def test_the_error_table_crosses_the_source_with_the_matrix_verdict(self) -> None:
        source, inputs = _two_signal_case()
        assert two_signal(source, inputs, THRESHOLD)["table_source_by_matrix"] == [[4, 1], [0, 1]]

    def test_both_wrong_counts_the_tests_both_signals_get_wrong(self) -> None:
        source, inputs = _two_signal_case()
        record = two_signal(source, inputs, THRESHOLD)
        assert record["both_wrong"] == 1
        conditional = json_object(record["source_wrong_given_matrix_wrong"], "conditional")
        assert conditional["point"] == pytest.approx(0.5)

    def test_a_lower_threshold_admits_the_less_confident_decision(self) -> None:
        source, inputs = _two_signal_case()
        assert two_signal(source, inputs, 0.4)["two_signal_aligned"] == 3

    def test_tests_without_a_signature_are_ignored(self) -> None:
        source, inputs = _two_signal_case()
        reduced = TwoSignalInputs({unit: sig for unit, sig in inputs.signatures.items() if unit != "u1"})
        assert two_signal(source, reduced, THRESHOLD)["tests"] == 5
