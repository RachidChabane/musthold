"""Determinism of source (1) (§8.6): pass 2 compared with pass 1."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from typing import Final

import numpy as np

from calibration.analysis.model import Decision
from calibration.analysis.stats.arrays import NAN
from calibration.analysis.stats.intervals import Estimate, Proportion
from calibration.domain.canonical import JsonValue
from calibration.domain.constants import UNSTABLE_FLIP_RATE
from calibration.domain.ports import DecodedAnswer

P99: Final = 99.0


@dataclass(frozen=True)
class DeterminismFigures:
    pairs: int
    identical_share: float
    max_delta: float
    p99_delta: float
    flips: Estimate
    unstable: bool

    def to_record(self) -> dict[str, JsonValue]:
        return {
            "pairs": self.pairs,
            "identical_share": None if np.isnan(self.identical_share) else self.identical_share,
            "max_abs_delta_confidence": None if np.isnan(self.max_delta) else self.max_delta,
            "p99_abs_delta_confidence": None if np.isnan(self.p99_delta) else self.p99_delta,
            "flip_rate": dict(self.flips.to_record()),
            "unstable": self.unstable,
        }


def answers_identical(
    first: Mapping[str, DecodedAnswer], second: Mapping[str, DecodedAnswer], questions: Sequence[str]
) -> bool:
    """Identical `confidence` and probabilities, at the precision the server reports, on the given questions."""
    return all(
        question in first
        and question in second
        and first[question].confidence == second[question].confidence
        and dict(first[question].probabilities) == dict(second[question].probabilities)
        for question in questions
    )


def determinism_of(
    first: Mapping[str, Decision], second: Mapping[str, Decision], identical: Mapping[str, bool]
) -> DeterminismFigures:
    """Per (kind, model): the share of identical answers, the spread of the confidence and the prediction flip rate
    over the units answered in both passes; a flip rate above 0.01 annotates the cell UNSTABLE."""
    shared = sorted(set(first) & set(second))
    if not shared:
        return DeterminismFigures(0, NAN, NAN, NAN, Proportion(0, 0).estimate(), False)
    deltas = np.array([abs(first[unit].confidence - second[unit].confidence) for unit in shared], dtype=np.float64)
    flips = Proportion(sum(1 for unit in shared if first[unit].prediction != second[unit].prediction), len(shared))
    return DeterminismFigures(
        pairs=len(shared),
        identical_share=sum(1 for unit in shared if identical.get(unit, False)) / len(shared),
        max_delta=float(deltas.max()),
        p99_delta=float(np.percentile(deltas, P99)),
        flips=flips.estimate(),
        unstable=flips.point > UNSTABLE_FLIP_RATE,
    )
