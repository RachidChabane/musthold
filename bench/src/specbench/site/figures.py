"""Figures and the honesty rules that govern how one is shown.

A figure is an estimate read from `results.json`, its interval and the sample it rests on. The rules:

- never an estimate without its interval and its sample size; a figure without an interval is not estimable;
- an undefined estimate is shown as undefined, with its reason;
- a reduction or a difference whose interval contains zero reads "no detectable difference";
- the reference arm reads as the reference, never as a result;
- an interval that collapses to one value warns that the sample, not certainty, produced it;
- fewer repositories than the cluster bootstrap needs for its nominal coverage are flagged on every table.
"""

from __future__ import annotations

import math
from collections.abc import Mapping
from dataclasses import dataclass
from enum import Enum
from typing import Final

MIN_REPOSITORIES_FOR_NOMINAL_COVERAGE: Final = 24
"""ADR 0004: at least 24 repository clusters keep the 95 % percentile cluster bootstrap near its nominal coverage."""
PERCENT: Final = 100.0
SECONDS_PER_MINUTE: Final = 60.0
CALL_DECIMALS: Final = 1
MINUTE_DECIMALS: Final = 1
PERCENT_DECIMALS: Final = 0
MINUS_SIGN: Final = "\u2212"
NARROW_SPACE: Final = "\u202f"
PERCENT_SIGN: Final = f"{NARROW_SPACE}%"
MINUTE_SIGN: Final = f"{NARROW_SPACE}min"


class FigureKind(Enum):
    """The unit of a figure: a rate shown in percent, model calls, or seconds shown in minutes."""

    RATE = "rate"
    CALLS = "calls"
    MINUTES = "minutes"


class Comparison(Enum):
    """How a figure is read against zero: not at all, as a reduction against a reference arm, or as a difference."""

    NONE = "none"
    REDUCTION = "reduction"
    DIFFERENCE = "difference"


class Reading(Enum):
    ESTIMATED = "estimated"
    REFERENCE = "reference"
    UNDEFINED = "undefined"
    NO_DETECTABLE_DIFFERENCE = "no detectable difference"
    ABOVE_ZERO = "above zero"
    BELOW_ZERO = "below zero"


class Caution(Enum):
    DEGENERATE_INTERVAL = "degenerate interval"


@dataclass(frozen=True)
class Estimate:
    """An estimate record of `results.json`: `None` stands for an undefined value."""

    value: float | None
    lo: float | None
    hi: float | None

    @classmethod
    def from_record(cls, record: Mapping[str, object]) -> Estimate:
        return cls(_number(record.get("estimate")), _number(record.get("lo")), _number(record.get("hi")))

    @property
    def is_defined(self) -> bool:
        return self.value is not None and self.lo is not None and self.hi is not None


def _number(value: object) -> float | None:
    if isinstance(value, bool) or not isinstance(value, int | float):
        return None
    number = float(value)
    return number if math.isfinite(number) else None


@dataclass(frozen=True)
class Unit:
    """What a sample counts, named for one and for several."""

    singular: str
    plural: str

    def of(self, n: int) -> str:
        return self.singular if n == 1 else self.plural


@dataclass(frozen=True)
class Sample:
    """What a figure rests on: `n` units (trunks, accepted changes, correct changes, criteria) and their repositories."""

    n: int
    unit: Unit
    repositories: int

    def describe(self) -> str:
        return f"n{NARROW_SPACE}={NARROW_SPACE}{self.n} {self.unit.of(self.n)}"


@dataclass(frozen=True)
class Figure:
    """One figure as `results.json` holds it, at `pointer` (its path in the results, segments joined by `/`)."""

    pointer: str
    kind: FigureKind
    estimate: Estimate
    sample: Sample
    comparison: Comparison = Comparison.NONE
    reference: bool = False
    undefined_reason: str = "not estimable on this sample"


def reading(figure: Figure) -> Reading:
    if figure.reference:
        return Reading.REFERENCE
    estimate = figure.estimate
    if figure.sample.n == 0 or not estimate.is_defined:
        return Reading.UNDEFINED
    if figure.comparison is Comparison.NONE:
        return Reading.ESTIMATED
    lo, hi = estimate.lo, estimate.hi
    if lo is None or hi is None:
        return Reading.UNDEFINED
    if lo <= 0.0 <= hi:
        return Reading.NO_DETECTABLE_DIFFERENCE
    return Reading.ABOVE_ZERO if lo > 0.0 else Reading.BELOW_ZERO


def cautions(figure: Figure) -> tuple[Caution, ...]:
    estimate = figure.estimate
    if reading(figure) in {Reading.UNDEFINED, Reading.REFERENCE}:
        return ()
    if estimate.lo == estimate.hi:
        return (Caution.DEGENERATE_INTERVAL,)
    return ()


def undefined_text(figure: Figure) -> str:
    if figure.sample.n == 0:
        return f"not estimable: no {figure.sample.unit.singular}"
    if figure.estimate.value is not None and not figure.estimate.is_defined:
        return "not estimable: no interval"
    return f"not estimable: {figure.undefined_reason}"


def too_few_repositories(repositories: int) -> bool:
    return repositories < MIN_REPOSITORIES_FOR_NOMINAL_COVERAGE


def _signed(value: float, decimals: int) -> str:
    rounded = round(value, decimals) + 0.0
    text = f"{abs(rounded):.{decimals}f}"
    return f"{MINUS_SIGN}{text}" if rounded < 0 else text


def format_value(kind: FigureKind, value: float) -> str:
    """One value in the unit of its kind: percentages for rates and reductions, calls, minutes."""
    if kind is FigureKind.CALLS:
        return _signed(value, CALL_DECIMALS)
    if kind is FigureKind.MINUTES:
        return f"{_signed(value / SECONDS_PER_MINUTE, MINUTE_DECIMALS)}{MINUTE_SIGN}"
    return f"{_signed(value * PERCENT, PERCENT_DECIMALS)}{PERCENT_SIGN}"


def format_interval(kind: FigureKind, lo: float, hi: float) -> str:
    return f"[{format_value(kind, lo)}, {format_value(kind, hi)}]"


def reading_text(figure: Figure, subject: str) -> str:
    """The plain reading of a figure that compares to zero; `subject` names what it is compared with."""
    match reading(figure):
        case Reading.REFERENCE:
            return f"reference: {subject}"
        case Reading.UNDEFINED:
            return undefined_text(figure)
        case Reading.NO_DETECTABLE_DIFFERENCE:
            return f"no detectable difference from {subject}"
        case Reading.ABOVE_ZERO:
            return f"fewer defects than {subject}" if figure.comparison is Comparison.REDUCTION else f"above {subject}"
        case Reading.BELOW_ZERO:
            return f"more defects than {subject}" if figure.comparison is Comparison.REDUCTION else f"below {subject}"
        case Reading.ESTIMATED:
            return ""


def caution_text(caution: Caution, figure: Figure) -> str:
    match caution:
        case Caution.DEGENERATE_INTERVAL:
            return "single-valued interval: every resample agrees at this sample size, which is not certainty"
