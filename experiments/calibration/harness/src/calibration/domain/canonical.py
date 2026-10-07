"""The byte-level serialization of every record and digest the harness writes."""

from __future__ import annotations

import hashlib
import json
import math
import re
from collections.abc import Mapping, Sequence
from typing import Final

from calibration.domain.constants import ENCODING, FLOAT_DECIMALS

JsonValue = None | bool | int | float | str | Sequence["JsonValue"] | Mapping[str, "JsonValue"]

COMPACT_SEPARATORS: Final = (",", ":")
DOCUMENT_INDENT: Final = 2
NEWLINE: Final = "\n"
_SLUG_FORBIDDEN: Final = re.compile(r"[^A-Za-z0-9_.-]")
SLUG_REPLACEMENT: Final = "_"


def canonical_json(value: JsonValue) -> str:
    """Compact text with sorted keys and UTF-8 characters; the form that is digested."""
    return json.dumps(value, sort_keys=True, separators=COMPACT_SEPARATORS, ensure_ascii=False, allow_nan=False)


def document_json(value: JsonValue) -> str:
    """The text of a committed document: sorted keys, two-space indent and one final newline."""
    return json.dumps(value, sort_keys=True, indent=DOCUMENT_INDENT, ensure_ascii=False, allow_nan=False) + NEWLINE


def line_json(value: JsonValue) -> str:
    """One line of a `.jsonl` file."""
    return canonical_json(value) + NEWLINE


def round_floats(value: JsonValue) -> JsonValue:
    """Every float rounded to `FLOAT_DECIMALS`; NaN and infinities become null. The one rounding point of a result."""
    if isinstance(value, bool) or value is None or isinstance(value, int | str):
        return value
    if isinstance(value, float):
        return round(value, FLOAT_DECIMALS) + 0.0 if math.isfinite(value) else None
    if isinstance(value, Mapping):
        return {key: round_floats(item) for key, item in value.items()}
    return [round_floats(item) for item in value]


def result_json(value: JsonValue) -> str:
    """The text of an analysis document: `document_json` of the rounded value."""
    return document_json(round_floats(value))


def sha256_hex(data: bytes | str) -> str:
    raw = data.encode(ENCODING) if isinstance(data, str) else data
    return hashlib.sha256(raw).hexdigest()


def slugify(text: str) -> str:
    """Every character outside `[A-Za-z0-9_.-]` becomes `_`; the node-id slug of the pilot's run records."""
    return _SLUG_FORBIDDEN.sub(SLUG_REPLACEMENT, text)
