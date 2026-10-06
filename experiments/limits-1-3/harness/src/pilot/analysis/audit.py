"""The A-OPUS audit sample (PREREG 6.7): stratified, seeded, ids and strata only."""

from __future__ import annotations

import random
from collections.abc import Mapping, Sequence
from dataclasses import dataclass

from pilot.analysis.model import Context
from pilot.domain.analysis_constants import (
    AUDIT_ORACLE_OTHER,
    AUDIT_ORACLE_VALID,
    AUDIT_PIPELINE_OTHER,
    AUDIT_PIPELINE_VALID,
    AUDIT_STRATA_SAMPLE_SIZE,
    AUDIT_STRATUM_ALWAYS_SURVIVED,
    AUDIT_STRATUM_FLOOR,
    AUDIT_STRATUM_PLANTED,
    AUDIT_STRATUM_SEPARATOR,
)


@dataclass(frozen=True)
class AuditSample:
    seed: int
    mutant_ids: tuple[str, ...]
    strata: Mapping[str, str]

    def to_mapping(self) -> dict[str, object]:
        return {"seed": self.seed, "mutant_ids": list(self.mutant_ids), "strata": dict(self.strata)}


def allocate(sizes: Mapping[str, int], total: int, floor: int) -> dict[str, int]:
    """Proportional allocation with a floor per non-empty cell, by largest remainder.

    Each non-empty cell gets min(floor, size); the rest is shared in proportion to the
    cells' remaining capacity, never exceeding a cell's size.
    """
    base = {cell: min(floor, size) for cell, size in sizes.items() if size > 0}
    left = max(total - sum(base.values()), 0)
    capacity = {cell: sizes[cell] - base[cell] for cell in base}
    pool = sum(capacity.values())
    if left == 0 or pool == 0:
        return base
    left = min(left, pool)
    quotas = {cell: left * capacity[cell] / pool for cell in base}
    extra = {cell: min(int(quotas[cell]), capacity[cell]) for cell in base}
    remaining = left - sum(extra.values())
    order = sorted(base, key=lambda c: (-(quotas[c] - int(quotas[c])), c))
    for cell in order:
        if remaining == 0:
            break
        if extra[cell] < capacity[cell]:
            extra[cell] += 1
            remaining -= 1
    return {cell: base[cell] + extra[cell] for cell in base}


def stratum_of(pipeline_valid: bool, oracle_valid: bool) -> str:
    pipeline = AUDIT_PIPELINE_VALID if pipeline_valid else AUDIT_PIPELINE_OTHER
    oracle = AUDIT_ORACLE_VALID if oracle_valid else AUDIT_ORACLE_OTHER
    return f"{pipeline}{AUDIT_STRATUM_SEPARATOR}{oracle}"


def draw_sample(
    cells: Mapping[str, Sequence[str]],
    always: Sequence[str],
    planted: Sequence[str],
    seed: int,
    size: int = AUDIT_STRATA_SAMPLE_SIZE,
    floor: int = AUDIT_STRATUM_FLOOR,
) -> AuditSample:
    rng = random.Random(seed)
    counts = allocate({cell: len(ids) for cell, ids in cells.items()}, size, floor)
    strata: dict[str, str] = {}
    for cell in sorted(cells):
        ids = sorted(cells[cell])
        rng.shuffle(ids)
        for mutant_id in ids[: counts.get(cell, 0)]:
            strata[mutant_id] = cell
    for mutant_id in always:
        strata.setdefault(mutant_id, AUDIT_STRATUM_ALWAYS_SURVIVED)
    for mutant_id in planted:
        strata.setdefault(mutant_id, AUDIT_STRATUM_PLANTED)
    return AuditSample(seed, tuple(sorted(strata)), dict(sorted(strata.items())))


def audit_sample(ctx: Context, seed: int) -> AuditSample:
    cells: dict[str, list[str]] = {}
    always: list[str] = []
    for view in ctx.arm_t_evaluated():
        pipeline_valid = view.target_valid
        oracle_valid = ctx.oracle_valid_for(view.mutant_id, view.target)
        cells.setdefault(stratum_of(pipeline_valid, oracle_valid), []).append(view.mutant_id)
        if pipeline_valid and view.survived_all:
            always.append(view.mutant_id)
    return draw_sample(cells, sorted(always), sorted(ctx.truth.planted), seed)
