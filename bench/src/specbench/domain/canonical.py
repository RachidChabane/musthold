"""Canonical JSON: the one byte-level serialization of every committed or digested record.

Keys are sorted, separators are compact, text is UTF-8 without ASCII escaping, and
the document ends with one newline. Floats are refused: a record that must hash
identically on every machine carries integers and strings only. Results that
legitimately hold ratios go through `round_floats` and `canonical_results_json`.
"""

from __future__ import annotations

import hashlib
import json
import math
from collections.abc import Mapping, Sequence
from typing import Final, cast

JsonValue = None | bool | int | float | str | Sequence["JsonValue"] | Mapping[str, "JsonValue"]

NEWLINE: Final = "\n"
FLOAT_DECIMALS: Final = 6
ENCODING: Final = "utf-8"
SEPARATORS: Final = (",", ":")


class CanonicalError(ValueError):
    """A value that has no canonical form."""


def _check_no_floats(value: object, path: str) -> None:
    if isinstance(value, float):
        raise CanonicalError(f"float at {path}: canonical records carry integers and strings only")
    if isinstance(value, Mapping):
        for key, item in cast(Mapping[object, object], value).items():
            if not isinstance(key, str):
                raise CanonicalError(f"non-string key at {path}")
            _check_no_floats(item, f"{path}.{key}")
    elif isinstance(value, (list, tuple)):
        for index, item in enumerate(cast(Sequence[object], value)):
            _check_no_floats(item, f"{path}[{index}]")


def _dump(value: object) -> str:
    return json.dumps(value, sort_keys=True, separators=SEPARATORS, ensure_ascii=False, allow_nan=False) + NEWLINE


def stable_json(value: JsonValue) -> str:
    """Deterministic text of any JSON value, floats included and unrounded; for archives, never for digests."""
    return _dump(value)


def canonical_json(value: JsonValue) -> str:
    """The canonical text of a float-free JSON value."""
    _check_no_floats(value, "$")
    return _dump(value)


def canonical_bytes(value: JsonValue) -> bytes:
    return canonical_json(value).encode(ENCODING)


def round_floats(value: JsonValue) -> JsonValue:
    """Every float rounded to `FLOAT_DECIMALS`, NaN and infinities replaced by null; the single rounding point."""
    if isinstance(value, bool) or value is None or isinstance(value, (int, str)):
        return value
    if isinstance(value, float):
        return round(value, FLOAT_DECIMALS) + 0.0 if math.isfinite(value) else None
    if isinstance(value, Mapping):
        return {key: round_floats(item) for key, item in value.items()}
    return [round_floats(item) for item in value]


def canonical_results_json(value: JsonValue) -> str:
    """Canonical text of an analysis result, whose ratios are floats rounded at one place."""
    return _dump(round_floats(value))


def parse_canonical(text: str) -> JsonValue:
    """The value of a canonical document, refusing text that is not byte-for-byte canonical."""
    value: JsonValue = json.loads(text)
    if canonical_json(value) != text:
        raise CanonicalError("document is valid JSON but not in canonical form")
    return value


def parse_stable_json(text: str) -> JsonValue:
    """The value of a document `stable_json` wrote, refusing text that is not byte-for-byte what it writes."""
    value: JsonValue = json.loads(text)
    if stable_json(value) != text:
        raise CanonicalError("document is valid JSON but not in stable form")
    return value


def sha256_hex(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()
