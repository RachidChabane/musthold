"""The blind-spot estimand (ADR 0006, PREREG 11): do the writer and the mutant source share the same blind spots?

Truth is model-free: a mutant is evaluated only when the oracle says it violates the criterion. A
mutant escapes a suite when no test of the suite has a reproduced kill. Everything here is a pure
function of per-cell escape counts, so the estimand is testable without a run.
"""

from __future__ import annotations

import math
from collections.abc import Callable, Mapping, Sequence
from dataclasses import dataclass
from enum import Enum
from typing import Any, Final

import numpy as np

from pilot.analysis.settings import AnalysisSettings
from pilot.analysis.stats import FloatArray, Interval, bootstrap_interval
from pilot.domain import analysis_constants as k
from pilot.domain.model_slate import MutantSource, Writer

NAN: Final = float("nan")
WRITERS: Final = (Writer.SONNET, Writer.OPUS)
SOURCES: Final = (MutantSource.SONNET, MutantSource.OPUS, MutantSource.OPERATOR)
CELL_ORDER: Final = tuple((writer, source) for writer in WRITERS for source in SOURCES)
CLUSTER_COLUMNS: Final = len(CELL_ORDER)
COUNT_COLUMN: Final = CLUSTER_COLUMNS
CELL_INDEX: Final = {cell: index for index, cell in enumerate(CELL_ORDER)}
KILLED: Final = k.KILLED_RESULT
ERROR: Final = k.ERROR_RESULT

Cell = tuple[Writer, MutantSource]


class Escape(Enum):
    ESCAPED = "escaped"
    CAUGHT = "caught"
    EXCLUDED = "excluded"


def escape_outcome(results: Sequence[str | None]) -> Escape:
    """One (mutant, writer) pair against the suite's tests; a missing result counts as an error.

    Caught: some test has a reproduced kill. Excluded: no reproduced kill and an error on some test.
    Escaped: every test survived or was flaky.
    """
    if any(result == KILLED for result in results):
        return Escape.CAUGHT
    if any(result is None or result == ERROR for result in results):
        return Escape.EXCLUDED
    return Escape.ESCAPED


@dataclass(frozen=True)
class CellCount:
    escaped: int = 0
    caught: int = 0
    excluded: int = 0

    @property
    def universe(self) -> int:
        return self.escaped + self.caught + self.excluded

    @property
    def evaluable(self) -> int:
        return self.escaped + self.caught

    @property
    def rate(self) -> float:
        return self.escaped / self.evaluable if self.evaluable else NAN

    def plus(self, other: CellCount) -> CellCount:
        return CellCount(self.escaped + other.escaped, self.caught + other.caught, self.excluded + other.excluded)


def count_outcomes(outcomes: Sequence[Escape]) -> CellCount:
    return CellCount(
        sum(o is Escape.ESCAPED for o in outcomes), sum(o is Escape.CAUGHT for o in outcomes),
        sum(o is Escape.EXCLUDED for o in outcomes),
    )


@dataclass(frozen=True)
class Exclusion:
    criterion: int
    reasons: tuple[str, ...]


Universes = Mapping[tuple[MutantSource, int], int]


def common_set(
    cells: Mapping[tuple[Writer, MutantSource, int], CellCount | None],
    criteria: Sequence[int],
    universes: Universes | None = None,
) -> tuple[tuple[int, ...], tuple[Exclusion, ...]]:
    """C*: criteria with both suites non-empty, a non-empty universe for every source and an evaluable pair in every cell.

    A cell whose suite is empty has no count (None). Exclusions carry every reason, per criterion.
    """
    included: list[int] = []
    excluded: list[Exclusion] = []
    for criterion in criteria:
        reasons = _exclusion_reasons(cells, criterion, universes)
        if reasons:
            excluded.append(Exclusion(criterion, tuple(reasons)))
        else:
            included.append(criterion)
    return tuple(included), tuple(excluded)


def _exclusion_reasons(
    cells: Mapping[tuple[Writer, MutantSource, int], CellCount | None], criterion: int, universes: Universes | None
) -> list[str]:
    reasons = [f"T_{w.value} is empty after the reference filter" for w in WRITERS if all(cells.get((w, s, criterion)) is None for s in SOURCES)]
    for source in SOURCES:
        if universes is not None:
            empty = universes.get((source, criterion), 0) == 0
        else:
            empty = not any(c.universe for w in WRITERS if (c := cells.get((w, source, criterion))) is not None)
        if empty:
            reasons.append(f"U({source.value}) is empty")
    for writer, source in CELL_ORDER:
        cell = cells.get((writer, source, criterion))
        if cell is not None and cell.universe and not cell.evaluable:
            reasons.append(f"every pair of ({writer.value}, {source.value}) is excluded")
    return reasons


def criterion_rates(cells: Mapping[tuple[Writer, MutantSource, int], CellCount | None], criterion: int) -> list[float]:
    """e_c(W, M) of the six primary cells in CELL_ORDER."""
    rates: list[float] = []
    for writer, source in CELL_ORDER:
        cell = cells[(writer, source, criterion)]
        assert cell is not None
        rates.append(cell.rate)
    return rates


def cell_means(rows: FloatArray) -> FloatArray:
    """e(W, M): the mean over criteria of e_c(W, M), each criterion weighing 1; rows hold one criterion each."""
    return rows[:, :CLUSTER_COLUMNS].mean(axis=0) if len(rows) else np.full(CLUSTER_COLUMNS, NAN)


@dataclass(frozen=True)
class Contrasts:
    delta_s: float
    delta_o: float
    sb_s: float
    sb_o: float
    sb: float


def contrasts(e: Mapping[Cell, float]) -> Contrasts:
    """Δ_W = e(W,W) − e(W,op); SB_S = Δ_S − [e(O,S) − e(O,op)], SB_O symmetric; SB = ½(SB_S + SB_O)."""
    s, o, op = MutantSource.SONNET, MutantSource.OPUS, MutantSource.OPERATOR
    ws, wo = Writer.SONNET, Writer.OPUS
    delta_s = e[(ws, s)] - e[(ws, op)]
    delta_o = e[(wo, o)] - e[(wo, op)]
    sb_s = delta_s - (e[(wo, s)] - e[(wo, op)])
    sb_o = delta_o - (e[(ws, o)] - e[(ws, op)])
    return Contrasts(delta_s, delta_o, sb_s, sb_o, 0.5 * (sb_s + sb_o))


def primary_statistic(sums: FloatArray) -> FloatArray:
    """SB over resampled clusters: the cell means come from the summed rates and the criterion count."""
    means = sums[:, :CLUSTER_COLUMNS] / sums[:, COUNT_COLUMN : COUNT_COLUMN + 1]
    index = CELL_INDEX
    ws, wo = Writer.SONNET, Writer.OPUS
    s, o = MutantSource.SONNET, MutantSource.OPUS
    return 0.5 * (
        means[:, index[(ws, s)]] + means[:, index[(wo, o)]] - means[:, index[(ws, o)]] - means[:, index[(wo, s)]]
    )


def reading(interval: Interval) -> str:
    """Fixed in advance; the first rule that holds wins, so a positive lower bound reads as indicated. The caller
    reads "undetermined" below the minimum number of criteria."""
    if not interval.is_defined:
        return k.BLIND_SPOT_UNDETERMINED
    if interval.lo > 0:
        return k.BLIND_SPOT_INDICATED
    if interval.hi < k.BLIND_SPOT_MATERIALITY:
        return k.BLIND_SPOT_NONE
    return k.BLIND_SPOT_UNDETERMINED


@dataclass(frozen=True)
class BlindSpotResult:
    c_star: tuple[int, ...]
    exclusions: tuple[Exclusion, ...]
    means: Mapping[Cell, float]
    pooled: Mapping[Cell, CellCount]
    contrasts: Contrasts | None
    interval: Interval
    reading: str
    indicated_below_materiality: bool

    def to_mapping(self) -> dict[str, Any]:
        found = self.contrasts
        return {
            "c_star": [f"PARK-{c}~1" for c in self.c_star],
            "exclusions": {f"PARK-{e.criterion}~1": list(e.reasons) for e in self.exclusions},
            "e": {f"{w.value}|{s.value}": _clean(v) for (w, s), v in self.means.items()},
            "pooled_counts": {f"{w.value}|{s.value}": [c.escaped, c.caught, c.excluded] for (w, s), c in self.pooled.items()},
            "delta_S": _clean(found.delta_s) if found else None,
            "delta_O": _clean(found.delta_o) if found else None,
            "SB_S": _clean(found.sb_s) if found else None,
            "SB_O": _clean(found.sb_o) if found else None,
            "SB": _clean(found.sb) if found else None,
            "interval": [_clean(self.interval.lo), _clean(self.interval.hi)],
            "reading": self.reading,
            "indicated_below_materiality": self.indicated_below_materiality,
        }


def _clean(value: float) -> float | None:
    return None if math.isnan(value) else value


def estimate(
    cells: Mapping[tuple[Writer, MutantSource, int], CellCount | None],
    criteria: Sequence[int],
    settings: AnalysisSettings,
    universes: Universes | None = None,
) -> BlindSpotResult:
    """The primary estimand over C*, its percentile cluster bootstrap and the three-way reading."""
    c_star, exclusions = common_set(cells, criteria, universes)
    pooled = {
        cell: _pool([count for c in c_star if (count := cells.get((*cell, c))) is not None]) for cell in CELL_ORDER
    }
    if not c_star:
        undefined = {cell: NAN for cell in CELL_ORDER}
        return BlindSpotResult(c_star, exclusions, undefined, pooled, None, Interval(NAN, NAN), k.BLIND_SPOT_UNDETERMINED, False)
    rows = np.array([[*criterion_rates(cells, c), 1.0] for c in c_star], dtype=np.float64)
    means = dict(zip(CELL_ORDER, cell_means(rows).tolist(), strict=True))
    interval = bootstrap_interval(rows, primary_statistic, settings.resamples, settings.seed)
    found = reading(interval) if len(c_star) >= k.BLIND_SPOT_MIN_CRITERIA else k.BLIND_SPOT_UNDETERMINED
    return BlindSpotResult(
        c_star, exclusions, means, pooled, contrasts(means), interval, found,
        found == k.BLIND_SPOT_INDICATED and interval.hi < k.BLIND_SPOT_MATERIALITY,
    )


def _pool(counts: Sequence[CellCount]) -> CellCount:
    total = CellCount()
    for count in counts:
        total = total.plus(count)
    return total


Hosts = Callable[[str], tuple[str, ...]]


@dataclass(frozen=True)
class LocationSplit:
    """Escaping model mutants whose edited definition also hosts an escaping operator mutant, versus not."""

    writer: Writer
    source: MutantSource
    escaping: int
    coverage_hole: int

    @property
    def model_specific(self) -> int:
        return self.escaping - self.coverage_hole


def location_split(
    writer: Writer,
    source: MutantSource,
    escaping_model: Mapping[int, Sequence[str]],
    escaping_operator: Mapping[int, Sequence[str]],
    hosts: Hosts,
    criteria: Sequence[int],
) -> LocationSplit:
    """Per criterion, a model mutant is a coverage hole when an escaping operator mutant edits a definition it edits."""
    escaping = holes = 0
    for criterion in criteria:
        operator_hosts = {host for mutant in escaping_operator.get(criterion, ()) for host in hosts(mutant)}
        for mutant in escaping_model.get(criterion, ()):
            escaping += 1
            holes += int(bool(operator_hosts & set(hosts(mutant))))
    return LocationSplit(writer, source, escaping, holes)
