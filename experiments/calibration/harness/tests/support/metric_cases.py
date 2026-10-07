"""Hand-built decisions and cells for the unit tests of the analysis layer."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from functools import cache
from typing import Final

from calibration.analysis.cell_metrics import CellMetrics, MetricSettings, compute_cell_metrics
from calibration.analysis.model import CellData, Decision, Kind
from calibration.domain.sources import SourceId

TEST_RESAMPLES: Final = 100
SETTINGS: Final = MetricSettings(resamples=TEST_RESAMPLES)
HALF: Final = 0.5
CLUSTERS: Final = ("PARK-1~1", "PARK-2~1", "PARK-3~1", "PARK-4~1")
MODEL: Final = "M1"
SOURCE_TEXT: Final = "system-one:nimble:9b@abc"
SCREEN_TAG: Final = "screen"
YES: Final = "yes"
NO: Final = "no"
REF_A: Final = "PARK-1~1"
REF_B: Final = "PARK-2~1"


def top_label_decision(
    index: int, cluster: str, confidence: float, *, correct: bool, accepted: str = REF_A, declared: str = REF_A
) -> Decision:
    """A K1 decision: the top-label confidence against correctness."""
    return Decision(
        unit=f"u{index}",
        cluster=cluster,
        confidence=confidence,
        correct=correct,
        accepted=frozenset({accepted}),
        declared_ref=declared,
        lenient_correct=correct,
        prediction=REF_A,
    )


def event_decision(
    index: int,
    cluster: str,
    event: float,
    *,
    truth: bool,
    tags: Mapping[str, str] | None = None,
    alt: float | None = None,
) -> Decision:
    """A K2 to K4 decision: a positive-event probability against truth; positive is predicted from 0.5."""
    positive = event >= HALF
    return Decision(
        unit=f"u{index}",
        cluster=cluster,
        confidence=event if positive else 1.0 - event,
        correct=positive == truth,
        event_probability=event,
        truth_positive=truth,
        predicted_positive=positive,
        prediction=str(positive),
        alt_confidence=alt,
        tags=dict(tags or {}),
    )


def make_cell(
    kind: Kind,
    decisions: Sequence[Decision],
    *,
    source: SourceId = SourceId.S1,
    model: str = MODEL,
    units: int | None = None,
    fall_through: Mapping[str, int] | None = None,
    units_without_votes: int = 0,
    unit_clusters: Sequence[str] | None = None,
) -> CellData:
    return CellData(
        kind=kind,
        source=source,
        model=model,
        confidence_source=SOURCE_TEXT,
        decisions=tuple(decisions),
        units=len(decisions) if units is None else units,
        fall_through=dict(fall_through or {}),
        units_without_votes=units_without_votes,
        unit_clusters=tuple(unit_clusters) if unit_clusters is not None else tuple(d.cluster for d in decisions),
    )


def spread_clusters(count: int) -> list[str]:
    """Cluster names cycling over the four criteria."""
    return [CLUSTERS[index % len(CLUSTERS)] for index in range(count)]


def mixed_event_cell(kind: Kind, *, tags: Mapping[str, str] | None = None) -> CellData:
    """Twelve decisions with both truth classes in every cluster and imperfect probabilities."""
    events = (0.9, 0.8, 0.6, 0.4, 0.3, 0.1)
    truths = (True, True, False, True, False, False)
    decisions = [
        event_decision(index, cluster, events[index % len(events)], truth=truths[index % len(truths)], tags=tags)
        for index, cluster in enumerate(spread_clusters(12))
    ]
    return make_cell(kind, decisions)


def mixed_top_label_cell() -> CellData:
    decisions = [
        top_label_decision(index, cluster, 0.9 if index % 4 else 0.3, correct=bool(index % 4))
        for index, cluster in enumerate(spread_clusters(12))
    ]
    return make_cell(Kind.K1, decisions)


@cache
def base_metrics(kind: Kind) -> CellMetrics:
    """Real metrics of a mixed cell of the kind, to be adjusted with `dataclasses.replace`."""
    cell = mixed_top_label_cell() if kind is Kind.K1 else mixed_event_cell(kind, tags={SCREEN_TAG: YES})
    metrics = compute_cell_metrics(cell, SETTINGS, equal_mass=False)
    assert metrics is not None
    return metrics
