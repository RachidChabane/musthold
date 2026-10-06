"""Aggregation of the alignment repetitions into a verdict (PREREG 7.4), label-blind."""

from __future__ import annotations

from collections import Counter, defaultdict
from collections.abc import Sequence
from dataclasses import dataclass
from enum import Enum
from typing import Any

from pilot.domain.pipeline_constants import (
    ALIGNMENT_MODAL_SHARE_THRESHOLD,
    ALIGNMENT_REPS,
    NONE_ANSWER,
)

COVERAGE_ALIGNED = ("FULL", "PARTIAL")
COVERAGE_PARTIAL = "PARTIAL"
COVERAGE_NONE = "NONE"


class AlignmentVerdict(Enum):
    ALIGNED = "ALIGNED"
    MISALIGNED = "MISALIGNED"
    UNDETERMINED = "UNDETERMINED"


@dataclass(frozen=True)
class Repetition:
    """One answered repetition, its token already mapped back to a criterion reference or NONE."""

    verifies_ref: str
    coverage: str
    confidence: float


@dataclass(frozen=True)
class AlignmentAggregate:
    modal_ref: str | None
    modal_share: float
    modal_coverage: str | None
    mean_confidence: float | None
    verdict: AlignmentVerdict
    verdict_s1: AlignmentVerdict
    missing_reps: int

    def to_mapping(self) -> dict[str, Any]:
        return {
            "modal_ref": self.modal_ref,
            "modal_share": self.modal_share,
            "modal_coverage": self.modal_coverage,
            "mean_confidence": self.mean_confidence,
            "verdict": self.verdict.value,
            "verdict_s1": self.verdict_s1.value,
            "missing_reps": self.missing_reps,
        }


def aggregate_alignment(
    repetitions: Sequence[Repetition], declared_ref: str, total_reps: int = ALIGNMENT_REPS
) -> AlignmentAggregate:
    missing = total_reps - len(repetitions)
    if not repetitions:
        return AlignmentAggregate(
            None, 0.0, None, None, AlignmentVerdict.UNDETERMINED, AlignmentVerdict.UNDETERMINED, missing
        )
    modal_ref = _modal_reference(repetitions)
    on_modal = [r for r in repetitions if r.verifies_ref == modal_ref]
    share = len(on_modal) / total_reps
    coverage = _modal_coverage(on_modal)
    verdict = _verdict(modal_ref, share, coverage, declared_ref)
    return AlignmentAggregate(
        modal_ref=modal_ref,
        modal_share=share,
        modal_coverage=coverage,
        mean_confidence=_mean(on_modal),
        verdict=verdict,
        verdict_s1=_sensitivity_s1(verdict, coverage),
        missing_reps=missing,
    )


def _mean(repetitions: Sequence[Repetition]) -> float:
    return sum(r.confidence for r in repetitions) / len(repetitions)


def _modal_reference(repetitions: Sequence[Repetition]) -> str:
    """Highest count; ties to the higher mean confidence, then lexicographic order."""
    grouped: dict[str, list[Repetition]] = defaultdict(list)
    for repetition in repetitions:
        grouped[repetition.verifies_ref].append(repetition)
    return min(grouped, key=lambda ref: (-len(grouped[ref]), -_mean(grouped[ref]), ref))


def _modal_coverage(on_modal: Sequence[Repetition]) -> str:
    counts = Counter(r.coverage for r in on_modal)
    return min(counts, key=lambda coverage: (-counts[coverage], coverage))


def _verdict(modal_ref: str, share: float, coverage: str, declared_ref: str) -> AlignmentVerdict:
    if share < ALIGNMENT_MODAL_SHARE_THRESHOLD:
        return AlignmentVerdict.UNDETERMINED
    if modal_ref == declared_ref and coverage in COVERAGE_ALIGNED:
        return AlignmentVerdict.ALIGNED
    return AlignmentVerdict.MISALIGNED


def _sensitivity_s1(verdict: AlignmentVerdict, coverage: str) -> AlignmentVerdict:
    if verdict is AlignmentVerdict.ALIGNED and coverage == COVERAGE_PARTIAL:
        return AlignmentVerdict.UNDETERMINED
    return verdict


__all__ = [
    "COVERAGE_NONE",
    "NONE_ANSWER",
    "AlignmentAggregate",
    "AlignmentVerdict",
    "Repetition",
    "aggregate_alignment",
]
