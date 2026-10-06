"""Criterion references: the `PARK-n~1` text form and the integer used inside the analysis."""

from __future__ import annotations

import re

from pilot.domain.analysis_constants import CRITERION_FORMAT
from pilot.domain.constants import CRITERION_REF_PATTERN

_REF = re.compile(CRITERION_REF_PATTERN)


def criterion_ref(number: int) -> str:
    return CRITERION_FORMAT.format(number=number)


def parse_criterion(ref: str) -> int:
    match = _REF.match(ref)
    if match is None:
        raise ValueError(f"not a criterion reference: {ref!r}")
    return int(match.group(1))


def parse_criterion_or_none(ref: str | None) -> int | None:
    if ref is None or _REF.match(ref) is None:
        return None
    return parse_criterion(ref)
