"""Source (2): decisions made from the pilot's recorded host votes (§7.2). No host model is called."""

from __future__ import annotations

from collections.abc import Mapping

from calibration.analysis.model import ALIGNMENT_KINDS, HOST_MODEL, CellData, Decision, Kind
from calibration.analysis.source1 import cluster_of, strata_tags
from calibration.analysis.truth import VALIDITY_JUDGES, PilotTruth, TestLabel
from calibration.analysis.votes import (
    ANSWER_VIOLATES,
    JudgeAggregate,
    JudgeVote,
    ValidityAggregate,
    ValidityVote,
    aggregate_judge_votes,
    aggregate_validity_votes,
)
from calibration.domain.items import Item, KindFamily
from calibration.domain.pilot_records import criterion_number
from calibration.domain.sources import HOST_VOTE_COUNT, SourceId, host_source_of

FIRST_REPETITION = 1


def _side(probability: float, positive: bool) -> float:
    return probability if positive else 1.0 - probability


def _votes_of(truth: PilotTruth, nodeid: str, count: int) -> list[JudgeVote]:
    cast = truth.alignment_votes.get(nodeid, {})
    return [vote for repetition, vote in sorted(cast.items()) if FIRST_REPETITION <= repetition <= count]


def _judge_k1(item: Item, label: TestLabel, aggregate: JudgeAggregate, consensus: bool) -> Decision:
    return Decision(
        unit=item.unit,
        cluster=label.cites,
        confidence=aggregate.share,
        correct=aggregate.modal_ref in label.k1_accepted(lenient=False),
        lenient_correct=aggregate.modal_ref in label.k1_accepted(lenient=True),
        accepted=label.k1_accepted(lenient=False),
        declared_ref=label.cites,
        consensus=consensus,
    )


def _judge_k2(item: Item, label: TestLabel, aggregate: JudgeAggregate, consensus: bool) -> Decision | None:
    truth = label.k2_positive
    if truth is None:
        return None
    return Decision(
        unit=item.unit,
        cluster=label.cites,
        confidence=aggregate.share,
        correct=aggregate.adequate == truth,
        event_probability=_side(aggregate.share, aggregate.adequate),
        truth_positive=truth,
        predicted_positive=aggregate.adequate,
        consensus=consensus,
    )


def _judge_k3(item: Item, label: TestLabel, aggregate: JudgeAggregate, consensus: bool) -> Decision:
    aligned = aggregate.modal_ref == item.declared and aggregate.adequate
    return Decision(
        unit=item.unit,
        cluster=label.cites,
        confidence=aggregate.share,
        correct=aligned == label.k3_positive,
        event_probability=_side(aggregate.share, aligned),
        truth_positive=label.k3_positive,
        predicted_positive=aligned,
        consensus=consensus,
    )


def host_vote_cells(
    source: SourceId, vote_count: int, items: Mapping[str, Item], truth: PilotTruth
) -> dict[Kind, CellData]:
    """K1, K2 and K3 cells of JH5 or JH3: the modal share of the first `vote_count` J-H repetitions."""
    labels = {label.nodeid: label for label in truth.labels}
    decisions: dict[Kind, list[Decision]] = {kind: [] for kind in ALIGNMENT_KINDS}
    units: dict[Kind, int] = dict.fromkeys(ALIGNMENT_KINDS, 0)
    without_votes: dict[Kind, int] = dict.fromkeys(ALIGNMENT_KINDS, 0)
    unit_clusters: dict[Kind, list[str]] = {kind: [] for kind in ALIGNMENT_KINDS}
    for request_id in sorted(items):
        item = items[request_id]
        if item.family is not KindFamily.ALIGNMENT or item.nodeid is None:
            continue
        label = labels[item.nodeid]
        kinds = [Kind.K1, Kind.K3] if label.k2_positive is None else list(ALIGNMENT_KINDS)
        for kind in kinds:
            units[kind] += 1
            unit_clusters[kind].append(label.cites)
        aggregate = aggregate_judge_votes(_votes_of(truth, item.nodeid, vote_count), vote_count)
        if aggregate is None:
            for kind in kinds:
                without_votes[kind] += 1
            continue
        consensus = item.nodeid in truth.consensus
        decisions[Kind.K1].append(_judge_k1(item, label, aggregate, consensus))
        k2 = _judge_k2(item, label, aggregate, consensus)
        if k2 is not None:
            decisions[Kind.K2].append(k2)
        decisions[Kind.K3].append(_judge_k3(item, label, aggregate, consensus))
    return {
        kind: CellData(
            kind,
            source,
            HOST_MODEL,
            host_source_of(source),
            tuple(found),
            units[kind],
            {},
            without_votes[kind],
            tuple(unit_clusters[kind]),
        )
        for kind, found in decisions.items()
    }


def _pair_votes(truth: PilotTruth, mutant_id: str, ref: str) -> list[ValidityVote]:
    judges = truth.validity_votes.get(mutant_id, {})
    return [judges[judge][ref] for judge in VALIDITY_JUDGES if ref in judges.get(judge, {})]


def _validity_decision(
    truth: PilotTruth, mutant_id: str, ref: str, aggregate: ValidityAggregate, truth_positive: bool
) -> Decision:
    tags = strata_tags(truth, mutant_id, ref)
    cluster = cluster_of(truth, mutant_id)
    unit = f"{mutant_id}|{ref}"
    if aggregate.modal_answer is None:
        return Decision(
            unit=unit,
            cluster=cluster,
            confidence=0.0,
            correct=False,
            event_probability=aggregate.violation_probability,
            truth_positive=truth_positive,
            has_prediction=False,
            tags=tags,
        )
    violates = aggregate.modal_answer == ANSWER_VIOLATES
    unanimous = aggregate.unanimous_confidence
    return Decision(
        unit=unit,
        cluster=cluster,
        confidence=aggregate.share,
        correct=violates == truth_positive,
        event_probability=aggregate.violation_probability,
        truth_positive=truth_positive,
        predicted_positive=violates,
        alt_confidence=unanimous,
        alt_event_probability=None if unanimous is None else _side(unanimous, violates),
        tags=tags,
    )


def validity_cell(items: Mapping[str, Item], truth: PilotTruth) -> CellData:
    """K4 cell of VH2: the pairs of the measured mutants that have at least one V-H record."""
    decisions: list[Decision] = []
    unit_clusters: list[str] = []
    units = 0
    without_votes = 0
    for request_id in sorted(items):
        item = items[request_id]
        if item.family is not KindFamily.SCREEN or item.mutant_id not in truth.oracle:
            continue
        mutant_id = item.unit
        refs = sorted((item.question_to_ref or {}).values(), key=criterion_number)
        units += len(refs)
        unit_clusters.extend([cluster_of(truth, mutant_id)] * len(refs))
        if mutant_id not in truth.validity_votes:
            without_votes += len(refs)
            continue
        for ref in refs:
            aggregate = aggregate_validity_votes(_pair_votes(truth, mutant_id, ref))
            positive = criterion_number(ref) in truth.oracle[mutant_id]
            decisions.append(_validity_decision(truth, mutant_id, ref, aggregate, positive))
    return CellData(
        Kind.K4,
        SourceId.VH2,
        HOST_MODEL,
        host_source_of(SourceId.VH2),
        tuple(decisions),
        units,
        {},
        without_votes,
        tuple(unit_clusters),
    )


def source2_cells(items: Mapping[str, Item], truth: PilotTruth) -> list[CellData]:
    """Every source-(2) cell: JH5 and JH3 for K1 to K3, VH2 for K4."""
    cells: list[CellData] = []
    for source in (SourceId.JH5, SourceId.JH3):
        cells.extend(host_vote_cells(source, HOST_VOTE_COUNT[source], items, truth).values())
    cells.append(validity_cell(items, truth))
    return cells
