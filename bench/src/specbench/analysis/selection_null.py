"""The yield-matched random-rejection comparator for X_sel: permute which trunks the gate rejects, keeping the yield."""

from __future__ import annotations

import math
from collections.abc import Sequence
from dataclasses import dataclass
from typing import Final

import numpy as np

from specbench.analysis.bootstrap import LOWER_PERCENTILE, NAN, UPPER_PERCENTILE
from specbench.domain.arms import Arm, Harness
from specbench.domain.records import TrunkRecord

DEFAULT_PERMUTATIONS: Final = 10_000


@dataclass(frozen=True)
class NullDistribution:
    permutations: int
    mean: float
    lo: float
    hi: float


def selection_null(
    trunks: Sequence[TrunkRecord], harness: Harness, arm: Arm, permutations: int, seed: int
) -> NullDistribution:
    """X_sel under random rejection with the arm's own yield, over `permutations` shuffles of its PASS decisions."""
    pairs = [
        (trunk.arms[arm].selection_pass, trunk.s_t.defective)
        for trunk in trunks
        if trunk.harness is harness and trunk.s_t is not None and arm in trunk.arms
        if trunk.arms[arm].selection_pass is not None
    ]
    if not pairs:
        return NullDistribution(0, NAN, NAN, NAN)
    # Sorted so that the shuffles, and therefore the distribution, do not depend on the order of the trunks.
    passed = np.sort(np.array([bool(p) for p, _ in pairs]))
    defective = np.sort(np.array([d for _, d in pairs]))
    base = defective.mean()
    if base == 0 or passed.sum() == 0:
        return NullDistribution(permutations, NAN, NAN, NAN)
    shuffled = np.random.default_rng(seed).permuted(np.tile(passed, (permutations, 1)), axis=1)
    pass_rate = (shuffled & defective).sum(axis=1) / shuffled.sum(axis=1)
    x = 1.0 - pass_rate / base
    lo, hi = np.percentile(x, [LOWER_PERCENTILE, UPPER_PERCENTILE])
    return NullDistribution(permutations, math.fsum(x.tolist()) / x.size, float(lo), float(hi))
