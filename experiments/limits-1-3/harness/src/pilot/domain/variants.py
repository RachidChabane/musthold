"""Variants of the fixture: violations, planted items and the refactor."""

from __future__ import annotations

import re
from dataclasses import dataclass
from enum import Enum

from pilot.domain.constants import REFACTOR_VARIANT_ID

_VIOLATION_PATTERN = re.compile(r"^PARK-(\d+)_([abc])$")
_PLANTED_PATTERN = re.compile(r"^PARK-(\d+)_n1$")


class VariantKind(Enum):
    VIOLATION = "violation"
    PLANTED = "planted"
    REFACTOR = "refactor"


@dataclass(frozen=True)
class VariantId:
    value: str
    kind: VariantKind
    criterion: int | None

    @classmethod
    def parse(cls, value: str) -> VariantId:
        if value == REFACTOR_VARIANT_ID:
            return cls(value, VariantKind.REFACTOR, None)
        for pattern, kind in (
            (_VIOLATION_PATTERN, VariantKind.VIOLATION),
            (_PLANTED_PATTERN, VariantKind.PLANTED),
        ):
            match = pattern.match(value)
            if match:
                return cls(value, kind, int(match.group(1)))
        raise ValueError(f"unrecognised variant id: {value!r}")

    @classmethod
    def parse_or_none(cls, value: str) -> VariantId | None:
        try:
            return cls.parse(value)
        except ValueError:
            return None

    @property
    def sort_key(self) -> tuple[int, int, str]:
        return (list(VariantKind).index(self.kind), self.criterion or 0, self.value)
