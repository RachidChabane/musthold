from __future__ import annotations

import json
import math
from typing import ClassVar

import pytest

from calibration.domain.confidence_formulas import (
    confidence_matches,
    entropy_confidence,
    expected_confidence,
    kev_choice_confidence,
    kev_score_confidence,
)
from calibration.domain.constants import CONFIDENCE_TOLERANCE
from calibration.domain.models import Flavour
from calibration.domain.server_wire import CHOICE, SCORE
from fixtures.wire_data import RECORDED_KEV_CHOICE_REPLY, RECORDED_OLLAMA_CHOICE_REPLY

COLOR_QUESTION = "color"
EXACT_ZERO = 1e-12


def _recorded_choice(reply: str) -> tuple[list[float], float]:
    answer = json.loads(reply)["answers"][COLOR_QUESTION]
    return list(answer["probabilities"].values()), answer["confidence"]


class TestRecordedReplies:
    def test_the_recorded_ollama_reply_reproduces_its_confidence(self) -> None:
        probabilities, stated = _recorded_choice(RECORDED_OLLAMA_CHOICE_REPLY)
        recomputed = expected_confidence(Flavour.OLLAMA, CHOICE, probabilities)
        assert recomputed == pytest.approx(stated, abs=CONFIDENCE_TOLERANCE)
        assert confidence_matches(stated, recomputed)

    def test_the_recorded_kev_reply_reproduces_its_confidence(self) -> None:
        probabilities, stated = _recorded_choice(RECORDED_KEV_CHOICE_REPLY)
        recomputed = expected_confidence(Flavour.KEV, CHOICE, probabilities)
        assert recomputed == pytest.approx(stated, abs=CONFIDENCE_TOLERANCE)
        assert confidence_matches(stated, recomputed)


class TestEntropyConfidence:
    def test_uniform_probabilities_give_zero(self) -> None:
        assert entropy_confidence([0.25] * 4) == pytest.approx(0.0, abs=EXACT_ZERO)

    def test_a_point_mass_gives_one(self) -> None:
        assert entropy_confidence([0.0, 1.0, 0.0]) == pytest.approx(1.0)

    def test_a_single_level_is_certain(self) -> None:
        assert entropy_confidence([1.0]) == 1.0

    def test_the_formula_is_one_minus_entropy_over_log_k(self) -> None:
        shares = [0.5, 0.25, 0.25]
        entropy = -sum(share * math.log(share) for share in shares)
        assert entropy_confidence(shares) == pytest.approx(1 - entropy / math.log(3))

    def test_unnormalized_probabilities_are_normalized(self) -> None:
        assert entropy_confidence([2.0, 1.0, 1.0]) == pytest.approx(entropy_confidence([0.5, 0.25, 0.25]))

    def test_all_zero_probabilities_read_as_uniform(self) -> None:
        assert entropy_confidence([0.0, 0.0, 0.0]) == pytest.approx(0.0, abs=EXACT_ZERO)


class TestKevChoiceConfidence:
    def test_uniform_probabilities_give_zero(self) -> None:
        assert kev_choice_confidence([0.2] * 5) == pytest.approx(0.0, abs=EXACT_ZERO)

    def test_a_point_mass_gives_one(self) -> None:
        assert kev_choice_confidence([0.0, 1.0, 0.0]) == pytest.approx(1.0)

    def test_the_formula_is_the_scaled_maximum(self) -> None:
        assert kev_choice_confidence([0.6, 0.3, 0.1]) == pytest.approx((0.6 - 1 / 3) / (1 - 1 / 3))

    def test_a_single_option_is_certain(self) -> None:
        assert kev_choice_confidence([1.0]) == 1.0


class TestKevScoreConfidence:
    def test_uniform_probabilities_give_zero(self) -> None:
        assert kev_score_confidence([0.2] * 5) == pytest.approx(0.0, abs=EXACT_ZERO)

    def test_a_point_mass_at_any_level_gives_one(self) -> None:
        for mode in range(4):
            probabilities = [1.0 if index == mode else 0.0 for index in range(4)]
            assert kev_score_confidence(probabilities) == pytest.approx(1.0)

    def test_the_spread_is_scaled_by_the_uniform_mean_absolute_deviation(self) -> None:
        probabilities = [0.0, 0.5, 0.5]
        uniform_deviation = (1 + 0 + 1) / 3
        assert kev_score_confidence(probabilities) == pytest.approx(1 - 0.5 * 1 / uniform_deviation)

    def test_the_first_most_likely_level_is_the_mode(self) -> None:
        assert kev_score_confidence([0.5, 0.0, 0.5]) == 0.0

    def test_the_confidence_never_goes_below_zero(self) -> None:
        assert kev_score_confidence([0.4, 0.0, 0.0, 0.0, 0.6]) >= 0.0

    def test_a_single_level_is_certain(self) -> None:
        assert kev_score_confidence([1.0]) == 1.0


class TestDispatch:
    PROBABILITIES: ClassVar[list[float]] = [0.6, 0.3, 0.1]

    def test_ollama_uses_the_entropy_formula_for_both_types(self) -> None:
        expected = entropy_confidence(self.PROBABILITIES)
        assert expected_confidence(Flavour.OLLAMA, CHOICE, self.PROBABILITIES) == expected
        assert expected_confidence(Flavour.OLLAMA, SCORE, self.PROBABILITIES) == expected

    def test_kev_uses_a_formula_per_type(self) -> None:
        assert expected_confidence(Flavour.KEV, CHOICE, self.PROBABILITIES) == kev_choice_confidence(self.PROBABILITIES)
        assert expected_confidence(Flavour.KEV, SCORE, self.PROBABILITIES) == kev_score_confidence(self.PROBABILITIES)

    def test_an_unknown_question_type_is_refused_for_kev(self) -> None:
        with pytest.raises(ValueError, match="unknown question type"):
            expected_confidence(Flavour.KEV, "noul", self.PROBABILITIES)


class TestTolerance:
    def test_a_difference_within_the_tolerance_matches(self) -> None:
        assert confidence_matches(0.5, 0.5 + CONFIDENCE_TOLERANCE / 2)

    def test_a_difference_beyond_the_tolerance_does_not(self) -> None:
        assert not confidence_matches(0.5, 0.5 + CONFIDENCE_TOLERANCE * 2)
