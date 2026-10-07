"""Source (1): the decisions of K1 to K4 that one model's recorded answers make (§3.2, §7.1, §7.4)."""

from __future__ import annotations

from collections import Counter
from collections.abc import Mapping
from dataclasses import dataclass

from calibration.analysis.model import (
    FALL_THROUGH_RELAY_REFUSED,
    FALL_THROUGH_TYPES,
    CellData,
    Decision,
    Kind,
)
from calibration.analysis.replies import ReplyOutcome
from calibration.analysis.truth import PilotTruth, TestLabel
from calibration.analysis.votes import ADEQUATE_LEVELS, NONE_ANSWER
from calibration.domain.items import Item, KindFamily
from calibration.domain.jsonread import RecordError
from calibration.domain.pilot_records import criterion_number
from calibration.domain.ports import DecodedAnswer, ProductReader, ScreenReading, ScreenReadRequest
from calibration.domain.server_wire import QuestionSpec, level_probability, question_specs
from calibration.domain.sources import SourceId

QUESTION_CRITERION = "criterion"
QUESTION_COVERAGE = "coverage"
ANSWER_VIOLATES = "VIOLATES"
STRATUM_ARM = "arm"
STRATUM_F3 = "f3"
STRATUM_TARGET = "target"
STRATUM_KILL = "kill"
STRATUM_SCREEN = "screen"
YES = "yes"
NO = "no"
ARM_T = "T"
NO_DIFF_FOUND = "NO_DIFF_FOUND"


def _confidence(answer: DecodedAnswer) -> float:
    """An absent confidence counts as 0 (§7.1)."""
    return answer.confidence if answer.confidence is not None else 0.0


def _spec(item: Item, question_id: str) -> QuestionSpec:
    return next(spec for spec in question_specs(item.closed) if spec.question_id == question_id)


@dataclass(frozen=True)
class AlignmentReading:
    """What a model answered about one test: the criterion it named and the coverage it rated, with confidences."""

    ref: str
    criterion_confidence: float
    adequate: bool
    coverage_confidence: float
    criterion_probability: float
    adequate_probability: float
    declared_probability: float


def read_alignment(item: Item, outcome: ReplyOutcome) -> AlignmentReading:
    criterion = outcome.answers[QUESTION_CRITERION]
    coverage = outcome.answers[QUESTION_COVERAGE]
    token_to_ref = item.token_to_ref or {}
    declared_token = next((token for token, ref in token_to_ref.items() if ref == item.declared), NONE_ANSWER)
    if outcome.returned is None:
        raise RecordError(f"{item.request_id}: an answered request carries no server reading")
    criterion_view = outcome.returned.answers[QUESTION_CRITERION]
    coverage_view = outcome.returned.answers[QUESTION_COVERAGE]
    levels = _spec(item, QUESTION_COVERAGE).options
    adequate_probability = sum(
        level_probability(coverage_view.probabilities, index, name)
        for index, name in enumerate(levels)
        if name in ADEQUATE_LEVELS
    )
    return AlignmentReading(
        ref=token_to_ref.get(criterion.choice, NONE_ANSWER),
        criterion_confidence=_confidence(criterion),
        adequate=coverage.choice in ADEQUATE_LEVELS,
        coverage_confidence=_confidence(coverage),
        criterion_probability=criterion_view.probabilities.get(criterion.choice, 0.0),
        adequate_probability=adequate_probability,
        declared_probability=criterion_view.probabilities.get(declared_token, 0.0),
    )


def _side(probability: float, positive: bool) -> float:
    return probability if positive else 1.0 - probability


def _k1(item: Item, label: TestLabel, reading: AlignmentReading, consensus: bool) -> Decision:
    return Decision(
        unit=item.unit,
        cluster=label.cites,
        confidence=reading.criterion_confidence,
        correct=reading.ref in label.k1_accepted(lenient=False),
        alt_confidence=reading.criterion_probability,
        lenient_correct=reading.ref in label.k1_accepted(lenient=True),
        accepted=label.k1_accepted(lenient=False),
        declared_ref=label.cites,
        prediction=reading.ref,
        consensus=consensus,
    )


def _k2(item: Item, label: TestLabel, reading: AlignmentReading, consensus: bool) -> Decision | None:
    truth = label.k2_positive
    if truth is None:
        return None
    confidence = reading.coverage_confidence
    return Decision(
        unit=item.unit,
        cluster=label.cites,
        confidence=confidence,
        correct=reading.adequate == truth,
        event_probability=_side(confidence, reading.adequate),
        truth_positive=truth,
        predicted_positive=reading.adequate,
        prediction=str(reading.adequate),
        alt_confidence=_side(reading.adequate_probability, reading.adequate),
        alt_event_probability=reading.adequate_probability,
        consensus=consensus,
    )


def _k3(item: Item, label: TestLabel, reading: AlignmentReading, consensus: bool) -> Decision:
    aligned = reading.ref == item.declared and reading.adequate
    confidence = min(reading.criterion_confidence, reading.coverage_confidence)
    q = min(reading.declared_probability, reading.adequate_probability)
    return Decision(
        unit=item.unit,
        cluster=label.cites,
        confidence=confidence,
        correct=aligned == label.k3_positive,
        event_probability=_side(confidence, aligned),
        truth_positive=label.k3_positive,
        predicted_positive=aligned,
        prediction=str(aligned),
        alt_confidence=_side(q, aligned),
        alt_event_probability=q,
        consensus=consensus,
    )


def alignment_cells(
    model_id: str,
    confidence_source: str,
    outcomes: Mapping[str, ReplyOutcome],
    items: Mapping[str, Item],
    truth: PilotTruth,
) -> dict[Kind, CellData]:
    """K1, K2 and K3 cells of one model; a fall-through is counted per kind and is no decision."""
    labels = {label.nodeid: label for label in truth.labels}
    decisions: dict[Kind, list[Decision]] = {Kind.K1: [], Kind.K2: [], Kind.K3: []}
    fall: dict[Kind, Counter[str]] = {kind: Counter() for kind in decisions}
    units = {Kind.K1: 0, Kind.K2: 0, Kind.K3: 0}
    unit_clusters: dict[Kind, list[str]] = {Kind.K1: [], Kind.K2: [], Kind.K3: []}
    for request_id, outcome in sorted(outcomes.items()):
        item = items[request_id]
        if item.family is not KindFamily.ALIGNMENT or item.nodeid is None:
            continue
        label = labels[item.nodeid]
        kinds = [Kind.K1, Kind.K3] if label.k2_positive is None else [Kind.K1, Kind.K2, Kind.K3]
        for kind in kinds:
            units[kind] += 1
            unit_clusters[kind].append(label.cites)
        if not outcome.answered:
            for kind in kinds:
                fall[kind][outcome.status] += 1
            continue
        reading = read_alignment(item, outcome)
        consensus = item.nodeid in truth.consensus
        decisions[Kind.K1].append(_k1(item, label, reading, consensus))
        k2 = _k2(item, label, reading, consensus)
        if k2 is not None:
            decisions[Kind.K2].append(k2)
        decisions[Kind.K3].append(_k3(item, label, reading, consensus))
    return {
        kind: CellData(
            kind,
            SourceId.S1,
            model_id,
            confidence_source,
            tuple(found),
            units[kind],
            {status: fall[kind][status] for status in FALL_THROUGH_TYPES},
            unit_clusters=tuple(unit_clusters[kind]),
        )
        for kind, found in decisions.items()
    }


@dataclass(frozen=True)
class ScreenPairs:
    """The K4 pairs of a model and the mutants left out for want of an oracle record."""

    cell: CellData
    without_oracle: tuple[str, ...]
    readings: Mapping[str, ScreenReading]


def cluster_of(truth: PilotTruth, mutant_id: str) -> str:
    meta = truth.mutants[mutant_id]
    if meta.arm == ARM_T:
        return f"T:{meta.generator_call_id}"
    return f"P:{truth.arm_p_sources[mutant_id]}"


def strata_tags(truth: PilotTruth, mutant_id: str, ref: str) -> dict[str, str]:
    meta = truth.mutants[mutant_id]
    survives = not truth.reproduced_kill(mutant_id, ref)
    f3 = truth.f3_status(mutant_id)
    return {
        STRATUM_ARM: meta.arm,
        STRATUM_F3: f3,
        STRATUM_TARGET: YES if meta.target == ref else NO,
        STRATUM_KILL: NO if survives else YES,
        STRATUM_SCREEN: YES if survives and f3 != NO_DIFF_FOUND else NO,
    }


def screen_pairs(
    model_id: str,
    confidence_source: str,
    outcomes: Mapping[str, ReplyOutcome],
    items: Mapping[str, Item],
    truth: PilotTruth,
    reader: ProductReader,
) -> ScreenPairs:
    """The K4 cell of one model: ten decisions per answered mutant, truth from the oracle classes."""
    wanted: list[tuple[Item, ReplyOutcome]] = []
    without_oracle: list[str] = []
    for request_id, outcome in sorted(outcomes.items()):
        item = items[request_id]
        if item.family is not KindFamily.SCREEN or item.mutant_id is None:
            continue
        if item.mutant_id not in truth.oracle:
            without_oracle.append(item.mutant_id)
            continue
        wanted.append((item, outcome))
    requests = [
        ScreenReadRequest(f"{item.request_id}#{question}", outcome.document or "", question)
        for item, outcome in wanted
        if outcome.answered
        for question in sorted(item.question_to_ref or {})
    ]
    readings = {reading.key: reading for reading in reader.read_screen(requests)}
    decisions: list[Decision] = []
    fall: Counter[str] = Counter()
    units = 0
    unit_clusters: list[str] = []
    for item, outcome in wanted:
        refs = item.question_to_ref or {}
        units += len(refs)
        unit_clusters.extend([cluster_of(truth, item.unit)] * len(refs))
        if not outcome.answered:
            fall[outcome.status] += len(refs)
            continue
        mutant_id = item.unit
        for question, ref in sorted(refs.items()):
            reading = readings[f"{item.request_id}#{question}"]
            if reading.error is not None or reading.choice is None:
                fall[FALL_THROUGH_RELAY_REFUSED] += 1
                continue
            violates = reading.choice == ANSWER_VIOLATES
            probability = reading.violation_probability
            decisions.append(
                Decision(
                    unit=f"{mutant_id}|{ref}",
                    cluster=cluster_of(truth, mutant_id),
                    confidence=_side(probability, violates),
                    correct=violates == (criterion_number(ref) in truth.oracle[mutant_id]),
                    event_probability=probability,
                    truth_positive=criterion_number(ref) in truth.oracle[mutant_id],
                    predicted_positive=violates,
                    prediction=str(violates),
                    alt_event_probability=reading.violates_returned if reading.has_probability else None,
                    tags=strata_tags(truth, mutant_id, ref),
                )
            )
    cell = CellData(
        Kind.K4,
        SourceId.S1,
        model_id,
        confidence_source,
        tuple(decisions),
        units,
        {status: fall[status] for status in FALL_THROUGH_TYPES},
        unit_clusters=tuple(unit_clusters),
    )
    return ScreenPairs(cell, tuple(sorted(without_oracle)), readings)
