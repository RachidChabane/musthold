"""`report-lint`: forbidden words outside quoted passages, a sample line and the descriptive label before every table,
and no figure across harnesses."""

from __future__ import annotations

import re
from dataclasses import dataclass

from specbench.domain.reporting import (
    DESCRIPTIVE_LABEL,
    FORBIDDEN_WORDS,
    POOLED_WORD,
    QUOTED_BEGIN_MARKER,
    QUOTED_END_MARKER,
    SAMPLE_LINE_PREFIX,
    SECTION_PREFIX,
    TABLE_ROW_PREFIX,
)

_WORD_BOUNDARY = r"(?<![A-Za-z0-9_])(?:{})(?![A-Za-z0-9_])"
_FORBIDDEN = re.compile(_WORD_BOUNDARY.format("|".join(re.escape(w) for w in FORBIDDEN_WORDS)), re.IGNORECASE)
_POOLED = re.compile(_WORD_BOUNDARY.format(re.escape(POOLED_WORD)), re.IGNORECASE)


@dataclass(frozen=True)
class LintFinding:
    line: int
    message: str

    def __str__(self) -> str:
        return f"line {self.line}: {self.message}"


def _unquoted(text: str) -> list[tuple[int, str]]:
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


def _sections(lines: list[tuple[int, str]]) -> list[list[tuple[int, str]]]:
    sections: list[list[tuple[int, str]]] = [[]]
    for number, line in lines:
        if line.startswith(SECTION_PREFIX):
            sections.append([])
        sections[-1].append((number, line))
    return sections


def lint_report(text: str) -> list[LintFinding]:
    lines = _unquoted(text)
    findings: list[LintFinding] = []
    for number, line in lines:
        findings += [LintFinding(number, f"forbidden word {m.group(0)!r}") for m in _FORBIDDEN.finditer(line)]
        findings += [LintFinding(number, f"figure across harnesses ({POOLED_WORD})") for _ in _POOLED.finditer(line)]
    for section in _sections(lines):
        first_table = next((number for number, line in section if line.startswith(TABLE_ROW_PREFIX)), None)
        if first_table is None:
            continue
        sample = next((line for _, line in section if line.startswith(SAMPLE_LINE_PREFIX)), None)
        if sample is None:
            findings.append(LintFinding(first_table, "table without a sample line in its section"))
        elif DESCRIPTIVE_LABEL not in sample:
            findings.append(LintFinding(first_table, f"sample line is not labelled {DESCRIPTIVE_LABEL}"))
    return findings
