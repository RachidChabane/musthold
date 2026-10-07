"""Source (2): agreement among host votes (§7.2). Pure functions of the recorded votes."""

from __future__ import annotations

import math
from collections import Counter, defaultdict
from collections.abc import Sequence
from dataclasses import dataclass
from typing import Final

NONE_ANSWER: Final = "NONE"
ADEQUATE_LEVELS: Final = ("FULL", "PARTIAL")
VERDICT_VIOLATED: Final = "VIOLATED"
VERDICT_NOT_VIOLATED: Final = "NOT_VIOLATED"
ANSWER_VIOLATES: Final = "VIOLATES"
ANSWER_CONSISTENT: Final = "CONSISTENT"
VALIDITY_VOTE_COUNT: Final = 2


@dataclass(frozen=True)
class JudgeVote:
    """One answered J-H repetition: the criterion it names (a reference or NONE), its coverage and stated confidence."""

    ref: str
    coverage: str
    confidence: float


@dataclass(frozen=True)
class JudgeAggregate:
    modal_ref: str
    share: float
    modal_coverage: str
    mean_confidence: float

    @property
    def adequate(self) -> bool:
        return self.modal_coverage in ADEQUATE_LEVELS


def _mean(values: Sequence[float]) -> float:
    return math.fsum(values) / len(values)


def aggregate_judge_votes(votes: Sequence[JudgeVote], total: int) -> JudgeAggregate | None:
    """The alignment module's `aggregate.Aggregate`: the modal criterion has the highest count, ties going to the
    higher mean stated confidence and then to the lexicographically smaller reference; the share is the modal count
    over `total`, so a missing vote counts against agreement; the modal coverage is the commonest among the votes on
    the modal criterion, ties going to the lexicographically smaller level. None when no vote was cast."""
    if not votes or total < len(votes):
        return None
    grouped: dict[str, list[JudgeVote]] = defaultdict(list)
    for vote in votes:
        grouped[vote.ref].append(vote)
    modal = min(grouped, key=lambda ref: (-len(grouped[ref]), -_mean([v.confidence for v in grouped[ref]]), ref))
    on_modal = grouped[modal]
    coverages = Counter(vote.coverage for vote in on_modal)
    coverage = min(coverages, key=lambda level: (-coverages[level], level))
    return JudgeAggregate(modal, len(on_modal) / total, coverage, _mean([v.confidence for v in on_modal]))


@dataclass(frozen=True)
class ValidityAggregate:
    """The V-H votes on one pair (m, c): the modal answer when there is one, and the agreement measures."""

    modal_answer: str | None
    share: float
    violation_probability: float
    voters: int
    unanimous_confidence: float | None

    @property
    def has_prediction(self) -> bool:
        return self.modal_answer is not None


@dataclass(frozen=True)
class ValidityVote:
    """A V-H verdict for one criterion: VIOLATED, NOT_VIOLATED or UNDETERMINED, with its stated confidence."""

    verdict: str
    confidence: float

    @property
    def answer(self) -> str | None:
        if self.verdict == VERDICT_VIOLATED:
            return ANSWER_VIOLATES
        if self.verdict == VERDICT_NOT_VIOLATED:
            return ANSWER_CONSISTENT
        return None


def aggregate_validity_votes(votes: Sequence[ValidityVote]) -> ValidityAggregate:
    """§7.2 V-H: the modal answer is whichever of VIOLATES and CONSISTENT has more votes (a tie or no answer leaves
    none); the share is the votes on it over 2; P_votes(VIOLATES) is the mean of 1 or 0 over the answering votes (0
    when none answered); when both votes agree the weakest stated confidence is kept for the secondary mapping."""
    answers = [(vote.answer, vote.confidence) for vote in votes if vote.answer is not None]
    violates = sum(1 for answer, _ in answers if answer == ANSWER_VIOLATES)
    consistent = len(answers) - violates
    probability = violates / len(answers) if answers else 0.0
    if violates == consistent:
        return ValidityAggregate(None, 0.0, probability, len(answers), None)
    modal = ANSWER_VIOLATES if violates > consistent else ANSWER_CONSISTENT
    on_modal = [confidence for answer, confidence in answers if answer == modal]
    unanimous = min(on_modal) if len(on_modal) == VALIDITY_VOTE_COUNT else None
    return ValidityAggregate(modal, len(on_modal) / VALIDITY_VOTE_COUNT, probability, len(answers), unanimous)
