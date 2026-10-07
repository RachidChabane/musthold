"""Vocabulary of the report and of its lint: what a sample report may and may not say."""

from __future__ import annotations

from typing import Final

DESCRIPTIVE_LABEL: Final = "descriptive"
SAMPLE_LINE_PREFIX: Final = "Sample: "
QUOTED_BEGIN_MARKER: Final = "<!-- quoted:begin -->"
QUOTED_END_MARKER: Final = "<!-- quoted:end -->"
POOLED_WORD: Final = "pooled"
TABLE_ROW_PREFIX: Final = "|"
SECTION_PREFIX: Final = "## "
FORBIDDEN_WORDS: Final = (
    "significant",
    "significantly",
    "proves",
    "proved",
    "proven",
    "demonstrates",
    "demonstrated",
    "confirms",
    "confirmed",
    "establishes",
    "conclusive",
    "p-value",
    "validated",
)
