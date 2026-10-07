"""What a `/v1/systemone` server must answer (§4.6): server validity, the state-size refusals and the typed reading.

The functions here judge bytes: the harness applies them while it runs, `analyze` applies them again to the
recorded bytes, and both reach the same classification.
"""

from __future__ import annotations

import json
import math
import re
from collections.abc import Mapping
from dataclasses import dataclass
from enum import Enum
from typing import Final

from calibration.domain.constants import (
    HTTP_BAD_REQUEST,
    HTTP_OK,
    HTTP_PAYLOAD_TOO_LARGE,
    HTTP_UNPROCESSABLE,
    PROBABILITY_SUM_TOLERANCE,
)
from calibration.domain.jsonread import RecordError, as_mapping, mapping, sequence, text
from calibration.domain.models import Flavour

CHOICE: Final = "choice"
SCORE: Final = "score"
_OLLAMA_TOO_MANY_TOKENS: Final = re.compile(
    r"prompt \d+ has \d+ tokens; expected 1[-\u2013]\d+ \(input is never truncated\)"
)
_OLLAMA_NO_CONTEXT: Final = re.compile(r"prompt \d+ requires \d+ context tokens for scoring; model has \d+")
_KEV_STATE_LENGTH: Final = re.compile(r"state is [\d,]+ tokens, over the [\d,]+-token limit")
_UTF8_ERRORS: Final = "replace"


class ReplyClass(Enum):
    """How a recorded reply reads to the harness."""

    VALID = "valid"
    STATE_TOO_LARGE = "state_too_large"
    INVALID = "invalid"


@dataclass(frozen=True)
class QuestionSpec:
    """One question of a closed set as the server must answer it: its type and its options or levels in order."""

    question_id: str
    question_type: str
    options: tuple[str, ...]

    @property
    def probability_keys(self) -> tuple[frozenset[str], ...]:
        """The key sets a server's `probabilities` may use: the options by name, or a score's levels by index."""
        by_name = frozenset(self.options)
        if self.question_type == SCORE:
            return (frozenset(str(index) for index in range(len(self.options))), by_name)
        return (by_name,)


def question_specs(closed: object) -> tuple[QuestionSpec, ...]:
    """The questions of a closed set, in order; `closed` is the parsed `closed` member of an item."""
    questions = sequence(mapping(closed, "closed").get("questions"), "closed.questions")
    specs: list[QuestionSpec] = []
    for index, raw in enumerate(questions):
        question = mapping(raw, f"closed.questions[{index}]")
        options = tuple(
            text(mapping(option, f"closed.questions[{index}].options[]").get("name"), "option name")
            for option in sequence(question.get("options"), f"closed.questions[{index}].options")
        )
        specs.append(
            QuestionSpec(text(question.get("id"), "question id"), text(question.get("type"), "question type"), options)
        )
    return tuple(specs)


def _decoded(body: bytes) -> str:
    return body.decode("utf-8", errors=_UTF8_ERRORS)


def is_state_too_large(flavour: Flavour, status: int | None, body: bytes) -> bool:
    """The refusals of §4.6 for a state or a body that is too large; a transport failure (no status) is none."""
    if status is None:
        return False
    if flavour is Flavour.OLLAMA:
        if status == HTTP_PAYLOAD_TOO_LARGE:
            return True
        message = _decoded(body)
        return status == HTTP_BAD_REQUEST and bool(
            _OLLAMA_TOO_MANY_TOKENS.search(message) or _OLLAMA_NO_CONTEXT.search(message)
        )
    return status == HTTP_UNPROCESSABLE and bool(_KEV_STATE_LENGTH.search(_decoded(body)))


def _finite_numbers(raw: Mapping[str, object]) -> dict[str, float] | None:
    values: dict[str, float] = {}
    for key, value in raw.items():
        if isinstance(value, bool) or not isinstance(value, int | float) or not math.isfinite(value):
            return None
        values[key] = float(value)
    return values


def _probabilities(raw: object, spec: QuestionSpec) -> dict[str, float] | None:
    declared = as_mapping(raw)
    if declared is None:
        return None
    values = _finite_numbers(declared)
    if values is None or frozenset(values) not in spec.probability_keys:
        return None
    return values if abs(math.fsum(values.values()) - 1.0) <= PROBABILITY_SUM_TOLERANCE else None


def _answer_problem(raw: object, spec: QuestionSpec) -> str | None:
    answer = as_mapping(raw)
    if answer is None:
        return f"{spec.question_id}: the answer is not an object"
    if answer.get("type") != spec.question_type:
        return f"{spec.question_id}: the answer is of type {answer.get('type')!r}, not {spec.question_type}"
    if spec.question_type == CHOICE and answer.get(CHOICE) not in spec.options:
        return f"{spec.question_id}: the choice is none of the options"
    if spec.question_type == SCORE:
        score = answer.get(SCORE)
        if isinstance(score, bool) or not isinstance(score, int | float) or not math.isfinite(score):
            return f"{spec.question_id}: the score is not a finite number"
    if _probabilities(answer.get("probabilities"), spec) is None:
        return f"{spec.question_id}: the probabilities do not name the options, are not finite or do not sum to 1"
    return None


def reply_problems(specs: tuple[QuestionSpec, ...], status: int | None, body: bytes) -> tuple[str, ...]:
    """Why a reply is not server-valid; empty when it is (§4.6)."""
    if status != HTTP_OK:
        return (f"HTTP status {status}",)
    try:
        document = as_mapping(json.loads(body))
    except (UnicodeDecodeError, json.JSONDecodeError) as error:
        return (f"the body is not JSON: {error}",)
    if document is None:
        return ("the body is not an object",)
    answers = as_mapping(document.get("answers"))
    if answers is None:
        return ("the reply has no answers object",)
    problems = [
        problem for spec in specs if (problem := _answer_problem(answers.get(spec.question_id), spec)) is not None
    ]
    return tuple(problems)


def classify_reply(flavour: Flavour, specs: tuple[QuestionSpec, ...], status: int | None, body: bytes) -> ReplyClass:
    if is_state_too_large(flavour, status, body):
        return ReplyClass.STATE_TOO_LARGE
    return ReplyClass.INVALID if reply_problems(specs, status, body) else ReplyClass.VALID


@dataclass(frozen=True)
class AnswerView:
    """One answer of a valid reply as the server stated it."""

    question_id: str
    question_type: str
    choice: str | None
    score: float | None
    confidence: float | None
    probabilities: Mapping[str, float]


@dataclass(frozen=True)
class ReplyView:
    """A valid reply: the model it names and its answers by question id."""

    model: str
    answers: Mapping[str, AnswerView]
    cached_prompt_tokens: int | None


def read_reply(specs: tuple[QuestionSpec, ...], body: bytes) -> ReplyView:
    """Reads a server-valid reply; raises `RecordError` on a reply that `reply_problems` would refuse."""
    document = mapping(json.loads(body), "reply")
    answers = mapping(document.get("answers"), "answers")
    views: dict[str, AnswerView] = {}
    for spec in specs:
        raw = mapping(answers.get(spec.question_id), spec.question_id)
        probabilities = _probabilities(raw.get("probabilities"), spec)
        if probabilities is None:
            raise RecordError(f"{spec.question_id}: no probabilities")
        confidence = raw.get("confidence")
        score = raw.get(SCORE)
        choice = raw.get(CHOICE)
        views[spec.question_id] = AnswerView(
            question_id=spec.question_id,
            question_type=spec.question_type,
            choice=choice if isinstance(choice, str) else None,
            score=float(score) if isinstance(score, int | float) and not isinstance(score, bool) else None,
            confidence=float(confidence)
            if isinstance(confidence, int | float) and not isinstance(confidence, bool)
            else None,
            probabilities=probabilities,
        )
    cached = document.get("prompt_eval_cached_count")
    return ReplyView(
        model=text(document.get("model", ""), "model") if isinstance(document.get("model", ""), str) else "",
        answers=views,
        cached_prompt_tokens=cached if isinstance(cached, int) and not isinstance(cached, bool) else None,
    )


def level_probability(probabilities: Mapping[str, float], index: int, name: str) -> float:
    """The probability a score answer gives to a level: its key is the level's index or its name."""
    if str(index) in probabilities:
        return probabilities[str(index)]
    return probabilities[name]
