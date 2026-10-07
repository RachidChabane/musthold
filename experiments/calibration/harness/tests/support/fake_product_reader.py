"""A pure-Python `ProductReader` that reads a recorded reply the way the Go builders do.

It mirrors `s1relay` (`systemonewire.DecodeClosed`, `DecodeReply`, `question.Set.Check`), `format/hostproto`
(`ParseClosedSet`, `AnswersFromSystemOne`, `ParseClosedAnswers`), `discpower` (`prompt.ParseScreen` and
`power.ViolationProbability`), `format.Threshold` and the endpoint value object of the relay.
"""

from __future__ import annotations

import hashlib
import json
import math
import re
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from decimal import Decimal
from typing import Final, cast
from urllib.parse import urlsplit

import yaml

from calibration.adapters.go_layout import BuilderFailure
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

STAGE_CLOSED: Final = "closed"
STAGE_REPLY: Final = "reply"
STAGE_CHECK: Final = "check"
STAGE_IMPORT: Final = "import"
STAGE_READ: Final = "read"
TYPE_CHOICE: Final = "choice"
TYPE_SCORE: Final = "score"
TYPE_NOUL: Final = "noul"
QUESTION_TYPES: Final = (TYPE_CHOICE, TYPE_NOUL, TYPE_SCORE)
MAX_QUESTIONS: Final = 64
MIN_OPTIONS: Final = 2
MAX_CHOICE_OPTIONS: Final = 26
NOUL_OPTIONS: Final = ("true", "false")
CLOSED_SET_FIELDS: Final = frozenset({"state", "questions"})
CLOSED_QUESTION_FIELDS: Final = frozenset({"id", "type", "instructions", "options"})
CLOSED_OPTION_FIELDS: Final = frozenset({"name", "description"})
MIN_CONFIDENCE: Final = 0.0
MAX_CONFIDENCE: Final = 1.0
CHOICE_VIOLATES: Final = "VIOLATES"
ROUTE: Final = "/v1/systemone"
UNKNOWN_MODEL: Final = "unknown"
SOURCE_PREFIX: Final = "system-one:"
ENDPOINT_SEPARATOR: Final = "@"
THRESHOLD_DEFAULT_KEY: Final = "default"
SOURCE_EXECUTION: Final = "execution"
SOURCE_HOST_VOTES: Final = "host-votes"
SOURCE_SYSTEM_ONE: Final = "system-one"
SOURCE_KINDS: Final = (SOURCE_EXECUTION, SOURCE_HOST_VOTES, SOURCE_SYSTEM_ONE)
HOST_VOTES_SOURCE: Final = re.compile(r"^host-votes\(([+-]?[0-9]+)\)$")
GO_EXPONENT_LOW: Final = -6
GO_EXPONENT_HIGH: Final = 21
LINE_SEPARATOR: Final = chr(0x2028)
PARAGRAPH_SEPARATOR: Final = chr(0x2029)
HTML_ESCAPES: Final = {
    "<": "\\u003c",
    ">": "\\u003e",
    "&": "\\u0026",
    LINE_SEPARATOR: "\\u2028",
    PARAGRAPH_SEPARATOR: "\\u2029",
}
DEFAULT_TAU_SCREEN: Final = 0.20
DEFAULT_TAU_ROUTE: Final = 0.90
DEFAULT_TEMPLATE_DIGEST: Final = "0" * 64
UNREADABLE_DOCUMENT: Final = "the answer document cannot be read"
HALF: Final = 0.5


class _Refused(Exception):
    """A reply the relay or the module refuses: where, and why."""

    def __init__(self, stage: str, message: str) -> None:
        super().__init__(message)
        self.stage = stage
        self.message = message


@dataclass(frozen=True)
class _Question:
    question_id: str
    question_type: str
    options: tuple[str, ...]


def _object(value: object, where: str, stage: str) -> Mapping[str, object]:
    if not isinstance(value, dict):
        raise _Refused(stage, f"{where} is not an object")
    return cast(Mapping[str, object], value)


def _text_member(record: Mapping[str, object], key: str, where: str, stage: str) -> str:
    value = record.get(key, "")
    if not isinstance(value, str):
        raise _Refused(stage, f"{where}.{key} is not a string")
    return value


def _is_number(value: object) -> bool:
    return isinstance(value, int | float) and not isinstance(value, bool)


def go_round(value: float) -> int:
    """`math.Round`: half away from zero."""
    return math.floor(abs(value) + HALF) * (1 if value >= 0 else -1)


def go_float_text(value: float) -> str:
    """The text `encoding/json` gives a float64: shortest digits, an exponent only below 1e-6 or from 1e21."""
    if value == 0:
        return "0"
    sign = "-" if value < 0 else ""
    digits = Decimal(repr(abs(value)))
    exponent = digits.adjusted()
    if exponent < GO_EXPONENT_LOW or exponent >= GO_EXPONENT_HIGH:
        mantissa_digits = digits.as_tuple().digits
        mantissa = str(mantissa_digits[0]) + ("." + "".join(map(str, mantissa_digits[1:])).rstrip("0")).rstrip(".")
        return f"{sign}{mantissa}e{exponent:+03d}".replace("e-0", "e-").replace("e+0", "e+")
    plain = format(digits, "f")
    if "." in plain:
        plain = plain.rstrip("0").rstrip(".")
    return sign + plain


def _go_string(value: str) -> str:
    encoded = json.dumps(value, ensure_ascii=False)
    return "".join(HTML_ESCAPES.get(character, character) for character in encoded)


def go_json(value: object) -> str:
    """Compact JSON as `json.Marshal` writes a `map[string]any`: sorted keys, HTML escaping, shortest floats."""
    if isinstance(value, dict):
        members = cast(dict[str, object], value)
        return "{" + ",".join(f"{_go_string(key)}:{go_json(members[key])}" for key in sorted(members)) + "}"
    if isinstance(value, list):
        return "[" + ",".join(go_json(item) for item in cast(list[object], value)) + "]"
    if isinstance(value, str):
        return _go_string(value)
    if isinstance(value, bool):
        return "true" if value else "false"
    if value is None:
        return "null"
    if isinstance(value, int | float):
        return go_float_text(float(value))
    raise TypeError(f"cannot encode {value!r}")


def _option_names(options: list[object]) -> list[object]:
    return [cast(Mapping[str, object], option).get("name", "") for option in options]


def _options_problem(question_type: object, options: list[object], where: str) -> str | None:
    names = _option_names(options)
    if question_type == TYPE_NOUL:
        if options and tuple(names) != NOUL_OPTIONS:
            return f"{where}.options a noul question has the options true and false, in that order"
        return None
    most = MAX_CHOICE_OPTIONS if question_type == TYPE_CHOICE else None
    if len(options) < MIN_OPTIONS or (most is not None and len(options) > most):
        return f"{where}.options has {len(options)} options of a {question_type} question"
    if any(not name for name in names) or len(set(names)) != len(names):
        return f"{where}.options a name is empty or used twice"
    return None


def _closed_problem(question: Mapping[str, object], index: int) -> str | None:
    where = f"closed.questions[{index}]"
    question_type = question.get("type")
    options = cast(list[object], question.get("options", []))
    if not question.get("instructions"):
        return f"{where}.instructions must not be empty"
    if question_type not in QUESTION_TYPES:
        return f"{where}.type {question_type!r} is not one of {QUESTION_TYPES}"
    problem = _options_problem(question_type, options, where)
    if problem is None and any(not cast(Mapping[str, object], o).get("description") for o in options):
        return f"{where}.options a description is empty"
    return problem


def _parse_closed(closed_text: str) -> tuple[str, list[_Question]]:
    try:
        document = json.loads(closed_text)
    except json.JSONDecodeError as error:
        raise _Refused(STAGE_CLOSED, f"read the closed request: {error}") from error
    body = _object(document, "closed", STAGE_CLOSED)
    if set(body) - CLOSED_SET_FIELDS:
        raise _Refused(STAGE_CLOSED, "the closed request has an unknown field")
    questions = body.get("questions", [])
    if not isinstance(questions, list) or not _text_member(body, "state", "closed", STAGE_CLOSED):
        raise _Refused(STAGE_CLOSED, "the closed request has no state or no question list")
    items = cast(list[object], questions)
    if not 1 <= len(items) <= MAX_QUESTIONS:
        raise _Refused(STAGE_CLOSED, f"{len(items)} questions, want 1 to {MAX_QUESTIONS}")
    parsed: list[_Question] = []
    for index, raw in enumerate(items):
        parsed.append(_parse_question(raw, index, {question.question_id for question in parsed}))
    return _text_member(body, "state", "closed", STAGE_CLOSED), parsed


def _parse_question(raw: object, index: int, seen: set[str]) -> _Question:
    where = f"closed.questions[{index}]"
    question = _object(raw, where, STAGE_CLOSED)
    if set(question) - CLOSED_QUESTION_FIELDS:
        raise _Refused(STAGE_CLOSED, f"{where} has an unknown field")
    question_id = _text_member(question, "id", where, STAGE_CLOSED)
    _text_member(question, "type", where, STAGE_CLOSED)
    _text_member(question, "instructions", where, STAGE_CLOSED)
    options = question.get("options", [])
    if not isinstance(options, list):
        raise _Refused(STAGE_CLOSED, f"{where}.options is not a list")
    names: list[str] = []
    for option in cast(list[object], options):
        record = _object(option, f"{where}.options[]", STAGE_CLOSED)
        if set(record) - CLOSED_OPTION_FIELDS:
            raise _Refused(STAGE_CLOSED, f"{where}.options[] has an unknown field")
        _text_member(record, "description", where, STAGE_CLOSED)
        names.append(_text_member(record, "name", where, STAGE_CLOSED))
    problem = _closed_problem(question, index)
    if problem is None and (not question_id or question_id in seen):
        problem = f"{where}.id {question_id!r} is empty or used twice"
    if problem is not None:
        raise _Refused(STAGE_CLOSED, problem)
    return _Question(question_id, str(question["type"]), tuple(names))


@dataclass(frozen=True)
class _RelayAnswer:
    question_id: str
    choice: str
    confidence: float | None


def _optional_number(record: Mapping[str, object], key: str, question_id: str) -> float | None:
    value = record.get(key)
    if value is None:
        return None
    if not _is_number(value):
        raise _Refused(STAGE_REPLY, f"answer to {question_id!r}: {key} is not a number")
    return float(cast(float, value))


def _relay_answer(raw: object, question: _Question) -> _RelayAnswer:
    record = _object(raw, f"answer to {question.question_id!r}", STAGE_REPLY)
    wire_type = record.get("type")
    choice = record.get("choice")
    if wire_type is not None and not isinstance(wire_type, str):
        raise _Refused(STAGE_REPLY, f"answer to {question.question_id!r}: type is not a string")
    if choice is not None and not isinstance(choice, str):
        raise _Refused(STAGE_REPLY, f"answer to {question.question_id!r}: choice is not a string")
    score = _optional_number(record, "score", question.question_id)
    confidence = _optional_number(record, "confidence", question.question_id)
    if question.question_type == TYPE_CHOICE:
        if wire_type != TYPE_CHOICE or choice is None:
            raise _Refused(STAGE_REPLY, f"a choice was asked and the reply is of type {wire_type!r}")
        return _RelayAnswer(question.question_id, choice, confidence)
    if question.question_type == TYPE_SCORE:
        if wire_type != TYPE_SCORE or score is None:
            raise _Refused(STAGE_REPLY, f"a score was asked and the reply is of type {wire_type!r}")
        level = go_round(score)
        if level < 0 or level >= len(question.options):
            raise _Refused(STAGE_REPLY, f"the score {score} is outside the {len(question.options)} levels")
        return _RelayAnswer(question.question_id, question.options[level], confidence)
    raise _Refused(STAGE_CLOSED, "unsupported question type")


def _decode_reply(reply_text: str, questions: list[_Question]) -> tuple[list[_RelayAnswer], str]:
    try:
        document = json.loads(reply_text)
    except json.JSONDecodeError as error:
        raise _Refused(STAGE_REPLY, f"read the reply: {error}") from error
    body = _object(document, "reply", STAGE_REPLY)
    model = body.get("model", "")
    if model is not None and not isinstance(model, str):
        raise _Refused(STAGE_REPLY, "read the reply: model is not a string")
    declared = body.get("answers")
    answers: Mapping[str, object] = _object(declared, "answers", STAGE_REPLY) if declared is not None else {}
    decoded: list[_RelayAnswer] = []
    for question in questions:
        if question.question_id not in answers:
            raise _Refused(STAGE_REPLY, f"the reply does not answer {question.question_id!r}")
        decoded.append(_relay_answer(answers[question.question_id], question))
    return decoded, model or ""


def _check_answers(answers: list[_RelayAnswer], questions: list[_Question]) -> None:
    for answer, question in zip(answers, questions, strict=True):
        if not answer.choice:
            raise _Refused(STAGE_CHECK, f"a question is not answered: {question.question_id!r}")
        if answer.choice not in question.options:
            raise _Refused(STAGE_CHECK, f"the answer is not one of the options: {answer.choice!r}")
        confidence = answer.confidence
        if confidence is not None and not MIN_CONFIDENCE <= confidence <= MAX_CONFIDENCE:
            raise _Refused(STAGE_CHECK, f"the confidence is outside [0,1]: {confidence}")


def _kept_probabilities(question: _Question, given: object) -> dict[str, float]:
    if given is None:
        return {}
    if not isinstance(given, dict):
        raise _Refused(STAGE_IMPORT, "the probabilities are not an object")
    declared = cast(Mapping[str, object], given)
    for value in declared.values():
        if not _is_number(value):
            raise _Refused(STAGE_IMPORT, "a probability is not a number")
    return {
        option: float(cast(float, declared[option]))
        for option in question.options
        if option in declared and MIN_CONFIDENCE <= float(cast(float, declared[option])) <= MAX_CONFIDENCE
    }


def _imported_answer(reply_text: str, question: _Question) -> tuple[str, float | None, dict[str, float]]:
    answers = _object(json.loads(reply_text).get("answers"), "answers", STAGE_IMPORT)
    given = _object(answers.get(question.question_id), question.question_id, STAGE_IMPORT)
    if question.question_type == TYPE_CHOICE:
        choice = given.get("choice")
        confidence = given.get("confidence")
        return (
            str(choice),
            None if confidence is None else float(cast(float, confidence)),
            _kept_probabilities(question, given.get("probabilities")),
        )
    score = float(cast(float, given.get("score")))
    index = max(0, min(len(question.options) - 1, go_round(score)))
    confidence = given.get("confidence")
    return question.options[index], None if confidence is None else float(cast(float, confidence)), {}


def _document(entries: list[tuple[str, str, float | None, dict[str, float]]]) -> str:
    answers: list[object] = []
    for question_id, choice, confidence, probabilities in entries:
        entry: dict[str, object] = {"id": question_id, "choice": choice}
        if confidence is not None:
            entry["confidence"] = confidence
        if probabilities:
            entry["probabilities"] = dict(probabilities)
        answers.append(entry)
    return go_json({"answers": answers})


def _decode_one(request: DecodeRequest) -> DecodeOutcome:
    try:
        _, questions = _parse_closed(request.closed)
        answers, model = _decode_reply(request.reply, questions)
        _check_answers(answers, questions)
        entries = [(question.question_id, *_imported_answer(request.reply, question)) for question in questions]
    except _Refused as refusal:
        return DecodeOutcome(request.key, False, refusal.stage, refusal.message, None, ())
    document = _document(entries)
    decoded = tuple(
        DecodedAnswer(question_id, choice, confidence, probabilities)
        for question_id, choice, confidence, probabilities in entries
    )
    return DecodeOutcome(request.key, True, None, None, model or None, decoded, document)


def _clean_path(path: str) -> str:
    """`path.Clean` of an absolute path."""
    parts: list[str] = []
    for part in path.split("/"):
        if part in ("", "."):
            continue
        if part == "..":
            if parts:
                parts.pop()
            continue
        parts.append(part)
    return "/" + "/".join(parts)


def _is_known_source(source: str) -> bool:
    if source == SOURCE_EXECUTION:
        return True
    votes = HOST_VOTES_SOURCE.match(source)
    if votes is not None:
        return int(votes.group(1)) >= 1
    return source.startswith(SOURCE_PREFIX) and len(source) > len(SOURCE_PREFIX)


def _source_kind(source: str) -> str | None:
    if not _is_known_source(source):
        return None
    cut = min((source.find(mark) for mark in "(:" if source.find(mark) >= 0), default=-1)
    return source[:cut] if cut >= 0 else source


def _validate_threshold(default: float, entries: Mapping[str, float]) -> None:
    if not MIN_CONFIDENCE <= default <= MAX_CONFIDENCE:
        raise ValueError(f"invalid threshold: {default} is outside [0,1]")
    for source in sorted(entries):
        if not _is_known_source(source) and source not in SOURCE_KINDS:
            raise ValueError(f"invalid threshold: {source!r} is not a source kind nor an exact source")
        if not MIN_CONFIDENCE <= entries[source] <= MAX_CONFIDENCE:
            raise ValueError(f"invalid threshold: {entries[source]} for {source!r} is outside [0,1]")


def _threshold_for(source: str, default: float, entries: Mapping[str, float]) -> float:
    if source in entries:
        return entries[source]
    kind = _source_kind(source)
    if kind is not None and kind in entries:
        return entries[kind]
    return default


def _threshold_override(content: str) -> tuple[float | None, dict[str, float]]:
    value: object = yaml.safe_load(content)
    if _is_number(value):
        return float(cast(float, value)), {}
    if not isinstance(value, dict):
        raise ValueError("invalid threshold: a threshold is a number or a mapping of numbers")
    default: float | None = None
    entries: dict[str, float] = {}
    for key, raw in cast(dict[object, object], value).items():
        if not isinstance(key, str) or not _is_number(raw):
            raise ValueError(f"invalid threshold: the entry {key!r} is not a number")
        if key == THRESHOLD_DEFAULT_KEY:
            default = float(cast(float, raw))
        else:
            entries[key] = float(cast(float, raw))
    return default, entries


class FakeProductReader:
    """`ProductReader` in pure Python, with the semantics of the compiled builders."""

    def __init__(self, defaults: ProductDefaults | None = None) -> None:
        self._defaults = defaults or ProductDefaults(
            DEFAULT_TAU_SCREEN, DEFAULT_TAU_ROUTE, DEFAULT_TEMPLATE_DIGEST, DEFAULT_TEMPLATE_DIGEST
        )

    def decode(self, requests: Sequence[DecodeRequest]) -> list[DecodeOutcome]:
        return [_decode_one(request) for request in requests]

    def read_screen(self, requests: Sequence[ScreenReadRequest]) -> list[ScreenReading]:
        return [_read_screen_answer(request) for request in requests]

    def endpoint_digest(self, address: str, model: str) -> EndpointDigest:
        trimmed = address.strip()
        if not trimmed:
            raise BuilderFailure(["endpoint-digest"], "the endpoint is empty")
        parts = urlsplit(trimmed)
        host = parts.netloc.rsplit("@", 1)[-1]
        if parts.scheme not in ("http", "https"):
            raise BuilderFailure(["endpoint-digest"], f"the endpoint must be an http or https URL: {parts.scheme!r}")
        if not parts.hostname:
            raise BuilderFailure(["endpoint-digest"], "the endpoint names no host")
        path = ROUTE if parts.path in ("", "/") else parts.path
        canonical = f"{parts.scheme.lower()}://{host.lower()}{_clean_path(path)}"
        digest = hashlib.sha256(canonical.encode("utf-8")).hexdigest()
        url = parts._replace(path=path, fragment="").geturl()
        source = f"{SOURCE_PREFIX}{model or UNKNOWN_MODEL}{ENDPOINT_SEPARATOR}{digest}"
        return EndpointDigest(url, digest, source)

    def check_threshold(self, entry_yaml: str, default: float, sources: Sequence[str]) -> ThresholdCheck:
        if default < 0:
            raise BuilderFailure(["check-threshold"], "--default is required")
        try:
            override_default, entries = _threshold_override(entry_yaml)
            applied_default = default if override_default is None else override_default
            _validate_threshold(applied_default, entries)
        except ValueError as error:
            raise BuilderFailure(["check-threshold"], str(error)) from error
        resolved = {source: _threshold_for(source, applied_default, entries) for source in entries}
        for source in sources:
            if source:
                resolved[source] = _threshold_for(source, applied_default, entries)
        return ThresholdCheck(applied_default, dict(entries), resolved)

    def defaults(self) -> ProductDefaults:
        return self._defaults


def _read_screen_answer(request: ScreenReadRequest) -> ScreenReading:
    def failed(message: str) -> ScreenReading:
        return ScreenReading(request.key, message, None, False, 0.0, 0.0)

    try:
        document = json.loads(request.document)
        answers = _object(document, "document", STAGE_READ).get("answers", [])
        if not isinstance(answers, list):
            return failed(UNREADABLE_DOCUMENT)
        parsed: dict[str, Mapping[str, object]] = {}
        for entry in cast(list[object], answers):
            record = _object(entry, "answer", STAGE_READ)
            parsed[_text_member(record, "id", "answer", STAGE_READ)] = record
    except (json.JSONDecodeError, _Refused):
        return failed(UNREADABLE_DOCUMENT)
    given = parsed.get(request.question)
    if given is None:
        return failed(f"the document does not answer {request.question}")
    choice = given.get("choice") or None
    probabilities = given.get("probabilities")
    stated: Mapping[str, object] = cast(Mapping[str, object], probabilities) if isinstance(probabilities, dict) else {}
    has_probability = CHOICE_VIOLATES in stated and _is_number(stated[CHOICE_VIOLATES])
    returned = float(cast(float, stated[CHOICE_VIOLATES])) if has_probability else 0.0
    violation = returned if has_probability else float(choice == CHOICE_VIOLATES)
    return ScreenReading(request.key, None, cast("str | None", choice), has_probability, returned, violation)
