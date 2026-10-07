"""Typed access to decoded JSON: every record parser reads fields through these, so a malformed record names its field."""

from __future__ import annotations

import re
from collections.abc import Mapping
from enum import Enum
from typing import TypeVar, cast

from calibration.domain.canonical import JsonValue

E = TypeVar("E", bound=Enum)


class RecordError(ValueError):
    """A record that breaks its schema."""


def mapping(value: object, where: str) -> Mapping[str, object]:
    if not isinstance(value, Mapping):
        raise RecordError(f"{where} must be an object")
    return cast(Mapping[str, object], value)


def json_object(value: object, where: str) -> Mapping[str, JsonValue]:
    """An object whose members are JSON values: what `json.loads` returns for a JSON object."""
    return cast(Mapping[str, JsonValue], mapping(value, where))


def as_mapping(value: object) -> Mapping[str, object] | None:
    """The value as an object, or None when it is any other JSON value."""
    return cast(Mapping[str, object], value) if isinstance(value, Mapping) else None


def as_list(value: object) -> list[object] | None:
    """The value as an array, or None when it is any other JSON value."""
    return cast(list[object], value) if isinstance(value, list) else None


def sequence(value: object, where: str) -> list[object]:
    if not isinstance(value, list):
        raise RecordError(f"{where} must be a list")
    return cast(list[object], value)


def text(value: object, where: str, pattern: re.Pattern[str] | None = None) -> str:
    if not isinstance(value, str) or (pattern is not None and pattern.match(value) is None):
        raise RecordError(f"{where} is missing or malformed")
    return value


def integer(value: object, where: str, minimum: int = 0) -> int:
    if isinstance(value, bool) or not isinstance(value, int) or value < minimum:
        raise RecordError(f"{where} must be an integer of at least {minimum}")
    return value


def boolean(value: object, where: str) -> bool:
    if not isinstance(value, bool):
        raise RecordError(f"{where} must be a boolean")
    return value


def optional_boolean(value: object, where: str) -> bool | None:
    return None if value is None else boolean(value, where)


def optional_text(value: object, where: str) -> str | None:
    return None if value is None else text(value, where)


def enum_of(kind: type[E], value: object, where: str) -> E:
    try:
        return kind(value)
    except ValueError as error:
        raise RecordError(f"{where} is not a {kind.__name__}") from error


def only_keys(body: Mapping[str, object], allowed: frozenset[str], where: str) -> None:
    """Refuse a record that carries a field its schema does not name."""
    unknown = sorted(set(body) - allowed)
    if unknown:
        raise RecordError(f"{where} has unknown fields {unknown}")
