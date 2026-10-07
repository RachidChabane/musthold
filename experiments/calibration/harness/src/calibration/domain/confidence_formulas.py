"""The confidence each server derives from its returned probabilities (§5.4), recomputed for the canary and `analyze`."""

from __future__ import annotations

import math
from collections.abc import Sequence
from typing import Final

from calibration.domain.constants import CONFIDENCE_TOLERANCE
from calibration.domain.models import Flavour
from calibration.domain.server_wire import CHOICE, SCORE

_CERTAIN: Final = 1.0


def _normalized(probabilities: Sequence[float]) -> list[float]:
    total = math.fsum(probabilities)
    count = len(probabilities)
    if total == 0:
        return [1.0 / count] * count
    return [value / total for value in probabilities]


def entropy_confidence(probabilities: Sequence[float]) -> float:
    """Ollama, choice and score: 1 - H(p) / ln K, with H in nats."""
    count = len(probabilities)
    if count == 1:
        return _CERTAIN
    shares = _normalized(probabilities)
    entropy = -math.fsum(share * math.log(share) for share in shares if share > 0)
    return _CERTAIN - entropy / math.log(count)


def kev_choice_confidence(probabilities: Sequence[float]) -> float:
    """Kev, choice: (p_max - 1/K) / (1 - 1/K)."""
    count = len(probabilities)
    if count == 1:
        return _CERTAIN
    return (max(_normalized(probabilities)) - 1.0 / count) / (_CERTAIN - 1.0 / count)


def kev_score_confidence(probabilities: Sequence[float]) -> float:
    """Kev, score: max(0, 1 - sum_i p_i |i - mode| / D), D the mean absolute deviation of the uniform distribution
    over the levels around its mean (L - 1) / 2, and mode the first most likely level."""
    levels = len(probabilities)
    if levels == 1:
        return _CERTAIN
    shares = _normalized(probabilities)
    mode = max(range(levels), key=lambda index: shares[index])
    deviation = math.fsum(abs(index - (levels - 1) / 2) for index in range(levels)) / levels
    spread = math.fsum(share * abs(index - mode) for index, share in enumerate(shares))
    return max(0.0, _CERTAIN - spread / deviation)


def expected_confidence(flavour: Flavour, question_type: str, probabilities: Sequence[float]) -> float:
    """The confidence the server of `flavour` states for a question of `question_type` with these probabilities."""
    if flavour is Flavour.OLLAMA:
        return entropy_confidence(probabilities)
    if question_type == CHOICE:
        return kev_choice_confidence(probabilities)
    if question_type == SCORE:
        return kev_score_confidence(probabilities)
    raise ValueError(f"unknown question type {question_type!r}")


def confidence_matches(stated: float, recomputed: float) -> bool:
    """Whether a stated confidence lies within the tolerance of §5.4 of the recomputed one."""
    return abs(stated - recomputed) <= CONFIDENCE_TOLERANCE
