"""The spec-gap draw: which two criteria of a task are removed in the spec-gap arm, chosen by seed before sealing."""

from __future__ import annotations

from typing import Final

from specbench.domain.canonical import JsonValue
from specbench.domain.mining import seeded_key
from specbench.domain.spec import GAP_CRITERIA_COUNT, SpecDraft, SpecError, criterion_id

GAP_FILE_NAME: Final = "construction/gaps.json"
GAP_DRAW_LABEL: Final = "spec-gap"


def draw_gap_criteria(seed: int, task_id: str, draft: SpecDraft) -> tuple[int, ...]:
    """The numbers, ascending, of the criteria drawn for the gap variants: the lowest seeded keys among the eligible."""
    eligible = [c for c in draft.criteria if c.is_gap_eligible]
    if len(eligible) < GAP_CRITERIA_COUNT:
        raise SpecError(f"{len(eligible)} gap-eligible criteria, {GAP_CRITERIA_COUNT} required")
    ranked = sorted(eligible, key=lambda c: seeded_key(seed, f"{GAP_DRAW_LABEL}:{criterion_id(task_id, c.number)}"))
    return tuple(sorted(c.number for c in ranked[:GAP_CRITERIA_COUNT]))


def gaps_record(seed: int, task_id: str, numbers: tuple[int, ...]) -> dict[str, JsonValue]:
    """The sealed record of a draw: the seed and the drawn criterion ids; dimension and hunks stay in `spec.json`."""
    return {"criteria": [criterion_id(task_id, n) for n in numbers], "seed": seed}
