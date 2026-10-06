"""`report-lint` (PREREG 13): forbidden words outside quoted templates; CONSENSUS without PRIMARY."""

from __future__ import annotations

import re
from dataclasses import dataclass

from pilot.domain.analysis_constants import (
    CONSENSUS_WORD,
    EXPLORATORY_WORD_LIMIT,
    FORBIDDEN_WORDS,
    PRIMARY_WORD,
    QUOTED_BEGIN_MARKER,
    QUOTED_END_MARKER,
)

EXPLORATORY_HEADING = re.compile(r"^#+\s.*exploratory", re.IGNORECASE)
HEADING = re.compile(r"^#+\s")
_FORBIDDEN = re.compile(
    r"(?<![A-Za-z0-9_])(" + "|".join(re.escape(w) for w in FORBIDDEN_WORDS) + r")(?![A-Za-z0-9_])", re.IGNORECASE
)


@dataclass(frozen=True)
class LintFinding:
    line: int
    message: str

    def __str__(self) -> str:
        return f"line {self.line}: {self.message}"


def unquoted_lines(text: str) -> list[tuple[int, str]]:
    kept: list[tuple[int, str]] = []
    quoted = False
    for number, line in enumerate(text.splitlines(), start=1):
        if line.strip() == QUOTED_BEGIN_MARKER:
            quoted = True
        elif line.strip() == QUOTED_END_MARKER:
            quoted = False
        elif not quoted:
            kept.append((number, line))
    return kept


def lint_report(text: str) -> list[LintFinding]:
    findings: list[LintFinding] = []
    for number, line in unquoted_lines(text):
        for match in _FORBIDDEN.finditer(line):
            findings.append(LintFinding(number, f"forbidden word {match.group(1)!r}"))
        if CONSENSUS_WORD in line and PRIMARY_WORD not in line:
            findings.append(LintFinding(number, f"{CONSENSUS_WORD} figure without its {PRIMARY_WORD} figure on the same line"))
    findings += _exploratory_length(text)
    return findings


def _exploratory_length(text: str) -> list[LintFinding]:
    words = 0
    inside = False
    start = 0
    for number, line in enumerate(text.splitlines(), start=1):
        if EXPLORATORY_HEADING.match(line):
            inside, start = True, number
        elif inside and HEADING.match(line):
            inside = False
        elif inside:
            words += len(line.split())
    if words > EXPLORATORY_WORD_LIMIT:
        return [LintFinding(start, f"exploratory section has {words} words, limit {EXPLORATORY_WORD_LIMIT}")]
    return []
