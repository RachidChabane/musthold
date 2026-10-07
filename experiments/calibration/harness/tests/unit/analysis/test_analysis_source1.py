"""Source (1) decisions of one model on a tiny hand-built world, against hand-computed figures."""

from __future__ import annotations

from collections.abc import Mapping
from typing import Final

import pytest

from calibration.analysis.model import CellData, Decision, Kind
from calibration.analysis.replies import ReplyOutcome
from calibration.analysis.source1 import (
    ScreenPairs,
    alignment_cells,
    cluster_of,
    screen_pairs,
    strata_tags,
)
from calibration.domain.ports import DecodedAnswer
from calibration.domain.schedule import alignment_request_id
from calibration.domain.sources import SourceId
from support.analysis_cases import (
    ALIGNMENT_ITEMS,
    CONSISTENT,
    MUTANT_F,
    MUTANT_N,
    MUTANT_P,
    MUTANT_R,
    MUTANT_T,
    NODE_GOOD,
    NODE_MIS,
    NODE_VAC,
    NONE,
    REF_ONE,
    REF_THREE,
    REF_TWO,
    SCREEN_ITEMS,
    TOKEN_ONE,
    TOKEN_THREE,
    TOKEN_TWO,
    VIOLATES,
    alignment_outcome,
    fall_through,
    screen_document,
    screen_outcome,
    truth,
)
from support.fake_product_reader import FakeProductReader

MODEL: Final = "M9"
SOURCE: Final = "system-one:tiny:model@abc"
STATE_TOO_LARGE: Final = "STATE_TOO_LARGE"
MISSING: Final = "MISSING"
RELAY_REFUSED: Final = "RELAY_REFUSED"
GOOD_CRITERION: Final = (TOKEN_ONE, 0.9, {TOKEN_ONE: 0.9, TOKEN_TWO: 0.05, TOKEN_THREE: 0.03, NONE: 0.02})
GOOD_COVERAGE: Final = ("FULL", 0.8, (0.05, 0.05, 0.1, 0.8))
VAC_CRITERION: Final = (NONE, 0.6, {TOKEN_ONE: 0.1, TOKEN_TWO: 0.1, TOKEN_THREE: 0.1, NONE: 0.7})
VAC_COVERAGE: Final = ("WEAK", 0.7, (0.1, 0.6, 0.2, 0.1))
CLUSTER_ONE: Final = "PARK-1~1"
CLUSTER_TWO: Final = "PARK-2~1"


def _alignment_outcomes() -> dict[str, ReplyOutcome]:
    return {
        alignment_request_id(NODE_GOOD): alignment_outcome(NODE_GOOD, GOOD_CRITERION, GOOD_COVERAGE),
        alignment_request_id(NODE_MIS): fall_through(alignment_request_id(NODE_MIS), STATE_TOO_LARGE),
        alignment_request_id(NODE_VAC): alignment_outcome(NODE_VAC, VAC_CRITERION, VAC_COVERAGE),
    }


@pytest.fixture(scope="module")
def cells() -> dict[Kind, CellData]:
    return alignment_cells(
        MODEL, SOURCE, _alignment_outcomes(), ALIGNMENT_ITEMS, truth(consensus=frozenset({NODE_GOOD}))
    )


def _decision(cell: CellData, unit: str) -> Decision:
    return next(decision for decision in cell.decisions if decision.unit == unit)


def test_a_fall_through_counts_in_k1_and_k3_but_not_in_k2(cells: dict[Kind, CellData]) -> None:
    assert (cells[Kind.K1].units, cells[Kind.K2].units, cells[Kind.K3].units) == (3, 2, 3)
    assert cells[Kind.K1].fall_through == {MISSING: 0, STATE_TOO_LARGE: 1, RELAY_REFUSED: 0}
    assert cells[Kind.K3].fall_through[STATE_TOO_LARGE] == 1
    assert cells[Kind.K2].fall_through[STATE_TOO_LARGE] == 0


def test_the_decisions_exclude_fall_throughs_and_mis_other_from_k2(cells: dict[Kind, CellData]) -> None:
    assert [d.unit for d in cells[Kind.K1].decisions] == [NODE_GOOD, NODE_VAC]
    assert [d.unit for d in cells[Kind.K2].decisions] == [NODE_GOOD, NODE_VAC]
    assert cells[Kind.K1].missing_share == pytest.approx(1 / 3)


def test_the_cells_carry_the_source_the_model_and_the_clusters(cells: dict[Kind, CellData]) -> None:
    cell = cells[Kind.K1]
    assert (cell.source, cell.model, cell.confidence_source) == (SourceId.S1, MODEL, SOURCE)
    assert cell.unit_clusters == (CLUSTER_ONE, CLUSTER_TWO, CLUSTER_TWO)
    assert cells[Kind.K2].unit_clusters == (CLUSTER_ONE, CLUSTER_TWO)


def test_k1_is_correct_when_the_named_criterion_is_accepted(cells: dict[Kind, CellData]) -> None:
    good, vacuous = _decision(cells[Kind.K1], NODE_GOOD), _decision(cells[Kind.K1], NODE_VAC)
    assert (good.confidence, good.correct, good.prediction) == (0.9, True, REF_ONE)
    assert (vacuous.confidence, vacuous.correct, vacuous.prediction) == (0.6, True, NONE)
    assert good.alt_confidence == 0.9
    assert vacuous.alt_confidence == 0.7
    assert good.accepted == frozenset({REF_ONE})
    assert vacuous.accepted == frozenset({NONE, REF_TWO})
    assert good.event_probability is None
    assert good.consensus
    assert not vacuous.consensus


def test_k2_reads_the_adequacy_of_the_coverage_level(cells: dict[Kind, CellData]) -> None:
    good, vacuous = _decision(cells[Kind.K2], NODE_GOOD), _decision(cells[Kind.K2], NODE_VAC)
    assert (good.predicted_positive, good.truth_positive, good.correct) == (True, True, True)
    assert good.event_probability == 0.8
    assert (vacuous.predicted_positive, vacuous.truth_positive, vacuous.correct) == (False, False, True)
    assert vacuous.event_probability == pytest.approx(0.3)
    assert good.alt_event_probability == pytest.approx(0.9)
    assert vacuous.alt_confidence == pytest.approx(0.7)


def test_k3_is_aligned_when_the_declared_criterion_is_named_with_adequate_coverage(cells: dict[Kind, CellData]) -> None:
    good, vacuous = _decision(cells[Kind.K3], NODE_GOOD), _decision(cells[Kind.K3], NODE_VAC)
    assert (good.predicted_positive, good.truth_positive, good.correct) == (True, True, True)
    assert (vacuous.predicted_positive, vacuous.truth_positive, vacuous.correct) == (False, False, True)


def test_k3_confidence_is_the_smaller_of_the_two_confidences(cells: dict[Kind, CellData]) -> None:
    good, vacuous = _decision(cells[Kind.K3], NODE_GOOD), _decision(cells[Kind.K3], NODE_VAC)
    assert good.confidence == 0.8
    assert good.event_probability == 0.8
    assert vacuous.confidence == 0.6
    assert vacuous.event_probability == pytest.approx(0.4)


def test_k3_returned_probability_is_the_smaller_of_the_two_probabilities(cells: dict[Kind, CellData]) -> None:
    good, vacuous = _decision(cells[Kind.K3], NODE_GOOD), _decision(cells[Kind.K3], NODE_VAC)
    assert good.alt_event_probability == pytest.approx(0.9)
    assert vacuous.alt_event_probability == pytest.approx(0.1)
    assert vacuous.alt_confidence == pytest.approx(0.9)


def test_a_wrong_k1_answer_is_incorrect() -> None:
    wrong = alignment_outcome(NODE_GOOD, (TOKEN_THREE, 0.7, {TOKEN_THREE: 0.7, TOKEN_ONE: 0.3}), GOOD_COVERAGE)
    outcomes = {alignment_request_id(NODE_GOOD): wrong}
    cell = alignment_cells(MODEL, SOURCE, outcomes, ALIGNMENT_ITEMS, truth())[Kind.K1]
    assert not cell.decisions[0].correct
    assert cell.decisions[0].prediction == REF_THREE


def test_an_absent_confidence_counts_as_zero() -> None:
    outcome = alignment_outcome(NODE_GOOD, (TOKEN_ONE, 0.0, {TOKEN_ONE: 1.0}), GOOD_COVERAGE)
    answers = dict(outcome.answers)
    answers["criterion"] = DecodedAnswer("criterion", TOKEN_ONE, None, {TOKEN_ONE: 1.0})
    stripped = ReplyOutcome(outcome.request_id, outcome.status, answers, outcome.returned, None, None)
    cell = alignment_cells(MODEL, SOURCE, {outcome.request_id: stripped}, ALIGNMENT_ITEMS, truth())[Kind.K3]
    assert cell.decisions[0].confidence == 0.0


def _screen_outcomes() -> dict[str, ReplyOutcome]:
    documents: Mapping[str, Mapping[str, tuple[str, float | None]]] = {
        MUTANT_T: {"q1": (VIOLATES, 0.8), "q2": (CONSISTENT, 0.1)},
        MUTANT_P: {"q1": (CONSISTENT, None), "q2": (VIOLATES, None)},
        MUTANT_R: {"q1": (VIOLATES, 0.6)},
        MUTANT_N: {"q1": (VIOLATES, 0.9), "q2": (VIOLATES, 0.9)},
    }
    outcomes = {
        screen_outcome(mutant, screen_document(answers)).request_id: screen_outcome(mutant, screen_document(answers))
        for mutant, answers in documents.items()
    }
    refused = fall_through(f"M|{MUTANT_F}", STATE_TOO_LARGE)
    outcomes[refused.request_id] = refused
    return outcomes


@pytest.fixture(scope="module")
def pairs() -> ScreenPairs:
    return screen_pairs(MODEL, SOURCE, _screen_outcomes(), SCREEN_ITEMS, truth(), FakeProductReader())


def _pair(pairs: ScreenPairs, mutant: str, ref: str) -> Decision:
    return next(d for d in pairs.cell.decisions if d.unit == f"{mutant}|{ref}")


def test_a_mutant_without_an_oracle_record_is_left_out_and_listed(pairs: ScreenPairs) -> None:
    assert pairs.without_oracle == (MUTANT_N,)
    assert all(not d.unit.startswith(MUTANT_N) for d in pairs.cell.decisions)


def test_each_answered_pair_is_a_decision_and_every_pair_a_unit(pairs: ScreenPairs) -> None:
    cell = pairs.cell
    assert cell.units == 8
    assert len(cell.decisions) == 5
    assert len(cell.unit_clusters) == 8
    assert cell.fall_through == {MISSING: 0, STATE_TOO_LARGE: 2, RELAY_REFUSED: 1}
    assert cell.missing_share == pytest.approx(3 / 8)


def test_the_screen_truth_comes_from_the_oracle(pairs: ScreenPairs) -> None:
    violating = _pair(pairs, MUTANT_T, REF_ONE)
    consistent = _pair(pairs, MUTANT_T, REF_TWO)
    assert (violating.truth_positive, violating.predicted_positive, violating.correct) == (True, True, True)
    assert violating.confidence == 0.8
    assert violating.event_probability == 0.8
    assert (consistent.truth_positive, consistent.predicted_positive, consistent.correct) == (False, False, True)
    assert consistent.confidence == pytest.approx(0.9)
    assert consistent.event_probability == pytest.approx(0.1)


def test_an_answer_without_a_probability_counts_one_for_violates_and_zero_for_consistent(pairs: ScreenPairs) -> None:
    consistent = _pair(pairs, MUTANT_P, REF_ONE)
    violates = _pair(pairs, MUTANT_P, REF_TWO)
    assert (consistent.event_probability, consistent.confidence, consistent.correct) == (0.0, 1.0, True)
    assert (violates.event_probability, violates.confidence, violates.correct) == (1.0, 1.0, False)
    assert consistent.alt_event_probability is None
    assert _pair(pairs, MUTANT_T, REF_ONE).alt_event_probability == 0.8


def test_a_document_that_does_not_answer_a_question_is_a_relay_refusal(pairs: ScreenPairs) -> None:
    assert _pair(pairs, MUTANT_R, REF_ONE).correct is False
    assert not any(d.unit == f"{MUTANT_R}|{REF_TWO}" for d in pairs.cell.decisions)


def test_the_clusters_are_the_generator_call_and_the_planted_source(pairs: ScreenPairs) -> None:
    assert _pair(pairs, MUTANT_T, REF_ONE).cluster == "T:gen-7"
    assert _pair(pairs, MUTANT_P, REF_ONE).cluster == "P:3"
    assert _pair(pairs, MUTANT_R, REF_ONE).cluster == "P:1"
    assert cluster_of(truth(), MUTANT_F) == "T:gen-8"


def test_the_readings_of_every_answered_pair_are_kept(pairs: ScreenPairs) -> None:
    assert len(pairs.readings) == 6


def test_strata_tags_of_a_killed_pair() -> None:
    tags = strata_tags(truth(), MUTANT_T, REF_ONE)
    assert tags == {"arm": "T", "f3": "DIFFERS", "target": "yes", "kill": "yes", "screen": "no"}


def test_strata_tags_of_a_surviving_pair_with_a_witness() -> None:
    tags = strata_tags(truth(), MUTANT_T, REF_TWO)
    assert tags == {"arm": "T", "f3": "DIFFERS", "target": "no", "kill": "no", "screen": "yes"}


def test_a_survivor_without_a_differential_witness_is_outside_the_screen_stratum() -> None:
    tags = strata_tags(truth(), MUTANT_P, REF_THREE)
    assert tags == {"arm": "P", "f3": "NO_DIFF_FOUND", "target": "yes", "kill": "no", "screen": "no"}
    assert strata_tags(truth(), MUTANT_F, REF_TWO)["f3"] == "SKIPPED"


def test_the_pairs_carry_their_strata_tags(pairs: ScreenPairs) -> None:
    assert _pair(pairs, MUTANT_T, REF_TWO).tags["screen"] == "yes"
    assert _pair(pairs, MUTANT_P, REF_ONE).tags["screen"] == "no"
