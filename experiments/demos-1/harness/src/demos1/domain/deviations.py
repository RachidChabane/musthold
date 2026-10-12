"""PREREG 14: one entry of DEVIATIONS.md, and the counts per kind the summary gives."""

from __future__ import annotations

import re
from collections import Counter
from dataclasses import dataclass
from datetime import datetime
from enum import StrEnum
from typing import Final

from demos1.domain.window import format_utc

NO_ATTEMPT: Final = "none"
_HEADING = re.compile(r"^## (\S+) · (\S+) · (\S+)$", re.MULTILINE)
FILE_TITLE: Final = "# Deviations of demos-1\n"


class DeviationKind(StrEnum):
    DEPARTURE = "departure"
    """Any departure from PREREG 5 to 13."""
    TOOL_VERSION = "tool-version"
    """A tool version, engine setting or image digest that differs from PREREG 2 or from the manifest."""
    PRODUCT_CHANGE = "product-change"
    SELECT_STOPPED = "select-stopped"
    WAIT_BOUND = "wait-bound"
    UNDEFINED_STAGE = "undefined-stage"


@dataclass(frozen=True)
class Deviation:
    at: datetime
    kind: DeviationKind
    attempt: str | None
    document_says: str
    happened: str

    def render(self) -> str:
        return (
            f"## {format_utc(self.at)} · {self.kind.value} · {self.attempt or NO_ATTEMPT}\n\n"
            f"- The pre-registration says: {self.document_says}\n"
            f"- What happened: {self.happened}\n\n"
        )


def counts_by_kind(document: str) -> dict[str, int]:
    """The number of entries of each kind, every kind present with zero when it has none."""
    counted = Counter(match.group(2) for match in _HEADING.finditer(document))
    return {kind.value: counted.get(kind.value, 0) for kind in DeviationKind}
