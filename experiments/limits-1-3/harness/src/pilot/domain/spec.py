"""Requirements parsed from the Shallnot Markdown spec, and the statement digest (ADR 0001)."""

from __future__ import annotations

import re
import unicodedata
from dataclasses import dataclass

from pilot.domain.ids import sha256_hex

_REQUIREMENT_LINE = re.compile(r"^- (?P<id>[A-Z]+-\d+)~(?P<revision>\d+): (?P<text>.*)$")
_CONTINUATION_LINE = re.compile(r"^\s+\S")
_WHITESPACE = re.compile(r"\s+")


@dataclass(frozen=True)
class Criterion:
    id: str
    revision: int
    statement: str

    @property
    def ref(self) -> str:
        return f"{self.id}~{self.revision}"

    @property
    def number(self) -> int:
        return int(self.id.rsplit("-", 1)[1])


def canonical_statement(text: str) -> str:
    """Canonical form c1: NFC, LF line endings, joined continuation lines, collapsed whitespace, trimmed."""
    normalized = unicodedata.normalize("NFC", text).replace("\r\n", "\n").replace("\r", "\n")
    return _WHITESPACE.sub(" ", normalized).strip()


def statement_digest(statement: str) -> str:
    return sha256_hex(canonical_statement(statement))


def parse_spec(text: str) -> tuple[Criterion, ...]:
    criteria: list[Criterion] = []
    current: tuple[str, int, list[str]] | None = None
    for line in text.splitlines():
        match = _REQUIREMENT_LINE.match(line)
        if match:
            current = _flush(criteria, current)
            current = (match["id"], int(match["revision"]), [match["text"]])
        elif current is not None and _CONTINUATION_LINE.match(line):
            current[2].append(line)
        else:
            current = _flush(criteria, current)
    _flush(criteria, current)
    return tuple(sorted(criteria, key=lambda c: (c.number, c.revision)))


def _flush(
    criteria: list[Criterion], current: tuple[str, int, list[str]] | None
) -> None:
    if current is not None:
        identifier, revision, parts = current
        criteria.append(Criterion(identifier, revision, canonical_statement(" ".join(parts))))
    return None
