"""Synthetic server replies in the real wire forms of the Ollama and Kev `/v1/systemone` routes.

The behaviour of a model is a deterministic function of (model, request, question): a tunable accuracy decides
whether it answers the right option, and a sharpness and a calibration decide how much probability the answer
takes. Confidence formulas are written here independently of the harness.
"""

from __future__ import annotations

import json
import math
from collections.abc import Mapping, Sequence
from dataclasses import dataclass, field
from enum import Enum
from typing import Final

from calibration.domain.models import Flavour, ModelSpec
from support.world_model import ADEQUATE_LEVELS, COVERAGE_LEVELS, unit_float

KEV_DECIMALS: Final = 4
MIN_PEAK_MARGIN: Final = 0.02
CALIBRATION_REWARD: Final = 0.2
CALIBRATION_PENALTY: Final = 0.3
OTHER_WEIGHT_FLOOR: Final = 0.2
SEPARATORS: Final = (",", ":")
EN_DASH: Final = chr(0x2013)
OLLAMA_TOO_LARGE: Final = f"prompt 0 has 5000 tokens; expected 1{EN_DASH}2048 (input is never truncated)"
KEV_TOO_LARGE: Final = (
    "state is 70,000 tokens, over the 65,536-token limit (the <state> token included): shorten the document or "
    "split it across requests; or start the server with KEV_TRUNCATE_STATES=1 to truncate it"
)
INVALID_BODY: Final = b"{not json"
HTTP_OK: Final = 200
HTTP_BAD_REQUEST: Final = 400
HTTP_UNPROCESSABLE: Final = 422
FIRST_ATTEMPT: Final = 1
SECOND_ATTEMPT: Final = 2
OUT_OF_RANGE_CONFIDENCE: Final = 1.2
SERVER_VERSION: Final = "synthetic-1"
LATENCY_MS: Final = 12.5
FLIP_SHIFT: Final = 0.5
PREFIX_CACHE_SHARE: Final = 0.5
WEAK_LEVEL: Final = COVERAGE_LEVELS.index("WEAK")
FULL_LEVEL: Final = COVERAGE_LEVELS.index("FULL")


class Fault(Enum):
    """What goes wrong with one request of a model."""

    STATE_TOO_LARGE = "state_too_large"
    INVALID_TWICE = "invalid_twice"
    NO_RESPONSE = "no_response"
    TRANSPORT_FAILURE = "transport_failure"
    CONFIDENCE_OUT_OF_RANGE = "confidence_out_of_range"


@dataclass(frozen=True)
class ModelBehaviour:
    """How a model answers: accuracy per family, sharpness and calibration of its probabilities, pass-2 flips and
    the faults it suffers, keyed by request id."""

    alignment_accuracy: float
    screen_accuracy: float
    sharpness: float
    calibration: float
    jitter: float = 0.05
    flip_rate: float = 0.0
    faults: Mapping[str, Fault] = field(default_factory=dict[str, Fault])


@dataclass(frozen=True)
class QuestionPlan:
    """One question of an item as the server must answer it, and the option a right answer names."""

    question_id: str
    question_type: str
    options: tuple[str, ...]
    target: int


@dataclass(frozen=True)
class StoredExchange:
    """The bytes the harness kept for one request."""

    status: int | None
    body: bytes
    attempt: int
    transport_error: str | None = None


def _entropy_confidence(probabilities: Sequence[float]) -> float:
    count = len(probabilities)
    if count == 1:
        return 1.0
    entropy = -sum(p * math.log(p) for p in probabilities if p > 0.0)
    return 1.0 - entropy / math.log(count)


def _kev_choice_confidence(probabilities: Sequence[float]) -> float:
    count = len(probabilities)
    return 1.0 if count == 1 else (max(probabilities) - 1 / count) / (1 - 1 / count)


def _kev_score_confidence(probabilities: Sequence[float]) -> float:
    levels = len(probabilities)
    mode = max(range(levels), key=probabilities.__getitem__)
    deviation = sum(abs(i - (levels - 1) / 2) for i in range(levels)) / levels
    spread = sum(p * abs(i - mode) for i, p in enumerate(probabilities))
    return max(0.0, 1.0 - spread / deviation)


def _distribution(
    model_id: str, request_id: str, question_id: str, count: int, chosen: int, peak: float
) -> list[float]:
    weights = [
        0.0 if index == chosen else OTHER_WEIGHT_FLOOR + unit_float(model_id, request_id, question_id, "w", index)
        for index in range(count)
    ]
    rest = 1.0 - peak
    total = sum(weights)
    return [peak if index == chosen else rest * weights[index] / total for index in range(count)]


def _wrong_index(model_id: str, request_id: str, plan: QuestionPlan) -> int:
    count = len(plan.options)
    if plan.question_type == "score":
        return WEAK_LEVEL if COVERAGE_LEVELS[plan.target] in ADEQUATE_LEVELS else FULL_LEVEL
    return (plan.target + 1 + int(unit_float(model_id, request_id, plan.question_id, "wrong") * (count - 1))) % count


def _is_right(
    behaviour: ModelBehaviour, accuracy: float, model_id: str, request_id: str, question_id: str, second_pass: bool
) -> bool:
    draw = unit_float(model_id, request_id, question_id, "draw")
    if second_pass and unit_float(model_id, request_id, question_id, "flip") < behaviour.flip_rate:
        draw = (draw + FLIP_SHIFT) % 1.0
    return draw < accuracy


def _peak(
    behaviour: ModelBehaviour, model_id: str, request_id: str, question_id: str, count: int, right: bool
) -> float:
    shift = CALIBRATION_REWARD if right else -CALIBRATION_PENALTY
    noise = behaviour.jitter * (2 * unit_float(model_id, request_id, question_id, "jitter") - 1)
    level = behaviour.sharpness + behaviour.calibration * shift + noise
    return min(1.0, max(1 / count + MIN_PEAK_MARGIN, level))


def _answer(
    model: ModelSpec,
    behaviour: ModelBehaviour,
    request_id: str,
    plan: QuestionPlan,
    accuracy: float,
    second_pass: bool,
) -> dict[str, object]:
    right = _is_right(behaviour, accuracy, model.model_id, request_id, plan.question_id, second_pass)
    chosen = plan.target if right else _wrong_index(model.model_id, request_id, plan)
    count = len(plan.options)
    peak = _peak(behaviour, model.model_id, request_id, plan.question_id, count, right)
    probabilities = _distribution(model.model_id, request_id, plan.question_id, count, chosen, peak)
    is_kev = model.flavour is Flavour.KEV
    if is_kev:
        probabilities = [round(value, KEV_DECIMALS) for value in probabilities]
    if plan.question_type == "choice":
        confidence = _kev_choice_confidence(probabilities) if is_kev else _entropy_confidence(probabilities)
        return {
            "type": "choice",
            "choice": plan.options[chosen],
            "probabilities": dict(zip(plan.options, probabilities, strict=True)),
            "confidence": round(confidence, KEV_DECIMALS) if is_kev else confidence,
        }
    confidence = _kev_score_confidence(probabilities) if is_kev else _entropy_confidence(probabilities)
    score = sum(index * value for index, value in enumerate(probabilities))
    return {
        "type": "score",
        "score": round(score, KEV_DECIMALS) if is_kev else score,
        "legend": {str(index): name for index, name in enumerate(plan.options)},
        "probabilities": {str(index): value for index, value in enumerate(probabilities)},
        "confidence": round(confidence, KEV_DECIMALS) if is_kev else confidence,
    }


def _encode(document: object) -> bytes:
    return json.dumps(document, ensure_ascii=False, separators=SEPARATORS).encode("utf-8")


def _too_large(model: ModelSpec) -> StoredExchange:
    if model.flavour is Flavour.OLLAMA:
        return StoredExchange(HTTP_BAD_REQUEST, _encode({"error": OLLAMA_TOO_LARGE}), FIRST_ATTEMPT)
    return StoredExchange(HTTP_UNPROCESSABLE, _encode({"detail": KEV_TOO_LARGE}), FIRST_ATTEMPT)


def exchange_for(
    model: ModelSpec,
    behaviour: ModelBehaviour,
    request_id: str,
    plans: Sequence[QuestionPlan],
    *,
    family_accuracy: float,
    second_pass: bool,
) -> StoredExchange | None:
    """What the harness stored for the request; None when no reply was ever recorded."""
    fault = behaviour.faults.get(request_id)
    if fault is Fault.NO_RESPONSE:
        return None
    if fault is Fault.STATE_TOO_LARGE:
        return _too_large(model)
    if fault is Fault.INVALID_TWICE:
        return StoredExchange(HTTP_OK, INVALID_BODY, SECOND_ATTEMPT)
    if fault is Fault.TRANSPORT_FAILURE:
        return StoredExchange(None, b"", SECOND_ATTEMPT, "timed out")
    answers = {
        plan.question_id: _answer(model, behaviour, request_id, plan, family_accuracy, second_pass) for plan in plans
    }
    if fault is Fault.CONFIDENCE_OUT_OF_RANGE:
        first = answers[plans[0].question_id]
        first["confidence"] = OUT_OF_RANGE_CONFIDENCE
    reply: dict[str, object] = {
        "model": model.identifier,
        "answers": answers,
        "usage": {"input_tokens": 100, "output_tokens": 1},
    }
    if model.flavour is Flavour.KEV:
        reply["latency_ms"] = LATENCY_MS
    return StoredExchange(HTTP_OK, _encode(reply), FIRST_ATTEMPT)


def prefix_cache_hit(model: ModelSpec, request_id: str) -> bool | None:
    """The Kev prefix-cache outcome recorded in the meta of a request; Ollama records none."""
    if model.flavour is not Flavour.KEV:
        return None
    return unit_float(model.model_id, request_id, "cache") < PREFIX_CACHE_SHARE
