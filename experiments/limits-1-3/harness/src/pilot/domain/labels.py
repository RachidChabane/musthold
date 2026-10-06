"""Test labels (PREREG 2.3) as the label file states them."""

from __future__ import annotations

import re
from collections.abc import Mapping
from dataclasses import dataclass
from enum import Enum
from typing import Any

from pilot.domain.constants import CRITERION_ID_PATTERN

_CLASS_PATTERN = re.compile(r"^([A-Z_]+)")
_PARENTHESIS_PATTERN = re.compile(r"\(([^)]*)\)")
_NUMBER_PATTERN = re.compile(r"\d+")
_CRITERION_PATTERN = re.compile(CRITERION_ID_PATTERN)


class TestClass(Enum):
    __test__ = False

    GOOD = "GOOD"
    GOOD_BROAD = "GOOD_BROAD"
    WEAK = "WEAK"
    VACUOUS = "VACUOUS"
    MIS_OTHER = "MIS_OTHER"
    OFFSPEC = "OFFSPEC"

    @property
    def is_gold_good(self) -> bool:
        return self in (TestClass.GOOD, TestClass.GOOD_BROAD)


@dataclass(frozen=True)
class TestLabel:
    __test__ = False

    nodeid: str
    cites: int
    test_class: TestClass
    other: int | None
    shape: str
    checks: str
    alignment_truth: str

    @classmethod
    def from_mapping(cls, raw: Mapping[str, Any]) -> TestLabel:
        cites = _first_criterion(str(raw["cites"]))
        if cites is None:
            raise ValueError(f"no criterion in cites: {raw['cites']!r}")
        label = str(raw["label"])
        checks = str(raw.get("checks", ""))
        test_class = _parse_class(label)
        return cls(
            nodeid=str(raw["nodeid"]),
            cites=cites,
            test_class=test_class,
            other=_other_criterion(test_class, label, checks, cites),
            shape=str(raw.get("shape", "")),
            checks=checks,
            alignment_truth=str(raw.get("alignment_truth", "")),
        )


def _parse_class(label: str) -> TestClass:
    match = _CLASS_PATTERN.match(label.strip())
    if not match:
        raise ValueError(f"unrecognised label: {label!r}")
    return TestClass(match.group(1))


def _first_criterion(text: str) -> int | None:
    numbers = _numbers(text)
    return numbers[0] if numbers else None


def _other_criterion(
    test_class: TestClass, label: str, checks: str, cites: int
) -> int | None:
    if test_class not in (TestClass.GOOD_BROAD, TestClass.MIS_OTHER):
        return None
    inside = _PARENTHESIS_PATTERN.search(label)
    candidates = _numbers(inside.group(1)) if inside else _numbers(checks)
    others = [n for n in candidates if n != cites]
    if not others:
        raise ValueError(f"{test_class.value} label names no second criterion: {label!r}")
    return others[0]


def _numbers(text: str) -> list[int]:
    ids = [int(m) for m in _CRITERION_PATTERN.findall(text)]
    return ids or [int(m) for m in _NUMBER_PATTERN.findall(text)]
