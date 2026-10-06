"""Validity-judge payload (prereg validity schema) as typed verdicts, plus its semantic checks."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass
from enum import Enum
from typing import Any

from pilot.domain.witness import Witness

REQUIREMENTS_KEY = "requirements"
EQUIVALENT_KEY = "equivalent"
CONFIDENCE_MIN = 0.0
CONFIDENCE_MAX = 1.0


class Verdict(Enum):
    VIOLATED = "VIOLATED"
    NOT_VIOLATED = "NOT_VIOLATED"
    UNDETERMINED = "UNDETERMINED"


@dataclass(frozen=True)
class CriterionVerdict:
    verdict: Verdict
    confidence: float
    witness: Witness | None


@dataclass(frozen=True)
class ValidityVerdict:
    equivalent: bool
    equivalent_confidence: float
    requirements: Mapping[str, CriterionVerdict]

    def witnesses(self) -> dict[str, Witness]:
        return {
            ref: found.witness
            for ref, found in self.requirements.items()
            if found.verdict is Verdict.VIOLATED and found.witness is not None
        }


def parse_validity(payload: Mapping[str, Any]) -> ValidityVerdict:
    equivalent = payload[EQUIVALENT_KEY]
    return ValidityVerdict(
        equivalent=bool(equivalent["value"]),
        equivalent_confidence=float(equivalent["confidence"]),
        requirements={
            ref: _criterion_verdict(entry) for ref, entry in payload[REQUIREMENTS_KEY].items()
        },
    )


def _criterion_verdict(entry: Mapping[str, Any]) -> CriterionVerdict:
    witness = entry["witness"]
    return CriterionVerdict(
        verdict=Verdict(entry["verdict"]),
        confidence=float(entry["confidence"]),
        witness=Witness.from_payload(witness) if witness is not None else None,
    )


def validity_problems(payload: Any) -> list[str]:
    """Constraints the schema cannot express: confidences in [0, 1], a witness for every VIOLATED."""
    problems: list[str] = []
    verdict = parse_validity(payload)
    if not CONFIDENCE_MIN <= verdict.equivalent_confidence <= CONFIDENCE_MAX:
        problems.append("equivalent.confidence outside [0, 1]")
    for ref, found in verdict.requirements.items():
        if not CONFIDENCE_MIN <= found.confidence <= CONFIDENCE_MAX:
            problems.append(f"{ref}: confidence outside [0, 1]")
        if found.verdict is Verdict.VIOLATED and found.witness is None:
            problems.append(f"{ref}: VIOLATED without a witness")
    return problems
