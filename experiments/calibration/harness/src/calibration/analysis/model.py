"""The decisions every metric reads: one record per (unit, kind) with its confidence, its prediction and its truth."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass, field
from enum import StrEnum

from calibration.domain.sources import SourceId


class Kind(StrEnum):
    K1 = "K1"
    K2 = "K2"
    K3 = "K3"
    K4 = "K4"


ALIGNMENT_KINDS = (Kind.K1, Kind.K2, Kind.K3)
HOST_MODEL = "host"
FALL_THROUGH_MISSING = "MISSING"
FALL_THROUGH_STATE_TOO_LARGE = "STATE_TOO_LARGE"
FALL_THROUGH_RELAY_REFUSED = "RELAY_REFUSED"
FALL_THROUGH_TYPES = (FALL_THROUGH_MISSING, FALL_THROUGH_STATE_TOO_LARGE, FALL_THROUGH_RELAY_REFUSED)


@dataclass(frozen=True)
class Decision:
    """One decision of a kind by a source.

    `confidence` is the primary top-label confidence c and `correct` whether the prediction is right.
    `event_probability` and `truth_positive` are the positive-event probability and truth of K2 to K4 (None for K1,
    whose event is "the prediction is correct"). `has_prediction` is False for a decision with no prediction: it
    counts in source (3) and is excluded from the top-label metrics. `alt_*` carry the alternative measure of the
    sensitivity analyses (the returned probability for a server, the weakest stated confidence for host votes).
    """

    unit: str
    cluster: str
    confidence: float
    correct: bool
    event_probability: float | None = None
    truth_positive: bool | None = None
    predicted_positive: bool | None = None
    has_prediction: bool = True
    alt_confidence: float | None = None
    alt_event_probability: float | None = None
    lenient_correct: bool | None = None
    accepted: frozenset[str] | None = None
    declared_ref: str | None = None
    prediction: str | None = None
    consensus: bool = True
    tags: Mapping[str, str] = field(default_factory=dict[str, str])


@dataclass(frozen=True)
class CellData:
    """The decisions of one (kind, source, model) cell and what was left out of it."""

    kind: Kind
    source: SourceId
    model: str
    confidence_source: str | None
    decisions: tuple[Decision, ...]
    units: int
    fall_through: Mapping[str, int] = field(default_factory=dict[str, int])
    units_without_votes: int = 0
    unit_clusters: tuple[str, ...] = ()
    blocked_reason: str | None = None

    @property
    def key(self) -> str:
        return f"{self.kind.value}|{self.source.value}|{self.model}"

    @property
    def predicted(self) -> tuple[Decision, ...]:
        return tuple(decision for decision in self.decisions if decision.has_prediction)

    @property
    def missing_share(self) -> float:
        """The share of the cell's units that are fall-throughs (S1) or without any vote (source 2)."""
        lost = sum(self.fall_through.values()) + self.units_without_votes
        return lost / self.units if self.units else 0.0
