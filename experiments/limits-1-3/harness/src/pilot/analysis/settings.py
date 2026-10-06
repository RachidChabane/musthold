"""Run-time knobs of the analysis; the defaults are the pre-registered values."""

from __future__ import annotations

from dataclasses import dataclass

from pilot.domain.analysis_constants import ANALYSIS_SEED, BOOTSTRAP_RESAMPLES, PERMUTATION_COUNT


@dataclass(frozen=True)
class AnalysisSettings:
    resamples: int = BOOTSTRAP_RESAMPLES
    permutations: int = PERMUTATION_COUNT
    seed: int = ANALYSIS_SEED
