"""Decoding of a JSON-lines event stream, shared by the harness stream parsers."""

from __future__ import annotations

import json
from collections.abc import Mapping
from dataclasses import dataclass

from specbench.domain.jsonread import RecordError, mapping, sequence


@dataclass(frozen=True)
class EventLine:
    """One decoded event with its zero-based line number in the stream."""

    line: int
    body: Mapping[str, object]


def decode_event_lines(stream: str, where: str) -> list[EventLine]:
    """Every non-blank line of the stream as a JSON object; a line that is not one names itself in the error."""
    events: list[EventLine] = []
    for number, raw in enumerate(stream.splitlines()):
        if not raw.strip():
            continue
        try:
            decoded: object = json.loads(raw)
        except json.JSONDecodeError as error:
            raise RecordError(f"{where} line {number} is not JSON") from error
        events.append(EventLine(number, mapping(decoded, f"{where} line {number}")))
    return events


def optional_mapping(body: Mapping[str, object], key: str, where: str) -> Mapping[str, object]:
    value = body.get(key)
    return {} if value is None else mapping(value, f"{where}.{key}")


def optional_sequence(body: Mapping[str, object], key: str, where: str) -> list[object]:
    value = body.get(key)
    return [] if value is None else sequence(value, f"{where}.{key}")


def count_field(body: Mapping[str, object], key: str) -> int:
    """A token count that the harness may leave out."""
    value = body.get(key)
    return value if isinstance(value, int) and not isinstance(value, bool) and value >= 0 else 0


def optional_cost(body: Mapping[str, object], key: str) -> float | None:
    value = body.get(key)
    return float(value) if isinstance(value, int | float) and not isinstance(value, bool) else None
