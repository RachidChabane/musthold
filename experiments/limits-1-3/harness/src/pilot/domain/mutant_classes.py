"""Mutant classes per (mutant, criterion) from the two host validity votes and the executed evidence (PREREG 6.5)."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from enum import Enum
from typing import Any

from pilot.domain.execution_validation import valid_x_criteria
from pilot.domain.mutants import DifferentialKind
from pilot.domain.outcomes import KillResult
from pilot.domain.pipeline_constants import TAU
from pilot.domain.validity_payload import ValidityVerdict, Verdict


class CriterionClass(Enum):
    VALID_W = "VALID_W"
    NOT_VIOLATING = "NOT_VIOLATING"
    UNDETERMINED = "UNDETERMINED"


@dataclass(frozen=True)
class JudgeFindings:
    """One vote's parsed verdict (None when its call is missing) and the criteria whose witness confirmed."""

    verdict: ValidityVerdict | None
    confirmed: frozenset[str] = frozenset()


@dataclass(frozen=True)
class MutantEvidence:
    mutant_id: str
    arm: str
    target: str | None
    kills: Mapping[str, KillResult]
    differential: DifferentialKind
    judges: Sequence[JudgeFindings]
    bindings: Mapping[str, str]

    @property
    def has_reproduced_kill(self) -> bool:
        return any(result is KillResult.KILLED for result in self.kills.values())

    @property
    def survived_all(self) -> bool:
        return bool(self.kills) and all(result.counts_as_survived for result in self.kills.values())


@dataclass(frozen=True)
class MutantClass:
    mutant_id: str
    arm: str
    target: str | None
    per_criterion: Mapping[str, CriterionClass]
    valid_x: frozenset[str]
    valid_confidence: Mapping[str, float]
    equivalent: bool
    unattributed: bool
    off_target: bool
    collateral: bool
    primary_included: bool
    primary_criterion: str | None
    discarded: bool
    survived_all: bool

    def to_mapping(self) -> dict[str, Any]:
        return {
            "mutant_id": self.mutant_id,
            "arm": self.arm,
            "target": self.target,
            "per_criterion": {ref: cls.value for ref, cls in self.per_criterion.items()},
            "valid_x": sorted(self.valid_x),
            "valid_confidence": dict(self.valid_confidence),
            "equivalent": self.equivalent,
            "unattributed": self.unattributed,
            "off_target": self.off_target,
            "collateral": self.collateral,
            "primary": {"included": self.primary_included, "criterion": self.primary_criterion},
            "discarded": self.discarded,
            "survived_all": self.survived_all,
        }


def classify_mutant(evidence: MutantEvidence, criteria: Sequence[str], tau: float = TAU) -> MutantClass:
    verdicts = [j.verdict for j in evidence.judges]
    both = all(v is not None for v in verdicts) and len(verdicts) > 0
    per_criterion: dict[str, CriterionClass] = {}
    confidence: dict[str, float] = {}
    for ref in criteria:
        per_criterion[ref], valid_confidence = _criterion_class(evidence, ref, tau, both)
        if valid_confidence is not None:
            confidence[ref] = valid_confidence
    valid = [ref for ref, cls in per_criterion.items() if cls is CriterionClass.VALID_W]
    collateral = len(valid) >= 2
    equivalent = _equivalent(evidence, tau, both)
    target = evidence.target
    target_valid = target is not None and per_criterion.get(target) is CriterionClass.VALID_W
    survived_all = evidence.survived_all
    return MutantClass(
        mutant_id=evidence.mutant_id,
        arm=evidence.arm,
        target=target,
        per_criterion=per_criterion,
        valid_x=valid_x_criteria(valid, True, evidence.kills, evidence.bindings),
        valid_confidence=confidence,
        equivalent=equivalent,
        unattributed=_unattributed(evidence, tau, both, criteria),
        off_target=target is not None and not target_valid and any(ref != target for ref in valid),
        collateral=collateral,
        primary_included=bool(target_valid and not collateral),
        primary_criterion=target if target_valid and not collateral else None,
        discarded=survived_all
        and (equivalent or (target is not None and per_criterion.get(target) is CriterionClass.NOT_VIOLATING)),
        survived_all=survived_all,
    )


def _criterion_class(
    evidence: MutantEvidence, ref: str, tau: float, both: bool
) -> tuple[CriterionClass, float | None]:
    if not both:
        return CriterionClass.UNDETERMINED, None
    found = [j.verdict.requirements[ref] for j in evidence.judges if j.verdict is not None]
    confirmed = any(ref in j.confirmed for j in evidence.judges)
    if all(f.verdict is Verdict.VIOLATED and f.confidence >= tau for f in found) and confirmed:
        return CriterionClass.VALID_W, min(f.confidence for f in found)
    if all(f.verdict is Verdict.NOT_VIOLATED and f.confidence >= tau for f in found):
        return CriterionClass.NOT_VIOLATING, None
    return CriterionClass.UNDETERMINED, None


def _equivalent(evidence: MutantEvidence, tau: float, both: bool) -> bool:
    if not both or evidence.has_reproduced_kill or evidence.differential is DifferentialKind.DIFFERS:
        return False
    return all(
        j.verdict is not None and j.verdict.equivalent and j.verdict.equivalent_confidence >= tau
        for j in evidence.judges
    )


def _unattributed(evidence: MutantEvidence, tau: float, both: bool, criteria: Sequence[str]) -> bool:
    differs = evidence.has_reproduced_kill or evidence.differential is DifferentialKind.DIFFERS
    if not both or not differs:
        return False
    return all(
        j.verdict is not None
        and all(
            j.verdict.requirements[ref].verdict is Verdict.NOT_VIOLATED
            and j.verdict.requirements[ref].confidence >= tau
            for ref in criteria
        )
        for j in evidence.judges
    )
