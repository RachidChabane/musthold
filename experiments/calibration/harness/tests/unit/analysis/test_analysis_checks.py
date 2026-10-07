"""Source (1) checks: confidence and formula checks, the screen regression check, the key check (PREREG 5.4, 7.4)."""

from __future__ import annotations

import math
from collections.abc import Mapping, Sequence
from typing import Final

import pytest

from calibration.analysis.checks import (
    CONFIDENCE_QUESTIONS,
    confidence_checks,
    formula_checks,
    key_check,
    screen_regression,
)
from calibration.analysis.model import ALIGNMENT_KINDS, Kind
from calibration.analysis.pipeline import AnalysisOutput, AnalyzeUseCase
from calibration.analysis.replies import ReplyOutcome
from calibration.domain.deviations import DeviationKind
from calibration.domain.items import Item
from calibration.domain.jsonread import json_object
from calibration.domain.models import MODEL_BY_ID
from calibration.domain.ports import (
    DecodedAnswer,
    DecodeOutcome,
    DecodeRequest,
    EndpointDigest,
    ProductDefaults,
    ScreenReading,
    ScreenReadRequest,
    ThresholdCheck,
)
from support.check_cases import (
    COVERAGE_QUESTION,
    CRITERION_QUESTION,
    LEVELS,
    SCREEN_QUESTION,
    alignment_item,
    cell,
    choice_view,
    decision,
    outcome,
    score_view,
    screen_item,
)
from support.fake_product_reader import FakeProductReader
from support.synthetic_world import build_world
from support.world_analysis import analysis_deps, analysis_settings, snapshot_matrix

CHOICE_PROBABILITIES: Final = (0.7, 0.2, 0.1)
CHOICE_KEYS: Final = ("K-0001", "K-0002", "NONE")
SCORE_PROBABILITIES: Final = (0.1, 0.2, 0.6, 0.1)
TAMPER: Final = 0.05
SPEARMAN_RESAMPLES: Final = 20
PERTURBATION: Final = 0.01
REGRESSION_RESAMPLES: Final = 20
OLLAMA: Final = MODEL_BY_ID["M1"]
KEV: Final = MODEL_BY_ID["M5"]
REQUEST_ID: Final = "A|t"


def _entropy_confidence(probabilities: Sequence[float]) -> float:
    entropy = -sum(p * math.log(p) for p in probabilities)
    return 1.0 - entropy / math.log(len(probabilities))


def _kev_choice_confidence(probabilities: Sequence[float]) -> float:
    count = len(probabilities)
    return (max(probabilities) - 1 / count) / (1 - 1 / count)


KEV_SCORE_CONFIDENCE: Final = 0.5


def _alignment_outcome(
    criterion_confidence: float | None,
    coverage_confidence: float | None,
    *,
    score_keys: Sequence[str],
) -> ReplyOutcome:
    choice = choice_view(
        CRITERION_QUESTION, "K-0001", criterion_confidence, dict(zip(CHOICE_KEYS, CHOICE_PROBABILITIES, strict=True))
    )
    score = score_view(
        COVERAGE_QUESTION, 2.0, coverage_confidence, dict(zip(score_keys, SCORE_PROBABILITIES, strict=True))
    )
    return outcome(REQUEST_ID, [choice, score])


INDEX_KEYS: Final = tuple(str(index) for index in range(len(LEVELS)))


def _outcomes(outcomes: ReplyOutcome) -> dict[str, ReplyOutcome]:
    return {outcomes.request_id: outcomes}


def _items() -> dict[str, Item]:
    return {REQUEST_ID: alignment_item(REQUEST_ID)}


class TestConfidenceChecks:
    def test_spearman_is_one_when_the_returned_probability_orders_like_the_confidence(self) -> None:
        decisions = [decision(f"u{i}", "c", 0.1 * i, True, alt=0.2 * i) for i in range(1, 6)]
        checks = confidence_checks({kind: cell(kind, decisions) for kind in ALIGNMENT_KINDS}, {})
        assert all(checks.spearman[kind] == pytest.approx(1.0) for kind in ALIGNMENT_KINDS)

    def test_spearman_is_minus_one_for_reversed_orders(self) -> None:
        decisions = [decision(f"u{i}", "c", 0.1 * i, True, alt=1.0 - 0.1 * i) for i in range(1, 6)]
        checks = confidence_checks({Kind.K1: cell(Kind.K1, decisions)}, {})
        assert checks.spearman[Kind.K1] == pytest.approx(-1.0)

    def test_decisions_without_an_alternative_measure_are_left_out(self) -> None:
        checks = confidence_checks({Kind.K1: cell(Kind.K1, [decision("u", "c", 0.5, True)])}, {})
        assert math.isnan(checks.spearman[Kind.K1])

    def test_answers_without_a_confidence_are_counted_per_question(self) -> None:
        silent = DecodedAnswer(CRITERION_QUESTION, "K-0001", None, {})
        stated = DecodedAnswer(COVERAGE_QUESTION, "FULL", 0.5, {})
        answered = outcome(REQUEST_ID, [], {CRITERION_QUESTION: silent, COVERAGE_QUESTION: stated})
        checks = confidence_checks({}, _outcomes(answered))
        assert checks.without_confidence == 1
        assert set(CONFIDENCE_QUESTIONS) == {CRITERION_QUESTION, COVERAGE_QUESTION}


class TestFormulaChecks:
    def test_matching_ollama_answers_produce_no_finding(self) -> None:
        entropy = _entropy_confidence(CHOICE_PROBABILITIES)
        score = _entropy_confidence(SCORE_PROBABILITIES)
        answered = _alignment_outcome(entropy, score, score_keys=INDEX_KEYS)
        checked, findings = formula_checks(OLLAMA, _outcomes(answered), _items())
        assert (checked, findings) == (2, [])

    def test_a_tampered_ollama_confidence_is_a_finding(self) -> None:
        entropy = _entropy_confidence(CHOICE_PROBABILITIES)
        answered = _alignment_outcome(entropy + TAMPER, _entropy_confidence(SCORE_PROBABILITIES), score_keys=INDEX_KEYS)
        _, findings = formula_checks(OLLAMA, _outcomes(answered), _items())
        assert [(f.request_id, f.question_id) for f in findings] == [(REQUEST_ID, CRITERION_QUESTION)]
        assert findings[0].stated == pytest.approx(entropy + TAMPER)
        assert findings[0].recomputed == pytest.approx(entropy)

    def test_a_tampered_kev_choice_confidence_is_a_finding(self) -> None:
        choice = _kev_choice_confidence(CHOICE_PROBABILITIES)
        answered = _alignment_outcome(choice - TAMPER, KEV_SCORE_CONFIDENCE, score_keys=INDEX_KEYS)
        _, findings = formula_checks(KEV, _outcomes(answered), _items())
        assert [f.question_id for f in findings] == [CRITERION_QUESTION]
        assert findings[0].recomputed == pytest.approx(choice)

    def test_a_tampered_kev_score_confidence_is_a_finding(self) -> None:
        answered = _alignment_outcome(
            _kev_choice_confidence(CHOICE_PROBABILITIES), KEV_SCORE_CONFIDENCE + TAMPER, score_keys=INDEX_KEYS
        )
        _, findings = formula_checks(KEV, _outcomes(answered), _items())
        assert [f.question_id for f in findings] == [COVERAGE_QUESTION]
        assert findings[0].recomputed == pytest.approx(KEV_SCORE_CONFIDENCE)

    def test_a_score_keyed_by_level_name_is_read_in_level_order(self) -> None:
        answered = _alignment_outcome(
            _kev_choice_confidence(CHOICE_PROBABILITIES), KEV_SCORE_CONFIDENCE, score_keys=LEVELS
        )
        checked, findings = formula_checks(KEV, _outcomes(answered), _items())
        assert (checked, findings) == (2, [])

    def test_an_absent_confidence_is_a_finding(self) -> None:
        answered = _alignment_outcome(None, KEV_SCORE_CONFIDENCE, score_keys=INDEX_KEYS)
        _, findings = formula_checks(KEV, _outcomes(answered), _items())
        assert [f.stated for f in findings] == [None]

    def test_a_confidence_inside_the_tolerance_is_no_finding(self) -> None:
        within = _kev_choice_confidence(CHOICE_PROBABILITIES) + 0.0005
        answered = _alignment_outcome(within, KEV_SCORE_CONFIDENCE, score_keys=INDEX_KEYS)
        assert formula_checks(KEV, _outcomes(answered), _items())[1] == []


def _screen_outcome(request_id: str, returned: float | None) -> ReplyOutcome:
    probabilities = {"CONSISTENT": 1.0} if returned is None else {"VIOLATES": returned, "CONSISTENT": 1.0 - returned}
    return outcome(request_id, [choice_view(SCREEN_QUESTION, "VIOLATES", None, probabilities)])


def _reading(request_id: str, product: float, *, error: str | None = None, has: bool = True) -> ScreenReading:
    return ScreenReading(f"{request_id}#{SCREEN_QUESTION}", error, "VIOLATES", has, product, product)


def _regression(pairs: Mapping[str, tuple[float | None, ScreenReading | None]]) -> tuple[object, ...]:
    outcomes = {request_id: _screen_outcome(request_id, returned) for request_id, (returned, _) in pairs.items()}
    items = {request_id: screen_item(request_id) for request_id in pairs}
    readings = {f"{request_id}#{SCREEN_QUESTION}": reading for request_id, (_, reading) in pairs.items() if reading}
    result = screen_regression(outcomes, items, readings)
    return (result.answers, result.without_probability, result.mismatches, result.monotone, result.failed)


class TestScreenRegression:
    def test_equal_and_ordered_answers_pass(self) -> None:
        pairs = {"M|a": (0.2, _reading("M|a", 0.2)), "M|b": (0.6, _reading("M|b", 0.6))}
        assert _regression(pairs) == (2, 0, (), True, False)

    def test_a_mismatch_beyond_the_tolerance_is_listed_per_answer(self) -> None:
        pairs = {"M|a": (0.2, _reading("M|a", 0.2 + 1e-6)), "M|b": (0.6, _reading("M|b", 0.6))}
        assert _regression(pairs) == (2, 0, ("M|a#q1",), True, True)

    def test_a_difference_inside_the_tolerance_passes(self) -> None:
        assert _regression({"M|a": (0.2, _reading("M|a", 0.2 + 1e-12))})[2] == ()

    def test_a_product_probability_that_decreases_in_the_returned_one_is_not_monotone(self) -> None:
        pairs = {"M|a": (0.2, _reading("M|a", 0.5)), "M|b": (0.4, _reading("M|b", 0.3))}
        _, _, _, monotone, failed = _regression(pairs)
        assert (monotone, failed) == (False, True)

    def test_answers_without_a_violates_probability_are_counted(self) -> None:
        pairs = {"M|a": (None, _reading("M|a", 1.0, has=False)), "M|b": (0.4, _reading("M|b", 0.4, has=False))}
        assert _regression(pairs) == (2, 2, (), True, False)

    def test_reading_errors_are_skipped(self) -> None:
        pairs = {"M|a": (0.2, _reading("M|a", 0.9, error="unreadable")), "M|b": (0.3, None)}
        assert _regression(pairs) == (0, 0, (), True, False)


class TestKeyCheck:
    def test_distinct_sources_have_no_collision(self) -> None:
        assert key_check({"M1": "system-one:a@1", "M2": "system-one:b@1"}).collisions == ()

    def test_two_models_sharing_a_source_collide(self) -> None:
        sources = {"M1": "s", "M2": "s", "M3": "t", "M4": "s"}
        assert key_check(sources).collisions == (("M1", "M2", "M4"),)


class PerturbingReader:
    """Delegates to the fake reader and shifts P(VIOLATES) of the first non-empty screen read, which is the first
    model's pass-1 read."""

    def __init__(self) -> None:
        self._inner = FakeProductReader()
        self._calls = 0

    def decode(self, requests: Sequence[DecodeRequest]) -> list[DecodeOutcome]:
        return self._inner.decode(requests)

    def read_screen(self, requests: Sequence[ScreenReadRequest]) -> list[ScreenReading]:
        readings = self._inner.read_screen(requests)
        self._calls += 1
        if self._calls > 1:
            return readings
        return [
            ScreenReading(
                r.key, r.error, r.choice, r.has_probability, r.violates_returned, r.violation_probability + PERTURBATION
            )
            if r.has_probability
            else r
            for r in readings
        ]

    def endpoint_digest(self, address: str, model: str) -> EndpointDigest:
        return self._inner.endpoint_digest(address, model)

    def check_threshold(self, entry_yaml: str, default: float, sources: Sequence[str]) -> ThresholdCheck:
        return self._inner.check_threshold(entry_yaml, default, sources)

    def defaults(self) -> ProductDefaults:
        return self._inner.defaults()


@pytest.fixture(scope="module")
def regression_output(tmp_path_factory: pytest.TempPathFactory) -> AnalysisOutput:
    root = tmp_path_factory.mktemp("regression")
    world = build_world(root / "world")
    deps = analysis_deps(snapshot_matrix(root / "snapshot"), reader=PerturbingReader())
    return AnalyzeUseCase(analysis_settings(world, REGRESSION_RESAMPLES), deps).run()


class TestRegressionBecomesAProtocolEntry:
    def test_a_failed_screen_regression_is_logged_with_k4_and_the_model_as_scope(
        self, regression_output: AnalysisOutput
    ) -> None:
        entries = regression_output.new_deviations
        assert [(entry.kind, entry.scope) for entry in entries] == [(DeviationKind.PROTOCOL, (Kind.K4.value, "M1"))]
        assert "screen regression check failed" in entries[0].text

    def test_the_entry_is_counted_in_the_deviation_summary(self, regression_output: AnalysisOutput) -> None:
        summary = json_object(regression_output.decisions["deviations"], "deviations")
        assert json_object(summary["counts"], "counts")[DeviationKind.PROTOCOL.value] == 1

    def test_only_the_perturbed_model_fails_its_regression(self, regression_output: AnalysisOutput) -> None:
        checks = json_object(regression_output.decisions["checks"], "checks")
        per_model = json_object(checks["per_model"], "per_model")
        failed = {
            model: json_object(json_object(record, model)["screen_regression"], "regression")["failed"]
            for model, record in per_model.items()
        }
        assert [model for model, flag in failed.items() if flag] == ["M1"]
