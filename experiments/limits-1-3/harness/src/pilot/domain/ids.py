"""Identifiers: mutant ids (RUN_RECORDS) and node-id slugs."""

from __future__ import annotations

import hashlib
import re
import textwrap
from typing import Any

import rfc8785

from pilot.domain.edits import EditSet
from pilot.domain.normalization import normalize
from pilot.domain.pipeline_constants import MUTANT_ID_LENGTH

_SLUG_FORBIDDEN = re.compile(r"[^A-Za-z0-9_.-]")


def slugify(text: str) -> str:
    return _SLUG_FORBIDDEN.sub("_", text)


def canonical_json(value: Any) -> bytes:
    return rfc8785.dumps(value)


def sha256_hex(data: bytes | str) -> str:
    raw = data.encode("utf-8") if isinstance(data, str) else data
    return hashlib.sha256(raw).hexdigest()


def normalized_edit_set(edit_set: EditSet) -> dict[str, Any]:
    """Edit set with every `new_source` normalized, edits ordered by (file, qualname)."""
    edits = sorted(edit_set.edits, key=lambda e: (e.file, e.qualname))
    return {
        "edits": [
            {"file": e.file, "qualname": e.qualname, "new_source": _normalized_source(e.new_source)}
            for e in edits
        ]
    }


def _normalized_source(source: str) -> str:
    dedented = textwrap.dedent(source).strip("\n")
    try:
        return normalize(dedented)
    except SyntaxError:
        return dedented


def mutant_digest(edit_set: EditSet) -> str:
    return sha256_hex(canonical_json(normalized_edit_set(edit_set)))


def mutant_id(edit_set: EditSet) -> str:
    return mutant_digest(edit_set)[:MUTANT_ID_LENGTH]
