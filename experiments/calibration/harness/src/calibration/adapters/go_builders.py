"""The compiled Go builders and the product's own reader, driven through their command lines."""

from __future__ import annotations

import json
from collections.abc import Mapping, Sequence
from pathlib import Path
from typing import Final, cast

from calibration.adapters.go_layout import (
    ALIGNMENT_BUILDER_BINARY,
    DECIDE_CLOSED_KIND,
    DECODE_BINARY,
    DISCPOWER_BUILDER_BINARY,
    LIST_SEPARATOR,
    BinaryInvoker,
    BuilderFailure,
)
from calibration.domain.canonical import JsonValue
from calibration.domain.jsonread import RecordError, boolean, mapping, optional_text, sequence, text
from calibration.domain.ports import (
    DecodedAnswer,
    DecodeOutcome,
    DecodeRequest,
    EndpointDigest,
    ProcessRunner,
    ProductDefaults,
    ScreenReading,
    ScreenReadRequest,
    ThresholdCheck,
)

__all__ = ["BinaryBuilders", "BinaryProductReader", "BuilderFailure"]

STATUS_OK: Final = "ok"
FLAG_PROJECT: Final = "--project"
FLAG_SHALLNOT: Final = "--shallnot"
FLAG_MODELS: Final = "--models"
FLAG_ADDRESS: Final = "--address"
FLAG_MODEL: Final = "--model"
FLAG_DEFAULT: Final = "--default"
FLAG_SOURCES: Final = "--sources"


def _as_json_object(command: Sequence[str], value: object) -> Mapping[str, JsonValue]:
    if not isinstance(value, Mapping):
        raise BuilderFailure(command, "the output is not a JSON object")
    return cast(Mapping[str, JsonValue], value)


class BinaryBuilders:
    """`Builders` over the compiled alignment and discpower builders; `env` is laid over the inherited environment."""

    def __init__(self, runner: ProcessRunner, binaries_dir: Path, env: Mapping[str, str] | None = None) -> None:
        self._alignment = BinaryInvoker(runner, binaries_dir / ALIGNMENT_BUILDER_BINARY, env)
        self._discpower = BinaryInvoker(runner, binaries_dir / DISCPOWER_BUILDER_BINARY, env)

    def alignment_build(self, project: Path, shallnot: Path, identifiers: Sequence[str]) -> Mapping[str, JsonValue]:
        return self._call(self._alignment, ["build", *_gateway_flags(project, shallnot, identifiers)])

    def alignment_defaults(self) -> Mapping[str, JsonValue]:
        return self._call(self._alignment, ["defaults"])

    def render_closed(self, closed_text: str, identifiers: Sequence[str]) -> Mapping[str, JsonValue]:
        return self._call(self._alignment, ["render", FLAG_MODELS, LIST_SEPARATOR.join(identifiers)], closed_text)

    def compare_export(self, project: Path, shallnot: Path, export_text: str) -> Mapping[str, JsonValue]:
        flags = [FLAG_PROJECT, str(project.absolute()), FLAG_SHALLNOT, str(shallnot.absolute())]
        return self._call(self._alignment, ["compare-export", *flags], export_text)

    def screen_build(
        self, project: Path, shallnot: Path, identifiers: Sequence[str], mutants: Sequence[Mapping[str, JsonValue]]
    ) -> Mapping[str, JsonValue]:
        stdin = json.dumps({"mutants": list(mutants)}, ensure_ascii=False, allow_nan=False)
        return self._call(self._discpower, ["build", *_gateway_flags(project, shallnot, identifiers)], stdin)

    def screen_defaults(self) -> Mapping[str, JsonValue]:
        return self._call(self._discpower, ["defaults"])

    @staticmethod
    def _call(invoker: BinaryInvoker, arguments: Sequence[str], stdin: str | None = None) -> Mapping[str, JsonValue]:
        return _as_json_object(arguments, invoker.call(arguments, stdin))


def _gateway_flags(project: Path, shallnot: Path, identifiers: Sequence[str]) -> list[str]:
    return [
        FLAG_PROJECT,
        str(project.absolute()),
        FLAG_SHALLNOT,
        str(shallnot.absolute()),
        FLAG_MODELS,
        LIST_SEPARATOR.join(identifiers),
    ]


class BinaryProductReader:
    """`ProductReader` over the compiled decoder and the two builders' read, threshold and defaults modes."""

    def __init__(self, runner: ProcessRunner, binaries_dir: Path, env: Mapping[str, str] | None = None) -> None:
        self._decoder = BinaryInvoker(runner, binaries_dir / DECODE_BINARY, env)
        self._alignment = BinaryInvoker(runner, binaries_dir / ALIGNMENT_BUILDER_BINARY, env)
        self._discpower = BinaryInvoker(runner, binaries_dir / DISCPOWER_BUILDER_BINARY, env)

    def decode(self, requests: Sequence[DecodeRequest]) -> list[DecodeOutcome]:
        if not requests:
            return []
        wire = [{"key": item.key, "closed": item.closed, "reply": item.reply} for item in requests]
        arguments = ["decode"]
        output = self._decoder.call(arguments, json.dumps(wire, ensure_ascii=False))
        return [_decode_outcome(entry) for entry in _list(arguments, output)]

    def read_screen(self, requests: Sequence[ScreenReadRequest]) -> list[ScreenReading]:
        if not requests:
            return []
        arguments = ["read"]
        output = self._discpower.call(arguments, screen_read_input(requests))
        return [_screen_reading(entry) for entry in _list(arguments, output)]

    def endpoint_digest(self, address: str, model: str) -> EndpointDigest:
        arguments = ["endpoint-digest", FLAG_ADDRESS, address, FLAG_MODEL, model]
        body = _record(arguments, self._decoder.call(arguments))
        return EndpointDigest(
            address=text(body.get("address"), "address"),
            digest=text(body.get("digest"), "digest"),
            source=text(body.get("source"), "source"),
        )

    def check_threshold(self, entry_yaml: str, default: float, sources: Sequence[str]) -> ThresholdCheck:
        arguments = ["check-threshold", FLAG_DEFAULT, repr(default), FLAG_SOURCES, LIST_SEPARATOR.join(sources)]
        body = _record(arguments, self._discpower.call(arguments, entry_yaml))
        return ThresholdCheck(
            default=_number(body.get("default"), "default"),
            by_source=_number_map(body.get("by_source"), "by_source"),
            resolved=_number_map(body.get("resolved"), "resolved"),
        )

    def defaults(self) -> ProductDefaults:
        alignment = _record(["defaults"], self._alignment.call(["defaults"]))
        screen = _record(["defaults"], self._discpower.call(["defaults"]))
        return ProductDefaults(
            tau_screen=_number(screen.get("tau_screen"), "tau_screen"),
            tau_route=_number(alignment.get("tau_route"), "tau_route"),
            screen_template_digest=_template_digest(screen),
            alignment_template_digest=_template_digest(alignment),
        )


def _record(command: Sequence[str], value: object) -> Mapping[str, object]:
    try:
        return mapping(value, "output")
    except RecordError as error:
        raise BuilderFailure(command, str(error)) from error


def _list(command: Sequence[str], value: object) -> list[object]:
    try:
        return sequence(value, "output")
    except RecordError as error:
        raise BuilderFailure(command, str(error)) from error


def _number(value: object, where: str) -> float:
    if isinstance(value, bool) or not isinstance(value, int | float):
        raise RecordError(f"{where} must be a number")
    return float(value)


def _number_map(value: object, where: str) -> dict[str, float]:
    return {key: _number(item, f"{where}.{key}") for key, item in mapping(value, where).items()}


def _template_digest(defaults: Mapping[str, object]) -> str:
    return text(
        mapping(defaults.get("template_digests"), "template_digests").get(DECIDE_CLOSED_KIND), DECIDE_CLOSED_KIND
    )


def screen_read_input(requests: Sequence[ScreenReadRequest]) -> str:
    """The input of `read`, with each answer document spliced in as the text it is."""
    entries: list[str] = []
    for request in requests:
        try:
            json.loads(request.document)
        except json.JSONDecodeError as error:
            raise BuilderFailure(["read"], f"the document of {request.key} is not JSON: {error}") from error
        key, question = json.dumps(request.key), json.dumps(request.question)
        entries.append(f'{{"key":{key},"document":{request.document},"question":{question}}}')
    return "[" + ",".join(entries) + "]"


def _decode_outcome(entry: object) -> DecodeOutcome:
    body = mapping(entry, "decode result")
    status = text(body.get("status"), "status")
    return DecodeOutcome(
        key=text(body.get("key"), "key"),
        accepted=status == STATUS_OK,
        stage=optional_text(body.get("stage"), "stage"),
        error=optional_text(body.get("error"), "error"),
        model=optional_text(body.get("model"), "model"),
        answers=tuple(_decoded_answer(answer) for answer in sequence(body.get("answers", []), "answers")),
    )


def _decoded_answer(entry: object) -> DecodedAnswer:
    body = mapping(entry, "answer")
    confidence = body.get("confidence")
    return DecodedAnswer(
        question_id=text(body.get("id"), "id"),
        choice=text(body.get("choice"), "choice"),
        confidence=None if confidence is None else _number(confidence, "confidence"),
        probabilities=_number_map(body.get("probabilities", {}), "probabilities"),
    )


def _screen_reading(entry: object) -> ScreenReading:
    body = mapping(entry, "read result")
    return ScreenReading(
        key=text(body.get("key"), "key"),
        error=optional_text(body.get("error"), "error"),
        choice=optional_text(body.get("choice"), "choice"),
        has_probability=boolean(body.get("has_probability"), "has_probability"),
        violates_returned=_number(body.get("violates_returned"), "violates_returned"),
        violation_probability=_number(body.get("violation_probability"), "violation_probability"),
    )
