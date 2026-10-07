"""Source (2) decisions from recorded host votes on a tiny hand-built world, against hand-computed figures."""

from __future__ import annotations

from collections.abc import Mapping
from typing import Final

import pytest

from calibration.analysis.model import HOST_MODEL, CellData, Decision, Kind
from calibration.analysis.source2 import host_vote_cells, source2_cells, validity_cell
from calibration.analysis.votes import JudgeVote, ValidityVote
from calibration.domain.sources import SourceId
from support.analysis_cases import (
    ALIGNMENT_ITEMS,
    MUTANT_P,
    MUTANT_R,
    MUTANT_T,
    NODE_GOOD,
    NODE_VAC,
    NONE,
    REF_ONE,
    REF_TWO,
    SCREEN_ITEMS,
    truth,
)

ITEMS: Final = {**ALIGNMENT_ITEMS, **SCREEN_ITEMS}
CLUSTER_ONE: Final = REF_ONE
CLUSTER_TWO: Final = REF_TWO
JUDGE_ONE: Final = "V-H1"
JUDGE_TWO: Final = "V-H2"
HIGH: Final = 0.9
MID: Final = 0.7
LOW: Final = 0.6
VOTES: Final[Mapping[str, Mapping[int, JudgeVote]]] = {
    NODE_GOOD: {
        1: JudgeVote(REF_ONE, "FULL", HIGH),
        2: JudgeVote(REF_ONE, "FULL", HIGH),
        3: JudgeVote(REF_ONE, "FULL", HIGH),
        4: JudgeVote(REF_TWO, "PARTIAL", 0.5),
        5: JudgeVote(REF_TWO, "PARTIAL", 0.5),
    },
    NODE_VAC: {
        1: JudgeVote(NONE, "WEAK", MID),
        2: JudgeVote(REF_TWO, "WEAK", LOW),
        3: JudgeVote(NONE, "WEAK", MID),
        4: JudgeVote(REF_TWO, "WEAK", LOW),
    },
}
VALIDITY: Final[Mapping[str, Mapping[str, Mapping[str, ValidityVote]]]] = {
    MUTANT_T: {
        JUDGE_ONE: {REF_ONE: ValidityVote("VIOLATED", HIGH), REF_TWO: ValidityVote("NOT_VIOLATED", 0.8)},
        JUDGE_TWO: {REF_ONE: ValidityVote("VIOLATED", MID), REF_TWO: ValidityVote("VIOLATED", LOW)},
    },
    MUTANT_P: {
        JUDGE_ONE: {REF_ONE: ValidityVote("UNDETERMINED", 0.0), REF_TWO: ValidityVote("NOT_VIOLATED", LOW)},
    },
}


def _decision(cell: CellData, unit: str) -> Decision:
    return next(decision for decision in cell.decisions if decision.unit == unit)


@pytest.fixture(scope="module")
def jh5() -> dict[Kind, CellData]:
    return host_vote_cells(SourceId.JH5, 5, ITEMS, truth(consensus=frozenset({NODE_GOOD}), alignment_votes=VOTES))


@pytest.fixture(scope="module")
def jh3() -> dict[Kind, CellData]:
    return host_vote_cells(SourceId.JH3, 3, ITEMS, truth(alignment_votes=VOTES))


def test_a_test_without_any_vote_is_counted_in_k1_and_k3_only(jh5: dict[Kind, CellData]) -> None:
    assert [jh5[kind].units_without_votes for kind in (Kind.K1, Kind.K2, Kind.K3)] == [1, 0, 1]
    assert [jh5[kind].units for kind in (Kind.K1, Kind.K2, Kind.K3)] == [3, 2, 3]
    assert jh5[Kind.K1].missing_share == pytest.approx(1 / 3)


def test_the_cells_name_the_host_and_the_vote_source(jh5: dict[Kind, CellData]) -> None:
    cell = jh5[Kind.K1]
    assert (cell.source, cell.model, cell.confidence_source) == (SourceId.JH5, HOST_MODEL, "host-votes(5)")
    assert cell.unit_clusters == (CLUSTER_ONE, CLUSTER_TWO, CLUSTER_TWO)
    assert [d.unit for d in cell.decisions] == [NODE_GOOD, NODE_VAC]


def test_the_confidence_is_the_modal_share_over_the_vote_count(jh5: dict[Kind, CellData]) -> None:
    good, vacuous = _decision(jh5[Kind.K1], NODE_GOOD), _decision(jh5[Kind.K1], NODE_VAC)
    assert good.confidence == pytest.approx(3 / 5)
    assert vacuous.confidence == pytest.approx(2 / 5)
    assert (good.correct, vacuous.correct) == (True, True)
    assert good.consensus
    assert not vacuous.consensus


def test_a_modal_tie_goes_to_the_higher_mean_confidence(jh5: dict[Kind, CellData]) -> None:
    assert _decision(jh5[Kind.K3], NODE_VAC).predicted_positive is False
    assert _decision(jh5[Kind.K1], NODE_VAC).correct


def test_k2_and_k3_read_the_modal_coverage(jh5: dict[Kind, CellData]) -> None:
    good2, vacuous2 = _decision(jh5[Kind.K2], NODE_GOOD), _decision(jh5[Kind.K2], NODE_VAC)
    assert (good2.predicted_positive, good2.truth_positive, good2.correct) == (True, True, True)
    assert good2.event_probability == pytest.approx(0.6)
    assert (vacuous2.predicted_positive, vacuous2.truth_positive) == (False, False)
    assert vacuous2.event_probability == pytest.approx(0.6)
    good3 = _decision(jh5[Kind.K3], NODE_GOOD)
    assert (good3.predicted_positive, good3.truth_positive, good3.correct) == (True, True, True)
    assert good3.confidence == pytest.approx(0.6)


def test_three_votes_use_the_first_three_repetitions(jh3: dict[Kind, CellData]) -> None:
    good, vacuous = _decision(jh3[Kind.K1], NODE_GOOD), _decision(jh3[Kind.K1], NODE_VAC)
    assert good.confidence == 1.0
    assert vacuous.confidence == pytest.approx(2 / 3)
    assert jh3[Kind.K1].confidence_source == "host-votes(3)"


def test_validity_pairs_with_no_modal_answer_have_no_prediction() -> None:
    cell = validity_cell(ITEMS, truth(validity_votes=VALIDITY))
    split = _decision(cell, f"{MUTANT_T}|{REF_TWO}")
    assert not split.has_prediction
    assert split.confidence == 0.0
    assert split.event_probability == pytest.approx(0.5)
    assert split.truth_positive is False
    abstained = _decision(cell, f"{MUTANT_P}|{REF_ONE}")
    assert not abstained.has_prediction
    assert abstained.event_probability == 0.0
    assert len(cell.predicted) == 2


def test_validity_agreement_gives_share_one_and_the_weakest_stated_confidence() -> None:
    cell = validity_cell(ITEMS, truth(validity_votes=VALIDITY))
    agreed = _decision(cell, f"{MUTANT_T}|{REF_ONE}")
    assert (agreed.confidence, agreed.event_probability) == (1.0, 1.0)
    assert (agreed.predicted_positive, agreed.truth_positive, agreed.correct) == (True, True, True)
    assert agreed.alt_confidence == MID
    assert agreed.alt_event_probability == MID
    assert agreed.cluster == "T:gen-7"
    assert agreed.tags["arm"] == "T"


def test_a_single_answering_vote_has_share_one_half_and_no_unanimous_confidence() -> None:
    cell = validity_cell(ITEMS, truth(validity_votes=VALIDITY))
    single = _decision(cell, f"{MUTANT_P}|{REF_TWO}")
    assert (single.confidence, single.predicted_positive, single.correct) == (0.5, False, True)
    assert single.alt_confidence is None
    assert single.cluster == "P:3"


def test_mutants_with_no_record_are_counted_and_mutants_without_oracle_are_left_out() -> None:
    cell = validity_cell(ITEMS, truth(validity_votes=VALIDITY))
    assert cell.units == 8
    assert cell.units_without_votes == 4
    assert len(cell.decisions) == 4
    assert all(not d.unit.startswith(MUTANT_R) for d in cell.decisions)
    assert all(not d.unit.startswith("mN") for d in cell.decisions)
    assert cell.missing_share == pytest.approx(4 / 8)
    assert (cell.source, cell.model, cell.confidence_source) == (SourceId.VH2, HOST_MODEL, "host-votes(2)")


def test_source_two_yields_seven_cells() -> None:
    cells = source2_cells(ITEMS, truth(alignment_votes=VOTES, validity_votes=VALIDITY))
    keys = sorted(cell.key for cell in cells)
    assert len(keys) == 7
    assert "K4|VH2|host" in keys
    assert {cell.source for cell in cells} == {SourceId.JH5, SourceId.JH3, SourceId.VH2}
