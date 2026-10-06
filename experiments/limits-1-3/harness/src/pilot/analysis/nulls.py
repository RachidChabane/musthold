"""Null comparators N0, N2, N3, N6 and N7 (PREREG 10)."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass

import numpy as np

from pilot.analysis.model import Context, matrix_pools
from pilot.analysis.stats import FloatArray
from pilot.domain.alignment_aggregate import AlignmentVerdict
from pilot.domain.analysis_constants import ARM_T
from pilot.domain.matrix import (
    PoolMutant,
    TestSignature,
    build_evidence,
    classify_tests,
    mean_dc,
)


@dataclass(frozen=True)
class BinaryScores:
    sensitivity: float
    specificity: float

    @property
    def balanced_accuracy(self) -> float:
        return (self.sensitivity + self.specificity) / 2


def balanced_scores(gold_bad: Sequence[bool], predicted_bad: Sequence[bool]) -> BinaryScores:
    """Sensitivity on gold bad tests and specificity on gold good tests."""
    bad = [p for g, p in zip(gold_bad, predicted_bad, strict=True) if g]
    good = [p for g, p in zip(gold_bad, predicted_bad, strict=True) if not g]
    sensitivity = sum(bad) / len(bad) if bad else float("nan")
    specificity = sum(not p for p in good) / len(good) if good else float("nan")
    return BinaryScores(sensitivity, specificity)


def matrix_predicted_bad(signatures: Mapping[str, TestSignature], nodeids: Sequence[str]) -> list[bool]:
    return [signatures[n].verdict is not AlignmentVerdict.ALIGNED for n in nodeids]


def n2_always_misaligned_precision(gold_bad: Sequence[bool]) -> float:
    """Precision of a judge that calls every test MISALIGNED: the gold bad share."""
    return sum(gold_bad) / len(gold_bad) if gold_bad else float("nan")


def n7_base_rate(ctx: Context) -> tuple[int, int]:
    """Oracle-valid-for-target arm-T candidates among the non-stillborn arm-T candidates."""
    candidates = [
        m for m in ctx.run.mutants.values() if m.arm == ARM_T and not _stillborn(ctx, m.mutant_id)
    ]
    valid = sum(1 for m in candidates if ctx.oracle_valid_for(m.mutant_id, m.target))
    return valid, len(candidates)


def _stillborn(ctx: Context, mutant_id: str) -> bool:
    record = ctx.run.processing.get(mutant_id)
    return record is None or record.stillborn


def n3_null_mean_dc(ctx: Context, permutations: int, seed: int) -> FloatArray:
    """Mean DC when VALID mutants are re-attributed to criteria, per-criterion counts fixed."""
    pool, unattributed = matrix_pools(ctx)
    declared = ctx.declared
    nodeids = list(declared)
    criteria = np.array([m.criterion for m in pool])
    rng = np.random.default_rng(seed)
    values = np.full(permutations, np.nan)
    for i in range(permutations):
        shuffled = rng.permutation(criteria)
        permuted = [PoolMutant(int(c), m.killed_by) for c, m in zip(shuffled, pool, strict=True)]
        evidence = build_evidence(nodeids, permuted, unattributed)
        value = mean_dc(classify_tests(evidence, declared).values())
        values[i] = np.nan if value is None else value
    return values


def n6_null_balanced_accuracy(ctx: Context, permutations: int, seed: int) -> FloatArray:
    """Balanced accuracy of the matrix verdict when declared criteria are permuted among tests."""
    pool, unattributed = matrix_pools(ctx)
    nodeids = list(ctx.declared)
    evidence = build_evidence(nodeids, pool, unattributed)
    gold_bad = [not ctx.gold_good(n) for n in nodeids]
    bindings = np.array([ctx.declared[n] for n in nodeids])
    rng = np.random.default_rng(seed)
    values = np.empty(permutations)
    for i in range(permutations):
        permuted = dict(zip(nodeids, (int(c) for c in rng.permutation(bindings)), strict=True))
        signatures = classify_tests(evidence, permuted)
        predicted = matrix_predicted_bad(signatures, nodeids)
        values[i] = balanced_scores(gold_bad, predicted).balanced_accuracy
    return values
