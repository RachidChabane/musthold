"""Result records of the analysis: one endpoint, its attribution and the n needed to decide it."""

from __future__ import annotations

import math
from collections.abc import Callable, Mapping
from dataclasses import dataclass, field, replace
from typing import Any

from pilot.analysis.bands import Outcome
from pilot.analysis.stats import Interval, wilson
from pilot.domain.analysis_constants import (
    N_NEEDED_BLOCKED,
    N_NEEDED_SEARCH_LIMIT,
    N_NEEDED_UNREACHABLE,
)

OutcomeAt = Callable[[int], Outcome]
DENSE_SEARCH_SPAN = 2000
GEOMETRIC_STEP = 1.03


@dataclass(frozen=True)
class EndpointResult:
    id: str
    limit: int
    measure: str
    n: int
    point: float
    lo: float
    hi: float
    band_go: str
    band_nogo: str
    outcome: Outcome
    uncapped_outcome: Outcome
    capped_by: tuple[str, ...] = ()
    attribution: Mapping[str, Any] | None = None
    components: tuple[Mapping[str, Any], ...] = ()
    details: Mapping[str, Any] = field(default_factory=dict)
    margin: float = 0.0
    outcome_at: OutcomeAt | None = None

    def with_cap(self, outcome: Outcome, capped_by: tuple[str, ...]) -> EndpointResult:
        return replace(self, outcome=outcome, capped_by=capped_by)

    def with_attribution(self, attribution: Mapping[str, Any]) -> EndpointResult:
        return replace(self, attribution=attribution)

    def to_mapping(self) -> dict[str, Any]:
        return {
            "id": self.id,
            "limit": self.limit,
            "measure": self.measure,
            "n": self.n,
            "point": _clean(self.point),
            "lo": _clean(self.lo),
            "hi": _clean(self.hi),
            "band_go": self.band_go,
            "band_nogo": self.band_nogo,
            "outcome": self.outcome.value,
            "outcome_before_caps": self.uncapped_outcome.value,
            "capped_by": list(self.capped_by),
            "attribution": self.attribution,
            "components": [dict(c) for c in self.components],
            "details": dict(self.details),
        }


def _clean(value: float) -> float | None:
    return None if math.isnan(value) else value


def search_n_needed(observed_n: int, outcome_at: OutcomeAt | None) -> int | str:
    """Smallest n at or above the observed n whose interval would decide the endpoint."""
    if outcome_at is None:
        return N_NEEDED_UNREACHABLE
    n = max(observed_n, 1)
    while n <= N_NEEDED_SEARCH_LIMIT:
        if outcome_at(n) is not Outcome.INCONCLUSIVE:
            return n
        step = 1 if n < observed_n + DENSE_SEARCH_SPAN else max(1, int(n * (GEOMETRIC_STEP - 1)))
        n += step
    return N_NEEDED_UNREACHABLE


def rescaled_wilson(point: float, n: int) -> Interval:
    return wilson(round(point * n), n)


def scaled_interval(point: float, interval: Interval, observed_n: int, n: int) -> Interval:
    """Interval whose distance from the point shrinks with the square root of n."""
    factor = math.sqrt(observed_n / n)
    return Interval(point - (point - interval.lo) * factor, point + (interval.hi - point) * factor)


def n_needed_text(result: EndpointResult) -> int | str:
    if result.capped_by:
        return N_NEEDED_BLOCKED
    return search_n_needed(result.n, result.outcome_at)
