"""Typed reads of decoded JSON: every field the tooling reads goes through one of these."""

from __future__ import annotations

import hashlib
import json
from collections.abc import Mapping, Sequence
from typing import cast

type JsonValue = bool | int | float | str | Sequence[JsonValue] | Mapping[str, JsonValue] | None
type JsonObject = Mapping[str, JsonValue]


class JsonShapeError(ValueError):
    """A JSON document lacks a field the tooling needs, or holds it with another type."""


def as_object(value: JsonValue, where: str = "value") -> JsonObject:
    if not isinstance(value, Mapping):
        raise JsonShapeError(f"{where} is not an object")
    return cast(JsonObject, value)


def as_list(value: JsonValue, where: str = "value") -> Sequence[JsonValue]:
    if isinstance(value, str) or not isinstance(value, Sequence):
        raise JsonShapeError(f"{where} is not a list")
    return cast(Sequence[JsonValue], value)


def field(document: JsonObject, name: str) -> JsonValue:
    if name not in document:
        raise JsonShapeError(f"missing field {name!r}")
    return document[name]


def text(document: JsonObject, name: str) -> str:
    value = field(document, name)
    if not isinstance(value, str):
        raise JsonShapeError(f"field {name!r} is not a string")
    return value


def optional_text(document: JsonObject, name: str) -> str | None:
    value = document.get(name)
    if value is None:
        return None
    if not isinstance(value, str):
        raise JsonShapeError(f"field {name!r} is not a string")
    return value


def integer(document: JsonObject, name: str) -> int:
    value = field(document, name)
    if isinstance(value, bool) or not isinstance(value, int):
        raise JsonShapeError(f"field {name!r} is not an integer")
    return value


def optional_integer(document: JsonObject, name: str) -> int | None:
    value = document.get(name)
    if value is None:
        return None
    if isinstance(value, bool) or not isinstance(value, int):
        raise JsonShapeError(f"field {name!r} is not an integer")
    return value


def boolean(document: JsonObject, name: str) -> bool:
    value = field(document, name)
    if not isinstance(value, bool):
        raise JsonShapeError(f"field {name!r} is not a boolean")
    return value


def optional_boolean(document: JsonObject, name: str) -> bool | None:
    value = document.get(name)
    if value is None:
        return None
    if not isinstance(value, bool):
        raise JsonShapeError(f"field {name!r} is not a boolean")
    return value


def child(document: JsonObject, name: str) -> JsonObject:
    return as_object(field(document, name), name)


def optional_child(document: JsonObject, name: str) -> JsonObject | None:
    value = document.get(name)
    if value is None:
        return None
    return as_object(value, name)


def children(document: JsonObject, name: str) -> list[JsonObject]:
    return [as_object(item, name) for item in as_list(field(document, name), name)]


def decode(raw: str) -> JsonValue:
    return cast(JsonValue, json.loads(raw))


def encode(value: JsonValue) -> str:
    """Stable, human-readable JSON: sorted keys, two-space indent, a final newline."""
    return json.dumps(value, indent=2, sort_keys=True, ensure_ascii=False) + "\n"


def encode_line(value: JsonValue) -> str:
    """One compact JSON Lines record, with sorted keys and a final newline."""
    return json.dumps(value, sort_keys=True, ensure_ascii=False, separators=(",", ":")) + "\n"


def sha256_hex(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()
