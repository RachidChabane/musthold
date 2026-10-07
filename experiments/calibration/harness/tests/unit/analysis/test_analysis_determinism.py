"""Determinism of source (1): the comparison of pass 2 with pass 1 (PREREG section 8.6)."""

from __future__ import annotations

import math
from dataclasses import replace

import pytest

from calibration.analysis.determinism import answers_identical, determinism_of
from calibration.analysis.model import Decision
from calibration.domain.ports import DecodedAnswer
from support.metric_cases import CLUSTERS

QUESTION = "criterion"
OTHER = "coverage"
FLIP_LIMIT_UNITS = 100


def _answer(question: str, confidence: float | None, **probabilities: float) -> DecodedAnswer:
    return DecodedAnswer(question, "K-1", confidence, probabilities)


def _decision(unit: str, confidence: float, prediction: str = "x") -> Decision:
    return Decision(unit, CLUSTERS[0], confidence, True, prediction=prediction)


def test_identical_answers_share_confidence_and_probabilities() -> None:
    first = {QUESTION: _answer(QUESTION, 0.9, a=0.9, b=0.1)}
    assert answers_identical(first, {QUESTION: _answer(QUESTION, 0.9, a=0.9, b=0.1)}, (QUESTION,))


@pytest.mark.parametrize(
    "second",
    [
        _answer(QUESTION, 0.8, a=0.9, b=0.1),
        _answer(QUESTION, 0.9, a=0.8, b=0.2),
        _answer(QUESTION, None, a=0.9, b=0.1),
    ],
)
def test_a_changed_confidence_or_probability_is_not_identical(second: DecodedAnswer) -> None:
    first = {QUESTION: _answer(QUESTION, 0.9, a=0.9, b=0.1)}
    assert not answers_identical(first, {QUESTION: second}, (QUESTION,))


def test_only_the_asked_questions_are_compared() -> None:
    first = {QUESTION: _answer(QUESTION, 0.9), OTHER: _answer(OTHER, 0.1)}
    second = {QUESTION: _answer(QUESTION, 0.9), OTHER: _answer(OTHER, 0.2)}
    assert answers_identical(first, second, (QUESTION,))
    assert not answers_identical(first, second, (QUESTION, OTHER))


def test_a_question_missing_from_one_pass_is_not_identical() -> None:
    first = {QUESTION: _answer(QUESTION, 0.9)}
    assert not answers_identical(first, {}, (QUESTION,))
    assert not answers_identical({}, first, (QUESTION,))


def test_the_spread_flips_and_identity_over_the_shared_units() -> None:
    first = {u: _decision(u, c) for u, c in (("a", 0.5), ("b", 0.5), ("c", 0.5), ("d", 0.5))}
    second = {
        "a": _decision("a", 0.5),
        "b": _decision("b", 0.6),
        "c": _decision("c", 0.7, prediction="y"),
        "d": _decision("d", 0.5),
        "extra": _decision("extra", 0.1),
    }
    figures = determinism_of(first, second, {"a": True, "b": False, "c": False, "d": True})
    assert figures.pairs == 4
    assert figures.identical_share == pytest.approx(0.5)
    assert figures.max_delta == pytest.approx(0.2)
    assert figures.p99_delta == pytest.approx(0.1 + 0.97 * 0.1)
    assert figures.flips.point == pytest.approx(0.25)
    assert figures.unstable


def test_a_unit_without_an_identity_flag_counts_as_changed() -> None:
    first = {"a": _decision("a", 0.5)}
    assert determinism_of(first, first, {}).identical_share == 0.0


def test_a_flip_rate_of_exactly_one_percent_is_stable() -> None:
    units = [f"u{index}" for index in range(FLIP_LIMIT_UNITS)]
    first = {unit: _decision(unit, 0.5) for unit in units}
    second = {unit: _decision(unit, 0.5) for unit in units}
    second[units[0]] = replace(second[units[0]], prediction="y")
    assert not determinism_of(first, second, {}).unstable
    second[units[1]] = replace(second[units[1]], prediction="y")
    assert determinism_of(first, second, {}).unstable


def test_no_shared_unit_gives_undefined_figures_and_a_stable_cell() -> None:
    figures = determinism_of({"a": _decision("a", 0.5)}, {"b": _decision("b", 0.5)}, {})
    assert figures.pairs == 0
    assert math.isnan(figures.identical_share)
    assert not figures.unstable
    record = figures.to_record()
    assert record["identical_share"] is None
    assert record["max_abs_delta_confidence"] is None
    assert record["p99_abs_delta_confidence"] is None


def test_the_record_carries_the_figures() -> None:
    first = {"a": _decision("a", 0.5)}
    record = determinism_of(first, first, {"a": True}).to_record()
    assert record["pairs"] == 1
    assert record["identical_share"] == 1.0
    assert record["max_abs_delta_confidence"] == 0.0
    assert record["unstable"] is False
