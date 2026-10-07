"""Source (2) vote aggregation: the alignment module's modal rule and the V-H agreement rule."""

from __future__ import annotations

from typing import Final

import pytest

from calibration.analysis.votes import (
    ANSWER_CONSISTENT,
    ANSWER_VIOLATES,
    JudgeVote,
    ValidityVote,
    aggregate_judge_votes,
    aggregate_validity_votes,
)

REF_ONE: Final = "PARK-1~1"
REF_TWO: Final = "PARK-2~1"
TOTAL_VOTES: Final = 5
LOW: Final = 0.5
HIGH: Final = 0.9
SHARE_PRECISION: Final = 1e-12


def _vote(ref: str, coverage: str = "FULL", confidence: float = HIGH) -> JudgeVote:
    return JudgeVote(ref, coverage, confidence)


def test_the_modal_criterion_has_the_highest_count() -> None:
    votes = [_vote(REF_ONE), _vote(REF_TWO), _vote(REF_TWO)]
    aggregate = aggregate_judge_votes(votes, TOTAL_VOTES)
    assert aggregate is not None
    assert aggregate.modal_ref == REF_TWO
    assert aggregate.share == pytest.approx(2 / TOTAL_VOTES, abs=SHARE_PRECISION)


def test_a_count_tie_goes_to_the_higher_mean_confidence() -> None:
    votes = [_vote(REF_ONE, confidence=LOW), _vote(REF_TWO, confidence=HIGH)]
    aggregate = aggregate_judge_votes(votes, 2)
    assert aggregate is not None
    assert aggregate.modal_ref == REF_TWO
    assert aggregate.mean_confidence == HIGH


def test_a_full_tie_goes_to_the_lexicographically_smaller_reference() -> None:
    votes = [_vote(REF_TWO), _vote(REF_ONE)]
    aggregate = aggregate_judge_votes(votes, 2)
    assert aggregate is not None
    assert aggregate.modal_ref == REF_ONE


def test_a_missing_vote_counts_against_agreement() -> None:
    votes = [_vote(REF_ONE)] * 3
    aggregate = aggregate_judge_votes(votes, TOTAL_VOTES)
    assert aggregate is not None
    assert aggregate.share == pytest.approx(3 / TOTAL_VOTES, abs=SHARE_PRECISION)


def test_the_modal_coverage_ties_go_to_the_smaller_level() -> None:
    votes = [_vote(REF_ONE, "PARTIAL"), _vote(REF_ONE, "FULL")]
    aggregate = aggregate_judge_votes(votes, 2)
    assert aggregate is not None
    assert aggregate.modal_coverage == "FULL"
    assert aggregate.adequate


def test_the_modal_coverage_counts_only_votes_on_the_modal_criterion() -> None:
    votes = [_vote(REF_ONE, "WEAK"), _vote(REF_ONE, "WEAK"), _vote(REF_TWO, "FULL")]
    aggregate = aggregate_judge_votes(votes, 3)
    assert aggregate is not None
    assert aggregate.modal_coverage == "WEAK"
    assert not aggregate.adequate


def test_no_vote_gives_no_aggregate() -> None:
    assert aggregate_judge_votes([], TOTAL_VOTES) is None


def test_more_votes_than_the_total_gives_no_aggregate() -> None:
    assert aggregate_judge_votes([_vote(REF_ONE)] * 3, 2) is None


def _verdict(name: str, confidence: float = HIGH) -> ValidityVote:
    return ValidityVote(name, confidence)


def test_two_agreeing_violated_votes_have_share_one_and_the_weakest_confidence() -> None:
    aggregate = aggregate_validity_votes([_verdict("VIOLATED", HIGH), _verdict("VIOLATED", LOW)])
    assert aggregate.modal_answer == ANSWER_VIOLATES
    assert aggregate.share == 1.0
    assert aggregate.violation_probability == 1.0
    assert aggregate.unanimous_confidence == LOW
    assert aggregate.has_prediction


def test_a_single_vote_has_share_one_half_and_no_unanimous_confidence() -> None:
    aggregate = aggregate_validity_votes([_verdict("NOT_VIOLATED")])
    assert aggregate.modal_answer == ANSWER_CONSISTENT
    assert aggregate.share == 0.5
    assert aggregate.violation_probability == 0.0
    assert aggregate.unanimous_confidence is None


def test_a_split_leaves_no_modal_answer_and_a_half_probability() -> None:
    aggregate = aggregate_validity_votes([_verdict("VIOLATED"), _verdict("NOT_VIOLATED")])
    assert aggregate.modal_answer is None
    assert not aggregate.has_prediction
    assert aggregate.violation_probability == 0.5
    assert aggregate.voters == 2


def test_an_abstention_leaves_the_other_vote_alone() -> None:
    aggregate = aggregate_validity_votes([_verdict("VIOLATED"), _verdict("UNDETERMINED")])
    assert aggregate.modal_answer == ANSWER_VIOLATES
    assert aggregate.share == 0.5
    assert aggregate.voters == 1


def test_only_abstentions_leave_no_prediction_and_zero_probability() -> None:
    aggregate = aggregate_validity_votes([_verdict("UNDETERMINED"), _verdict("UNDETERMINED")])
    assert aggregate.modal_answer is None
    assert aggregate.violation_probability == 0.0
    assert aggregate.voters == 0


def test_no_votes_leave_no_prediction() -> None:
    aggregate = aggregate_validity_votes([])
    assert aggregate.modal_answer is None
    assert aggregate.share == 0.0
