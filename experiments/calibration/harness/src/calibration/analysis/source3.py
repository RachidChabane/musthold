"""Source (3): UNDETERMINED at the operating points of §7.3."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Final

from calibration.analysis.model import CellData, Decision, Kind
from calibration.domain.canonical import JsonValue
from calibration.domain.constants import (
    GATE_THRESHOLD_JH3,
    GATE_THRESHOLD_JH5,
    GATE_THRESHOLD_S1,
    VH2_JUDGE_THRESHOLD,
)
from calibration.domain.sources import SourceId

RULE_CONFIDENCE: Final = "confidence at or above the threshold"
RULE_UNANIMOUS: Final = "both votes on the same answer with stated confidence at or above the threshold"


@dataclass(frozen=True)
class OperatingPoint:
    rule: str
    threshold: float
    product_rule: str


def operating_points(source: SourceId, kind: Kind) -> list[OperatingPoint]:
    """The operating points of the table of §7.3 for a source and a kind."""
    if source is SourceId.S1 and kind is Kind.K3:
        return [OperatingPoint(RULE_CONFIDENCE, GATE_THRESHOLD_S1, "alignment tauRoute, default")]
    if source is SourceId.S1 and kind is Kind.K1:
        return [OperatingPoint(RULE_CONFIDENCE, GATE_THRESHOLD_S1, "reported for comparison")]
    if source is SourceId.JH5 and kind is not Kind.K4:
        return [OperatingPoint(RULE_CONFIDENCE, GATE_THRESHOLD_JH5, "alignment modalMin, default, for host-votes(5)")]
    if source is SourceId.JH3 and kind is not Kind.K4:
        return [OperatingPoint(RULE_CONFIDENCE, GATE_THRESHOLD_JH3, "alignment tauRoute for host-votes(3)")]
    if source is SourceId.VH2:
        return [OperatingPoint(RULE_UNANIMOUS, VH2_JUDGE_THRESHOLD, "discpower tauJudge, default, for host-votes(2)")]
    return []


def decided(decision: Decision, point: OperatingPoint, source: SourceId) -> bool:
    """A decision is decided when it has a prediction whose confidence reaches the threshold; for VH2 the stated
    confidences of the two agreeing votes must."""
    if not decision.has_prediction:
        return False
    if source is SourceId.VH2:
        return decision.alt_confidence is not None and decision.alt_confidence >= point.threshold
    return decision.confidence >= point.threshold


def _composition(undetermined: list[Decision], kind: Kind) -> dict[str, JsonValue]:
    if kind is Kind.K1:
        predicted = [d for d in undetermined if d.has_prediction]
        return {
            "would_be_correct": sum(1 for d in predicted if d.correct),
            "would_be_wrong": sum(1 for d in predicted if not d.correct),
            "no_prediction": len(undetermined) - len(predicted),
        }
    return {
        "truth_positive": sum(1 for d in undetermined if d.truth_positive is True),
        "truth_negative": sum(1 for d in undetermined if d.truth_positive is False),
        "no_prediction": sum(1 for d in undetermined if not d.has_prediction),
    }


def undetermined_figures(cell: CellData, point: OperatingPoint) -> dict[str, JsonValue]:
    """The UNDETERMINED rate, the error rate among the decided decisions and the truth composition of the rest."""
    everything = list(cell.decisions)
    decided_set = [d for d in everything if decided(d, point, cell.source)]
    undetermined = [d for d in everything if not decided(d, point, cell.source)]
    total = len(everything)
    return {
        "rule": point.rule,
        "threshold": point.threshold,
        "product_rule": point.product_rule,
        "decisions": total,
        "decided": len(decided_set),
        "undetermined": len(undetermined),
        "undetermined_rate": len(undetermined) / total if total else None,
        "error_rate_among_decided": (
            sum(1 for d in decided_set if not d.correct) / len(decided_set) if decided_set else None
        ),
        "composition": _composition(undetermined, cell.kind),
    }
