"""Intervals, proportions with counts and the serialisable estimate record."""

from __future__ import annotations

import math
from dataclasses import dataclass
from typing import Final

from calibration.analysis.stats.arrays import NAN, json_number, safe_ratio
from calibration.domain.constants import WILSON_Z

WILSON_METHOD: Final = "wilson"
BOOTSTRAP_METHOD: Final = "cluster bootstrap"
DECISION_METHOD: Final = "decision interval"


@dataclass(frozen=True)
class Interval:
    lo: float
    hi: float

    @property
    def is_defined(self) -> bool:
        return not (math.isnan(self.lo) or math.isnan(self.hi))


UNDEFINED_INTERVAL: Final = Interval(NAN, NAN)
UNIT_INTERVAL: Final = Interval(0.0, 1.0)


def wilson(successes: int, total: int) -> Interval:
    """Wilson score interval at 95 %; the unit interval when nothing was observed."""
    if total <= 0:
        return UNIT_INTERVAL
    point = successes / total
    z_squared = WILSON_Z**2
    scale = 1.0 + z_squared / total
    centre = (point + z_squared / (2 * total)) / scale
    half = WILSON_Z * math.sqrt(point * (1.0 - point) / total + z_squared / (4 * total**2)) / scale
    return Interval(min(max(0.0, centre - half), point), max(min(1.0, centre + half), point))


def decision_interval(wilson_interval: Interval, bootstrap_interval: Interval) -> Interval:
    """The wider of the two intervals; undefined as soon as one of them is."""
    if not (wilson_interval.is_defined and bootstrap_interval.is_defined):
        return UNDEFINED_INTERVAL
    return Interval(min(wilson_interval.lo, bootstrap_interval.lo), max(wilson_interval.hi, bootstrap_interval.hi))


@dataclass(frozen=True)
class Estimate:
    point: float
    lo: float
    hi: float
    method: str

    def to_record(self) -> dict[str, float | str | None]:
        return {
            "point": json_number(self.point),
            "lo": json_number(self.lo),
            "hi": json_number(self.hi),
            "method": self.method,
        }


@dataclass(frozen=True)
class Proportion:
    """A count of successes among a count of units; its point estimate is undefined over zero units."""

    successes: int
    total: int

    @property
    def point(self) -> float:
        return safe_ratio(self.successes, self.total)

    @property
    def wilson(self) -> Interval:
        return wilson(self.successes, self.total)

    def estimate(self) -> Estimate:
        if self.total == 0:
            return Estimate(NAN, NAN, NAN, WILSON_METHOD)
        interval = self.wilson
        return Estimate(self.point, interval.lo, interval.hi, WILSON_METHOD)
