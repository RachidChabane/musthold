"""Source (1) checks (§5.4, §7.4): confidence and formula checks, the screen regression check and the key check."""

from __future__ import annotations

import math
from collections import Counter
from collections.abc import Mapping
from dataclasses import dataclass, field
from itertools import pairwise
from typing import Final

import numpy as np

from calibration.analysis.model import CellData, Kind
from calibration.analysis.replies import ReplyOutcome
from calibration.analysis.source1 import ANSWER_VIOLATES, QUESTION_COVERAGE, QUESTION_CRITERION
from calibration.analysis.stats.ranking import spearman
from calibration.domain.canonical import JsonValue
from calibration.domain.confidence_formulas import confidence_matches, expected_confidence
from calibration.domain.items import Item, KindFamily
from calibration.domain.models import ModelSpec
from calibration.domain.ports import ScreenReading
from calibration.domain.server_wire import AnswerView, question_specs

SCREEN_PROBABILITY_TOLERANCE: Final = 1e-9
CONFIDENCE_QUESTIONS: Final = (QUESTION_CRITERION, QUESTION_COVERAGE)


@dataclass(frozen=True)
class ConfidenceChecks:
    """K1 to K3 per model: the rank correlation of the product confidence with the returned probability of the
    predicted option, and the answers that state no confidence."""

    spearman: Mapping[Kind, float]
    without_confidence: int


def confidence_checks(cells: Mapping[Kind, CellData], outcomes: Mapping[str, ReplyOutcome]) -> ConfidenceChecks:
    correlations: dict[Kind, float] = {}
    for kind, cell in cells.items():
        pairs = [(d.confidence, d.alt_confidence) for d in cell.decisions if d.alt_confidence is not None]
        first = np.array([pair[0] for pair in pairs], dtype=np.float64)
        second = np.array([pair[1] for pair in pairs], dtype=np.float64)
        correlations[kind] = spearman(first, second)
    missing = sum(
        1
        for outcome in outcomes.values()
        if outcome.answered
        for question in CONFIDENCE_QUESTIONS
        if question in outcome.answers and outcome.answers[question].confidence is None
    )
    return ConfidenceChecks(correlations, missing)


@dataclass(frozen=True)
class FormulaFinding:
    request_id: str
    question_id: str
    stated: float | None
    recomputed: float

    def to_record(self) -> dict[str, JsonValue]:
        return {
            "request": self.request_id,
            "question": self.question_id,
            "stated": self.stated,
            "recomputed": self.recomputed,
        }


def _ordered(probabilities: Mapping[str, float], options: tuple[str, ...], is_score: bool) -> list[float]:
    keys = [str(i) for i in range(len(options))] if is_score and str(0) in probabilities else list(options)
    return [probabilities[key] for key in keys]


def formula_checks(
    model: ModelSpec, outcomes: Mapping[str, ReplyOutcome], items: Mapping[str, Item]
) -> tuple[int, list[FormulaFinding]]:
    """Recomputes each server's confidence from its returned probabilities on every answer (§5.4): the answers
    checked and the ones outside the 0.001 tolerance."""
    checked = 0
    findings: list[FormulaFinding] = []
    for request_id, outcome in sorted(outcomes.items()):
        if not outcome.answered or outcome.returned is None:
            continue
        for spec in question_specs(items[request_id].closed):
            view: AnswerView = outcome.returned.answers[spec.question_id]
            probabilities = _ordered(view.probabilities, spec.options, spec.question_type == "score")
            recomputed = expected_confidence(model.flavour, spec.question_type, probabilities)
            checked += 1
            if view.confidence is None or not confidence_matches(view.confidence, recomputed):
                findings.append(FormulaFinding(request_id, spec.question_id, view.confidence, recomputed))
    return checked, findings


@dataclass(frozen=True)
class ScreenRegression:
    """K4 per model: the product's P(VIOLATES) against the server's returned probability of VIOLATES."""

    answers: int
    without_probability: int
    mismatches: tuple[str, ...]
    monotone: bool

    @property
    def failed(self) -> bool:
        return bool(self.mismatches) or not self.monotone

    def to_record(self) -> dict[str, JsonValue]:
        return {
            "answers": self.answers,
            "without_probability": self.without_probability,
            "mismatches": list(self.mismatches),
            "monotone": self.monotone,
            "failed": self.failed,
        }


def screen_regression(
    outcomes: Mapping[str, ReplyOutcome], items: Mapping[str, Item], readings: Mapping[str, ScreenReading]
) -> ScreenRegression:
    """The product's P(VIOLATES) must equal the server's returned probability within 1e-9 and be non-decreasing in
    it over all the model's answers; answers whose document carries no probability of VIOLATES are counted."""
    pairs: list[tuple[float, float]] = []
    mismatches: list[str] = []
    without = 0
    for request_id, outcome in sorted(outcomes.items()):
        item = items[request_id]
        if item.family is not KindFamily.SCREEN or not outcome.answered or outcome.returned is None:
            continue
        for question in sorted(item.question_to_ref or {}):
            reading = readings.get(f"{request_id}#{question}")
            if reading is None or reading.error is not None:
                continue
            returned = outcome.returned.answers[question].probabilities.get(ANSWER_VIOLATES)
            if returned is None or not reading.has_probability:
                without += 1
                continue
            pairs.append((returned, reading.violation_probability))
            if not math.isclose(
                returned, reading.violation_probability, rel_tol=0.0, abs_tol=SCREEN_PROBABILITY_TOLERANCE
            ):
                mismatches.append(f"{request_id}#{question}")
    ordered = sorted(pairs)
    monotone = all(later[1] >= earlier[1] for earlier, later in pairwise(ordered))
    return ScreenRegression(len(pairs) + without, without, tuple(mismatches), monotone)


@dataclass(frozen=True)
class KeyCheck:
    """Each model's exact `confidenceSource` string; two models sharing a string are a key collision."""

    sources: Mapping[str, str]
    collisions: tuple[tuple[str, ...], ...] = field(default_factory=tuple)


def key_check(sources: Mapping[str, str]) -> KeyCheck:
    counts = Counter(sources.values())
    shared = tuple(
        tuple(sorted(model for model, source in sources.items() if source == duplicate))
        for duplicate in sorted(source for source, count in counts.items() if count > 1)
    )
    return KeyCheck(dict(sources), shared)
