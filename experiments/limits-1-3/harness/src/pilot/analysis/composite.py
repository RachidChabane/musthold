"""Precondition caps and the composite verdict (PREREG 10)."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass

from pilot.analysis.bands import Outcome
from pilot.analysis.preconditions import Precondition
from pilot.analysis.results import EndpointResult, n_needed_text
from pilot.domain import analysis_constants as k


def apply_preconditions(
    results: Sequence[EndpointResult], preconditions: Sequence[Precondition]
) -> list[EndpointResult]:
    """A failed or unevaluable precondition caps its endpoints at INCONCLUSIVE.

    The cap only removes GO; NO-GO stands unless the precondition invalidates the whole
    measurement (`forces_inconclusive`), in which case the endpoint is INCONCLUSIVE.
    """
    capped: list[EndpointResult] = []
    for result in results:
        failing = [p for p in preconditions if not p.holds and result.id in p.affects]
        if not failing:
            capped.append(result)
            continue
        forced = any(p.forces_inconclusive for p in failing)
        outcome = Outcome.INCONCLUSIVE if forced or result.outcome is Outcome.GO else result.outcome
        capped.append(result.with_cap(outcome, tuple(dict.fromkeys(p.id for p in failing))))
    return capped


@dataclass(frozen=True)
class Composite:
    verdict: str
    components: tuple[str, ...]
    endpoints: tuple[str, ...]
    n_needed: Mapping[str, int | str]

    @property
    def text(self) -> str:
        if self.verdict == k.COMPOSITE_REDESIGN:
            return f"{k.COMPOSITE_REDESIGN}({', '.join(self.components)})"
        if self.verdict == k.COMPOSITE_INCONCLUSIVE:
            needed = ", ".join(f"{e}: {self.n_needed[e]}" for e in self.endpoints)
            return f"{k.COMPOSITE_INCONCLUSIVE}({', '.join(self.endpoints)}; n_needed {{{needed}}})"
        return self.verdict

    def to_mapping(self) -> dict[str, object]:
        return {
            "verdict": self.verdict,
            "text": self.text,
            "components": list(self.components),
            "endpoints": list(self.endpoints),
            "n_needed": dict(self.n_needed),
        }


def named_component(result: EndpointResult) -> str:
    """The component a NO-GO endpoint points at: the substitution result, else the fallback map."""
    if result.attribution is not None and result.id in k.GATE_ENDPOINTS:
        return str(result.attribution["component"])
    return k.FALLBACK_COMPONENT[result.id]


def composite_verdict(results: Sequence[EndpointResult], preconditions: Sequence[Precondition]) -> Composite:
    no_go = [r for r in results if r.outcome is Outcome.NO_GO]
    if no_go:
        components = tuple(dict.fromkeys(named_component(r) for r in no_go))
        return Composite(k.COMPOSITE_REDESIGN, components, tuple(r.id for r in no_go), {})
    everything_go = all(r.outcome is Outcome.GO for r in results)
    if everything_go and all(p.holds for p in preconditions):
        return Composite(k.COMPOSITE_PROCEED, (), (), {})
    open_endpoints = [r for r in results if r.outcome is not Outcome.GO]
    return Composite(
        k.COMPOSITE_INCONCLUSIVE,
        (),
        tuple(r.id for r in open_endpoints),
        {r.id: n_needed_text(r) for r in open_endpoints},
    )
