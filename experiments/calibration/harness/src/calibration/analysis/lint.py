"""`report-lint` (§15.2): the cases in which a report fails."""

from __future__ import annotations

import re
from dataclasses import dataclass
from typing import Final

from calibration.analysis.model import ALIGNMENT_KINDS, Kind
from calibration.analysis.report_text import (
    ADR_PHRASE,
    CONCLUSION_BEGIN,
    CONCLUSION_END,
    EXPLORATORY_LABEL,
    EXPLORATORY_WORD_LIMIT,
    QUOTED_BEGIN,
    QUOTED_END,
    SECTION_PREFIX,
    SECTIONS,
    SOURCE2_NOTE,
    VENDOR_FIGURES,
    VENDOR_MARK,
)
from calibration.domain.models import MODEL_IDS
from calibration.domain.sources import SourceId

FORBIDDEN_WORDS: Final = (
    "works",
    "proves",
    "proven",
    "validated",
    "confirm",
    "generalizes",
    "well-calibrated",
    "human audit",
)
_WORD_BOUNDARY: Final = r"(?<![A-Za-z0-9_-])(?:{})(?![A-Za-z0-9_-])"
_FORBIDDEN: Final = re.compile(_WORD_BOUNDARY.format("|".join(re.escape(w) for w in FORBIDDEN_WORDS)), re.IGNORECASE)
TABLE_PREFIX: Final = "|"
SOURCE2_MARKERS: Final = tuple(source.value for source in (SourceId.JH5, SourceId.JH3, SourceId.VH2))
BANDS_SECTION: Final = "Bands"
VENDOR_SECTION: Final = "Vendor-reported figures"
EXPLORATORY_SECTION: Final = "Exploratory discussion"
RETURNED_PROBABILITY: Final = "returned probability"
SERVER_WORDS: Final = ("Ollama", "Kev")
SEPARATOR_ROW: Final = re.compile(r"^\|[\s:|-]+\|$")


@dataclass(frozen=True)
class LintFinding:
    line: int
    message: str

    def __str__(self) -> str:
        return f"line {self.line}: {self.message}"


def _visible(lines: list[str]) -> list[tuple[int, str]]:
    """The lines outside quoted run text and conclusion sentences, with their numbers."""
    kept: list[tuple[int, str]] = []
    hidden = False
    for number, line in enumerate(lines, start=1):
        stripped = line.strip()
        if stripped in (QUOTED_BEGIN, CONCLUSION_BEGIN):
            hidden = True
        elif stripped in (QUOTED_END, CONCLUSION_END):
            hidden = False
        elif not hidden:
            kept.append((number, line.replace(ADR_PHRASE, "")))
    return kept


def _sections(lines: list[tuple[int, str]]) -> dict[str, list[tuple[int, str]]]:
    sections: dict[str, list[tuple[int, str]]] = {}
    current: list[tuple[int, str]] = []
    for number, line in lines:
        if line.startswith(SECTION_PREFIX):
            title = line.removeprefix(SECTION_PREFIX).strip()
            title = re.sub(r"^\d+\.\s*", "", title)
            current = sections.setdefault(title, [])
        current.append((number, line))
    return sections


def _tables(section: list[tuple[int, str]]) -> list[list[tuple[int, str]]]:
    blocks: list[list[tuple[int, str]]] = []
    block: list[tuple[int, str]] = []
    for number, line in section:
        if line.startswith(TABLE_PREFIX):
            block.append((number, line))
        elif block:
            blocks.append(block)
            block = []
    if block:
        blocks.append(block)
    return blocks


def _cells(row: str) -> list[str]:
    return [cell.strip() for cell in row.strip().strip("|").split("|")]


def expected_band_rows() -> set[tuple[str, str, str]]:
    """Every (kind, source, model) cell the band table must hold: 31 rows."""
    rows: set[tuple[str, str, str]] = set()
    for model in MODEL_IDS:
        rows.update((kind.value, SourceId.S1.value, model) for kind in Kind)
    for source in (SourceId.JH5, SourceId.JH3):
        rows.update((kind.value, source.value, "host") for kind in ALIGNMENT_KINDS)
    rows.add((Kind.K4.value, SourceId.VH2.value, "host"))
    return rows


def _band_table_findings(section: list[tuple[int, str]]) -> list[LintFinding]:
    present: set[tuple[str, str, str]] = set()
    for block in _tables(section):
        for _, row in block:
            if SEPARATOR_ROW.match(row):
                continue
            cells = _cells(row)
            if len(cells) >= 3:
                present.add((cells[0], cells[1], cells[2]))
    start = section[0][0] if section else 1
    return [
        LintFinding(start, f"the band table lacks the cell {' / '.join(missing)}")
        for missing in sorted(expected_band_rows() - present)
    ]


def lint_report(text: str) -> list[LintFinding]:
    """Forbidden words, CONSENSUS without PRIMARY, a returned-probability figure without its server, a table of
    source-(2) figures without the fixed note, a vendor figure outside the vendor table, a missing band-table cell and
    an exploratory discussion beyond 300 words."""
    lines = _visible(text.splitlines())
    findings: list[LintFinding] = []
    for number, line in lines:
        findings += [LintFinding(number, f"forbidden word {m.group(0)!r}") for m in _FORBIDDEN.finditer(line)]
    sections = _sections(lines)
    findings += [LintFinding(1, f"the section {title!r} is missing") for title in SECTIONS if title not in sections]
    for title, section in sections.items():
        findings += _section_findings(title, section)
    if BANDS_SECTION in sections:
        findings += _band_table_findings(sections[BANDS_SECTION])
    return sorted(findings, key=lambda finding: (finding.line, finding.message))


def _section_findings(title: str, section: list[tuple[int, str]]) -> list[LintFinding]:
    findings: list[LintFinding] = []
    body = "\n".join(line for _, line in section)
    for block in _tables(section):
        rows = [row for _, row in block]
        text = "\n".join(rows)
        if "CONSENSUS" in text and "PRIMARY" not in text:
            findings.append(LintFinding(block[0][0], "a CONSENSUS figure appears without its PRIMARY figure"))
        if (
            any(marker in cell for row in rows for cell in _cells(row)[:3] for marker in SOURCE2_MARKERS)
            and SOURCE2_NOTE not in body
        ):
            findings.append(LintFinding(block[0][0], "a table of source-(2) figures lacks the fixed note"))
        for number, row in block:
            if RETURNED_PROBABILITY in row.lower() and not any(server in row for server in SERVER_WORDS):
                findings.append(LintFinding(number, "a returned-probability figure appears without its server"))
    if title != VENDOR_SECTION:
        findings += _vendor_findings(section)
    if title == EXPLORATORY_SECTION:
        findings += _exploratory_findings(section)
    return findings


def _vendor_findings(section: list[tuple[int, str]]) -> list[LintFinding]:
    marks = [figure[2] for figure in VENDOR_FIGURES]
    return [
        LintFinding(number, "a figure of the vendor table appears outside it")
        for number, line in section
        if VENDOR_MARK in line or any(mark in line for mark in marks)
    ]


def _exploratory_findings(section: list[tuple[int, str]]) -> list[LintFinding]:
    body = " ".join(line for _, line in section[1:] if line.strip())
    start = section[0][0]
    findings: list[LintFinding] = []
    if EXPLORATORY_LABEL.lower() not in body.lower():
        findings.append(LintFinding(start, "the discussion is not labelled exploratory"))
    if len(body.split()) > EXPLORATORY_WORD_LIMIT:
        findings.append(LintFinding(start, f"the exploratory discussion exceeds {EXPLORATORY_WORD_LIMIT} words"))
    return findings
